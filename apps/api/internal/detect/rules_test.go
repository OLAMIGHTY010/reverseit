package detect

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/rules"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

func (r *rig) withRules() *rules.Store {
	st := &rules.Store{Audit: &audit.Appender{Now: func() time.Time { return r.now }}, Now: func() time.Time { return r.now }}
	r.det.Rules = &rules.Engine{Store: st, Now: func() time.Time { return r.now }}
	return st
}

func (r *rig) newRule(t *testing.T, st *rules.Store, doc string) rules.Rule {
	t.Helper()
	rl, err := rules.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		_, err := st.Create(context.Background(), tx, r.tenant, uuid.New(), rl)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return rl
}

func (r *rig) step(t *testing.T, fn func(ctx context.Context, tx pgx.Tx) error) {
	t.Helper()
	if err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error { return fn(context.Background(), tx) }); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) activate(t *testing.T, st *rules.Store, rl rules.Rule) {
	t.Helper()
	maker, requester, checker := uuid.New(), uuid.New(), uuid.New()
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := st.StartShadow(ctx, tx, r.tenant, maker, rl.RuleID, rl.Version)
		return err
	})
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.det.Rules.Backtest(ctx, tx, r.tenant, rl.RuleID, rl.Version, 100)
		return err
	})
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := st.RequestActivation(ctx, tx, r.tenant, requester, rl.RuleID, rl.Version, "shadow looked right")
		return err
	})
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := st.Decide(ctx, tx, r.tenant, checker, rl.RuleID, rl.Version, true, "approved")
		return err
	})
}

const stuckRule = `{"rule_id":"DET-LONG-1","version":1,"code":"UNK001","risk":"HIGH","root_cause":"switch_response_failure",
 "explanation":"Still waiting for a final answer more than ten minutes after the last event.",
 "all":[{"field":"lifecycle_state","op":"in","value":["PENDING","DEBITED"]},{"field":"seconds_since_last_event","op":"gte","value":600}]}`

func TestALiveRuleOpensACaseUnderItsOwnIdAndVersionAndAShadowRuleOnlyRecords(t *testing.T) {
	r := newRig(t)
	st := r.withRules()
	rl := r.newRule(t, st, stuckRule)

	at := r.now.Add(-20 * time.Minute)
	r.event(t, "INS-RL1", txn.EvDebitPosted, at)
	id := r.event(t, "INS-RL1", txn.EvTransferSent, at.Add(time.Second)).TxnID

	// Shadow first: it is evaluated and recorded and opens nothing.
	maker := uuid.New()
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := st.StartShadow(ctx, tx, r.tenant, maker, rl.RuleID, 1)
		return err
	})
	if got := r.eval(t, id); len(got) != 0 {
		t.Fatalf("a shadow rule opened %d cases", len(got))
	}
	if len(r.allCases(t)) != 0 {
		t.Fatal("a case exists")
	}
	var shadow int
	_ = r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM rule_evaluations WHERE mode = 'shadow' AND outcome = 'matched'`).Scan(&shadow)
	})
	if shadow != 1 {
		t.Fatalf("%d shadow matches recorded", shadow)
	}

	// Approve it. Now a sweep, with no new event, opens the case.
	requester, checker := uuid.New(), uuid.New()
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := st.RequestActivation(ctx, tx, r.tenant, requester, rl.RuleID, 1, "matched the stuck transfer in shadow")
		return err
	})
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := st.Decide(ctx, tx, r.tenant, checker, rl.RuleID, 1, true, "approved")
		return err
	})
	var found []Finding
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		found, err = r.det.SweepRules(ctx, tx, r.tenant)
		return err
	})
	if len(found) != 1 || !found[0].Created {
		t.Fatalf("%+v", found)
	}
	c := found[0].Case
	if c.Classification != cases.CodeUnknown || c.RuleID != "DET-LONG-1" || c.RuleVersion != "1" || c.RiskGrade != "HIGH" || c.RootCause != "switch_response_failure" {
		t.Fatalf("%+v", c)
	}
	// Running again changes nothing.
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		again, err := r.det.SweepRules(ctx, tx, r.tenant)
		if len(again) != 1 || again[0].Created {
			t.Fatalf("%+v", again)
		}
		return err
	})
	if n := len(r.allCases(t)); n != 1 {
		t.Fatalf("%d cases", n)
	}
}

func TestARuleCannotCallAnUnknownOutcomeAFailureAndTheBuiltInRulesStillRun(t *testing.T) {
	r := newRig(t)
	st := r.withRules()
	// The only way to open FAIL001 declaratively is to require a confirmed failure.
	if _, err := rules.Parse([]byte(`{"rule_id":"DET-BAD-1","version":1,"code":"FAIL001","root_cause":"some_cause","explanation":"Anything that waited too long.",
		"all":[{"field":"seconds_since_last_event","op":"gte","value":60}]}`)); err == nil {
		t.Fatal("a rule that would call a wait a failure was accepted")
	}
	// A live custom rule does not disturb the built-in ones.
	rl := r.newRule(t, st, stuckRule)
	at := r.now.Add(-time.Hour)
	failed := r.failedTransfer(t, "INS-RL2", at)
	r.activate(t, st, rl)
	got := r.eval(t, failed)
	if len(got) != 1 || got[0].Case.Classification != cases.CodeNoCredit || got[0].Case.RuleID != RuleFailure {
		t.Fatalf("%+v", got)
	}
}

func TestARuleThatNeedsAFreshSourceOpensNothingWhileTheSourceIsSilent(t *testing.T) {
	r := newRig(t)
	st := r.withRules()
	doc := `{"rule_id":"DET-FRESH-1","version":1,"code":"UNK001","root_cause":"switch_response_failure","explanation":"Waiting on a source that is itself reporting.",
		"requires_fresh":["SWITCH_A"],"all":[{"field":"lifecycle_state","op":"in","value":["PENDING"]},{"field":"seconds_since_last_event","op":"gte","value":60}]}`
	rl := r.newRule(t, st, doc)
	at := r.now.Add(-10 * time.Minute)
	r.event(t, "INS-RL3", txn.EvDebitPosted, at)
	r.event(t, "INS-RL3", txn.EvTransferSent, at.Add(time.Second))
	r.activate(t, st, rl)

	sweep := func() []Finding {
		var f []Finding
		r.step(t, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			f, err = r.det.SweepRules(ctx, tx, r.tenant)
			return err
		})
		return f
	}
	// SWITCH_A has no expectation, so it is not known to be fresh: nothing is opened.
	if f := sweep(); len(f) != 0 {
		t.Fatalf("a rule ran against a source that is not known to be fresh: %+v", f)
	}
}
