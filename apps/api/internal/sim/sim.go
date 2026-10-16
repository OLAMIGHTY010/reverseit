// Package sim generates synthetic transaction evidence with known answers.
//
// Every run is a pure function of a seed: the same seed gives byte-for-byte the same
// events and the same ground-truth labels, on any machine. Each scenario says what
// the evidence is and what a correct system should conclude from it, so detection can
// be scored against labels written before the detector ran. Nothing here is real
// customer data, and nothing here moves money.
//
// The labels are domain expectations (for example "a timeout is never a failure"),
// not a record of what the detector currently does. Where the detector disagrees,
// the disagreement is a finding to investigate, not a label to edit.
package sim

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Sources the scenarios send from.
const (
	SrcSwitch = "SWITCH_A"
	SrcCore   = "CORE_LEDGER"
	SrcOther  = "CORE_B"
)

// DefaultStart is the simulated clock's start, a fixed instant so runs are
// reproducible. It is in the past relative to any real run.
var DefaultStart = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

// TimedEvent is one event and the moment it reaches the platform. SourceTS is on the
// envelope; Arrive can be later (a delayed or replayed delivery) or reordered.
type TimedEvent struct {
	Arrive   time.Time    `json:"arrive"`
	Envelope txn.Envelope `json:"envelope"`
}

// Truth is the correct answer for one transaction.
type Truth struct {
	Scenario       string `json:"scenario"`
	InstructionRef string `json:"instruction_ref"`
	// FinalLifecycle is the state the evidence supports once everything has arrived.
	FinalLifecycle string `json:"final_lifecycle"`
	// Findings are the exception codes a correct detector opens for this transaction
	// when events arrive in order. Empty means no exception.
	Findings []string `json:"findings"`
	// Note explains the label in plain words.
	Note string `json:"note"`
}

// Run is a generated set of events with its labels.
type Run struct {
	Seed   uint64       `json:"seed"`
	Start  time.Time    `json:"start"`
	Events []TimedEvent `json:"-"`
	Truth  []Truth      `json:"-"`
}

// Scenario is one named pattern of evidence.
type Scenario struct {
	Name        string
	Description string
	build       func(g *gen)
}

type gen struct {
	rng    *rand.Rand
	start  time.Time
	n      int
	events []TimedEvent
	truth  []Truth
}

func (g *gen) hex(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(g.rng.UintN(256))
	}
	return hex.EncodeToString(b)
}

// channel picks the channel for a transfer whose outcome does not depend on it. The
// detector classifies some failures by channel (USSD and gateway failures as vendor
// drops, for example), so scenarios about failures keep a fixed channel and the others
// vary, which exercises every channel the platform accepts without changing any label.
func (g *gen) channel() string {
	return []string{"MOBILE", "INTERNET", "USSD", "POS", "CARD"}[g.rng.UintN(5)]
}

func (g *gen) token() string { return "acct_" + g.hex(16) }

// amount is between 1,000.00 and 50,000.00 in minor units.
func (g *gen) amount() txn.Amount { return txn.Amount(100_000 + g.rng.Int64N(4_900_001)) }

// tx is one simulated transfer under construction.
type tx struct {
	g       *gen
	core    txn.Core
	at      time.Time
	refs    map[string]string
	scen    string
	counter int
}

// newTx makes a transfer with its own payer and payee, starting at at, so one
// transfer is never mistaken for a repeat of another unless a scenario means it to be.
func (g *gen) newTx(scen, rail, channel string, at time.Time) *tx {
	g.n++
	ref := fmt.Sprintf("SIM-%04d", g.n)
	return &tx{g: g, scen: scen, at: at, refs: map[string]string{"RRN": fmt.Sprintf("RRN%09d", g.n), "STAN": fmt.Sprintf("%06d", g.n)},
		core: txn.Core{InstructionRef: ref, Channel: channel, Rail: rail, AmountMinor: g.amount(), Currency: "NGN",
			DebitAccountToken: g.token(), BeneficiaryBank: "SIM BANK", BeneficiaryToken: g.token(), SessionID: "SES-" + ref}}
}

// emit adds an event offset seconds after the transfer started, from source, arriving
// after a small delay (deterministic jitter of up to two seconds).
func (t *tx) emit(source, typ string, offset time.Duration) *tx {
	t.counter++
	ts := t.at.Add(offset)
	delay := time.Duration(t.g.rng.Int64N(2000)) * time.Millisecond
	e := txn.Envelope{SourceSystem: source, SourceEventRef: fmt.Sprintf("%s-%s-%d", t.core.InstructionRef, typ, t.counter), EventType: typ, SourceTS: ts,
		Txn: t.core, Refs: t.refs}
	t.g.events = append(t.g.events, TimedEvent{Arrive: ts.Add(delay), Envelope: e})
	return t
}

func (t *tx) label(final string, note string, findings ...string) {
	if findings == nil {
		findings = []string{}
	}
	t.g.truth = append(t.g.truth, Truth{Scenario: t.scen, InstructionRef: t.core.InstructionRef, FinalLifecycle: final, Findings: findings, Note: note})
}

// clone makes a second transfer for the same payer, payee and amount (a repeat).
func (t *tx) clone(at time.Time) *tx {
	t.g.n++
	ref := fmt.Sprintf("SIM-%04d", t.g.n)
	c := *t
	c.core.InstructionRef = ref
	c.core.SessionID = "SES-" + ref
	c.at = at
	c.refs = map[string]string{"RRN": fmt.Sprintf("RRN%09d", t.g.n), "STAN": fmt.Sprintf("%06d", t.g.n)}
	c.counter = 0
	return &c
}

func s(n int) time.Duration { return time.Duration(n) * time.Second }
func m(n int) time.Duration { return time.Duration(n) * time.Minute }

// base is the usual start of a transfer: debit, then sent to the switch.
func (t *tx) begin() *tx {
	return t.emit(SrcCore, txn.EvDebitPosted, 0).emit(SrcSwitch, txn.EvTransferSent, s(1))
}

// Scenarios is the catalogue, in a stable order.
var Scenarios = []Scenario{
	{"normal_success", "debit, sent, credit confirmed within seconds", func(g *gen) {
		t := g.newTx("normal_success", "NIP", g.channel(), g.next())
		t.begin().emit(SrcSwitch, txn.EvCreditConfirmed, s(3))
		t.label(txn.StateConfirmedSuccess, "a confirmed credit is a success; nothing to raise")
	}},
	{"slow_success", "credit confirmed after two minutes, inside the waiting threshold", func(g *gen) {
		t := g.newTx("slow_success", "NIP", g.channel(), g.next())
		t.begin().emit(SrcSwitch, txn.EvCreditConfirmed, s(120))
		t.label(txn.StateConfirmedSuccess, "slow is not stuck: the answer arrived before the waiting threshold")
	}},
	{"failed_credit", "debit posted, beneficiary credit reported failed", func(g *gen) {
		t := g.newTx("failed_credit", "NIP", "MOBILE", g.next())
		t.begin().emit(SrcSwitch, txn.EvCreditFailed, s(3))
		t.label(txn.StateConfirmedFailure, "money left the customer and the credit failed", "FAIL001")
	}},
	{"switch_timeout", "debit posted, the switch never answered", func(g *gen) {
		t := g.newTx("switch_timeout", "NIP", g.channel(), g.next())
		t.begin().emit(SrcSwitch, txn.EvTransferTimeout, s(30))
		t.label(txn.StateUnknown, "a timeout is unknown, never a confirmed failure", "UNK001")
	}},
	{"timeout_then_late_success", "timeout, then the credit is confirmed ten minutes later", func(g *gen) {
		t := g.newTx("timeout_then_late_success", "NIP", g.channel(), g.next())
		t.begin().emit(SrcSwitch, txn.EvTransferTimeout, s(30)).emit(SrcSwitch, txn.EvCreditConfirmed, m(10))
		t.label(txn.StateConfirmedSuccess, "the late confirmation shows it succeeded; the unknown case is raised and then closed", "UNK001")
	}},
	{"timeout_then_failure", "timeout, then the credit is reported failed", func(g *gen) {
		t := g.newTx("timeout_then_failure", "NIP", "MOBILE", g.next())
		t.begin().emit(SrcSwitch, txn.EvTransferTimeout, s(30)).emit(SrcSwitch, txn.EvCreditFailed, m(10))
		t.label(txn.StateConfirmedFailure, "the later failure settles it", "UNK001", "FAIL001")
	}},
	{"duplicate_debit", "the same payer, payee and amount debited twice within seconds", func(g *gen) {
		a := g.newTx("duplicate_debit", "NIP", g.channel(), g.next())
		a.begin().emit(SrcSwitch, txn.EvCreditConfirmed, s(3))
		a.label(txn.StateConfirmedSuccess, "the first debit is legitimate")
		b := a.clone(a.at.Add(s(6)))
		b.begin().emit(SrcSwitch, txn.EvCreditConfirmed, s(3))
		b.label(txn.StateConfirmedSuccess, "a second identical debit seconds later is a duplicate; the case belongs to the later one", "DUP001")
	}},
	{"retry_after_failure", "the first attempt failed, the customer retried at once", func(g *gen) {
		a := g.newTx("retry_after_failure", "NIP", "MOBILE", g.next())
		a.begin().emit(SrcSwitch, txn.EvCreditFailed, s(3))
		a.label(txn.StateConfirmedFailure, "the first attempt failed", "FAIL001")
		b := a.clone(a.at.Add(s(10)))
		b.begin().emit(SrcSwitch, txn.EvCreditConfirmed, s(3))
		b.label(txn.StateConfirmedSuccess, "a retry after a confirmed failure is not a duplicate")
	}},
	{"repeat_outside_window", "the same payer, payee and amount ten minutes apart", func(g *gen) {
		a := g.newTx("repeat_outside_window", "NIP", g.channel(), g.next())
		a.begin().emit(SrcSwitch, txn.EvCreditConfirmed, s(3))
		a.label(txn.StateConfirmedSuccess, "ordinary payment")
		b := a.clone(a.at.Add(m(10)))
		b.begin().emit(SrcSwitch, txn.EvCreditConfirmed, s(3))
		b.label(txn.StateConfirmedSuccess, "a repeat well outside the duplicate window is a legitimate repeat payment")
	}},
	{"same_amount_other_payee", "the same payer and amount to two different payees seconds apart", func(g *gen) {
		a := g.newTx("same_amount_other_payee", "NIP", g.channel(), g.next())
		a.begin().emit(SrcSwitch, txn.EvCreditConfirmed, s(3))
		a.label(txn.StateConfirmedSuccess, "ordinary payment")
		b := a.clone(a.at.Add(s(6)))
		b.core.BeneficiaryToken = g.token()
		b.begin().emit(SrcSwitch, txn.EvCreditConfirmed, s(3))
		b.label(txn.StateConfirmedSuccess, "a different beneficiary means a different payment, not a duplicate")
	}},
	{"conflicting_evidence", "one source confirms the credit and another reports it failed", func(g *gen) {
		t := g.newTx("conflicting_evidence", "NIP", g.channel(), g.next())
		t.begin().emit(SrcSwitch, txn.EvCreditConfirmed, s(3)).emit(SrcOther, txn.EvCreditFailed, s(5))
		t.label(txn.StateConflicting, "sources disagree; that is raised as unknown and never resolved by guessing", "UNK001")
	}},
	{"stuck_pending", "sent to the switch, or debited and never sent, and never heard of again", func(g *gen) {
		t := g.newTx("stuck_pending", "NIP", "MOBILE", g.next())
		t.begin()
		t.label(txn.StatePending, "no answer past the rail's waiting threshold is raised as unknown by the periodic sweep", "UNK001")
		u := g.newTx("stuck_pending", "NIP", "MOBILE", g.next())
		u.emit(SrcCore, txn.EvDebitPosted, 0)
		u.label(txn.StateDebited, "a debit that was never sent is also stuck once past the threshold", "UNK001")
	}},
	{"atm_dispense_fault", "ATM debit posted, cash not dispensed", func(g *gen) {
		t := g.newTx("atm_dispense_fault", "ATM", "ATM", g.next())
		t.begin().emit(SrcSwitch, txn.EvCreditFailed, s(4))
		t.label(txn.StateConfirmedFailure, "cash was not dispensed after the debit", "ATM001")
	}},
	{"disputed_fee", "a fee debit that failed to be applied as authorised", func(g *gen) {
		t := g.newTx("disputed_fee", "FEE", "API", g.next())
		t.begin().emit(SrcSwitch, txn.EvCreditFailed, s(4))
		t.label(txn.StateConfirmedFailure, "a disputed fee", "FEE001")
	}},
	{"vendor_drop", "a biller aggregator failed to fulfil after the debit", func(g *gen) {
		t := g.newTx("vendor_drop", "VENDOR", "GATEWAY", g.next())
		t.begin().emit(SrcSwitch, txn.EvCreditFailed, s(4))
		t.label(txn.StateConfirmedFailure, "third party failed to fulfil", "VND001")
	}},
	{"amount_mismatch", "ledger and switch disagree on the amount", func(g *gen) {
		t := g.newTx("amount_mismatch", "MISMATCH", "API", g.next())
		t.begin().emit(SrcSwitch, txn.EvCreditFailed, s(4))
		t.label(txn.StateConfirmedFailure, "a discrepancy between sources on amount or currency", "MIS001")
	}},
	{"incomplete_routing", "an intermediary switch dropped the transfer before the beneficiary institution", func(g *gen) {
		t := g.newTx("incomplete_routing", "INTERCHANGE", "API", g.next())
		t.begin().emit(SrcSwitch, txn.EvCreditFailed, s(4))
		t.label(txn.StateConfirmedFailure, "multi-leg routing dropped the transfer", "INC001")
	}},
	{"delayed_reversal", "credit failed and the reversal posted twenty minutes later", func(g *gen) {
		t := g.newTx("delayed_reversal", "NIP", "MOBILE", g.next())
		t.begin().emit(SrcSwitch, txn.EvCreditFailed, s(3)).emit(SrcCore, txn.EvReversalPosted, m(20))
		t.label(txn.StateReversed, "the failure was raised; the reversal then posted", "FAIL001")
	}},
}

// next returns the start of the next scenario, a little after the previous.
func (g *gen) next() time.Time {
	return g.start.Add(time.Duration(g.n) * 25 * time.Second).Add(time.Duration(g.rng.Int64N(10)) * time.Second)
}

// Names lists the scenario names in order.
func Names() []string {
	out := make([]string, len(Scenarios))
	for i, sc := range Scenarios {
		out[i] = sc.Name
	}
	return out
}

func find(name string) (Scenario, bool) {
	for _, sc := range Scenarios {
		if sc.Name == name {
			return sc, true
		}
	}
	return Scenario{}, false
}

// Options choose what to generate.
type Options struct {
	Seed  uint64
	Start time.Time
	// Plan lists scenario names; a name may repeat. Empty means one of each.
	Plan []string
}

// Generate builds a run. The same options always give the same run.
func Generate(o Options) (Run, error) {
	if o.Start.IsZero() {
		o.Start = DefaultStart
	}
	plan := o.Plan
	if len(plan) == 0 {
		plan = Names()
	}
	g := &gen{rng: rand.New(rand.NewPCG(o.Seed, o.Seed^0x9e3779b97f4a7c15)), start: o.Start.UTC()}
	for _, name := range plan {
		sc, ok := find(name)
		if !ok {
			return Run{}, fmt.Errorf("sim: unknown scenario %q; known: %s", name, strings.Join(Names(), ", "))
		}
		sc.build(g)
	}
	r := Run{Seed: o.Seed, Start: g.start, Events: g.events, Truth: g.truth}
	r.sort()
	return r, nil
}

func (r *Run) sort() {
	sort.SliceStable(r.Events, func(i, j int) bool {
		a, b := r.Events[i], r.Events[j]
		if !a.Arrive.Equal(b.Arrive) {
			return a.Arrive.Before(b.Arrive)
		}
		return a.Envelope.SourceEventRef < b.Envelope.SourceEventRef
	})
}

// Demo50 is the fifty-transaction demonstration: a realistic mix of ordinary and
// faulty transfers with exactly one unknown (a switch timeout). It is deterministic
// for a seed.
func Demo50(seed uint64, start time.Time) (Run, error) {
	// Counting transactions: duplicate_debit, retry_after_failure, repeat_outside_window
	// and same_amount_other_payee make two each.
	var plan []string
	add := func(name string, n int) {
		for i := 0; i < n; i++ {
			plan = append(plan, name)
		}
	}
	// Transfers per entry: one each, except the four repeat scenarios which make two.
	add("normal_success", 24)         // 24
	add("slow_success", 3)            // 27
	add("failed_credit", 3)           // 30
	add("switch_timeout", 1)          // 31, the single unknown
	add("duplicate_debit", 2)         // 35
	add("retry_after_failure", 2)     // 39
	add("repeat_outside_window", 2)   // 43
	add("same_amount_other_payee", 1) // 45
	add("atm_dispense_fault", 1)      // 46
	add("disputed_fee", 1)            // 47
	add("vendor_drop", 1)             // 48
	add("amount_mismatch", 1)         // 49
	add("delayed_reversal", 1)        // 50
	return Generate(Options{Seed: seed, Start: start, Plan: plan})
}

// EventsJSONL writes one event per line.
func (r Run) EventsJSONL() []byte {
	var b strings.Builder
	for _, e := range r.Events {
		line, _ := json.Marshal(e)
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// TruthJSONL writes one label per line, ordered by instruction reference.
func (r Run) TruthJSONL() []byte {
	t := append([]Truth(nil), r.Truth...)
	sort.Slice(t, func(i, j int) bool { return t[i].InstructionRef < t[j].InstructionRef })
	var b strings.Builder
	for _, x := range t {
		line, _ := json.Marshal(x)
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// Transactions counts the labelled transactions.
func (r Run) Transactions() int { return len(r.Truth) }
