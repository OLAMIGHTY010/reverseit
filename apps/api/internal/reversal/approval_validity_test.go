package reversal

import (
	"context"
	"errors"
	"testing"
	"time"
)

func (r *rig) approvalRow(t *testing.T, revID interface{ String() string }) (status string, expires *time.Time, decided *time.Time, policy string) {
	t.Helper()
	err := r.pool.QueryRow(context.Background(), `SELECT status, expires_at, decided_at, policy_id FROM approvals
		WHERE tenant_id = $1 AND reversal_id = $2::uuid ORDER BY created_at DESC LIMIT 1`, r.tenant, revID.String()).Scan(&status, &expires, &decided, &policy)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestAnApprovalRecordsWhenItsAuthorityEndsFromTheConfiguredValidity(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	r.set(t, "approval.validity_seconds", 900)
	r.set(t, "reversal.auto_eligible.FAIL001", 0)
	s := r.failed(t, "INS-AV1", 10)
	res := r.auto(t, s.caseID)
	rev, err := r.svc.Decide(context.Background(), r.tenant, r.user("SUPERVISOR"), res.Reversal.ApprovalID, true, "ok")
	if err != nil {
		t.Fatal(err)
	}
	status, expires, decided, policy := r.approvalRow(t, rev.ID)
	if status != "APPROVED" || expires == nil || decided == nil || policy != "reversal.FAIL001.NIP" {
		t.Fatalf("%s %v %v %q", status, expires, decided, policy)
	}
	if got := expires.Sub(*decided); got < 899*time.Second || got > 901*time.Second {
		t.Fatalf("expected 900 seconds of validity, got %s", got)
	}
	if rev.ApprovalExpiresAt == nil || !rev.ApprovalExpiresAt.Equal(*expires) {
		t.Fatalf("the reversal should show when its approval ends: %+v", rev.ApprovalExpiresAt)
	}
}

func TestNothingCanBeApprovedWithoutAnApprovedValidityPeriodButItCanStillBeRejected(t *testing.T) {
	// rig.policy sets a validity period, so this tenant gets every policy except that one.
	r2 := newRig(t)
	for k, v := range map[string]int{"duplicate.window_seconds": 30, "status_query.freshness_seconds": 120, "waiting_threshold_seconds.NIP": 300,
		"approval.auto_limit_minor": 5_000_000, "reversal.simulated_execution_enabled": 1, "approval.limit_minor.SUPERVISOR": 100_000_000} {
		r2.set(t, k, v)
	}
	s := r2.failed(t, "INS-AV2", 10)
	res := r2.auto(t, s.caseID)
	if res.Outcome != "awaiting_approval" {
		t.Fatalf("%+v", res)
	}
	ctx := context.Background()
	if _, err := r2.svc.Decide(ctx, r2.tenant, r2.user("SUPERVISOR"), res.Reversal.ApprovalID, true, "ok"); !errors.Is(err, ErrNoApprovalPolicy) {
		t.Fatalf("approving without a validity period must fail closed, got %v", err)
	}
	if status, _, _, _ := r2.approvalRow(t, res.Reversal.ID); status != "PENDING" {
		t.Fatalf("the approval changed to %s", status)
	}
	if r2.providerRequests(t, s.ref) != 0 {
		t.Fatal("something was submitted")
	}
	// Refusing needs no validity period.
	if _, err := r2.svc.Decide(ctx, r2.tenant, r2.user("SUPERVISOR"), res.Reversal.ApprovalID, false, "not satisfied"); err != nil {
		t.Fatalf("rejecting must stay possible: %v", err)
	}
}
