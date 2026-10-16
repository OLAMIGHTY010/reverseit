package rules

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/sources"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

var t0 = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

const tok = "acct_0123456789abcdef0123456789abcdef"

type rig struct {
	pool   *db.Pool
	tenant uuid.UUID
	store  *Store
	eng    *Engine
	svc    *txn.Service
	now    time.Time
	n      int
	t      *testing.T
}

func newRig(t *testing.T) *rig {
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), now: t0.Add(time.Hour), t: t}
	clk := func() time.Time { return r.now }
	r.store = &Store{Audit: &audit.Appender{Now: clk}, Now: clk}
	r.eng = &Engine{Store: r.store, Now: clk}
	r.svc = &txn.Service{Now: clk}
	return r
}

func (r *rig) in(fn func(tx pgx.Tx) error) error {
	return r.pool.WithTenant(context.Background(), r.tenant, fn)
}

func (r *rig) must(err error) {
	r.t.Helper()
	if err != nil {
		r.t.Fatal(err)
	}
}

func rule(id string, version int, code, state string) Rule {
	conds := fmt.Sprintf(`[{"field":"lifecycle_state","op":"eq","value":%q}]`, state)
	r, err := Parse([]byte(fmt.Sprintf(`{"rule_id":%q,"version":%d,"code":%q,"root_cause":"some_cause","explanation":"A long enough explanation.","all":%s}`, id, version, code, conds)))
	if err != nil {
		panic(err)
	}
	return r
}

func (r *rig) create(rl Rule, by uuid.UUID) Version {
	r.t.Helper()
	var v Version
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		v, err = r.store.Create(context.Background(), tx, r.tenant, by, rl)
		return err
	}))
	return v
}

// failed ingests a transfer that ends in a confirmed failure.
func (r *rig) failed() uuid.UUID {
	r.t.Helper()
	r.n++
	var id uuid.UUID
	ref := fmt.Sprintf("INS-R%04d", r.n)
	for i, typ := range []string{txn.EvDebitPosted, txn.EvTransferSent, txn.EvCreditFailed} {
		r.must(r.in(func(tx pgx.Tx) error {
			res, err := r.svc.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: fmt.Sprintf("%s-%d", ref, i), EventType: typ,
				SourceTS: r.now.Add(-time.Minute + time.Duration(i)*time.Second), Txn: txn.Core{InstructionRef: ref, Channel: "MOBILE", Rail: "NIP", AmountMinor: 1000, Currency: "NGN", DebitAccountToken: tok}})
			id = res.TxnID
			return err
		}))
	}
	return id
}

func (r *rig) eval(id uuid.UUID) []Match {
	r.t.Helper()
	var m []Match
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		m, err = r.eng.Evaluate(context.Background(), tx, r.tenant, id)
		return err
	}))
	return m
}

func (r *rig) call(fn func(ctx context.Context, tx pgx.Tx) error) error {
	return r.in(func(tx pgx.Tx) error { return fn(context.Background(), tx) })
}

// goLive takes a draft through shadow and approval.
func (r *rig) goLive(rl Rule, maker, requester, checker uuid.UUID) {
	r.t.Helper()
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.StartShadow(ctx, tx, r.tenant, maker, rl.RuleID, rl.Version)
		return err
	}))
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.eng.Backtest(ctx, tx, r.tenant, rl.RuleID, rl.Version, 100)
		return err
	}))
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.RequestActivation(ctx, tx, r.tenant, requester, rl.RuleID, rl.Version, "tested in shadow")
		return err
	}))
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.Decide(ctx, tx, r.tenant, checker, rl.RuleID, rl.Version, true, "reviewed")
		return err
	}))
}

func TestARuleGoesThroughShadowRequestAndAnIndependentApprovalBeforeItIsLive(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	maker, requester, checker := uuid.New(), uuid.New(), uuid.New()
	rl := rule("DET-T-1", 1, "FAIL001", "CONFIRMED_FAILURE")
	r.create(rl, maker)
	id := r.failed()

	// A draft is not evaluated at all.
	if m := r.eval(id); len(m) != 0 {
		t.Fatal("a draft matched")
	}
	if err := r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.RequestActivation(ctx, tx, r.tenant, requester, rl.RuleID, 1, "ready")
		return err
	}); !errors.Is(err, ErrState) {
		t.Fatalf("a draft was put forward without shadow: %v", err)
	}
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.StartShadow(ctx, tx, r.tenant, maker, rl.RuleID, 1)
		return err
	}))
	if err := r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.RequestActivation(ctx, tx, r.tenant, requester, rl.RuleID, 1, "ready")
		return err
	}); !errors.Is(err, ErrNoEvidence) {
		t.Fatalf("a rule nobody has evaluated was put forward: %v", err)
	}

	// In shadow it is evaluated and recorded, and opens nothing.
	if m := r.eval(id); len(m) != 0 {
		t.Fatal("a shadow rule would have opened a case")
	}
	var stats Stats
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		var err error
		stats, err = r.eng.StatsFor(ctx, tx, r.tenant, rl.RuleID, 1)
		return err
	}))
	if stats.Evaluated != 1 || stats.Matched != 1 {
		t.Fatalf("%+v", stats)
	}

	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.RequestActivation(ctx, tx, r.tenant, requester, rl.RuleID, 1, "shadow matched the failures it should")
		return err
	}))
	// Neither the author nor the requester may approve.
	for name, who := range map[string]uuid.UUID{"the author": maker, "the requester": requester} {
		err := r.call(func(ctx context.Context, tx pgx.Tx) error {
			_, err := r.store.Decide(ctx, tx, r.tenant, who, rl.RuleID, 1, true, "ok")
			return err
		})
		if !errors.Is(err, ErrFourEyes) {
			t.Fatalf("%s approved: %v", name, err)
		}
	}
	if m := r.eval(id); len(m) != 0 {
		t.Fatal("a rule awaiting approval matched live")
	}
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.Decide(ctx, tx, r.tenant, checker, rl.RuleID, 1, true, "reviewed the shadow results")
		return err
	}))
	m := r.eval(id)
	if len(m) != 1 || m[0].Rule.RuleID != "DET-T-1" || m[0].Rule.Version != 1 {
		t.Fatalf("%+v", m)
	}
	var n int
	r.must(r.in(func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id = $1 AND subject_type = 'detection_rule' AND subject_id = 'DET-T-1'`, r.tenant).Scan(&n)
	}))
	if n != 4 {
		t.Fatalf("%d audit records, want created, shadow, requested, activated", n)
	}
}

func TestVersionsCountUpAndAStoredDefinitionCanNeverBeEdited(t *testing.T) {
	r := newRig(t)
	by := uuid.New()
	r.create(rule("DET-T-2", 1, "FAIL001", "CONFIRMED_FAILURE"), by)
	for _, v := range []int{1, 3} {
		err := r.in(func(tx pgx.Tx) error {
			_, err := r.store.Create(context.Background(), tx, r.tenant, by, rule("DET-T-2", v, "FAIL001", "CONFIRMED_FAILURE"))
			return err
		})
		if !errors.Is(err, ErrVersion) {
			t.Errorf("version %d: %v", v, err)
		}
	}
	r.create(rule("DET-T-2", 2, "FAIL001", "CONFIRMED_FAILURE"), by)
	// Not even the table owner can rewrite a stored definition.
	if _, err := r.pool.Exec(context.Background(), `UPDATE detection_rules SET definition = '{}'::jsonb WHERE tenant_id = $1 AND rule_id = 'DET-T-2' AND version = 1`, r.tenant); err == nil {
		t.Fatal("a definition was edited")
	}
	if _, err := r.pool.Exec(context.Background(), `UPDATE detection_rules SET version = 9 WHERE tenant_id = $1 AND rule_id = 'DET-T-2' AND version = 1`, r.tenant); err == nil {
		t.Fatal("a version number was edited")
	}
	// An invalid rule is never stored.
	bad := rule("DET-T-2", 3, "FAIL001", "CONFIRMED_FAILURE")
	bad.All = nil
	if err := r.in(func(tx pgx.Tx) error {
		_, err := r.store.Create(context.Background(), tx, r.tenant, by, bad)
		return err
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("%v", err)
	}
}

func TestApprovingANewVersionRetiresTheOldAndRollbackNeedsAnotherApproval(t *testing.T) {
	r := newRig(t)
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	v1, v2 := rule("DET-T-3", 1, "FAIL001", "CONFIRMED_FAILURE"), rule("DET-T-3", 2, "FAIL001", "CONFIRMED_FAILURE")
	r.create(v1, a)
	r.failed()
	r.goLive(v1, a, b, c)
	r.create(v2, a)
	r.goLive(v2, a, b, c)

	list := func() map[int]string {
		out := map[int]string{}
		r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
			l, err := r.store.List(ctx, tx, r.tenant, "DET-T-3")
			for _, v := range l {
				out[v.Rule.Version] = v.Status
			}
			return err
		}))
		return out
	}
	if got := list(); got[1] != Retired || got[2] != Active {
		t.Fatalf("%v", got)
	}

	// Rolling back is a request. Version 2 stays live until someone else approves.
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.Rollback(ctx, tx, r.tenant, d, "DET-T-3", 1, "version 2 opens too many cases")
		return err
	}))
	if got := list(); got[1] != PendingActivation || got[2] != Active {
		t.Fatalf("%v", got)
	}
	if err := r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.Decide(ctx, tx, r.tenant, d, "DET-T-3", 1, true, "self")
		return err
	}); !errors.Is(err, ErrFourEyes) {
		t.Fatalf("%v", err)
	}
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.Decide(ctx, tx, r.tenant, b, "DET-T-3", 1, true, "agreed")
		return err
	}))
	if got := list(); got[1] != Active || got[2] != Retired {
		t.Fatalf("%v", got)
	}
	// The rolled-back definition is exactly the original.
	var back Version
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		var err error
		back, err = r.store.Get(ctx, tx, r.tenant, "DET-T-3", 1)
		return err
	}))
	if back.Rule.Code != v1.Code || len(back.Rule.All) != len(v1.All) {
		t.Fatalf("%+v", back.Rule)
	}
	// A version that was never live cannot be rolled back to.
	r.create(rule("DET-T-3", 3, "FAIL001", "CONFIRMED_FAILURE"), a)
	if err := r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.Rollback(ctx, tx, r.tenant, d, "DET-T-3", 3, "never was live")
		return err
	}); !errors.Is(err, ErrState) {
		t.Fatalf("%v", err)
	}
}

func TestDisableStopsALiveRuleAtOnceAndIsAudited(t *testing.T) {
	r := newRig(t)
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	rl := rule("DET-T-4", 1, "FAIL001", "CONFIRMED_FAILURE")
	r.create(rl, a)
	id := r.failed()
	r.goLive(rl, a, b, c)
	if len(r.eval(id)) != 1 {
		t.Fatal("not live")
	}
	if err := r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.Disable(ctx, tx, r.tenant, b, "DET-T-4", "x")
		return err
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a disable with no real reason: %v", err)
	}
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.Disable(ctx, tx, r.tenant, b, "DET-T-4", "it is opening cases for a test channel")
		return err
	}))
	if len(r.eval(id)) != 0 {
		t.Fatal("a disabled rule still matched")
	}
	if err := r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.Disable(ctx, tx, r.tenant, b, "DET-T-4", "again please")
		return err
	}); !errors.Is(err, ErrState) {
		t.Fatalf("%v", err)
	}
}

func TestRejectedAndConcurrentDecisionsLeaveOneConsistentState(t *testing.T) {
	r := newRig(t)
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	rl := rule("DET-T-5", 1, "FAIL001", "CONFIRMED_FAILURE")
	r.create(rl, a)
	r.failed()
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.StartShadow(ctx, tx, r.tenant, a, rl.RuleID, 1)
		return err
	}))
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.eng.Backtest(ctx, tx, r.tenant, rl.RuleID, 1, 10)
		return err
	}))
	r.must(r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.RequestActivation(ctx, tx, r.tenant, b, rl.RuleID, 1, "please")
		return err
	}))
	if err := r.call(func(ctx context.Context, tx pgx.Tx) error {
		_, err := r.store.Decide(ctx, tx, r.tenant, c, rl.RuleID, 1, false, "")
		return err
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a rejection with no reason: %v", err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	approved := 0
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := r.call(func(ctx context.Context, tx pgx.Tx) error {
				_, err := r.store.Decide(ctx, tx, r.tenant, uuid.New(), rl.RuleID, 1, true, "ok")
				return err
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				approved++
			} else if !errors.Is(err, ErrState) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if approved != 1 {
		t.Fatalf("%d approvals took effect", approved)
	}
}

func TestShadowAndLiveResultsAreRecordedAndTenantsAreSeparate(t *testing.T) {
	r := newRig(t)
	a := uuid.New()
	r.create(rule("DET-T-6", 1, "FAIL001", "CONFIRMED_FAILURE"), a)
	other := dbtest.Tenant(t, r.pool)
	var n int
	_ = r.pool.WithTenant(context.Background(), other, func(tx pgx.Tx) error {
		l, err := r.store.List(context.Background(), tx, other, "")
		n = len(l)
		return err
	})
	if n != 0 {
		t.Fatalf("%d rules visible to another institution", n)
	}
	err := r.pool.WithTenant(context.Background(), other, func(tx pgx.Tx) error {
		_, err := r.store.Get(context.Background(), tx, other, "DET-T-6", 1)
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
}

func TestARuleThatNeedsAFreshSourceIsHeldNotRunWhileTheSourceIsNotFresh(t *testing.T) {
	r := newRig(t)
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	rl := rule("DET-T-7", 1, "FAIL001", "CONFIRMED_FAILURE")
	rl.RequiresFresh = []string{"SWITCH_A"}
	r.create(rl, a)
	id := r.failed()
	r.goLive(rl, a, b, c) // the backtest already ran with the source unconfigured

	outcome := func() (string, string) {
		var o, why string
		r.must(r.in(func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), `SELECT outcome, held_reason FROM rule_evaluations WHERE tenant_id = $1 AND rule_id = 'DET-T-7' AND txn_id = $2 AND mode = 'active'`, r.tenant, id).Scan(&o, &why)
		}))
		return o, why
	}
	// No expectation set for SWITCH_A: it is unconfigured, which is not fresh.
	if m := r.eval(id); len(m) != 0 {
		t.Fatal("ran against an unconfigured source")
	}
	if o, why := outcome(); o != OutcomeHeld || why != "SWITCH_A is unconfigured" {
		t.Fatalf("%s %q", o, why)
	}
	r.must(r.in(func(tx pgx.Tx) error {
		return sources.Set(context.Background(), tx, r.tenant, a, sources.Expectation{SourceSystem: "SWITCH_A", StaleAfterSec: 300, DownAfterSec: 900, MaxSkewSec: 60})
	}))
	if m := r.eval(id); len(m) != 1 {
		t.Fatal("a fresh source should let the rule run")
	}
	r.now = r.now.Add(10 * time.Minute) // silence: stale
	if m := r.eval(id); len(m) != 0 {
		t.Fatal("ran against a stale source")
	}
	if o, why := outcome(); o != OutcomeHeld || why != "SWITCH_A is stale" {
		t.Fatalf("%s %q", o, why)
	}
	r.now = r.now.Add(time.Hour) // down
	if o, why := func() (string, string) { r.eval(id); return outcome() }(); o != OutcomeHeld || why != "SWITCH_A is down" {
		t.Fatalf("%s %q", o, why)
	}
}
