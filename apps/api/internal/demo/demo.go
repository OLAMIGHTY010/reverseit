// Package demo seeds and replays the demonstration scenario. Everything it
// creates is synthetic and every money movement is simulated. It refuses to run
// in a production environment (see cmd/demo).
package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/detect"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/reversal"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// PolicyLabel is attached to every demonstration policy value.
const PolicyLabel = "SYNTHETIC DEMO POLICY - not approved for production"

const (
	// Tokens stand in for account numbers. They are not real accounts. Each
	// scenario transfer has its own payer and payee so that one is never
	// mistaken for a duplicate of another.

	FailRef = "DEMO-FAIL-001"
	DupRef1 = "DEMO-DUP-001"
	DupRef2 = "DEMO-DUP-002"
	UnkRef  = "DEMO-UNK-001"
)

// Policies are the synthetic values the demonstration runs under. They are not
// bank policy, and are installed through the same maker-checker as any change.
var Policies = map[string]int{
	"duplicate.window_seconds":              30,
	"status_query.freshness_seconds":        120,
	"waiting_threshold_seconds.NIP":         300,
	"approval.auto_limit_minor":             5_000_000,
	"reversal.auto_eligible.FAIL001":        1,
	"reversal.simulated_execution_enabled":  1,
	"approval.validity_seconds":             3600, // approvals last an hour in the demonstration
	"approval.limit_minor.SUPERVISOR":       100_000_000,
	"approval.limit_minor.REVERSAL_OFFICER": 5_000_000,
}

// Env holds the services the scenario uses.
type Env struct {
	Pool     *db.Pool
	Config   *configsvc.Service
	Cases    *cases.Service
	Detector *detect.Detector
	Ingest   *txn.Service
	Reversal *reversal.Service
	Auth     *identity.Authenticator
	Store    *identity.PGUserStore
}

// NewEnv builds the services around a pool. All reversal sources are simulated.
func NewEnv(ctx context.Context, pool *db.Pool) (*Env, error) {
	policy, err := authz.Load(ctx, pool)
	if err != nil {
		return nil, err
	}
	app := &audit.Appender{}
	cfg := &configsvc.Service{Audit: app}
	cs := &cases.Service{Audit: app, Config: cfg}
	ing := &txn.Service{}
	kill := &killswitch.Service{Audit: app}
	store := &identity.PGUserStore{Pool: pool}
	return &Env{Pool: pool, Config: cfg, Cases: cs, Ingest: ing, Store: store,
		Detector: &detect.Detector{Cases: cs, Config: cfg, Kill: kill},
		Reversal: &reversal.Service{Pool: pool, Config: cfg, Kill: kill, Cases: cs, Audit: app, Txn: ing, Policy: policy,
			SoD: authz.SoD{Conflicts: &authz.PGConflictChecker{Pool: pool}}, Status: reversal.SimStatus{}, Ledger: reversal.SimLedger{}, Provider: reversal.SimProvider{}},
		Auth: &identity.Authenticator{Store: store},
	}, nil
}

// Operator is a seeded demonstration user.
type Operator struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Roles     []string  `json:"roles"`
	OTPAuth   string    `json:"otpauth_uri"`
	secret    []byte
	Principal identity.Principal `json:"-"`
}

// Seeded describes what Seed made.
type Seeded struct {
	Institution string    `json:"institution"`
	Tenant      uuid.UUID `json:"tenant_id"`
	Officer     Operator  `json:"officer"`
	Supervisor  Operator  `json:"supervisor"`
}

// Secret returns the operator's authenticator secret, for tests and the demo
// command's one-time-code helper.
func (o Operator) Secret() []byte { return o.secret }

// Seed creates the demonstration institution, two operators with development
// authenticators, and the synthetic policies. It is safe to run again: users
// and policies that already exist are kept, authenticators are re-enrolled.
func Seed(ctx context.Context, env *Env, code, password string) (Seeded, error) {
	out := Seeded{Institution: code}
	make := func(email, name string, roles []string) (Operator, error) {
		tenant, id, err := identity.Bootstrap(ctx, env.Pool, identity.BootstrapSpec{InstitutionCode: code, InstitutionName: "Demo Bank (synthetic)",
			Email: email, DisplayName: name, Password: password, Roles: roles})
		if err != nil {
			// Already seeded: look the user up and reuse it.
			tid, terr := env.Store.TenantIDByCode(ctx, code)
			if terr != nil {
				return Operator{}, err
			}
			u, ferr := env.Store.FindByEmail(ctx, tid, email)
			if ferr != nil {
				return Operator{}, err
			}
			tenant, id = tid, u.ID
		}
		out.Tenant = tenant
		secret, err := identity.NewTOTPSecret()
		if err != nil {
			return Operator{}, err
		}
		if err := env.Store.EnrollTOTP(ctx, tenant, id, secret); err != nil {
			return Operator{}, err
		}
		return Operator{ID: id, Email: email, Roles: roles, OTPAuth: identity.OTPAuthURI(code, email, secret), secret: secret,
			Principal: identity.Principal{UserID: id, TenantID: tenant, Roles: roles, MFA: true}}, nil
	}
	var err error
	if out.Officer, err = make("officer@demo.example", "Demo Officer", []string{"REVERSAL_OFFICER", "TXN_ANALYST"}); err != nil {
		return out, fmt.Errorf("demo: officer: %w", err)
	}
	if out.Supervisor, err = make("supervisor@demo.example", "Demo Supervisor", []string{"SUPERVISOR"}); err != nil {
		return out, fmt.Errorf("demo: supervisor: %w", err)
	}
	for k, v := range Policies {
		if err := installPolicy(ctx, env, out, k, v); err != nil {
			return out, fmt.Errorf("demo: policy %s: %w", k, err)
		}
	}
	return out, nil
}

func installPolicy(ctx context.Context, env *Env, s Seeded, key string, value int) error {
	raw, _ := json.Marshal(value)
	return env.Pool.WithTenant(ctx, s.Tenant, func(tx pgx.Tx) error {
		if cur, err := env.Config.Effective(ctx, tx, s.Tenant, key, time.Now()); err == nil && cur.Value == int64(value) {
			return nil
		} else if err != nil && !errors.Is(err, configsvc.ErrNotConfigured) {
			return err
		}
		v, err := env.Config.Propose(ctx, tx, s.Tenant, s.Officer.ID, configsvc.ProposeInput{Key: key, Value: raw,
			SourceReference: PolicyLabel, EffectiveFrom: time.Now(), VerifyStatus: "unverified", Reason: "synthetic policy for the demonstration"})
		if err != nil {
			return err
		}
		_, err = env.Config.Decide(ctx, tx, s.Tenant, s.Supervisor.ID, key, v.Version, true, "")
		return err
	})
}

// Reset removes the scenario's transactions, cases, reversals, notifications and
// simulated ledger, so Play can run again from nothing. Users, policies and the
// audit trail are kept.
func Reset(ctx context.Context, pool *db.Pool, tenant uuid.UUID) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		return err
	}
	for _, t := range []string{"sim_provider_requests", "sim_provider_script", "sim_ledger_entries", "sim_status_script", "approvals", "reversals",
		"notifications", "case_evidence", "case_events", "cases", "transaction_events", "quarantined_events", "transactions", "outbox_events", "processed_events"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+t+" WHERE tenant_id = $1", tenant); err != nil {
			return fmt.Errorf("demo: reset %s: %w", t, err)
		}
	}
	return tx.Commit(ctx)
}

// Step is one thing that happened, in order.
type Step struct {
	N      int    `json:"n"`
	What   string `json:"what"`
	Detail string `json:"detail,omitempty"`
}

// Report is what Play returns. Nothing in it is a real transaction.
type Report struct {
	Steps []Step `json:"steps"`
	Fail  Result `json:"failed_transfer"`
	Dup   Result `json:"duplicate"`
	Unk   Result `json:"unknown"`
	// PendingApproval is the approval id a supervisor can decide for the duplicate.
	PendingApproval string `json:"pending_approval_id,omitempty"`
	Simulated       bool   `json:"simulated"`
}

// Result is how one scenario transaction ended.
type Result struct {
	InstructionRef string `json:"instruction_ref"`
	CaseRef        string `json:"case_ref"`
	CaseStatus     string `json:"case_status"`
	Outcome        string `json:"outcome"`
	Financial      string `json:"financial_status,omitempty"`
}

type player struct {
	env    *Env
	tenant uuid.UUID
	n      int
	rep    *Report
}

func (p *player) step(what, detail string) {
	p.n++
	p.rep.Steps = append(p.rep.Steps, Step{N: p.n, What: what, Detail: detail})
}

// party is the synthetic payer and payee of one scenario transfer.
type party struct {
	payer, payee string
	amount       int64
}

var (
	failParty = party{"acct_00000000000000000000000000d3a001", "acct_00000000000000000000000000d3b001", 1850000}
	dupParty  = party{"acct_00000000000000000000000000d3a002", "acct_00000000000000000000000000d3b002", 920000}
	unkParty  = party{"acct_00000000000000000000000000d3a003", "acct_00000000000000000000000000d3b003", 2750000}
)

func (p *player) event(ctx context.Context, who party, ref, typ string, at time.Time, session string) (uuid.UUID, error) {
	var res txn.Result
	err := p.env.Pool.WithTenant(ctx, p.tenant, func(tx pgx.Tx) error {
		var err error
		res, err = p.env.Ingest.Ingest(ctx, tx, p.tenant, txn.Envelope{SourceSystem: "DEMO_SWITCH", SourceEventRef: ref + ":" + typ, EventType: typ, SourceTS: at,
			Txn: txn.Core{InstructionRef: ref, Channel: "MOBILE", Rail: "NIP", AmountMinor: txn.Amount(who.amount), Currency: "NGN",
				DebitAccountToken: who.payer, BeneficiaryToken: who.payee, SessionID: session}})
		return err
	})
	return res.TxnID, err
}

func (p *player) detect(ctx context.Context, id uuid.UUID, code string) (uuid.UUID, error) {
	var found uuid.UUID
	err := p.env.Pool.WithTenant(ctx, p.tenant, func(tx pgx.Tx) error {
		fs, err := p.env.Detector.Evaluate(ctx, tx, p.tenant, id)
		for _, f := range fs {
			if f.Case.Classification == code {
				found = f.Case.ID
			}
		}
		return err
	})
	if err == nil && found == uuid.Nil {
		err = fmt.Errorf("demo: the detector opened no %s case", code)
	}
	return found, err
}

func (p *player) result(ctx context.Context, ref string, caseID uuid.UUID, outcome string) (Result, error) {
	r := Result{InstructionRef: ref, Outcome: outcome}
	err := p.env.Pool.WithTenant(ctx, p.tenant, func(tx pgx.Tx) error {
		c, err := p.env.Cases.Get(ctx, tx, p.tenant, caseID)
		if err != nil {
			return err
		}
		r.CaseRef, r.CaseStatus = c.Ref, c.Status
		revs, err := p.env.Reversal.ForCase(ctx, tx, p.tenant, caseID)
		if len(revs) > 0 {
			r.Financial = revs[0].FinancialStatus
		}
		return err
	})
	return r, err
}

// Play runs the scenario against an empty tenant (call Reset first). It
// ingests events as the simulated switch would, lets the detector and the
// reversal automation do their work as the worker does, and reports what
// happened. The duplicate is left waiting for a person to approve. The unknown
// transaction is held and never reversed.
func Play(ctx context.Context, env *Env, tenant uuid.UUID) (Report, error) {
	rep := Report{Simulated: true}
	p := &player{env: env, tenant: tenant, rep: &rep}
	at := time.Now().Add(-10 * time.Minute)
	seed := func(who party, ref, outcome string) error {
		return env.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			if err := reversal.SeedDebit(ctx, tx, tenant, ref, who.amount, "NGN"); err != nil {
				return err
			}
			return reversal.ScriptStatus(ctx, tx, tenant, ref, outcome, false, false, 0)
		})
	}

	// 1. A transfer is debited and the beneficiary credit fails.
	for i, typ := range []string{txn.EvDebitPosted, txn.EvTransferSent} {
		if _, err := p.event(ctx, failParty, FailRef, typ, at.Add(time.Duration(i)*time.Second), "S-"+FailRef); err != nil {
			return rep, err
		}
	}
	p.step("Customer debit posted and transfer sent", FailRef)
	failID, err := p.event(ctx, failParty, FailRef, txn.EvCreditFailed, at.Add(2*time.Second), "S-"+FailRef)
	if err != nil {
		return rep, err
	}
	p.step("Beneficiary credit failed; ingestion accepted the event", FailRef)
	if err := seed(failParty, FailRef, reversal.OutcomeFailure); err != nil {
		return rep, err
	}
	failCase, err := p.detect(ctx, failID, cases.CodeNoCredit)
	if err != nil {
		return rep, err
	}
	p.step("Detector found a confirmed failure and opened a case", "FAIL001")
	auto, err := env.Reversal.AutoReverse(ctx, tenant, failCase)
	if err != nil {
		return rep, err
	}
	p.step("Status verified, eight checks passed, simulated reversal executed and checked against the ledger", auto.Outcome)
	if rep.Fail, err = p.result(ctx, FailRef, failCase, auto.Outcome); err != nil {
		return rep, err
	}

	// 2. The same transfer is debited twice.
	if _, err := p.event(ctx, dupParty, DupRef1, txn.EvDebitPosted, at, "S-"+DupRef1); err != nil {
		return rep, err
	}
	dupID, err := p.event(ctx, dupParty, DupRef2, txn.EvDebitPosted, at.Add(3*time.Second), "S-"+DupRef1)
	if err != nil {
		return rep, err
	}
	if _, err := p.event(ctx, dupParty, DupRef1, txn.EvCreditConfirmed, at.Add(time.Second), "S-"+DupRef1); err != nil {
		return rep, err
	}
	for _, ref := range []string{DupRef1, DupRef2} {
		if err := seed(dupParty, ref, reversal.OutcomeFailure); err != nil {
			return rep, err
		}
	}
	dupCase, err := p.detect(ctx, dupID, cases.CodeDuplicate)
	if err != nil {
		return rep, err
	}
	p.step("A second debit for the same transfer was detected", "DUP001")
	auto, err = env.Reversal.AutoReverse(ctx, tenant, dupCase)
	if err != nil {
		return rep, err
	}
	p.step("Duplicates are never reversed automatically; waiting for an approver", auto.Outcome)
	if auto.Reversal != nil {
		rep.PendingApproval = auto.Reversal.ApprovalID.String()
	}
	if rep.Dup, err = p.result(ctx, DupRef2, dupCase, auto.Outcome); err != nil {
		return rep, err
	}

	// 3. A transfer whose outcome nobody knows.
	for i, typ := range []string{txn.EvDebitPosted, txn.EvTransferSent} {
		if _, err := p.event(ctx, unkParty, UnkRef, typ, at.Add(time.Duration(i)*time.Second), "S-"+UnkRef); err != nil {
			return rep, err
		}
	}
	unkID, err := p.event(ctx, unkParty, UnkRef, txn.EvTransferTimeout, at.Add(2*time.Second), "S-"+UnkRef)
	if err != nil {
		return rep, err
	}
	if err := seed(unkParty, UnkRef, reversal.OutcomeUnknown); err != nil {
		return rep, err
	}
	unkCase, err := p.detect(ctx, unkID, cases.CodeUnknown)
	if err != nil {
		return rep, err
	}
	p.step("The switch timed out; the outcome is unknown", "UNK001")
	auto, err = env.Reversal.AutoReverse(ctx, tenant, unkCase)
	if err != nil {
		return rep, err
	}
	p.step("Held: an unknown outcome is never reversed", auto.Outcome)
	rep.Unk, err = p.result(ctx, UnkRef, unkCase, auto.Outcome)
	return rep, err
}
