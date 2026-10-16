package sim

import (
	"bytes"
	"testing"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

func TestThereAreEighteenNamedScenariosEachWithALabel(t *testing.T) {
	if len(Scenarios) != 18 {
		t.Fatalf("%d scenarios", len(Scenarios))
	}
	seen := map[string]bool{}
	for _, sc := range Scenarios {
		if seen[sc.Name] || sc.Description == "" {
			t.Errorf("duplicate or undescribed scenario %q", sc.Name)
		}
		seen[sc.Name] = true
		r, err := Generate(Options{Seed: 1, Plan: []string{sc.Name}})
		if err != nil || len(r.Truth) == 0 || len(r.Events) == 0 {
			t.Fatalf("%s: %v", sc.Name, err)
		}
		for _, tr := range r.Truth {
			if tr.Note == "" || tr.FinalLifecycle == "" || tr.Scenario != sc.Name {
				t.Errorf("%s: %+v", sc.Name, tr)
			}
		}
	}
}

func TestTheSameSeedGivesTheSameBytesAndADifferentSeedDoesNot(t *testing.T) {
	a, _ := Generate(Options{Seed: 42})
	b, _ := Generate(Options{Seed: 42})
	c, _ := Generate(Options{Seed: 43})
	if !bytes.Equal(a.EventsJSONL(), b.EventsJSONL()) || !bytes.Equal(a.TruthJSONL(), b.TruthJSONL()) {
		t.Fatal("the same seed gave different output")
	}
	if bytes.Equal(a.EventsJSONL(), c.EventsJSONL()) {
		t.Fatal("different seeds gave identical events")
	}
}

func TestEveryGeneratedEventIsValidAndReferencesAreUnique(t *testing.T) {
	r, err := Generate(Options{Seed: 7, Plan: append(Names(), Names()...)})
	if err != nil {
		t.Fatal(err)
	}
	now := r.Events[len(r.Events)-1].Arrive.Add(time.Minute)
	refs := map[string]bool{}
	for _, e := range r.Events {
		if err := e.Envelope.Validate(now); err != nil {
			t.Fatalf("%s: %v", e.Envelope.SourceEventRef, err)
		}
		if refs[e.Envelope.SourceEventRef] {
			t.Fatalf("event reference %s reused", e.Envelope.SourceEventRef)
		}
		refs[e.Envelope.SourceEventRef] = true
		if e.Arrive.Before(e.Envelope.SourceTS) {
			t.Fatalf("%s arrives before it happened", e.Envelope.SourceEventRef)
		}
	}
	for i := 1; i < len(r.Events); i++ {
		if r.Events[i].Arrive.Before(r.Events[i-1].Arrive) {
			t.Fatal("events are not in arrival order")
		}
	}
}

func TestUnknownScenarioNamesAreRefused(t *testing.T) {
	if _, err := Generate(Options{Plan: []string{"does_not_exist"}}); err == nil {
		t.Fatal("accepted")
	}
}

func TestDemo50HasFiftyTransactionsAndExactlyOneUnknown(t *testing.T) {
	r, err := Demo50(2026, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Transactions() != 50 {
		t.Fatalf("%d transactions", r.Transactions())
	}
	unknown := 0
	for _, tr := range r.Truth {
		if tr.FinalLifecycle == txn.StateUnknown {
			unknown++
		}
		for _, f := range tr.Findings {
			if f == "UNK001" {
				unknown += 100
			}
		}
	}
	if unknown != 101 {
		t.Fatalf("expected exactly one unknown transaction and one UNK001 finding, got score %d", unknown)
	}
	again, _ := Demo50(2026, time.Time{})
	if !bytes.Equal(r.EventsJSONL(), again.EventsJSONL()) {
		t.Fatal("not deterministic")
	}
}

func TestLabelsDescribeWhatTheDomainRequires(t *testing.T) {
	r, _ := Generate(Options{Seed: 1})
	by := map[string]Truth{}
	for _, tr := range r.Truth {
		by[tr.Scenario+"/"+tr.InstructionRef] = tr
	}
	has := func(scen string, code string) bool {
		for _, tr := range r.Truth {
			if tr.Scenario != scen {
				continue
			}
			for _, f := range tr.Findings {
				if f == code {
					return true
				}
			}
		}
		return false
	}
	// A timeout is never labelled a failure; a legitimate repeat is never labelled a duplicate.
	if has("switch_timeout", "FAIL001") || !has("switch_timeout", "UNK001") {
		t.Error("a timeout must be unknown")
	}
	for _, name := range []string{"repeat_outside_window", "same_amount_other_payee", "retry_after_failure"} {
		if has(name, "DUP001") {
			t.Errorf("%s was labelled a duplicate", name)
		}
	}
	if !has("duplicate_debit", "DUP001") {
		t.Error("the duplicate was not labelled")
	}
	_ = by
}

func TestTheFiftyTransactionRunSpansSeveralChannels(t *testing.T) {
	r, _ := Demo50(2026, time.Time{})
	seen := map[string]bool{}
	for _, e := range r.Events {
		seen[e.Envelope.Txn.Channel] = true
	}
	if len(seen) < 5 {
		t.Fatalf("only %d channels: %v", len(seen), seen)
	}
}
