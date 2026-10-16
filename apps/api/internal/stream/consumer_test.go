package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

func testPool(t *testing.T) *db.Pool {
	t.Helper()
	p := dbtest.Pool(t)
	ctx := context.Background()
	if _, err := p.Exec(ctx, `DELETE FROM processed_events`); err != nil {
		t.Fatal(err)
	}
	// Handlers run as the tenant role, so it needs access to the scratch table.
	if _, err := p.Exec(ctx, `DROP TABLE IF EXISTS stream_test_effects;
		CREATE TABLE stream_test_effects (event_id uuid PRIMARY KEY);
		GRANT SELECT, INSERT ON stream_test_effects TO sr_app`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = p.Exec(context.Background(), `DROP TABLE IF EXISTS stream_test_effects`) })
	return p
}

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set; skipping Redis integration test")
	}
	c := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

type fixture struct {
	tenant uuid.UUID
	pool   *db.Pool
	rc     *redis.Client
	stream string
	group  string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testPool(t)
	f := &fixture{
		tenant: dbtest.Tenant(t, pool),
		pool:   pool,
		rc:     testRedis(t),
		stream: fmt.Sprintf("test.stream.%d", time.Now().UnixNano()),
		group:  "test-group",
	}
	t.Cleanup(func() {
		f.rc.Del(context.Background(), f.stream, DeadLetterStream(f.stream))
	})
	return f
}

func (f *fixture) consumer(name string, h Handler) *Consumer {
	return &Consumer{
		Pool: f.pool, Redis: f.rc, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Stream: f.stream, Group: f.group, Name: name, Handler: h,
		Block: 50 * time.Millisecond, ClaimIdle: 100 * time.Millisecond, MaxDeliveries: 3,
	}
}

func (f *fixture) publish(t *testing.T, eventID uuid.UUID) string {
	t.Helper()
	id, err := f.rc.XAdd(context.Background(), &redis.XAddArgs{Stream: f.stream, Values: map[string]any{
		"tenant_id": f.tenant.String(), "event_id": eventID.String(), "type": "test.happened", "aggregate_id": "A-1",
		"payload": `{"n":1}`, "trace_id": "trace-1",
	}}).Result()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fixture) pending(t *testing.T) int64 {
	t.Helper()
	p, err := f.rc.XPending(context.Background(), f.stream, f.group).Result()
	if err != nil {
		t.Fatal(err)
	}
	return p.Count
}

func (f *fixture) effects(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM stream_test_effects`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// recordEffect is a handler that writes one row per event in the shared transaction.
func recordEffect(ctx context.Context, tx pgx.Tx, m Message) error {
	_, err := tx.Exec(ctx, `INSERT INTO stream_test_effects (event_id) VALUES ($1)`, m.EventID)
	return err
}

func TestSetupValidatesAndIsRepeatable(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	bad := f.consumer("c1", nil)
	if err := bad.Setup(ctx); err == nil {
		t.Fatal("missing handler accepted")
	}

	c := f.consumer("c1", recordEffect)
	if err := c.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Setup(ctx); err != nil {
		t.Fatalf("second setup failed: %v", err)
	}
}

func TestProcessesMessageOnceAndAcknowledges(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var got Message
	c := f.consumer("c1", func(ctx context.Context, tx pgx.Tx, m Message) error {
		got = m
		return recordEffect(ctx, tx, m)
	})
	if err := c.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	evt := uuid.New()
	f.publish(t, evt)

	n, err := c.RunOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if got.EventID != evt || got.Type != "test.happened" || got.AggregateID != "A-1" ||
		got.TraceID != "trace-1" || string(got.Payload) != `{"n":1}` || got.Deliveries != 1 {
		t.Fatalf("handler saw %+v", got)
	}
	if f.effects(t) != 1 || f.pending(t) != 0 {
		t.Fatalf("effects=%d pending=%d", f.effects(t), f.pending(t))
	}
}

func TestDuplicateEventIsSkippedButAcknowledged(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var calls atomic.Int32
	c := f.consumer("c1", func(ctx context.Context, tx pgx.Tx, m Message) error {
		calls.Add(1)
		return recordEffect(ctx, tx, m)
	})
	if err := c.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	evt := uuid.New()
	f.publish(t, evt)
	f.publish(t, evt) // relay re-published after a crash

	if _, err := c.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || f.effects(t) != 1 {
		t.Fatalf("handler ran %d times, effects=%d; want 1 and 1", calls.Load(), f.effects(t))
	}
	if f.pending(t) != 0 {
		t.Fatalf("duplicate left %d pending", f.pending(t))
	}
}

func TestHandlerErrorRollsBackAndRetriesAfterIdle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var calls atomic.Int32
	c := f.consumer("c1", func(ctx context.Context, tx pgx.Tx, m Message) error {
		if err := recordEffect(ctx, tx, m); err != nil {
			return err
		}
		if calls.Add(1) == 1 {
			return errors.New("temporary failure")
		}
		return nil
	})
	if err := c.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	f.publish(t, uuid.New())

	if _, err := c.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if f.effects(t) != 0 {
		t.Fatal("failed handler's write was not rolled back")
	}
	var processed int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM processed_events`).Scan(&processed)
	if processed != 0 {
		t.Fatalf("failed handler left %d processed_events rows", processed)
	}
	if f.pending(t) != 1 {
		t.Fatalf("pending=%d, want 1 awaiting retry", f.pending(t))
	}

	time.Sleep(150 * time.Millisecond) // longer than ClaimIdle
	if _, err := c.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || f.effects(t) != 1 || f.pending(t) != 0 {
		t.Fatalf("calls=%d effects=%d pending=%d", calls.Load(), f.effects(t), f.pending(t))
	}
}

func TestPoisonMessageIsDeadLetteredAfterMaxDeliveries(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var calls atomic.Int32
	c := f.consumer("c1", func(ctx context.Context, tx pgx.Tx, m Message) error {
		calls.Add(1)
		return errors.New("always fails")
	})
	if err := c.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	evt := uuid.New()
	f.publish(t, evt)

	for i := 0; i < 8; i++ {
		if _, err := c.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
		if f.pending(t) == 0 {
			break
		}
		time.Sleep(150 * time.Millisecond) // longer than ClaimIdle
	}

	if f.pending(t) != 0 {
		t.Fatalf("message still pending: %d", f.pending(t))
	}
	if calls.Load() != 3 {
		t.Fatalf("handler ran %d times, want MaxDeliveries (3)", calls.Load())
	}
	dl, err := f.rc.XRange(ctx, DeadLetterStream(f.stream), "-", "+").Result()
	if err != nil || len(dl) != 1 {
		t.Fatalf("dead letters=%d err=%v", len(dl), err)
	}
	v := dl[0].Values
	if v["event_id"] != evt.String() || v["dlq_reason"] != "exceeded maximum deliveries" ||
		v["dlq_source_stream"] != f.stream || v["dlq_group"] != f.group || v["dlq_original_id"] == "" {
		t.Fatalf("dead letter missing detail: %+v", v)
	}
}

func TestMalformedMessagesGoStraightToDeadLetter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var calls atomic.Int32
	c := f.consumer("c1", func(ctx context.Context, tx pgx.Tx, m Message) error {
		calls.Add(1)
		return nil
	})
	if err := c.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	tid := f.tenant.String()
	for _, values := range []map[string]any{
		{"tenant_id": tid, "type": "x", "payload": "{}"},                                             // no event id
		{"tenant_id": tid, "event_id": "not-a-uuid", "type": "x", "payload": "{}"},                   // bad id
		{"tenant_id": tid, "event_id": uuid.NewString(), "payload": "{}"},                            // no type
		{"tenant_id": tid, "event_id": uuid.NewString(), "type": "x"},                                // no payload
		{"event_id": uuid.NewString(), "type": "x", "payload": "{}"},                                 // no tenant
		{"tenant_id": "nope", "event_id": uuid.NewString(), "type": "x", "payload": "{}"},            // bad tenant
		{"tenant_id": uuid.Nil.String(), "event_id": uuid.NewString(), "type": "x", "payload": "{}"}, // nil tenant
	} {
		if err := f.rc.XAdd(ctx, &redis.XAddArgs{Stream: f.stream, Values: values}).Err(); err != nil {
			t.Fatal(err)
		}
	}

	if n, err := c.RunOnce(ctx); err != nil || n != 7 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("handler ran %d times for malformed messages", calls.Load())
	}
	if l, _ := f.rc.XLen(ctx, DeadLetterStream(f.stream)).Result(); l != 7 {
		t.Fatalf("dead letters=%d, want 7", l)
	}
	if f.pending(t) != 0 {
		t.Fatalf("pending=%d", f.pending(t))
	}
}

func TestAnotherConsumerRecoversWorkLeftByACrashedOne(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	c2 := f.consumer("survivor", recordEffect)
	if err := c2.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	evt := uuid.New()
	f.publish(t, evt)

	// "crashed" reads the entry and never acknowledges it.
	res, err := f.rc.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: f.group, Consumer: "crashed", Streams: []string{f.stream, ">"}, Count: 10, Block: 50 * time.Millisecond,
	}).Result()
	if err != nil || len(res) != 1 || len(res[0].Messages) != 1 {
		t.Fatalf("setup read failed: %v %+v", err, res)
	}
	if f.pending(t) != 1 {
		t.Fatal("expected one pending entry owned by the crashed consumer")
	}

	// Too soon: the survivor leaves it alone.
	if n, err := c2.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("claimed too early: n=%d err=%v", n, err)
	}

	time.Sleep(150 * time.Millisecond)
	if n, err := c2.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("recovery n=%d err=%v", n, err)
	}
	if f.effects(t) != 1 || f.pending(t) != 0 {
		t.Fatalf("effects=%d pending=%d", f.effects(t), f.pending(t))
	}
}

func TestCrashAfterCommitBeforeAckDoesNotRepeatTheEffect(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var calls atomic.Int32
	h := func(ctx context.Context, tx pgx.Tx, m Message) error {
		calls.Add(1)
		return recordEffect(ctx, tx, m)
	}
	c := f.consumer("c1", h)
	if err := c.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	evt := uuid.New()
	f.publish(t, evt)

	// Simulate: the handler committed, then the process died before XACK.
	if _, err := f.pool.Exec(ctx, `INSERT INTO processed_events (tenant_id, consumer, event_id) VALUES ($1, $2, $3)`, f.tenant, f.group, evt); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || f.effects(t) != 0 || f.pending(t) != 0 {
		t.Fatalf("calls=%d effects=%d pending=%d; want 0 0 0", calls.Load(), f.effects(t), f.pending(t))
	}
}

func TestRunProcessesUntilCancelled(t *testing.T) {
	f := newFixture(t)
	c := f.consumer("c1", recordEffect)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	f.publish(t, uuid.New())
	deadline := time.Now().Add(3 * time.Second)
	for f.effects(t) < 1 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if f.effects(t) != 1 {
		t.Fatal("Run did not process the message")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestReplayDeadLettersReturnsEntriesToTheStream(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var fail atomic.Bool
	fail.Store(true)
	c := f.consumer("c1", func(ctx context.Context, tx pgx.Tx, m Message) error {
		if fail.Load() {
			return errors.New("downstream broken")
		}
		return recordEffect(ctx, tx, m)
	})
	if err := c.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	evt := uuid.New()
	f.publish(t, evt)

	for i := 0; i < 8; i++ {
		if _, err := c.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
		if f.pending(t) == 0 {
			break
		}
		time.Sleep(150 * time.Millisecond) // longer than ClaimIdle
	}
	if l, _ := f.rc.XLen(ctx, DeadLetterStream(f.stream)).Result(); l != 1 {
		t.Fatalf("expected 1 dead letter, got %d", l)
	}

	fail.Store(false) // the cause is fixed
	n, err := ReplayDeadLetters(ctx, f.rc, f.stream, 10)
	if err != nil || n != 1 {
		t.Fatalf("replay n=%d err=%v", n, err)
	}
	if l, _ := f.rc.XLen(ctx, DeadLetterStream(f.stream)).Result(); l != 0 {
		t.Fatalf("dead letter stream still has %d entries", l)
	}

	entries, _ := f.rc.XRevRangeN(ctx, f.stream, "+", "-", 1).Result()
	for k := range entries[0].Values {
		if len(k) >= 4 && k[:4] == "dlq_" {
			t.Fatalf("replayed entry kept annotation %s", k)
		}
	}

	if _, err := c.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if f.effects(t) != 1 || f.pending(t) != 0 {
		t.Fatalf("effects=%d pending=%d after replay", f.effects(t), f.pending(t))
	}
}

func TestReplayDeadLettersOnEmptyStream(t *testing.T) {
	f := newFixture(t)
	n, err := ReplayDeadLetters(context.Background(), f.rc, f.stream, 10)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestHandlerRunsInsideTheMessagesTenant(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var seen string
	var role string
	c := f.consumer("c1", func(ctx context.Context, tx pgx.Tx, m Message) error {
		if m.TenantID != f.tenant {
			t.Errorf("message tenant %s, want %s", m.TenantID, f.tenant)
		}
		if err := tx.QueryRow(ctx, `SELECT current_setting('app.tenant_id'), current_user`).Scan(&seen, &role); err != nil {
			return err
		}
		return recordEffect(ctx, tx, m)
	})
	if err := c.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	f.publish(t, uuid.New())
	if _, err := c.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if seen != f.tenant.String() || role != db.TenantRole {
		t.Fatalf("handler ran with tenant %q as role %q; want %s as %s", seen, role, f.tenant, db.TenantRole)
	}
}
