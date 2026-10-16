package reversal

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/events"
)

type domainEvent struct {
	Type    string
	Payload map[string]any
}

// domain returns the reversal events stored for the rig's tenant, in order.
func (r *rig) domain(t *testing.T) []domainEvent {
	t.Helper()
	var out []domainEvent
	r.tx(t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT event_type, payload FROM outbox_events WHERE stream = $1 ORDER BY id`, events.StreamReversals)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e domainEvent
			var raw []byte
			if err := rows.Scan(&e.Type, &raw); err != nil {
				return err
			}
			_ = json.Unmarshal(raw, &e.Payload)
			out = append(out, e)
		}
		return rows.Err()
	})
	return out
}

func types(es []domainEvent) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.Type)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEventsFollowTheRealStateChangesAndNeverRepeat(t *testing.T) {
	r := newRig(t)
	r.policy(t)
	r.set(t, "reversal.auto_eligible.FAIL001", 0)
	s := r.failed(t, "INS-EV1", 10)
	ctx := context.Background()

	res := r.auto(t, s.caseID)
	if res.Outcome != "awaiting_approval" {
		t.Fatalf("%+v", res)
	}
	if got := types(r.domain(t)); !equal(got, []string{events.ReversalRequested}) {
		t.Fatalf("after the request: %v", got)
	}
	// The same request again changes nothing and so emits nothing.
	r.auto(t, s.caseID)
	if got := types(r.domain(t)); len(got) != 1 {
		t.Fatalf("a repeated request emitted %v", got)
	}

	rev, err := r.svc.Decide(ctx, r.tenant, r.user("SUPERVISOR"), res.Reversal.ApprovalID, true, "ok")
	if err != nil {
		t.Fatal(err)
	}
	if rev, err = r.svc.Execute(ctx, r.tenant, rev.ID); err != nil {
		t.Fatal(err)
	}
	submitted := r.domain(t)
	if got := types(submitted); !equal(got, []string{events.ReversalRequested, events.ReversalApproved, events.ReversalSubmitted}) {
		t.Fatalf("after execution: %v", got)
	}
	if p := submitted[2].Payload; p["financial_status"] != "UNVERIFIED" || p["accepted_status"] != "ACCEPTED" {
		t.Fatalf("a submitted reversal is not a verified one: %+v", p)
	}

	if rev, err = r.svc.Advance(ctx, r.tenant, rev.ID); err != nil || rev.FinancialStatus != "VERIFIED" {
		t.Fatalf("%v %+v", err, rev)
	}
	// Repeating the later steps emits nothing more.
	if _, err = r.svc.Execute(ctx, r.tenant, rev.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = r.svc.Advance(ctx, r.tenant, rev.ID); err != nil {
		t.Fatal(err)
	}
	all := r.domain(t)
	if got := types(all); !equal(got, []string{events.ReversalRequested, events.ReversalApproved, events.ReversalSubmitted, events.ReversalVerified}) {
		t.Fatalf("final events: %v", got)
	}
	v := all[3].Payload
	if v["financial_status"] != "VERIFIED" || v["reversal_ref"] != rev.Ref || v["amount_minor"] != "1850000" || v["currency"] != "NGN" ||
		v["case_id"] != rev.CaseID.String() || v["tenant_id"] != r.tenant.String() || v["schema_version"] != float64(events.SchemaVersion) {
		t.Fatalf("verified payload: %+v", v)
	}
	for _, e := range all {
		for _, k := range []string{"debit_account_token", "beneficiary_token", "idempotency_key", "provider_ref", "gate"} {
			if _, ok := e.Payload[k]; ok {
				t.Errorf("%s carries %s", e.Type, k)
			}
		}
	}
}

func TestABlockedGateARejectedApprovalAndATimeoutPublishNoSuccessEvent(t *testing.T) {
	t.Run("blocked gate creates no reversal and no event", func(t *testing.T) {
		r := newRig(t)
		r.policy(t)
		s := r.failed(t, "INS-EV2", 10)
		r.script(t, OutcomeUnknown, false, false, 0, s.ref)
		if res := r.auto(t, s.caseID); res.Outcome != "held" {
			t.Fatalf("%+v", res)
		}
		if got := r.domain(t); len(got) != 0 {
			t.Fatalf("a blocked gate emitted %v", types(got))
		}
	})
	t.Run("rejected approval is requested and nothing else", func(t *testing.T) {
		r := newRig(t)
		r.policy(t)
		r.set(t, "reversal.auto_eligible.FAIL001", 0)
		s := r.failed(t, "INS-EV3", 10)
		res := r.auto(t, s.caseID)
		if _, err := r.svc.Decide(context.Background(), r.tenant, r.user("SUPERVISOR"), res.Reversal.ApprovalID, false, "not satisfied"); err != nil {
			t.Fatal(err)
		}
		if got := types(r.domain(t)); !equal(got, []string{events.ReversalRequested}) {
			t.Fatalf("%v", got)
		}
	})
	t.Run("provider rejection is a failure, not a success", func(t *testing.T) {
		r := newRig(t)
		r.policy(t)
		s := r.failed(t, "INS-EV4", 10)
		r.provider(t, s.ref, "REJECT")
		r.auto(t, s.caseID)
		got := r.domain(t)
		if last := got[len(got)-1]; last.Type != events.ReversalFailed || last.Payload["failure_code"] != "PROVIDER_REJECTED" || last.Payload["financial_status"] != "FAILED" {
			t.Fatalf("%+v", got)
		}
		for _, e := range got {
			if e.Type == events.ReversalVerified {
				t.Fatal("a rejected reversal was reported as verified")
			}
		}
	})
	t.Run("a provider that claims success without a posting is not verified", func(t *testing.T) {
		r := newRig(t)
		r.policy(t)
		s := r.failed(t, "INS-EV5", 10)
		r.provider(t, s.ref, "GHOST")
		r.auto(t, s.caseID)
		got := r.domain(t)
		if last := got[len(got)-1]; last.Type != events.ReversalFailed || last.Payload["failure_code"] != "LEDGER_MISMATCH" {
			t.Fatalf("%+v", got)
		}
		for _, e := range got {
			if e.Type == events.ReversalVerified {
				t.Fatal("an unverified claim became a verified event")
			}
		}
	})
	t.Run("a timeout is not a submission, and a later verification is reported once", func(t *testing.T) {
		r := newRig(t)
		r.policy(t)
		s := r.failed(t, "INS-EV6", 10)
		r.provider(t, s.ref, "TIMEOUT")
		res := r.auto(t, s.caseID)
		verified := 0
		for _, e := range r.domain(t) {
			if e.Type == events.ReversalSubmitted {
				t.Fatal("an unacknowledged submission was reported as submitted")
			}
			if e.Type == events.ReversalVerified {
				verified++
			}
		}
		// The same key found out what happened. Verified is reported only if the ledger agrees, and only once.
		if got := r.reversals(t, s.caseID); len(got) != 1 || (got[0].FinancialStatus == "VERIFIED") != (verified == 1) || verified > 1 {
			t.Fatalf("verified events %d, reversals %+v, result %+v", verified, got, res)
		}
	})
}
