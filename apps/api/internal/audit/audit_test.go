package audit

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type rig struct {
	pool   *db.Pool
	tenant uuid.UUID
	app    *Appender
	actor  uuid.UUID
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := dbtest.Pool(t)
	return &rig{pool: pool, tenant: dbtest.Tenant(t, pool), app: &Appender{}, actor: uuid.New()}
}

func (r *rig) event(action string) Event {
	return Event{Actor: ActorUser, ActorID: r.actor, Action: action, SubjectType: "case", SubjectID: "CASE-1"}
}

func (r *rig) append(t *testing.T, e Event) Record {
	t.Helper()
	var rec Record
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		rec, err = r.app.Append(context.Background(), tx, r.tenant, e)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func (r *rig) verify(t *testing.T) VerifyResult {
	t.Helper()
	var res VerifyResult
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		res, err = Verify(context.Background(), tx, r.tenant)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// tamper runs sql as the privileged role with triggers and foreign keys off,
// the way someone with database superuser rights could.
func (r *rig) tamper(t *testing.T, sql string, args ...any) {
	t.Helper()
	err := r.pool.WithTx(context.Background(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `SET LOCAL session_replication_role = replica`); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), sql, args...)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAppendChainsRecordsAndVerifies(t *testing.T) {
	r := newRig(t)
	a := r.append(t, r.event("case.opened"))
	b := r.append(t, r.event("case.assigned"))
	c := r.append(t, r.event("case.closed"))

	if a.ChainSeq != 1 || b.ChainSeq != 2 || c.ChainSeq != 3 {
		t.Fatalf("sequence: %d %d %d", a.ChainSeq, b.ChainSeq, c.ChainSeq)
	}
	if a.PrevHash != Genesis(r.tenant) || b.PrevHash != a.RecordHash || c.PrevHash != b.RecordHash {
		t.Fatal("records are not linked")
	}
	res := r.verify(t)
	if !res.OK || res.Checked != 3 || res.Head.Seq != 3 || res.Head.Hash != c.RecordHash {
		t.Fatalf("verify: %+v", res)
	}
}

func TestAnEmptyChainVerifies(t *testing.T) {
	r := newRig(t)
	res := r.verify(t)
	if !res.OK || res.Checked != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestEveryFieldSurvivesTheDatabaseRoundTrip(t *testing.T) {
	r := newRig(t)
	e := Event{
		Actor: ActorService, ActorID: r.actor, Action: "reversal.submitted",
		SubjectType: "reversal", SubjectID: "RV-000921", ReasonCode: "DEBIT_NO_CREDIT_CONFIRMED_FAILED",
		EvidenceRefs: []string{"EV-77120", "EV-77121"}, BeforeHash: "aa", AfterHash: "bb",
		PolicyVersion: "P-NIPREV-3.2", IP: "203.0.113.7", Device: "svc-1", Sensitive: true,
		OccurredAt: time.Date(2026, 10, 15, 9, 12, 41, 123456789, time.FixedZone("WAT", 3600)),
		Details: map[string]any{
			"amount_minor": int64(1850000), "ratio": 1.5, "html": "<b>&</b>", "unicode": "₦ naira",
			"nested": map[string]any{"b": []any{1, "two", true, nil}, "a": "x"},
		},
	}
	rec := r.append(t, e)
	if rec.OccurredAt.Nanosecond()%1000 != 0 {
		t.Fatal("timestamp not truncated to microseconds")
	}

	var got []Record
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		got, err = List(context.Background(), tx, r.tenant, Filter{}, 0, 10)
		return err
	})
	if err != nil || len(got) != 1 {
		t.Fatalf("list: %v %d", err, len(got))
	}
	g := got[0]
	if g.RecordHash != rec.RecordHash || g.ActorID != r.actor || g.Actor != ActorService || g.SubjectID != "RV-000921" ||
		g.ReasonCode != e.ReasonCode || len(g.EvidenceRefs) != 2 || g.PolicyVersion != "P-NIPREV-3.2" ||
		!g.Sensitive || g.IP != "203.0.113.7" || !g.OccurredAt.Equal(rec.OccurredAt) {
		t.Fatalf("round trip lost data: %+v", g)
	}
	if res := r.verify(t); !res.OK {
		t.Fatalf("a faithfully stored record fails verification: %+v", res)
	}
}

func TestSystemEventsNeedNoActorID(t *testing.T) {
	r := newRig(t)
	rec := r.append(t, Event{Actor: ActorSystem, Action: "scheduler.tick"})
	if rec.ActorID != uuid.Nil || !r.verify(t).OK {
		t.Fatalf("%+v", rec)
	}
}

func TestInvalidEventsAreRefused(t *testing.T) {
	r := newRig(t)
	bad := map[string]Event{
		"user without id":    {Actor: ActorUser, Action: "a.b"},
		"unknown actor type": {Actor: "robot", Action: "a.b"},
		"empty action":       {Actor: ActorSystem},
		"upper case action":  {Actor: ActorSystem, Action: "Case.Opened"},
		"action with space":  {Actor: ActorSystem, Action: "case opened"},
		"password in detail": {Actor: ActorSystem, Action: "a.b", Details: map[string]any{"user_password": "x"}},
		"token in detail":    {Actor: ActorSystem, Action: "a.b", Details: map[string]any{"AccessToken": "x"}},
		"unmarshalable":      {Actor: ActorSystem, Action: "a.b", Details: map[string]any{"c": make(chan int)}},
	}
	for name, e := range bad {
		err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			_, err := r.app.Append(context.Background(), tx, r.tenant, e)
			return err
		})
		if err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if res := r.verify(t); res.Checked != 0 {
		t.Fatalf("refused events left %d records", res.Checked)
	}
}

func TestRolledBackAppendLeavesNoTraceAndTheChainContinues(t *testing.T) {
	r := newRig(t)
	boom := errors.New("business change failed")
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		if _, err := r.app.Append(context.Background(), tx, r.tenant, r.event("case.opened")); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if res := r.verify(t); res.Checked != 0 {
		t.Fatalf("rolled back event persisted: %+v", res)
	}
	if rec := r.append(t, r.event("case.opened")); rec.ChainSeq != 1 || rec.PrevHash != Genesis(r.tenant) {
		t.Fatalf("chain did not restart cleanly: %+v", rec)
	}
}

func TestConcurrentWritersNeverForkTheChain(t *testing.T) {
	r := newRig(t)
	const n = 25
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
				_, err := r.app.Append(context.Background(), tx, r.tenant, r.event("case.noted"))
				return err
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("append failed under concurrency: %v", err)
		}
	}
	if res := r.verify(t); !res.OK || res.Checked != n || res.Head.Seq != n {
		t.Fatalf("%+v", res)
	}
}

func TestDatabaseRefusesAForkedOrSkippedRecord(t *testing.T) {
	r := newRig(t)
	first := r.append(t, r.event("case.opened"))

	insert := func(seq int64, prev string) error {
		return r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(), `
				INSERT INTO audit_events (tenant_id, chain_seq, id, occurred_at, actor_type, action, prev_hash, record_hash)
				VALUES ($1, $2, $3, now(), 'system', 'forged.entry', $4, $5)`,
				r.tenant, seq, uuid.New(), prev, strings.Repeat("c", 64))
			return err
		})
	}
	if insert(1, Genesis(r.tenant)) == nil {
		t.Error("a second record with sequence 1 was accepted")
	}
	if insert(5, first.RecordHash) == nil {
		t.Error("a record skipping ahead was accepted")
	}
	if insert(2, strings.Repeat("d", 64)) == nil {
		t.Error("a record with the wrong previous hash was accepted")
	}
	if res := r.verify(t); !res.OK || res.Checked != 1 {
		t.Fatalf("refused inserts changed the chain: %+v", res)
	}
}

func TestTenantRoleCannotChangeOrRemoveAuditRecordsOrTouchTheHead(t *testing.T) {
	r := newRig(t)
	r.append(t, r.event("case.opened"))

	for name, sql := range map[string]string{
		"update":       `UPDATE audit_events SET action = 'case.hidden'`,
		"delete":       `DELETE FROM audit_events`,
		"truncate":     `TRUNCATE audit_events`,
		"read head":    `SELECT * FROM audit_heads`,
		"rewrite head": `UPDATE audit_heads SET last_hash = 'x'`,
		"delete head":  `DELETE FROM audit_heads`,
	} {
		err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(), sql)
			return err
		})
		if err == nil {
			t.Errorf("tenant role could %s", name)
		}
	}
	if res := r.verify(t); !res.OK || res.Checked != 1 {
		t.Fatalf("%+v", res)
	}
}

func TestEvenTheOwnerCannotUpdateOrDeleteWithoutSwitchingTriggersOff(t *testing.T) {
	r := newRig(t)
	r.append(t, r.event("case.opened"))
	for _, sql := range []string{`UPDATE audit_events SET action = 'case.hidden'`, `DELETE FROM audit_events`, `TRUNCATE audit_events`} {
		if _, err := r.pool.Exec(context.Background(), sql); err == nil {
			t.Errorf("owner ran %q", sql)
		}
	}
}

// ---- tamper detection ------------------------------------------------------

func buildChain(t *testing.T, r *rig, n int) []Record {
	t.Helper()
	var recs []Record
	for i := 0; i < n; i++ {
		e := r.event("case.noted")
		e.Details = map[string]any{"n": i, "note": "original"}
		recs = append(recs, r.append(t, e))
	}
	return recs
}

func TestVerifyDetectsEveryKindOfTampering(t *testing.T) {
	cases := []struct {
		name    string
		sql     string
		args    func(r *rig) []any
		badSeq  int64
		contain string
	}{
		{"changed details", `UPDATE audit_events SET details = '{"n": 1, "note": "edited"}' WHERE tenant_id = $1 AND chain_seq = 2`,
			nil, 2, "content"},
		{"changed action", `UPDATE audit_events SET action = 'case.deleted' WHERE tenant_id = $1 AND chain_seq = 3`,
			nil, 3, "content"},
		{"changed actor", `UPDATE audit_events SET actor_id = gen_random_uuid() WHERE tenant_id = $1 AND chain_seq = 1`,
			nil, 1, "content"},
		{"changed time", `UPDATE audit_events SET occurred_at = occurred_at + interval '1 second' WHERE tenant_id = $1 AND chain_seq = 2`,
			nil, 2, "content"},
		{"changed sensitive flag", `UPDATE audit_events SET sensitive_access = true WHERE tenant_id = $1 AND chain_seq = 4`,
			nil, 4, "content"},
		{"changed previous hash", `UPDATE audit_events SET prev_hash = repeat('e', 64) WHERE tenant_id = $1 AND chain_seq = 3`,
			nil, 3, "previous hash"},
		{"deleted middle record", `DELETE FROM audit_events WHERE tenant_id = $1 AND chain_seq = 2`,
			nil, 2, "gap"},
		{"deleted first record", `DELETE FROM audit_events WHERE tenant_id = $1 AND chain_seq = 1`,
			nil, 1, "gap"},
		{"deleted last record", `DELETE FROM audit_events WHERE tenant_id = $1 AND chain_seq = 5`,
			nil, 5, "missing"},
		{"deleted all records", `DELETE FROM audit_events WHERE tenant_id = $1`,
			nil, 1, "missing"},
		{"swapped two records", `UPDATE audit_events SET chain_seq = 99 WHERE tenant_id = '{T}' AND chain_seq = 2;
			UPDATE audit_events SET chain_seq = 2 WHERE tenant_id = '{T}' AND chain_seq = 3;
			UPDATE audit_events SET chain_seq = 3 WHERE tenant_id = '{T}' AND chain_seq = 99`,
			nil, 2, "previous hash"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			buildChain(t, r, 5)
			if res := r.verify(t); !res.OK {
				t.Fatalf("clean chain failed: %+v", res)
			}
			if strings.Contains(c.sql, "{T}") {
				r.tamper(t, strings.ReplaceAll(c.sql, "{T}", r.tenant.String())) // several statements, no parameters
			} else {
				r.tamper(t, c.sql, r.tenant)
			}
			res := r.verify(t)
			if res.OK {
				t.Fatal("tampering went undetected")
			}
			if res.FirstBadSeq != c.badSeq {
				t.Errorf("first bad record %d, want %d (%s)", res.FirstBadSeq, c.badSeq, res.Reason)
			}
			if !strings.Contains(res.Reason, c.contain) {
				t.Errorf("reason %q should mention %q", res.Reason, c.contain)
			}
		})
	}
}

func TestVerifyDetectsAnAppendedForgeryThatBypassedTheHead(t *testing.T) {
	r := newRig(t)
	recs := buildChain(t, r, 3)
	r.tamper(t, `INSERT INTO audit_events (tenant_id, chain_seq, id, occurred_at, actor_type, action, prev_hash, record_hash)
		VALUES ($1, 4, gen_random_uuid(), now(), 'system', 'forged.entry', $2, repeat('f', 64))`, r.tenant, recs[2].RecordHash)
	res := r.verify(t)
	if res.OK || res.FirstBadSeq != 4 {
		t.Fatalf("forged tail record accepted: %+v", res)
	}
}

func TestVerifyIsPerTenantAndRefusesTheWrongTenantContext(t *testing.T) {
	a, b := newRig(t), newRig(t)
	buildChain(t, a, 2)
	buildChain(t, b, 3)
	if ra, rb := a.verify(t), b.verify(t); !ra.OK || ra.Checked != 2 || !rb.OK || rb.Checked != 3 {
		t.Fatalf("a=%+v b=%+v", ra, rb)
	}
	// Tampering with A's chain does not affect B.
	a.tamper(t, `UPDATE audit_events SET action = 'x.y' WHERE tenant_id = $1 AND chain_seq = 1`, a.tenant)
	if a.verify(t).OK || !b.verify(t).OK {
		t.Fatal("tampering leaked across tenants")
	}

	// Asking for tenant A from inside tenant B's transaction is an error, not an empty chain.
	err := b.pool.WithTenant(context.Background(), b.tenant, func(tx pgx.Tx) error {
		if _, err := Verify(context.Background(), tx, a.tenant); err == nil {
			t.Error("verify accepted another tenant's id")
		}
		if _, err := List(context.Background(), tx, a.tenant, Filter{}, 0, 10); err == nil {
			t.Error("list accepted another tenant's id")
		}
		if _, err := b.app.Append(context.Background(), tx, a.tenant, a.event("case.opened")); err == nil {
			t.Error("append accepted another tenant's id")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestListFiltersAndPages(t *testing.T) {
	r := newRig(t)
	other := uuid.New()
	r.append(t, Event{Actor: ActorUser, ActorID: r.actor, Action: "case.opened", SubjectType: "case", SubjectID: "C1"})
	r.append(t, Event{Actor: ActorUser, ActorID: other, Action: "case.opened", SubjectType: "case", SubjectID: "C2"})
	r.append(t, Event{Actor: ActorUser, ActorID: r.actor, Action: "case.closed", SubjectType: "case", SubjectID: "C1"})
	r.append(t, Event{Actor: ActorSystem, Action: "scheduler.tick"})

	list := func(f Filter, after int64, limit int) []Record {
		var out []Record
		err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			var err error
			out, err = List(context.Background(), tx, r.tenant, f, after, limit)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if n := len(list(Filter{}, 0, 100)); n != 4 {
		t.Errorf("all: %d", n)
	}
	if n := len(list(Filter{SubjectID: "C1"}, 0, 100)); n != 2 {
		t.Errorf("subject: %d", n)
	}
	if n := len(list(Filter{ActorID: other}, 0, 100)); n != 1 {
		t.Errorf("actor: %d", n)
	}
	if n := len(list(Filter{Action: "case.opened"}, 0, 100)); n != 2 {
		t.Errorf("action: %d", n)
	}
	page1 := list(Filter{}, 0, 3)
	page2 := list(Filter{}, page1[len(page1)-1].ChainSeq, 3)
	if len(page1) != 3 || len(page2) != 1 || page2[0].ChainSeq != 4 {
		t.Errorf("paging: %d then %d", len(page1), len(page2))
	}
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		for _, limit := range []int{0, -1, 501} {
			if _, err := List(context.Background(), tx, r.tenant, Filter{}, 0, limit); err == nil {
				t.Errorf("limit %d accepted", limit)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
