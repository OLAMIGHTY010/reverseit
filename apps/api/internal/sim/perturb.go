package sim

import (
	"math/rand/v2"
	"sort"
	"time"
)

// Outage withholds everything from one source between From and To; it is all
// delivered at To, in its original order, as when a feed recovers and catches up.
type Outage struct {
	Source   string
	From, To time.Time
}

// Perturbation changes how a run is delivered without changing what happened.
// The ground-truth labels stay the same: the evidence is the same evidence, so the
// platform should reach the same conclusion about each transaction.
type Perturbation struct {
	// Jitter delays each delivery by a random time up to this long, so events cross.
	Jitter time.Duration
	// RedeliverPercent (0 to 100) of events are delivered a second time, later.
	RedeliverPercent int
	// Outages hold back a source for a period.
	Outages []Outage
	// Skew shifts the time a source puts on its events (not when they arrive), as a
	// source with a wrong clock would. Keep it inside what ingestion tolerates.
	Skew map[string]time.Duration
}

// Perturb returns a copy of r delivered under p. It is deterministic for a seed.
func Perturb(r Run, p Perturbation, seed uint64) Run {
	rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeefcafef00d))
	out := Run{Seed: r.Seed, Start: r.Start, Truth: append([]Truth(nil), r.Truth...)}
	for _, e := range r.Events {
		if off, ok := p.Skew[e.Envelope.SourceSystem]; ok {
			e.Envelope.SourceTS = e.Envelope.SourceTS.Add(off)
		}
		if p.Jitter > 0 {
			e.Arrive = e.Arrive.Add(time.Duration(rng.Int64N(int64(p.Jitter))))
		}
		for _, o := range p.Outages {
			if e.Envelope.SourceSystem == o.Source && !e.Arrive.Before(o.From) && e.Arrive.Before(o.To) {
				e.Arrive = o.To
			}
		}
		out.Events = append(out.Events, e)
		if p.RedeliverPercent > 0 && rng.IntN(100) < p.RedeliverPercent {
			again := e
			again.Arrive = e.Arrive.Add(time.Duration(1+rng.Int64N(120)) * time.Second)
			out.Events = append(out.Events, again)
		}
	}
	out.sort()
	// Events held until the same instant keep their order of occurrence.
	sort.SliceStable(out.Events, func(i, j int) bool {
		a, b := out.Events[i], out.Events[j]
		if a.Arrive.Equal(b.Arrive) {
			return a.Envelope.SourceTS.Before(b.Envelope.SourceTS)
		}
		return a.Arrive.Before(b.Arrive)
	})
	return out
}
