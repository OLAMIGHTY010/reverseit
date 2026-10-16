package reversal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
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
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

const (
	tokA = "acct_0123456789abcdef0123456789abcdef"
	tokB = "acct_fedcba9876543210fedcba9876543210"
)

type rig struct {
	pool   *db.Pool
	svc    *Service
	ing    *txn.Service
	det    *detect.Detector
	cfg    *configsvc.Service
	kill   *killswitch.Service
	tenant uuid.UUID
	n      int
	// freezeAfterSetup, when set, is applied by approvedUnderFreeze (freeze tests).
	freezeAfterSetup func()
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := dbtest.Pool(t)
	policy, err := authz.Load(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	app := &audit.Appender{}
	cfg := &configsvc.Service{Audit: app}
	cs := &cases.Service{Audit: app, Config: cfg}
	kill := &killswitch.Service{Audit: app}
	ing := &txn.Service{}
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), ing: ing, cfg: cfg, kill: kill,
		det: &detect.Detector{Cases: cs, Config: cfg}}
	r.svc = &Service{Pool: pool, Config: cfg, Kill: kill, Cases: cs, Audit: app, Txn: ing, Policy: policy,
		SoD:    authz.SoD{Conflicts: &authz.PGConflictChecker{Pool: pool}},
		Status: SimStatus{}, Ledger: SimLedger{}, Provider: SimProvider{}}
	return r
}

func (r *rig) tx(t *testing.T, fn func(tx pgx.Tx) error) {
	t.Helper()
	if err := r.pool.WithTenant(context.Background(), r.tenant, fn); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) set(t *testing.T, key string, value int) {
	t.Helper()
	raw, _ := json.Marshal(value)
	r.tx(t, func(tx pgx.Tx) error {
		v, err := r.cfg.Propose(context.Background(), tx, r.tenant, uuid.New(), configsvc.ProposeInput{Key: key, Value: raw,
			SourceReference: "SYNTHETIC DEMO POLICY - not approved for production", EffectiveFrom: time.Now(), VerifyStatus: "unverified", Reason: "synthetic policy for tests"})
		if err != nil {
			return err
		}
		_, err = r.cfg.Decide(context.Background(), tx, r.tenant, uuid.New(), key, v.Version, true, "")
		return err
	})
}

// policy installs the synthetic policy the demonstration uses.
func (r *rig) policy(t *testing.T) {
	t.Helper()
	for k, v := range map[string]int{
		"duplicate.window_seconds": 30, "status_query.freshness_seconds": 120, "waiting_threshold_seconds.NIP": 300,
		"approval.auto_limit_minor": 5_000_000, "reversal.auto_eligible.FAIL001": 1, "reversal.simulated_execution_enabled": 1,
		"approval.limit_minor.SUPERVISOR": 100_000_000, "approval.validity_seconds": 3600, "approval.limit_minor.REVERSAL_OFFICER": 5_000_000,
	} {
		r.set(t, k, v)
	}
}

func (r *rig) event(t *testing.T, ref, typ string, at time.Time, mut ...func(*txn.Envelope)) uuid.UUID {
	t.Helper()
	r.n++
	e := txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: fmt.Sprintf("EV-%s-%d", ref, r.n), EventType: typ, SourceTS: at,
		Txn: txn.Core{InstructionRef: ref, Channel: "MOBILE", Rail: "NIP", AmountMinor: 1850000, Currency: "NGN", DebitAccountToken: tokA,
			BeneficiaryBank: "TEST BANK", BeneficiaryToken: tokB, SessionID: "S-" + ref}}
	for _, m := range mut {
		m(&e)
	}
	var res txn.Result
	r.tx(t, func(tx pgx.Tx) error {
		var err error
		res, err = r.ing.Ingest(context.Background(), tx, r.tenant, e)
		return err
	})
	return res.TxnID
}

type scn struct {
	ref    string
	txnID  uuid.UUID
	caseID uuid.UUID
}

// failed builds a failed transfer in the past, with the matching ledger debit and a
// status source that confirms the failure, and returns its detected case.
func (r *rig) failed(t *testing.T, ref string, ageMinutes int, mut ...func(*txn.Envelope)) scn {
	t.Helper()
	at := time.Now().Add(-time.Duration(ageMinutes) * time.Minute)
	r.event(t, ref, txn.EvDebitPosted, at, mut...)
	r.event(t, ref, txn.EvTransferSent, at.Add(time.Second), mut...)
	id := r.event(t, ref, txn.EvCreditFailed, at.Add(2*time.Second), mut...)
	r.tx(t, func(tx pgx.Tx) error {
		if err := SeedDebit(context.Background(), tx, r.tenant, ref, 1850000, "NGN"); err != nil {
			return err
		}
		return ScriptStatus(context.Background(), tx, r.tenant, ref, OutcomeFailure, false, false, 0)
	})
	return scn{ref: ref, txnID: id, caseID: r.detect(t, id, cases.CodeNoCredit)}
}

func (r *rig) detect(t *testing.T, id uuid.UUID, code string) uuid.UUID {
	t.Helper()
	var found uuid.UUID
	r.tx(t, func(tx pgx.Tx) error {
		fs, err := r.det.Evaluate(context.Background(), tx, r.tenant, id)
		for _, f := range fs {
			if f.Case.Classification == code {
				found = f.Case.ID
			}
		}
		return err
	})
	if found == uuid.Nil {
		t.Fatalf("no %s case detected", code)
	}
	return found
}

func (r *rig) kase(t *testing.T, id uuid.UUID) cases.Case {
	t.Helper()
	var c cases.Case
	r.tx(t, func(tx pgx.Tx) error {
		var err error
		c, err = r.svc.Cases.Get(context.Background(), tx, r.tenant, id)
		return err
	})
	return c
}

func (r *rig) reversals(t *testing.T, caseID uuid.UUID) []Reversal {
	t.Helper()
	var out []Reversal
	r.tx(t, func(tx pgx.Tx) error {
		var err error
		out, err = r.svc.ForCase(context.Background(), tx, r.tenant, caseID)
		return err
	})
	return out
}

func (r *rig) ledgerCount(t *testing.T, ref string) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM sim_ledger_entries WHERE tenant_id = $1 AND instruction_ref = $2 AND kind = 'REVERSAL_CREDIT'`, r.tenant, ref).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) providerRequests(t *testing.T, ref string) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM sim_provider_requests WHERE tenant_id = $1 AND instruction_ref = $2`, r.tenant, ref).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) txnState(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var s string
	if err := r.pool.QueryRow(context.Background(), `SELECT lifecycle_state FROM transactions WHERE tenant_id = $1 AND id = $2`, r.tenant, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (r *rig) notes(t *testing.T, caseID uuid.UUID, audience string) []notify.Stored {
	t.Helper()
	var out []notify.Stored
	r.tx(t, func(tx pgx.Tx) error {
		var err error
		out, err = notify.ForCase(context.Background(), tx, r.tenant, caseID, audience)
		return err
	})
	return out
}

func (r *rig) script(t *testing.T, status string, credited, settled bool, age int, ref string) {
	t.Helper()
	r.tx(t, func(tx pgx.Tx) error {
		return ScriptStatus(context.Background(), tx, r.tenant, ref, status, credited, settled, age)
	})
}

func (r *rig) provider(t *testing.T, ref, behaviour string) {
	t.Helper()
	r.tx(t, func(tx pgx.Tx) error { return ScriptProvider(context.Background(), tx, r.tenant, ref, behaviour) })
}

func (r *rig) auto(t *testing.T, caseID uuid.UUID) AutoResult {
	t.Helper()
	res, err := r.svc.AutoReverse(context.Background(), r.tenant, caseID)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (r *rig) user(roles ...string) *identity.Principal {
	return &identity.Principal{UserID: uuid.New(), TenantID: r.tenant, Roles: roles, MFA: true}
}

func (r *rig) chainOK(t *testing.T) {
	t.Helper()
	r.tx(t, func(tx pgx.Tx) error {
		res, err := audit.Verify(context.Background(), tx, r.tenant)
		if err != nil || !res.OK {
			t.Fatalf("audit chain: %+v %v", res, err)
		}
		return nil
	})
}

func hasFailed(g Gate, id int) bool {
	for _, c := range g.Failed() {
		if c.ID == id {
			return true
		}
	}
	return false
}

// ---- the happy path -----------------------------------------------------------

func TestConfirmedFailureIsReversedVerifiedAndOnlyThenReportedToTheCustomer(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-H1", 10)

	res := r.auto(t, s.caseID)
	if res.Outcome != "reversed" || res.Reversal == nil {
		t.Fatalf("%+v", res)
	}
	rev := res.Reversal
	if rev.AcceptedStatus != "ACCEPTED" || rev.ProcessingStatus != "COMPLETED" || rev.FinancialStatus != "VERIFIED" || rev.ApprovalStatus != "NOT_REQUIRED" ||
		rev.RequestedByType != "system" || rev.VerifiedAt == nil || rev.PostedAt == nil || !strings.HasPrefix(rev.Ref, "RV-") {
		t.Fatalf("tracks: %+v", rev)
	}
	if len(rev.Gate.Checks) != 8 || len(rev.Gate.Failed()) != 0 || rev.Gate.Verdict != PassAuto || rev.Gate.StatusOutcome != OutcomeFailure {
		t.Fatalf("gate: %+v", rev.Gate)
	}
	if got := r.kase(t, s.caseID); got.Status != cases.StResolved || got.ResolvedAt == nil {
		t.Fatalf("case: %+v", got)
	}
	if st := r.txnState(t, s.txnID); st != txn.StateReversed {
		t.Fatalf("transaction is %s", st)
	}
	if r.ledgerCount(t, "INS-H1") != 1 || r.providerRequests(t, "INS-H1") != 1 {
		t.Fatalf("ledger postings %d, provider requests %d; want 1 and 1", r.ledgerCount(t, "INS-H1"), r.providerRequests(t, "INS-H1"))
	}
	customer := r.notes(t, s.caseID, notify.Customer)
	if len(customer) != 2 || customer[0].Stage != notify.StageDetected || customer[1].Stage != notify.StageVerified || !strings.Contains(customer[1].Body, "reversed") {
		t.Fatalf("customer messages: %+v", customer)
	}
	r.chainOK(t)

	// A second run is a no-op: nothing is reversed twice.
	again, err := r.svc.AutoReverse(context.Background(), r.tenant, s.caseID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Outcome == "reversed" && again.Reversal.ID != rev.ID {
		t.Fatalf("a second reversal was created: %+v", again)
	}
	if r.ledgerCount(t, "INS-H1") != 1 || len(r.reversals(t, s.caseID)) != 1 {
		t.Fatal("repeating the automatic path changed the books")
	}
}

func TestTheThreeTracksStaySeparateUntilTheLedgerConfirms(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-T3", 10)
	out, err := r.svc.Request(context.Background(), r.tenant, RequestInput{CaseID: s.caseID, IdempotencyKey: "k-t3", ReasonCode: "TEST", Who: Requester{}})
	if err != nil || out.Blocked || out.Reversal.ApprovalStatus != "NOT_REQUIRED" {
		t.Fatalf("%+v %v", out, err)
	}
	if rev := out.Reversal; rev.AcceptedStatus != "PENDING" || rev.ProcessingStatus != "NOT_STARTED" || rev.FinancialStatus != "UNVERIFIED" {
		t.Fatalf("after requesting: %+v", rev)
	}
	rev, err := r.svc.Execute(context.Background(), r.tenant, out.Reversal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rev.AcceptedStatus != "ACCEPTED" || rev.ProcessingStatus != "IN_PROGRESS" || rev.FinancialStatus != "UNVERIFIED" {
		t.Fatalf("an acknowledgement is not a refund: %+v", rev)
	}
	if r.kase(t, s.caseID).Status != cases.StRemediationSubmitted || r.ledgerCount(t, "INS-T3") != 0 {
		t.Fatal("the case or ledger moved before the provider finished")
	}
	for _, n := range r.notes(t, s.caseID, notify.Customer) {
		if n.Stage == notify.StageVerified {
			t.Fatal("the customer was told the money was returned before it was verified")
		}
	}
	// Executing again must not submit again.
	if _, err := r.svc.Execute(context.Background(), r.tenant, out.Reversal.ID); err != nil {
		t.Fatal(err)
	}
	if r.providerRequests(t, "INS-T3") != 1 {
		t.Fatal("a second Execute submitted again")
	}
	rev, err = r.svc.Advance(context.Background(), r.tenant, out.Reversal.ID)
	if err != nil || rev.FinancialStatus != "VERIFIED" {
		t.Fatalf("%+v %v", rev, err)
	}
}

// ---- the gate blocks ------------------------------------------------------------

func TestUnknownPendingConflictingAndSuccessfulOutcomesAreNeverReversed(t *testing.T) {
	for _, outcome := range []string{OutcomeUnknown, OutcomePending, OutcomeConflicting, OutcomeSuccess} {
		t.Run(outcome, func(t *testing.T) {
			r := newRig(t)
			r.policy(t)
			s := r.failed(t, "INS-B1", 10)
			r.script(t, outcome, outcome == OutcomeSuccess, false, 0, s.ref)

			res := r.auto(t, s.caseID)
			if res.Outcome != "held" || !hasFailed(res.Gate, 2) {
				t.Fatalf("%+v", res)
			}
			if len(r.reversals(t, s.caseID)) != 0 || r.ledgerCount(t, s.ref) != 0 || r.providerRequests(t, s.ref) != 0 {
				t.Fatal("something was reversed or submitted although the outcome is " + outcome)
			}
			if got := r.kase(t, s.caseID); got.Status != cases.StDetected {
				t.Fatalf("the case moved to %s", got.Status)
			}
			held := false
			for _, n := range r.notes(t, s.caseID, notify.Operations) {
				held = held || n.Template == notify.TplOpsHeld
			}
			if !held {
				t.Fatal("operations were not told the reversal was blocked")
			}
		})
	}
}

func TestEachOfTheEightChecksCanBlock(t *testing.T) {
	type tc struct {
		name  string
		check int
		prep  func(r *rig, t *testing.T, s scn)
		age   int
	}
	cases := []tc{
		{"no ledger debit", 1, func(r *rig, t *testing.T, s scn) {
			if _, err := r.pool.Exec(context.Background(), `DELETE FROM sim_ledger_entries WHERE tenant_id = $1 AND instruction_ref = $2`, r.tenant, s.ref); err != nil {
				t.Fatal(err)
			}
		}, 10},
		{"ledger amount differs", 1, func(r *rig, t *testing.T, s scn) {
			if _, err := r.pool.Exec(context.Background(), `UPDATE sim_ledger_entries SET amount_minor = 1 WHERE tenant_id = $1 AND instruction_ref = $2`, r.tenant, s.ref); err != nil {
				t.Fatal(err)
			}
		}, 10},
		{"stale status evidence", 2, func(r *rig, t *testing.T, s scn) { r.script(t, OutcomeFailure, false, false, 3600, s.ref) }, 10},
		{"beneficiary was credited", 3, func(r *rig, t *testing.T, s scn) { r.script(t, OutcomeFailure, true, false, 0, s.ref) }, 10},
		{"settlement paid out", 3, func(r *rig, t *testing.T, s scn) { r.script(t, OutcomeFailure, false, true, 0, s.ref) }, 10},
		{"waiting time not elapsed", 4, func(r *rig, t *testing.T, s scn) {}, 0},
		{"no approved freshness limit", 2, func(r *rig, t *testing.T, s scn) {
			// Tenant with no policy: a separate tenant is used below.
		}, 10},
	}
	for _, c := range cases[:6] {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			r.policy(t)
			s := r.failed(t, "INS-G1", c.age)
			c.prep(r, t, s)
			res := r.auto(t, s.caseID)
			if res.Outcome != "held" || !hasFailed(res.Gate, c.check) {
				t.Fatalf("check %d should block: %+v", c.check, res.Gate.Failed())
			}
			if len(r.reversals(t, s.caseID)) != 0 || r.providerRequests(t, s.ref) != 0 || r.ledgerCount(t, s.ref) != 0 {
				t.Fatal("work was done although the gate failed")
			}
		})
	}
}

func TestWithoutApprovedPolicyNothingIsReversed(t *testing.T) {
	r := newRig(t) // no policy at all
	s := r.failed(t, "INS-NP", 10)
	res := r.auto(t, s.caseID)
	if res.Outcome != "held" || !hasFailed(res.Gate, 2) || !hasFailed(res.Gate, 4) || !hasFailed(res.Gate, 6) {
		t.Fatalf("an unconfigured institution must be refused: %+v", res.Gate.Failed())
	}
	if r.providerRequests(t, s.ref) != 0 {
		t.Fatal("the provider was called")
	}
}

func TestExecutionDisabledByPolicyBlocks(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	r.set(t, "reversal.simulated_execution_enabled", 0)
	s := r.failed(t, "INS-XD", 10)
	if res := r.auto(t, s.caseID); res.Outcome != "held" || !hasFailed(res.Gate, 6) {
		t.Fatalf("%+v", res.Gate.Failed())
	}
}

func TestTheKillSwitchStopsAutomationAtEveryScope(t *testing.T) {
	for name, target := range map[string]killswitch.Target{
		"global": {Scope: killswitch.ScopeGlobal}, "rail": {Scope: killswitch.ScopeRail, Value: "NIP"},
		"exception type": {Scope: killswitch.ScopeExceptionType, Value: "FAIL001"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.policy(t)
			s := r.failed(t, "INS-K1", 10)
			r.tx(t, func(tx pgx.Tx) error {
				_, err := r.kill.Freeze(context.Background(), tx, r.tenant, uuid.New(), target, "incident under review")
				return err
			})
			res := r.auto(t, s.caseID)
			if res.Outcome != "held" || !hasFailed(res.Gate, 6) {
				t.Fatalf("%+v", res.Gate.Failed())
			}
			if r.providerRequests(t, s.ref) != 0 {
				t.Fatal("the provider was called while frozen")
			}
		})
	}
}

func TestEvidenceThatChangesBetweenApprovalAndExecutionStopsTheReversal(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	r.set(t, "reversal.auto_eligible.FAIL001", 0)
	s := r.failed(t, "INS-CH", 10)
	res := r.auto(t, s.caseID)
	rev, err := r.svc.Decide(context.Background(), r.tenant, r.user("SUPERVISOR"), res.Reversal.ApprovalID, true, "approved on the evidence at the time")
	if err != nil {
		t.Fatal(err)
	}
	// The source now says the transfer actually succeeded.
	r.script(t, OutcomeSuccess, true, true, 0, s.ref)
	out, err := r.svc.Execute(context.Background(), r.tenant, rev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.AcceptedStatus != "REJECTED" || r.providerRequests(t, s.ref) != 0 || r.ledgerCount(t, s.ref) != 0 {
		t.Fatalf("a reversal ran on stale approval: %+v", out)
	}
}

// ---- approvals ----------------------------------------------------------------------

func TestDuplicatesAreNeverReversedAutomaticallyAndNeedAnApprover(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	at := time.Now().Add(-10 * time.Minute)
	r.event(t, "INS-DU1", txn.EvDebitPosted, at)
	dup := r.event(t, "INS-DU2", txn.EvDebitPosted, at.Add(3*time.Second), func(e *txn.Envelope) { e.Txn.SessionID = "S-INS-DU1" })
	r.event(t, "INS-DU1", txn.EvCreditConfirmed, at.Add(time.Second))
	r.tx(t, func(tx pgx.Tx) error {
		for _, ref := range []string{"INS-DU1", "INS-DU2"} {
			if err := SeedDebit(context.Background(), tx, r.tenant, ref, 1850000, "NGN"); err != nil {
				return err
			}
		}
		// The second debit was posted twice for one transfer: the source never credited it.
		return ScriptStatus(context.Background(), tx, r.tenant, "INS-DU2", OutcomeFailure, false, false, 0)
	})
	caseID := r.detect(t, dup, cases.CodeDuplicate)

	res := r.auto(t, caseID)
	if res.Outcome != "awaiting_approval" || res.Reversal == nil || res.Reversal.ApprovalStatus != "PENDING" || res.Gate.Verdict != PassNeedsApproval {
		t.Fatalf("%+v", res)
	}
	if r.providerRequests(t, "INS-DU2") != 0 || r.ledgerCount(t, "INS-DU2") != 0 {
		t.Fatal("a duplicate was reversed without an approval")
	}
	if got := r.kase(t, caseID); got.Status != cases.StAwaitingApproval {
		t.Fatalf("case is %s", got.Status)
	}
	// Running the automatic path again changes nothing.
	again := r.auto(t, caseID)
	if again.Reversal == nil || again.Reversal.ID != res.Reversal.ID || r.providerRequests(t, "INS-DU2") != 0 {
		t.Fatalf("%+v", again)
	}
}

func TestApprovalRulesAreEnforced(t *testing.T) {
	setup := func(t *testing.T) (*rig, scn, Reversal) {
		r := newRig(t)
		r.policy(t)
		r.set(t, "reversal.auto_eligible.FAIL001", 0)
		s := r.failed(t, "INS-AP", 10)
		res := r.auto(t, s.caseID)
		if res.Outcome != "awaiting_approval" {
			t.Fatalf("%+v", res)
		}
		return r, s, *res.Reversal
	}
	ctx := context.Background()

	t.Run("roles and MFA", func(t *testing.T) {
		r, _, rev := setup(t)
		noMFA := r.user("SUPERVISOR")
		noMFA.MFA = false
		if _, err := r.svc.Decide(ctx, r.tenant, noMFA, rev.ApprovalID, true, "ok"); !errors.Is(err, authz.ErrMFARequired) {
			t.Errorf("no MFA: %v", err)
		}
		for _, role := range []string{"TXN_ANALYST", "AGENT", "EXECUTIVE", "ADMIN_INTEGRATION", "RISK_COMPLIANCE"} {
			if _, err := r.svc.Decide(ctx, r.tenant, r.user(role), rev.ApprovalID, true, "ok"); !errors.Is(err, authz.ErrForbidden) {
				t.Errorf("%s approved: %v", role, err)
			}
		}
		if got, _ := r.svc.Policy, 0; got == nil {
			t.Fatal("policy")
		}
		var st string
		_ = r.pool.QueryRow(ctx, `SELECT status FROM approvals WHERE tenant_id = $1 AND id = $2`, r.tenant, rev.ApprovalID).Scan(&st)
		if st != "PENDING" {
			t.Fatalf("a refused attempt changed the approval to %s", st)
		}
	})

	t.Run("limits", func(t *testing.T) {
		r, _, rev := setup(t)
		r.set(t, "approval.limit_minor.HEAD_OPS", 100) // far below the amount
		if _, err := r.svc.Decide(ctx, r.tenant, r.user("HEAD_OPS"), rev.ApprovalID, true, "ok"); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("over the checker's limit: %v", err)
		}
		// A role with no configured limit cannot approve at all.
		r2, _, rev2 := setup(t)
		if _, err := r2.svc.Decide(ctx, r2.tenant, r2.user("HEAD_OPS"), rev2.ApprovalID, true, "ok"); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("no limit configured: %v", err)
		}
	})

	t.Run("a maker cannot check their own request", func(t *testing.T) {
		r := newRig(t)
		r.policy(t)
		s := r.failed(t, "INS-MK", 10)
		officer := r.user("REVERSAL_OFFICER", "SUPERVISOR")
		out, err := r.svc.Request(ctx, r.tenant, RequestInput{CaseID: s.caseID, IdempotencyKey: "k-mk", ReasonCode: "CUSTOMER_REPORTED", Who: Requester{Principal: officer}})
		if err != nil || out.Blocked || out.Reversal.ApprovalStatus != "PENDING" {
			t.Fatalf("a person's request must wait for approval: %+v %v", out, err)
		}
		if _, err := r.svc.Decide(ctx, r.tenant, officer, out.Reversal.ApprovalID, true, "I approve my own request"); !errors.Is(err, authz.ErrSameActor) {
			t.Fatalf("self approval: %v", err)
		}
		// The database refuses it even if the service were bypassed.
		_, dberr := r.pool.Exec(ctx, `UPDATE approvals SET status = 'APPROVED', checker = maker WHERE tenant_id = $1 AND id = $2`, r.tenant, out.Reversal.ApprovalID)
		if dberr == nil {
			t.Fatal("the database accepted a self-approval")
		}
		if r.providerRequests(t, s.ref) != 0 {
			t.Fatal("the provider was called")
		}
		other := r.user("SUPERVISOR")
		rev, err := r.svc.Decide(ctx, r.tenant, other, out.Reversal.ApprovalID, true, "second pair of eyes")
		if err != nil || rev.ApprovalStatus != "APPROVED" {
			t.Fatalf("%+v %v", rev, err)
		}
	})

	t.Run("a conflict of interest blocks the approver", func(t *testing.T) {
		r, _, rev := setup(t)
		sup := r.user("SUPERVISOR")
		store := &identity.PGUserStore{Pool: r.pool}
		id, err := store.Create(ctx, r.tenant, "conflicted@example.test", "Conflicted", "a long enough passphrase", []string{"SUPERVISOR"})
		if err != nil {
			t.Fatal(err)
		}
		sup.UserID = id
		r.tx(t, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO staff_account_links (tenant_id, user_id, account_token) VALUES ($1,$2,$3)`, r.tenant, id, tokA)
			return err
		})
		if _, err := r.svc.Decide(ctx, r.tenant, sup, rev.ApprovalID, true, "ok"); !errors.Is(err, authz.ErrConflictOfInterest) {
			t.Fatalf("%v", err)
		}
	})

	t.Run("approve, execute, verify", func(t *testing.T) {
		r, s, rev := setup(t)
		got, err := r.svc.Decide(ctx, r.tenant, r.user("SUPERVISOR"), rev.ApprovalID, true, "evidence checked")
		if err != nil || got.ApprovalStatus != "APPROVED" {
			t.Fatalf("%+v %v", got, err)
		}
		if r.providerRequests(t, s.ref) != 0 {
			t.Fatal("approving alone must not submit anything")
		}
		if _, err := r.svc.Decide(ctx, r.tenant, r.user("SUPERVISOR"), rev.ApprovalID, true, "again"); !errors.Is(err, ErrNotPending) {
			t.Errorf("deciding twice: %v", err)
		}
		if _, err := r.svc.Execute(ctx, r.tenant, rev.ID); err != nil {
			t.Fatal(err)
		}
		final, err := r.svc.Advance(ctx, r.tenant, rev.ID)
		if err != nil || final.FinancialStatus != "VERIFIED" || r.kase(t, s.caseID).Status != cases.StResolved {
			t.Fatalf("%+v %v", final, err)
		}
		r.chainOK(t)
	})

	t.Run("rejection frees the lock and returns the case to investigation", func(t *testing.T) {
		r, s, rev := setup(t)
		if _, err := r.svc.Decide(ctx, r.tenant, r.user("SUPERVISOR"), rev.ApprovalID, false, ""); err == nil {
			t.Fatal("a rejection without a reason was accepted")
		}
		got, err := r.svc.Decide(ctx, r.tenant, r.user("SUPERVISOR"), rev.ApprovalID, false, "the customer withdrew the complaint")
		if err != nil || got.ApprovalStatus != "REJECTED" {
			t.Fatalf("%+v %v", got, err)
		}
		if c := r.kase(t, s.caseID); c.Status != cases.StUnderInvestigation {
			t.Fatalf("case is %s", c.Status)
		}
		if _, err := r.svc.Execute(ctx, r.tenant, rev.ID); err == nil {
			t.Fatal("a rejected reversal was executed")
		}
		if r.providerRequests(t, s.ref) != 0 {
			t.Fatal("the provider was called")
		}
		// A fresh request can now be made.
		out, err := r.svc.Request(ctx, r.tenant, RequestInput{CaseID: s.caseID, IdempotencyKey: "k-retry", ReasonCode: "RETRY", Who: Requester{}})
		if err != nil || out.Blocked {
			t.Fatalf("the lock was not freed: %+v %v", out, err)
		}
	})
}

func TestARoleWithoutTheGrantCannotRequestAReversal(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-RR", 10)
	for _, role := range []string{"TXN_ANALYST", "AGENT", "EXECUTIVE", "HEAD_OPS"} {
		out, err := r.svc.Request(context.Background(), r.tenant, RequestInput{CaseID: s.caseID, IdempotencyKey: "k-" + role, ReasonCode: "X", Who: Requester{Principal: r.user(role)}})
		if err != nil || !out.Blocked || !hasFailed(out.Gate, 7) {
			t.Errorf("%s: %+v %v", role, out.Gate.Failed(), err)
		}
	}
	if len(r.reversals(t, s.caseID)) != 0 {
		t.Fatal("a reversal exists")
	}
}

// ---- idempotency and the lock --------------------------------------------------------

func TestTheSameKeyReturnsTheSameReversalAndADifferentRequestIsRefused(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	r.set(t, "reversal.auto_eligible.FAIL001", 0)
	s := r.failed(t, "INS-ID", 10)
	ctx := context.Background()
	in := RequestInput{CaseID: s.caseID, IdempotencyKey: "same-key", ReasonCode: "CUSTOMER_REPORTED", Who: Requester{}}
	first, err := r.svc.Request(ctx, r.tenant, in)
	if err != nil || first.Existing {
		t.Fatalf("%+v %v", first, err)
	}
	for i := 0; i < 3; i++ {
		again, err := r.svc.Request(ctx, r.tenant, in)
		if err != nil || !again.Existing || again.Reversal.ID != first.Reversal.ID {
			t.Fatalf("repeat %d: %+v %v", i, again, err)
		}
	}
	if len(r.reversals(t, s.caseID)) != 1 {
		t.Fatal("repeats created extra reversals")
	}
	in.ReasonCode = "SOMETHING_ELSE"
	if _, err := r.svc.Request(ctx, r.tenant, in); !errors.Is(err, ErrKeyReuse) {
		t.Fatalf("a different request under the same key: %v", err)
	}
}

func TestOnlyOneLiveReversalCanExistPerTransactionEvenUnderConcurrency(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	r.set(t, "reversal.auto_eligible.FAIL001", 0)
	s := r.failed(t, "INS-CC", 10)
	const n = 10
	var wg sync.WaitGroup
	outs := make(chan Outcome, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := r.svc.Request(context.Background(), r.tenant, RequestInput{CaseID: s.caseID, IdempotencyKey: fmt.Sprintf("race-%d", i), ReasonCode: "RACE", Who: Requester{}})
			outs <- out
			errs <- err
		}(i)
	}
	wg.Wait()
	close(outs)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a concurrent request failed outright: %v", err)
		}
	}
	created := 0
	for o := range outs {
		if !o.Blocked {
			created++
		}
	}
	if created != 1 || len(r.reversals(t, s.caseID)) != 1 {
		t.Fatalf("%d requests were accepted and %d reversals exist; want exactly 1", created, len(r.reversals(t, s.caseID)))
	}
}

func TestAReversalAlreadyDoneBlocksAnotherForTheSameDebit(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-TW", 10)
	if res := r.auto(t, s.caseID); res.Outcome != "reversed" {
		t.Fatalf("%+v", res)
	}
	// A second case for the same debit cannot be reversed again.
	r.tx(t, func(tx pgx.Tx) error {
		id := s.txnID
		_, _, err := r.svc.Cases.Open(context.Background(), tx, r.tenant, cases.OpenInput{Classification: cases.CodeNoCredit, Source: cases.SourceManual, TxnID: &id,
			Rail: "NIP", Actor: cases.System})
		return err
	})
	var second uuid.UUID
	r.tx(t, func(tx pgx.Tx) error {
		l, err := r.svc.Cases.List(context.Background(), tx, r.tenant, cases.Filter{Source: cases.SourceManual}, nil, 5)
		if len(l) > 0 {
			second = l[0].ID
		}
		return err
	})
	out, err := r.svc.Request(context.Background(), r.tenant, RequestInput{CaseID: second, IdempotencyKey: "k-again", ReasonCode: "AGAIN", Who: Requester{}})
	if err != nil || !out.Blocked || !hasFailed(out.Gate, 5) {
		t.Fatalf("%+v %v", out.Gate.Failed(), err)
	}
	if r.ledgerCount(t, s.ref) != 1 {
		t.Fatal("a second reversal posted")
	}
}

// ---- provider behaviour ---------------------------------------------------------------

func TestARejectedReversalFailsSafelyOpensAReversalFailureCaseAndTellsNoOneItWorked(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-RJ", 10)
	r.provider(t, s.ref, "REJECT")
	res := r.auto(t, s.caseID)
	if res.Outcome != "failed" || res.Reversal.AcceptedStatus != "REJECTED" || res.Reversal.FinancialStatus != "FAILED" {
		t.Fatalf("%+v", res)
	}
	if r.ledgerCount(t, s.ref) != 0 || r.txnState(t, s.txnID) == txn.StateReversed {
		t.Fatal("money or state moved although the provider refused")
	}
	var rev001 int
	_ = r.pool.QueryRow(context.Background(), `SELECT count(*) FROM cases WHERE tenant_id = $1 AND classification = 'REV001'`, r.tenant).Scan(&rev001)
	if rev001 != 1 {
		t.Fatalf("%d REV001 cases", rev001)
	}
	if got := r.kase(t, s.caseID); got.Status == cases.StResolved {
		t.Fatal("the case was resolved")
	}
	for _, n := range r.notes(t, s.caseID, notify.Customer) {
		if n.Stage == notify.StageVerified {
			t.Fatal("the customer was told it worked")
		}
	}
	// The lock is free again: after the cause is fixed the reversal can be retried.
	r.provider(t, s.ref, "SUCCESS")
	out, err := r.svc.Request(context.Background(), r.tenant, RequestInput{CaseID: s.caseID, IdempotencyKey: "k-after-reject", ReasonCode: "RETRY", Who: Requester{}})
	if err != nil || out.Blocked {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestATimeoutIsUnknownAndTheSameKeyFindsOutWhatHappenedWithoutPostingTwice(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-TO", 10)
	r.provider(t, s.ref, "TIMEOUT")
	ctx := context.Background()

	out, err := r.svc.Request(ctx, r.tenant, RequestInput{CaseID: s.caseID, IdempotencyKey: "k-to", ReasonCode: "TEST", Who: Requester{}})
	if err != nil || out.Blocked {
		t.Fatalf("%+v %v", out, err)
	}
	rev, err := r.svc.Execute(ctx, r.tenant, out.Reversal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rev.AcceptedStatus != "UNKNOWN" || rev.ProcessingStatus != "UNKNOWN" || rev.FinancialStatus != "UNVERIFIED" {
		t.Fatalf("a timeout must be UNKNOWN, not failed: %+v", rev)
	}
	// Executing again must not blindly resubmit while the outcome is unknown.
	rev, err = r.svc.Execute(ctx, r.tenant, rev.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Asking the provider by the same key learns that it did act.
	rev, err = r.svc.Advance(ctx, r.tenant, rev.ID)
	if err != nil || rev.FinancialStatus != "VERIFIED" {
		t.Fatalf("%+v %v", rev, err)
	}
	if r.ledgerCount(t, s.ref) != 1 || r.providerRequests(t, s.ref) != 1 {
		t.Fatalf("ledger %d, provider requests %d; want 1 and 1", r.ledgerCount(t, s.ref), r.providerRequests(t, s.ref))
	}
}

func TestADelayedPostingIsVerifiedOnlyOnceTheLedgerShowsIt(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-DL", 10)
	r.provider(t, s.ref, "DELAYED")
	ctx := context.Background()
	out, _ := r.svc.Request(ctx, r.tenant, RequestInput{CaseID: s.caseID, IdempotencyKey: "k-dl", ReasonCode: "TEST", Who: Requester{}})
	rev, err := r.svc.Execute(ctx, r.tenant, out.Reversal.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		rev, err = r.svc.Advance(ctx, r.tenant, rev.ID)
		if err != nil || rev.FinancialStatus != "UNVERIFIED" || rev.ProcessingStatus != "IN_PROGRESS" {
			t.Fatalf("poll %d: %+v %v", i, rev, err)
		}
	}
	rev, err = r.svc.Advance(ctx, r.tenant, rev.ID)
	if err != nil || rev.FinancialStatus != "VERIFIED" {
		t.Fatalf("third poll: %+v %v", rev, err)
	}
}

func TestAProviderThatClaimsSuccessWithoutAPostingIsCaught(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-GH", 10)
	r.provider(t, s.ref, "GHOST")
	res := r.auto(t, s.caseID)
	if res.Outcome != "failed" || res.Reversal.ProcessingStatus != "COMPLETED" || res.Reversal.FinancialStatus != "FAILED" ||
		!strings.Contains(res.Reversal.FailureReason, "no matching reversal posting") {
		t.Fatalf("the provider's word was accepted: %+v", res.Reversal)
	}
	if r.kase(t, s.caseID).Status == cases.StResolved {
		t.Fatal("the case was resolved on the provider's word")
	}
	for _, n := range r.notes(t, s.caseID, notify.Customer) {
		if n.Stage == notify.StageVerified {
			t.Fatal("the customer was told the money was returned")
		}
	}
	var rev001 int
	_ = r.pool.QueryRow(context.Background(), `SELECT count(*) FROM cases WHERE tenant_id = $1 AND classification = 'REV001'`, r.tenant).Scan(&rev001)
	if rev001 != 1 {
		t.Fatalf("%d REV001 cases", rev001)
	}
}

// ---- the sweep and isolation --------------------------------------------------------------

func TestTheSweepHandlesEveryOpenCaseThroughTheSameGate(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	good := r.failed(t, "INS-SW1", 10)
	unknown := r.failed(t, "INS-SW2", 10)
	r.script(t, OutcomeUnknown, false, false, 0, unknown.ref)

	results, err := r.svc.Sweep(context.Background(), r.tenant)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]string{}
	for _, res := range results {
		by[res.CaseRef] = res.Outcome
	}
	if len(results) != 2 {
		t.Fatalf("%v", by)
	}
	if r.kase(t, good.caseID).Status != cases.StResolved || r.kase(t, unknown.caseID).Status != cases.StDetected {
		t.Fatalf("good: %s, unknown: %s", r.kase(t, good.caseID).Status, r.kase(t, unknown.caseID).Status)
	}
	if r.providerRequests(t, unknown.ref) != 0 {
		t.Fatal("the sweep submitted an unknown-outcome transaction")
	}
	// Sweeping again does nothing new.
	if _, err := r.svc.Sweep(context.Background(), r.tenant); err != nil || r.ledgerCount(t, good.ref) != 1 {
		t.Fatalf("%v", err)
	}
}

func TestReversalsAreTenantIsolatedAndEveryStepIsAudited(t *testing.T) {
	a, b := newRig(t), newRig(t)
	a.policy(t)
	s := a.failed(t, "INS-IS", 10)
	res := a.auto(t, s.caseID)
	if res.Outcome != "reversed" {
		t.Fatalf("%+v", res)
	}
	b.tx(t, func(tx pgx.Tx) error {
		if _, err := b.svc.Get(context.Background(), tx, b.tenant, res.Reversal.ID.String()); !errors.Is(err, ErrNotFound) {
			t.Errorf("tenant B read tenant A's reversal: %v", err)
		}
		return nil
	})
	var actions []string
	a.tx(t, func(tx pgx.Tx) error {
		recs, err := audit.List(context.Background(), tx, a.tenant, audit.Filter{SubjectType: "reversal"}, 0, 50)
		for _, rec := range recs {
			actions = append(actions, rec.Action)
		}
		return err
	})
	want := []string{"reversal.requested", "reversal.submitted", "reversal.verified"}
	if len(actions) != len(want) {
		t.Fatalf("audit actions %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("audit actions %v, want %v", actions, want)
		}
	}
	a.chainOK(t)
}

func TestAssessChangesNothing(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-AS", 10)
	var g Gate
	r.tx(t, func(tx pgx.Tx) error {
		var err error
		g, err = r.svc.Assess(context.Background(), tx, r.tenant, s.caseID, Requester{})
		return err
	})
	if g.Verdict != PassAuto || len(g.Checks) != 8 {
		t.Fatalf("%+v", g)
	}
	if len(r.reversals(t, s.caseID)) != 0 || r.providerRequests(t, s.ref) != 0 || r.kase(t, s.caseID).Status != cases.StDetected {
		t.Fatal("an assessment changed something")
	}
}
