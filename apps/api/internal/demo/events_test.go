package demo

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/events"
	"github.com/smartreversal/smartreversal/apps/api/internal/outbox"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/stream"
)

func countEvents(t *testing.T, env *Env, tenant uuid.UUID, typ string) int {
	t.Helper()
	return count(t, env, tenant, `SELECT count(*) FROM outbox_events WHERE tenant_id = $1 AND event_type = '`+typ+`'`)
}

// The scenario's domain events go through the real outbox, relay and Redis
// Streams. Events survive a Redis outage, are consumed once per group, and
// replaying the demonstration neither repeats financial remediation nor
// reports a second verification.
func TestDomainEventsFlowThroughTheRelayAndStreamsAndTheDemoReplayStaysSafe(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set; skipping Redis integration test")
	}
	ctx := context.Background()
	pool := dbtest.Pool(t)
	tenant := dbtest.Tenant(t, pool)
	var code string
	if err := pool.QueryRow(ctx, `SELECT code FROM institutions WHERE id = $1`, tenant).Scan(&code); err != nil {
		t.Fatal(err)
	}
	env, err := NewEnv(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Seed(ctx, env, code, "a long synthetic demo passphrase"); err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	good := redis.NewClient(&redis.Options{Addr: addr})
	defer good.Close()
	streams := []string{events.StreamCases, events.StreamReversals, events.StreamNotifications}
	clean := func() { good.Del(ctx, append(streams, stream.DeadLetterStream(events.StreamCases))...) }
	clean()
	defer clean()
	if _, err := pool.Exec(ctx, `UPDATE outbox_events SET published_at = now() WHERE published_at IS NULL`); err != nil {
		t.Fatal(err) // other tests' leftovers are not ours to publish
	}

	for run := 1; run <= 2; run++ {
		clean() // Reset removes outbox rows; the streams of the previous run would still hold theirs
		if err := Reset(ctx, pool, tenant); err != nil {
			t.Fatal(err)
		}
		rep, err := Play(ctx, env, tenant)
		if err != nil {
			t.Fatal(err)
		}

		// What the scenario should have announced, from the outbox itself.
		want := map[string]int{
			events.CaseOpened:        3, // failed transfer, duplicate, unknown
			events.ReversalRequested: 2, // failed transfer (automatic) and duplicate (waiting)
			events.ReversalApproved:  0, // nobody approved anything
			events.ReversalSubmitted: 1, // only the failed transfer reached the provider
			events.ReversalVerified:  1, // and only it was verified against the ledger
			events.ReversalFailed:    0,
		}
		for typ, n := range want {
			if got := countEvents(t, env, tenant, typ); got != n {
				t.Fatalf("run %d: %s events = %d, want %d", run, typ, got, n)
			}
		}
		if countEvents(t, env, tenant, events.CaseTransitioned) < 4 || countEvents(t, env, tenant, events.NotificationRecorded) < 4 {
			t.Fatalf("run %d: expected transitions and notifications to be announced", run)
		}
		// The unknown transfer produced no reversal event of any kind.
		if n := count(t, env, tenant, `SELECT count(*) FROM outbox_events o WHERE o.tenant_id = $1 AND o.stream = 'reversals'
			AND o.payload->>'case_id' IN (SELECT id::text FROM cases WHERE tenant_id = $1 AND classification = 'UNK001')`); n != 0 {
			t.Fatalf("run %d: %d reversal events for the unknown transfer", run, n)
		}
		if rep.Unk.Outcome != "held" || rep.Dup.CaseStatus != "AWAITING_APPROVAL" || rep.Fail.Financial != "VERIFIED" {
			t.Fatalf("run %d: the scenario changed: %+v", run, rep)
		}
		if n := count(t, env, tenant, `SELECT count(*) FROM sim_ledger_entries WHERE tenant_id = $1 AND kind = 'REVERSAL_CREDIT'`); n != 1 {
			t.Fatalf("run %d: %d reversal postings, want 1", run, n)
		}

		// Redis is down: nothing is lost, nothing is marked published.
		dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
		down := &outbox.Relay{Pool: pool, Redis: dead, Logger: quiet}
		if _, err := down.PublishPending(ctx); err == nil {
			t.Fatalf("run %d: publishing to a dead Redis should fail", run)
		}
		dead.Close()
		var pending int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE tenant_id = $1 AND published_at IS NULL`, tenant).Scan(&pending); err != nil || pending == 0 {
			t.Fatalf("run %d: events must wait for Redis: %d %v", run, pending, err)
		}

		// Redis is back: everything is published.
		up := &outbox.Relay{Pool: pool, Redis: good, Logger: quiet}
		for {
			n, err := up.PublishPending(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				break
			}
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE tenant_id = $1 AND published_at IS NULL`, tenant).Scan(&pending); err != nil || pending != 0 {
			t.Fatalf("run %d: %d events still unpublished: %v", run, pending, err)
		}

		// Two members of one group share each stream. Every event is handled once.
		var mu sync.Mutex
		handled := map[uuid.UUID]int{}
		group := fmt.Sprintf("domain-test-%d-%d", run, time.Now().UnixNano())
		handler := func(_ context.Context, _ pgx.Tx, m stream.Message) error {
			if m.TenantID != tenant {
				return nil
			}
			mu.Lock()
			handled[m.EventID]++
			mu.Unlock()
			return nil
		}
		for _, name := range streams {
			for _, member := range []string{"a", "b"} {
				c := &stream.Consumer{Pool: pool, Redis: good, Logger: quiet, Stream: name, Group: group, Name: member, Handler: handler,
					Block: 20 * time.Millisecond, ClaimIdle: time.Hour}
				if err := c.Setup(ctx); err != nil {
					t.Fatal(err)
				}
				for {
					n, err := c.RunOnce(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if n == 0 {
						break
					}
				}
			}
		}
		var published int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE tenant_id = $1 AND stream = ANY($2)`, tenant, streams).Scan(&published); err != nil {
			t.Fatal(err)
		}
		if len(handled) != published {
			t.Fatalf("run %d: handled %d distinct events, published %d", run, len(handled), published)
		}
		for id, n := range handled {
			if n != 1 {
				t.Fatalf("run %d: event %s handled %d times", run, id, n)
			}
		}

		// Publishing everything again (as recovery after Redis data loss would)
		// must not make the group handle anything twice.
		if _, err := outbox.Requeue(ctx, pool, 0); err != nil {
			t.Fatal(err)
		}
		for {
			n, err := up.PublishPending(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				break
			}
		}
		c := &stream.Consumer{Pool: pool, Redis: good, Logger: quiet, Stream: events.StreamReversals, Group: group, Name: "a", Handler: handler,
			Block: 20 * time.Millisecond, ClaimIdle: time.Hour}
		for {
			n, err := c.RunOnce(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				break
			}
		}
		for id, n := range handled {
			if n != 1 {
				t.Fatalf("run %d: after a replay event %s was handled %d times", run, id, n)
			}
		}
	}
}
