package txn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

var t0 = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

const tokA = "acct_0123456789abcdef0123456789abcdef"
const tokB = "acct_fedcba9876543210fedcba9876543210"

func core(ref string) Core {
	return Core{InstructionRef: ref, Channel: "MOBILE", Rail: "NIP", AmountMinor: 1850000, Currency: "NGN",
		DebitAccountToken: tokA, BeneficiaryBank: "TEST BANK", BeneficiaryToken: tokB, SessionID: "SESS-" + ref}
}

func env(ref, typ, evRef string, at time.Time) Envelope {
	return Envelope{SourceSystem: "SWITCH_A", SourceEventRef: evRef, EventType: typ, SourceTS: at, Txn: core(ref)}
}

// ---- amounts ---------------------------------------------------------------

func TestAmountIsADecimalStringOfMinorUnits(t *testing.T) {
	var a Amount
	if err := json.Unmarshal([]byte(`"1850000"`), &a); err != nil || a != 1850000 {
		t.Fatalf("%v %v", a, err)
	}
	if b, _ := json.Marshal(Amount(42)); string(b) != `"42"` {
		t.Fatalf("marshal: %s", b)
	}
	var bare Amount
	if err := json.Unmarshal([]byte(`1850000`), &bare); err != nil || bare != 1850000 {
		t.Fatalf("a bare integer, as the event contract shows, must be read exactly: %v %v", bare, err)
	}
	var big Amount
	if err := json.Unmarshal([]byte(`9000000000000000`), &big); err != nil || big != Amount(MaxAmountMinor) {
		t.Fatalf("a bare integer above 2^53 must stay exact: %v %v", big, err)
	}
	for _, bad := range []string{`1850000.5`, `1850000.0`, `1e6`, `+5`, `-5`, `007`, `"1850.00"`, `"-5"`, `"+5"`, `"0"`, `"007"`, `""`, `"1e6"`, `" 5"`, `"99999999999999999"`, `"9000000000000001"`, `null`, `true`} {
		var x Amount
		if err := json.Unmarshal([]byte(bad), &x); err == nil {
			t.Errorf("%s accepted as %d", bad, x)
		}
	}
	if n, err := ParseAmount("9000000000000000"); err != nil || n != MaxAmountMinor {
		t.Errorf("the maximum: %d %v", n, err)
	}
}

// ---- validation ------------------------------------------------------------

func TestEnvelopeValidation(t *testing.T) {
	good := env("INS-0001", EvDebitPosted, "EV-0001", t0)
	if err := good.Validate(t0); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Envelope){
		"lower case source":      func(e *Envelope) { e.SourceSystem = "switch a" },
		"short event ref":        func(e *Envelope) { e.SourceEventRef = "x" },
		"unknown event type":     func(e *Envelope) { e.EventType = "MONEY_APPEARED" },
		"no timestamp":           func(e *Envelope) { e.SourceTS = time.Time{} },
		"future timestamp":       func(e *Envelope) { e.SourceTS = t0.Add(time.Hour) },
		"ancient timestamp":      func(e *Envelope) { e.SourceTS = t0.AddDate(-3, 0, 0) },
		"bad instruction ref":    func(e *Envelope) { e.Txn.InstructionRef = "a b" },
		"unknown channel":        func(e *Envelope) { e.Txn.Channel = "FAX" },
		"bad rail":               func(e *Envelope) { e.Txn.Rail = "n" },
		"zero amount":            func(e *Envelope) { e.Txn.AmountMinor = 0 },
		"huge amount":            func(e *Envelope) { e.Txn.AmountMinor = Amount(MaxAmountMinor + 1) },
		"bad currency":           func(e *Envelope) { e.Txn.Currency = "naira" },
		"a raw account number":   func(e *Envelope) { e.Txn.DebitAccountToken = "0123456789" },
		"bad beneficiary token":  func(e *Envelope) { e.Txn.BeneficiaryToken = "0123456789" },
		"odd bank characters":    func(e *Envelope) { e.Txn.BeneficiaryBank = "<script>" },
		"odd session characters": func(e *Envelope) { e.Txn.SessionID = "a b" },
	}
	for name, mutate := range cases {
		e := good
		mutate(&e)
		if err := e.Validate(t0); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestHashIgnoresNothingThatMattersAndIsStable(t *testing.T) {
	a := env("INS-0001", EvDebitPosted, "EV-0001", t0)
	b := a
	if a.Hash() != b.Hash() {
		t.Fatal("hash is not stable")
	}
	b.Txn.AmountMinor++
	if a.Hash() == b.Hash() {
		t.Fatal("a different amount hashed the same")
	}
	c := a
	c.SourceTS = a.SourceTS.In(time.FixedZone("WAT", 3600))
	if a.Hash() != c.Hash() {
		t.Fatal("the same instant in another zone hashed differently")
	}
}

// ---- projection ------------------------------------------------------------

func seen(types ...string) []Seen {
	out := make([]Seen, len(types))
	for i, ty := range types {
		out[i] = Seen{Type: ty, At: t0.Add(time.Duration(i) * time.Second)}
	}
	return out
}

func TestProjectionStates(t *testing.T) {
	cases := []struct {
		name   string
		events []string
		state  string
		credit string
	}{
		{"nothing", nil, StateInitiated, CreditNone},
		{"debit only", []string{EvDebitPosted}, StateDebited, CreditNone},
		{"sent and waiting", []string{EvDebitPosted, EvTransferSent}, StatePending, CreditPending},
		{"success", []string{EvDebitPosted, EvTransferSent, EvCreditConfirmed}, StateConfirmedSuccess, CreditConfirmed},
		{"confirmed failure", []string{EvDebitPosted, EvTransferSent, EvCreditFailed}, StateConfirmedFailure, CreditFailed},
		{"a timeout is unknown, not failed", []string{EvDebitPosted, EvTransferSent, EvTransferTimeout}, StateUnknown, CreditUnknown},
		{"late success after a timeout", []string{EvDebitPosted, EvTransferSent, EvTransferTimeout, EvCreditConfirmed}, StateConfirmedSuccess, CreditConfirmed},
		{"failure after a timeout", []string{EvDebitPosted, EvTransferTimeout, EvCreditFailed}, StateConfirmedFailure, CreditFailed},
		{"success and failure together conflict", []string{EvDebitPosted, EvCreditConfirmed, EvCreditFailed}, StateConflicting, CreditConflicting},
		{"reversed after failure", []string{EvDebitPosted, EvCreditFailed, EvReversalPosted}, StateReversed, CreditFailed},
		{"a reversal of a success is not a reversal state", []string{EvDebitPosted, EvCreditConfirmed, EvReversalPosted}, StateConfirmedSuccess, CreditConfirmed},
		{"credit evidence without a debit", []string{EvCreditFailed}, StateInitiated, CreditFailed},
	}
	for _, c := range cases {
		p := Project(seen(c.events...))
		if p.Lifecycle != c.state || p.CreditStatus != c.credit {
			t.Errorf("%s: %s / %s, want %s / %s", c.name, p.Lifecycle, p.CreditStatus, c.state, c.credit)
		}
	}
	p := Project(seen(EvDebitPosted, EvCreditFailed, EvSettlementCompleted, EvReversalPosted))
	if p.SettlementState != SettlementCompleted || p.ReversalStatus != ReversalPosted || p.DebitStatus != DebitPosted || p.EventCount != 4 {
		t.Errorf("flags: %+v", p)
	}
}

func permutations(a []Seen) [][]Seen {
	if len(a) <= 1 {
		return [][]Seen{append([]Seen(nil), a...)}
	}
	var out [][]Seen
	for i := range a {
		rest := append(append([]Seen(nil), a[:i]...), a[i+1:]...)
		for _, p := range permutations(rest) {
			out = append(out, append([]Seen{a[i]}, p...))
		}
	}
	return out
}

func TestProjectionDoesNotDependOnArrivalOrder(t *testing.T) {
	events := seen(EvDebitPosted, EvTransferSent, EvTransferTimeout, EvCreditFailed, EvSettlementCompleted, EvReversalPosted)
	want := Project(events)
	for i, perm := range permutations(events) {
		got := Project(perm)
		if got.Lifecycle != want.Lifecycle || got.CreditStatus != want.CreditStatus || got.SettlementState != want.SettlementState ||
			got.ReversalStatus != want.ReversalStatus || !got.FirstEventAt.Equal(want.FirstEventAt) || !got.LastEventAt.Equal(want.LastEventAt) ||
			(got.FirstDebitAt == nil) != (want.FirstDebitAt == nil) || !got.FirstDebitAt.Equal(*want.FirstDebitAt) {
			t.Fatalf("permutation %d changed the result: %+v vs %+v", i, got, want)
		}
	}
}

// ---- ingestion against PostgreSQL -----------------------------------------

type rig struct {
	pool   *db.Pool
	svc    *Service
	tenant uuid.UUID
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := dbtest.Pool(t)
	return &rig{pool: pool, svc: &Service{Now: func() time.Time { return t0.Add(time.Hour) }}, tenant: dbtest.Tenant(t, pool)}
}

func (r *rig) ingest(t *testing.T, e Envelope) (Result, error) {
	t.Helper()
	var res Result
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		res, err = r.svc.Ingest(context.Background(), tx, r.tenant, e)
		return err
	})
	return res, err
}

func (r *rig) row(t *testing.T, ref string) (state, credit string, events int) {
	t.Helper()
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT lifecycle_state, credit_status, event_count FROM transactions WHERE instruction_ref = $1`, ref).Scan(&state, &credit, &events)
	})
	if err != nil {
		t.Fatal(err)
	}
	return
}

func (r *rig) outboxCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events WHERE tenant_id = $1 AND stream = $2`, r.tenant, Stream).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEventsBuildATransactionStepByStep(t *testing.T) {
	r := newRig(t)
	steps := []struct{ typ, ref, state string }{
		{EvDebitPosted, "EV-1", StateDebited},
		{EvTransferSent, "EV-2", StatePending},
		{EvCreditFailed, "EV-3", StateConfirmedFailure},
	}
	for i, s := range steps {
		res, err := r.ingest(t, env("INS-0001", s.typ, s.ref, t0.Add(time.Duration(i)*time.Second)))
		if err != nil || res.Outcome != Accepted || res.Lifecycle != s.state {
			t.Fatalf("step %d: %+v %v", i, res, err)
		}
	}
	if state, credit, n := r.row(t, "INS-0001"); state != StateConfirmedFailure || credit != CreditFailed || n != 3 {
		t.Fatalf("%s %s %d", state, credit, n)
	}
	if r.outboxCount(t) != 3 {
		t.Fatalf("outbox has %d messages, want one per accepted event", r.outboxCount(t))
	}
}

func TestRepeatedEventsAreIdempotent(t *testing.T) {
	r := newRig(t)
	e := env("INS-0002", EvDebitPosted, "EV-1", t0)
	first, err := r.ingest(t, e)
	if err != nil || first.Outcome != Accepted {
		t.Fatal(first, err)
	}
	for i := 0; i < 3; i++ {
		again, err := r.ingest(t, e)
		if err != nil || again.Outcome != Duplicate || again.EventID != first.EventID || again.TxnID != first.TxnID {
			t.Fatalf("repeat %d: %+v %v", i, again, err)
		}
	}
	if _, _, n := r.row(t, "INS-0002"); n != 1 {
		t.Fatalf("%d events stored", n)
	}
	if r.outboxCount(t) != 1 {
		t.Fatalf("repeats published %d messages", r.outboxCount(t))
	}
}

func TestSameReferenceWithDifferentContentIsAConflict(t *testing.T) {
	r := newRig(t)
	e := env("INS-0003", EvDebitPosted, "EV-1", t0)
	if _, err := r.ingest(t, e); err != nil {
		t.Fatal(err)
	}
	changed := e
	changed.EventType = EvCreditFailed
	if _, err := r.ingest(t, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict not reported: %v", err)
	}
	if state, _, n := r.row(t, "INS-0003"); state != StateDebited || n != 1 {
		t.Fatalf("the conflicting event changed state: %s %d", state, n)
	}
}

func TestAnEventCannotRewriteTheCoreFactsOfATransaction(t *testing.T) {
	r := newRig(t)
	if _, err := r.ingest(t, env("INS-0004", EvDebitPosted, "EV-1", t0)); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Envelope){
		"amount":   func(e *Envelope) { e.Txn.AmountMinor = 1 },
		"currency": func(e *Envelope) { e.Txn.Currency = "USD" },
		"account":  func(e *Envelope) { e.Txn.DebitAccountToken = tokB },
		"rail":     func(e *Envelope) { e.Txn.Rail = "POS" },
	} {
		e := env("INS-0004", EvTransferSent, "EV-2-"+name, t0.Add(time.Second))
		mutate(&e)
		if _, err := r.ingest(t, e); !errors.Is(err, ErrCoreMismatch) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, n := r.row(t, "INS-0004"); n != 1 {
		t.Fatalf("rejected events were stored: %d", n)
	}
}

func TestLateAndOutOfOrderEventsConverge(t *testing.T) {
	r := newRig(t)
	// The credit failure arrives before the debit that explains it.
	order := []Envelope{
		env("INS-0005", EvCreditFailed, "EV-3", t0.Add(2*time.Second)),
		env("INS-0005", EvDebitPosted, "EV-1", t0),
		env("INS-0005", EvTransferTimeout, "EV-2", t0.Add(time.Second)),
	}
	for _, e := range order {
		if _, err := r.ingest(t, e); err != nil {
			t.Fatal(err)
		}
	}
	if state, credit, _ := r.row(t, "INS-0005"); state != StateConfirmedFailure || credit != CreditFailed {
		t.Fatalf("%s %s", state, credit)
	}
}

func TestATimeoutNeverBecomesAFailure(t *testing.T) {
	r := newRig(t)
	for i, typ := range []string{EvDebitPosted, EvTransferSent, EvTransferTimeout} {
		if _, err := r.ingest(t, env("INS-0006", typ, fmt.Sprintf("EV-%d", i), t0.Add(time.Duration(i)*time.Second))); err != nil {
			t.Fatal(err)
		}
	}
	if state, credit, _ := r.row(t, "INS-0006"); state != StateUnknown || credit != CreditUnknown {
		t.Fatalf("%s %s", state, credit)
	}
}

func TestConcurrentEventsForOneTransactionAreAllKept(t *testing.T) {
	r := newRig(t)
	types := []string{EvDebitPosted, EvTransferSent, EvCreditFailed, EvSettlementCompleted, EvTransferTimeout, EvReversalPosted}
	var wg sync.WaitGroup
	errs := make(chan error, len(types))
	for i, ty := range types {
		wg.Add(1)
		go func(i int, ty string) {
			defer wg.Done()
			_, err := r.ingest(t, env("INS-0007", ty, fmt.Sprintf("EV-%d", i), t0.Add(time.Duration(i)*time.Second)))
			errs <- err
		}(i, ty)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ingest failed: %v", err)
		}
	}
	if state, _, n := r.row(t, "INS-0007"); n != len(types) || state != StateReversed {
		t.Fatalf("state %s with %d events, want REVERSED with %d", state, n, len(types))
	}
}

func TestEventsAreTenantIsolatedAndAppendOnly(t *testing.T) {
	a, b := newRig(t), newRig(t)
	if _, err := a.ingest(t, env("INS-0008", EvDebitPosted, "EV-1", t0)); err != nil {
		t.Fatal(err)
	}
	// The same references in another tenant are independent.
	if res, err := b.ingest(t, env("INS-0008", EvDebitPosted, "EV-1", t0)); err != nil || res.Outcome != Accepted {
		t.Fatalf("tenant B: %+v %v", res, err)
	}
	var n int
	_ = b.pool.WithTenant(context.Background(), b.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM transactions`).Scan(&n)
	})
	if n != 1 {
		t.Fatalf("tenant B sees %d transactions", n)
	}
	for _, stmt := range []string{`UPDATE transaction_events SET event_type = 'CREDIT_CONFIRMED'`, `DELETE FROM transaction_events`} {
		if err := a.pool.WithTenant(context.Background(), a.tenant, func(tx pgx.Tx) error { _, err := tx.Exec(context.Background(), stmt); return err }); err == nil {
			t.Errorf("tenant role ran %q", stmt)
		}
		if _, err := a.pool.Exec(context.Background(), stmt); err == nil {
			t.Errorf("owner ran %q", stmt)
		}
	}
}

func TestInvalidEventsAreQuarantinedNotStored(t *testing.T) {
	r := newRig(t)
	bad := env("INS-0009", EvDebitPosted, "EV-1", t0)
	bad.Txn.DebitAccountToken = "0123456789"
	if _, err := r.ingest(t, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("%v", err)
	}
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		return r.svc.Quarantine(context.Background(), tx, r.tenant, "SWITCH_A", strings.Repeat("r", 900), bad.Hash())
	})
	if err != nil {
		t.Fatal(err)
	}
	var q, txns int
	_ = r.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM quarantined_events WHERE tenant_id = $1), (SELECT count(*) FROM transactions WHERE tenant_id = $1)`, r.tenant).Scan(&q, &txns)
	if q != 1 || txns != 0 {
		t.Fatalf("quarantined %d, transactions %d", q, txns)
	}
}

// ---- external references ---------------------------------------------------

func (r *rig) refs(t *testing.T, ref string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT ref_type, ref_value FROM transaction_refs r JOIN transactions t ON t.tenant_id = r.tenant_id AND t.id = r.txn_id WHERE t.instruction_ref = $1`, ref)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				return err
			}
			out[k] = v
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExternalReferencesAreRecordedWithTheTransaction(t *testing.T) {
	r := newRig(t)
	e := env("INS-R001", EvDebitPosted, "EV-R1", t0)
	e.Refs = map[string]string{"RRN": "RRN000000777", "STAN": "000777"}
	if _, err := r.ingest(t, e); err != nil {
		t.Fatal(err)
	}
	later := env("INS-R001", EvTransferSent, "EV-R2", t0.Add(time.Second))
	later.Refs = map[string]string{"GATEWAY_REF": "GW-777"}
	if _, err := r.ingest(t, later); err != nil {
		t.Fatal(err)
	}
	got := r.refs(t, "INS-R001")
	want := map[string]string{"RRN": "RRN000000777", "STAN": "000777", "GATEWAY_REF": "GW-777", "SESSION_ID": "SESS-INS-R001"}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	// A repeat adds nothing.
	if _, err := r.ingest(t, e); err != nil || len(r.refs(t, "INS-R001")) != 4 {
		t.Fatalf("%v %v", err, r.refs(t, "INS-R001"))
	}
}

func TestReferencesAreValidatedAndPartOfTheEventsIdentity(t *testing.T) {
	good := env("INS-R002", EvDebitPosted, "EV-R3", t0)
	good.Refs = map[string]string{"RRN": "RRN1"}
	if err := good.Validate(t0); err != nil {
		t.Fatal(err)
	}
	for name, refs := range map[string]map[string]string{
		"unknown type": {"PAN": "4111"}, "empty value": {"RRN": ""}, "odd characters": {"RRN": "a b"}, "too long": {"RRN": string(make([]byte, 65))},
	} {
		e := good
		e.Refs = refs
		if err := e.Validate(t0); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	other := good
	other.Refs = map[string]string{"RRN": "RRN2"}
	if good.Hash() == other.Hash() {
		t.Fatal("different references hashed the same")
	}
	none := env("INS-R002", EvDebitPosted, "EV-R3", t0)
	if none.Hash() == good.Hash() {
		t.Fatal("an event with references hashed like one without")
	}
}

func TestTheSameEventDeliveredConcurrentlyIsStoredOnceAndReportedAsADuplicate(t *testing.T) {
	r := newRig(t)
	e := env("INS-R003", EvDebitPosted, "EV-R9", t0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted, dup := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := r.ingest(t, e)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				t.Error(err)
			case res.Outcome == Accepted:
				accepted++
			default:
				dup++
			}
		}()
	}
	wg.Wait()
	if accepted != 1 || dup != 7 {
		t.Fatalf("accepted=%d duplicate=%d", accepted, dup)
	}
	if _, _, n := r.row(t, "INS-R003"); n != 1 || r.outboxCount(t) != 1 {
		t.Fatalf("events=%d outbox=%d", n, r.outboxCount(t))
	}
}

func TestRefsAreReadAsAListOrAnObjectAndAlwaysWrittenAsASortedList(t *testing.T) {
	var list, obj Envelope
	base := `{"source_system":"SWITCH_A","source_event_ref":"EV-R100","event_type":"DEBIT_POSTED","source_ts":"2026-10-15T09:00:00Z","txn":{"instruction_ref":"INS-R100","channel":"MOBILE","rail":"NIP","amount_minor":1850000,"currency":"NGN","debit_account_token":"acct_0123456789abcdef0123456789abcdef"},"refs":`
	if err := json.Unmarshal([]byte(base+`[{"type":"STAN","value":"000123"},{"type":"RRN","value":"RRN000000123"}]}`), &list); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(base+`{"RRN":"RRN000000123","STAN":"000123"}}`), &obj); err != nil {
		t.Fatal(err)
	}
	if list.Hash() != obj.Hash() {
		t.Fatal("the same references hashed differently by form")
	}
	out, _ := json.Marshal(list)
	if !strings.Contains(string(out), `"refs":[{"type":"RRN","value":"RRN000000123"},{"type":"STAN","value":"000123"}]`) {
		t.Fatalf("not a sorted list: %s", out)
	}
	for name, doc := range map[string]string{
		"type given twice": `[{"type":"RRN","value":"A1"},{"type":"RRN","value":"B2"}]`,
		"unknown key":      `[{"type":"RRN","value":"A1","x":1}]`,
		"a string":         `"RRN"`,
		"a number":         `5`,
	} {
		var e Envelope
		if err := json.Unmarshal([]byte(base+doc+`}`), &e); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestOptionalContractFieldsAreValidated(t *testing.T) {
	good := env("INS-C001", EvDebitPosted, "EV-C001", t0)
	seq := int64(7)
	good.Seq, good.InstitutionID, good.RawPayloadHash = &seq, "INST-DEMO-01", "sha256:7c1e0000"
	if err := good.Validate(t0); err != nil {
		t.Fatal(err)
	}
	neg := int64(-1)
	for name, mut := range map[string]func(*Envelope){
		"negative seq":    func(e *Envelope) { e.Seq = &neg },
		"odd institution": func(e *Envelope) { e.InstitutionID = "a b" },
		"short raw hash":  func(e *Envelope) { e.RawPayloadHash = "abc" },
		"odd raw hash":    func(e *Envelope) { e.RawPayloadHash = "sha256:7c1e 0000" },
	} {
		e := good
		mut(&e)
		if err := e.Validate(t0); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	other := good
	other.RawPayloadHash = "sha256:aaaa0000"
	if good.Hash() == other.Hash() {
		t.Fatal("a different raw payload hash is a different event")
	}
}
