// Package routing decides who a piece of work goes to when the usual person is
// away: delegations, on-call shifts and escalation steps.
//
// It routes; it does not authorise. A delegate still needs the right role and
// passes the same permission, MFA and maker-checker checks as anyone else, so a
// delegation can never lend authority the delegate does not already hold.
package routing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
)

// Errors.
var (
	ErrInvalid      = errors.New("routing: invalid input")
	ErrNotFound     = errors.New("routing: not found")
	ErrNotYours     = errors.New("routing: only the delegator can revoke a delegation")
	ErrNoSharedRole = errors.New("routing: the delegate holds none of the delegator's roles")
	ErrAlreadyDone  = errors.New("routing: already revoked")
)

// maxHops bounds how far a chain of delegations is followed, so a loop or a long
// chain cannot send work somewhere unexpected.
const maxHops = 3

// Service holds no state.
type Service struct {
	Audit *audit.Appender
	Now   func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Delegation hands incoming work from one person to another for a time.
type Delegation struct {
	ID        uuid.UUID
	Delegator uuid.UUID
	Delegate  uuid.UUID
	StartsAt  time.Time
	EndsAt    time.Time
	Reason    string
	Revoked   bool
}

// Delegate records that actor's work goes to delegate between starts and ends.
// Only a person can delegate their own work. The delegate must be an active user
// who holds at least one of the delegator's roles, and the window is at most 30
// days. Several delegations may exist; the one that started first and is still
// active wins.
func (s *Service) Delegate(ctx context.Context, tx pgx.Tx, tenant, actor, delegate uuid.UUID, starts, ends time.Time, reason string) (Delegation, error) {
	now := s.now()
	switch {
	case actor == uuid.Nil || delegate == uuid.Nil || actor == delegate:
		return Delegation{}, fmt.Errorf("%w: delegate must be a different person", ErrInvalid)
	case len(strings.TrimSpace(reason)) < 5:
		return Delegation{}, fmt.Errorf("%w: a reason is required", ErrInvalid)
	case !ends.After(starts):
		return Delegation{}, fmt.Errorf("%w: the end must be after the start", ErrInvalid)
	case ends.Sub(starts) > 30*24*time.Hour:
		return Delegation{}, fmt.Errorf("%w: a delegation lasts at most 30 days", ErrInvalid)
	case starts.Before(now.Add(-time.Minute)):
		return Delegation{}, fmt.Errorf("%w: a delegation cannot start in the past", ErrInvalid)
	}

	var active, shared bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE id = $2 AND status = 'active'),
		       EXISTS (SELECT 1 FROM user_roles a JOIN user_roles b ON a.role = b.role WHERE a.user_id = $1 AND b.user_id = $2)`,
		actor, delegate).Scan(&active, &shared); err != nil {
		return Delegation{}, fmt.Errorf("routing: check delegate: %w", err)
	}
	if !active {
		return Delegation{}, fmt.Errorf("%w: the delegate is not an active user", ErrNotFound)
	}
	if !shared {
		return Delegation{}, ErrNoSharedRole
	}

	d := Delegation{ID: uuid.New(), Delegator: actor, Delegate: delegate, StartsAt: starts.UTC(), EndsAt: ends.UTC(), Reason: strings.TrimSpace(reason)}
	if _, err := tx.Exec(ctx, `
		INSERT INTO delegations (tenant_id, id, delegator, delegate, starts_at, ends_at, reason, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, tenant, d.ID, actor, delegate, d.StartsAt, d.EndsAt, d.Reason, now); err != nil {
		return Delegation{}, fmt.Errorf("routing: insert delegation: %w", err)
	}
	if _, err := s.Audit.Append(ctx, tx, tenant, audit.Event{
		Actor: audit.ActorUser, ActorID: actor, Action: "delegation.created", SubjectType: "delegation", SubjectID: d.ID.String(),
		Details: map[string]any{"delegate": delegate.String(), "starts_at": d.StartsAt.Format(time.RFC3339), "ends_at": d.EndsAt.Format(time.RFC3339)},
	}); err != nil {
		return Delegation{}, err
	}
	return d, nil
}

// Revoke ends a delegation early. Only the delegator may do it.
func (s *Service) Revoke(ctx context.Context, tx pgx.Tx, tenant, actor, id uuid.UUID) error {
	var delegator uuid.UUID
	err := tx.QueryRow(ctx, `SELECT delegator FROM delegations WHERE tenant_id = $1 AND id = $2`, tenant, id).Scan(&delegator)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if delegator != actor {
		return ErrNotYours
	}
	tag, err := tx.Exec(ctx, `INSERT INTO delegation_revocations (tenant_id, delegation_id, revoked_by, revoked_at)
		VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, tenant, id, actor, s.now())
	if err != nil {
		return fmt.Errorf("routing: revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAlreadyDone
	}
	_, err = s.Audit.Append(ctx, tx, tenant, audit.Event{
		Actor: audit.ActorUser, ActorID: actor, Action: "delegation.revoked", SubjectType: "delegation", SubjectID: id.String(),
	})
	return err
}

// Assignee follows active delegations from user and returns who should receive
// the work at time at, up to three hops. Via lists the delegations followed, in
// order. A loop stops at the first repeated person.
type Routed struct {
	Assignee uuid.UUID
	Via      []uuid.UUID
}

// Assignee returns where work addressed to user goes at time at.
func (s *Service) Assignee(ctx context.Context, tx pgx.Tx, tenant, user uuid.UUID, at time.Time) (Routed, error) {
	out := Routed{Assignee: user}
	seen := map[uuid.UUID]bool{user: true}
	for hop := 0; hop < maxHops; hop++ {
		var id, to uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT d.id, d.delegate FROM delegations d
			 WHERE d.tenant_id = $1 AND d.delegator = $2 AND d.starts_at <= $3 AND d.ends_at > $3
			   AND NOT EXISTS (SELECT 1 FROM delegation_revocations r WHERE r.tenant_id = d.tenant_id AND r.delegation_id = d.id)
			   AND EXISTS (SELECT 1 FROM users u WHERE u.id = d.delegate AND u.status = 'active')
			 ORDER BY d.starts_at, d.id LIMIT 1`, tenant, out.Assignee, at.UTC()).Scan(&id, &to)
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return Routed{}, fmt.Errorf("routing: follow delegation: %w", err)
		}
		if seen[to] {
			return out, nil // a loop: stay with the last person reached
		}
		seen[to] = true
		out.Assignee = to
		out.Via = append(out.Via, id)
	}
	return out, nil
}

// AddShift puts a person on call for a role. The person must hold the role.
func (s *Service) AddShift(ctx context.Context, tx pgx.Tx, tenant, actor uuid.UUID, role string, user uuid.UUID, starts, ends time.Time) (uuid.UUID, error) {
	if !ends.After(starts) || ends.Sub(starts) > 31*24*time.Hour {
		return uuid.Nil, fmt.Errorf("%w: a shift ends after it starts and lasts at most 31 days", ErrInvalid)
	}
	var holds bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles ur JOIN users u ON u.id = ur.user_id
		WHERE ur.user_id = $1 AND ur.role = $2 AND u.status = 'active')`, user, role).Scan(&holds); err != nil {
		return uuid.Nil, fmt.Errorf("routing: check role: %w", err)
	}
	if !holds {
		return uuid.Nil, fmt.Errorf("%w: that person does not hold the role %s", ErrInvalid, role)
	}
	id := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO on_call_shifts (tenant_id, id, role, user_id, starts_at, ends_at, created_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, tenant, id, role, user, starts.UTC(), ends.UTC(), actor, s.now()); err != nil {
		return uuid.Nil, fmt.Errorf("routing: insert shift: %w", err)
	}
	_, err := s.Audit.Append(ctx, tx, tenant, audit.Event{
		Actor: audit.ActorUser, ActorID: actor, Action: "oncall.shift_added", SubjectType: "oncall", SubjectID: role,
		Details: map[string]any{"user": user.String(), "starts_at": starts.UTC().Format(time.RFC3339), "ends_at": ends.UTC().Format(time.RFC3339)},
	})
	return id, err
}

// OnCall returns the people on call for role at time at, after applying any
// delegations, without duplicates, in the order shifts started.
func (s *Service) OnCall(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, role string, at time.Time) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT s.user_id FROM on_call_shifts s JOIN users u ON u.id = s.user_id AND u.status = 'active'
		 WHERE s.tenant_id = $1 AND s.role = $2 AND s.starts_at <= $3 AND s.ends_at > $3
		 ORDER BY s.starts_at, s.id`, tenant, role, at.UTC())
	if err != nil {
		return nil, fmt.Errorf("routing: on call: %w", err)
	}
	var base []uuid.UUID
	for rows.Next() {
		var u uuid.UUID
		if err := rows.Scan(&u); err != nil {
			rows.Close()
			return nil, err
		}
		base = append(base, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	for _, u := range base {
		r, err := s.Assignee(ctx, tx, tenant, u, at)
		if err != nil {
			return nil, err
		}
		if !seen[r.Assignee] {
			seen[r.Assignee] = true
			out = append(out, r.Assignee)
		}
	}
	return out, nil
}

// SetEscalation says that work for fromRole goes to toRole when no one in
// fromRole is on call. Setting it again replaces the earlier step.
func (s *Service) SetEscalation(ctx context.Context, tx pgx.Tx, tenant, actor uuid.UUID, fromRole, toRole string) error {
	if fromRole == "" || toRole == "" || fromRole == toRole {
		return fmt.Errorf("%w: two different roles are required", ErrInvalid)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO escalation_steps (tenant_id, from_role, to_role, set_by, set_at) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (tenant_id, from_role) DO UPDATE SET to_role = EXCLUDED.to_role, set_by = EXCLUDED.set_by, set_at = EXCLUDED.set_at`,
		tenant, fromRole, toRole, actor, s.now()); err != nil {
		return fmt.Errorf("routing: set escalation: %w", err)
	}
	_, err := s.Audit.Append(ctx, tx, tenant, audit.Event{
		Actor: audit.ActorUser, ActorID: actor, Action: "escalation.set", SubjectType: "escalation", SubjectID: fromRole,
		Details: map[string]any{"from_role": fromRole, "to_role": toRole},
	})
	return err
}

// Target is where escalated work lands.
type Target struct {
	Role  string
	Users []uuid.UUID
}

// Escalate walks the escalation steps from fromRole and returns the first role
// that has someone on call, with those people. It follows at most five steps and
// stops at a loop. When no one anywhere is on call it returns ErrNotFound: work
// must not disappear, and the caller decides what to do with an unowned item.
func (s *Service) Escalate(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, fromRole string, at time.Time) (Target, error) {
	role := fromRole
	seen := map[string]bool{role: true}
	for i := 0; i < 5; i++ {
		var next string
		err := tx.QueryRow(ctx, `SELECT to_role FROM escalation_steps WHERE tenant_id = $1 AND from_role = $2`, tenant, role).Scan(&next)
		if errors.Is(err, pgx.ErrNoRows) {
			return Target{}, ErrNotFound
		}
		if err != nil {
			return Target{}, fmt.Errorf("routing: read escalation: %w", err)
		}
		if seen[next] {
			return Target{}, ErrNotFound
		}
		seen[next] = true
		users, err := s.OnCall(ctx, tx, tenant, next, at)
		if err != nil {
			return Target{}, err
		}
		if len(users) > 0 {
			return Target{Role: next, Users: users}, nil
		}
		role = next
	}
	return Target{}, ErrNotFound
}

// ActiveDelegations lists a person's delegations that have not ended or been revoked.
func (s *Service) ActiveDelegations(ctx context.Context, tx pgx.Tx, tenant, delegator uuid.UUID, at time.Time) ([]Delegation, error) {
	rows, err := tx.Query(ctx, `
		SELECT d.id, d.delegator, d.delegate, d.starts_at, d.ends_at, d.reason FROM delegations d
		 WHERE d.tenant_id = $1 AND d.delegator = $2 AND d.ends_at > $3
		   AND NOT EXISTS (SELECT 1 FROM delegation_revocations r WHERE r.tenant_id = d.tenant_id AND r.delegation_id = d.id)
		 ORDER BY d.starts_at`, tenant, delegator, at.UTC())
	if err != nil {
		return nil, fmt.Errorf("routing: list delegations: %w", err)
	}
	defer rows.Close()
	var out []Delegation
	for rows.Next() {
		var d Delegation
		if err := rows.Scan(&d.ID, &d.Delegator, &d.Delegate, &d.StartsAt, &d.EndsAt, &d.Reason); err != nil {
			return nil, err
		}
		d.StartsAt, d.EndsAt = d.StartsAt.UTC(), d.EndsAt.UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}
