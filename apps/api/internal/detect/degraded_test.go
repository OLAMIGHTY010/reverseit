package detect

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/sources"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

func (r *rig) expect(t *testing.T, source string, stale, down int) {
	t.Helper()
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		return sources.Set(ctx, tx, r.tenant, uuid.New(), sources.Expectation{SourceSystem: source, StaleAfterSec: stale, DownAfterSec: down, MaxSkewSec: 60})
	})
}

func (r *rig) holds(t *testing.T) map[string]int {
	t.Helper()
	var m map[string]int
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		m, err = OpenHolds(ctx, tx, r.tenant)
		return err
	})
	return m
}

func TestAStuckTransferIsHeldWhileItsSourceIsSilentAndRaisedOnceTheSourceIsBack(t *testing.T) {
	r := newRig(t)
	r.setConfig(t, "waiting_threshold_seconds.NIP", 300)
	at := r.now
	r.event(t, "INS-DG1", txn.EvDebitPosted, at)
	r.event(t, "INS-DG1", txn.EvTransferSent, at.Add(time.Second))
	r.expect(t, "SWITCH_A", 120, 600)

	// 20 minutes pass with nothing from SWITCH_A at all: the source is down, and the
	// transfer's silence says nothing about the transfer.
	r.now = at.Add(20 * time.Minute)
	if f := r.sweep(t); len(f) != 0 {
		t.Fatalf("a case was raised while the source was down: %+v", f)
	}
	if h := r.holds(t); h[cases.CodeUnknown] != 1 {
		t.Fatalf("%v", h)
	}
	if n := len(r.allCases(t)); n != 0 {
		t.Fatalf("%d cases", n)
	}

	// The source is heard from again (another transfer's event), so it is fresh. The
	// first transfer is still unanswered past its threshold, so now it is raised.
	r.event(t, "INS-DG2", txn.EvDebitPosted, r.now)
	f := r.sweep(t)
	var unk int
	for _, x := range f {
		if x.Case.Classification == cases.CodeUnknown && x.Created {
			unk++
		}
	}
	if unk != 1 {
		t.Fatalf("%d unknown cases opened, want the one stuck transfer: %+v", unk, f)
	}
	if h := r.holds(t); h[cases.CodeUnknown] != 0 {
		t.Fatalf("the hold was not released: %v", h)
	}
}

func TestATransferThatIsAnsweredWhileTheSourceWasDownNeverGetsACaseAndItsHoldEnds(t *testing.T) {
	r := newRig(t)
	r.setConfig(t, "waiting_threshold_seconds.NIP", 300)
	at := r.now
	r.event(t, "INS-DG3", txn.EvDebitPosted, at)
	id := r.event(t, "INS-DG3", txn.EvTransferSent, at.Add(time.Second)).TxnID
	r.expect(t, "SWITCH_A", 120, 600)
	r.now = at.Add(20 * time.Minute)
	r.sweep(t)
	if r.holds(t)[cases.CodeUnknown] != 1 {
		t.Fatal("expected a hold")
	}
	// The recovered source delivers the answer that was queued up behind the outage.
	r.event(t, "INS-DG3", txn.EvCreditConfirmed, at.Add(5*time.Second))
	r.eval(t, id)
	if h := r.holds(t); h[cases.CodeUnknown] != 0 {
		t.Fatalf("a resolved transfer is still held: %v", h)
	}
	if f := r.sweep(t); len(f) != 0 || len(r.allCases(t)) != 0 {
		t.Fatalf("a transfer that succeeded got a case: %+v", f)
	}
}

func TestWithNoExpectationSetBehaviourIsUnchanged(t *testing.T) {
	r := newRig(t)
	r.setConfig(t, "waiting_threshold_seconds.NIP", 300)
	at := r.now
	r.event(t, "INS-DG4", txn.EvDebitPosted, at)
	r.event(t, "INS-DG4", txn.EvTransferSent, at.Add(time.Second))
	r.now = at.Add(20 * time.Minute)
	f := r.sweep(t)
	if len(f) != 1 || f[0].Case.Classification != cases.CodeUnknown {
		t.Fatalf("an unconfigured source must not change what the sweep does: %+v", f)
	}
	if len(r.holds(t)) != 0 {
		t.Fatal("a hold was recorded for an unconfigured source")
	}
}

func TestARetryIsNotCalledADuplicateWhileTheFirstAttemptsAnswerIsStuckBehindADownSource(t *testing.T) {
	r := newRig(t)
	r.setConfig(t, "duplicate.window_seconds", 30)
	at := r.now
	// First attempt: debited and sent, the failure is stuck behind the outage.
	r.event(t, "INS-DG5", txn.EvDebitPosted, at)
	first := r.event(t, "INS-DG5", txn.EvTransferSent, at.Add(time.Second)).TxnID
	r.expect(t, "SWITCH_A", 5, 20)
	// Eight seconds later the customer retries (same payer, payee and amount).
	r.now = at.Add(25 * time.Second) // SWITCH_A has been silent 24 seconds: down
	second := r.event(t, "INS-DG6", txn.EvDebitPosted, at.Add(8*time.Second), func(e *txn.Envelope) { e.SourceSystem = "CORE_LEDGER" }).TxnID
	if got := r.eval(t, second); len(got) != 0 {
		t.Fatalf("a retry was called a duplicate while the first attempt's outcome could not be known: %+v", got)
	}
	if r.holds(t)[cases.CodeDuplicate] != 1 {
		t.Fatalf("expected a held duplicate: %v", r.holds(t))
	}

	// The queued failure of the first attempt arrives. The retry was a retry.
	r.now = at.Add(30 * time.Second)
	r.event(t, "INS-DG5", txn.EvCreditFailed, at.Add(3*time.Second))
	r.eval(t, first)
	var sweeped []Finding
	r.step(t, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		sweeped, err = r.det.SweepHolds(ctx, tx, r.tenant)
		return err
	})
	for _, f := range sweeped {
		if f.Case.Classification == cases.CodeDuplicate {
			t.Fatalf("the retry was flagged: %+v", f)
		}
	}
	for _, c := range r.allCases(t) {
		if c.Classification == cases.CodeDuplicate {
			t.Fatalf("a duplicate case exists for a retry after a failure: %+v", c)
		}
	}
	if h := r.holds(t); h[cases.CodeDuplicate] != 0 {
		t.Fatalf("hold not released: %v", h)
	}
}

func TestAGenuineDuplicateHeldDuringAnOutageIsRaisedWhenTheFirstAttemptTurnsOutToHaveSucceeded(t *testing.T) {
	r := newRig(t)
	r.setConfig(t, "duplicate.window_seconds", 30)
	at := r.now
	r.event(t, "INS-DG7", txn.EvDebitPosted, at)
	first := r.event(t, "INS-DG7", txn.EvTransferSent, at.Add(time.Second)).TxnID
	r.expect(t, "SWITCH_A", 5, 20)
	r.now = at.Add(25 * time.Second)
	second := r.event(t, "INS-DG8", txn.EvDebitPosted, at.Add(8*time.Second), func(e *txn.Envelope) { e.SourceSystem = "CORE_LEDGER" }).TxnID
	if got := r.eval(t, second); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
	// The queued answer says the first attempt succeeded: the second debit was a duplicate.
	r.now = at.Add(40 * time.Second)
	r.event(t, "INS-DG7", txn.EvCreditConfirmed, at.Add(3*time.Second))
	r.eval(t, first)
	var found bool
	for _, c := range r.allCases(t) {
		if c.Classification == cases.CodeDuplicate {
			found = true
		}
	}
	if !found {
		t.Fatal("the genuine duplicate was lost")
	}
	// The retry's own hold ends the next time the sweep looks at it.
	r.step(t, func(ctx context.Context, tx pgx.Tx) error { _, err := r.det.SweepHolds(ctx, tx, r.tenant); return err })
	if h := r.holds(t); h[cases.CodeDuplicate] != 0 {
		t.Fatalf("%v", h)
	}
	n := 0
	for _, c := range r.allCases(t) {
		if c.Classification == cases.CodeDuplicate {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d duplicate cases", n)
	}
}
