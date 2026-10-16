package inbound

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/correlate"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/stream"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

const (
	tokA = "acct_0123456789abcdef0123456789abcdef"
	tokB = "acct_fedcba9876543210fedcba9876543210"
)

var t0 = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

type rig struct {
	pool   *db.Pool
	rc     *redis.Client
	tenant uuid.UUID
	h      *Handler
	c      *stream.Consumer
	name   string
	t      *testing.T
}

func newRig(t *testing.T, withCorrelate bool) *rig {
	t.Helper()
	pool := dbtest.Pool(t)
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set; skipping Redis integration test")
	}
	rc := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rc.Close() })
	svc := &txn.Service{Now: func() time.Time { return t0.Add(time.Hour) }}
	r := &rig{pool: pool, rc: rc, tenant: dbtest.Tenant(t, pool), t: t, name: fmt.Sprintf("test.inbound.%d", time.Now().UnixNano())}
	r.h = &Handler{Txn: svc}
	if withCorrelate {
		r.h.Correlate = &correlate.Matcher{Txn: svc}
	}
	r.c = &stream.Consumer{Pool: pool, Redis: rc, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Stream: r.name, Group: Group, Name: "t1",
		Handler: r.h.Handle, Block: 50 * time.Millisecond, ClaimIdle: 100 * time.Millisecond, MaxDeliveries: 3}
	if err := r.c.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rc.Del(context.Background(), r.name, stream.DeadLetterStream(r.name)) })
	return r
}

func (r *rig) send(typ string, payload any) {
	r.t.Helper()
	e, err := Entry(r.tenant, typ, payload)
	if err != nil {
		r.t.Fatal(err)
	}
	r.xadd(e)
}

func (r *rig) sendRaw(typ, payload string) {
	r.t.Helper()
	r.xadd(map[string]any{"tenant_id": r.tenant.String(), "event_id": uuid.NewString(), "type": typ, "payload": payload})
}

func (r *rig) xadd(v map[string]any) {
	r.t.Helper()
	if err := r.rc.XAdd(context.Background(), &redis.XAddArgs{Stream: r.name, Values: v}).Err(); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) drain() {
	r.t.Helper()
	for i := 0; i < 4; i++ {
		if _, err := r.c.RunOnce(context.Background()); err != nil {
			r.t.Fatal(err)
		}
	}
}

func (r *rig) count(q string) (n int) {
	r.t.Helper()
	if err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), q).Scan(&n)
	}); err != nil {
		r.t.Fatal(err)
	}
	return
}

func (r *rig) pending() int64 {
	p, err := r.rc.XPending(context.Background(), r.name, Group).Result()
	if err != nil {
		r.t.Fatal(err)
	}
	return p.Count
}

func (r *rig) dlq() int64 {
	return r.rc.XLen(context.Background(), stream.DeadLetterStream(r.name)).Val()
}

func event(instr, evRef, typ string, at time.Time) txn.Envelope {
	return txn.Envelope{SourceSystem: "SWITCH_Q", SourceEventRef: evRef, EventType: typ, SourceTS: at,
		Txn:  txn.Core{InstructionRef: instr, Channel: "MOBILE", Rail: "NIP", AmountMinor: 1850000, Currency: "NGN", DebitAccountToken: tokA, BeneficiaryToken: tokB},
		Refs: map[string]string{"GATEWAY_REF": "GW-" + instr}}
}

func TestAnEventWrittenToTheStreamIsIngestedOnceHoweverOftenItIsSent(t *testing.T) {
	r := newRig(t, false)
	e := event("INS-Q100", "EV-Q100", txn.EvDebitPosted, t0)
	r.send(TypeEvent, e)
	r.send(TypeEvent, e) // a second stream entry carrying the same event
	r.drain()
	if n := r.count(`SELECT count(*) FROM transaction_events`); n != 1 {
		t.Fatalf("%d events stored", n)
	}
	if r.pending() != 0 || r.dlq() != 0 || r.count(`SELECT count(*) FROM quarantined_events`) != 0 {
		t.Fatal("a repeat should be a quiet duplicate")
	}
}

func TestEventsThatCanNeverSucceedAreQuarantinedWholeAndAcknowledgedNotRetried(t *testing.T) {
	r := newRig(t, false)
	good := event("INS-Q101", "EV-Q101", txn.EvDebitPosted, t0)
	r.send(TypeEvent, good)

	bad := event("INS-Q102", "EV-Q102", txn.EvDebitPosted, t0)
	bad.Txn.Channel = "FAX" // invalid
	r.send(TypeEvent, bad)

	mismatch := event("INS-Q101", "EV-Q103", txn.EvTransferSent, t0.Add(time.Second))
	mismatch.Txn.AmountMinor = 99 // contradicts the transaction
	r.send(TypeEvent, mismatch)

	reused := event("INS-Q101", "EV-Q101", txn.EvDebitPosted, t0)
	reused.Txn.BeneficiaryBank = "OTHER BANK" // same reference, different content
	r.send(TypeEvent, reused)

	r.sendRaw(TypeEvent, `{"source_system": "SWITCH_Q", "nonsense": true}`)
	r.sendRaw("transaction.mystery", `{}`)
	r.drain()

	if r.pending() != 0 || r.dlq() != 0 {
		t.Fatalf("pending %d, dead-lettered %d: refusals must be quarantined and acknowledged", r.pending(), r.dlq())
	}
	if n := r.count(`SELECT count(*) FROM transaction_events`); n != 1 {
		t.Fatalf("%d events stored; only the good one belongs", n)
	}
	if n := r.count(`SELECT count(*) FROM quarantined_events`); n != 5 {
		t.Fatalf("%d quarantined, want 5", n)
	}
	if n := r.count(`SELECT count(*) FROM quarantined_events WHERE payload IS NOT NULL`); n != 3 {
		t.Fatalf("%d kept whole for replay, want the 3 that decoded", n)
	}
	var amount int64
	_ = r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT amount_minor FROM transactions WHERE instruction_ref = 'INS-Q101'`).Scan(&amount)
	})
	if amount != 1850000 {
		t.Fatalf("a contradicting message changed the transaction to %d", amount)
	}
	// A refused invalid transaction must not leave a half-created transaction behind.
	if n := r.count(`SELECT count(*) FROM transactions WHERE instruction_ref = 'INS-Q102'`); n != 0 {
		t.Fatal("a refused event left a transaction")
	}
}

func TestAQuarantinedInboundEventCanBeReplayedOnceTheCauseIsFixed(t *testing.T) {
	r := newRig(t, false)
	future := event("INS-Q110", "EV-Q110", txn.EvDebitPosted, t0.Add(2*time.Hour)) // the source's clock is ahead
	r.send(TypeEvent, future)
	r.drain()
	if r.count(`SELECT count(*) FROM quarantined_events WHERE status = 'open'`) != 1 {
		t.Fatal("not quarantined")
	}
	var id uuid.UUID
	_ = r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id FROM quarantined_events`).Scan(&id)
	})
	r.h.Txn.Now = func() time.Time { return t0.Add(2*time.Hour + 10*time.Minute) }
	var res txn.ReplayResult
	if err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		res, err = r.h.Txn.Replay(context.Background(), tx, r.tenant, id)
		return err
	}); err != nil || !res.Replayed {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestObservationsAreCorrelatedOrQueuedAndBadOnesQuarantined(t *testing.T) {
	r := newRig(t, true)
	r.send(TypeEvent, event("INS-Q120", "EV-Q120", txn.EvDebitPosted, t0))
	good := correlate.Observation{SourceSystem: "GATEWAY_Q", SourceEventRef: "OB-Q120", EventType: txn.EvCreditConfirmed, SourceTS: t0.Add(2 * time.Second),
		Refs: map[string]string{"GATEWAY_REF": "GW-INS-Q120"}, AmountMinor: 1850000, Currency: "NGN"}
	r.send(TypeObservation, good)
	weak := correlate.Observation{SourceSystem: "GATEWAY_Q", SourceEventRef: "OB-Q121", EventType: txn.EvCreditConfirmed, SourceTS: t0.Add(3 * time.Second),
		Refs: map[string]string{"RRN": "NOPE"}, AmountMinor: 1850000, Currency: "NGN"}
	r.send(TypeObservation, weak)
	invalid := good
	invalid.SourceEventRef, invalid.Refs = "OB-Q122", nil
	r.send(TypeObservation, invalid)
	r.drain()
	if r.pending() != 0 || r.dlq() != 0 {
		t.Fatalf("pending %d dlq %d", r.pending(), r.dlq())
	}
	if n := r.count(`SELECT event_count FROM transactions WHERE instruction_ref = 'INS-Q120'`); n != 2 {
		t.Fatalf("the confident observation was not attached: %d events", n)
	}
	if n := r.count(`SELECT count(*) FROM evidence_correlations WHERE status = 'open'`); n != 1 {
		t.Fatalf("%d queued", n)
	}
	if n := r.count(`SELECT count(*) FROM quarantined_events`); n != 1 {
		t.Fatalf("%d quarantined", n)
	}
}

func TestAnInfrastructureFailureIsRetriedNotQuarantined(t *testing.T) {
	r := newRig(t, false) // observations cannot be handled here: that is a configuration fault, not bad data
	r.send(TypeObservation, correlate.Observation{SourceSystem: "G", SourceEventRef: "OB-Q130"})
	r.drain()
	if r.pending() == 0 && r.dlq() == 0 {
		t.Fatal("the message vanished")
	}
	if r.count(`SELECT count(*) FROM quarantined_events`) != 0 {
		t.Fatal("a fault in our own setup was blamed on the source's data")
	}
}
