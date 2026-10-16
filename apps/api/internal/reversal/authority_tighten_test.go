package reversal

import (
	"context"
	"testing"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
)

// A reversal that policy let automation submit, but which has not been submitted
// yet, must not go out on that authority once policy has been tightened so that a
// second person is needed. Nobody approved it, so there is nothing to carry over.
func TestPolicyTightenedBeforeAnAutomaticReversalIsSubmittedStopsIt(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	ctx := context.Background()
	r.set(t, "reversal.auto_eligible.FAIL001", 1)
	s := r.failed(t, "INS-AT1", 10)
	out, err := r.svc.Request(ctx, r.tenant, RequestInput{CaseID: s.caseID, IdempotencyKey: "auto-at1", ReasonCode: "AUTOMATIC_ASSESSMENT", Who: Requester{}})
	if err != nil || out.Blocked || out.Reversal.ApprovalStatus != "NOT_REQUIRED" {
		t.Fatalf("expected an automatic reversal waiting to be submitted: %+v %v", out, err)
	}
	r.set(t, "reversal.auto_eligible.FAIL001", 0)

	for _, run := range []func(){
		func() { _, _ = r.svc.Execute(ctx, r.tenant, out.Reversal.ID) },
		func() { r.sweep(t) },
	} {
		run()
		if r.providerRequests(t, s.ref) != 0 || r.ledgerCount(t, s.ref) != 0 {
			t.Fatal("a reversal that policy now says needs a second person was submitted on the old authority")
		}
	}
	got := r.reversals(t, s.caseID)
	if len(got) != 1 || got[0].FinancialStatus == "VERIFIED" || got[0].AcceptedStatus != "PENDING" || got[0].ApprovalStatus != "PENDING" {
		t.Fatalf("it must now be waiting for a person: %+v", got)
	}
	if r.approvalCount(t, got[0].ID) != 1 || r.audits(t, "reversal.escalated_to_approval") != 1 || r.caseEvents(t, s.caseID, "approval_now_required") != 1 {
		t.Fatal("the escalation was not recorded exactly once")
	}
	if r.kase(t, s.caseID).Status != cases.StAwaitingApproval {
		t.Fatalf("case is %s", r.kase(t, s.caseID).Status)
	}

	// A person decides, as for any other reversal, and only then does it go out once.
	rev, err := r.svc.Decide(ctx, r.tenant, r.user("SUPERVISOR"), got[0].ApprovalID, true, "checked")
	if err != nil || rev.ApprovalStatus != "APPROVED" {
		t.Fatalf("%+v %v", rev, err)
	}
	if _, err := r.svc.Execute(ctx, r.tenant, rev.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := r.svc.Advance(ctx, r.tenant, rev.ID); err != nil || got.FinancialStatus != "VERIFIED" || r.providerRequests(t, s.ref) != 1 || r.ledgerCount(t, s.ref) != 1 {
		t.Fatalf("expected exactly one submission after the approval: %v %+v", err, got)
	}
	r.chainOK(t)
}

// Loosening policy must not approve anything that was waiting for a person.
func TestPolicyLoosenedWhileAReversalAwaitsApprovalDoesNotApproveIt(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	ctx := context.Background()
	r.set(t, "reversal.auto_eligible.FAIL001", 0)
	s := r.failed(t, "INS-AT2", 10)
	res := r.auto(t, s.caseID)
	if res.Outcome != "awaiting_approval" {
		t.Fatalf("%+v", res)
	}
	r.set(t, "reversal.auto_eligible.FAIL001", 1)
	r.set(t, "approval.auto_limit_minor", 100_000_000)
	for _, run := range []func(){
		func() { _, _ = r.svc.Execute(ctx, r.tenant, res.Reversal.ID) },
		func() { r.sweep(t) },
		func() { r.auto(t, s.caseID) },
	} {
		run()
		got := r.reversals(t, s.caseID)
		if len(got) != 1 || got[0].ApprovalStatus != "PENDING" || got[0].AcceptedStatus != "PENDING" || r.providerRequests(t, s.ref) != 0 {
			t.Fatalf("a reversal waiting for a person moved without one: %+v", got)
		}
	}
}
