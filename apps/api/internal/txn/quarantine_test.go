package txn

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

func (r *rig) quarantine(t *testing.T, e Envelope, reason string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		if err := r.svc.QuarantineEnvelope(context.Background(), tx, r.tenant, e, reason); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), `SELECT id FROM quarantined_events WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 1`, r.tenant).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) replay(t *testing.T, id uuid.UUID) (ReplayResult, error) {
	t.Helper()
	var res ReplayResult
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		res, err = r.svc.Replay(context.Background(), tx, r.tenant, id)
		return err
	})
	return res, err
}

func (r *rig) qrow(t *testing.T, id uuid.UUID) (status string, attempts int) {
	t.Helper()
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT status, attempts FROM quarantined_events WHERE id = $1`, id).Scan(&status, &attempts)
	})
	if err != nil {
		t.Fatal(err)
	}
	return
}

func (r *rig) eventCount(t *testing.T) (n int) {
	t.Helper()
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM transaction_events WHERE tenant_id = $1`, r.tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return
}

func TestReplayIsRefusedWhileTheCauseRemainsAndSucceedsOnceItIsFixed(t *testing.T) {
	r := newRig(t)
	// The service clock reads one hour after t0. This event claims to be from two
	// hours after t0, which is in the future, as when a source's clock runs ahead.
	e := env("INS-Q001", EvDebitPosted, "EV-Q001", t0.Add(2*time.Hour))
	id := r.quarantine(t, e, "source_ts is in the future")

	res, err := r.replay(t, id)
	if err != nil || res.Replayed || res.Outcome != "refused_again" {
		t.Fatalf("%+v %v", res, err)
	}
	if st, n := r.qrow(t, id); st != "open" || n != 1 || r.eventCount(t) != 0 {
		t.Fatalf("a refused replay changed state: %s %d events=%d", st, n, r.eventCount(t))
	}

	r.svc.Now = func() time.Time { return t0.Add(2*time.Hour + 10*time.Minute) } // time has caught up
	res, err = r.replay(t, id)
	if err != nil || !res.Replayed || res.Outcome != Accepted {
		t.Fatalf("%+v %v", res, err)
	}
	if st, n := r.qrow(t, id); st != "replayed" || n != 2 || r.eventCount(t) != 1 {
		t.Fatalf("%s %d %d", st, n, r.eventCount(t))
	}
	if state, _, _ := r.row(t, "INS-Q001"); state != StateDebited {
		t.Fatalf("the replayed event did not project: %s", state)
	}
	if _, err := r.replay(t, id); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("a replayed event was replayed again: %v", err)
	}
}

func TestReplayOfAnEventThatConflictsStaysRefusedAndChangesNothing(t *testing.T) {
	r := newRig(t)
	if _, err := r.ingest(t, env("INS-Q002", EvDebitPosted, "EV-Q002", t0)); err != nil {
		t.Fatal(err)
	}
	changed := env("INS-Q002", EvDebitPosted, "EV-Q002", t0)
	changed.Txn.AmountMinor = 99
	id := r.quarantine(t, changed, "reference reused")
	res, err := r.replay(t, id)
	if err != nil || res.Replayed || res.Outcome != "refused_again" {
		t.Fatalf("%+v %v", res, err)
	}
	var amount int64
	_ = r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT amount_minor FROM transactions WHERE instruction_ref = 'INS-Q002'`).Scan(&amount)
	})
	if amount != 1850000 || r.eventCount(t) != 1 {
		t.Fatalf("amount %d events %d", amount, r.eventCount(t))
	}
}

func TestReplayOfAnEventAlreadyStoredIsReportedAsADuplicate(t *testing.T) {
	r := newRig(t)
	e := env("INS-Q003", EvDebitPosted, "EV-Q003", t0)
	if _, err := r.ingest(t, e); err != nil {
		t.Fatal(err)
	}
	id := r.quarantine(t, e, "arrived while the source was retrying")
	res, err := r.replay(t, id)
	if err != nil || !res.Replayed || res.Outcome != Duplicate || r.eventCount(t) != 1 {
		t.Fatalf("%+v %v %d", res, err, r.eventCount(t))
	}
}

func TestOnlyOneOfSeveralConcurrentReplaysRuns(t *testing.T) {
	r := newRig(t)
	id := r.quarantine(t, env("INS-Q004", EvDebitPosted, "EV-Q004", t0), "retry")
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, notOpen := 0, 0
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.replay(t, id)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, ErrNotOpen):
				notOpen++
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || notOpen != 5 || r.eventCount(t) != 1 {
		t.Fatalf("ok=%d notOpen=%d events=%d", ok, notOpen, r.eventCount(t))
	}
}

func TestRowsWithoutAPayloadCannotBeReplayedAndDiscardKeepsTheRow(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if err := r.pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error {
		return r.svc.Quarantine(ctx, tx, r.tenant, "SWITCH_A", "malformed JSON", "hash")
	}); err != nil {
		t.Fatal(err)
	}
	var list []Quarantined
	_ = r.pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error {
		var err error
		list, err = ListQuarantined(ctx, tx, r.tenant, "", 10)
		return err
	})
	if len(list) != 1 || list[0].Replayable {
		t.Fatalf("%+v", list)
	}
	if _, err := r.replay(t, list[0].ID); !errors.Is(err, ErrNotReplayable) {
		t.Fatalf("%v", err)
	}
	if err := r.pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error { return Discard(ctx, tx, r.tenant, list[0].ID, "unreadable") }); err != nil {
		t.Fatal(err)
	}
	if st, _ := r.qrow(t, list[0].ID); st != "discarded" {
		t.Fatal(st)
	}
	err := r.pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error { return Discard(ctx, tx, r.tenant, list[0].ID, "again") })
	if !errors.Is(err, ErrNotOpen) {
		t.Fatalf("%v", err)
	}
	err = r.pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error { return Discard(ctx, tx, r.tenant, uuid.New(), "x") })
	if !errors.Is(err, ErrQuarantineNotFound) {
		t.Fatalf("%v", err)
	}
}

func TestAnotherInstitutionCannotSeeOrReplayQuarantinedEvents(t *testing.T) {
	r := newRig(t)
	id := r.quarantine(t, env("INS-Q005", EvDebitPosted, "EV-Q005", t0), "x")
	other := dbtest.Tenant(t, r.pool)
	ctx := context.Background()
	err := r.pool.WithTenant(ctx, other, func(tx pgx.Tx) error {
		_, err := r.svc.Replay(ctx, tx, other, id)
		return err
	})
	if !errors.Is(err, ErrQuarantineNotFound) {
		t.Fatalf("%v", err)
	}
	var n int
	_ = r.pool.WithTenant(ctx, other, func(tx pgx.Tx) error {
		l, _ := ListQuarantined(ctx, tx, other, "", 10)
		n = len(l)
		return nil
	})
	if n != 0 {
		t.Fatalf("%d visible", n)
	}
}
