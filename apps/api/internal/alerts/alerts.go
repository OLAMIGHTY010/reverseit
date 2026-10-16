// Package alerts raises internal alerts to the people who must act, once per condition.
//
// An alert is deduplicated: a condition that is still true is one alert with a count of
// how often it was raised, not a flood. During a known incident, alerts about its
// members are recorded as suppressed rather than paged. An alert is routed to a role
// and to the people on call for it; if no one is on call it is escalated along the
// configured steps, and if no one anywhere is on call it stays visible as unrouted.
// Work must not disappear. An unacknowledged alert escalates after an approved time;
// with no approved time it does not escalate by itself.
package alerts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/routing"
)

// Kinds, severities and statuses.
const (
	CriticalException   = "critical_exception"
	HighValueUnresolved = "high_value_unresolved"
	SLAApproaching      = "sla_approaching"
	SLABreached         = "sla_breached"
	ReversalFailure     = "reversal_failure"
	WidespreadIncident  = "widespread_incident"
	IntegrationOutage   = "integration_outage"
	ApprovalRequest     = "approval_request"
	UnusualSpike        = "unusual_spike"
	ApprovalExpired     = "approval_expired"

	Open         = "open"
	Acknowledged = "acknowledged"
	Suppressed   = "suppressed"
	Resolved     = "resolved"

	// MaxLevel is how far an alert escalates.
	MaxLevel = 3
)

var kinds = map[string]bool{CriticalException: true, HighValueUnresolved: true, SLAApproaching: true, SLABreached: true, ReversalFailure: true,
	WidespreadIncident: true, IntegrationOutage: true, ApprovalRequest: true, UnusualSpike: true, ApprovalExpired: true}

var severities = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}

// Errors.
var (
	ErrInvalid  = errors.New("alerts: invalid alert")
	ErrNotFound = errors.New("alerts: no such alert")
	ErrState    = errors.New("alerts: the alert is not in a state that allows this")
)

// Service raises and manages alerts.
type Service struct {
	Routing *routing.Service
	Config  *configsvc.Service
	Audit   *audit.Appender
	Now     func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Alert is a stored alert.
type Alert struct {
	ID              uuid.UUID   `json:"id"`
	Kind            string      `json:"kind"`
	Severity        string      `json:"severity"`
	DedupeKey       string      `json:"dedupe_key"`
	SubjectType     string      `json:"subject_type,omitempty"`
	SubjectRef      string      `json:"subject_ref,omitempty"`
	Body            string      `json:"body"`
	TargetRole      string      `json:"target_role"`
	RoutedTo        []uuid.UUID `json:"routed_to"`
	Unrouted        bool        `json:"unrouted"`
	Status          string      `json:"status"`
	SuppressedBy    *uuid.UUID  `json:"suppressed_by_incident,omitempty"`
	Occurrences     int         `json:"occurrences"`
	EscalationLevel int         `json:"escalation_level"`
	EscalatedRole   string      `json:"escalated_role,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	LastRaisedAt    time.Time   `json:"last_raised_at"`
	AcknowledgedBy  *uuid.UUID  `json:"acknowledged_by,omitempty"`
	AcknowledgedAt  *time.Time  `json:"acknowledged_at,omitempty"`
}

// Input is a condition to raise.
type Input struct {
	Kind        string
	Severity    string
	DedupeKey   string
	SubjectType string
	SubjectRef  string
	Body        string
	TargetRole  string
	// IncidentID, when set, says a known incident covers this condition, so it is
	// recorded as suppressed and pages no one.
	IncidentID *uuid.UUID
}

// Outcome of a raise.
const (
	Created     = "created"
	Repeated    = "repeated"
	Suppression = "suppressed"
)

const columns = `id, kind, severity, dedupe_key, subject_type, subject_ref, body, target_role, routed_to, unrouted, status, suppressed_by, occurrences,
	escalation_level, escalated_role, created_at, last_raised_at, acknowledged_by, acknowledged_at`

func scan(row pgx.Row) (Alert, error) {
	var a Alert
	err := row.Scan(&a.ID, &a.Kind, &a.Severity, &a.DedupeKey, &a.SubjectType, &a.SubjectRef, &a.Body, &a.TargetRole, &a.RoutedTo, &a.Unrouted, &a.Status, &a.SuppressedBy,
		&a.Occurrences, &a.EscalationLevel, &a.EscalatedRole, &a.CreatedAt, &a.LastRaisedAt, &a.AcknowledgedBy, &a.AcknowledgedAt)
	a.CreatedAt, a.LastRaisedAt = a.CreatedAt.UTC(), a.LastRaisedAt.UTC()
	return a, err
}

// Raise records a condition. If an unresolved alert for the same condition exists it
// counts the repeat and nothing else; otherwise it creates one. The body must carry no
// account numbers; callers pass references and amounts only.
func (s *Service) Raise(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, in Input) (Alert, string, error) {
	switch {
	case !kinds[in.Kind]:
		return Alert{}, "", fmt.Errorf("%w: unknown kind %q", ErrInvalid, in.Kind)
	case !severities[in.Severity]:
		return Alert{}, "", fmt.Errorf("%w: unknown severity %q", ErrInvalid, in.Severity)
	case strings.TrimSpace(in.DedupeKey) == "" || len(in.DedupeKey) > 200:
		return Alert{}, "", fmt.Errorf("%w: a dedupe key of 1 to 200 characters is required", ErrInvalid)
	case strings.TrimSpace(in.Body) == "" || len(in.Body) > 500:
		return Alert{}, "", fmt.Errorf("%w: a body of 1 to 500 characters is required", ErrInvalid)
	case strings.TrimSpace(in.TargetRole) == "":
		return Alert{}, "", fmt.Errorf("%w: a target role is required", ErrInvalid)
	}
	now := s.now()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenant.String()+"|alert|"+in.DedupeKey); err != nil {
		return Alert{}, "", err
	}
	existing, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM alerts WHERE tenant_id = $1 AND dedupe_key = $2 AND status <> 'resolved'`, tenant, in.DedupeKey))
	if err == nil {
		if _, err := tx.Exec(ctx, `UPDATE alerts SET occurrences = occurrences + 1, last_raised_at = $3 WHERE tenant_id = $1 AND id = $2`, tenant, existing.ID, now); err != nil {
			return Alert{}, "", err
		}
		existing.Occurrences++
		outcome := Repeated
		if existing.Status == Suppressed {
			outcome = Suppression
		}
		return existing, outcome, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Alert{}, "", err
	}

	status, users, unrouted := Open, []uuid.UUID(nil), false
	if in.IncidentID != nil {
		status = Suppressed
	} else {
		users, unrouted, err = s.route(ctx, tx, tenant, in.TargetRole, now)
		if err != nil {
			return Alert{}, "", err
		}
	}
	if users == nil {
		users = []uuid.UUID{}
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO alerts (tenant_id, kind, severity, dedupe_key, subject_type, subject_ref, body, target_role, routed_to, unrouted, status, suppressed_by, created_at, last_raised_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13) RETURNING id`,
		tenant, in.Kind, in.Severity, in.DedupeKey, in.SubjectType, in.SubjectRef, in.Body, in.TargetRole, users, unrouted, status, in.IncidentID, now).Scan(&id); err != nil {
		return Alert{}, "", err
	}
	a, err := s.Get(ctx, tx, tenant, id)
	if err != nil {
		return Alert{}, "", err
	}
	outcome := Created
	if status == Suppressed {
		outcome = Suppression
	}
	return a, outcome, nil
}

// route finds who is on call for the role, escalating along the configured steps when
// nobody is, and says when nobody anywhere is.
func (s *Service) route(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, role string, at time.Time) ([]uuid.UUID, bool, error) {
	if s.Routing == nil {
		return nil, true, nil
	}
	users, err := s.Routing.OnCall(ctx, tx, tenant, role, at)
	if err != nil {
		return nil, false, err
	}
	if len(users) > 0 {
		return users, false, nil
	}
	t, err := s.Routing.Escalate(ctx, tx, tenant, role, at)
	if errors.Is(err, routing.ErrNotFound) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return t.Users, false, nil
}

// Get loads one alert.
func (s *Service) Get(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (Alert, error) {
	a, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM alerts WHERE tenant_id = $1 AND id = $2`, tenant, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Alert{}, ErrNotFound
	}
	return a, err
}

// List returns alerts of a status (open when empty), newest first.
func (s *Service) List(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, status string) ([]Alert, error) {
	if status == "" {
		status = Open
	}
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM alerts WHERE tenant_id = $1 AND status = $2 ORDER BY created_at DESC, id LIMIT 200`, tenant, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Acknowledge records that a person has taken an open alert. It stops escalation. The
// caller checks that the person may act on it.
func (s *Service) Acknowledge(ctx context.Context, tx pgx.Tx, tenant, by, id uuid.UUID) (Alert, error) {
	tag, err := tx.Exec(ctx, `UPDATE alerts SET status = 'acknowledged', acknowledged_by = $3, acknowledged_at = $4 WHERE tenant_id = $1 AND id = $2 AND status = 'open'`, tenant, id, by, s.now())
	if err != nil {
		return Alert{}, err
	}
	if tag.RowsAffected() == 0 {
		a, err := s.Get(ctx, tx, tenant, id)
		if err != nil {
			return Alert{}, err
		}
		return a, ErrState
	}
	a, err := s.Get(ctx, tx, tenant, id)
	if err != nil {
		return a, err
	}
	return a, s.audit(ctx, tx, tenant, by, "alert.acknowledged", a, nil)
}

// Resolve closes an alert because its condition has cleared. A later occurrence of the
// condition raises a fresh one.
func (s *Service) Resolve(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, dedupeKey string) error {
	_, err := tx.Exec(ctx, `UPDATE alerts SET status = 'resolved', resolved_at = $3 WHERE tenant_id = $1 AND dedupe_key = $2 AND status <> 'resolved'`, tenant, dedupeKey, s.now())
	return err
}

func (s *Service) audit(ctx context.Context, tx pgx.Tx, tenant, by uuid.UUID, action string, a Alert, extra map[string]any) error {
	if s.Audit == nil {
		return nil
	}
	d := map[string]any{"kind": a.Kind, "severity": a.Severity, "target_role": a.TargetRole}
	for k, v := range extra {
		d[k] = v
	}
	actor := audit.ActorUser
	if by == uuid.Nil {
		actor = audit.ActorSystem
	}
	_, err := s.Audit.Append(ctx, tx, tenant, audit.Event{Actor: actor, ActorID: by, Action: action, SubjectType: "alert", SubjectID: a.ID.String(), Details: d})
	return err
}

// Escalate moves every open, unacknowledged alert that has waited longer than the
// approved time to the next role that has someone on call, one level at a time, up to
// MaxLevel. With no approved alert.escalate_after_minutes nothing escalates by itself.
// It returns how many alerts it escalated.
func (s *Service) Escalate(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (int, error) {
	if s.Routing == nil || s.Config == nil {
		return 0, nil
	}
	v, err := s.Config.Effective(ctx, tx, tenant, "alert.escalate_after_minutes", s.now())
	if errors.Is(err, configsvc.ErrNotConfigured) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	wait := time.Duration(v.Value) * time.Minute
	now := s.now()
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM alerts WHERE tenant_id = $1 AND status = 'open' AND escalation_level < $2
		AND coalesce(escalated_at, created_at) <= $3 ORDER BY created_at, id`, tenant, MaxLevel, now.Add(-wait))
	if err != nil {
		return 0, err
	}
	var due []Alert
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	n := 0
	for _, a := range due {
		from := a.TargetRole
		if a.EscalatedRole != "" {
			from = a.EscalatedRole
		}
		t, err := s.Routing.Escalate(ctx, tx, tenant, from, now)
		if errors.Is(err, routing.ErrNotFound) {
			// Nobody further up is on call. The alert stays open and visible, and the
			// attempt is not repeated until another wait has passed.
			if _, err := tx.Exec(ctx, `UPDATE alerts SET escalated_at = $3, unrouted = true WHERE tenant_id = $1 AND id = $2`, tenant, a.ID, now); err != nil {
				return n, err
			}
			continue
		}
		if err != nil {
			return n, err
		}
		if _, err := tx.Exec(ctx, `UPDATE alerts SET escalation_level = escalation_level + 1, escalated_role = $3, routed_to = $4, unrouted = false, escalated_at = $5 WHERE tenant_id = $1 AND id = $2`,
			tenant, a.ID, t.Role, t.Users, now); err != nil {
			return n, err
		}
		a.EscalatedRole = t.Role
		if err := s.audit(ctx, tx, tenant, uuid.Nil, "alert.escalated", a, map[string]any{"to_role": t.Role, "level": a.EscalationLevel + 1}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
