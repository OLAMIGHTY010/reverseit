package correlate

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

var t0 = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

const tokA = "acct_0123456789abcdef0123456789abcdef"

type rig struct {
	pool   *db.Pool
	m      *Matcher
	tenant uuid.UUID
	t      *testing.T
}

func newRig(t *testing.T) *rig {
	pool := dbtest.Pool(t)
	svc := &txn.Service{Now: func() time.Time { return t0.Add(time.Hour) }}
	return &rig{pool: pool, m: &Matcher{Txn: svc}, tenant: dbtest.Tenant(t, pool), t: t}
}

func (r *rig) in(fn func(tx pgx.Tx) error) error {
	return r.pool.WithTenant(context.Background(), r.tenant, fn)
}

// core ingests the first event of a transaction as our own system would, with the
// references the switch later quotes.
func (r *rig) core(instr string, amount txn.Amount, refs map[string]string, at time.Time) uuid.UUID {
	r.t.Helper()
	var id uuid.UUID
	err := r.in(func(tx pgx.Tx) error {
		res, err := r.m.Txn.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "CORE", SourceEventRef: "CORE-" + instr, EventType: txn.EvDebitPosted, SourceTS: at,
			Txn:  txn.Core{InstructionRef: instr, Channel: "MOBILE", Rail: "NIP", AmountMinor: amount, Currency: "NGN", DebitAccountToken: tokA},
			Refs: refs})
		id = res.TxnID
		return err
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return id
}

func obs(ref string, refs map[string]string, amount txn.Amount, at time.Time) Observation {
	return Observation{SourceSystem: "SWITCH_X", SourceEventRef: ref, EventType: txn.EvCreditConfirmed, SourceTS: at, Refs: refs, AmountMinor: amount, Currency: "NGN"}
}

func (r *rig) resolve(o Observation) (Decision, error) {
	r.t.Helper()
	var d Decision
	err := r.in(func(tx pgx.Tx) error {
		var err error
		d, err = r.m.Resolve(context.Background(), tx, r.tenant, o)
		return err
	})
	return d, err
}

func (r *rig) events(instr string) (n int, state string) {
	r.t.Helper()
	if err := r.in(func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT event_count, lifecycle_state FROM transactions WHERE instruction_ref = $1`, instr).Scan(&n, &state)
	}); err != nil {
		r.t.Fatal(err)
	}
	return
}

func (r *rig) open() []Item {
	r.t.Helper()
	var out []Item
	if err := r.in(func(tx pgx.Tx) error {
		var err error
		out, err = List(context.Background(), tx, r.tenant, "", 50)
		return err
	}); err != nil {
		r.t.Fatal(err)
	}
	return out
}

func TestStrongReferencesLinkAutomaticallyAndTheEventProjects(t *testing.T) {
	r := newRig(t)
	// One transaction per scheme, so each rule is tested on its own.
	cases := []struct {
		name string
		refs map[string]string
	}{
		{"gw", map[string]string{"GATEWAY_REF": "GW-A"}},
		{"ledger", map[string]string{"LEDGER_ID": "LG-B"}},
		{"rrnstan", map[string]string{"RRN": "RRN-C", "STAN": "000333"}},
		{"session", map[string]string{"SESSION_ID": "SESS-D"}},
	}
	for _, c := range cases {
		id := r.core("INS-"+c.name, 1850000, c.refs, t0)
		d, err := r.resolve(obs("OB-"+c.name, c.refs, 1850000, t0.Add(5*time.Second)))
		if err != nil || !d.Linked || d.TxnID != id || d.Confidence < 0.9 || len(d.Basis) == 0 {
			t.Fatalf("%s: %+v %v", c.name, d, err)
		}
		if n, state := r.events("INS-" + c.name); n != 2 || state != txn.StateConfirmedSuccess {
			t.Fatalf("%s: events=%d state=%s", c.name, n, state)
		}
	}
	if len(r.open()) != 0 {
		t.Fatal("confident links were left in the open queue")
	}
}

func TestWeakOrDisagreeingEvidenceIsQueuedNotGuessed(t *testing.T) {
	r := newRig(t)
	r.core("INS-W1", 1850000, map[string]string{"RRN": "RRN-W", "STAN": "000444", "LEDGER_ID": "LG-W"}, t0)
	cases := map[string]Observation{
		"RRN alone":      obs("OB-W1", map[string]string{"RRN": "RRN-W"}, 1850000, t0.Add(time.Second)),
		"STAN alone":     obs("OB-W2", map[string]string{"STAN": "000444"}, 1850000, t0.Add(time.Second)),
		"amount differs": obs("OB-W3", map[string]string{"LEDGER_ID": "LG-W"}, 1850001, t0.Add(time.Second)),
		"currency differs": func() Observation {
			o := obs("OB-W4", map[string]string{"LEDGER_ID": "LG-W"}, 1850000, t0.Add(time.Second))
			o.Currency = "USD"
			return o
		}(),
		"a week later":        obs("OB-W5", map[string]string{"LEDGER_ID": "LG-W"}, 1850000, t0.Add(7*24*time.Hour)),
		"no shared reference": obs("OB-W6", map[string]string{"LEDGER_ID": "LG-OTHER"}, 1850000, t0.Add(time.Second)),
	}
	for name, o := range cases {
		d, err := r.resolve(o)
		if err != nil || d.Linked || d.Reason == "" || d.Queued == uuid.Nil {
			t.Errorf("%s: %+v %v", name, d, err)
		}
	}
	if n, _ := r.events("INS-W1"); n != 1 {
		t.Fatalf("weak evidence reached the transaction: %d events", n)
	}
	if len(r.open()) != len(cases) {
		t.Fatalf("%d queued", len(r.open()))
	}
}

func TestAReusedReferenceMatchingTwoTransactionsIsAmbiguous(t *testing.T) {
	r := newRig(t)
	same := map[string]string{"RRN": "RRN-R", "STAN": "000555"}
	r.core("INS-R1", 1850000, same, t0)
	r.core("INS-R2", 1850000, same, t0.Add(time.Hour))
	d, err := r.resolve(obs("OB-R1", same, 1850000, t0.Add(time.Hour+time.Second)))
	if err != nil || d.Linked || len(d.Candidates) != 2 || d.Reason == "" {
		t.Fatalf("%+v %v", d, err)
	}
	for _, instr := range []string{"INS-R1", "INS-R2"} {
		if n, _ := r.events(instr); n != 1 {
			t.Fatalf("%s took evidence it could not be sure of", instr)
		}
	}
}

func TestEvidenceThatArrivesBeforeItsTransactionIsLinkedWhenItAppears(t *testing.T) {
	r := newRig(t)
	refs := map[string]string{"GATEWAY_REF": "GW-LATE"}
	d, err := r.resolve(obs("OB-L1", refs, 1850000, t0.Add(5*time.Second)))
	if err != nil || d.Linked {
		t.Fatalf("%+v %v", d, err)
	}
	var linked int
	if err := r.in(func(tx pgx.Tx) error {
		var err error
		linked, err = r.m.Retry(context.Background(), tx, r.tenant)
		return err
	}); err != nil || linked != 0 {
		t.Fatalf("%d %v", linked, err)
	}
	id := r.core("INS-L1", 1850000, refs, t0)
	if err := r.in(func(tx pgx.Tx) error {
		var err error
		linked, err = r.m.Retry(context.Background(), tx, r.tenant)
		return err
	}); err != nil || linked != 1 {
		t.Fatalf("%d %v", linked, err)
	}
	if n, state := r.events("INS-L1"); n != 2 || state != txn.StateConfirmedSuccess || len(r.open()) != 0 {
		t.Fatalf("%d %s", n, state)
	}
	var got *uuid.UUID
	_ = r.in(func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT linked_txn_id FROM evidence_correlations WHERE source_event_ref = 'OB-L1'`).Scan(&got)
	})
	if got == nil || *got != id {
		t.Fatal("the link was not recorded")
	}
}

func TestRepeatingAnObservationChangesNothingAndReusingItsReferenceConflicts(t *testing.T) {
	r := newRig(t)
	refs := map[string]string{"GATEWAY_REF": "GW-REP"}
	r.core("INS-P1", 1850000, refs, t0)
	o := obs("OB-P1", refs, 1850000, t0.Add(time.Second))
	first, err := r.resolve(o)
	if err != nil || !first.Linked {
		t.Fatalf("%+v %v", first, err)
	}
	again, err := r.resolve(o)
	if err != nil || !again.Duplicate || !again.Linked || again.TxnID != first.TxnID {
		t.Fatalf("%+v %v", again, err)
	}
	if n, _ := r.events("INS-P1"); n != 2 {
		t.Fatalf("%d events", n)
	}
	changed := o
	changed.AmountMinor = 5
	if _, err := r.resolve(changed); !errors.Is(err, txn.ErrConflict) {
		t.Fatalf("%v", err)
	}
	for name, bad := range map[string]Observation{
		"no refs":     {SourceSystem: "S", SourceEventRef: "OB-X1", EventType: txn.EvCreditConfirmed, SourceTS: t0, AmountMinor: 5, Currency: "NGN"},
		"unknown ref": {SourceSystem: "S", SourceEventRef: "OB-X2", EventType: txn.EvCreditConfirmed, SourceTS: t0, AmountMinor: 5, Currency: "NGN", Refs: map[string]string{"PAN": "1"}},
		"no amount":   {SourceSystem: "S", SourceEventRef: "OB-X3", EventType: txn.EvCreditConfirmed, SourceTS: t0, Currency: "NGN", Refs: map[string]string{"RRN": "1"}},
	} {
		if _, err := r.resolve(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestConcurrentDeliveriesOfOneObservationLinkOnce(t *testing.T) {
	r := newRig(t)
	refs := map[string]string{"GATEWAY_REF": "GW-CC"}
	r.core("INS-C1", 1850000, refs, t0)
	o := obs("OB-C1", refs, 1850000, t0.Add(time.Second))
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := r.resolve(o); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n, _ := r.events("INS-C1"); n != 2 {
		t.Fatalf("%d events", n)
	}
}

func TestAPersonCanLinkQueuedEvidenceOnlyWhenAmountAndCurrencyAgree(t *testing.T) {
	r := newRig(t)
	id := r.core("INS-M1", 1850000, map[string]string{"RRN": "RRN-M"}, t0)
	d, _ := r.resolve(obs("OB-M1", map[string]string{"RRN": "RRN-M"}, 1850000, t0.Add(time.Second))) // RRN alone: queued
	bad, _ := r.resolve(obs("OB-M2", map[string]string{"RRN": "RRN-M"}, 99, t0.Add(2*time.Second)))
	if d.Linked || bad.Linked {
		t.Fatal("expected both queued")
	}
	me := uuid.New()
	err := r.in(func(tx pgx.Tx) error {
		_, err := r.m.ManualLink(context.Background(), tx, r.tenant, bad.Queued, id, me, "looks right")
		return err
	})
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("a disagreeing amount was linked: %v", err)
	}
	if err := r.in(func(tx pgx.Tx) error {
		_, err := r.m.ManualLink(context.Background(), tx, r.tenant, d.Queued, id, me, "RRN and amount agree, confirmed with the switch team")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if n, state := r.events("INS-M1"); n != 2 || state != txn.StateConfirmedSuccess {
		t.Fatalf("%d %s", n, state)
	}
	var method string
	var by *uuid.UUID
	_ = r.in(func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT link_method, resolved_by FROM evidence_correlations WHERE id = $1`, d.Queued).Scan(&method, &by)
	})
	if method != "manual" || by == nil || *by != me {
		t.Fatalf("%s %v", method, by)
	}
	if err := r.in(func(tx pgx.Tx) error {
		_, err := r.m.ManualLink(context.Background(), tx, r.tenant, d.Queued, id, me, "again")
		return err
	}); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("%v", err)
	}
	if err := r.in(func(tx pgx.Tx) error {
		return Discard(context.Background(), tx, r.tenant, bad.Queued, me, "wrong amount, source error")
	}); err != nil {
		t.Fatal(err)
	}
	if len(r.open()) != 0 {
		t.Fatal("queue should be empty")
	}
	if err := r.in(func(tx pgx.Tx) error { return Discard(context.Background(), tx, r.tenant, uuid.New(), me, "x") }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
}

func TestOneInstitutionsReferencesNeverMatchAnothers(t *testing.T) {
	r := newRig(t)
	refs := map[string]string{"GATEWAY_REF": "GW-ISO"}
	r.core("INS-I1", 1850000, refs, t0)
	other := &rig{pool: r.pool, m: r.m, tenant: dbtest.Tenant(t, r.pool), t: t}
	d, err := other.resolve(obs("OB-I1", refs, 1850000, t0.Add(time.Second)))
	if err != nil || d.Linked || len(d.Candidates) != 0 {
		t.Fatalf("%+v %v", d, err)
	}
	if n, _ := r.events("INS-I1"); n != 1 {
		t.Fatal("evidence crossed institutions")
	}
}
