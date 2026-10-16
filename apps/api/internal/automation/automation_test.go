package automation

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

func TestADeferralIsKeptOnceAndCanBeResumedOnce(t *testing.T) {
	pool := dbtest.Pool(t)
	tenant := dbtest.Tenant(t, pool)
	ctx := context.Background()
	subject := uuid.New()
	run := func(fn func(tx pgx.Tx) error) {
		t.Helper()
		if err := pool.WithTenant(ctx, tenant, fn); err != nil {
			t.Fatal(err)
		}
	}
	run(func(tx pgx.Tx) error {
		if err := Defer(ctx, tx, tenant, KindDetect, subject, &killswitch.FrozenError{Scope: killswitch.ScopeRail, Target: "NIP"}); err != nil {
			return err
		}
		// Blocked again, by a wider freeze: still one open row, now naming the new scope.
		return Defer(ctx, tx, tenant, KindDetect, subject, &killswitch.FrozenError{Scope: killswitch.ScopeGlobal})
	})
	run(func(tx pgx.Tx) error {
		c, err := Counts(ctx, tx, tenant)
		if err != nil || c[KindDetect] != 1 {
			t.Fatalf("counts %v %v", c, err)
		}
		items, err := Claim(ctx, tx, tenant, KindDetect, 10)
		if err != nil || len(items) != 1 || items[0].SubjectID != subject || items[0].BlockedScope != "global" {
			t.Fatalf("claim %+v %v", items, err)
		}
		return Resumed(ctx, tx, tenant, items[0].ID)
	})
	run(func(tx pgx.Tx) error {
		items, _ := Claim(ctx, tx, tenant, KindDetect, 10)
		if len(items) != 0 {
			t.Fatalf("a resumed item was claimed again: %+v", items)
		}
		// The same subject can be held again later.
		return Defer(ctx, tx, tenant, KindDetect, subject, &killswitch.FrozenError{Scope: killswitch.ScopeGlobal})
	})
	run(func(tx pgx.Tx) error {
		c, _ := Counts(ctx, tx, tenant)
		if c[KindDetect] != 1 {
			t.Fatalf("counts %v", c)
		}
		return nil
	})
}

func TestTwoWorkersNeverClaimTheSameItem(t *testing.T) {
	pool := dbtest.Pool(t)
	tenant := dbtest.Tenant(t, pool)
	ctx := context.Background()
	if err := pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		for i := 0; i < 4; i++ {
			if err := Defer(ctx, tx, tenant, KindDetect, uuid.New(), &killswitch.FrozenError{Scope: killswitch.ScopeGlobal}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Worker A holds two items locked inside its open transaction while worker B claims.
	var a, b []Deferred
	err := pool.WithTenant(ctx, tenant, func(txA pgx.Tx) error {
		var err error
		if a, err = Claim(ctx, txA, tenant, KindDetect, 2); err != nil {
			return err
		}
		return pool.WithTenant(ctx, tenant, func(txB pgx.Tx) error {
			b, err = Claim(ctx, txB, tenant, KindDetect, 10)
			return err
		})
	})
	if err != nil || len(a) != 2 {
		t.Fatalf("a: %v %v", a, err)
	}
	if len(b) != 2 {
		t.Fatalf("worker B should get the other two, got %d", len(b))
	}
	seen := map[uuid.UUID]bool{}
	for _, d := range append(a, b...) {
		if seen[d.ID] {
			t.Fatalf("item %s claimed twice", d.ID)
		}
		seen[d.ID] = true
	}
}

func TestJobStateReportsPausesAndOnlyMovesSinceOnAChange(t *testing.T) {
	pool := dbtest.Pool(t)
	tenant, other := dbtest.Tenant(t, pool), dbtest.Tenant(t, pool)
	ctx := context.Background()
	set := func(paused bool, frozen string) {
		t.Helper()
		if err := pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			return RecordJob(ctx, tx, tenant, JobReversalSweep, paused, frozen, map[string]any{"handled": 3})
		}); err != nil {
			t.Fatal(err)
		}
	}
	get := func(tn uuid.UUID) []Job {
		var out []Job
		if err := pool.WithTenant(ctx, tn, func(tx pgx.Tx) error {
			var err error
			out, err = Jobs(ctx, tx, tn)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	set(false, "")
	first := get(tenant)[0]
	time.Sleep(20 * time.Millisecond)
	set(false, "rail:NIP")
	same := get(tenant)[0]
	if !same.Since.Equal(first.Since) || same.Frozen != "rail:NIP" || same.State != "running" {
		t.Fatalf("%+v then %+v", first, same)
	}
	set(true, "global")
	paused := get(tenant)[0]
	if paused.State != "paused" || !paused.Since.After(first.Since) {
		t.Fatalf("%+v", paused)
	}
	if len(get(other)) != 0 {
		t.Fatal("another institution must not see these jobs")
	}
}
