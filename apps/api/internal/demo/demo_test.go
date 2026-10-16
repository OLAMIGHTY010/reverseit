package demo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

func count(t *testing.T, env *Env, tenant uuid.UUID, sql string, args ...any) int {
	t.Helper()
	var n int
	args = append([]any{tenant}, args...)
	if err := env.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTheDemonstrationScenarioRunsEndToEndAndCanBeRepeated(t *testing.T) {
	pool := dbtest.Pool(t)
	tenant := dbtest.Tenant(t, pool)
	var code string
	if err := pool.QueryRow(context.Background(), `SELECT code FROM institutions WHERE id = $1`, tenant).Scan(&code); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	env, err := NewEnv(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	signer, _, _ := identity.GenerateKey()
	sg, _ := identity.NewSigner("k1", signer, time.Hour)
	env.Auth.Signer = sg

	seeded, err := Seed(ctx, env, code, "a long synthetic demo passphrase")
	if err != nil {
		t.Fatal(err)
	}
	again, err := Seed(ctx, env, code, "a long synthetic demo passphrase")
	if err != nil || again.Officer.ID != seeded.Officer.ID {
		t.Fatalf("seeding twice must keep the same users: %v", err)
	}
	seeded = again // the authenticator is re-enrolled on every seed
	if !strings.Contains(seeded.Supervisor.OTPAuth, "synthetic") {
		t.Fatalf("the authenticator must be labelled synthetic: %s", seeded.Supervisor.OTPAuth)
	}
	// Every installed policy carries the synthetic label and is unverified.
	if n := count(t, env, tenant, `SELECT count(*) FROM config_versions WHERE tenant_id = $1 AND (source_reference <> $2 OR verify_status <> 'unverified')`, PolicyLabel); n != 0 {
		t.Fatalf("%d policy versions are not labelled synthetic", n)
	}

	for run := 1; run <= 2; run++ {
		if err := Reset(ctx, pool, tenant); err != nil {
			t.Fatal(err)
		}
		rep, err := Play(ctx, env, tenant)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if len(rep.Steps) < 8 || !rep.Simulated {
			t.Fatalf("run %d: %+v", run, rep)
		}

		if n := count(t, env, tenant, `SELECT count(*) FROM cases WHERE tenant_id = $1`); n != 3 {
			t.Fatalf("run %d: the scenario should open exactly three cases, got %d", run, n)
		}

		// The failed transfer is reversed and verified, and the case is resolved.
		if rep.Fail.Outcome != "reversed" || rep.Fail.Financial != "VERIFIED" || rep.Fail.CaseStatus != "RESOLVED" {
			t.Fatalf("run %d failed transfer: %+v", run, rep.Fail)
		}
		if n := count(t, env, tenant, `SELECT count(*) FROM sim_ledger_entries WHERE tenant_id = $1 AND instruction_ref = $2 AND kind = 'REVERSAL_CREDIT'`, FailRef); n != 1 {
			t.Fatalf("run %d: expected one ledger credit for the failed transfer, got %d", run, n)
		}
		// The customer is told only after verification.
		if n := count(t, env, tenant, `SELECT count(*) FROM notifications n JOIN cases c ON c.tenant_id = n.tenant_id AND c.id = n.case_id
			WHERE n.tenant_id = $1 AND c.case_ref = $2 AND n.audience = 'customer'`, rep.Fail.CaseRef); n != 2 {
			t.Fatalf("run %d: expected a detected and a verified customer message, got %d", run, n)
		}

		// The duplicate waits for a person; nothing moved.
		if rep.Dup.Outcome != "awaiting_approval" || rep.Dup.CaseStatus != "AWAITING_APPROVAL" || rep.PendingApproval == "" {
			t.Fatalf("run %d duplicate: %+v", run, rep.Dup)
		}
		if n := count(t, env, tenant, `SELECT count(*) FROM sim_ledger_entries WHERE tenant_id = $1 AND instruction_ref = $2 AND kind = 'REVERSAL_CREDIT'`, DupRef2); n != 0 {
			t.Fatalf("run %d: a duplicate moved money without approval", run)
		}

		// The unknown outcome is held and has no reversal at all.
		if rep.Unk.Outcome != "held" || rep.Unk.Financial != "" {
			t.Fatalf("run %d unknown: %+v", run, rep.Unk)
		}
		if n := count(t, env, tenant, `SELECT count(*) FROM reversals r JOIN transactions t ON t.tenant_id = r.tenant_id AND t.id = r.txn_id
			WHERE r.tenant_id = $1 AND t.instruction_ref = $2`, UnkRef); n != 0 {
			t.Fatalf("run %d: an unknown outcome was reversed", run)
		}

		if run == 2 {
			// A supervisor signs in with a one-time code and approves the duplicate.
			res, err := env.Auth.LoginOTP(ctx, code, seeded.Supervisor.Email, "a long synthetic demo passphrase", identity.TOTPCode(seeded.Supervisor.Secret(), time.Now()))
			if err != nil || !res.MFA {
				t.Fatalf("supervisor login: %v %+v", err, res)
			}
			approver := identity.Principal{UserID: res.UserID, TenantID: tenant, Roles: res.Roles, MFA: res.MFA}
			id, _ := uuid.Parse(rep.PendingApproval)
			rev, err := env.Reversal.Decide(ctx, tenant, &approver, id, true, "checked against the statement")
			if err != nil {
				t.Fatal(err)
			}
			if rev, err = env.Reversal.Execute(ctx, tenant, rev.ID); err != nil {
				t.Fatal(err)
			}
			if rev, err = env.Reversal.Advance(ctx, tenant, rev.ID); err != nil || rev.FinancialStatus != "VERIFIED" {
				t.Fatalf("approved duplicate: %v %+v", err, rev)
			}
			// A decided approval cannot be decided again.
			if _, err := env.Reversal.Decide(ctx, tenant, &approver, id, true, "again"); err == nil {
				t.Fatal("a decided approval accepted another decision")
			}
		}
	}

	err = pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		res, err := audit.Verify(ctx, tx, tenant)
		if err == nil && !res.OK {
			t.Errorf("the audit chain does not verify: %+v", res)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
