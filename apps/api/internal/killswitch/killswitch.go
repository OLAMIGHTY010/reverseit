// Package killswitch freezes automation at four scopes: everything, one rail, one
// exception type, or one policy. Freezing takes one person and is immediate.
// Lifting a freeze takes two different people. Every consumer that could act on
// money must call Check first and stop on any error: a failure to read the
// switch counts as frozen.
package killswitch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
)

// Scope is how wide a freeze is.
type Scope string

const (
	ScopeGlobal        Scope = "global"
	ScopeRail          Scope = "rail"
	ScopeExceptionType Scope = "exception_type"
	ScopePolicy        Scope = "policy"
)

// Errors.
var (
	ErrInvalid        = errors.New("killswitch: invalid input")
	ErrNotFrozen      = errors.New("killswitch: this scope is not frozen")
	ErrPending        = errors.New("killswitch: an unfreeze request is already open for this scope")
	ErrNoSuchRequest  = errors.New("killswitch: no such open unfreeze request")
	ErrSameActor      = errors.New("killswitch: the requester cannot decide their own unfreeze request")
	ErrAlreadyDecided = errors.New("killswitch: this unfreeze request was already decided")
)

// FrozenError is returned by Check when an action is blocked.
type FrozenError struct {
	Scope  Scope
	Target string
}

func (e *FrozenError) Error() string {
	if e.Target == "" {
		return fmt.Sprintf("killswitch: automation is frozen (%s)", e.Scope)
	}
	return fmt.Sprintf("killswitch: automation is frozen (%s %s)", e.Scope, e.Target)
}

// Target names what a freeze applies to. Value is empty for the global scope.
type Target struct {
	Scope Scope
	Value string
}

func (t Target) validate() error {
	ok := false
	switch t.Scope {
	case ScopeGlobal:
		ok = t.Value == ""
	case ScopeRail:
		ok = matches(t.Value, 2, 30, func(r rune) bool { return isUpperDigit(r) || r == '_' })
	case ScopeExceptionType:
		ok = matches(t.Value, 3, 30, func(r rune) bool { return isUpperDigit(r) || r == '_' || r == '-' })
	case ScopePolicy:
		ok = matches(t.Value, 1, 60, func(r rune) bool {
			return isUpperDigit(r) || (r >= 'a' && r <= 'z') || r == '.' || r == '_' || r == '-'
		})
	}
	if !ok {
		return fmt.Errorf("%w: scope %q cannot take target %q", ErrInvalid, t.Scope, t.Value)
	}
	return nil
}

func isUpperDigit(r rune) bool { return (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') }

func matches(s string, min, max int, ok func(rune) bool) bool {
	if len(s) < min || len(s) > max {
		return false
	}
	for _, r := range s {
		if !ok(r) {
			return false
		}
	}
	return true
}

// Frozen describes one frozen scope.
type Frozen struct {
	Target Target
	Seq    int64
	By     uuid.UUID
	Reason string
	At     time.Time
}

// Pending is an open request to lift a freeze.
type Pending struct {
	Target      Target
	RequestSeq  int64
	RequestedBy uuid.UUID
	Reason      string
	At          time.Time
}

// State is the current switch position for a tenant.
type State struct {
	Frozen  []Frozen
	Pending []Pending
}

// Action is what a consumer is about to do. Leave a field empty when it does not
// apply, such as no policy.
type Action struct {
	Rail          string
	ExceptionType string
	PolicyID      string
	// PolicyUnknown says the action is governed by a policy that could not be
	// identified. Any policy-scope freeze then covers it: a missing identity must
	// never let an action slip past a freeze.
	PolicyUnknown bool
}

// Service reads and changes the switch.
type Service struct {
	Audit *audit.Appender
}

// State returns the frozen scopes and open unfreeze requests.
func (s *Service) State(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (State, error) {
	var st State
	rows, err := tx.Query(ctx, `
		SELECT scope, target, seq, actor_id, reason, created_at FROM (
			SELECT DISTINCT ON (scope, target) scope, target, kind, seq, actor_id, reason, created_at
			  FROM kill_switch_events
			 WHERE tenant_id = $1 AND kind IN ('freeze', 'unfreeze_approve')
			 ORDER BY scope, target, seq DESC) latest
		WHERE kind = 'freeze' ORDER BY seq`, tenant)
	if err != nil {
		return State{}, fmt.Errorf("killswitch: read state: %w", err)
	}
	for rows.Next() {
		var f Frozen
		var scope string
		if err := rows.Scan(&scope, &f.Target.Value, &f.Seq, &f.By, &f.Reason, &f.At); err != nil {
			rows.Close()
			return State{}, err
		}
		f.Target.Scope, f.At = Scope(scope), f.At.UTC()
		st.Frozen = append(st.Frozen, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return State{}, err
	}

	prows, err := tx.Query(ctx, `
		SELECT r.scope, r.target, r.seq, r.actor_id, r.reason, r.created_at
		  FROM kill_switch_events r
		 WHERE r.tenant_id = $1 AND r.kind = 'unfreeze_request'
		   AND NOT EXISTS (SELECT 1 FROM kill_switch_events d WHERE d.tenant_id = r.tenant_id AND d.request_seq = r.seq)
		 ORDER BY r.seq`, tenant)
	if err != nil {
		return State{}, fmt.Errorf("killswitch: read requests: %w", err)
	}
	defer prows.Close()
	for prows.Next() {
		var p Pending
		var scope string
		if err := prows.Scan(&scope, &p.Target.Value, &p.RequestSeq, &p.RequestedBy, &p.Reason, &p.At); err != nil {
			return State{}, err
		}
		p.Target.Scope, p.At = Scope(scope), p.At.UTC()
		st.Pending = append(st.Pending, p)
	}
	return st, prows.Err()
}

// Check returns nil only if no freeze covers the action. Any error, including a
// failure to read the switch, means the caller must not proceed.
func (s *Service) Check(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, a Action) error {
	st, err := s.State(ctx, tx, tenant)
	if err != nil {
		return fmt.Errorf("killswitch: cannot read the switch, treating automation as frozen: %w", err)
	}
	return st.Covers(a)
}

// Covers returns a *FrozenError if a frozen scope in st covers the action, and
// nil otherwise. It lets a job read the switch once and judge many actions.
func (st State) Covers(a Action) error {
	for _, f := range st.Frozen {
		switch f.Target.Scope {
		case ScopeGlobal:
			return &FrozenError{Scope: ScopeGlobal}
		case ScopeRail:
			if a.Rail != "" && a.Rail == f.Target.Value {
				return &FrozenError{Scope: ScopeRail, Target: a.Rail}
			}
		case ScopeExceptionType:
			if a.ExceptionType != "" && a.ExceptionType == f.Target.Value {
				return &FrozenError{Scope: ScopeExceptionType, Target: a.ExceptionType}
			}
		case ScopePolicy:
			if a.PolicyID != "" && a.PolicyID == f.Target.Value {
				return &FrozenError{Scope: ScopePolicy, Target: a.PolicyID}
			}
			if a.PolicyUnknown {
				return &FrozenError{Scope: ScopePolicy, Target: f.Target.Value}
			}
		}
	}
	return nil
}

func (s *Service) frozenAt(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, t Target) (*Frozen, *Pending, error) {
	st, err := s.State(ctx, tx, tenant)
	if err != nil {
		return nil, nil, err
	}
	var f *Frozen
	var p *Pending
	for i := range st.Frozen {
		if st.Frozen[i].Target == t {
			f = &st.Frozen[i]
		}
	}
	for i := range st.Pending {
		if st.Pending[i].Target == t {
			p = &st.Pending[i]
		}
	}
	return f, p, nil
}

func (s *Service) insert(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, t Target, kind string, actor uuid.UUID, reason string, requestSeq *int64) (int64, error) {
	var seq int64
	err := tx.QueryRow(ctx, `
		INSERT INTO kill_switch_events (tenant_id, scope, target, kind, actor_id, reason, request_seq)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING seq`,
		tenant, string(t.Scope), t.Value, kind, actor, strings.TrimSpace(reason), requestSeq).Scan(&seq)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) {
			switch pg.Code {
			case "23001":
				return 0, ErrSameActor
			case "23505":
				return 0, ErrAlreadyDecided
			}
		}
		return 0, fmt.Errorf("killswitch: insert: %w", err)
	}
	return seq, nil
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, tenant, actor uuid.UUID, action string, t Target, reason string, seq int64) error {
	_, err := s.Audit.Append(ctx, tx, tenant, audit.Event{
		Actor: audit.ActorUser, ActorID: actor, Action: action, SubjectType: "killswitch",
		SubjectID: strings.Trim(string(t.Scope)+":"+t.Value, ":"), ReasonCode: strings.TrimSpace(reason),
		Details: map[string]any{"scope": string(t.Scope), "target": t.Value, "seq": seq},
	})
	return err
}

func checkActorReason(actor uuid.UUID, reason string) error {
	if actor == uuid.Nil {
		return fmt.Errorf("%w: actor is required", ErrInvalid)
	}
	if len(strings.TrimSpace(reason)) < 5 {
		return fmt.Errorf("%w: a reason of at least 5 characters is required", ErrInvalid)
	}
	return nil
}

// Freeze stops automation at the given scope. It is immediate and needs one
// person. Freezing a scope that is already frozen changes nothing and reports
// changed as false.
func (s *Service) Freeze(ctx context.Context, tx pgx.Tx, tenant, actor uuid.UUID, t Target, reason string) (changed bool, err error) {
	if err := t.validate(); err != nil {
		return false, err
	}
	if err := checkActorReason(actor, reason); err != nil {
		return false, err
	}
	f, _, err := s.frozenAt(ctx, tx, tenant, t)
	if err != nil {
		return false, err
	}
	if f != nil {
		return false, nil
	}
	seq, err := s.insert(ctx, tx, tenant, t, "freeze", actor, reason, nil)
	if err != nil {
		return false, err
	}
	return true, s.record(ctx, tx, tenant, actor, "killswitch.frozen", t, reason, seq)
}

// RequestUnfreeze asks for a freeze to be lifted. Someone else must approve.
func (s *Service) RequestUnfreeze(ctx context.Context, tx pgx.Tx, tenant, actor uuid.UUID, t Target, reason string) (requestSeq int64, err error) {
	if err := t.validate(); err != nil {
		return 0, err
	}
	if err := checkActorReason(actor, reason); err != nil {
		return 0, err
	}
	f, p, err := s.frozenAt(ctx, tx, tenant, t)
	if err != nil {
		return 0, err
	}
	if f == nil {
		return 0, ErrNotFrozen
	}
	if p != nil {
		return 0, ErrPending
	}
	seq, err := s.insert(ctx, tx, tenant, t, "unfreeze_request", actor, reason, nil)
	if err != nil {
		return 0, err
	}
	return seq, s.record(ctx, tx, tenant, actor, "killswitch.unfreeze_requested", t, reason, seq)
}

// DecideUnfreeze approves or rejects an open request. The decider must not be the
// requester; the database enforces this as well.
func (s *Service) DecideUnfreeze(ctx context.Context, tx pgx.Tx, tenant, actor uuid.UUID, requestSeq int64, approve bool, reason string) error {
	if err := checkActorReason(actor, reason); err != nil {
		return err
	}
	st, err := s.State(ctx, tx, tenant)
	if err != nil {
		return err
	}
	var req *Pending
	for i := range st.Pending {
		if st.Pending[i].RequestSeq == requestSeq {
			req = &st.Pending[i]
		}
	}
	if req == nil {
		return ErrNoSuchRequest
	}
	if req.RequestedBy == actor {
		return ErrSameActor
	}
	kind, action := "unfreeze_reject", "killswitch.unfreeze_rejected"
	if approve {
		kind, action = "unfreeze_approve", "killswitch.unfrozen"
	}
	seq, err := s.insert(ctx, tx, tenant, req.Target, kind, actor, reason, &requestSeq)
	if err != nil {
		return err
	}
	return s.record(ctx, tx, tenant, actor, action, req.Target, reason, seq)
}
