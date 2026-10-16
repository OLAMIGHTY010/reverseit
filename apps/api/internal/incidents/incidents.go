// Package incidents groups exceptions that look like one problem.
//
// Exceptions on the same rail, channel and error signature inside a time window are
// grouped into one incident, so that a burst of timeouts from one switch is one thing to
// handle and one alert, not hundreds. Grouping needs two approved numbers, the window and
// the smallest group; without them nothing is grouped, because choosing them would be
// inventing a policy.
//
// An incident records correlation, not cause. It starts as "correlated" and a person may
// raise it to "probable" or "confirmed" with a note. Nothing here moves money.
package incidents

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/alerts"
	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
)

// Errors.
var (
	ErrNotFound = errors.New("incidents: no such incident")
	ErrInvalid  = errors.New("incidents: invalid request")
	ErrState    = errors.New("incidents: the incident is not in a state that allows this")
)

// Service groups cases into incidents.
type Service struct {
	Config *configsvc.Service
	Alerts *alerts.Service
	Audit  *audit.Appender
	Now    func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Incident is a group of exceptions.
type Incident struct {
	ID              uuid.UUID        `json:"id"`
	Ref             string           `json:"incident_ref"`
	Rail            string           `json:"rail"`
	Channel         string           `json:"channel"`
	Signature       string           `json:"signature"`
	Status          string           `json:"status"`
	Severity        string           `json:"severity"`
	StartAt         time.Time        `json:"start_at"`
	LastSeenAt      time.Time        `json:"last_seen_at"`
	AffectedCount   int              `json:"affected_count"`
	Exposure        map[string]int64 `json:"exposure_by_currency"`
	SuspectedCause  string           `json:"suspected_cause"`
	CausationStatus string           `json:"causation_status"`
	CausationNote   string           `json:"causation_note,omitempty"`
	ReviewNote      string           `json:"review_note,omitempty"`
	ResolvedAt      *time.Time       `json:"resolved_at,omitempty"`
}

// Result of a grouping pass.
type Result struct {
	Opened  int    `json:"opened"`
	Updated int    `json:"updated"`
	Linked  int    `json:"linked"`
	Skipped string `json:"skipped,omitempty"`
}

type groupKey struct{ rail, channel, signature string }

type member struct {
	id       uuid.UUID
	opened   time.Time
	priority string
	exposure int64
	currency string
}

// Group finds exceptions opened inside the approved window, groups them by rail,
// channel and error signature, and opens or extends an incident for each group at or
// above the approved size. It is idempotent: running it again changes nothing that has
// not changed. One alert is raised per incident.
func (s *Service) Group(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (Result, error) {
	var res Result
	now := s.now()
	win, err := s.Config.Effective(ctx, tx, tenant, "incident.group_window_minutes", now)
	if errors.Is(err, configsvc.ErrNotConfigured) {
		res.Skipped = "incident.group_window_minutes is not approved"
		return res, nil
	}
	if err != nil {
		return res, err
	}
	size, err := s.Config.Effective(ctx, tx, tenant, "incident.min_group_size", now)
	if errors.Is(err, configsvc.ErrNotConfigured) {
		res.Skipped = "incident.min_group_size is not approved"
		return res, nil
	}
	if err != nil {
		return res, err
	}
	window := time.Duration(win.Value) * time.Minute

	rows, err := tx.Query(ctx, `SELECT c.id, c.opened_at, c.priority, coalesce(c.exposure_minor, 0), coalesce(c.currency, ''), t.rail, t.channel,
			CASE WHEN c.root_cause <> '' THEN c.root_cause ELSE c.classification END
		FROM cases c JOIN transactions t ON t.tenant_id = c.tenant_id AND t.id = c.txn_id
		WHERE c.tenant_id = $1 AND c.opened_at > $2 AND c.opened_at <= $3
		  AND c.status NOT IN ('RESOLVED', 'CLOSED', 'CLOSED_NO_EXCEPTION', 'CLOSED_CUSTOMER_ERROR') ORDER BY c.opened_at, c.id`, tenant, now.Add(-window), now)
	if err != nil {
		return res, err
	}
	groups := map[groupKey][]member{}
	for rows.Next() {
		var m member
		var k groupKey
		if err := rows.Scan(&m.id, &m.opened, &m.priority, &m.exposure, &m.currency, &k.rail, &k.channel, &k.signature); err != nil {
			rows.Close()
			return res, err
		}
		groups[k] = append(groups[k], m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}
	keys := make([]groupKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })

	for _, k := range keys {
		ms := groups[k]
		// An open incident of this scope keeps growing however small today's group is.
		var inc Incident
		var haveOpen bool
		if e, err := s.openFor(ctx, tx, tenant, k); err == nil {
			inc, haveOpen = e, true
		} else if !errors.Is(err, ErrNotFound) {
			return res, err
		}
		if !haveOpen && int64(len(ms)) < size.Value {
			continue
		}
		if !haveOpen {
			var seq int64
			if err := tx.QueryRow(ctx, `SELECT nextval('incident_number_seq')`).Scan(&seq); err != nil {
				return res, err
			}
			ref := fmt.Sprintf("INC-%d-%05d", now.Year(), seq)
			if err := tx.QueryRow(ctx, `INSERT INTO incidents (tenant_id, incident_ref, rail, channel, signature, severity, start_at, last_seen_at, suspected_cause, created_at)
				VALUES ($1,$2,$3,$4,$5,'MEDIUM',$6,$6,$7,$6) RETURNING id`, tenant, ref, k.rail, k.channel, k.signature, ms[0].opened, k.signature).Scan(&inc.ID); err != nil {
				return res, err
			}
			res.Opened++
			if err := s.audit(ctx, tx, tenant, "incident.opened", ref, map[string]any{"rail": k.rail, "channel": k.channel, "signature": k.signature, "cases": len(ms)}); err != nil {
				return res, err
			}
		}
		for _, m := range ms {
			tag, err := tx.Exec(ctx, `INSERT INTO incident_cases (tenant_id, incident_id, case_id, linked_at) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, tenant, inc.ID, m.id, now)
			if err != nil {
				return res, err
			}
			res.Linked += int(tag.RowsAffected())
		}
		if err := s.refresh(ctx, tx, tenant, inc.ID, now); err != nil {
			return res, err
		}
		if haveOpen {
			res.Updated++
		}
		if s.Alerts != nil {
			if err := s.raise(ctx, tx, tenant, inc.ID); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

func (s *Service) openFor(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, k groupKey) (Incident, error) {
	return s.one(ctx, tx, `WHERE tenant_id = $1 AND rail = $2 AND channel = $3 AND signature = $4 AND status = 'open'`, tenant, k.rail, k.channel, k.signature)
}

const cols = `id, incident_ref, rail, channel, signature, status, severity, start_at, last_seen_at, affected_count, exposure, suspected_cause, causation_status, causation_note, review_note, resolved_at`

func (s *Service) one(ctx context.Context, tx pgx.Tx, where string, args ...any) (Incident, error) {
	var i Incident
	var exp map[string]int64
	err := tx.QueryRow(ctx, `SELECT `+cols+` FROM incidents `+where, args...).Scan(&i.ID, &i.Ref, &i.Rail, &i.Channel, &i.Signature, &i.Status, &i.Severity, &i.StartAt, &i.LastSeenAt,
		&i.AffectedCount, &exp, &i.SuspectedCause, &i.CausationStatus, &i.CausationNote, &i.ReviewNote, &i.ResolvedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return i, ErrNotFound
	}
	i.Exposure = exp
	i.StartAt, i.LastSeenAt = i.StartAt.UTC(), i.LastSeenAt.UTC()
	return i, err
}

// refresh recomputes an incident's counts, exposure, severity and span from its members.
func (s *Service) refresh(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, now time.Time) error {
	rows, err := tx.Query(ctx, `SELECT c.opened_at, c.priority, coalesce(c.exposure_minor, 0), coalesce(c.currency, '') FROM incident_cases i
		JOIN cases c ON c.tenant_id = i.tenant_id AND c.id = i.case_id WHERE i.tenant_id = $1 AND i.incident_id = $2`, tenant, id)
	if err != nil {
		return err
	}
	var n int
	exp := map[string]int64{}
	sev := "MEDIUM"
	var first, last time.Time
	for rows.Next() {
		var opened time.Time
		var prio, cur string
		var amt int64
		if err := rows.Scan(&opened, &prio, &amt, &cur); err != nil {
			rows.Close()
			return err
		}
		n++
		if cur != "" {
			exp[cur] += amt
		}
		if prio == "CRITICAL" || (prio == "HIGH" && sev != "CRITICAL") {
			sev = prio
		}
		if first.IsZero() || opened.Before(first) {
			first = opened
		}
		if opened.After(last) {
			last = opened
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE incidents SET affected_count = $3, exposure = $4, severity = $5, start_at = $6, last_seen_at = $7 WHERE tenant_id = $1 AND id = $2`,
		tenant, id, n, exp, sev, first.UTC(), last.UTC())
	return err
}

func (s *Service) raise(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) error {
	i, err := s.one(ctx, tx, `WHERE tenant_id = $1 AND id = $2`, tenant, id)
	if err != nil {
		return err
	}
	_, _, err = s.Alerts.Raise(ctx, tx, tenant, alerts.Input{Kind: alerts.WidespreadIncident, Severity: i.Severity, DedupeKey: "incident:" + id.String(), SubjectType: "incident", SubjectRef: i.Ref,
		Body:       fmt.Sprintf("Incident %s: %d exceptions on %s via %s with the signature %q since %s.", i.Ref, i.AffectedCount, i.Rail, i.Channel, i.Signature, i.StartAt.Format("15:04")),
		TargetRole: "HEAD_OPS"})
	return err
}

func (s *Service) audit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, action, ref string, details map[string]any) error {
	if s.Audit == nil {
		return nil
	}
	_, err := s.Audit.Append(ctx, tx, tenant, audit.Event{Actor: audit.ActorSystem, Action: action, SubjectType: "incident", SubjectID: ref, Details: details})
	return err
}

// Covering returns an alerts.Covered function: the open incident a case belongs to, if any.
func Covering() alerts.Covered {
	return func(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID) (*uuid.UUID, error) {
		var id uuid.UUID
		err := tx.QueryRow(ctx, `SELECT i.incident_id FROM incident_cases i JOIN incidents n ON n.tenant_id = i.tenant_id AND n.id = i.incident_id AND n.status = 'open'
			WHERE i.tenant_id = $1 AND i.case_id = $2`, tenant, caseID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &id, nil
	}
}

// Get loads one incident by id or reference.
func (s *Service) Get(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, idOrRef string) (Incident, error) {
	if id, err := uuid.Parse(idOrRef); err == nil {
		return s.one(ctx, tx, `WHERE tenant_id = $1 AND id = $2`, tenant, id)
	}
	return s.one(ctx, tx, `WHERE tenant_id = $1 AND incident_ref = $2`, tenant, idOrRef)
}

// List returns incidents of a status (open when empty), newest first.
func (s *Service) List(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, status string) ([]Incident, error) {
	if status == "" {
		status = "open"
	}
	rows, err := tx.Query(ctx, `SELECT id FROM incidents WHERE tenant_id = $1 AND status = $2 ORDER BY start_at DESC, id LIMIT 200`, tenant, status)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := []Incident{}
	for _, id := range ids {
		i, err := s.one(ctx, tx, `WHERE tenant_id = $1 AND id = $2`, tenant, id)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// CaseIDs returns the cases in an incident, so a screen can go from incident to
// transactions and back.
func (s *Service) CaseIDs(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT case_id FROM incident_cases WHERE tenant_id = $1 AND incident_id = $2 ORDER BY linked_at, case_id`, tenant, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var c uuid.UUID
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetCausation records a person's judgement of cause. It may only move up
// (correlated, probable, confirmed), and a reason is required.
func (s *Service) SetCausation(ctx context.Context, tx pgx.Tx, tenant, by, id uuid.UUID, status, cause, note string) (Incident, error) {
	order := map[string]int{"correlated": 0, "probable": 1, "confirmed": 2}
	if _, ok := order[status]; !ok {
		return Incident{}, fmt.Errorf("%w: status must be correlated, probable or confirmed", ErrInvalid)
	}
	if len(strings.TrimSpace(note)) < 5 || len(note) > 500 {
		return Incident{}, fmt.Errorf("%w: a note of 5 to 500 characters is required", ErrInvalid)
	}
	i, err := s.one(ctx, tx, `WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenant, id)
	if err != nil {
		return i, err
	}
	if order[status] < order[i.CausationStatus] {
		return i, fmt.Errorf("%w: the cause cannot be downgraded from %s", ErrState, i.CausationStatus)
	}
	if cause = strings.TrimSpace(cause); cause == "" {
		cause = i.SuspectedCause
	}
	if _, err := tx.Exec(ctx, `UPDATE incidents SET causation_status = $3, suspected_cause = $4, causation_note = $5 WHERE tenant_id = $1 AND id = $2`, tenant, id, status, cause, strings.TrimSpace(note)); err != nil {
		return i, err
	}
	if s.Audit != nil {
		if _, err := s.Audit.Append(ctx, tx, tenant, audit.Event{Actor: audit.ActorUser, ActorID: by, Action: "incident.causation", SubjectType: "incident", SubjectID: i.Ref,
			Details: map[string]any{"causation_status": status, "suspected_cause": cause, "note": note}}); err != nil {
			return i, err
		}
	}
	return s.one(ctx, tx, `WHERE tenant_id = $1 AND id = $2`, tenant, id)
}

// Resolve closes an incident with a post-incident review note. Its alert is resolved.
// It does not close the member cases.
func (s *Service) Resolve(ctx context.Context, tx pgx.Tx, tenant, by, id uuid.UUID, reviewNote string) (Incident, error) {
	if len(strings.TrimSpace(reviewNote)) < 10 || len(reviewNote) > 1000 {
		return Incident{}, fmt.Errorf("%w: a review note of 10 to 1000 characters is required", ErrInvalid)
	}
	i, err := s.one(ctx, tx, `WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenant, id)
	if err != nil {
		return i, err
	}
	if i.Status != "open" {
		return i, ErrState
	}
	if _, err := tx.Exec(ctx, `UPDATE incidents SET status = 'resolved', resolved_at = $3, reviewed_by = $4, review_note = $5 WHERE tenant_id = $1 AND id = $2`, tenant, id, s.now(), by, strings.TrimSpace(reviewNote)); err != nil {
		return i, err
	}
	if s.Alerts != nil {
		if err := s.Alerts.Resolve(ctx, tx, tenant, "incident:"+id.String()); err != nil {
			return i, err
		}
	}
	if s.Audit != nil {
		if _, err := s.Audit.Append(ctx, tx, tenant, audit.Event{Actor: audit.ActorUser, ActorID: by, Action: "incident.resolved", SubjectType: "incident", SubjectID: i.Ref,
			Details: map[string]any{"review_note": reviewNote}}); err != nil {
			return i, err
		}
	}
	return s.one(ctx, tx, `WHERE tenant_id = $1 AND id = $2`, tenant, id)
}
