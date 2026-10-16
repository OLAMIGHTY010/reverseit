package incidents

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/alerts"
	"github.com/smartreversal/smartreversal/apps/api/internal/bizcal"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
)

// Spike is an hour with more exceptions than the same hour of earlier weeks would lead
// anyone to expect.
type Spike struct {
	Rail      string  `json:"rail"`
	Channel   string  `json:"channel"`
	Count     int     `json:"count"`
	Mean      float64 `json:"baseline_mean"`
	StdDev    float64 `json:"baseline_std_dev"`
	Weeks     int     `json:"baseline_weeks"`
	Threshold float64 `json:"threshold"`
}

// SpikeResult is what a spike pass found, and why it did not look when it did not.
type SpikeResult struct {
	Spikes  []Spike `json:"spikes"`
	Skipped string  `json:"skipped,omitempty"`
}

// DetectSpikes is a deterministic spike detector. For each rail and channel it counts
// exceptions opened in the last hour and compares that with the same hour of the week
// in earlier weeks: a spike is a count at least the approved minimum and above the
// mean plus the approved number of standard deviations. Only weeks with data on a
// business day count towards the baseline, a holiday never does, and nothing is judged
// on a day that is not a business day, because its baseline would be wrong. With fewer
// weeks of history than approved, or any of the three numbers unapproved, it says so
// and judges nothing. A spike raises an alert; it does not open an incident.
func (s *Service) DetectSpikes(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (SpikeResult, error) {
	res := SpikeResult{Spikes: []Spike{}}
	now := s.now()
	get := func(key string) (int64, bool, error) {
		v, err := s.Config.Effective(ctx, tx, tenant, key, now)
		if errors.Is(err, configsvc.ErrNotConfigured) {
			return 0, false, nil
		}
		return v.Value, err == nil, err
	}
	sigma, ok1, err := get("incident.spike_sigma_tenths")
	if err != nil {
		return res, err
	}
	minWeeks, ok2, err := get("incident.spike_min_weeks")
	if err != nil {
		return res, err
	}
	minCount, ok3, err := get("incident.spike_min_count")
	if err != nil {
		return res, err
	}
	if !ok1 || !ok2 || !ok3 {
		res.Skipped = "incident.spike_sigma_tenths, incident.spike_min_weeks and incident.spike_min_count must all be approved"
		return res, nil
	}
	cal, err := bizcal.Load(ctx, tx, s.Config, tenant, now)
	if err != nil {
		return res, err
	}
	isBusiness := func(t time.Time) bool {
		l := t.In(cal.Location())
		return cal.IsBusinessDay(bizcal.Date{Year: l.Year(), Month: l.Month(), Day: l.Day()})
	}
	if !isBusiness(now) {
		res.Skipped = "today is not a business day, so there is no comparable baseline"
		return res, nil
	}

	// Which weeks count: a business day on which the platform saw transactions.
	const lookback = 12
	var weeks []int
	for k := 1; k <= lookback; k++ {
		at := now.AddDate(0, 0, -7*k)
		if !isBusiness(at) {
			continue
		}
		l := at.In(cal.Location())
		day0 := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, cal.Location())
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*)::int FROM transactions WHERE tenant_id = $1 AND first_event_at >= $2 AND first_event_at < $3`, tenant, day0, day0.AddDate(0, 0, 1)).Scan(&n); err != nil {
			return res, err
		}
		if n > 0 {
			weeks = append(weeks, k)
		}
	}
	if int64(len(weeks)) < minWeeks {
		res.Skipped = fmt.Sprintf("only %d comparable weeks of history, %d are needed", len(weeks), minWeeks)
		return res, nil
	}

	count := func(from, to time.Time) (map[[2]string]int, error) {
		rows, err := tx.Query(ctx, `SELECT t.rail, t.channel, count(*)::int FROM cases c JOIN transactions t ON t.tenant_id = c.tenant_id AND t.id = c.txn_id
			WHERE c.tenant_id = $1 AND c.opened_at > $2 AND c.opened_at <= $3 GROUP BY 1, 2`, tenant, from, to)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := map[[2]string]int{}
		for rows.Next() {
			var rail, ch string
			var n int
			if err := rows.Scan(&rail, &ch, &n); err != nil {
				return nil, err
			}
			out[[2]string{rail, ch}] = n
		}
		return out, rows.Err()
	}
	current, err := count(now.Add(-time.Hour), now)
	if err != nil {
		return res, err
	}
	history := make([]map[[2]string]int, 0, len(weeks))
	for _, k := range weeks {
		end := now.AddDate(0, 0, -7*k)
		h, err := count(end.Add(-time.Hour), end)
		if err != nil {
			return res, err
		}
		history = append(history, h)
	}

	keys := make([][2]string, 0, len(current))
	for k := range current {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
	for _, k := range keys {
		c := current[k]
		var sum float64
		for _, h := range history {
			sum += float64(h[k])
		}
		mean := sum / float64(len(history))
		var sq float64
		for _, h := range history {
			d := float64(h[k]) - mean
			sq += d * d
		}
		sd := math.Sqrt(sq / float64(len(history)))
		threshold := mean + float64(sigma)/10*sd
		if int64(c) < minCount || float64(c) <= threshold {
			continue
		}
		sp := Spike{Rail: k[0], Channel: k[1], Count: c, Mean: mean, StdDev: sd, Weeks: len(history), Threshold: threshold}
		res.Spikes = append(res.Spikes, sp)
		if s.Alerts != nil {
			l := now.In(cal.Location())
			key := fmt.Sprintf("spike:%s:%s:%s", k[0], k[1], l.Format("2006-01-02T15"))
			if _, _, err := s.Alerts.Raise(ctx, tx, tenant, alerts.Input{Kind: alerts.UnusualSpike, Severity: "MEDIUM", DedupeKey: key, SubjectType: "rail", SubjectRef: k[0] + "/" + k[1],
				Body:       fmt.Sprintf("%d exceptions in the last hour on %s via %s, against an average of %.1f (standard deviation %.1f) for this hour over %d earlier weeks.", c, k[0], k[1], mean, sd, len(history)),
				TargetRole: "HEAD_OPS"}); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}
