package demo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/events"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/reversal"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// A failed transfer arrives while automation is frozen. Evidence is kept, nothing
// is opened or reversed, and after an authorised unfreeze the scenario completes
// as it would have: one case, one reversal, one ledger posting, one verification.
func TestAFailedTransferThatArrivesDuringAFreezeIsResolvedOnceAfterTheUnfreeze(t *testing.T) {
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
	if _, err := Seed(ctx, env, code, "a long synthetic demo passphrase"); err != nil {
		t.Fatal(err)
	}
	if err := Reset(ctx, pool, tenant); err != nil {
		t.Fatal(err)
	}
	kill := env.Detector.Kill
	run := func(fn func(tx pgx.Tx) error) {
		t.Helper()
		if err := pool.WithTenant(ctx, tenant, fn); err != nil {
			t.Fatal(err)
		}
	}
	run(func(tx pgx.Tx) error {
		_, err := kill.Freeze(ctx, tx, tenant, uuid.New(), killswitch.Target{Scope: killswitch.ScopeGlobal}, "incident under review")
		return err
	})

	rep := &Report{}
	p := &player{env: env, tenant: tenant, rep: rep}
	at := timeAgo(10)
	for i, typ := range []string{txn.EvDebitPosted, txn.EvTransferSent} {
		if _, err := p.event(ctx, failParty, FailRef, typ, at.Add(secs(i)), "S-"+FailRef); err != nil {
			t.Fatal(err)
		}
	}
	id, err := p.event(ctx, failParty, FailRef, txn.EvCreditFailed, at.Add(secs(2)), "S-"+FailRef)
	if err != nil {
		t.Fatal(err)
	}
	run(func(tx pgx.Tx) error {
		if err := reversal.SeedDebit(ctx, tx, tenant, FailRef, failParty.amount, "NGN"); err != nil {
			return err
		}
		return reversal.ScriptStatus(ctx, tx, tenant, FailRef, reversal.OutcomeFailure, false, false, 0)
	})
	run(func(tx pgx.Tx) error { _, err := env.Detector.Evaluate(ctx, tx, tenant, id); return err })
	if _, err := env.Reversal.Sweep(ctx, tenant); err != nil {
		t.Fatal(err)
	}

	if n := count(t, env, tenant, `SELECT count(*) FROM transaction_events WHERE tenant_id = $1`); n != 3 {
		t.Fatalf("evidence lost during the freeze: %d", n)
	}
	for sql, what := range map[string]string{
		`SELECT count(*) FROM cases WHERE tenant_id = $1`:                                           "cases",
		`SELECT count(*) FROM reversals WHERE tenant_id = $1`:                                       "reversals",
		`SELECT count(*) FROM sim_provider_requests WHERE tenant_id = $1`:                           "provider requests",
		`SELECT count(*) FROM sim_ledger_entries WHERE tenant_id = $1 AND kind = 'REVERSAL_CREDIT'`: "ledger credits",
	} {
		if n := count(t, env, tenant, sql); n != 0 {
			t.Fatalf("%d %s while frozen", n, what)
		}
	}
	if n := count(t, env, tenant, `SELECT count(*) FROM deferred_work WHERE tenant_id = $1 AND resumed_at IS NULL`); n != 1 {
		t.Fatalf("held work: %d", n)
	}

	run(func(tx pgx.Tx) error {
		seq, err := kill.RequestUnfreeze(ctx, tx, tenant, uuid.New(), killswitch.Target{Scope: killswitch.ScopeGlobal}, "incident closed")
		if err != nil {
			return err
		}
		return kill.DecideUnfreeze(ctx, tx, tenant, uuid.New(), seq, true, "confirmed closed")
	})
	for i := 0; i < 2; i++ { // the second pass must change nothing
		if _, err := env.Detector.ResumeAll(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if _, err := env.Reversal.Sweep(ctx, tenant); err != nil {
			t.Fatal(err)
		}
	}

	var status, financial string
	if err := pool.QueryRow(ctx, `SELECT c.status, r.financial_status FROM cases c JOIN reversals r ON r.tenant_id = c.tenant_id AND r.case_id = c.id WHERE c.tenant_id = $1`,
		tenant).Scan(&status, &financial); err != nil {
		t.Fatal(err)
	}
	if status != "RESOLVED" || financial != "VERIFIED" {
		t.Fatalf("after the unfreeze: case %s, reversal %s", status, financial)
	}
	if n := count(t, env, tenant, `SELECT count(*) FROM sim_ledger_entries WHERE tenant_id = $1 AND kind = 'REVERSAL_CREDIT'`); n != 1 {
		t.Fatalf("%d ledger credits", n)
	}
	if n := count(t, env, tenant, `SELECT count(*) FROM sim_provider_requests WHERE tenant_id = $1`); n != 1 {
		t.Fatalf("%d provider requests", n)
	}
	if n := count(t, env, tenant, `SELECT count(*) FROM notifications WHERE tenant_id = $1 AND audience = 'customer'`); n != 2 {
		t.Fatalf("expected the detected and verified messages once each, got %d", n)
	}
	if n := countEvents(t, env, tenant, events.ReversalVerified); n != 1 {
		t.Fatalf("%d verified events", n)
	}
	run(func(tx pgx.Tx) error {
		if res, err := audit.Verify(ctx, tx, tenant); err != nil || !res.OK {
			t.Fatalf("audit chain: %+v %v", res, err)
		}
		return nil
	})
}

func timeAgo(minutes int) time.Time { return time.Now().Add(-time.Duration(minutes) * time.Minute) }
func secs(n int) time.Duration      { return time.Duration(n) * time.Second }
