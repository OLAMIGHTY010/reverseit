package simrun

import (
	"context"
	"testing"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/sim"
	"github.com/smartreversal/smartreversal/apps/api/internal/sources"
)

func play(t *testing.T, run sim.Run) Report { return playWith(t, run, Options{}) }

func playWith(t *testing.T, run sim.Run, o Options) Report {
	t.Helper()
	pool := dbtest.Pool(t)
	h := NewHarness(pool, dbtest.Tenant(t, pool), run.Start.Add(-time.Minute))
	ctx := context.Background()
	if err := h.InstallPolicies(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Play(ctx, run, o); err != nil {
		t.Fatal(err)
	}
	got, err := h.Observe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return Score(run.Truth, got)
}

func TestEveryScenarioInOrderMatchesItsLabel(t *testing.T) {
	run, err := sim.Generate(sim.Options{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	rep := play(t, run)
	for _, m := range rep.Mismatches {
		t.Errorf("%s (%s): %s", m.InstructionRef, m.Scenario, m.What)
	}
	t.Logf("precision %.2f recall %.2f lifecycle %d/%d by code %+v", rep.Precision, rep.Recall, rep.LifecycleCorrect, rep.Transactions, rep.ByCode)
}

func TestScoringCountsEveryKindOfDisagreement(t *testing.T) {
	labels := []sim.Truth{
		{Scenario: "a", InstructionRef: "T1", FinalLifecycle: "UNKNOWN", Findings: []string{"UNK001"}},
		{Scenario: "b", InstructionRef: "T2", FinalLifecycle: "CONFIRMED_SUCCESS", Findings: []string{}},
		{Scenario: "c", InstructionRef: "T3", FinalLifecycle: "CONFIRMED_FAILURE", Findings: []string{"FAIL001"}},
		{Scenario: "d", InstructionRef: "T4", FinalLifecycle: "CONFIRMED_SUCCESS", Findings: []string{}},
	}
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	got := map[string]*Observed{
		"T1": {InstructionRef: "T1", Lifecycle: "CONFIRMED_FAILURE", Cases: []ObservedCase{{Code: "FAIL001", LatencySecs: 4}}}, // wrong state, wrong code
		"T2": {InstructionRef: "T2", Lifecycle: "CONFIRMED_SUCCESS"},                                                           // right
		"T3": {InstructionRef: "T3", Lifecycle: "CONFIRMED_FAILURE", Cases: []ObservedCase{{Code: "FAIL001", LatencySecs: 10}, {Code: "FAIL001"}}},
		// T4 never stored
	}
	_ = at
	r := Score(labels, got)
	if r.LifecycleCorrect != 2 {
		t.Errorf("lifecycle %d", r.LifecycleCorrect)
	}
	if c := r.ByCode["UNK001"]; c.TP != 0 || c.FN != 1 {
		t.Errorf("UNK001 %+v", c)
	}
	if c := r.ByCode["FAIL001"]; c.TP != 1 || c.FP != 2 {
		t.Errorf("FAIL001 %+v (one unexpected on T1, one extra on T3)", c)
	}
	if r.Precision != 1.0/3 || r.Recall != 0.5 {
		t.Errorf("precision %v recall %v", r.Precision, r.Recall)
	}
	if len(r.Mismatches) != 5 {
		t.Errorf("%d mismatches: %+v", len(r.Mismatches), r.Mismatches)
	}
	if l := r.Latency["FAIL001"]; l.N != 1 || l.P50 != 10 {
		t.Errorf("%+v", l)
	}
	if empty := Score(nil, nil); empty.Precision != 1 || empty.Recall != 1 {
		t.Errorf("%+v", empty)
	}
}

func TestRedeliveryAndReorderingNeverChangeWhatEachTransferIs(t *testing.T) {
	base, _ := sim.Generate(sim.Options{Seed: 11, Plan: append(sim.Names(), sim.Names()...)})
	run := sim.Perturb(base, sim.Perturbation{Jitter: 90 * time.Second, RedeliverPercent: 30}, 3)
	rep := play(t, run)
	if rep.LifecycleCorrect != rep.Transactions {
		for _, m := range rep.Mismatches {
			t.Logf("%s (%s): %s", m.InstructionRef, m.Scenario, m.What)
		}
		t.Fatalf("lifecycle correct for %d of %d", rep.LifecycleCorrect, rep.Transactions)
	}
}

func TestASourceOutageAndRecoveryEndsInTheSameStates(t *testing.T) {
	base, _ := sim.Generate(sim.Options{Seed: 12})
	run := sim.Perturb(base, sim.Perturbation{Outages: []sim.Outage{{Source: sim.SrcSwitch, From: base.Start.Add(2 * time.Minute), To: base.Start.Add(40 * time.Minute)}}}, 1)
	rep := play(t, run)
	for _, m := range rep.Mismatches {
		t.Logf("%s (%s): %s", m.InstructionRef, m.Scenario, m.What)
	}
	if rep.LifecycleCorrect != rep.Transactions {
		t.Fatalf("lifecycle correct for %d of %d", rep.LifecycleCorrect, rep.Transactions)
	}
}

func TestSkewWithinWhatIngestionToleratesDoesNotChangeTheOutcome(t *testing.T) {
	base, _ := sim.Generate(sim.Options{Seed: 13})
	run := sim.Perturb(base, sim.Perturbation{Skew: map[string]time.Duration{sim.SrcSwitch: 3 * time.Minute}}, 1)
	rep := play(t, run)
	for _, m := range rep.Mismatches {
		t.Logf("%s (%s): %s", m.InstructionRef, m.Scenario, m.What)
	}
	if rep.LifecycleCorrect != rep.Transactions {
		t.Fatalf("lifecycle correct for %d of %d", rep.LifecycleCorrect, rep.Transactions)
	}
}

func TestFiftyTransactionDemonstrationIsScoredAndLatencyIsReported(t *testing.T) {
	run, err := sim.Demo50(2026, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	rep := play(t, run)
	for _, m := range rep.Mismatches {
		t.Errorf("%s (%s): %s", m.InstructionRef, m.Scenario, m.What)
	}
	if rep.Transactions != 50 || rep.ByCode["UNK001"].TP != 1 || rep.ByCode["UNK001"].FP != 0 {
		t.Fatalf("%+v", rep)
	}
	for code, l := range rep.Latency {
		t.Logf("%s: n=%d p50=%.0fs p95=%.0fs max=%.0fs (simulated time)", code, l.N, l.P50, l.P95, l.Max)
	}
}

func TestTheOutageFalsePositivesGoAwayOnceThePlatformKnowsWhatItsSourcesNormallyDo(t *testing.T) {
	base, _ := sim.Generate(sim.Options{Seed: 12})
	outage := sim.Perturbation{Outages: []sim.Outage{{Source: sim.SrcSwitch, From: base.Start.Add(2 * time.Minute), To: base.Start.Add(40 * time.Minute)}}}
	run := sim.Perturb(base, outage, 1)

	without := play(t, run)
	with := playWith(t, run, Options{Expectations: map[string]sources.Expectation{
		sim.SrcSwitch: {SourceSystem: sim.SrcSwitch, StaleAfterSec: 120, DownAfterSec: 300, MaxSkewSec: 60},
	}})
	count := func(r Report, what string) int {
		n := 0
		for _, m := range r.Mismatches {
			if m.What == what {
				n++
			}
		}
		return n
	}
	for _, what := range []string{"unexpected UNK001 case", "unexpected DUP001 case"} {
		t.Logf("%s: %d without expectations, %d with", what, count(without, what), count(with, what))
	}
	if count(without, "unexpected UNK001 case") == 0 || count(without, "unexpected DUP001 case") == 0 {
		t.Fatal("the outage no longer produces the false positives this test is about; revisit it")
	}
	if n := count(with, "unexpected UNK001 case"); n != 0 {
		t.Errorf("%d unknown cases were still raised for transfers only waiting on the silent source", n)
	}
	// A retry that arrives before the silent source has been noticed as silent (here, a
	// retry ten seconds after the last word from the switch, while the source is still
	// inside its stale threshold) is still judged on what is known. That is a limit of
	// any silence-based check and is recorded, not hidden.
	if n := count(with, "unexpected DUP001 case"); n > count(without, "unexpected DUP001 case") {
		t.Errorf("%d retries were called duplicates, more than without expectations", n)
	}
	if with.LifecycleCorrect != with.Transactions {
		t.Errorf("lifecycle correct for %d of %d", with.LifecycleCorrect, with.Transactions)
	}
	for _, m := range with.Mismatches {
		t.Logf("remaining: %s (%s): %s", m.InstructionRef, m.Scenario, m.What)
	}
}
