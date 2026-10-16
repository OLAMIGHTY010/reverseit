package reversal

import (
	"context"
	"testing"

	"github.com/smartreversal/smartreversal/apps/api/internal/events"
)

func TestADecisionNamesItsPolicyAndKeepsTheVersionItWasMadeUnder(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	r.set(t, "reversal.auto_eligible.FAIL001", 0)
	s := r.failed(t, "INS-PID", 10)
	ctx := context.Background()

	res := r.auto(t, s.caseID)
	rev := *res.Reversal
	if rev.PolicyID != "reversal.FAIL001.NIP" || rev.PolicyVersion == "" || rev.PolicyVersion == "no approved policy" {
		t.Fatalf("policy identity: %+v", rev)
	}
	original := rev.PolicyVersion

	// The configuration changes after the decision. The recorded version does not.
	r.set(t, "status_query.freshness_seconds", 300)
	r.set(t, "approval.auto_limit_minor", 4_000_000)

	rev, err := r.svc.Decide(ctx, r.tenant, r.user("SUPERVISOR"), rev.ApprovalID, true, "ok")
	if err != nil {
		t.Fatal(err)
	}
	if rev, err = r.svc.Execute(ctx, r.tenant, rev.ID); err != nil {
		t.Fatal(err)
	}
	if rev, err = r.svc.Advance(ctx, r.tenant, rev.ID); err != nil || rev.FinancialStatus != "VERIFIED" {
		t.Fatalf("%v %+v", err, rev)
	}
	if rev.PolicyID != "reversal.FAIL001.NIP" || rev.PolicyVersion != original {
		t.Fatalf("the original decision's version was rewritten: %q became %q", original, rev.PolicyVersion)
	}

	// Every audit record about this reversal names the policy.
	var total, named int
	if err := r.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE details->>'policy_id' = 'reversal.FAIL001.NIP')
		FROM audit_events WHERE tenant_id = $1 AND subject_type = 'reversal' AND subject_id = $2`, r.tenant, rev.Ref).Scan(&total, &named); err != nil {
		t.Fatal(err)
	}
	if total < 4 || named != total {
		t.Fatalf("%d audit records, %d name the policy", total, named)
	}
	// The domain events carry the id, not the version string.
	found := false
	for _, e := range r.domain(t) {
		if e.Type == events.ReversalVerified && e.Payload["policy_id"] == "reversal.FAIL001.NIP" {
			found = true
		}
		if _, ok := e.Payload["policy_version"]; ok {
			t.Errorf("%s carries the version string", e.Type)
		}
	}
	if !found {
		t.Fatal("reversal.verified does not carry policy_id")
	}
}

func TestABlockedDecisionIsAuditedWithItsPolicyId(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	s := r.failed(t, "INS-PIDB", 10)
	r.script(t, OutcomeUnknown, false, false, 0, s.ref)
	r.auto(t, s.caseID)
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE tenant_id = $1 AND action = 'reversal.blocked' AND details->>'policy_id' = 'reversal.FAIL001.NIP'`, r.tenant).Scan(&n); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
}
