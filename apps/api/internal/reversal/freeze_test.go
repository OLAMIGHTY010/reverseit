package reversal

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/automation"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
)

func (r *rig) freeze(t *testing.T, scope killswitch.Scope, target string) {
	t.Helper()
	r.tx(t, func(tx pgx.Tx) error {
		_, err := r.kill.Freeze(context.Background(), tx, r.tenant, uuid.New(), killswitch.Target{Scope: scope, Value: target}, "incident under review")
		return err
	})
}

func (r *rig) unfreeze(t *testing.T, scope killswitch.Scope, target string) {
	t.Helper()
	r.tx(t, func(tx pgx.Tx) error {
		seq, err := r.kill.RequestUnfreeze(context.Background(), tx, r.tenant, uuid.New(), killswitch.Target{Scope: scope, Value: target}, "incident closed")
		if err != nil {
			return err
		}
		return r.kill.DecideUnfreeze(context.Background(), tx, r.tenant, uuid.New(), seq, true, "confirmed closed")
	})
}

func (r *rig) sweep(t *testing.T) []AutoResult {
	t.Helper()
	out, err := r.svc.Sweep(context.Background(), r.tenant)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *rig) caseEvents(t *testing.T, caseID uuid.UUID, kind string) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM case_events WHERE tenant_id = $1 AND case_id = $2 AND kind = $3`, r.tenant, caseID, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) job(t *testing.T) automation.Job {
	t.Helper()
	var jobs []automation.Job
	r.tx(t, func(tx pgx.Tx) error {
		var err error
		jobs, err = automation.Jobs(context.Background(), tx, r.tenant)
		return err
	})
	for _, j := range jobs {
		if j.Job == automation.JobReversalSweep {
			return j
		}
	}
	t.Fatal("the reversal sweep recorded no state")
	return automation.Job{}
}

// approved builds a failed transfer whose reversal needs a person, and approves it.
func (r *rig) approved(t *testing.T, ref string) (scn, Reversal) {
	t.Helper()
	r.set(t, "reversal.auto_eligible.FAIL001", 0)
	s := r.failed(t, ref, 10)
	res := r.auto(t, s.caseID)
	if res.Outcome != "awaiting_approval" {
		t.Fatalf("%+v", res)
	}
	rev, err := r.svc.Decide(context.Background(), r.tenant, r.user("SUPERVISOR"), res.Reversal.ApprovalID, true, "looks right")
	if err != nil || rev.ApprovalStatus != "APPROVED" {
		t.Fatalf("%+v %v", rev, err)
	}
	return s, rev
}

var covering = map[string]struct {
	scope  killswitch.Scope
	target string
}{
	"global":         {killswitch.ScopeGlobal, ""},
	"rail":           {killswitch.ScopeRail, "NIP"},
	"exception type": {killswitch.ScopeExceptionType, "FAIL001"},
	"policy":         {killswitch.ScopePolicy, "reversal.FAIL001.NIP"},
}

var notCovering = map[string]struct {
	scope  killswitch.Scope
	target string
}{
	"another rail":           {killswitch.ScopeRail, "CARD"},
	"another exception type": {killswitch.ScopeExceptionType, "DUP001"},
	"another rail's policy":  {killswitch.ScopePolicy, "reversal.FAIL001.CARD"},
	"another type's policy":  {killswitch.ScopePolicy, "reversal.DUP001.NIP"},
	"a detector rule":        {killswitch.ScopePolicy, "DET-FAIL-1"},
}

func TestAFreezeAfterApprovalParksTheReversalAndItIsSubmittedOnceAfterAnAuthorisedUnfreeze(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-PK")
	ctx := context.Background()
	r.freeze(t, killswitch.ScopeGlobal, "")

	out, err := r.svc.Execute(ctx, r.tenant, rev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.AcceptedStatus != "PENDING" || out.FinancialStatus != "UNVERIFIED" || out.ApprovalStatus != "APPROVED" || out.ParkedAt == nil {
		t.Fatalf("a frozen reversal must be parked, still approved and not failed: %+v", out)
	}
	if r.providerRequests(t, s.ref) != 0 || r.ledgerCount(t, s.ref) != 0 {
		t.Fatal("money moved while frozen")
	}
	if got := r.kase(t, s.caseID); got.Status != cases.StAwaitingApproval {
		t.Fatalf("case is %s", got.Status)
	}
	// Trying again and sweeping both leave it parked, and the parking is recorded once.
	if _, err := r.svc.Execute(ctx, r.tenant, rev.ID); err != nil {
		t.Fatal(err)
	}
	r.sweep(t)
	if n := r.caseEvents(t, s.caseID, "reversal_parked"); n != 1 {
		t.Fatalf("parking recorded %d times", n)
	}
	if got := r.reversals(t, s.caseID); len(got) != 1 || got[0].ParkedAt == nil {
		t.Fatalf("%+v", got)
	}

	r.unfreeze(t, killswitch.ScopeGlobal, "")
	r.sweep(t)
	got := r.reversals(t, s.caseID)
	if len(got) != 1 || got[0].FinancialStatus != "VERIFIED" || got[0].ParkedAt != nil {
		t.Fatalf("after the unfreeze: %+v", got)
	}
	if r.providerRequests(t, s.ref) != 1 || r.ledgerCount(t, s.ref) != 1 {
		t.Fatalf("expected one submission and one posting, got %d and %d", r.providerRequests(t, s.ref), r.ledgerCount(t, s.ref))
	}
	if r.kase(t, s.caseID).Status != cases.StResolved || r.caseEvents(t, s.caseID, "reversal_resumed") != 1 {
		t.Fatal("the case did not resolve, or the resumption was not recorded once")
	}
	r.sweep(t)
	if r.providerRequests(t, s.ref) != 1 {
		t.Fatal("a later sweep submitted again")
	}
	r.chainOK(t)
}

func TestEveryScopeAgainstEveryNewSubmissionPath(t *testing.T) {
	type path struct {
		name string
		run  func(r *rig, t *testing.T) (reversed bool)
	}
	paths := []path{
		{"the automatic sweep", func(r *rig, t *testing.T) bool {
			s := r.failed(t, "INS-SW", 10)
			r.freezeAfterSetup()
			noise := r.caseEvents(t, s.caseID, "reversal_blocked")
			r.sweep(t)
			if r.caseEvents(t, s.caseID, "reversal_blocked") != noise {
				t.Fatal("the sweep ran the gate for a case a freeze covers")
			}
			return len(r.reversals(t, s.caseID)) == 1 && r.reversals(t, s.caseID)[0].FinancialStatus == "VERIFIED"
		}},
		{"a parked approved submission", func(r *rig, t *testing.T) bool {
			s, rev := r.approvedUnderFreeze(t, "INS-AP")
			if _, err := r.svc.Execute(context.Background(), r.tenant, rev.ID); err != nil {
				t.Fatal(err)
			}
			r.sweep(t)
			return r.providerRequests(t, s.ref) == 1
		}},
		{"a person's request", func(r *rig, t *testing.T) bool {
			s := r.failed(t, "INS-HU", 10)
			r.freezeAfterSetup()
			out, err := r.svc.Request(context.Background(), r.tenant, RequestInput{CaseID: s.caseID, IdempotencyKey: "k-" + s.ref, ReasonCode: "CONFIRMED_FAILED_CREDIT",
				Who: Requester{Principal: r.user("REVERSAL_OFFICER")}})
			if err != nil {
				t.Fatal(err)
			}
			if out.Blocked && !hasFailed(out.Gate, 6) {
				t.Fatalf("blocked for the wrong reason: %+v", out.Gate.Failed())
			}
			return !out.Blocked
		}},
	}
	for _, p := range paths {
		for name, f := range covering {
			p, name, f := p, name, f
			t.Run(p.name+" is held by a "+name+" freeze", func(t *testing.T) {
				r := newRig(t)
				r.policy(t)
				r.freezeAfterSetup = func() { r.freeze(t, f.scope, f.target) }
				if p.run(r, t) {
					t.Fatal("a covering freeze did not stop a new submission")
				}
			})
		}
		for name, f := range notCovering {
			p, name, f := p, name, f
			t.Run(p.name+" ignores a freeze on "+name, func(t *testing.T) {
				r := newRig(t)
				r.policy(t)
				r.freezeAfterSetup = func() { r.freeze(t, f.scope, f.target) }
				if !p.run(r, t) {
					t.Fatal("a freeze that does not cover the case stopped it")
				}
			})
		}
	}
}

// approvedUnderFreeze is approved(), then applies the freeze the test chose.
func (r *rig) approvedUnderFreeze(t *testing.T, ref string) (scn, Reversal) {
	s, rev := r.approved(t, ref)
	if r.freezeAfterSetup != nil {
		r.freezeAfterSetup()
	}
	return s, rev
}

func TestVerificationOfSubmittedReversalsContinuesWhileSubmissionsAreFrozen(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-VF", 10)
	r.provider(t, s.ref, "DELAYED") // accepted now, posted after a few polls
	res := r.auto(t, s.caseID)
	if res.Outcome != "in_progress" {
		t.Fatalf("%+v", res)
	}
	r.freeze(t, killswitch.ScopeGlobal, "")
	for i := 0; i < 6 && r.reversals(t, s.caseID)[0].FinancialStatus != "VERIFIED"; i++ {
		r.sweep(t)
	}
	got := r.reversals(t, s.caseID)[0]
	if got.FinancialStatus != "VERIFIED" || got.VerifiedAt == nil {
		t.Fatalf("a submitted reversal must still be reconciled during a freeze: %+v", got)
	}
	if r.kase(t, s.caseID).Status != cases.StResolved {
		t.Fatal("the case did not resolve")
	}
	if r.providerRequests(t, s.ref) != 1 || r.ledgerCount(t, s.ref) != 1 {
		t.Fatal("exactly one submission and one posting expected")
	}
	if j := r.job(t); j.State != "paused" || j.Frozen != "global" {
		t.Fatalf("the job should show new work is paused: %+v", j)
	}
	if n := len(r.notes(t, s.caseID, "customer")); n != 2 {
		t.Fatalf("the customer should have the detected and verified messages, got %d", n)
	}
}

func TestSweepJobStateShowsPausesAndHeldCounts(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-JS", 10)
	r.freeze(t, killswitch.ScopeRail, "NIP")
	r.sweep(t)
	j := r.job(t)
	if j.State != "running" || j.Frozen != "rail:NIP" || string(j.LastRun) == "" {
		t.Fatalf("%+v", j)
	}
	var res map[string]any
	_ = json.Unmarshal(j.LastRun, &res)
	if res["held_new_requests"] != float64(1) || res["new_requests"] != float64(0) {
		t.Fatalf("held counts: %s", j.LastRun)
	}
	if len(r.reversals(t, s.caseID)) != 0 {
		t.Fatal("a reversal was requested under a freeze")
	}
	r.unfreeze(t, killswitch.ScopeRail, "NIP")
	r.sweep(t)
	if j := r.job(t); j.State != "running" || j.Frozen != "" {
		t.Fatalf("%+v", j)
	}
	if got := r.reversals(t, s.caseID); len(got) != 1 || got[0].FinancialStatus != "VERIFIED" {
		t.Fatalf("held work was not done after the unfreeze: %+v", got)
	}
}

func TestTwoSweepsAfterAnUnfreezeSubmitAParkedReversalOnce(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-CC")
	r.freeze(t, killswitch.ScopeGlobal, "")
	if _, err := r.svc.Execute(context.Background(), r.tenant, rev.ID); err != nil {
		t.Fatal(err)
	}
	r.unfreeze(t, killswitch.ScopeGlobal, "")
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.svc.Sweep(context.Background(), r.tenant); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if r.providerRequests(t, s.ref) != 1 || r.ledgerCount(t, s.ref) != 1 {
		t.Fatalf("concurrent sweeps submitted %d times and posted %d times", r.providerRequests(t, s.ref), r.ledgerCount(t, s.ref))
	}
	if got := r.reversals(t, s.caseID); len(got) != 1 || got[0].FinancialStatus != "VERIFIED" {
		t.Fatalf("%+v", got)
	}
}

func TestSafetyAlertsAreStillRecordedAndNotRepeatedDuringAFreeze(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-AL", 10)
	r.freeze(t, killswitch.ScopeGlobal, "")
	req := func() {
		if _, err := r.svc.Request(context.Background(), r.tenant, RequestInput{CaseID: s.caseID, IdempotencyKey: "k-al", ReasonCode: "CONFIRMED_FAILED_CREDIT",
			Who: Requester{Principal: r.user("REVERSAL_OFFICER")}}); err != nil {
			t.Fatal(err)
		}
	}
	req()
	req()
	ops := r.notes(t, s.caseID, "operations")
	held := 0
	for _, n := range ops {
		if n.Template == "ops.reversal_held.v1" {
			held++
		}
	}
	if held != 1 {
		t.Fatalf("expected one held alert (deduplicated), got %d in %+v", held, ops)
	}
}

func TestAMissingPolicyIdentityNeverSlipsPastAPolicyFreeze(t *testing.T) {
	st := killswitch.State{Frozen: []killswitch.Frozen{{Target: killswitch.Target{Scope: killswitch.ScopePolicy, Value: "reversal.FAIL001.NIP"}}}}
	for name, a := range map[string]killswitch.Action{
		"no exception code": actionFor("", "NIP"),
		"no rail":           actionFor("FAIL001", ""),
		"neither":           actionFor("", ""),
	} {
		if !a.PolicyUnknown || a.PolicyID != "" {
			t.Errorf("%s: the identity should be reported unknown: %+v", name, a)
		}
		if st.Covers(a) == nil {
			t.Errorf("%s: an unidentified policy slipped past a policy freeze", name)
		}
	}
	if a := actionFor("FAIL001", "NIP"); a.PolicyUnknown || a.PolicyID != "reversal.FAIL001.NIP" || st.Covers(a) == nil {
		t.Fatalf("a known policy should match its own freeze: %+v", a)
	}
	// Unknown identity is not covered by freezes that are not about policy.
	rail := killswitch.State{Frozen: []killswitch.Frozen{{Target: killswitch.Target{Scope: killswitch.ScopeRail, Value: "CARD"}}}}
	if rail.Covers(actionFor("", "NIP")) != nil {
		t.Fatal("a rail freeze on another rail covered the action")
	}
}
