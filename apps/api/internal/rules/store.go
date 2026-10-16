package rules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
)

// Statuses of a rule version.
const (
	Draft             = "draft"
	Shadow            = "shadow"
	PendingActivation = "pending_activation"
	Active            = "active"
	Retired           = "retired"
	Rejected          = "rejected"
)

// Errors from the lifecycle.
var (
	ErrNotFound   = errors.New("rules: no such rule version")
	ErrState      = errors.New("rules: the rule version is not in a state that allows this")
	ErrFourEyes   = errors.New("rules: the person who made or requested a change may not approve it")
	ErrNoEvidence = errors.New("rules: the rule has not been evaluated against any transaction yet")
	ErrVersion    = errors.New("rules: the next version of a rule must be exactly one more than the latest")
)

// Store keeps rules and moves them through their lifecycle. Every change is audited
// in the caller's transaction.
type Store struct {
	Audit *audit.Appender
	Now   func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Version is a stored rule version and where it is in its lifecycle.
type Version struct {
	Rule                  Rule       `json:"rule"`
	Status                string     `json:"status"`
	CreatedBy             uuid.UUID  `json:"created_by"`
	CreatedAt             time.Time  `json:"created_at"`
	ShadowStartedAt       *time.Time `json:"shadow_started_at,omitempty"`
	ActivationRequestedBy *uuid.UUID `json:"activation_requested_by,omitempty"`
	ActivationReason      string     `json:"activation_reason,omitempty"`
	DecidedBy             *uuid.UUID `json:"decided_by,omitempty"`
	DecisionReason        string     `json:"decision_reason,omitempty"`
	ActivatedAt           *time.Time `json:"activated_at,omitempty"`
	RetiredAt             *time.Time `json:"retired_at,omitempty"`
	RetiredReason         string     `json:"retired_reason,omitempty"`
}

const versionColumns = `definition, status, created_by, created_at, shadow_started_at, activation_requested_by, activation_reason,
	decided_by, decision_reason, activated_at, retired_at, retired_reason`

func scanVersion(row pgx.Row) (Version, error) {
	var v Version
	var def []byte
	err := row.Scan(&def, &v.Status, &v.CreatedBy, &v.CreatedAt, &v.ShadowStartedAt, &v.ActivationRequestedBy, &v.ActivationReason,
		&v.DecidedBy, &v.DecisionReason, &v.ActivatedAt, &v.RetiredAt, &v.RetiredReason)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(def, &v.Rule)
}

func (s *Store) audit(ctx context.Context, tx pgx.Tx, tenant, by uuid.UUID, action string, r Rule, details map[string]any) error {
	if s.Audit == nil {
		return errors.New("rules: an audit appender is required")
	}
	if details == nil {
		details = map[string]any{}
	}
	details["version"] = r.Version
	_, err := s.Audit.Append(ctx, tx, tenant, audit.Event{Actor: audit.ActorUser, ActorID: by, Action: action, SubjectType: "detection_rule",
		SubjectID: r.RuleID, PolicyVersion: fmt.Sprintf("%s@%d", r.RuleID, r.Version), Details: details})
	return err
}

// Create saves a validated rule as a new draft. Versions count up by one, so history
// has no gaps and no version is ever replaced.
func (s *Store) Create(ctx context.Context, tx pgx.Tx, tenant, by uuid.UUID, r Rule) (Version, error) {
	if err := r.Validate(); err != nil {
		return Version{}, err
	}
	// Serialise creation of versions of one rule.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenant.String()+"|rule|"+r.RuleID); err != nil {
		return Version{}, err
	}
	var latest int
	if err := tx.QueryRow(ctx, `SELECT coalesce(max(version), 0) FROM detection_rules WHERE tenant_id = $1 AND rule_id = $2`, tenant, r.RuleID).Scan(&latest); err != nil {
		return Version{}, err
	}
	if r.Version != latest+1 {
		return Version{}, fmt.Errorf("%w: latest is %d, got %d", ErrVersion, latest, r.Version)
	}
	def, _ := json.Marshal(r)
	sum := sha256.Sum256(def)
	if _, err := tx.Exec(ctx, `INSERT INTO detection_rules (tenant_id, rule_id, version, definition, definition_hash, created_by) VALUES ($1,$2,$3,$4,$5,$6)`,
		tenant, r.RuleID, r.Version, def, hex.EncodeToString(sum[:]), by); err != nil {
		return Version{}, err
	}
	if err := s.audit(ctx, tx, tenant, by, "rules.created", r, map[string]any{"code": r.Code}); err != nil {
		return Version{}, err
	}
	return s.Get(ctx, tx, tenant, r.RuleID, r.Version)
}

// Get loads one version.
func (s *Store) Get(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ruleID string, version int) (Version, error) {
	v, err := scanVersion(tx.QueryRow(ctx, `SELECT `+versionColumns+` FROM detection_rules WHERE tenant_id = $1 AND rule_id = $2 AND version = $3`, tenant, ruleID, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	return v, err
}

// lock loads a version and holds its row until the transaction ends.
func (s *Store) lock(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ruleID string, version int) (Version, error) {
	if _, err := tx.Exec(ctx, `SELECT 1 FROM detection_rules WHERE tenant_id = $1 AND rule_id = $2 AND version = $3 FOR UPDATE`, tenant, ruleID, version); err != nil {
		return Version{}, err
	}
	return s.Get(ctx, tx, tenant, ruleID, version)
}

// StartShadow moves a draft to shadow: it is evaluated against transactions as they
// arrive and the results are recorded, but it opens no cases.
func (s *Store) StartShadow(ctx context.Context, tx pgx.Tx, tenant, by uuid.UUID, ruleID string, version int) (Version, error) {
	v, err := s.lock(ctx, tx, tenant, ruleID, version)
	if err != nil {
		return Version{}, err
	}
	if v.Status != Draft {
		return Version{}, fmt.Errorf("%w: it is %s", ErrState, v.Status)
	}
	if _, err := tx.Exec(ctx, `UPDATE detection_rules SET status = 'shadow', shadow_started_at = $4 WHERE tenant_id = $1 AND rule_id = $2 AND version = $3`,
		tenant, ruleID, version, s.now()); err != nil {
		return Version{}, err
	}
	if err := s.audit(ctx, tx, tenant, by, "rules.shadow_started", v.Rule, nil); err != nil {
		return Version{}, err
	}
	return s.Get(ctx, tx, tenant, ruleID, version)
}

// RequestActivation asks for a shadow version to become live. It needs evidence that
// the rule has been evaluated against at least one transaction, so an approver has
// something to look at. The approver sees the shadow results through Stats.
func (s *Store) RequestActivation(ctx context.Context, tx pgx.Tx, tenant, by uuid.UUID, ruleID string, version int, reason string) (Version, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < 5 || len(reason) > 500 {
		return Version{}, fmt.Errorf("%w: a reason of 5 to 500 characters is required", ErrInvalid)
	}
	v, err := s.lock(ctx, tx, tenant, ruleID, version)
	if err != nil {
		return Version{}, err
	}
	if v.Status != Shadow {
		return Version{}, fmt.Errorf("%w: it is %s; a rule must run in shadow first", ErrState, v.Status)
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM rule_evaluations WHERE tenant_id = $1 AND rule_id = $2 AND version = $3`, tenant, ruleID, version).Scan(&n); err != nil {
		return Version{}, err
	}
	if n == 0 {
		return Version{}, ErrNoEvidence
	}
	if _, err := tx.Exec(ctx, `UPDATE detection_rules SET status = 'pending_activation', activation_requested_by = $4, activation_requested_at = $5, activation_reason = $6
		WHERE tenant_id = $1 AND rule_id = $2 AND version = $3`, tenant, ruleID, version, by, s.now(), reason); err != nil {
		return Version{}, err
	}
	if err := s.audit(ctx, tx, tenant, by, "rules.activation_requested", v.Rule, map[string]any{"reason": reason, "evaluations": n}); err != nil {
		return Version{}, err
	}
	return s.Get(ctx, tx, tenant, ruleID, version)
}

// Decide approves or rejects a requested activation. The decider may not be the
// person who created the version or asked for activation. Approving makes it live and
// retires the version that was live, in the same transaction, so there is never a gap
// and never two.
func (s *Store) Decide(ctx context.Context, tx pgx.Tx, tenant, by uuid.UUID, ruleID string, version int, approve bool, reason string) (Version, error) {
	reason = strings.TrimSpace(reason)
	if !approve && reason == "" {
		return Version{}, fmt.Errorf("%w: a reason is required to reject", ErrInvalid)
	}
	v, err := s.lock(ctx, tx, tenant, ruleID, version)
	if err != nil {
		return Version{}, err
	}
	if v.Status != PendingActivation {
		return Version{}, fmt.Errorf("%w: it is %s", ErrState, v.Status)
	}
	if by == v.CreatedBy || (v.ActivationRequestedBy != nil && by == *v.ActivationRequestedBy) {
		return Version{}, ErrFourEyes
	}
	now := s.now()
	if !approve {
		if _, err := tx.Exec(ctx, `UPDATE detection_rules SET status = 'rejected', decided_by = $4, decided_at = $5, decision_reason = $6 WHERE tenant_id = $1 AND rule_id = $2 AND version = $3`,
			tenant, ruleID, version, by, now, reason); err != nil {
			return Version{}, err
		}
		if err := s.audit(ctx, tx, tenant, by, "rules.activation_rejected", v.Rule, map[string]any{"reason": reason}); err != nil {
			return Version{}, err
		}
		return s.Get(ctx, tx, tenant, ruleID, version)
	}
	var prev *int
	if err := tx.QueryRow(ctx, `UPDATE detection_rules SET status = 'retired', retired_at = $3, retired_by = $4, retired_reason = $5
		WHERE tenant_id = $1 AND rule_id = $2 AND status = 'active' RETURNING version`, tenant, ruleID, now, by, fmt.Sprintf("superseded by version %d", version)).Scan(&prev); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Version{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE detection_rules SET status = 'active', decided_by = $4, decided_at = $5, decision_reason = $6, activated_at = $5,
			retired_at = NULL, retired_by = NULL, retired_reason = ''
		WHERE tenant_id = $1 AND rule_id = $2 AND version = $3`, tenant, ruleID, version, by, now, reason); err != nil {
		return Version{}, err
	}
	details := map[string]any{"reason": reason}
	if prev != nil {
		details["superseded_version"] = *prev
	}
	if err := s.audit(ctx, tx, tenant, by, "rules.activated", v.Rule, details); err != nil {
		return Version{}, err
	}
	return s.Get(ctx, tx, tenant, ruleID, version)
}

// Rollback asks for an earlier, once-live version to become live again. It is an
// activation request like any other: the current version stays live until a different
// person approves, and the old definition is exactly what it was.
func (s *Store) Rollback(ctx context.Context, tx pgx.Tx, tenant, by uuid.UUID, ruleID string, toVersion int, reason string) (Version, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < 5 || len(reason) > 500 {
		return Version{}, fmt.Errorf("%w: a reason of 5 to 500 characters is required", ErrInvalid)
	}
	v, err := s.lock(ctx, tx, tenant, ruleID, toVersion)
	if err != nil {
		return Version{}, err
	}
	if v.Status != Retired || v.ActivatedAt == nil {
		return Version{}, fmt.Errorf("%w: only a version that was once live can be rolled back to; it is %s", ErrState, v.Status)
	}
	if _, err := tx.Exec(ctx, `UPDATE detection_rules SET status = 'pending_activation', activation_requested_by = $4, activation_requested_at = $5, activation_reason = $6,
			decided_by = NULL, decided_at = NULL, decision_reason = ''
		WHERE tenant_id = $1 AND rule_id = $2 AND version = $3`, tenant, ruleID, toVersion, by, s.now(), "Rollback: "+reason); err != nil {
		return Version{}, err
	}
	if err := s.audit(ctx, tx, tenant, by, "rules.rollback_requested", v.Rule, map[string]any{"reason": reason}); err != nil {
		return Version{}, err
	}
	return s.Get(ctx, tx, tenant, ruleID, toVersion)
}

// Disable retires the live version at once, so a rule that is doing harm stops
// immediately. It does not need a second person, because it can only reduce what the
// rule does; the reason is recorded and audited. Turning a rule back on goes through
// Rollback and approval.
func (s *Store) Disable(ctx context.Context, tx pgx.Tx, tenant, by uuid.UUID, ruleID, reason string) (Version, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) < 5 || len(reason) > 500 {
		return Version{}, fmt.Errorf("%w: a reason of 5 to 500 characters is required", ErrInvalid)
	}
	var version int
	err := tx.QueryRow(ctx, `UPDATE detection_rules SET status = 'retired', retired_at = $3, retired_by = $4, retired_reason = $5
		WHERE tenant_id = $1 AND rule_id = $2 AND status = 'active' RETURNING version`, tenant, ruleID, s.now(), by, "Disabled: "+reason).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, fmt.Errorf("%w: no live version", ErrState)
	}
	if err != nil {
		return Version{}, err
	}
	v, err := s.Get(ctx, tx, tenant, ruleID, version)
	if err != nil {
		return Version{}, err
	}
	if err := s.audit(ctx, tx, tenant, by, "rules.disabled", v.Rule, map[string]any{"reason": reason}); err != nil {
		return Version{}, err
	}
	return v, nil
}

// List returns the latest version of every rule, or every version of one rule.
func (s *Store) List(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ruleID string) ([]Version, error) {
	q := `SELECT ` + versionColumns + ` FROM detection_rules WHERE tenant_id = $1 AND ($2 = '' OR rule_id = $2) ORDER BY rule_id, version DESC`
	rows, err := tx.Query(ctx, q, tenant, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	seen := map[string]bool{}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		if ruleID == "" && seen[v.Rule.RuleID] {
			continue
		}
		seen[v.Rule.RuleID] = true
		out = append(out, v)
	}
	return out, rows.Err()
}
