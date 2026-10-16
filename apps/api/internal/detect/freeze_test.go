package detect

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/automation"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/outbox"
	"github.com/smartreversal/smartreversal/apps/api/internal/stream"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

func (r *rig) withKill() *killswitch.Service {
	k := &killswitch.Service{Audit: &audit.Appender{}}
	r.det.Kill = k
	return k
}

func (r *rig) tx(t *testing.T, fn func(tx pgx.Tx) error) {
	t.Helper()
	if err := r.pool.WithTenant(context.Background(), r.tenant, fn); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) freeze(t *testing.T, scope killswitch.Scope, target string) {
	t.Helper()
	r.tx(t, func(tx pgx.Tx) error {
		_, err := r.det.Kill.Freeze(context.Background(), tx, r.tenant, uuid.New(), killswitch.Target{Scope: scope, Value: target}, "stop it now please")
		return err
	})
}

// unfreeze lifts a freeze the way the control requires: one person asks, another approves.
func (r *rig) unfreeze(t *testing.T, scope killswitch.Scope, target string) {
	t.Helper()
	r.tx(t, func(tx pgx.Tx) error {
		seq, err := r.det.Kill.RequestUnfreeze(context.Background(), tx, r.tenant, uuid.New(), killswitch.Target{Scope: scope, Value: target}, "all clear for now")
		if err != nil {
			return err
		}
		return r.det.Kill.DecideUnfreeze(context.Background(), tx, r.tenant, uuid.New(), seq, true, "confirmed all clear")
	})
}

func (r *rig) deferred(t *testing.T) int {
	t.Helper()
	var n int
	r.tx(t, func(tx pgx.Tx) error {
		c, err := automation.Counts(context.Background(), tx, r.tenant)
		n = c[automation.KindDetect]
		return err
	})
	return n
}

func (r *rig) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(), sql, append([]any{r.tenant}, args...)...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) resume(t *testing.T) (resumed, held int) {
	t.Helper()
	r.tx(t, func(tx pgx.Tx) error {
		var err error
		resumed, held, err = r.det.ResumeDeferred(context.Background(), tx, r.tenant, 100)
		return err
	})
	return
}

func (r *rig) sweep(t *testing.T) []Finding {
	t.Helper()
	var out []Finding
	r.tx(t, func(tx pgx.Tx) error {
		var err error
		out, err = r.det.SweepStuck(context.Background(), tx, r.tenant)
		return err
	})
	return out
}

// action is one thing the detector does that a freeze can stop.
type action struct {
	name string
	code string // exception code the action concerns
	rule string
	// setup builds the world and returns what to run and a check that the action took effect.
	setup func(r *rig, t *testing.T) (run func(), done func() bool)
}

func actions() []action {
	return []action{
		{"open a failure case", cases.CodeNoCredit, RuleFailure, func(r *rig, t *testing.T) (func(), func() bool) {
			id := r.failedTransfer(t, "FZ-FAIL", r.now.Add(-10*time.Minute))
			return func() { r.eval(t, id) }, func() bool { return codes(r.allCases(t))[cases.CodeNoCredit] == 1 }
		}},
		{"open an unknown case", cases.CodeUnknown, RuleUnknown, func(r *rig, t *testing.T) (func(), func() bool) {
			r.event(t, "FZ-UNK", txn.EvDebitPosted, r.now.Add(-10*time.Minute))
			r.event(t, "FZ-UNK", txn.EvTransferSent, r.now.Add(-9*time.Minute))
			id := r.event(t, "FZ-UNK", txn.EvTransferTimeout, r.now.Add(-8*time.Minute)).TxnID
			return func() { r.eval(t, id) }, func() bool { return codes(r.allCases(t))[cases.CodeUnknown] == 1 }
		}},
		{"open a duplicate case", cases.CodeDuplicate, RuleDuplicate, func(r *rig, t *testing.T) (func(), func() bool) {
			r.setConfig(t, "duplicate.window_seconds", 30)
			debit(r, t, "FZ-D1", r.now.Add(-5*time.Minute), "SESSION-FZ")
			second := debit(r, t, "FZ-D2", r.now.Add(-5*time.Minute).Add(3*time.Second), "SESSION-FZ")
			return func() { r.eval(t, second) }, func() bool { return codes(r.allCases(t))[cases.CodeDuplicate] == 1 }
		}},
		{"open a stuck-transaction case in the sweep", cases.CodeUnknown, RuleStuck, func(r *rig, t *testing.T) (func(), func() bool) {
			r.event(t, "FZ-STK", txn.EvDebitPosted, r.now.Add(-20*time.Minute))
			r.event(t, "FZ-STK", txn.EvTransferSent, r.now.Add(-19*time.Minute))
			r.setConfig(t, "waiting_threshold_seconds.NIP", 60)
			return func() { r.sweep(t) }, func() bool { return codes(r.allCases(t))[cases.CodeUnknown] == 1 }
		}},
		{"close an unknown case after late success", cases.CodeUnknown, RuleUnknown, func(r *rig, t *testing.T) (func(), func() bool) {
			r.event(t, "FZ-LATE", txn.EvDebitPosted, r.now.Add(-10*time.Minute))
			r.event(t, "FZ-LATE", txn.EvTransferSent, r.now.Add(-9*time.Minute))
			id := r.event(t, "FZ-LATE", txn.EvTransferTimeout, r.now.Add(-8*time.Minute)).TxnID
			r.eval(t, id) // opened before any freeze
			r.event(t, "FZ-LATE", txn.EvCreditConfirmed, r.now.Add(-time.Minute))
			return func() { r.eval(t, id) }, func() bool {
				cs := r.allCases(t)
				return len(cs) == 1 && cs[0].Status == cases.StClosedNoException
			}
		}},
	}
}

func TestEveryFreezeScopeAgainstEveryDetectorAction(t *testing.T) {
	freezes := []struct {
		name   string
		scope  killswitch.Scope
		target func(a action) string
		covers func(a action) bool
	}{
		{"global", killswitch.ScopeGlobal, func(action) string { return "" }, func(action) bool { return true }},
		{"the action's rail", killswitch.ScopeRail, func(action) string { return "NIP" }, func(action) bool { return true }},
		{"another rail", killswitch.ScopeRail, func(action) string { return "CARD" }, func(action) bool { return false }},
		{"the action's exception type", killswitch.ScopeExceptionType, func(a action) string { return a.code }, func(action) bool { return true }},
		{"another exception type", killswitch.ScopeExceptionType, func(action) string { return "REV001" }, func(action) bool { return false }},
		{"the action's policy", killswitch.ScopePolicy, func(a action) string { return a.rule }, func(action) bool { return true }},
		{"another policy", killswitch.ScopePolicy, func(action) string { return "SOME-OTHER-RULE" }, func(action) bool { return false }},
	}
	for _, a := range actions() {
		for _, f := range freezes {
			a, f := a, f
			t.Run(a.name+" under "+f.name, func(t *testing.T) {
				r := newRig(t)
				r.withKill()
				run, done := a.setup(r, t)
				events := r.count(t, `SELECT count(*) FROM transaction_events WHERE tenant_id = $1`)
				messages := r.count(t, `SELECT count(*) FROM notifications WHERE tenant_id = $1 AND stage = 'detected'`)
				r.freeze(t, f.scope, f.target(a))
				run()
				if f.covers(a) {
					if done() {
						t.Fatal("a covering freeze did not stop the action")
					}
					if r.deferred(t) != 1 {
						t.Fatalf("held work must be recorded, got %d open", r.deferred(t))
					}
					if n := r.count(t, `SELECT count(*) FROM notifications WHERE tenant_id = $1 AND stage = 'detected'`); n != messages {
						t.Fatalf("customer messages went from %d to %d while detection was held", messages, n)
					}
				} else {
					if !done() {
						t.Fatal("a freeze that does not cover the action stopped it")
					}
					if r.deferred(t) != 0 {
						t.Fatal("nothing should be held")
					}
				}
				// Evidence is never held back.
				if got := r.count(t, `SELECT count(*) FROM transaction_events WHERE tenant_id = $1`); got != events {
					t.Fatalf("events changed from %d to %d", events, got)
				}
			})
		}
	}
}

func TestAFreezeThatArrivesWhileTheJobIsRunningStopsTheAction(t *testing.T) {
	r := newRig(t)
	r.withKill()
	id := r.failedTransfer(t, "FZ-MID", r.now.Add(-10*time.Minute))
	frozen := false
	// The job has read the transaction and decided to open a case. Another operator
	// freezes the rail before it acts. The check happens at the action, so it sees it.
	r.det.beforeAction = func() {
		if !frozen {
			frozen = true
			r.freeze(t, killswitch.ScopeRail, "NIP")
		}
	}
	r.eval(t, id)
	if len(r.allCases(t)) != 0 || r.deferred(t) != 1 {
		t.Fatalf("the freeze did not stop the in-flight job: %d cases, %d held", len(r.allCases(t)), r.deferred(t))
	}
}

func TestWorkQueuedWhileFrozenIsKeptAndResumedOnceAfterAnAuthorisedUnfreeze(t *testing.T) {
	r := newRig(t)
	r.withKill()
	r.freeze(t, killswitch.ScopeGlobal, "")

	var ids []uuid.UUID
	for i := 0; i < 3; i++ {
		ids = append(ids, r.failedTransfer(t, fmt.Sprintf("FZ-Q%d", i), r.now.Add(-10*time.Minute)))
	}
	for _, id := range ids {
		r.eval(t, id)
		r.eval(t, id) // the same event delivered twice
	}
	if len(r.allCases(t)) != 0 || r.deferred(t) != 3 {
		t.Fatalf("expected 3 held and no cases, got %d held and %d cases", r.deferred(t), len(r.allCases(t)))
	}
	// Ingestion carried on: every event is stored, projected and announced.
	if n := r.count(t, `SELECT count(*) FROM transaction_events WHERE tenant_id = $1`); n != 9 {
		t.Fatalf("evidence was lost during the freeze: %d events", n)
	}
	if n := r.count(t, `SELECT count(*) FROM outbox_events WHERE tenant_id = $1 AND event_type = 'transaction.updated'`); n != 9 {
		t.Fatalf("expected 9 transaction.updated events, got %d", n)
	}
	if n := r.count(t, `SELECT count(*) FROM transactions WHERE tenant_id = $1 AND lifecycle_state = 'CONFIRMED_FAILURE'`); n != 3 {
		t.Fatalf("the projection must keep working: %d", n)
	}

	// While the freeze stands, resuming changes nothing and loses nothing.
	if resumed, held := r.resume(t); resumed != 0 || held != 3 {
		t.Fatalf("resume under a freeze: %d resumed, %d held", resumed, held)
	}
	if r.deferred(t) != 3 {
		t.Fatal("held work was dropped")
	}

	r.unfreeze(t, killswitch.ScopeGlobal, "")
	if resumed, held := r.resume(t); resumed != 3 || held != 0 {
		t.Fatalf("after the unfreeze: %d resumed, %d held", resumed, held)
	}
	cs := r.allCases(t)
	if len(cs) != 3 || r.deferred(t) != 0 {
		t.Fatalf("expected 3 cases and nothing held: %d cases, %d held", len(cs), r.deferred(t))
	}
	// One customer message per case, ops alerts too, and the case says it was late.
	if n := r.count(t, `SELECT count(*) FROM notifications WHERE tenant_id = $1 AND stage = 'detected'`); n != 3 {
		t.Fatalf("expected 3 customer detection messages, got %d", n)
	}
	if n := r.count(t, `SELECT count(*) FROM case_events WHERE tenant_id = $1 AND kind = 'opened' AND detail->>'explanation' LIKE '%Opened late%'`); n != 3 {
		t.Fatalf("resumed cases must say they were held: %d", n)
	}
	// A case is dated from the original detection, not from the resume.
	if n := r.count(t, `SELECT count(*) FROM cases c JOIN deferred_work d ON d.tenant_id = c.tenant_id AND d.subject_id = c.txn_id
		WHERE c.tenant_id = $1 AND abs(extract(epoch FROM c.opened_at - d.created_at)) < 0.01`); n != 3 {
		t.Fatalf("cases should be dated from the held detection: %d", n)
	}
	// Running it again does nothing, and events retried afterwards add nothing.
	if resumed, _ := r.resume(t); resumed != 0 {
		t.Fatal("resumed twice")
	}
	r.eval(t, ids[0])
	if len(r.allCases(t)) != 3 || r.count(t, `SELECT count(*) FROM notifications WHERE tenant_id = $1 AND stage = 'detected'`) != 3 {
		t.Fatal("a retried event duplicated work")
	}
	// Audit continuity: the chain holds.
	r.tx(t, func(tx pgx.Tx) error {
		if res, err := audit.Verify(context.Background(), tx, r.tenant); err != nil || !res.OK {
			t.Fatalf("audit chain: %+v %v", res, err)
		}
		return nil
	})
}

func TestTwoWorkersResumingAtOnceOpenEachCaseOnce(t *testing.T) {
	r := newRig(t)
	r.withKill()
	r.freeze(t, killswitch.ScopeGlobal, "")
	for i := 0; i < 8; i++ {
		r.eval(t, r.failedTransfer(t, fmt.Sprintf("FZ-W%d", i), r.now.Add(-10*time.Minute)))
	}
	if r.deferred(t) != 8 {
		t.Fatalf("held %d", r.deferred(t))
	}
	r.unfreeze(t, killswitch.ScopeGlobal, "")

	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0
	errs := make(chan error, 4)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
				n, _, err := r.det.ResumeDeferred(context.Background(), tx, r.tenant, 3)
				mu.Lock()
				total += n
				mu.Unlock()
				return err
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	// Drain whatever the first pass left, then check the totals.
	more, _ := r.resume(t)
	if total+more != 8 || len(r.allCases(t)) != 8 || r.deferred(t) != 0 {
		t.Fatalf("resumed %d+%d, %d cases, %d held", total, more, len(r.allCases(t)), r.deferred(t))
	}
	if n := r.count(t, `SELECT count(*) FROM notifications WHERE tenant_id = $1 AND stage = 'detected'`); n != 8 {
		t.Fatalf("customer messages: %d", n)
	}
}

func TestTheDetectorSweepJobPausesUnderAGlobalFreezeAndShowsNarrowerOnes(t *testing.T) {
	r := newRig(t)
	r.withKill()
	r.event(t, "FZ-JOB", txn.EvDebitPosted, r.now.Add(-20*time.Minute))
	r.event(t, "FZ-JOB", txn.EvTransferSent, r.now.Add(-19*time.Minute))
	r.setConfig(t, "waiting_threshold_seconds.NIP", 60)
	job := func() automation.Job {
		var out []automation.Job
		r.tx(t, func(tx pgx.Tx) error {
			var err error
			out, err = automation.Jobs(context.Background(), tx, r.tenant)
			return err
		})
		for _, j := range out {
			if j.Job == automation.JobDetectorSweep {
				return j
			}
		}
		t.Fatal("no sweep job recorded")
		return automation.Job{}
	}
	sweepAll := func() {
		if _, err := r.det.SweepAll(context.Background(), r.pool); err != nil {
			t.Fatal(err)
		}
	}

	r.freeze(t, killswitch.ScopeGlobal, "")
	sweepAll()
	if j := job(); j.State != "paused" || j.Frozen != "global" {
		t.Fatalf("%+v", j)
	}
	if len(r.allCases(t)) != 0 {
		t.Fatal("a paused sweep opened a case")
	}
	r.unfreeze(t, killswitch.ScopeGlobal, "")

	r.freeze(t, killswitch.ScopeRail, "CARD")
	sweepAll()
	if j := job(); j.State != "running" || j.Frozen != "rail:CARD" {
		t.Fatalf("a narrow freeze should be shown but not pause the job: %+v", j)
	}
	if codes(r.allCases(t))[cases.CodeUnknown] != 1 {
		t.Fatal("a freeze on another rail stopped the sweep")
	}
}

// A message that arrives from the stream while a freeze covers it is handled, not
// retried into the dead-letter stream and not lost: it becomes durable deferred
// work in PostgreSQL and is carried out after the unfreeze.
func TestStreamMessagesHeldByAFreezeAreAcknowledgedOnlyBecauseTheyAreStoredAndResume(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set; skipping Redis integration test")
	}
	r := newRig(t)
	r.withKill()
	ctx := context.Background()
	rc := redis.NewClient(&redis.Options{Addr: addr})
	defer rc.Close()
	name := fmt.Sprintf("test.freeze.%d", time.Now().UnixNano())
	defer rc.Del(ctx, name, stream.DeadLetterStream(name))

	if _, err := r.pool.Exec(ctx, `DELETE FROM outbox_events WHERE stream = $1`, txn.Stream); err != nil {
		t.Fatal(err)
	}
	r.freeze(t, killswitch.ScopeRail, "NIP")
	id := r.failedTransfer(t, "FZ-STREAM", r.now.Add(-10*time.Minute))
	if _, err := r.pool.Exec(ctx, `UPDATE outbox_events SET stream = $2 WHERE tenant_id = $1 AND stream = $3`, r.tenant, name, txn.Stream); err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	relay := &outbox.Relay{Pool: r.pool, Redis: rc, Logger: quiet}
	if n, err := relay.PublishPending(ctx); err != nil || n < 3 {
		t.Fatalf("relay published %d: %v", n, err)
	}
	c := &stream.Consumer{Pool: r.pool, Redis: rc, Logger: quiet, Stream: name, Group: "detector-freeze", Name: "c1", Handler: r.det.Handle,
		Block: 50 * time.Millisecond, ClaimIdle: time.Hour, MaxDeliveries: 2}
	if err := c.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := c.RunOnce(ctx); err != nil || n < 3 {
		t.Fatalf("consumer handled %d: %v", n, err)
	}
	if len(r.allCases(t)) != 0 || r.deferred(t) != 1 {
		t.Fatalf("expected one held item and no case: %d cases, %d held", len(r.allCases(t)), r.deferred(t))
	}
	if pending, err := rc.XPending(ctx, name, "detector-freeze").Result(); err != nil || pending.Count != 0 {
		t.Fatalf("messages left unacknowledged: %+v %v", pending, err)
	}
	if n, err := rc.XLen(ctx, stream.DeadLetterStream(name)).Result(); err != nil || n != 0 {
		t.Fatalf("held messages must not be dead-lettered: %d %v", n, err)
	}

	r.unfreeze(t, killswitch.ScopeRail, "NIP")
	if resumed, held := r.resume(t); resumed != 1 || held != 0 {
		t.Fatalf("%d resumed, %d held", resumed, held)
	}
	cs := r.allCases(t)
	if len(cs) != 1 || cs[0].TxnID == nil || *cs[0].TxnID != id {
		t.Fatalf("%+v", cs)
	}
}
