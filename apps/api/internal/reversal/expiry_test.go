package reversal

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/events"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
)

// lapse makes the newest approval of a reversal expire now. Approvals are decided
// once and cannot be edited, so the test steps outside that rule to stand in for
// the passing of time.
func (r *rig) lapse(t *testing.T, revID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE approvals SET expires_at = now() - interval '1 second' WHERE tenant_id = $1 AND reversal_id = $2`, r.tenant, revID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) audits(t *testing.T, action string) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE tenant_id = $1 AND action = $2`, r.tenant, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) alerts(t *testing.T, template string) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE tenant_id = $1 AND template_id = $2`, r.tenant, template).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) approvalCount(t *testing.T, revID uuid.UUID) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM approvals WHERE tenant_id = $1 AND reversal_id = $2`, r.tenant, revID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAnExpiredApprovalIsNeverAuthorityToSubmit(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-EX1")
	ctx := context.Background()
	r.lapse(t, rev.ID)

	out, err := r.svc.Execute(ctx, r.tenant, rev.ID)
	if !errors.Is(err, ErrApprovalExpired) {
		t.Fatalf("expected ErrApprovalExpired, got %v", err)
	}
	if out.ApprovalStatus != "EXPIRED" || out.AcceptedStatus != "PENDING" || out.FinancialStatus != "UNVERIFIED" {
		t.Fatalf("%+v", out)
	}
	if r.providerRequests(t, s.ref) != 0 || r.ledgerCount(t, s.ref) != 0 {
		t.Fatal("something was submitted on an expired approval")
	}
	// The lapse is recorded once however often it is tried again.
	for i := 0; i < 3; i++ {
		if _, err := r.svc.Execute(ctx, r.tenant, rev.ID); !errors.Is(err, ErrApprovalExpired) {
			t.Fatalf("retry %d: %v", i, err)
		}
	}
	if r.caseEvents(t, s.caseID, "approval_expired") != 1 || r.audits(t, "reversal.approval_expired") != 1 || r.alerts(t, "ops.approval_expired.v1") != 1 {
		t.Fatal("the lapse must be recorded once in the case history, the audit trail and the alerts")
	}
	if got := r.kase(t, s.caseID).Status; got != cases.StAwaitingApproval {
		t.Fatalf("case is %s", got)
	}
	// It still holds the remediation lock, so nothing else can be started for this debit.
	if res := r.auto(t, s.caseID); res.Outcome != "approval_expired" {
		t.Fatalf("the automatic path should report the lapse: %+v", res)
	}
	r.chainOK(t)
}

func TestAnApprovalThatLapsesDuringAFreezeIsRecordedAndStillNeedsRenewalAfterTheUnfreeze(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-EX2")
	ctx := context.Background()
	r.freeze(t, killswitch.ScopeGlobal, "")
	// Parked while still valid.
	out, err := r.svc.Execute(ctx, r.tenant, rev.ID)
	if err != nil || out.ParkedAt == nil {
		t.Fatalf("%v %+v", err, out)
	}
	r.lapse(t, rev.ID)

	// The sweep records the lapse even though submissions are frozen.
	r.sweep(t)
	got := r.reversals(t, s.caseID)[0]
	if got.ApprovalStatus != "EXPIRED" || r.providerRequests(t, s.ref) != 0 {
		t.Fatalf("%+v", got)
	}
	r.unfreeze(t, killswitch.ScopeGlobal, "")
	r.sweep(t)
	r.sweep(t)
	if r.providerRequests(t, s.ref) != 0 || r.ledgerCount(t, s.ref) != 0 {
		t.Fatal("a lapsed approval was honoured after the unfreeze")
	}
	if r.reversals(t, s.caseID)[0].ApprovalStatus != "EXPIRED" || r.caseEvents(t, s.caseID, "approval_expired") != 1 {
		t.Fatal("expected one recorded lapse and an expired reversal")
	}
}

func TestAnApprovalThatLapsesWhileParkedIsCaughtWhenTheUnfreezeComes(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-EX3")
	ctx := context.Background()
	r.freeze(t, killswitch.ScopeRail, "NIP")
	if out, err := r.svc.Execute(ctx, r.tenant, rev.ID); err != nil || out.ParkedAt == nil {
		t.Fatalf("%v %+v", err, out)
	}
	r.unfreeze(t, killswitch.ScopeRail, "NIP")
	r.lapse(t, rev.ID) // time passes after the freeze lifts but before the sweep runs
	r.sweep(t)
	if r.providerRequests(t, s.ref) != 0 || r.reversals(t, s.caseID)[0].ApprovalStatus != "EXPIRED" {
		t.Fatalf("%+v", r.reversals(t, s.caseID)[0])
	}
}

func TestAFreshApprovalByADifferentAuthorisedCheckerRestoresTheReversalAndItSubmitsOnce(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-EX4")
	ctx := context.Background()
	r.lapse(t, rev.ID)
	r.sweep(t)

	officer := r.user("REVERSAL_OFFICER")
	// A reason is needed, and a reversal that has not lapsed cannot be renewed.
	if _, err := r.svc.RequestReapproval(ctx, r.tenant, officer, rev.ID, " "); !errors.Is(err, cases.ErrInvalid) {
		t.Fatalf("a reason is required: %v", err)
	}
	fresh, err := r.svc.RequestReapproval(ctx, r.tenant, officer, rev.ID, "customer still waiting")
	if err != nil || fresh.ApprovalStatus != "PENDING" || fresh.ApprovalID == rev.ApprovalID {
		t.Fatalf("%v %+v", err, fresh)
	}
	// Asking again while one is waiting changes nothing.
	again, err := r.svc.RequestReapproval(ctx, r.tenant, officer, rev.ID, "customer still waiting")
	if err != nil || again.ApprovalID != fresh.ApprovalID || r.approvalCount(t, rev.ID) != 2 {
		t.Fatalf("%v %+v approvals=%d", err, again, r.approvalCount(t, rev.ID))
	}
	// Maker-checker holds: the person who asked cannot approve it, and nothing moved.
	if _, err := r.svc.Decide(ctx, r.tenant, officer, fresh.ApprovalID, true, "mine"); err == nil {
		t.Fatal("the requester approved their own fresh approval")
	}
	if r.providerRequests(t, s.ref) != 0 {
		t.Fatal("submitted before a fresh approval")
	}
	// Someone without the authority cannot either.
	if _, err := r.svc.Decide(ctx, r.tenant, r.user("TXN_ANALYST"), fresh.ApprovalID, true, "no"); err == nil {
		t.Fatal("a role without the grant approved")
	}
	// A different authorised checker can.
	done, err := r.svc.Decide(ctx, r.tenant, r.user("SUPERVISOR"), fresh.ApprovalID, true, "renewed")
	if err != nil || done.ApprovalStatus != "APPROVED" || done.ApprovalExpiresAt == nil || !done.ApprovalExpiresAt.After(time.Now()) {
		t.Fatalf("%v %+v", err, done)
	}
	// Four sweeps at once submit it exactly once.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.svc.Sweep(ctx, r.tenant); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	final := r.reversals(t, s.caseID)
	if len(final) != 1 || final[0].FinancialStatus != "VERIFIED" || final[0].ID != rev.ID {
		t.Fatalf("%+v", final)
	}
	if r.providerRequests(t, s.ref) != 1 || r.ledgerCount(t, s.ref) != 1 {
		t.Fatalf("submitted %d times, posted %d times", r.providerRequests(t, s.ref), r.ledgerCount(t, s.ref))
	}
	// Both approvals are on record, the lapse and the renewal are audited, and the
	// domain events say so once each: two approvals, one submission, one verification.
	if r.approvalCount(t, rev.ID) != 2 || r.audits(t, "reversal.reapproval_requested") != 1 {
		t.Fatal("history incomplete")
	}
	counts := map[string]int{}
	for _, e := range r.domain(t) {
		counts[e.Type]++
	}
	if counts[events.ReversalRequested] != 1 || counts[events.ReversalApproved] != 2 || counts[events.ReversalSubmitted] != 1 || counts[events.ReversalVerified] != 1 {
		t.Fatalf("events: %v", counts)
	}
	r.chainOK(t)
}

func TestAFreshApprovalCannotBeRequestedWhenTheChecksNoLongerPassOrTheApprovalHasNotLapsed(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-EX5")
	ctx := context.Background()
	officer := r.user("REVERSAL_OFFICER")
	if _, err := r.svc.RequestReapproval(ctx, r.tenant, officer, rev.ID, "too early"); !errors.Is(err, ErrBadState) {
		t.Fatalf("a valid approval cannot be renewed early: %v", err)
	}
	r.lapse(t, rev.ID)
	r.sweep(t)
	// The authoritative source now says the outcome is unknown: no fresh approval.
	r.script(t, OutcomeUnknown, false, false, 0, s.ref)
	_, err := r.svc.RequestReapproval(ctx, r.tenant, officer, rev.ID, "try anyway")
	var blocked *BlockedError
	if !errors.As(err, &blocked) || !hasFailed(blocked.Gate, 2) {
		t.Fatalf("expected a blocked gate, got %v", err)
	}
	if r.reversals(t, s.caseID)[0].ApprovalStatus != "EXPIRED" || r.approvalCount(t, rev.ID) != 1 {
		t.Fatal("a blocked renewal changed the reversal")
	}
}

func TestConcurrentExecutionsOfAnExpiredApprovalRecordOneLapseAndSubmitNothing(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-EX6")
	r.lapse(t, rev.ID)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.svc.Execute(context.Background(), r.tenant, rev.ID); err != nil && !errors.Is(err, ErrApprovalExpired) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if r.caseEvents(t, s.caseID, "approval_expired") != 1 || r.audits(t, "reversal.approval_expired") != 1 || r.alerts(t, "ops.approval_expired.v1") != 1 {
		t.Fatal("concurrent attempts recorded the lapse more than once")
	}
	if r.providerRequests(t, s.ref) != 0 {
		t.Fatal("something was submitted")
	}
}
