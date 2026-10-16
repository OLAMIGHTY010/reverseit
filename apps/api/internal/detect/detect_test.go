package detect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
	"github.com/smartreversal/smartreversal/apps/api/internal/outbox"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/stream"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

const (
	tokA = "acct_0123456789abcdef0123456789abcdef"
	tokB = "acct_fedcba9876543210fedcba9876543210"
)

type rig struct {
	pool   *db.Pool
	ing    *txn.Service
	det    *Detector
	cfg    *configsvc.Service
	tenant uuid.UUID
	now    time.Time
	n      int
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), now: time.Now().UTC().Truncate(time.Second)}
	app := &audit.Appender{Now: func() time.Time { return r.now }}
	r.cfg = &configsvc.Service{Audit: app, Now: func() time.Time { return r.now }}
	r.ing = &txn.Service{Now: func() time.Time { return r.now }}
	cs := &cases.Service{Audit: app, Config: r.cfg, Now: func() time.Time { return r.now }}
	r.det = &Detector{Cases: cs, Config: r.cfg, Now: func() time.Time { return r.now }}
	return r
}

func (r *rig) setConfig(t *testing.T, key string, value int) {
	t.Helper()
	raw, _ := json.Marshal(value)
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		v, err := r.cfg.Propose(context.Background(), tx, r.tenant, uuid.New(), configsvc.ProposeInput{Key: key, Value: raw,
			SourceReference: "SYNTHETIC DEMO POLICY - not approved for production", EffectiveFrom: r.now, VerifyStatus: "unverified", Reason: "synthetic policy for tests"})
		if err != nil {
			return err
		}
		_, err = r.cfg.Decide(context.Background(), tx, r.tenant, uuid.New(), key, v.Version, true, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (r *rig) event(t *testing.T, ref, typ string, at time.Time, mut ...func(*txn.Envelope)) txn.Result {
	t.Helper()
	r.n++
	e := txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: fmt.Sprintf("EV-%s-%d", ref, r.n), EventType: typ, SourceTS: at,
		Txn: txn.Core{InstructionRef: ref, Channel: "MOBILE", Rail: "NIP", AmountMinor: 1850000, Currency: "NGN", DebitAccountToken: tokA,
			BeneficiaryBank: "TEST BANK", BeneficiaryToken: tokB, SessionID: "S-" + ref}}
	for _, m := range mut {
		m(&e)
	}
	var res txn.Result
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		res, err = r.ing.Ingest(context.Background(), tx, r.tenant, e)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (r *rig) eval(t *testing.T, id uuid.UUID) []Finding {
	t.Helper()
	var out []Finding
	if err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		out, err = r.det.Evaluate(context.Background(), tx, r.tenant, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *rig) allCases(t *testing.T) []cases.Case {
	t.Helper()
	var out []cases.Case
	if err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		out, err = r.det.Cases.List(context.Background(), tx, r.tenant, cases.Filter{}, nil, 100)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func codes(cs []cases.Case) map[string]int {
	m := map[string]int{}
	for _, c := range cs {
		m[c.Classification]++
	}
	return m
}

func (r *rig) failedTransfer(t *testing.T, ref string, at time.Time, mut ...func(*txn.Envelope)) uuid.UUID {
	t.Helper()
	r.event(t, ref, txn.EvDebitPosted, at, mut...)
	r.event(t, ref, txn.EvTransferSent, at.Add(time.Second), mut...)
	return r.event(t, ref, txn.EvCreditFailed, at.Add(2*time.Second), mut...).TxnID
}

func TestAConfirmedFailureOpensOneCaseWithEvidenceAndNotifications(t *testing.T) {
	r := newRig(t)
	id := r.failedTransfer(t, "INS-F1", r.now.Add(-10*time.Minute))
	got := r.eval(t, id)
	if len(got) != 1 || !got[0].Created || got[0].Case.Classification != cases.CodeNoCredit || got[0].Case.Source != cases.SourceBot ||
		got[0].Case.RuleID != RuleFailure || got[0].Case.RootCause != "beneficiary_side_failure" {
		t.Fatalf("%+v", got)
	}
	again := r.eval(t, id)
	if len(again) != 1 || again[0].Created || again[0].Case.ID != got[0].Case.ID {
		t.Fatalf("a second look made another case: %+v", again)
	}

	var ev []cases.Evidence
	var customer, ops []notify.Stored
	_ = r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		ev, _ = r.det.Cases.Evidence(context.Background(), tx, r.tenant, got[0].Case.ID)
		customer, _ = notify.ForCase(context.Background(), tx, r.tenant, got[0].Case.ID, notify.Customer)
		ops, _ = notify.ForCase(context.Background(), tx, r.tenant, got[0].Case.ID, notify.Operations)
		return nil
	})
	if len(ev) != 3 || ev[0].Kind != "transaction_event" || ev[0].Hash == "" {
		t.Fatalf("evidence: %+v", ev)
	}
	if len(customer) != 1 || customer[0].Stage != notify.StageDetected || len(ops) != 1 {
		t.Fatalf("notifications: %+v %+v", customer, ops)
	}
}

func TestSuccessAndPendingTransactionsOpenNothing(t *testing.T) {
	r := newRig(t)
	ok := r.event(t, "INS-OK", txn.EvDebitPosted, r.now.Add(-time.Minute)).TxnID
	r.event(t, "INS-OK", txn.EvTransferSent, r.now.Add(-50*time.Second))
	r.event(t, "INS-OK", txn.EvCreditConfirmed, r.now.Add(-40*time.Second))
	pending := r.event(t, "INS-PEND", txn.EvDebitPosted, r.now.Add(-time.Minute)).TxnID
	r.event(t, "INS-PEND", txn.EvTransferSent, r.now.Add(-50*time.Second))
	if f := r.eval(t, ok); len(f) != 0 {
		t.Fatalf("success: %+v", f)
	}
	if f := r.eval(t, pending); len(f) != 0 {
		t.Fatalf("pending is not an exception: %+v", f)
	}
	if n := len(r.allCases(t)); n != 0 {
		t.Fatalf("%d cases", n)
	}
}

func TestATimeoutIsUnknownNotFailureAndALateSuccessClosesTheCase(t *testing.T) {
	r := newRig(t)
	r.event(t, "INS-U1", txn.EvDebitPosted, r.now.Add(-10*time.Minute))
	r.event(t, "INS-U1", txn.EvTransferSent, r.now.Add(-9*time.Minute))
	id := r.event(t, "INS-U1", txn.EvTransferTimeout, r.now.Add(-8*time.Minute)).TxnID
	f := r.eval(t, id)
	if len(f) != 1 || f[0].Case.Classification != cases.CodeUnknown || f[0].Case.RootCause != "switch_response_failure" {
		t.Fatalf("%+v", f)
	}
	if c := codes(r.allCases(t)); c[cases.CodeNoCredit] != 0 {
		t.Fatalf("a timeout was treated as a failure: %v", c)
	}

	r.event(t, "INS-U1", txn.EvCreditConfirmed, r.now.Add(-time.Minute))
	r.eval(t, id)
	cs := r.allCases(t)
	if len(cs) != 1 || cs[0].Status != cases.StClosedNoException || cs[0].ClosedReason != "LATE_SUCCESS_CONFIRMED" {
		t.Fatalf("late success: %+v", cs)
	}
}

func TestConflictingEvidenceIsKeptForAPerson(t *testing.T) {
	r := newRig(t)
	r.event(t, "INS-C1", txn.EvDebitPosted, r.now.Add(-5*time.Minute))
	r.event(t, "INS-C1", txn.EvCreditConfirmed, r.now.Add(-4*time.Minute))
	id := r.event(t, "INS-C1", txn.EvCreditFailed, r.now.Add(-3*time.Minute)).TxnID
	f := r.eval(t, id)
	if len(f) != 1 || f[0].Case.Classification != cases.CodeUnknown || f[0].Case.RootCause != "conflicting_evidence" {
		t.Fatalf("%+v", f)
	}
}

func TestStuckTransactionsNeedAnApprovedThreshold(t *testing.T) {
	r := newRig(t)
	r.event(t, "INS-S1", txn.EvDebitPosted, r.now.Add(-20*time.Minute))
	r.event(t, "INS-S1", txn.EvTransferSent, r.now.Add(-19*time.Minute))
	sweep := func() []Finding {
		var out []Finding
		_ = r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			var err error
			out, err = r.det.SweepStuck(context.Background(), tx, r.tenant)
			if err != nil {
				t.Fatal(err)
			}
			return nil
		})
		return out
	}
	if f := sweep(); len(f) != 0 {
		t.Fatalf("with no approved threshold nothing may be assumed: %+v", f)
	}
	r.setConfig(t, "waiting_threshold_seconds.NIP", 3600)
	if f := sweep(); len(f) != 0 {
		t.Fatalf("19 minutes is under a 60 minute threshold: %+v", f)
	}
	r.now = r.now.Add(2 * time.Hour)
	r.setConfig(t, "waiting_threshold_seconds.NIP", 600)
	f := sweep()
	if len(f) != 1 || f[0].Case.Classification != cases.CodeUnknown || f[0].Case.RuleID != RuleStuck {
		t.Fatalf("%+v", f)
	}
	if again := sweep(); len(again) != 1 || again[0].Created {
		t.Fatalf("the sweep made a second case: %+v", again)
	}
}

func debit(r *rig, t *testing.T, ref string, at time.Time, session string, mut ...func(*txn.Envelope)) uuid.UUID {
	t.Helper()
	m := append([]func(*txn.Envelope){func(e *txn.Envelope) { e.Txn.SessionID = session }}, mut...)
	return r.event(t, ref, txn.EvDebitPosted, at, m...).TxnID
}

func TestDuplicateDebitsAreFlaggedOnTheLaterOneAndGradedBySession(t *testing.T) {
	r := newRig(t)
	r.setConfig(t, "duplicate.window_seconds", 30)
	first := debit(r, t, "INS-D1", r.now.Add(-5*time.Minute), "SESSION-X")
	second := debit(r, t, "INS-D2", r.now.Add(-5*time.Minute).Add(3*time.Second), "SESSION-X")

	// Looking at the earlier one first or the later one first gives the same case.
	f1 := r.eval(t, first)
	if len(f1) != 1 || !f1[0].Created || f1[0].Case.Classification != cases.CodeDuplicate || f1[0].Case.RiskGrade != "HIGH" || f1[0].Case.Priority != "HIGH" {
		t.Fatalf("%+v", f1)
	}
	f2 := r.eval(t, second)
	if len(f2) != 1 || f2[0].Created || f2[0].Case.ID != f1[0].Case.ID {
		t.Fatalf("seen from the other side: %+v", f2)
	}
	if f1[0].Case.TxnID == nil || *f1[0].Case.TxnID != second {
		t.Fatalf("the case belongs to the later debit: %+v", f1[0].Case)
	}
	var ev []cases.Evidence
	_ = r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		ev, _ = r.det.Cases.Evidence(context.Background(), tx, r.tenant, f1[0].Case.ID)
		return nil
	})
	related := 0
	for _, e := range ev {
		if e.Kind == "related_transaction" && e.Ref == "INS-D1" {
			related++
		}
	}
	if related != 1 {
		t.Fatalf("the earlier debit is not attached as evidence: %+v", ev)
	}
}

func TestDifferentSessionsAreMediumRisk(t *testing.T) {
	r := newRig(t)
	r.setConfig(t, "duplicate.window_seconds", 30)
	debit(r, t, "INS-M1", r.now.Add(-5*time.Minute), "SESSION-A")
	second := debit(r, t, "INS-M2", r.now.Add(-5*time.Minute).Add(5*time.Second), "SESSION-B")
	f := r.eval(t, second)
	if len(f) != 1 || f[0].Case.RiskGrade != "MEDIUM" {
		t.Fatalf("%+v", f)
	}
}

func TestThingsThatAreNotDuplicates(t *testing.T) {
	cases := map[string]func(r *rig, t *testing.T) uuid.UUID{
		"outside the window": func(r *rig, t *testing.T) uuid.UUID {
			debit(r, t, "INS-N1", r.now.Add(-10*time.Minute), "S1")
			return debit(r, t, "INS-N2", r.now.Add(-10*time.Minute).Add(5*time.Minute), "S2")
		},
		"a different amount": func(r *rig, t *testing.T) uuid.UUID {
			debit(r, t, "INS-N3", r.now.Add(-10*time.Minute), "S1")
			return debit(r, t, "INS-N4", r.now.Add(-10*time.Minute).Add(2*time.Second), "S2", func(e *txn.Envelope) { e.Txn.AmountMinor = 1850001 })
		},
		"a different beneficiary": func(r *rig, t *testing.T) uuid.UUID {
			debit(r, t, "INS-N5", r.now.Add(-10*time.Minute), "S1")
			return debit(r, t, "INS-N6", r.now.Add(-10*time.Minute).Add(2*time.Second), "S2", func(e *txn.Envelope) { e.Txn.BeneficiaryToken = "acct_00000000000000000000000000000001" })
		},
		"a different customer": func(r *rig, t *testing.T) uuid.UUID {
			debit(r, t, "INS-N7", r.now.Add(-10*time.Minute), "S1")
			return debit(r, t, "INS-N8", r.now.Add(-10*time.Minute).Add(2*time.Second), "S2", func(e *txn.Envelope) { e.Txn.DebitAccountToken = "acct_00000000000000000000000000000002" })
		},
		"a retry after a confirmed failure": func(r *rig, t *testing.T) uuid.UUID {
			r.failedTransfer(t, "INS-N9", r.now.Add(-10*time.Minute), func(e *txn.Envelope) { e.Txn.SessionID = "S1" })
			return debit(r, t, "INS-N10", r.now.Add(-10*time.Minute).Add(10*time.Second), "S2")
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.setConfig(t, "duplicate.window_seconds", 30)
			id := setup(r, t)
			for _, f := range r.eval(t, id) {
				if f.Case.Classification == "DUP001" {
					t.Fatalf("flagged as a duplicate: %+v", f.Case)
				}
			}
		})
	}
}

func TestNoApprovedWindowMeansNoDuplicateRule(t *testing.T) {
	r := newRig(t)
	debit(r, t, "INS-W1", r.now.Add(-5*time.Minute), "SESSION-X")
	second := debit(r, t, "INS-W2", r.now.Add(-5*time.Minute).Add(time.Second), "SESSION-X")
	if f := r.eval(t, second); len(f) != 0 {
		t.Fatalf("a duplicate rule ran without an approved window: %+v", f)
	}
}

func TestDetectionIsTenantIsolated(t *testing.T) {
	a, b := newRig(t), newRig(t)
	a.setConfig(t, "duplicate.window_seconds", 30)
	b.setConfig(t, "duplicate.window_seconds", 30)
	debit(a, t, "INS-T1", a.now.Add(-5*time.Minute), "S1")
	idB := debit(b, t, "INS-T2", b.now.Add(-5*time.Minute).Add(time.Second), "S1")
	if f := b.eval(t, idB); len(f) != 0 {
		t.Fatalf("tenant B's debit matched tenant A's: %+v", f)
	}
	if got := b.eval(t, uuid.New()); got != nil {
		t.Fatalf("an unknown id produced findings: %+v", got)
	}
}

// ---- through Redis Streams ---------------------------------------------------

func TestTheDetectorRunsOffTheOutboxAndRedisStreams(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set; skipping Redis integration test")
	}
	r := newRig(t)
	ctx := context.Background()
	rc := redis.NewClient(&redis.Options{Addr: addr})
	defer rc.Close()
	streamName := fmt.Sprintf("test.txn.%d", time.Now().UnixNano())
	defer rc.Del(ctx, streamName, stream.DeadLetterStream(streamName))

	// The transactions are ingested normally, which writes outbox rows on the "transactions" stream.
	if _, err := r.pool.Exec(ctx, `DELETE FROM outbox_events WHERE stream = $1`, txn.Stream); err != nil {
		t.Fatal(err)
	}
	id := r.failedTransfer(t, "INS-R1", r.now.Add(-10*time.Minute))

	// The relay publishes them. A test-local stream name keeps tests apart.
	if _, err := r.pool.Exec(ctx, `UPDATE outbox_events SET stream = $2 WHERE tenant_id = $1 AND stream = $3`, r.tenant, streamName, txn.Stream); err != nil {
		t.Fatal(err)
	}
	relay := &outbox.Relay{Pool: r.pool, Redis: rc, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if n, err := relay.PublishPending(ctx); err != nil || n < 3 {
		t.Fatalf("relay published %d: %v", n, err)
	}

	c := &stream.Consumer{Pool: r.pool, Redis: rc, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Stream: streamName, Group: "detector-test", Name: "c1", Handler: r.det.Handle, Block: 50 * time.Millisecond, ClaimIdle: time.Hour}
	if err := c.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := c.RunOnce(ctx); err != nil || n < 3 {
		t.Fatalf("consumer handled %d: %v", n, err)
	}
	cs := r.allCases(t)
	if len(cs) != 1 || cs[0].Classification != cases.CodeNoCredit || *cs[0].TxnID != id {
		t.Fatalf("cases after the pipeline ran: %+v", cs)
	}
}

func TestAtmHardwareFailureOpensATM001Case(t *testing.T) {
	r := newRig(t)
	id := r.failedTransfer(t, "ATM-001", r.now.Add(-10*time.Minute), func(e *txn.Envelope) {
		e.Txn.Rail = "ATM"
		e.Txn.Channel = "ATM"
	})
	got := r.eval(t, id)
	if len(got) != 1 || !got[0].Created || got[0].Case.Classification != cases.CodeAtmFault ||
		got[0].Case.RuleID != RuleATM || got[0].Case.RootCause != "atm_hardware_fault" ||
		got[0].Case.Priority != "HIGH" {
		t.Fatalf("expected ATM001 case, got: %+v", got)
	}
}

func TestDisputedFeeOpensFEE001Case(t *testing.T) {
	r := newRig(t)
	id := r.failedTransfer(t, "FEE-001", r.now.Add(-10*time.Minute), func(e *txn.Envelope) {
		e.Txn.Rail = "FEE"
	})
	got := r.eval(t, id)
	if len(got) != 1 || !got[0].Created || got[0].Case.Classification != cases.CodeDisputedFee ||
		got[0].Case.RuleID != RuleFee || got[0].Case.RootCause != "unauthorized_fee_surcharge" ||
		got[0].Case.Priority != "LOW" {
		t.Fatalf("expected FEE001 case, got: %+v", got)
	}
}

func TestVendorAggregatorDropOpensVND001Case(t *testing.T) {
	r := newRig(t)
	id := r.failedTransfer(t, "VND-001", r.now.Add(-10*time.Minute), func(e *txn.Envelope) {
		e.Txn.Channel = "GATEWAY"
		e.Txn.Rail = "MERCHANT"
	})
	got := r.eval(t, id)
	if len(got) != 1 || !got[0].Created || got[0].Case.Classification != cases.CodeVendorDrop ||
		got[0].Case.RuleID != RuleVendor || got[0].Case.RootCause != "vendor_aggregator_drop" ||
		got[0].Case.Priority != "MEDIUM" {
		t.Fatalf("expected VND001 case, got: %+v", got)
	}
}

func TestIntermediaryRoutingDropOpensINC001Case(t *testing.T) {
	r := newRig(t)
	id := r.failedTransfer(t, "INC-001", r.now.Add(-10*time.Minute), func(e *txn.Envelope) {
		e.Txn.Rail = "INTERCHANGE"
	})
	got := r.eval(t, id)
	if len(got) != 1 || !got[0].Created || got[0].Case.Classification != cases.CodeIncomplete ||
		got[0].Case.RuleID != RuleIncomplete || got[0].Case.RootCause != "multi_leg_routing_drop" ||
		got[0].Case.Priority != "MEDIUM" {
		t.Fatalf("expected INC001 case, got: %+v", got)
	}
}

func TestAmountMismatchOpensMIS001Case(t *testing.T) {
	r := newRig(t)
	r.event(t, "MIS-001", txn.EvDebitPosted, r.now.Add(-5*time.Minute), func(e *txn.Envelope) {
		e.Txn.Rail = "MISMATCH"
	})
	r.event(t, "MIS-001", txn.EvCreditConfirmed, r.now.Add(-4*time.Minute), func(e *txn.Envelope) {
		e.Txn.Rail = "MISMATCH"
	})
	id := r.event(t, "MIS-001", txn.EvCreditFailed, r.now.Add(-3*time.Minute), func(e *txn.Envelope) {
		e.Txn.Rail = "MISMATCH"
	}).TxnID
	got := r.eval(t, id)
	if len(got) != 1 || !got[0].Created || got[0].Case.Classification != cases.CodeMismatch ||
		got[0].Case.RuleID != RuleMismatch || got[0].Case.RootCause != "amount_currency_mismatch" ||
		got[0].Case.Priority != "HIGH" {
		t.Fatalf("expected MIS001 case, got: %+v", got)
	}
}
