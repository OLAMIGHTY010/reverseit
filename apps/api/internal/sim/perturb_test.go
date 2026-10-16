package sim

import (
	"bytes"
	"testing"
	"time"
)

func TestPerturbationIsDeterministicAndKeepsTheLabels(t *testing.T) {
	r, _ := Generate(Options{Seed: 5})
	p := Perturbation{Jitter: 2 * time.Minute, RedeliverPercent: 20}
	a, b := Perturb(r, p, 9), Perturb(r, p, 9)
	if !bytes.Equal(a.EventsJSONL(), b.EventsJSONL()) {
		t.Fatal("not deterministic")
	}
	if !bytes.Equal(a.TruthJSONL(), r.TruthJSONL()) {
		t.Fatal("perturbing delivery changed what is true")
	}
	if len(a.Events) <= len(r.Events) {
		t.Fatal("nothing was redelivered")
	}
	for i := 1; i < len(a.Events); i++ {
		if a.Events[i].Arrive.Before(a.Events[i-1].Arrive) {
			t.Fatal("not in arrival order")
		}
	}
}

func TestJitterReordersEventsOfOneTransfer(t *testing.T) {
	r, _ := Generate(Options{Seed: 5, Plan: []string{"normal_success"}})
	reordered := false
	for seed := uint64(0); seed < 20 && !reordered; seed++ {
		p := Perturb(r, Perturbation{Jitter: time.Minute}, seed)
		for i := 1; i < len(p.Events); i++ {
			if p.Events[i].Envelope.SourceTS.Before(p.Events[i-1].Envelope.SourceTS) {
				reordered = true
			}
		}
	}
	if !reordered {
		t.Fatal("jitter never produced an out-of-order delivery")
	}
}

func TestAnOutageHoldsASourceBackAndThenDeliversItInOrder(t *testing.T) {
	r, _ := Generate(Options{Seed: 5})
	from, to := r.Start.Add(2*time.Minute), r.Start.Add(30*time.Minute)
	p := Perturb(r, Perturbation{Outages: []Outage{{Source: SrcSwitch, From: from, To: to}}}, 1)
	held := 0
	for _, e := range p.Events {
		if e.Envelope.SourceSystem == SrcSwitch && e.Envelope.SourceTS.After(from) && e.Envelope.SourceTS.Before(to) {
			if e.Arrive.Before(to) {
				t.Fatalf("%s was delivered during the outage", e.Envelope.SourceEventRef)
			}
			held++
		}
		if e.Envelope.SourceSystem != SrcSwitch && e.Arrive.After(to) && e.Envelope.SourceTS.Before(from) {
			t.Fatal("another source was held back")
		}
	}
	if held == 0 {
		t.Fatal("the outage held nothing")
	}
}

func TestSkewShiftsTheTimeOnEventsNotTheirArrival(t *testing.T) {
	r, _ := Generate(Options{Seed: 5, Plan: []string{"normal_success"}})
	p := Perturb(r, Perturbation{Skew: map[string]time.Duration{SrcSwitch: 40 * time.Second}}, 1)
	for i, e := range r.Events {
		q := p.Events[i]
		want := e.Envelope.SourceTS
		if e.Envelope.SourceSystem == SrcSwitch {
			want = want.Add(40 * time.Second)
		}
		if !q.Envelope.SourceTS.Equal(want) || !q.Arrive.Equal(e.Arrive) {
			t.Fatalf("%s: source time %s arrive %s", q.Envelope.SourceEventRef, q.Envelope.SourceTS, q.Arrive)
		}
	}
}
