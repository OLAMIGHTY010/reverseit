package simrun

import (
	"fmt"
	"math"
	"sort"

	"github.com/smartreversal/smartreversal/apps/api/internal/sim"
)

// Mismatch is one place where the platform disagreed with the label.
type Mismatch struct {
	InstructionRef string `json:"instruction_ref"`
	Scenario       string `json:"scenario"`
	What           string `json:"what"`
}

// CodeScore counts, for one exception code, labelled findings that were raised
// (true positives), raised but not labelled (false positives) and labelled but not
// raised (false negatives).
type CodeScore struct {
	TP int `json:"tp"`
	FP int `json:"fp"`
	FN int `json:"fn"`
}

// Report is the result of scoring a run.
type Report struct {
	Transactions     int                  `json:"transactions"`
	LifecycleCorrect int                  `json:"lifecycle_correct"`
	ByCode           map[string]CodeScore `json:"by_code"`
	Precision        float64              `json:"precision"`
	Recall           float64              `json:"recall"`
	// FalsePositives and Misses list the individual disagreements.
	Mismatches []Mismatch `json:"mismatches"`
	// Latency is the simulated delay from the first debit to the case being opened,
	// per code, in seconds. See the package comment: simulated time, not performance.
	Latency map[string]Latency `json:"latency_secs_by_code"`
}

// Latency summarises detection delay for one code.
type Latency struct {
	N   int     `json:"n"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	Max float64 `json:"max"`
}

// Score compares what the platform did with the labels. A finding counts as raised
// if a case of that code was ever opened for the transaction, whether or not it was
// later closed, because the label says whether detection was right to raise it.
// Precision and recall are over findings, and are only meaningful for the labelled
// synthetic cases that were played. They are not a claim about real traffic.
func Score(labels []sim.Truth, got map[string]*Observed) Report {
	r := Report{Transactions: len(labels), ByCode: map[string]CodeScore{}, Latency: map[string]Latency{}}
	lat := map[string][]float64{}
	add := func(code string, f func(*CodeScore)) {
		c := r.ByCode[code]
		f(&c)
		r.ByCode[code] = c
	}
	for _, l := range labels {
		o := got[l.InstructionRef]
		if o == nil {
			r.Mismatches = append(r.Mismatches, Mismatch{l.InstructionRef, l.Scenario, "the transaction was never stored"})
			for _, code := range l.Findings {
				add(code, func(c *CodeScore) { c.FN++ })
			}
			continue
		}
		if o.Lifecycle == l.FinalLifecycle {
			r.LifecycleCorrect++
		} else {
			r.Mismatches = append(r.Mismatches, Mismatch{l.InstructionRef, l.Scenario, fmt.Sprintf("lifecycle %s, expected %s", o.Lifecycle, l.FinalLifecycle)})
		}
		want := map[string]bool{}
		for _, code := range l.Findings {
			want[code] = true
		}
		have := map[string]ObservedCase{}
		for _, c := range o.Cases {
			if _, dup := have[c.Code]; dup {
				r.Mismatches = append(r.Mismatches, Mismatch{l.InstructionRef, l.Scenario, "more than one " + c.Code + " case for one transaction"})
				add(c.Code, func(s *CodeScore) { s.FP++ })
				continue
			}
			have[c.Code] = c
		}
		for code := range want {
			if c, ok := have[code]; ok {
				add(code, func(s *CodeScore) { s.TP++ })
				lat[code] = append(lat[code], c.LatencySecs)
			} else {
				add(code, func(s *CodeScore) { s.FN++ })
				r.Mismatches = append(r.Mismatches, Mismatch{l.InstructionRef, l.Scenario, "expected a " + code + " case and none was opened"})
			}
		}
		for code := range have {
			if !want[code] {
				add(code, func(s *CodeScore) { s.FP++ })
				r.Mismatches = append(r.Mismatches, Mismatch{l.InstructionRef, l.Scenario, "unexpected " + code + " case"})
			}
		}
	}
	var tp, fp, fn int
	for _, c := range r.ByCode {
		tp, fp, fn = tp+c.TP, fp+c.FP, fn+c.FN
	}
	r.Precision, r.Recall = ratio(tp, tp+fp), ratio(tp, tp+fn)
	for code, v := range lat {
		sort.Float64s(v)
		r.Latency[code] = Latency{N: len(v), P50: nearest(v, 0.50), P95: nearest(v, 0.95), Max: v[len(v)-1]}
	}
	sort.Slice(r.Mismatches, func(i, j int) bool {
		if r.Mismatches[i].InstructionRef != r.Mismatches[j].InstructionRef {
			return r.Mismatches[i].InstructionRef < r.Mismatches[j].InstructionRef
		}
		return r.Mismatches[i].What < r.Mismatches[j].What
	})
	return r
}

// ratio is 1 when there is nothing to be right about, so an empty denominator is not
// reported as a failure.
func ratio(a, b int) float64 {
	if b == 0 {
		return 1
	}
	return float64(a) / float64(b)
}

func nearest(sorted []float64, p float64) float64 {
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}
