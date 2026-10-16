// Package deadline computes the deadlines a customer-reported or detected dispute
// is held to.
//
// The dispute type to clock mapping is deterministic and fixed here (Section 10.4 of
// the scope), and a language model never chooses it. Every duration comes from the
// regulatory clock registry or from other approved configuration. Nothing is assumed:
// a clock nobody has approved yields no regulatory deadline, and says so.
//
// Two deadlines are kept apart. The regulatory one runs from the situation's start
// event; the internal target is the bank's own, earlier, and may start from a different
// moment (for a failed instant transfer, the regulatory view runs from the complaint and
// the internal target from the failure). Both are returned with where each came from.
package deadline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/bizcal"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
)

// Dispute types.
const (
	FailedTransfer   = "FAILED_TRANSFER"
	DuplicateDebit   = "DUPLICATE_DEBIT"
	PosWebFailure    = "POS_WEB_FAILURE"
	AtmOnUs          = "ATM_ON_US"
	AtmNotOnUs       = "ATM_NOT_ON_US"
	WrongAccountOrAm = "WRONG_ACCOUNT_OR_AMOUNT"
	UnmatchedClaim   = "UNMATCHED_CLAIM"
)

// States of a deadline.
const (
	OnTrack     = "on_track"
	Approaching = "approaching"
	Urgent      = "urgent"
	Overdue     = "overdue"
)

// ErrUnknownDispute is returned for a dispute type with no clock mapping.
var ErrUnknownDispute = errors.New("deadline: unknown dispute type")

type start int

const (
	fromFailure start = iota
	fromReport
	fromTransactionDate
)

// rule maps a dispute type to its clock and to the moments the clocks start from.
type rule struct {
	clock      string // regulatory clock id in the registry; empty for none
	regulatory start
	internal   start
}

// rules is the deterministic mapping of Section 10.4. The clock ids are those of the
// seeded registry (configsvc.SeedClocks).
var rules = map[string]rule{
	FailedTransfer:   {"FAILED_INSTANT_TRANSFER", fromReport, fromFailure},
	DuplicateDebit:   {"FAILED_INSTANT_TRANSFER", fromReport, fromFailure}, // until classified further
	WrongAccountOrAm: {"WRONG_ACCOUNT_REQUEST", fromTransactionDate, fromTransactionDate},
	PosWebFailure:    {"POS_WEB_DISPUTE", fromReport, fromReport},
	AtmOnUs:          {"ATM_ON_US_MANUAL_REFUND", fromFailure, fromFailure},
	AtmNotOnUs:       {"ATM_NOT_ON_US", fromFailure, fromFailure},
	UnmatchedClaim:   {"", fromReport, fromReport},
}

// ClockFor returns the registry clock id for a dispute type ("" for none).
func ClockFor(dispute string) (string, bool) {
	r, ok := rules[dispute]
	return r.clock, ok
}

// Facts are the moments a deadline can start from. A zero time means not known.
type Facts struct {
	FailureAt         time.Time
	ReportedAt        time.Time
	TransactionDateAt time.Time
}

func (f Facts) at(s start) time.Time {
	switch s {
	case fromFailure:
		return f.FailureAt
	case fromReport:
		return f.ReportedAt
	}
	return f.TransactionDateAt
}

// Regulatory is the deadline from an approved regulatory clock.
type Regulatory struct {
	ClockID    string    `json:"clock_id"`
	Situation  string    `json:"situation"`
	Due        time.Time `json:"due"`
	StartedAt  time.Time `json:"started_at"`
	StartEvent string    `json:"start_event"`
	Duration   int       `json:"duration"`
	Unit       string    `json:"unit"`
	// Status is the registry status of the clock: confirmed, reported, draft or
	// conflicting. Anything other than confirmed means the figure is unverified.
	Status        string `json:"status"`
	Source        string `json:"source_reference"`
	ConfigVersion int    `json:"config_version"`
}

// Internal is the bank's own target.
type Internal struct {
	Due       time.Time `json:"due"`
	StartedAt time.Time `json:"started_at"`
	Hours     int       `json:"hours"`
	Source    string    `json:"source"`
}

// Result is a computed deadline.
type Result struct {
	Dispute    string      `json:"dispute_type"`
	Regulatory *Regulatory `json:"regulatory,omitempty"`
	Internal   *Internal   `json:"internal_target,omitempty"`
	// Binding is the earlier of the two deadlines that exist: the one to show as
	// "your bank target". Nil when neither exists.
	Binding *time.Time `json:"binding_due,omitempty"`
	// State is computed against the binding deadline. It is on_track, approaching,
	// urgent or overdue. "approaching" and "urgent" need approved percentages; without
	// them a deadline is only on_track or overdue.
	State        string    `json:"state"`
	RemainingSec int64     `json:"remaining_seconds"`
	Missing      []string  `json:"not_configured,omitempty"`
	Now          time.Time `json:"as_of"`
}

// Engine computes deadlines.
type Engine struct {
	Config *configsvc.Service
	Now    func() time.Time
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

// Compute returns the deadlines for a dispute. tx is a tenant transaction.
func (e *Engine) Compute(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, dispute string, f Facts) (Result, error) {
	r, ok := rules[dispute]
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrUnknownDispute, dispute)
	}
	now := e.now()
	res := Result{Dispute: dispute, Now: now, State: OnTrack}

	if r.clock != "" {
		started := f.at(r.regulatory)
		switch v, err := e.Config.Effective(ctx, tx, tenant, configsvc.ClockKey(r.clock), now); {
		case errors.Is(err, configsvc.ErrNotConfigured):
			res.Missing = append(res.Missing, configsvc.ClockKey(r.clock)+": no approved clock")
		case err != nil:
			return res, err
		case started.IsZero():
			res.Missing = append(res.Missing, "the moment the regulatory clock starts from is not known")
		default:
			var c configsvc.Clock
			if err := json.Unmarshal(v.Raw, &c); err != nil {
				return res, err
			}
			due, err := e.add(ctx, tx, tenant, started, c)
			if err != nil {
				return res, err
			}
			res.Regulatory = &Regulatory{ClockID: r.clock, Situation: c.Situation, Due: due, StartedAt: started.UTC(), StartEvent: c.StartEvent,
				Duration: c.Duration, Unit: c.Unit, Status: v.VerifyStatus, Source: v.SourceReference, ConfigVersion: v.Version}
		}
		key := "internal_target_hours." + r.clock
		switch v, err := e.Config.Effective(ctx, tx, tenant, key, now); {
		case errors.Is(err, configsvc.ErrNotConfigured):
			res.Missing = append(res.Missing, key+": no approved internal target")
		case err != nil:
			return res, err
		default:
			if s := f.at(r.internal); !s.IsZero() {
				res.Internal = &Internal{Due: s.UTC().Add(time.Duration(v.Value) * time.Hour), StartedAt: s.UTC(), Hours: int(v.Value), Source: key}
			} else {
				res.Missing = append(res.Missing, "the moment the internal target starts from is not known")
			}
		}
	} else {
		// A claim with no match has no regulatory clock until it is matched; the bank
		// holds itself to an acknowledgement target.
		key := "customer.ack_target_hours"
		switch v, err := e.Config.Effective(ctx, tx, tenant, key, now); {
		case errors.Is(err, configsvc.ErrNotConfigured):
			res.Missing = append(res.Missing, key+": no approved acknowledgement target")
		case err != nil:
			return res, err
		case f.ReportedAt.IsZero():
			res.Missing = append(res.Missing, "the report time is not known")
		default:
			res.Internal = &Internal{Due: f.ReportedAt.UTC().Add(time.Duration(v.Value) * time.Hour), StartedAt: f.ReportedAt.UTC(), Hours: int(v.Value), Source: key}
		}
	}

	var bind *time.Time
	var bindStart time.Time
	if res.Regulatory != nil {
		d, s := res.Regulatory.Due, res.Regulatory.StartedAt
		bind, bindStart = &d, s
	}
	if res.Internal != nil && (bind == nil || res.Internal.Due.Before(*bind)) {
		d, s := res.Internal.Due, res.Internal.StartedAt
		bind, bindStart = &d, s
	}
	if bind == nil {
		return res, nil
	}
	res.Binding = bind
	res.RemainingSec = int64(bind.Sub(now).Seconds())
	var err error
	res.State, err = e.state(ctx, tx, tenant, bindStart, *bind, now)
	return res, err
}

func (e *Engine) add(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from time.Time, c configsvc.Clock) (time.Time, error) {
	switch c.Unit {
	case "minutes":
		return from.Add(time.Duration(c.Duration) * time.Minute).UTC(), nil
	case "hours":
		return from.Add(time.Duration(c.Duration) * time.Hour).UTC(), nil
	case "working_days":
		cal, err := bizcal.Load(ctx, tx, e.Config, tenant, from)
		if err != nil {
			return time.Time{}, err
		}
		due, err := cal.AddWorkingDays(from, c.Duration)
		return due.UTC(), err
	}
	return time.Time{}, fmt.Errorf("deadline: clock unit %q", c.Unit)
}

// state compares the share of the time used against approved percentages.
func (e *Engine) state(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, started, due, now time.Time) (string, error) {
	if !now.Before(due) {
		return Overdue, nil
	}
	total := due.Sub(started)
	if total <= 0 {
		return OnTrack, nil
	}
	used := float64(now.Sub(started)) / float64(total) * 100
	for _, c := range []struct{ key, state string }{{"deadline.urgent_percent", Urgent}, {"deadline.approaching_percent", Approaching}} {
		v, err := e.Config.Effective(ctx, tx, tenant, c.key, now)
		if errors.Is(err, configsvc.ErrNotConfigured) {
			continue
		}
		if err != nil {
			return "", err
		}
		if used >= float64(v.Value) {
			return c.state, nil
		}
	}
	return OnTrack, nil
}
