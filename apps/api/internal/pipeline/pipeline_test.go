package pipeline

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/outbox"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type rig struct {
	pool   *db.Pool
	tenant uuid.UUID
	rc     *redis.Client
	t      *testing.T
}

func newRig(t *testing.T) *rig {
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), t: t}
	if addr := os.Getenv("TEST_REDIS_ADDR"); addr != "" {
		r.rc = redis.NewClient(&redis.Options{Addr: addr})
		t.Cleanup(func() { _ = r.rc.Close() })
	}
	return r
}

func (r *rig) enqueue(tenant uuid.UUID, stream string) {
	r.t.Helper()
	if err := r.pool.WithTenant(context.Background(), tenant, func(tx pgx.Tx) error {
		_, err := outbox.Enqueue(context.Background(), tx, outbox.Event{TenantID: tenant, Stream: stream, Type: "t.happened", AggregateID: "A", Payload: map[string]int{"n": 1}})
		return err
	}); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) collect(tenant uuid.UUID, rc *redis.Client, extra ...string) Report {
	r.t.Helper()
	var rep Report
	if err := r.pool.WithTenant(context.Background(), tenant, func(tx pgx.Tx) error {
		var err error
		rep, err = Collect(context.Background(), tx, tenant, rc, extra...)
		return err
	}); err != nil {
		r.t.Fatal(err)
	}
	return rep
}

func TestOutboxBacklogAgeAndFailuresAreCountedPerInstitution(t *testing.T) {
	r := newRig(t)
	other := dbtest.Tenant(t, r.pool)
	if rep := r.collect(r.tenant, nil); rep.Outbox.Unpublished != 0 || rep.Outbox.OldestAgeSecs != 0 || rep.RedisAvailable || len(rep.Streams) != 0 {
		t.Fatalf("%+v", rep)
	}
	r.enqueue(r.tenant, "pipeline.test.a")
	r.enqueue(r.tenant, "pipeline.test.a")
	r.enqueue(other, "pipeline.test.a")
	if _, err := r.pool.Exec(context.Background(), `UPDATE outbox_events SET created_at = now() - interval '90 seconds' WHERE tenant_id = $1`, r.tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := r.pool.Exec(context.Background(), `UPDATE outbox_events SET attempts = 3, last_error = 'redis down' WHERE id = (SELECT min(id) FROM outbox_events WHERE tenant_id = $1)`, r.tenant); err != nil {
		t.Fatal(err)
	}
	rep := r.collect(r.tenant, nil)
	if rep.Outbox.Unpublished != 2 || rep.Outbox.Failing != 1 || rep.Outbox.OldestAgeSecs < 89 || rep.Outbox.OldestAgeSecs > 120 {
		t.Fatalf("%+v", rep.Outbox)
	}
	if rep := r.collect(other, nil); rep.Outbox.Unpublished != 1 {
		t.Fatalf("another institution's backlog leaked: %+v", rep.Outbox)
	}
	if _, err := r.pool.Exec(context.Background(), `UPDATE outbox_events SET published_at = now() WHERE tenant_id = $1`, r.tenant); err != nil {
		t.Fatal(err)
	}
	if rep := r.collect(r.tenant, nil); rep.Outbox.Unpublished != 0 {
		t.Fatalf("published rows still counted: %+v", rep.Outbox)
	}
}

func TestStreamLengthPendingLagAndDeadLettersAreReported(t *testing.T) {
	r := newRig(t)
	if r.rc == nil {
		t.Skip("TEST_REDIS_ADDR not set; skipping Redis integration test")
	}
	ctx := context.Background()
	name := fmt.Sprintf("pipeline.test.%d", time.Now().UnixNano())
	t.Cleanup(func() { r.rc.Del(context.Background(), name, name+".dlq") })
	for i := 0; i < 3; i++ {
		if err := r.rc.XAdd(ctx, &redis.XAddArgs{Stream: name, Values: map[string]any{"n": i}}).Err(); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.rc.XGroupCreate(ctx, name, "consumers", "0").Err(); err != nil {
		t.Fatal(err)
	}
	// One entry delivered and not acknowledged: pending 1, lag 2.
	if err := r.rc.XReadGroup(ctx, &redis.XReadGroupArgs{Group: "consumers", Consumer: "c1", Streams: []string{name, ">"}, Count: 1}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := r.rc.XAdd(ctx, &redis.XAddArgs{Stream: name + ".dlq", Values: map[string]any{"x": 1}}).Err(); err != nil {
		t.Fatal(err)
	}
	rep := r.collect(r.tenant, r.rc, name, name+".nothere")
	if !rep.RedisAvailable {
		t.Fatal("redis reported unavailable")
	}
	var got *Stream
	var missing *Stream
	for i := range rep.Streams {
		switch rep.Streams[i].Name {
		case name:
			got = &rep.Streams[i]
		case name + ".nothere":
			missing = &rep.Streams[i]
		}
	}
	if got == nil || got.Length != 3 || got.DeadLetters != 1 || len(got.Groups) != 1 {
		t.Fatalf("%+v", rep.Streams)
	}
	g := got.Groups[0]
	if g.Name != "consumers" || g.Pending != 1 || g.Consumers != 1 || g.Lag == nil || *g.Lag != 2 {
		t.Fatalf("%+v", g)
	}
	if missing == nil || missing.Length != 0 || len(missing.Groups) != 0 {
		t.Fatalf("a stream that does not exist should read as empty: %+v", missing)
	}
}

func TestAnUnreachableRedisIsReportedAsUnavailableNotAsEmpty(t *testing.T) {
	r := newRig(t)
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = dead.Close() })
	rep := r.collect(r.tenant, dead)
	if rep.RedisAvailable || len(rep.Streams) != 0 {
		t.Fatalf("%+v", rep)
	}
}
