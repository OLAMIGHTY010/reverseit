// Package simrun plays a simulator run through the real ingestion and detection code
// on a simulated clock and scores the result against the run's labels.
//
// Time in a run is simulated. The latencies reported are the delays built into the
// rules and the scenario (for example a waiting threshold), measured on that clock.
// They say nothing about how fast the code runs on real hardware.
package simrun

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/detect"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/sim"
	"github.com/smartreversal/smartreversal/apps/api/internal/sources"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// PolicyLabel marks every value installed here.
const PolicyLabel = "SYNTHETIC DEMO POLICY - not approved for production"

// Policies are the synthetic values a run is played under.
var Policies = map[string]int{
	"duplicate.window_seconds":      30,
	"waiting_threshold_seconds.NIP": 300,
}

// Clock is the simulated time every service in the harness reads.
type Clock struct{ t time.Time }

// Now returns the simulated time.
func (c *Clock) Now() time.Time { return c.t }

// Harness holds the real services, driven by one simulated clock.
type Harness struct {
	Pool   *db.Pool
	Tenant uuid.UUID
	Clock  *Clock
	Txn    *txn.Service
	Det    *detect.Detector
	Config *configsvc.Service
}

// NewHarness builds the services for a tenant. The clock starts at start.
func NewHarness(pool *db.Pool, tenant uuid.UUID, start time.Time) *Harness {
	clk := &Clock{t: start.UTC()}
	app := &audit.Appender{Now: clk.Now}
	cfg := &configsvc.Service{Audit: app, Now: clk.Now}
	cs := &cases.Service{Audit: app, Config: cfg, Now: clk.Now}
	return &Harness{Pool: pool, Tenant: tenant, Clock: clk, Config: cfg,
		Txn: &txn.Service{Now: clk.Now}, Det: &detect.Detector{Cases: cs, Config: cfg, Now: clk.Now}}
}

// InstallPolicies installs the synthetic policies through the normal proposal and
// decision path, effective from the clock's current time.
func (h *Harness) InstallPolicies(ctx context.Context) error {
	keys := make([]string, 0, len(Policies))
	for k := range Policies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		raw, _ := json.Marshal(Policies[key])
		err := h.Pool.WithTenant(ctx, h.Tenant, func(tx pgx.Tx) error {
			v, err := h.Config.Propose(ctx, tx, h.Tenant, uuid.New(), configsvc.ProposeInput{Key: key, Value: raw, SourceReference: PolicyLabel,
				EffectiveFrom: h.Clock.t, VerifyStatus: "unverified", Reason: "synthetic policy for a simulator run"})
			if err != nil {
				return err
			}
			_, err = h.Config.Decide(ctx, tx, h.Tenant, uuid.New(), key, v.Version, true, "")
			return err
		})
		if err != nil {
			return fmt.Errorf("simrun: install %s: %w", key, err)
		}
	}
	return nil
}

// Options control how a run is played.
type Options struct {
	// SweepEvery is how often, in simulated time, the stuck-transaction sweep runs.
	SweepEvery time.Duration
	// Settle is how long after the last event the sweep keeps running.
	Settle time.Duration
	// Expectations are set for a source as soon as it has sent its first event,
	// because an expectation can only be set for a source that has spoken.
	Expectations map[string]sources.Expectation
}

func (o *Options) defaults() {
	if o.SweepEvery <= 0 {
		o.SweepEvery = time.Minute
	}
	if o.Settle <= 0 {
		o.Settle = time.Hour
	}
}

// Play delivers every event at its arrival time, evaluates each accepted
// transaction as the worker would, and runs the stuck sweep on the simulated clock.
func (h *Harness) Play(ctx context.Context, run sim.Run, o Options) error {
	o.defaults()
	var last time.Time
	sweepTo := func(until time.Time) error {
		for next := last.Add(o.SweepEvery); !next.After(until); next = next.Add(o.SweepEvery) {
			h.Clock.t = next
			if err := h.Pool.WithTenant(ctx, h.Tenant, func(tx pgx.Tx) error {
				_, err := h.Det.SweepStuck(ctx, tx, h.Tenant)
				return err
			}); err != nil {
				return err
			}
			last = next
		}
		return nil
	}
	if len(run.Events) > 0 {
		last = run.Events[0].Arrive.Add(-time.Second)
	}
	expected := map[string]bool{}
	for _, e := range run.Events {
		if err := sweepTo(e.Arrive); err != nil {
			return err
		}
		h.Clock.t = e.Arrive
		err := h.Pool.WithTenant(ctx, h.Tenant, func(tx pgx.Tx) error {
			res, err := h.Txn.Ingest(ctx, tx, h.Tenant, e.Envelope)
			if err != nil {
				return err
			}
			_, err = h.Det.Evaluate(ctx, tx, h.Tenant, res.TxnID)
			return err
		})
		if err != nil {
			return fmt.Errorf("simrun: %s: %w", e.Envelope.SourceEventRef, err)
		}
		last = e.Arrive
		if x, ok := o.Expectations[e.Envelope.SourceSystem]; ok && !expected[e.Envelope.SourceSystem] {
			if err := h.Pool.WithTenant(ctx, h.Tenant, func(tx pgx.Tx) error {
				return sources.Set(ctx, tx, h.Tenant, uuid.New(), x)
			}); err != nil {
				return fmt.Errorf("simrun: expectation for %s: %w", x.SourceSystem, err)
			}
			expected[e.Envelope.SourceSystem] = true
		}
	}
	if len(run.Events) > 0 {
		return sweepTo(run.Events[len(run.Events)-1].Arrive.Add(o.Settle))
	}
	return nil
}

// Observed is what the platform concluded about one transaction.
type Observed struct {
	InstructionRef string
	Lifecycle      string
	Cases          []ObservedCase
}

// ObservedCase is a case opened for a transaction.
type ObservedCase struct {
	Code        string
	Status      string
	ClosedBy    string
	OpenedAt    time.Time
	FirstDebit  time.Time
	LatencySecs float64
}

// Observe reads the outcome of every transaction in the tenant.
func (h *Harness) Observe(ctx context.Context) (map[string]*Observed, error) {
	out := map[string]*Observed{}
	err := h.Pool.WithTenant(ctx, h.Tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, instruction_ref, lifecycle_state, first_debit_at FROM transactions WHERE tenant_id = $1`, h.Tenant)
		if err != nil {
			return err
		}
		byID := map[uuid.UUID]*Observed{}
		debit := map[uuid.UUID]time.Time{}
		for rows.Next() {
			var id uuid.UUID
			var o Observed
			var fd *time.Time
			if err := rows.Scan(&id, &o.InstructionRef, &o.Lifecycle, &fd); err != nil {
				rows.Close()
				return err
			}
			if fd != nil {
				debit[id] = fd.UTC()
			}
			cp := o
			byID[id] = &cp
			out[o.InstructionRef] = &cp
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		list, err := h.Det.Cases.List(ctx, tx, h.Tenant, cases.Filter{}, nil, 201)
		if err != nil {
			return err
		}
		// List pages are bounded, so read until nothing new comes back.
		seen := map[uuid.UUID]bool{}
		var cur *cases.Cursor
		for len(list) > 0 {
			for _, c := range list {
				if seen[c.ID] {
					continue
				}
				seen[c.ID] = true
				if c.TxnID == nil || byID[*c.TxnID] == nil {
					continue
				}
				oc := ObservedCase{Code: c.Classification, Status: c.Status, ClosedBy: c.ClosedReason, OpenedAt: c.OpenedAt.UTC(), FirstDebit: debit[*c.TxnID]}
				if !oc.FirstDebit.IsZero() {
					oc.LatencySecs = oc.OpenedAt.Sub(oc.FirstDebit).Seconds()
				}
				byID[*c.TxnID].Cases = append(byID[*c.TxnID].Cases, oc)
			}
			if len(list) < 201 {
				break
			}
			last := list[len(list)-1]
			c := cases.CursorFor(last)
			cur = &c
			if list, err = h.Det.Cases.List(ctx, tx, h.Tenant, cases.Filter{}, cur, 201); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}
