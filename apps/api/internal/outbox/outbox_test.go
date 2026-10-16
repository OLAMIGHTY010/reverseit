package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

func TestValidate(t *testing.T) {
	good := Event{TenantID: uuid.New(), Stream: "transactions", Type: "transaction.ingested", AggregateID: "TXN-1", Payload: map[string]string{"a": "b"}}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Event){
		"empty stream":    func(e *Event) { e.Stream = "" },
		"upper stream":    func(e *Event) { e.Stream = "Transactions" },
		"space in stream": func(e *Event) { e.Stream = "a b" },
		"long stream":     func(e *Event) { e.Stream = string(make([]byte, 101)) },
		"no tenant":       func(e *Event) { e.TenantID = uuid.Nil },
		"no type":         func(e *Event) { e.Type = "" },
		"no aggregate":    func(e *Event) { e.AggregateID = "" },
	} {
		e := good
		mutate(&e)
		if err := e.Validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func testPool(t *testing.T) *db.Pool {
	t.Helper()
	p := dbtest.Pool(t)
	if _, err := p.Exec(context.Background(), `DELETE FROM outbox_events`); err != nil {
		t.Fatal(err)
	}
	return p
}

func countEvents(t *testing.T, p *db.Pool) int {
	t.Helper()
	var n int
	if err := p.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEnqueueCommitKeepsEvent(t *testing.T) {
	p := testPool(t)
	tid := dbtest.Tenant(t, p)
	ctx := context.Background()

	var id uuid.UUID
	err := p.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		id, err = Enqueue(ctx, tx, Event{
			TenantID: tid, Stream: "transactions", Type: "transaction.ingested", AggregateID: "TXN-1",
			Payload: map[string]any{"amount_minor": 1850000}, TraceID: "trace-1",
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	var typ, trace string
	var published *time.Time
	var payload string
	err = p.QueryRow(ctx, `SELECT event_type, trace_id, published_at, payload::text FROM outbox_events WHERE event_id = $1`, id).
		Scan(&typ, &trace, &published, &payload)
	if err != nil {
		t.Fatal(err)
	}
	if typ != "transaction.ingested" || trace != "trace-1" || published != nil || payload != `{"amount_minor": 1850000}` {
		t.Fatalf("unexpected row: %s %s %v %s", typ, trace, published, payload)
	}
}

func TestEnqueueRollbackLeavesNoEvent(t *testing.T) {
	p := testPool(t)
	tid := dbtest.Tenant(t, p)
	ctx := context.Background()
	boom := errors.New("state change failed")

	err := p.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := Enqueue(ctx, tx, Event{TenantID: tid, Stream: "s", Type: "t", AggregateID: "a", Payload: 1}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := countEvents(t, p); n != 0 {
		t.Fatalf("%d events after rollback, want 0", n)
	}
}

func TestEnqueueRejectsDuplicateEventIDAndBadPayload(t *testing.T) {
	p := testPool(t)
	tid := dbtest.Tenant(t, p)
	ctx := context.Background()
	id := uuid.New()

	err := p.WithTx(ctx, func(tx pgx.Tx) error {
		e := Event{TenantID: tid, ID: id, Stream: "s", Type: "t", AggregateID: "a", Payload: 1}
		if _, err := Enqueue(ctx, tx, e); err != nil {
			return err
		}
		_, err := Enqueue(ctx, tx, e)
		return err
	})
	if err == nil {
		t.Fatal("duplicate event id was accepted")
	}
	if n := countEvents(t, p); n != 0 {
		t.Fatalf("duplicate attempt left %d events", n)
	}

	err = p.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := Enqueue(ctx, tx, Event{TenantID: tid, Stream: "s", Type: "t", AggregateID: "a", Payload: make(chan int)})
		return err
	})
	if err == nil {
		t.Fatal("unmarshalable payload was accepted")
	}
	err = p.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := Enqueue(ctx, tx, Event{TenantID: tid, Stream: "s", Type: "t", AggregateID: "a", Payload: nil})
		return err
	})
	if err == nil {
		t.Fatal("nil payload was accepted")
	}
}

// Tenant code can add events for its own tenant, and see only those. It can
// neither forge an event for another tenant nor alter or delete any event.
func TestOutboxIsTenantIsolatedAndAppendOnlyForTenants(t *testing.T) {
	p := testPool(t)
	a, b := dbtest.Tenant(t, p), dbtest.Tenant(t, p)
	ctx := context.Background()

	add := func(tenant, as uuid.UUID) error {
		return p.WithTenant(ctx, as, func(tx pgx.Tx) error {
			_, err := Enqueue(ctx, tx, Event{TenantID: tenant, Stream: "s", Type: "t", AggregateID: "a", Payload: 1})
			return err
		})
	}
	if err := add(a, a); err != nil {
		t.Fatalf("tenant A could not add its own event: %v", err)
	}
	if err := add(b, b); err != nil {
		t.Fatal(err)
	}
	if err := add(b, a); err == nil {
		t.Fatal("tenant A forged an event for tenant B")
	}

	err := p.WithTenant(ctx, a, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM outbox_events`).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("tenant A sees %d events, want 1", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, stmt := range []string{
		`UPDATE outbox_events SET published_at = now()`,
		`DELETE FROM outbox_events`,
	} {
		err := p.WithTenant(ctx, a, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, stmt)
			return err
		})
		if err == nil {
			t.Errorf("tenant role was allowed to run %q", stmt)
		}
	}
}
