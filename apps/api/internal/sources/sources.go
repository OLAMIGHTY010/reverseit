// Package sources reports how fresh each source system's evidence is and how far its
// clock is from ours.
//
// Freshness matters because a missing event is only evidence of a problem if the
// source is actually talking. A source that has gone quiet makes every transaction it
// feeds look stuck. Expectations (how long a silence is stale, how long it is down,
// how much clock skew is tolerable) are configuration set by people who know the
// source. Nothing here assumes any value: a source with no expectation is reported
// as unconfigured, never as fresh.
package sources

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Status values.
const (
	Fresh        = "fresh"
	Stale        = "stale"
	Down         = "down"
	Unconfigured = "unconfigured"
)

// SkewSample is how many of a source's newest events are used to measure its clock.
const SkewSample = 200

// Expectation is what is normal for one source.
type Expectation struct {
	SourceSystem  string `json:"source_system"`
	StaleAfterSec int    `json:"stale_after_secs"`
	DownAfterSec  int    `json:"down_after_secs"`
	MaxSkewSec    int    `json:"max_skew_secs"`
}

// ErrInvalid wraps a bad expectation.
var ErrInvalid = errors.New("sources: invalid expectation")

func (e Expectation) validate() error {
	switch {
	case e.SourceSystem == "":
		return fmt.Errorf("%w: source_system is required", ErrInvalid)
	case e.StaleAfterSec < 1 || e.DownAfterSec <= e.StaleAfterSec:
		return fmt.Errorf("%w: stale_after_secs must be at least 1 and less than down_after_secs", ErrInvalid)
	case e.DownAfterSec > 2592000:
		return fmt.Errorf("%w: down_after_secs may not exceed 30 days", ErrInvalid)
	case e.MaxSkewSec < 0 || e.MaxSkewSec > 86400:
		return fmt.Errorf("%w: max_skew_secs must be between 0 and 86400", ErrInvalid)
	}
	return nil
}

// Set stores the expectation for a source, replacing the previous one.
func Set(ctx context.Context, tx pgx.Tx, tenant, by uuid.UUID, e Expectation) error {
	if err := e.validate(); err != nil {
		return err
	}
	var known int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM transaction_events WHERE tenant_id = $1 AND source_system = $2`, tenant, e.SourceSystem).Scan(&known); err != nil {
		return err
	}
	if known == 0 {
		return fmt.Errorf("%w: no events have been received from %q", ErrInvalid, e.SourceSystem)
	}
	_, err := tx.Exec(ctx, `INSERT INTO source_expectations (tenant_id, source_system, stale_after_secs, down_after_secs, max_skew_secs, updated_by, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6, now())
		ON CONFLICT (tenant_id, source_system) DO UPDATE SET stale_after_secs = EXCLUDED.stale_after_secs, down_after_secs = EXCLUDED.down_after_secs,
			max_skew_secs = EXCLUDED.max_skew_secs, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		tenant, e.SourceSystem, e.StaleAfterSec, e.DownAfterSec, e.MaxSkewSec, by)
	return err
}

// Status is one source's health.
type Status struct {
	SourceSystem   string       `json:"source_system"`
	Status         string       `json:"status"`
	Expectation    *Expectation `json:"expectation,omitempty"`
	LastReceivedAt time.Time    `json:"last_received_at"`
	LastEventAt    time.Time    `json:"last_event_at"`
	SilenceSec     int64        `json:"silence_secs"`
	Events         int          `json:"events"`
	SkewP50Sec     float64      `json:"skew_p50_secs"`
	SkewP95Sec     float64      `json:"skew_p95_secs"`
	SkewSamples    int          `json:"skew_samples"`
	SkewExceeded   bool         `json:"skew_exceeded"`
}

// Report returns the health of every source that has sent events, at the given
// time. Silence is measured from the last time we received something from the
// source, not from the time on its events, so a source that sends old events is not
// mistaken for a silent one. Skew is received time minus source time over the
// newest SkewSample events; it is meaningful for sources that deliver events as they
// happen, and not for a file imported long after the events occurred.
func Report(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, now time.Time) ([]Status, error) {
	rows, err := tx.Query(ctx, `SELECT e.source_system, max(e.received_at), max(e.source_ts), count(*)::int,
			x.stale_after_secs, x.down_after_secs, x.max_skew_secs
		FROM transaction_events e
		LEFT JOIN source_expectations x ON x.tenant_id = e.tenant_id AND x.source_system = e.source_system
		WHERE e.tenant_id = $1 GROUP BY e.source_system, x.stale_after_secs, x.down_after_secs, x.max_skew_secs ORDER BY e.source_system`, tenant)
	if err != nil {
		return nil, err
	}
	var out []Status
	for rows.Next() {
		var s Status
		var stale, down, skew *int
		if err := rows.Scan(&s.SourceSystem, &s.LastReceivedAt, &s.LastEventAt, &s.Events, &stale, &down, &skew); err != nil {
			rows.Close()
			return nil, err
		}
		s.LastReceivedAt, s.LastEventAt = s.LastReceivedAt.UTC(), s.LastEventAt.UTC()
		s.SilenceSec = int64(now.Sub(s.LastReceivedAt).Seconds())
		if s.SilenceSec < 0 {
			s.SilenceSec = 0
		}
		s.Status = Unconfigured
		if stale != nil {
			s.Expectation = &Expectation{SourceSystem: s.SourceSystem, StaleAfterSec: *stale, DownAfterSec: *down, MaxSkewSec: *skew}
			switch {
			case s.SilenceSec >= int64(*down):
				s.Status = Down
			case s.SilenceSec >= int64(*stale):
				s.Status = Stale
			default:
				s.Status = Fresh
			}
		}
		out = append(out, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		skews, err := skewSample(ctx, tx, tenant, out[i].SourceSystem)
		if err != nil {
			return nil, err
		}
		out[i].SkewSamples = len(skews)
		if len(skews) > 0 {
			out[i].SkewP50Sec, out[i].SkewP95Sec = percentile(skews, 0.50), percentile(skews, 0.95)
			if out[i].Expectation != nil {
				limit := float64(out[i].Expectation.MaxSkewSec)
				out[i].SkewExceeded = absf(out[i].SkewP50Sec) > limit || absf(out[i].SkewP95Sec) > limit
			}
		}
	}
	return out, nil
}

func skewSample(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, source string) ([]float64, error) {
	rows, err := tx.Query(ctx, `SELECT extract(epoch FROM (received_at - source_ts))::float8 FROM transaction_events
		WHERE tenant_id = $1 AND source_system = $2 ORDER BY received_at DESC, id LIMIT $3`, tenant, source, SkewSample)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var v []float64
	for rows.Next() {
		var x float64
		if err := rows.Scan(&x); err != nil {
			return nil, err
		}
		v = append(v, x)
	}
	return v, rows.Err()
}

// percentile uses the nearest-rank method on a copy.
func percentile(v []float64, p float64) float64 {
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	rank := int(p*float64(len(c))+0.999999999) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(c) {
		rank = len(c) - 1
	}
	return c[rank]
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Degraded returns the sources that have an expectation and are stale or down at
// now, mapped to "stale" or "down". A source with no expectation is not in the map:
// the platform does not assume a threshold, so for callers that must not change
// behaviour until someone has set one, it is simply not reported as degraded. Use
// Report to see unconfigured sources.
func Degraded(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, now time.Time) (map[string]string, error) {
	rows, err := tx.Query(ctx, `SELECT x.source_system, x.stale_after_secs, x.down_after_secs,
			(SELECT max(e.received_at) FROM transaction_events e WHERE e.tenant_id = x.tenant_id AND e.source_system = x.source_system)
		FROM source_expectations x WHERE x.tenant_id = $1`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var src string
		var stale, down int
		var last *time.Time
		if err := rows.Scan(&src, &stale, &down, &last); err != nil {
			return nil, err
		}
		if last == nil {
			continue // an expectation for a source that has sent nothing yet: nothing to be silent about
		}
		silence := int64(now.Sub(*last).Seconds())
		switch {
		case silence >= int64(down):
			out[src] = Down
		case silence >= int64(stale):
			out[src] = Stale
		}
	}
	return out, rows.Err()
}
