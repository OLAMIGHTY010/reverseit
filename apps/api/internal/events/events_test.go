package events

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

func TestEventDefinitionsCarryNoSensitiveFieldNames(t *testing.T) {
	banned := []string{"account", "password", "secret", "token", "email", "phone", "name", "narration", "note", "body", "otp", "key"}
	for _, typ := range Types() {
		for _, f := range Fields(typ) {
			for _, b := range banned {
				if strings.Contains(f, b) && f != "template_id" {
					t.Errorf("%s may carry %q, which looks sensitive", typ, f)
				}
			}
		}
	}
	if len(Types()) != 10 {
		t.Fatalf("expected the ten approved event types, got %v", Types())
	}
}

func TestEmitRefusesUnknownTypesUnlistedFieldsAndNestedValues(t *testing.T) {
	pool := dbtest.Pool(t)
	tenant := dbtest.Tenant(t, pool)
	cases := map[string]Event{
		"unknown type":   {Type: "case.deleted", AggregateID: "a", Key: "k", Fields: map[string]any{}},
		"unlisted field": {Type: CaseOpened, AggregateID: "a", Key: "k", Fields: map[string]any{"debit_account_token": "acct_x"}},
		"nested value":   {Type: CaseOpened, AggregateID: "a", Key: "k", Fields: map[string]any{"case_ref": map[string]any{"x": 1}}},
		"long value":     {Type: CaseOpened, AggregateID: "a", Key: "k", Fields: map[string]any{"case_ref": strings.Repeat("x", 201)}},
		"no key":         {Type: CaseOpened, AggregateID: "a", Fields: map[string]any{}},
	}
	for name, e := range cases {
		err := pool.WithTenant(context.Background(), tenant, func(tx pgx.Tx) error {
			_, err := Emit(context.Background(), tx, tenant, e)
			return err
		})
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func count(t *testing.T, pool interface {
	WithTenant(context.Context, uuid.UUID, func(pgx.Tx) error) error
}, tenant uuid.UUID, typ string) int {
	t.Helper()
	var n int
	err := pool.WithTenant(context.Background(), tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events WHERE event_type = $1`, typ).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEmitStoresOncePerOccurrenceWithAStableIDAndVersionedPayload(t *testing.T) {
	pool := dbtest.Pool(t)
	tenant := dbtest.Tenant(t, pool)
	ctx := context.Background()
	e := Event{Type: CaseTransitioned, AggregateID: "c1", Key: "c1:2", Fields: map[string]any{"case_id": "c1", "from_status": "DETECTED", "to_status": "UNDER_INVESTIGATION", "version": 2}}
	var first, second bool
	err := pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		if first, err = Emit(ctx, tx, tenant, e); err != nil {
			return err
		}
		second, err = Emit(ctx, tx, tenant, e)
		return err
	})
	if err != nil || !first || second {
		t.Fatalf("first=%v second=%v err=%v", first, second, err)
	}
	if n := count(t, pool, tenant, CaseTransitioned); n != 1 {
		t.Fatalf("expected one stored event, got %d", n)
	}
	var id uuid.UUID
	var stream string
	var raw []byte
	err = pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT event_id, stream, payload FROM outbox_events WHERE event_type = $1`, CaseTransitioned).Scan(&id, &stream, &raw)
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != ID(tenant, CaseTransitioned, "c1:2") || stream != StreamCases {
		t.Fatalf("id=%s stream=%s", id, stream)
	}
	var p map[string]any
	_ = json.Unmarshal(raw, &p)
	if p["schema_version"] != float64(SchemaVersion) || p["tenant_id"] != tenant.String() || p["to_status"] != "UNDER_INVESTIGATION" || p["occurred_at"] == nil {
		t.Fatalf("payload: %s", raw)
	}
	// A different occurrence is a different event.
	e.Key = "c1:3"
	_ = pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error { _, err := Emit(ctx, tx, tenant, e); return err })
	if n := count(t, pool, tenant, CaseTransitioned); n != 2 {
		t.Fatalf("expected two events, got %d", n)
	}
}

func TestARolledBackChangePublishesNothingAndEventsStayWithTheirTenant(t *testing.T) {
	pool := dbtest.Pool(t)
	a, b := dbtest.Tenant(t, pool), dbtest.Tenant(t, pool)
	ctx := context.Background()
	boom := errors.New("rolled back")
	err := pool.WithTenant(ctx, a, func(tx pgx.Tx) error {
		if _, err := Emit(ctx, tx, a, Event{Type: CaseOpened, AggregateID: "x", Key: "x", Fields: map[string]any{"case_id": "x"}}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || count(t, pool, a, CaseOpened) != 0 {
		t.Fatalf("a rolled back transaction must not leave an event: %v", err)
	}
	if err := pool.WithTenant(ctx, a, func(tx pgx.Tx) error {
		_, err := Emit(ctx, tx, a, Event{Type: CaseOpened, AggregateID: "y", Key: "y", Fields: map[string]any{"case_id": "y"}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, a, CaseOpened) != 1 || count(t, pool, b, CaseOpened) != 0 {
		t.Fatal("events must be visible to their own tenant only")
	}
	// The same key in another tenant is a different event.
	if ID(a, CaseOpened, "y") == ID(b, CaseOpened, "y") {
		t.Fatal("event ids must differ between tenants")
	}
}
