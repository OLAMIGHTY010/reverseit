// Package metrics defines the reporting metrics and computes them from the platform's
// own records.
//
// Every metric has a name, a definition, a formula, the tables it reads, the period it
// covers and its known limits, so a number on a screen can be traced to what it counts.
// Values are measured from stored records; none is estimated and none is a target. A
// metric with no samples is null, not zero. The figures describe the synthetic data in
// the period asked about and say nothing about real traffic.
//
// Not implemented on purpose: "complaints prevented". The pack asks for it only if the
// method is defined and approved, and no baseline method exists.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Definition describes one metric.
type Definition struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Definition  string   `json:"definition"`
	Formula     string   `json:"formula"`
	Sources     []string `json:"source_tables"`
	Period      string   `json:"period"`
	Unit        string   `json:"unit"`
	Limitations string   `json:"limitations"`
}

// Catalogue lists every metric this package computes.
var Catalogue = []Definition{
	{"exception_volume", "Exception volume", "Cases opened in the period, by classification and by source (bot or customer).", "count(cases) where opened_at in period",
		[]string{"cases"}, "cases opened in [from, to)", "cases", "Counts cases, not transactions; one transaction can have several cases."},
	{"detection_latency", "Detection latency", "Time from the last event of the transaction that the platform had received to the moment a detector opened the case. p50 and p95 by linear interpolation.",
		"p50 and p95 of (cases.opened_at - max(transaction_events.source_ts where received_at <= opened_at)) for bot cases", []string{"cases", "transaction_events"}, "bot cases opened in [from, to)", "seconds",
		"Measured from the latest event the case could rest on, so it excludes the wait for a source to send the deciding event. Rules that wait on a threshold include that wait. Not a performance measurement of the code."},
	{"time_to_first_investigation", "Time to first investigation", "Time from a case opening to the first time a person (not a system action) moved it to investigation.",
		"p50 and p95 of (first case_event with actor_type = 'user' and to_status = 'UNDER_INVESTIGATION') - opened_at", []string{"cases", "case_events"}, "cases opened in [from, to) that were investigated", "seconds",
		"Cases nobody has investigated yet are not counted, which flatters the figure when there is a backlog; see exception_volume and exposure."},
	{"resolution_time", "Resolution time", "Time from a case opening to its resolution or closure.", "p50 and p95 of (resolved_at - opened_at)", []string{"cases"}, "cases with resolved_at in [from, to)", "seconds",
		"Only finished cases count. A closed case that was reopened keeps its first resolution time."},
	{"customer_wait", "Customer wait", "Time from the original debit to a verified reversal.", "p50 and p95 of (reversals.verified_at - transactions.first_debit_at)", []string{"reversals", "transactions"},
		"reversals with verified_at in [from, to)", "seconds", "Reversals only. A transaction resolved without a reversal has no customer wait measured."},
	{"proactive_detection_rate", "Proactive detection rate", "Share of cases the bank found that no customer had reported.",
		"count(bot cases with no customer contact and no customer-reported case on the same transaction) / count(cases)", []string{"cases", "case_contacts"}, "cases opened in [from, to)", "ratio",
		"A customer who reports later is not counted against a case already open at the time. The pack forbids calling this complaints prevented; it is not."},
	{"sla_compliance", "SLA compliance", "Share of finished cases that carry a deadline and were finished by it.", "count(resolved_at <= sla_due_at) / count(cases with sla_due_at and resolved_at)",
		[]string{"cases"}, "cases with resolved_at in [from, to)", "ratio", "Only cases with an approved clock have a deadline. Cases without one are not counted either way."},
	{"reversal_rates", "Reversal rates", "Reversals created, and how many reached each of the three tracks.", "counts of accepted_status, processing_status and financial_status of reversals created in the period; completion = VERIFIED / created; failure = FAILED / created",
		[]string{"reversals"}, "reversals created in [from, to)", "count and ratio", "A reversal created late in the period may not have finished. Acceptance is not proof of posting: only VERIFIED counts as complete."},
	{"exposure", "Exposure", "Money flagged, submitted, reversed and still open, kept as separate numbers per currency.",
		"gross_flagged = sum(exposure of cases opened); submitted = sum(amount of reversals submitted); reversed = sum(amount of reversals VERIFIED); still_open = sum(exposure of non-terminal cases opened by the end of the period)",
		[]string{"cases", "reversals"}, "see each number", "minor units", "Exposure is the transaction amount at risk, not a loss. The four numbers are not summed to one."},
	{"false_positive_rate", "False-positive rate", "Share of finished bot cases that were closed because nothing was wrong.", "count(CLOSED_NO_EXCEPTION) / count(finished bot cases)",
		[]string{"cases"}, "bot cases with resolved_at in [from, to)", "ratio", "Counts what operations decided, which is itself a judgement. Unfinished cases are not counted."},
	{"automation", "Automation and manual intervention", "Share of verified reversals the system requested itself, and share of created reversals that needed a person.",
		"automation_rate = count(VERIFIED and requested_by_type = 'system') / count(VERIFIED); manual_intervention_rate = count(requested_by_type = 'user' or approval_status = 'APPROVED') / count(created)",
		[]string{"reversals"}, "reversals created (or verified) in [from, to)", "ratio", "An automatic reversal that was approved by a person counts as manual."},
	{"repeat_contact_rate", "Repeat contact rate", "Share of cases a customer contacted about that were contacted about more than once.", "count(cases with 2 or more contacts) / count(cases with 1 or more contacts)",
		[]string{"case_contacts"}, "cases opened in [from, to) with at least one contact", "ratio", "A contact is one customer report attached to the case."},
	{"recurring_root_causes", "Recurring root causes", "The root causes tagged on cases, most frequent first.", "count(cases) grouped by root_cause", []string{"cases"}, "cases opened in [from, to)", "cases",
		"Root causes are tagged by detectors and people and are as good as those tags."},
}

// Value is a computed metric.
type Value struct {
	Name      string             `json:"name"`
	Samples   int                `json:"samples"`
	Unit      string             `json:"unit"`
	P50       *float64           `json:"p50,omitempty"`
	P95       *float64           `json:"p95,omitempty"`
	Value     *float64           `json:"value,omitempty"`
	Breakdown map[string]float64 `json:"breakdown,omitempty"`
	// ByCurrency holds metrics that are money; each currency stands alone.
	ByCurrency map[string]map[string]int64 `json:"by_currency,omitempty"`
	Note       string                      `json:"note,omitempty"`
}

// Report is every metric for a period.
type Report struct {
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
	Simulated bool      `json:"simulated"`
	Metrics   []Value   `json:"metrics"`
}

// MaxPeriod bounds a request.
const MaxPeriod = 366 * 24 * time.Hour

// ErrPeriod means the period is empty, reversed or too long.
var ErrPeriod = errors.New("metrics: the period must be a positive span of at most 366 days")

type breakdown = map[string]float64

func ptr(f float64) *float64 { return &f }

// Compute calculates every metric for [from, to) in the tenant's transaction.
func Compute(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Report, error) {
	if !to.After(from) || to.Sub(from) > MaxPeriod {
		return Report{}, ErrPeriod
	}
	r := Report{From: from.UTC(), To: to.UTC(), Simulated: true}
	add := func(v Value, err error) error {
		if err != nil {
			return fmt.Errorf("metrics: %s: %w", v.Name, err)
		}
		r.Metrics = append(r.Metrics, v)
		return nil
	}
	fns := []func(context.Context, pgx.Tx, uuid.UUID, time.Time, time.Time) (Value, error){
		volume, detectionLatency, firstInvestigation, resolutionTime, customerWait, proactive, slaCompliance, reversalRates, exposure, falsePositives, automation, repeatContact, rootCauses,
	}
	for _, f := range fns {
		v, err := f(ctx, tx, tenant, from, to)
		if err := add(v, err); err != nil {
			return Report{}, err
		}
	}
	return r, nil
}

// percentiles runs a query returning one number per row (seconds) and summarises it.
func percentiles(ctx context.Context, tx pgx.Tx, name, sql string, args ...any) (Value, error) {
	v := Value{Name: name, Unit: "seconds"}
	var p50, p95 *float64
	err := tx.QueryRow(ctx, `SELECT count(x)::int, percentile_cont(0.5) WITHIN GROUP (ORDER BY x), percentile_cont(0.95) WITHIN GROUP (ORDER BY x) FROM (`+sql+`) q`, args...).Scan(&v.Samples, &p50, &p95)
	if err != nil {
		return v, err
	}
	v.P50, v.P95 = p50, p95
	if v.Samples == 0 {
		v.Note = "no samples in this period"
	}
	return v, nil
}

func groupCounts(ctx context.Context, tx pgx.Tx, sql string, args ...any) (breakdown, int, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := breakdown{}
	total := 0
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, 0, err
		}
		out[k] = float64(n)
		total += n
	}
	return out, total, rows.Err()
}

func volume(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	v := Value{Name: "exception_volume", Unit: "cases"}
	by, total, err := groupCounts(ctx, tx, `SELECT classification || '/' || source, count(*)::int FROM cases WHERE tenant_id = $1 AND opened_at >= $2 AND opened_at < $3 GROUP BY 1 ORDER BY 1`, tenant, from, to)
	v.Breakdown, v.Samples = by, total
	f := float64(total)
	v.Value = &f
	return v, err
}

func detectionLatency(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	return percentiles(ctx, tx, "detection_latency", `SELECT extract(epoch FROM (c.opened_at - e.last_ts))::float8 AS x FROM cases c
		JOIN LATERAL (SELECT max(source_ts) AS last_ts FROM transaction_events WHERE tenant_id = c.tenant_id AND txn_id = c.txn_id AND received_at <= c.opened_at) e ON e.last_ts IS NOT NULL
		WHERE c.tenant_id = $1 AND c.source = 'bot' AND c.txn_id IS NOT NULL AND c.opened_at >= $2 AND c.opened_at < $3`, tenant, from, to)
}

func firstInvestigation(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	return percentiles(ctx, tx, "time_to_first_investigation", `SELECT extract(epoch FROM (e.at - c.opened_at))::float8 AS x FROM cases c
		JOIN LATERAL (SELECT min(at) AS at FROM case_events WHERE tenant_id = c.tenant_id AND case_id = c.id AND actor_type = 'user' AND to_status = 'UNDER_INVESTIGATION') e ON e.at IS NOT NULL
		WHERE c.tenant_id = $1 AND c.opened_at >= $2 AND c.opened_at < $3`, tenant, from, to)
}

func resolutionTime(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	return percentiles(ctx, tx, "resolution_time", `SELECT extract(epoch FROM (resolved_at - opened_at))::float8 AS x FROM cases
		WHERE tenant_id = $1 AND resolved_at >= $2 AND resolved_at < $3`, tenant, from, to)
}

func customerWait(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	return percentiles(ctx, tx, "customer_wait", `SELECT extract(epoch FROM (r.verified_at - coalesce(t.first_debit_at, t.first_event_at)))::float8 AS x FROM reversals r
		JOIN transactions t ON t.tenant_id = r.tenant_id AND t.id = r.txn_id
		WHERE r.tenant_id = $1 AND r.financial_status = 'VERIFIED' AND r.verified_at >= $2 AND r.verified_at < $3`, tenant, from, to)
}

func ratio(ctx context.Context, tx pgx.Tx, name, sql string, args ...any) (Value, error) {
	v := Value{Name: name, Unit: "ratio"}
	var num, den int
	if err := tx.QueryRow(ctx, sql, args...).Scan(&num, &den); err != nil {
		return v, err
	}
	v.Samples = den
	if den == 0 {
		v.Note = "no samples in this period"
		return v, nil
	}
	v.Value = ptr(float64(num) / float64(den))
	v.Breakdown = breakdown{"numerator": float64(num), "denominator": float64(den)}
	return v, nil
}

func proactive(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	return ratio(ctx, tx, "proactive_detection_rate", `SELECT
		count(*) FILTER (WHERE c.source = 'bot'
			AND NOT EXISTS (SELECT 1 FROM case_contacts k WHERE k.tenant_id = c.tenant_id AND k.case_id = c.id)
			AND NOT EXISTS (SELECT 1 FROM cases o WHERE o.tenant_id = c.tenant_id AND o.txn_id = c.txn_id AND o.source = 'customer'))::int,
		count(*)::int FROM cases c WHERE c.tenant_id = $1 AND c.opened_at >= $2 AND c.opened_at < $3`, tenant, from, to)
}

func slaCompliance(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	return ratio(ctx, tx, "sla_compliance", `SELECT count(*) FILTER (WHERE resolved_at <= sla_due_at)::int, count(*)::int FROM cases
		WHERE tenant_id = $1 AND sla_due_at IS NOT NULL AND resolved_at >= $2 AND resolved_at < $3`, tenant, from, to)
}

func reversalRates(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	v := Value{Name: "reversal_rates", Unit: "count and ratio", Breakdown: breakdown{}}
	for _, col := range []string{"accepted_status", "processing_status", "financial_status"} {
		by, _, err := groupCounts(ctx, tx, `SELECT '`+col+`.' || `+col+`, count(*)::int FROM reversals WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3 GROUP BY 1`, tenant, from, to)
		if err != nil {
			return v, err
		}
		for k, n := range by {
			v.Breakdown[k] = n
		}
	}
	var created, verified, failed int
	if err := tx.QueryRow(ctx, `SELECT count(*)::int, count(*) FILTER (WHERE financial_status = 'VERIFIED')::int, count(*) FILTER (WHERE financial_status = 'FAILED')::int
		FROM reversals WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3`, tenant, from, to).Scan(&created, &verified, &failed); err != nil {
		return v, err
	}
	v.Samples = created
	v.Breakdown["created"] = float64(created)
	if created > 0 {
		v.Value = ptr(float64(verified) / float64(created)) // completion
		v.Breakdown["completion_rate"] = float64(verified) / float64(created)
		v.Breakdown["failure_rate"] = float64(failed) / float64(created)
	} else {
		v.Note = "no samples in this period"
	}
	return v, nil
}

func exposure(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	v := Value{Name: "exposure", Unit: "minor units", ByCurrency: map[string]map[string]int64{}}
	put := func(cur, key string, n int64) {
		if v.ByCurrency[cur] == nil {
			v.ByCurrency[cur] = map[string]int64{"gross_flagged": 0, "submitted": 0, "reversed": 0, "still_open": 0}
		}
		v.ByCurrency[cur][key] = n
	}
	queries := []struct {
		key, sql string
	}{
		{"gross_flagged", `SELECT coalesce(currency, ''), coalesce(sum(exposure_minor), 0)::bigint FROM cases WHERE tenant_id = $1 AND opened_at >= $2 AND opened_at < $3 AND currency IS NOT NULL GROUP BY 1`},
		{"submitted", `SELECT currency, coalesce(sum(amount_minor), 0)::bigint FROM reversals WHERE tenant_id = $1 AND submitted_at >= $2 AND submitted_at < $3 GROUP BY 1`},
		{"reversed", `SELECT currency, coalesce(sum(amount_minor), 0)::bigint FROM reversals WHERE tenant_id = $1 AND financial_status = 'VERIFIED' AND verified_at >= $2 AND verified_at < $3 GROUP BY 1`},
		{"still_open", `SELECT coalesce(currency, ''), coalesce(sum(exposure_minor), 0)::bigint FROM cases WHERE tenant_id = $1 AND $2::timestamptz IS NOT NULL AND opened_at < $3 AND currency IS NOT NULL
			AND status NOT IN ('RESOLVED', 'CLOSED', 'CLOSED_NO_EXCEPTION', 'CLOSED_CUSTOMER_ERROR') GROUP BY 1`},
	}
	for _, q := range queries {
		rows, err := tx.Query(ctx, q.sql, tenant, from, to)
		if err != nil {
			return v, err
		}
		for rows.Next() {
			var cur string
			var n int64
			if err := rows.Scan(&cur, &n); err != nil {
				rows.Close()
				return v, err
			}
			put(cur, q.key, n)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return v, err
		}
	}
	v.Samples = len(v.ByCurrency)
	return v, nil
}

func falsePositives(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	return ratio(ctx, tx, "false_positive_rate", `SELECT count(*) FILTER (WHERE status = 'CLOSED_NO_EXCEPTION')::int, count(*)::int FROM cases
		WHERE tenant_id = $1 AND source = 'bot' AND resolved_at >= $2 AND resolved_at < $3 AND status IN ('RESOLVED', 'CLOSED', 'CLOSED_NO_EXCEPTION', 'CLOSED_CUSTOMER_ERROR')`, tenant, from, to)
}

func automation(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	v := Value{Name: "automation", Unit: "ratio", Breakdown: breakdown{}}
	var verified, auto, created, manual int
	if err := tx.QueryRow(ctx, `SELECT
		(SELECT count(*)::int FROM reversals WHERE tenant_id = $1 AND financial_status = 'VERIFIED' AND verified_at >= $2 AND verified_at < $3),
		(SELECT count(*)::int FROM reversals WHERE tenant_id = $1 AND financial_status = 'VERIFIED' AND requested_by_type = 'system' AND approval_status <> 'APPROVED' AND verified_at >= $2 AND verified_at < $3),
		(SELECT count(*)::int FROM reversals WHERE tenant_id = $1 AND created_at >= $2 AND created_at < $3),
		(SELECT count(*)::int FROM reversals WHERE tenant_id = $1 AND (requested_by_type = 'user' OR approval_status = 'APPROVED') AND created_at >= $2 AND created_at < $3)`,
		tenant, from, to).Scan(&verified, &auto, &created, &manual); err != nil {
		return v, err
	}
	v.Samples = created
	v.Breakdown["verified"], v.Breakdown["automatic"], v.Breakdown["created"], v.Breakdown["needed_a_person"] = float64(verified), float64(auto), float64(created), float64(manual)
	if verified > 0 {
		v.Value = ptr(float64(auto) / float64(verified))
		v.Breakdown["automation_rate"] = float64(auto) / float64(verified)
	}
	if created > 0 {
		v.Breakdown["manual_intervention_rate"] = float64(manual) / float64(created)
	}
	if verified == 0 && created == 0 {
		v.Note = "no samples in this period"
	}
	return v, nil
}

func repeatContact(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	return ratio(ctx, tx, "repeat_contact_rate", `SELECT count(*) FILTER (WHERE n >= 2)::int, count(*)::int FROM (
		SELECT count(*) AS n FROM case_contacts k JOIN cases c ON c.tenant_id = k.tenant_id AND c.id = k.case_id
		WHERE k.tenant_id = $1 AND c.opened_at >= $2 AND c.opened_at < $3 GROUP BY k.case_id) q`, tenant, from, to)
}

func rootCauses(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, from, to time.Time) (Value, error) {
	v := Value{Name: "recurring_root_causes", Unit: "cases"}
	by, total, err := groupCounts(ctx, tx, `SELECT root_cause, count(*)::int FROM (SELECT root_cause FROM cases WHERE tenant_id = $1 AND opened_at >= $2 AND opened_at < $3 AND root_cause <> '') q
		GROUP BY root_cause ORDER BY count(*) DESC, root_cause LIMIT 10`, tenant, from, to)
	v.Breakdown, v.Samples = by, total
	return v, err
}
