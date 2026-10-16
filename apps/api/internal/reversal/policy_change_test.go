package reversal

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func (r *rig) basisOf(t *testing.T, approvalID interface{ String() string }) (basis, policyVersion string) {
	t.Helper()
	if err := r.pool.QueryRow(context.Background(), `SELECT authority_basis, policy_version FROM approvals WHERE tenant_id = $1 AND id = $2::uuid`,
		r.tenant, approvalID.String()).Scan(&basis, &policyVersion); err != nil {
		t.Fatal(err)
	}
	return
}

func TestAnApprovalRecordsTheSettingsItRelied(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	_, rev := r.approved(t, "INS-PB1")
	basis, version := r.basisOf(t, rev.ApprovalID)
	for _, want := range []string{"approval.auto_limit_minor@", "approval.validity_seconds@", "approval.limit_minor.SUPERVISOR@", "reversal.simulated_execution_enabled@", "reversal.auto_eligible.FAIL001@"} {
		if !contains(basis, want) {
			t.Errorf("authority basis %q lacks %s", basis, want)
		}
	}
	if contains(basis, "status_query") || version == "" {
		t.Errorf("evidence settings do not belong in the basis, and the full version set is kept: %q / %q", basis, version)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestAChangeToTheSettingsAnApprovalReliedOnInvalidatesItAndAFreshApprovalCompletes(t *testing.T) {
	for name, change := range map[string]func(r *rig, t *testing.T){
		"the automatic limit":          func(r *rig, t *testing.T) { r.set(t, "approval.auto_limit_minor", 4_000_000) },
		"the approver's role limit":    func(r *rig, t *testing.T) { r.set(t, "approval.limit_minor.SUPERVISOR", 90_000_000) },
		"the approval validity period": func(r *rig, t *testing.T) { r.set(t, "approval.validity_seconds", 1800) },
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.policy(t)
			s, rev := r.approved(t, "INS-PC")
			ctx := context.Background()
			originalVersion := rev.PolicyVersion
			change(r, t)

			out, err := r.svc.Execute(ctx, r.tenant, rev.ID)
			if !errors.Is(err, ErrApprovalInvalidated) {
				t.Fatalf("expected the approval to be invalidated, got %v", err)
			}
			if out.ApprovalStatus != "EXPIRED" || r.providerRequests(t, s.ref) != 0 || r.ledgerCount(t, s.ref) != 0 {
				t.Fatalf("%+v", out)
			}
			if r.audits(t, "reversal.approval_invalidated") != 1 || r.caseEvents(t, s.caseID, "approval_invalidated") != 1 {
				t.Fatal("the invalidation must be recorded once")
			}
			// The decision's own policy version is untouched.
			if out.PolicyVersion != originalVersion {
				t.Fatalf("the original decision's version was rewritten: %q became %q", originalVersion, out.PolicyVersion)
			}
			// Fresh approval under the new policy goes through.
			fresh, err := r.svc.RequestReapproval(ctx, r.tenant, r.user("REVERSAL_OFFICER"), rev.ID, "policy changed")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.svc.Decide(ctx, r.tenant, r.user("SUPERVISOR"), fresh.ApprovalID, true, "renewed"); err != nil {
				t.Fatal(err)
			}
			first, _ := r.basisOf(t, rev.ApprovalID)
			second, _ := r.basisOf(t, fresh.ApprovalID)
			if first == second {
				t.Fatal("the fresh approval should record the new policy")
			}
			if _, err := r.svc.Execute(ctx, r.tenant, rev.ID); err != nil {
				t.Fatal(err)
			}
			if got, err := r.svc.Advance(ctx, r.tenant, rev.ID); err != nil || got.FinancialStatus != "VERIFIED" {
				t.Fatalf("%v %+v", err, got)
			}
			if r.providerRequests(t, s.ref) != 1 || r.ledgerCount(t, s.ref) != 1 {
				t.Fatal("expected exactly one submission and one posting")
			}
		})
	}
}

func TestSettingsThatOnlyDescribeEvidenceDoNotInvalidateAnApproval(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-PC2")
	r.set(t, "status_query.freshness_seconds", 300) // the gate checks this afresh on every submission
	r.set(t, "waiting_threshold_seconds.NIP", 200)
	if _, err := r.svc.Execute(context.Background(), r.tenant, rev.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := r.svc.Advance(context.Background(), r.tenant, rev.ID); err != nil || got.FinancialStatus != "VERIFIED" || r.providerRequests(t, s.ref) != 1 {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestWhenPolicyNowLetsAutomationSubmitTheEarlierApprovalIsNotNeeded(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-PC3") // approved because automation was not allowed
	r.set(t, "reversal.auto_eligible.FAIL001", 1)
	if _, err := r.svc.Execute(context.Background(), r.tenant, rev.ID); err != nil {
		t.Fatalf("a reversal policy now permits automatically must not need a fresh approval: %v", err)
	}
	if got, err := r.svc.Advance(context.Background(), r.tenant, rev.ID); err != nil || got.FinancialStatus != "VERIFIED" || r.providerRequests(t, s.ref) != 1 {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestAnApprovalThatDidNotRecordItsPolicyIsNotTrusted(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-PC4")
	r.clearBasis(t, rev.ID) // as an approval from before this was recorded
	if _, err := r.svc.Execute(context.Background(), r.tenant, rev.ID); !errors.Is(err, ErrApprovalInvalidated) {
		t.Fatalf("an approval with no recorded basis must fail closed, got %v", err)
	}
	if r.providerRequests(t, s.ref) != 0 {
		t.Fatal("something was submitted")
	}
}

func (r *rig) clearBasis(t *testing.T, revID interface{ String() string }) {
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
	if _, err := tx.Exec(ctx, `UPDATE approvals SET authority_basis = '' WHERE tenant_id = $1 AND reversal_id = $2::uuid`, r.tenant, revID.String()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyDisabledAfterApprovalStopsTheSubmission(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-PC5")
	r.set(t, "reversal.simulated_execution_enabled", 0)
	out, err := r.svc.Execute(context.Background(), r.tenant, rev.ID)
	if err != nil && !errors.Is(err, ErrApprovalInvalidated) {
		t.Fatal(err)
	}
	if r.providerRequests(t, s.ref) != 0 || r.ledgerCount(t, s.ref) != 0 || out.FinancialStatus == "VERIFIED" {
		t.Fatalf("a disabled policy let a reversal through: %+v", out)
	}
}

func TestConcurrentExecutionsAfterAPolicyChangeRecordOneInvalidationAndSubmitNothing(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s, rev := r.approved(t, "INS-PC6")
	r.set(t, "approval.auto_limit_minor", 3_000_000)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.svc.Execute(context.Background(), r.tenant, rev.ID); err != nil && !errors.Is(err, ErrApprovalInvalidated) && !errors.Is(err, ErrApprovalExpired) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if r.audits(t, "reversal.approval_invalidated") != 1 || r.caseEvents(t, s.caseID, "approval_invalidated") != 1 || r.providerRequests(t, s.ref) != 0 {
		t.Fatal("concurrent attempts recorded the invalidation more than once, or submitted")
	}
}
