package routing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type rig struct {
	pool   *db.Pool
	svc    *Service
	tenant uuid.UUID
	now    time.Time
	users  *identity.PGUserStore
	n      int
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), now: time.Now().UTC().Truncate(time.Second), users: &identity.PGUserStore{Pool: pool}}
	r.svc = &Service{Audit: &audit.Appender{}, Now: func() time.Time { return r.now }}
	return r
}

func (r *rig) user(t *testing.T, roles ...string) uuid.UUID {
	t.Helper()
	r.n++
	id, err := r.users.Create(context.Background(), r.tenant, "u"+string(rune('a'+r.n))+"@example.test", "User", "a long enough passphrase", roles)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) tx(t *testing.T, fn func(tx pgx.Tx) error) error {
	t.Helper()
	return r.pool.WithTenant(context.Background(), r.tenant, fn)
}

func (r *rig) delegate(t *testing.T, from, to uuid.UUID, start, end time.Time) (Delegation, error) {
	t.Helper()
	var d Delegation
	err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		d, err = r.svc.Delegate(context.Background(), tx, r.tenant, from, to, start, end, "out of office on leave")
		return err
	})
	return d, err
}

func (r *rig) assignee(t *testing.T, u uuid.UUID, at time.Time) Routed {
	t.Helper()
	var out Routed
	if err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		out, err = r.svc.Assignee(context.Background(), tx, r.tenant, u, at)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

const day = 24 * time.Hour

func TestWorkGoesToTheDelegateOnlyWhileTheDelegationIsActive(t *testing.T) {
	r := newRig(t)
	alice, bob := r.user(t, "SUPERVISOR"), r.user(t, "SUPERVISOR")
	start, end := r.now.Add(day), r.now.Add(8*day)
	d, err := r.delegate(t, alice, bob, start, end)
	if err != nil {
		t.Fatal(err)
	}

	for name, c := range map[string]struct {
		at   time.Time
		want uuid.UUID
	}{
		"before it starts":      {r.now, alice},
		"on the first instant":  {start, bob},
		"in the middle":         {r.now.Add(4 * day), bob},
		"at the end it is over": {end, alice},
		"after":                 {end.Add(time.Hour), alice},
	} {
		if got := r.assignee(t, alice, c.at); got.Assignee != c.want {
			t.Errorf("%s: %s, want %s", name, got.Assignee, c.want)
		}
	}
	if got := r.assignee(t, alice, r.now.Add(2*day)); len(got.Via) != 1 || got.Via[0] != d.ID {
		t.Errorf("the delegation followed is not reported: %+v", got)
	}
	if got := r.assignee(t, bob, r.now.Add(2*day)); got.Assignee != bob {
		t.Errorf("work for the delegate was redirected: %+v", got)
	}
}

func TestOnlyTheDelegatorCanRevokeAndRevokingStopsRouting(t *testing.T) {
	r := newRig(t)
	alice, bob, carol := r.user(t, "SUPERVISOR"), r.user(t, "SUPERVISOR"), r.user(t, "SUPERVISOR")
	d, _ := r.delegate(t, alice, bob, r.now, r.now.Add(5*day))
	at := r.now.Add(time.Hour)
	if r.assignee(t, alice, at).Assignee != bob {
		t.Fatal("setup: delegation not active")
	}

	revoke := func(who uuid.UUID) error {
		return r.tx(t, func(tx pgx.Tx) error { return r.svc.Revoke(context.Background(), tx, r.tenant, who, d.ID) })
	}
	if err := revoke(bob); !errors.Is(err, ErrNotYours) {
		t.Errorf("the delegate revoked: %v", err)
	}
	if err := revoke(carol); !errors.Is(err, ErrNotYours) {
		t.Errorf("a stranger revoked: %v", err)
	}
	if r.assignee(t, alice, at).Assignee != bob {
		t.Fatal("a refused revoke still stopped the delegation")
	}
	if err := revoke(alice); err != nil {
		t.Fatal(err)
	}
	if r.assignee(t, alice, at).Assignee != alice {
		t.Error("work still goes to the delegate after revocation")
	}
	if err := revoke(alice); !errors.Is(err, ErrAlreadyDone) {
		t.Errorf("revoking twice: %v", err)
	}
	if err := r.tx(t, func(tx pgx.Tx) error { return r.svc.Revoke(context.Background(), tx, r.tenant, alice, uuid.New()) }); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown delegation: %v", err)
	}
}

func TestDelegationRules(t *testing.T) {
	r := newRig(t)
	sup, other, analyst := r.user(t, "SUPERVISOR"), r.user(t, "SUPERVISOR"), r.user(t, "TXN_ANALYST")
	off := r.user(t, "SUPERVISOR")
	if _, err := r.pool.Exec(context.Background(), `UPDATE users SET status = 'disabled' WHERE id = $1`, off); err != nil {
		t.Fatal(err)
	}
	start := r.now.Add(time.Hour)

	cases := map[string]struct {
		to         uuid.UUID
		start, end time.Time
		want       error
	}{
		"to oneself":                     {sup, start, start.Add(day), ErrInvalid},
		"ends before it starts":          {other, start, start.Add(-time.Hour), ErrInvalid},
		"longer than 30 days":            {other, start, start.Add(31 * day), ErrInvalid},
		"starts in the past":             {other, r.now.Add(-time.Hour), start, ErrInvalid},
		"to a disabled user":             {off, start, start.Add(day), ErrNotFound},
		"to a stranger":                  {uuid.New(), start, start.Add(day), ErrNotFound},
		"to someone with no shared role": {analyst, start, start.Add(day), ErrNoSharedRole},
	}
	for name, c := range cases {
		if _, err := r.delegate(t, sup, c.to, c.start, c.end); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if _, err := r.delegate(t, sup, other, start, start.Add(30*day)); err != nil {
		t.Errorf("exactly 30 days should be allowed: %v", err)
	}
	// A blank reason is refused.
	err := r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.Delegate(context.Background(), tx, r.tenant, sup, other, start, start.Add(day), "  ")
		return err
	})
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("blank reason: %v", err)
	}
}

func TestDelegationChainsAreFollowedAFewHopsAndLoopsAreSafe(t *testing.T) {
	r := newRig(t)
	a, b, c, d, e := r.user(t, "SUPERVISOR"), r.user(t, "SUPERVISOR"), r.user(t, "SUPERVISOR"), r.user(t, "SUPERVISOR"), r.user(t, "SUPERVISOR")
	start, end := r.now, r.now.Add(5*day)
	for _, p := range [][2]uuid.UUID{{a, b}, {b, c}, {c, d}, {d, e}} {
		if _, err := r.delegate(t, p[0], p[1], start, end); err != nil {
			t.Fatal(err)
		}
	}
	got := r.assignee(t, a, r.now.Add(time.Hour))
	if got.Assignee != d || len(got.Via) != 3 {
		t.Fatalf("a chain is followed at most three hops: %+v (want %s)", got, d)
	}

	// A loop: e delegates back to a. The walk stops instead of circling.
	if _, err := r.delegate(t, e, a, start, end); err != nil {
		t.Fatal(err)
	}
	loop := r.assignee(t, e, r.now.Add(time.Hour))
	if len(loop.Via) > maxHops {
		t.Fatalf("loop followed %d hops", len(loop.Via))
	}
}

func TestDelegationsAreHistoryAndTenantIsolated(t *testing.T) {
	r := newRig(t)
	alice, bob := r.user(t, "SUPERVISOR"), r.user(t, "SUPERVISOR")
	r.delegate(t, alice, bob, r.now, r.now.Add(day))
	for _, stmt := range []string{`UPDATE delegations SET reason = 'x'`, `DELETE FROM delegations`, `DELETE FROM delegation_revocations`, `UPDATE on_call_shifts SET role = 'AGENT'`, `DELETE FROM on_call_shifts`} {
		if err := r.tx(t, func(tx pgx.Tx) error { _, err := tx.Exec(context.Background(), stmt); return err }); err == nil {
			t.Errorf("tenant role ran %q", stmt)
		}
	}
	other := newRig(t)
	var list []Delegation
	_ = other.tx(t, func(tx pgx.Tx) error {
		var err error
		list, err = other.svc.ActiveDelegations(context.Background(), tx, other.tenant, alice, other.now)
		return err
	})
	if len(list) != 0 {
		t.Fatalf("another tenant sees delegations: %+v", list)
	}
	var mine []Delegation
	_ = r.tx(t, func(tx pgx.Tx) error {
		var err error
		mine, err = r.svc.ActiveDelegations(context.Background(), tx, r.tenant, alice, r.now)
		return err
	})
	if len(mine) != 1 || mine[0].Delegate != bob {
		t.Fatalf("own delegations: %+v", mine)
	}
}

func TestOnCallShiftsAndDelegation(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	officer1, officer2, outsider := r.user(t, "REVERSAL_OFFICER"), r.user(t, "REVERSAL_OFFICER"), r.user(t, "TXN_ANALYST")
	admin := r.user(t, "ADMIN_INTEGRATION")

	add := func(role string, u uuid.UUID, s, e time.Time) error {
		return r.tx(t, func(tx pgx.Tx) error { _, err := r.svc.AddShift(ctx, tx, r.tenant, admin, role, u, s, e); return err })
	}
	if err := add("REVERSAL_OFFICER", officer1, r.now, r.now.Add(8*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := add("REVERSAL_OFFICER", officer2, r.now.Add(8*time.Hour), r.now.Add(16*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := add("REVERSAL_OFFICER", outsider, r.now, r.now.Add(time.Hour)); !errors.Is(err, ErrInvalid) {
		t.Errorf("a shift for someone without the role: %v", err)
	}
	if err := add("REVERSAL_OFFICER", officer1, r.now.Add(time.Hour), r.now); !errors.Is(err, ErrInvalid) {
		t.Errorf("a shift that ends first: %v", err)
	}

	onCall := func(role string, at time.Time) []uuid.UUID {
		var out []uuid.UUID
		if err := r.tx(t, func(tx pgx.Tx) error {
			var err error
			out, err = r.svc.OnCall(ctx, tx, r.tenant, role, at)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := onCall("REVERSAL_OFFICER", r.now.Add(time.Hour)); len(got) != 1 || got[0] != officer1 {
		t.Errorf("first shift: %v", got)
	}
	if got := onCall("REVERSAL_OFFICER", r.now.Add(9*time.Hour)); len(got) != 1 || got[0] != officer2 {
		t.Errorf("second shift: %v", got)
	}
	if got := onCall("REVERSAL_OFFICER", r.now.Add(20*time.Hour)); len(got) != 0 {
		t.Errorf("after the shifts: %v", got)
	}
	if got := onCall("SUPERVISOR", r.now.Add(time.Hour)); len(got) != 0 {
		t.Errorf("another role: %v", got)
	}

	// officer1 is out of office: the on-call duty lands on a colleague.
	colleague := r.user(t, "REVERSAL_OFFICER")
	if _, err := r.delegate(t, officer1, colleague, r.now, r.now.Add(2*day)); err != nil {
		t.Fatal(err)
	}
	if got := onCall("REVERSAL_OFFICER", r.now.Add(time.Hour)); len(got) != 1 || got[0] != colleague {
		t.Errorf("delegation did not move the on-call duty: %v", got)
	}
}

func TestEscalationWalksUpUntilSomeoneIsOnCall(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	admin := r.user(t, "ADMIN_INTEGRATION")
	sup, head := r.user(t, "SUPERVISOR"), r.user(t, "HEAD_OPS")
	step := func(from, to string) {
		if err := r.tx(t, func(tx pgx.Tx) error { return r.svc.SetEscalation(ctx, tx, r.tenant, admin, from, to) }); err != nil {
			t.Fatal(err)
		}
	}
	escalate := func(from string) (Target, error) {
		var out Target
		err := r.tx(t, func(tx pgx.Tx) error {
			var err error
			out, err = r.svc.Escalate(ctx, tx, r.tenant, from, r.now.Add(time.Hour))
			return err
		})
		return out, err
	}
	shift := func(role string, u uuid.UUID) {
		if err := r.tx(t, func(tx pgx.Tx) error {
			_, err := r.svc.AddShift(ctx, tx, r.tenant, admin, role, u, r.now, r.now.Add(day))
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := escalate("REVERSAL_OFFICER"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no steps configured: %v", err)
	}
	step("REVERSAL_OFFICER", "SUPERVISOR")
	step("SUPERVISOR", "HEAD_OPS")
	if _, err := escalate("REVERSAL_OFFICER"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nobody on call anywhere must not invent a target: %v", err)
	}
	shift("HEAD_OPS", head)
	got, err := escalate("REVERSAL_OFFICER")
	if err != nil || got.Role != "HEAD_OPS" || len(got.Users) != 1 || got.Users[0] != head {
		t.Fatalf("skipping an empty role: %+v %v", got, err)
	}
	shift("SUPERVISOR", sup)
	got, err = escalate("REVERSAL_OFFICER")
	if err != nil || got.Role != "SUPERVISOR" || got.Users[0] != sup {
		t.Fatalf("first role with someone on call: %+v %v", got, err)
	}

	// A step can be replaced, and loops and bad input are handled.
	step("REVERSAL_OFFICER", "HEAD_OPS")
	if got, _ := escalate("REVERSAL_OFFICER"); got.Role != "HEAD_OPS" {
		t.Errorf("replaced step: %+v", got)
	}
	step("HEAD_OPS", "REVERSAL_OFFICER")
	step("SUPERVISOR", "REVERSAL_OFFICER")
	if _, err := escalate("TXN_ANALYST"); !errors.Is(err, ErrNotFound) {
		t.Errorf("no step for the role: %v", err)
	}
	for name, c := range map[string][2]string{"same role": {"SUPERVISOR", "SUPERVISOR"}, "empty": {"", "SUPERVISOR"}} {
		if err := r.tx(t, func(tx pgx.Tx) error { return r.svc.SetEscalation(ctx, tx, r.tenant, admin, c[0], c[1]) }); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := r.tx(t, func(tx pgx.Tx) error {
		return r.svc.SetEscalation(ctx, tx, r.tenant, admin, "NO_SUCH_ROLE", "SUPERVISOR")
	}); err == nil {
		t.Error("an unknown role was accepted")
	}
}

func TestRoutingChangesAreAuditedAndTheChainVerifies(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	alice, bob := r.user(t, "SUPERVISOR"), r.user(t, "SUPERVISOR")
	d, _ := r.delegate(t, alice, bob, r.now, r.now.Add(day))
	_ = r.tx(t, func(tx pgx.Tx) error { return r.svc.Revoke(ctx, tx, r.tenant, alice, d.ID) })
	_ = r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.AddShift(ctx, tx, r.tenant, alice, "SUPERVISOR", bob, r.now, r.now.Add(day))
		return err
	})
	_ = r.tx(t, func(tx pgx.Tx) error { return r.svc.SetEscalation(ctx, tx, r.tenant, alice, "SUPERVISOR", "HEAD_OPS") })

	var actions []string
	var res audit.VerifyResult
	if err := r.tx(t, func(tx pgx.Tx) error {
		recs, err := audit.List(ctx, tx, r.tenant, audit.Filter{ActorID: alice}, 0, 50)
		for _, rec := range recs {
			actions = append(actions, rec.Action)
		}
		if err != nil {
			return err
		}
		res, err = audit.Verify(ctx, tx, r.tenant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"delegation.created", "delegation.revoked", "oncall.shift_added", "escalation.set"}
	if len(actions) != len(want) || !res.OK {
		t.Fatalf("actions %v, chain %+v", actions, res)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("actions %v, want %v", actions, want)
		}
	}
}
