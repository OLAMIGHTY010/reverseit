package cases

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/events"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
)

type stored struct {
	Type, Aggregate string
	Payload         map[string]any
}

func (r *rig) outbox(t *testing.T, typ string) []stored {
	t.Helper()
	var out []stored
	err := r.tx(t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT event_type, aggregate_id, payload FROM outbox_events WHERE event_type = $1 ORDER BY id`, typ)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s stored
			var raw []byte
			if err := rows.Scan(&s.Type, &s.Aggregate, &raw); err != nil {
				return err
			}
			_ = json.Unmarshal(raw, &s.Payload)
			out = append(out, s)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOpeningACaseEmitsOneEventAndAnIdempotentReopenEmitsNone(t *testing.T) {
	r := newRig(t)
	txn := r.txn(t, "EV-OPEN-1")
	c, created := r.open(t, botCase(txn, CodeNoCredit))
	if !created {
		t.Fatal("expected a new case")
	}
	if _, again := r.open(t, botCase(txn, CodeNoCredit)); again {
		t.Fatal("expected the same case")
	}
	got := r.outbox(t, events.CaseOpened)
	if len(got) != 1 || got[0].Aggregate != c.ID.String() {
		t.Fatalf("expected exactly one case.opened for %s, got %+v", c.ID, got)
	}
	p := got[0].Payload
	if p["case_ref"] != c.Ref || p["classification"] != CodeNoCredit || p["status"] != StDetected || p["txn_id"] != txn.String() ||
		p["tenant_id"] != r.tenant.String() || p["schema_version"] != float64(events.SchemaVersion) {
		t.Fatalf("payload: %+v", p)
	}
	for _, k := range []string{"debit_account_token", "beneficiary_token", "complaint", "explanation"} {
		if _, ok := p[k]; ok {
			t.Errorf("payload must not carry %s", k)
		}
	}
}

func TestEveryTransitionEmitsOnceAndRejectedOrRolledBackMovesEmitNothing(t *testing.T) {
	r := newRig(t)
	c, _ := r.open(t, botCase(r.txn(t, "EV-TR-1"), CodeNoCredit))
	ctx := context.Background()

	move := func(to string) error {
		return r.tx(t, func(tx pgx.Tx) error {
			_, err := r.svc.Transition(ctx, tx, r.tenant, c.ID, r.user, to, 0, "TEST")
			return err
		})
	}
	if err := move(StUnderInvestigation); err != nil {
		t.Fatal(err)
	}
	if err := move(StResolved); !errors.Is(err, ErrBadMove) {
		t.Fatalf("expected a refused move, got %v", err)
	}
	// A move whose transaction rolls back publishes nothing.
	boom := errors.New("rolled back")
	err := r.tx(t, func(tx pgx.Tx) error {
		if _, err := r.svc.Transition(ctx, tx, r.tenant, c.ID, r.user, StAwaitingEvidence, 0, "TEST"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	// A stale version is refused and emits nothing.
	err = r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.Transition(ctx, tx, r.tenant, c.ID, r.user, StAwaitingEvidence, 1, "TEST")
		return err
	})
	if !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("expected a stale version, got %v", err)
	}

	got := r.outbox(t, events.CaseTransitioned)
	if len(got) != 1 {
		t.Fatalf("expected one case.transitioned, got %+v", got)
	}
	p := got[0].Payload
	if p["from_status"] != StDetected || p["to_status"] != StUnderInvestigation || p["version"] != float64(2) || p["case_id"] != c.ID.String() {
		t.Fatalf("payload: %+v", p)
	}
	// A second real transition is a second event with a different id.
	if err := move(StAwaitingEvidence); err != nil {
		t.Fatal(err)
	}
	if got = r.outbox(t, events.CaseTransitioned); len(got) != 2 {
		t.Fatalf("expected two events, got %d", len(got))
	}
	_ = uuid.Nil
}

func TestRecordingANotificationEmitsOneEventWithoutItsTextAndADuplicateEmitsNone(t *testing.T) {
	r := newRig(t)
	c, _ := r.open(t, botCase(r.txn(t, "EV-NOTE-1"), CodeNoCredit))
	ctx := context.Background()
	msg := notify.CustomerDetected(1850000, "NGN", "EV-NOTE-1")

	record := func(key string) (bool, error) {
		var created bool
		err := r.tx(t, func(tx pgx.Tx) error {
			var err error
			created, err = notify.Record(ctx, tx, r.tenant, c.ID, msg, key)
			return err
		})
		return created, err
	}
	if created, err := record("k1"); err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	if created, err := record("k1"); err != nil || created {
		t.Fatalf("a duplicate must record nothing: %v %v", created, err)
	}
	got := r.outbox(t, events.NotificationRecorded)
	if len(got) != 1 {
		t.Fatalf("expected one notification.recorded, got %d", len(got))
	}
	p := got[0].Payload
	if p["audience"] != notify.Customer || p["stage"] != notify.StageDetected || p["template_id"] != msg.Template || p["case_id"] != c.ID.String() || p["notification_id"] == nil {
		t.Fatalf("payload: %+v", p)
	}
	if _, ok := p["body"]; ok {
		t.Fatal("the message text must not be in the event")
	}

	// A rolled back recording publishes nothing.
	boom := errors.New("rolled back")
	err := r.tx(t, func(tx pgx.Tx) error {
		if _, err := notify.Record(ctx, tx, r.tenant, c.ID, msg, "k2"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || len(r.outbox(t, events.NotificationRecorded)) != 1 {
		t.Fatalf("rollback: %v", err)
	}
}
