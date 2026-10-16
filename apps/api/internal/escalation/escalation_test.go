package escalation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type rig struct {
	pool   *db.Pool
	tenant uuid.UUID
	t      *testing.T
}

func newRig(t *testing.T) *rig {
	pool := dbtest.Pool(t)
	return &rig{pool: pool, tenant: dbtest.Tenant(t, pool), t: t}
}

func (r *rig) do(fn func(tx pgx.Tx) error) error {
	return r.pool.WithTenant(context.Background(), r.tenant, fn)
}

var step1 = Step{No: 1, Label: "Contact the bank's dispute channel", Contact: "SYNTHETIC PLACEHOLDER CONTACT", Wording: "If this is not resolved by the target time, please contact the bank's dispute channel."}

func TestAStepIsShownOnlyAfterSomeoneOtherThanItsAuthorApprovesIt(t *testing.T) {
	r := newRig(t)
	author, reviewer := uuid.New(), uuid.New()
	ctx := context.Background()
	if err := r.do(func(tx pgx.Tx) error { return Propose(ctx, tx, r.tenant, author, step1) }); err != nil {
		t.Fatal(err)
	}
	approved := func() int {
		var n int
		_ = r.do(func(tx pgx.Tx) error { s, err := Approved(ctx, tx, r.tenant); n = len(s); return err })
		return n
	}
	if approved() != 0 {
		t.Fatal("a draft was shown to customers")
	}
	if err := r.do(func(tx pgx.Tx) error { return Decide(ctx, tx, r.tenant, author, 1, true, "", time.Now()) }); !errors.Is(err, ErrFourEyes) {
		t.Fatalf("the author approved their own step: %v", err)
	}
	if err := r.do(func(tx pgx.Tx) error { return Decide(ctx, tx, r.tenant, reviewer, 1, false, "", time.Now()) }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a rejection without a note: %v", err)
	}
	if err := r.do(func(tx pgx.Tx) error {
		return Decide(ctx, tx, r.tenant, reviewer, 1, true, "wording checked", time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if approved() != 1 {
		t.Fatal("an approved step is not shown")
	}
	// Approved wording cannot be overwritten, and a decision is made once.
	if err := r.do(func(tx pgx.Tx) error {
		s := step1
		s.Wording = "Some different wording that nobody has reviewed."
		return Propose(ctx, tx, r.tenant, author, s)
	}); !errors.Is(err, ErrState) {
		t.Fatalf("approved wording changed: %v", err)
	}
	if err := r.do(func(tx pgx.Tx) error { return Decide(ctx, tx, r.tenant, uuid.New(), 1, true, "again", time.Now()) }); !errors.Is(err, ErrState) {
		t.Fatalf("%v", err)
	}
	if err := r.do(func(tx pgx.Tx) error { return Withdraw(ctx, tx, r.tenant, 1) }); err != nil {
		t.Fatal(err)
	}
	if approved() != 0 {
		t.Fatal("a withdrawn step is still shown")
	}
}

func TestStepsAreValidatedOrderedAndPerInstitution(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	a, b := uuid.New(), uuid.New()
	for name, s := range map[string]Step{
		"step zero":     {No: 0, Label: "abc", Contact: "contact", Wording: "long enough wording"},
		"step eleven":   {No: 11, Label: "abc", Contact: "contact", Wording: "long enough wording"},
		"no label":      {No: 1, Contact: "contact", Wording: "long enough wording"},
		"no contact":    {No: 1, Label: "abc", Wording: "long enough wording"},
		"short wording": {No: 1, Label: "abc", Contact: "contact", Wording: "short"},
		"long wording":  {No: 1, Label: "abc", Contact: "contact", Wording: strings.Repeat("w", 601)},
	} {
		if err := r.do(func(tx pgx.Tx) error { return Propose(ctx, tx, r.tenant, a, s) }); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, n := range []int{3, 1, 2} {
		s := step1
		s.No = n
		_ = r.do(func(tx pgx.Tx) error { return Propose(ctx, tx, r.tenant, a, s) })
		_ = r.do(func(tx pgx.Tx) error { return Decide(ctx, tx, r.tenant, b, n, true, "ok", time.Now()) })
	}
	var got []Step
	_ = r.do(func(tx pgx.Tx) error { var err error; got, err = Approved(ctx, tx, r.tenant); return err })
	if len(got) != 3 || got[0].No != 1 || got[2].No != 3 {
		t.Fatalf("%+v", got)
	}
	other := dbtest.Tenant(t, r.pool)
	_ = r.pool.WithTenant(ctx, other, func(tx pgx.Tx) error { s, err := Approved(ctx, tx, other); got = s; return err })
	if len(got) != 0 {
		t.Fatalf("another institution sees %d steps", len(got))
	}
	if err := r.do(func(tx pgx.Tx) error { return Decide(ctx, tx, r.tenant, b, 9, true, "x", time.Now()) }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
}

func TestTheLadderIsDueOnlyWhenTheBindingDeadlineHasPassedAndTheCaseIsUnfinished(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Minute), now.Add(time.Hour)
	for name, c := range map[string]struct {
		sla, internal *time.Time
		finished      bool
		want          bool
	}{
		"no deadline at all":              {nil, nil, false, false},
		"regulatory not yet":              {&future, nil, false, false},
		"regulatory passed":               {&past, nil, false, true},
		"internal passed, regulatory not": {&future, &past, false, true},
		"exactly at the deadline":         {&now, nil, false, true},
		"passed but the case is finished": {&past, &past, true, false},
	} {
		if got := Due(now, c.sla, c.internal, c.finished); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}
