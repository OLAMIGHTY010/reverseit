package outbox

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

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

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func enqueue(t *testing.T, p *db.Pool, stream string, n int) {
	t.Helper()
	ctx := context.Background()
	tid := dbtest.Tenant(t, p)
	err := p.WithTx(ctx, func(tx pgx.Tx) error {
		for i := 1; i <= n; i++ {
			if _, err := Enqueue(ctx, tx, Event{
				TenantID: tid, Stream: stream, Type: "test.happened", AggregateID: "AGG-1",
				Payload: map[string]int{"n": i}, TraceID: "trace-x",
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func uniqueStream() string {
	return fmt.Sprintf("test.relay.%d", time.Now().UnixNano())
}

func TestPublishPendingPublishesInOrderAndMarksRows(t *testing.T) {
	p := testPool(t)
	rc := testRedis(t)
	ctx := context.Background()
	stream := uniqueStream()
	t.Cleanup(func() { rc.Del(ctx, stream) })

	enqueue(t, p, stream, 3)
	r := &Relay{Pool: p, Redis: rc, Logger: quiet()}

	n, err := r.PublishPending(ctx)
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}

	msgs, err := rc.XRange(ctx, stream, "-", "+").Result()
	if err != nil || len(msgs) != 3 {
		t.Fatalf("stream has %d messages, err=%v", len(msgs), err)
	}
	for i, m := range msgs {
		want := fmt.Sprintf(`{"n": %d}`, i+1)
		if m.Values["payload"] != want || m.Values["type"] != "test.happened" ||
			m.Values["aggregate_id"] != "AGG-1" || m.Values["trace_id"] != "trace-x" || m.Values["event_id"] == "" {
			t.Fatalf("message %d wrong: %+v", i, m.Values)
		}
	}

	var unpublished int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE published_at IS NULL`).Scan(&unpublished); err != nil || unpublished != 0 {
		t.Fatalf("unpublished=%d err=%v", unpublished, err)
	}

	n, err = r.PublishPending(ctx)
	if err != nil || n != 0 {
		t.Fatalf("second pass n=%d err=%v", n, err)
	}
	if l, _ := rc.XLen(ctx, stream).Result(); l != 3 {
		t.Fatalf("stream length %d after second pass, want 3", l)
	}
}

func TestPublishPendingKeepsRowsWhenRedisIsDown(t *testing.T) {
	p := testPool(t)
	good := testRedis(t)
	ctx := context.Background()
	stream := uniqueStream()
	t.Cleanup(func() { good.Del(ctx, stream) })

	enqueue(t, p, stream, 2)

	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, MaxRetries: -1})
	defer dead.Close()
	r := &Relay{Pool: p, Redis: dead, Logger: quiet()}

	n, err := r.PublishPending(ctx)
	if err == nil || n != 0 {
		t.Fatalf("expected error and zero published, got n=%d err=%v", n, err)
	}
	var unpublished, attempts int
	var lastErr string
	if err := p.QueryRow(ctx, `SELECT count(*) FILTER (WHERE published_at IS NULL), max(attempts), max(last_error) FROM outbox_events`).
		Scan(&unpublished, &attempts, &lastErr); err != nil {
		t.Fatal(err)
	}
	if unpublished != 2 || attempts != 1 || lastErr == "" {
		t.Fatalf("unpublished=%d attempts=%d last_error=%q", unpublished, attempts, lastErr)
	}

	// Redis comes back: nothing was lost.
	r.Redis = good
	n, err = r.PublishPending(ctx)
	if err != nil || n != 2 {
		t.Fatalf("recovery n=%d err=%v", n, err)
	}
	if l, _ := good.XLen(ctx, stream).Result(); l != 2 {
		t.Fatalf("stream length %d, want 2", l)
	}
}

func TestPublishPendingRespectsBatchSize(t *testing.T) {
	p := testPool(t)
	rc := testRedis(t)
	ctx := context.Background()
	stream := uniqueStream()
	t.Cleanup(func() { rc.Del(ctx, stream) })

	enqueue(t, p, stream, 5)
	r := &Relay{Pool: p, Redis: rc, Logger: quiet(), BatchSize: 2}

	for _, want := range []int{2, 2, 1, 0} {
		n, err := r.PublishPending(ctx)
		if err != nil || n != want {
			t.Fatalf("n=%d want %d err=%v", n, want, err)
		}
	}
}

func TestConcurrentRelaysDoNotDuplicate(t *testing.T) {
	p := testPool(t)
	rc := testRedis(t)
	ctx := context.Background()
	stream := uniqueStream()
	t.Cleanup(func() { rc.Del(ctx, stream) })

	enqueue(t, p, stream, 20)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := &Relay{Pool: p, Redis: rc, Logger: quiet(), BatchSize: 5}
			for j := 0; j < 6; j++ {
				if _, err := r.PublishPending(ctx); err != nil {
					t.Errorf("publish: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	if l, _ := rc.XLen(ctx, stream).Result(); l != 20 {
		t.Fatalf("stream length %d, want exactly 20", l)
	}
	msgs, _ := rc.XRange(ctx, stream, "-", "+").Result()
	for i, m := range msgs {
		if m.Values["payload"] != fmt.Sprintf(`{"n": %d}`, i%20+1) {
			t.Fatalf("order broken at %d: %v", i, m.Values["payload"])
		}
	}
}

func TestRequeueReplaysAfterStreamLoss(t *testing.T) {
	p := testPool(t)
	rc := testRedis(t)
	ctx := context.Background()
	stream := uniqueStream()
	t.Cleanup(func() { rc.Del(ctx, stream) })

	enqueue(t, p, stream, 3)
	r := &Relay{Pool: p, Redis: rc, Logger: quiet()}
	if n, err := r.PublishPending(ctx); err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}

	// Redis loses the stream.
	rc.Del(ctx, stream)

	count, err := Requeue(ctx, p, 0)
	if err != nil || count != 3 {
		t.Fatalf("requeue count=%d err=%v", count, err)
	}
	if n, err := r.PublishPending(ctx); err != nil || n != 3 {
		t.Fatalf("replay n=%d err=%v", n, err)
	}
	if l, _ := rc.XLen(ctx, stream).Result(); l != 3 {
		t.Fatalf("stream length %d after replay, want 3", l)
	}
}

func TestRunStopsOnCancelAndRejectsBadInterval(t *testing.T) {
	p := testPool(t)
	rc := testRedis(t)
	r := &Relay{Pool: p, Redis: rc, Logger: quiet()}

	if err := r.Run(context.Background(), 0); err == nil {
		t.Fatal("zero interval accepted")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx, 20*time.Millisecond) }()
	time.Sleep(100 * time.Millisecond)
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
