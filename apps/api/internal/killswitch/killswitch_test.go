package killswitch

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type rig struct {
	pool   *db.Pool
	svc    *Service
	tenant uuid.UUID
	a, b   uuid.UUID // two different people
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := dbtest.Pool(t)
	return &rig{pool: pool, svc: &Service{Audit: &audit.Appender{}}, tenant: dbtest.Tenant(t, pool), a: uuid.New(), b: uuid.New()}
}

func (r *rig) tx(t *testing.T, fn func(tx pgx.Tx) error) error {
	t.Helper()
	return r.pool.WithTenant(context.Background(), r.tenant, fn)
}

func (r *rig) freeze(t *testing.T, tg Target) {
	t.Helper()
	if err := r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.Freeze(context.Background(), tx, r.tenant, r.a, tg, "incident under review")
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) check(t *testing.T, a Action) error {
	t.Helper()
	var out error
	if err := r.tx(t, func(tx pgx.Tx) error { out = r.svc.Check(context.Background(), tx, r.tenant, a); return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *rig) state(t *testing.T) State {
	t.Helper()
	var st State
	if err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		st, err = r.svc.State(context.Background(), tx, r.tenant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return st
}

var (
	global = Target{ScopeGlobal, ""}
	nip    = Target{ScopeRail, "NIP"}
	dup    = Target{ScopeExceptionType, "DUP001"}
	pol    = Target{ScopePolicy, "P-NIPREV-3.2"}
)

func TestNothingIsFrozenByDefault(t *testing.T) {
	r := newRig(t)
	if err := r.check(t, Action{Rail: "NIP", ExceptionType: "DUP001", PolicyID: "P-NIPREV-3.2"}); err != nil {
		t.Fatal(err)
	}
	if st := r.state(t); len(st.Frozen) != 0 || len(st.Pending) != 0 {
		t.Fatalf("%+v", st)
	}
}

func TestEachScopeBlocksOnlyWhatItCovers(t *testing.T) {
	cases := []struct {
		name    string
		freeze  Target
		blocked []Action
		allowed []Action
	}{
		{"global blocks everything", global,
			[]Action{{}, {Rail: "NIP"}, {ExceptionType: "FAIL001"}, {PolicyID: "x"}}, nil},
		{"rail", nip,
			[]Action{{Rail: "NIP"}, {Rail: "NIP", ExceptionType: "FAIL001"}},
			[]Action{{}, {Rail: "POS"}, {ExceptionType: "DUP001"}, {Rail: "NIPX"}}},
		{"exception type", dup,
			[]Action{{ExceptionType: "DUP001"}, {Rail: "POS", ExceptionType: "DUP001"}},
			[]Action{{}, {ExceptionType: "FAIL001"}, {Rail: "NIP"}}},
		{"policy", pol,
			[]Action{{PolicyID: "P-NIPREV-3.2"}},
			[]Action{{}, {PolicyID: "P-NIPREV-3.1"}, {Rail: "NIP"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			r.freeze(t, c.freeze)
			for _, a := range c.blocked {
				var fe *FrozenError
				err := r.check(t, a)
				if !errors.As(err, &fe) || fe.Scope != c.freeze.Scope {
					t.Errorf("%+v should be blocked by %s: %v", a, c.freeze.Scope, err)
				}
			}
			for _, a := range c.allowed {
				if err := r.check(t, a); err != nil {
					t.Errorf("%+v wrongly blocked: %v", a, err)
				}
			}
		})
	}
}

func TestFreezingIsImmediateIdempotentAndNeedsOnePerson(t *testing.T) {
	r := newRig(t)
	var changed1, changed2 bool
	if err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		changed1, err = r.svc.Freeze(context.Background(), tx, r.tenant, r.a, nip, "suspected duplicate posting")
		return err
	}); err != nil || !changed1 {
		t.Fatalf("first freeze: %v %v", changed1, err)
	}
	if err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		changed2, err = r.svc.Freeze(context.Background(), tx, r.tenant, r.b, nip, "second person agrees")
		return err
	}); err != nil || changed2 {
		t.Fatalf("second freeze should change nothing: %v %v", changed2, err)
	}
	st := r.state(t)
	if len(st.Frozen) != 1 || st.Frozen[0].By != r.a || st.Frozen[0].Reason != "suspected duplicate posting" {
		t.Fatalf("%+v", st)
	}
}

func TestLiftingNeedsTwoDifferentPeople(t *testing.T) {
	r := newRig(t)
	r.freeze(t, nip)
	ctx := context.Background()

	var seq int64
	if err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		seq, err = r.svc.RequestUnfreeze(ctx, tx, r.tenant, r.a, nip, "root cause fixed and verified")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.check(t, Action{Rail: "NIP"}); err == nil {
		t.Fatal("a request alone lifted the freeze")
	}
	if st := r.state(t); len(st.Pending) != 1 || st.Pending[0].RequestedBy != r.a || st.Pending[0].RequestSeq != seq {
		t.Fatalf("pending: %+v", st.Pending)
	}

	// The requester cannot approve, through the service or the database.
	err := r.tx(t, func(tx pgx.Tx) error {
		return r.svc.DecideUnfreeze(ctx, tx, r.tenant, r.a, seq, true, "approving my own request")
	})
	if !errors.Is(err, ErrSameActor) {
		t.Fatalf("service: %v", err)
	}
	err = r.tx(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO kill_switch_events (tenant_id, scope, target, kind, actor_id, reason, request_seq)
			VALUES ($1, 'rail', 'NIP', 'unfreeze_approve', $2, 'self approval', $3)`, r.tenant, r.a, seq)
		return err
	})
	if err == nil {
		t.Fatal("the database accepted a self-approved unfreeze")
	}
	if err := r.check(t, Action{Rail: "NIP"}); err == nil {
		t.Fatal("a self approval lifted the freeze")
	}

	// A second person approves.
	if err := r.tx(t, func(tx pgx.Tx) error {
		return r.svc.DecideUnfreeze(ctx, tx, r.tenant, r.b, seq, true, "verified the fix")
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.check(t, Action{Rail: "NIP"}); err != nil {
		t.Fatalf("still frozen after a valid approval: %v", err)
	}
	if st := r.state(t); len(st.Frozen) != 0 || len(st.Pending) != 0 {
		t.Fatalf("%+v", st)
	}

	// It can be frozen again afterwards.
	r.freeze(t, nip)
	if err := r.check(t, Action{Rail: "NIP"}); err == nil {
		t.Fatal("re-freeze had no effect")
	}
}

func TestRejectedRequestKeepsTheFreezeAndAllowsANewRequest(t *testing.T) {
	r := newRig(t)
	r.freeze(t, global)
	ctx := context.Background()
	var seq int64
	_ = r.tx(t, func(tx pgx.Tx) error {
		var err error
		seq, err = r.svc.RequestUnfreeze(ctx, tx, r.tenant, r.a, global, "please resume automation")
		return err
	})
	if err := r.tx(t, func(tx pgx.Tx) error {
		return r.svc.DecideUnfreeze(ctx, tx, r.tenant, r.b, seq, false, "root cause not shown")
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.check(t, Action{}); err == nil {
		t.Fatal("a rejected request lifted the freeze")
	}
	if st := r.state(t); len(st.Frozen) != 1 || len(st.Pending) != 0 {
		t.Fatalf("%+v", st)
	}
	err := r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.RequestUnfreeze(ctx, tx, r.tenant, r.a, global, "second attempt with evidence")
		return err
	})
	if err != nil {
		t.Fatalf("a new request after a rejection: %v", err)
	}
}

func TestRequestAndDecisionRules(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.freeze(t, nip)
	var seq int64
	_ = r.tx(t, func(tx pgx.Tx) error {
		var err error
		seq, err = r.svc.RequestUnfreeze(ctx, tx, r.tenant, r.a, nip, "root cause fixed")
		return err
	})

	if err := r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.RequestUnfreeze(ctx, tx, r.tenant, r.b, nip, "another request")
		return err
	}); !errors.Is(err, ErrPending) {
		t.Errorf("second open request: %v", err)
	}
	if err := r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.RequestUnfreeze(ctx, tx, r.tenant, r.a, pol, "not frozen at all")
		return err
	}); !errors.Is(err, ErrNotFrozen) {
		t.Errorf("request for a scope that is not frozen: %v", err)
	}
	if err := r.tx(t, func(tx pgx.Tx) error {
		return r.svc.DecideUnfreeze(ctx, tx, r.tenant, r.b, 9999, true, "no such request")
	}); !errors.Is(err, ErrNoSuchRequest) {
		t.Errorf("unknown request: %v", err)
	}
	if err := r.tx(t, func(tx pgx.Tx) error { return r.svc.DecideUnfreeze(ctx, tx, r.tenant, r.b, seq, true, "ok") }); !errors.Is(err, ErrInvalid) {
		t.Errorf("short reason: %v", err)
	}
	if err := r.tx(t, func(tx pgx.Tx) error {
		return r.svc.DecideUnfreeze(ctx, tx, r.tenant, r.b, seq, true, "verified the fix")
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.tx(t, func(tx pgx.Tx) error {
		return r.svc.DecideUnfreeze(ctx, tx, r.tenant, uuid.New(), seq, true, "decided twice")
	}); !errors.Is(err, ErrNoSuchRequest) {
		t.Errorf("deciding a closed request: %v", err)
	}
}

func TestInputValidation(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	for name, tg := range map[string]Target{
		"unknown scope":            {"everything", ""},
		"global with a target":     {ScopeGlobal, "NIP"},
		"rail without a target":    {ScopeRail, ""},
		"lower case rail":          {ScopeRail, "nip"},
		"rail with a space":        {ScopeRail, "N P"},
		"exception type too short": {ScopeExceptionType, "D"},
		"policy with a slash":      {ScopePolicy, "a/b"},
		"policy too long":          {ScopePolicy, string(make([]byte, 61))},
	} {
		err := r.tx(t, func(tx pgx.Tx) error {
			_, err := r.svc.Freeze(ctx, tx, r.tenant, r.a, tg, "incident under review")
			return err
		})
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, c := range map[string]struct {
		actor  uuid.UUID
		reason string
	}{"no actor": {uuid.Nil, "incident under review"}, "no reason": {r.a, ""}, "short reason": {r.a, "no"}} {
		err := r.tx(t, func(tx pgx.Tx) error {
			_, err := r.svc.Freeze(ctx, tx, r.tenant, c.actor, nip, c.reason)
			return err
		})
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if st := r.state(t); len(st.Frozen) != 0 {
		t.Fatalf("invalid input froze something: %+v", st)
	}
}

func TestACheckThatCannotReadTheSwitchBlocks(t *testing.T) {
	r := newRig(t)
	// Break the read: query inside a transaction that has already failed.
	err := r.tx(t, func(tx pgx.Tx) error {
		_, _ = tx.Exec(context.Background(), `SELECT 1/0`) // aborts the transaction
		if err := r.svc.Check(context.Background(), tx, r.tenant, Action{Rail: "NIP"}); err == nil {
			t.Error("Check allowed an action although it could not read the switch")
		}
		return nil
	})
	_ = err
}

func TestEventsAreAppendOnlyAndTenantIsolated(t *testing.T) {
	r := newRig(t)
	r.freeze(t, nip)
	for _, stmt := range []string{`UPDATE kill_switch_events SET reason = 'x'`, `DELETE FROM kill_switch_events`, `TRUNCATE kill_switch_events`} {
		if err := r.tx(t, func(tx pgx.Tx) error { _, err := tx.Exec(context.Background(), stmt); return err }); err == nil {
			t.Errorf("tenant role ran %q", stmt)
		}
		if _, err := r.pool.Exec(context.Background(), stmt); err == nil {
			t.Errorf("owner ran %q", stmt)
		}
	}
	other := newRig(t)
	if err := other.check(t, Action{Rail: "NIP"}); err != nil {
		t.Fatalf("another tenant was frozen by this one: %v", err)
	}
}

func TestChangesAreAudited(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.freeze(t, nip)
	var seq int64
	_ = r.tx(t, func(tx pgx.Tx) error {
		var err error
		seq, err = r.svc.RequestUnfreeze(ctx, tx, r.tenant, r.a, nip, "root cause fixed")
		return err
	})
	_ = r.tx(t, func(tx pgx.Tx) error {
		return r.svc.DecideUnfreeze(ctx, tx, r.tenant, r.b, seq, true, "verified the fix")
	})

	var recs []audit.Record
	var res audit.VerifyResult
	if err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		if recs, err = audit.List(ctx, tx, r.tenant, audit.Filter{SubjectType: "killswitch"}, 0, 20); err != nil {
			return err
		}
		res, err = audit.Verify(ctx, tx, r.tenant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"killswitch.frozen", "killswitch.unfreeze_requested", "killswitch.unfrozen"}
	if len(recs) != 3 {
		t.Fatalf("%d records", len(recs))
	}
	for i, w := range want {
		if recs[i].Action != w {
			t.Errorf("record %d is %s, want %s", i, recs[i].Action, w)
		}
	}
	if recs[0].ActorID != r.a || recs[2].ActorID != r.b || recs[0].SubjectID != "rail:NIP" || !res.OK {
		t.Errorf("attribution or chain wrong: %+v %+v", recs, res)
	}
}

func TestAPolicyFreezeCoversAnActionWhosePolicyIsUnknown(t *testing.T) {
	policy := State{Frozen: []Frozen{{Target: Target{Scope: ScopePolicy, Value: "reversal.FAIL001.NIP"}}}}
	for name, tc := range map[string]struct {
		a    Action
		want bool
	}{
		"the named policy":              {Action{PolicyID: "reversal.FAIL001.NIP"}, true},
		"another policy":                {Action{PolicyID: "reversal.FAIL001.CARD"}, false},
		"no policy and not marked":      {Action{Rail: "NIP", ExceptionType: "FAIL001"}, false},
		"policy unknown":                {Action{Rail: "NIP", PolicyUnknown: true}, true},
		"unknown, with a mismatched id": {Action{PolicyID: "x", PolicyUnknown: true}, true},
	} {
		if got := policy.Covers(tc.a) != nil; got != tc.want {
			t.Errorf("%s: covered=%v want %v", name, got, tc.want)
		}
	}
	rail := State{Frozen: []Frozen{{Target: Target{Scope: ScopeRail, Value: "CARD"}}}}
	if rail.Covers(Action{Rail: "NIP", PolicyUnknown: true}) != nil {
		t.Error("an unknown policy must not make a rail freeze on another rail cover the action")
	}
}
