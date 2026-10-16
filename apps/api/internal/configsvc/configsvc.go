// Package configsvc holds versioned, maker-checker configuration (FR-39).
//
// Rules, thresholds, windows, approval limits and waiting times live here, not in
// code. Every change is a new immutable version with its source and verification
// status, takes effect only after a different person approves it, and is written
// to the audit chain. Nothing is guessed: a key with no approved version is
// "not configured", and callers must treat that as a reason to stop.
package configsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
)

// Errors callers can test for.
var (
	ErrUnknownKey     = errors.New("configsvc: unknown configuration key")
	ErrInvalidValue   = errors.New("configsvc: value is not valid for this key")
	ErrInvalidInput   = errors.New("configsvc: invalid input")
	ErrNotConfigured  = errors.New("configsvc: no approved value for this key")
	ErrNotFound       = errors.New("configsvc: no such version")
	ErrAlreadyDecided = errors.New("configsvc: this version already has a decision")
	ErrSelfApproval   = errors.New("configsvc: the proposer cannot decide their own change")
	ErrConflict       = errors.New("configsvc: another change to this key was made at the same time")
)

// KeyDef describes a configuration key and the values it accepts.
type KeyDef struct {
	Pattern     *regexp.Regexp
	Min, Max    int64
	Unit        string
	Description string
	// Clock marks a key whose value is a regulatory clock definition (an object)
	// instead of an integer.
	Clock bool
}

// Keys is the registry of configuration keys. A value is stored only for a key
// that matches one of these, and must be an integer in range. No defaults are
// built in: the registry says what can be configured, never what the value is.
var Keys = []KeyDef{
	{regexp.MustCompile(`^duplicate\.window_seconds$`), 1, 3600, "seconds", "Window for flagging a potential duplicate debit", false},
	{regexp.MustCompile(`^status_query\.freshness_seconds$`), 1, 86400, "seconds", "How old a status answer may be and still count as fresh evidence", false},
	{regexp.MustCompile(`^waiting_threshold_seconds\.[A-Z0-9_]{2,30}$`), 0, 7 * 86400, "seconds", "How long to wait on a rail before a reversal may be considered", false},
	{regexp.MustCompile(`^approval\.auto_limit_minor$`), 0, 1 << 52, "minor units", "Largest amount that may be reversed without a human approval", false},
	{regexp.MustCompile(`^approval\.validity_seconds$`), 60, 30 * 86400, "seconds", "How long a reversal approval stays valid before it must be renewed", false},
	{regexp.MustCompile(`^approval\.limit_minor\.[A-Z_]{3,40}$`), 0, 1 << 52, "minor units", "Largest amount a role may approve", false},
	{regexp.MustCompile(`^sla\.pause_max_hours$`), 0, 24 * 30, "hours", "Longest an SLA clock may stay paused without supervisor re-approval", false},
	{regexp.MustCompile(`^calendar\.holiday\.[0-9]{4}-[0-9]{2}-[0-9]{2}$`), 0, 1, "flag", "1 marks a public holiday on that date, 0 marks a working day that would otherwise be off", false},
	{regexp.MustCompile(`^calendar\.cutoff_minute\.[A-Z0-9_]{2,30}$`), 0, 1439, "minutes after midnight", "Daily cut-off for a rail, in the institution's local time", false},
	{regexp.MustCompile(`^calendar\.settlement_lag_days\.[A-Z0-9_]{2,30}$`), 0, 10, "business days", "Business days between the cut-off date and settlement on a rail", false},
	{regexp.MustCompile(`^reversal\.auto_eligible\.[A-Z]{3,4}[0-9]{3}$`), 0, 1, "flag", "1 lets automation reverse this exception type without a human approval, within the auto limit", false},
	{regexp.MustCompile(`^reversal\.simulated_execution_enabled$`), 0, 1, "flag", "1 allows reversals to run against the SIMULATED provider. There is no live provider in this build", false},
	{regexp.MustCompile(`^customer\.ack_target_hours$`), 1, 24 * 30, "hours", "Hours within which the bank acknowledges a customer claim that matches no transaction yet", false},
	{regexp.MustCompile(`^internal_target_hours\.[A-Z0-9_]{3,40}$`), 0, 24 * 30, "hours", "The bank's own target for a situation, earlier than its regulatory clock, keyed by the clock id", false},
	{regexp.MustCompile(`^deadline\.approaching_percent$`), 1, 99, "percent", "Share of the time to a deadline already used at which a case is shown as approaching", false},
	{regexp.MustCompile(`^deadline\.urgent_percent$`), 1, 99, "percent", "Share of the time to a deadline already used at which a case is shown as urgent", false},
	{regexp.MustCompile(`^customer_match\.window_seconds$`), 1, 7 * 86400, "seconds", "How far apart in time a customer's reported transaction and a stored one may be and still be offered as a match", false},
	{regexp.MustCompile(`^alert\.escalate_after_minutes$`), 1, 1440, "minutes", "How long an alert may wait unacknowledged before it is escalated to the next role", false},
	{regexp.MustCompile(`^alert\.high_value_minor$`), 1, 1 << 52, "minor units", "An unresolved case at or above this amount raises a high-value alert", false},
	{regexp.MustCompile(`^alert\.high_value_age_minutes$`), 1, 7 * 1440, "minutes", "How long a high-value case may stay unresolved before the alert is raised", false},
	{regexp.MustCompile(`^incident\.group_window_minutes$`), 1, 1440, "minutes", "Exceptions on the same rail, channel and error signature inside this window may form an incident", false},
	{regexp.MustCompile(`^incident\.min_group_size$`), 2, 10000, "cases", "How many matching exceptions inside the window it takes to open an incident", false},
	{regexp.MustCompile(`^incident\.spike_sigma_tenths$`), 10, 100, "tenths of a standard deviation", "How far above the same hour of earlier weeks a count must be to count as a spike", false},
	{regexp.MustCompile(`^incident\.spike_min_weeks$`), 1, 52, "weeks", "How many earlier weeks of the same hour are needed before a spike can be called", false},
	{regexp.MustCompile(`^incident\.spike_min_count$`), 1, 10000, "cases", "The fewest exceptions in the hour that can ever be called a spike", false},
	{regexp.MustCompile(`^customer\.reports_per_hour$`), 1, 1000, "reports", "Most reports one customer may create in an hour, as abuse protection", false},
	{Pattern: regexp.MustCompile(`^regulatory_clock\.[A-Z0-9_]{3,40}$`), Description: "A regulatory or contractual clock: situation, duration, unit, start event", Clock: true},
}

// Lookup returns the definition for key.
func Lookup(key string) (KeyDef, bool) {
	for _, d := range Keys {
		if d.Pattern.MatchString(key) {
			return d, true
		}
	}
	return KeyDef{}, false
}

var verifyStatuses = map[string]bool{"confirmed": true, "unverified": true, "reported": true, "draft": true, "conflicting": true}

// ProposeInput is a requested change.
type ProposeInput struct {
	Key             string
	Value           json.RawMessage
	SourceReference string
	EffectiveFrom   time.Time
	VerifyStatus    string
	Reason          string
}

// Version is one stored version of a key, with its decision if there is one.
type Version struct {
	Key     string
	Version int
	// Value is the integer value for integer keys, and zero for clock keys.
	Value int64
	// Raw is the stored value as JSON, for every key.
	Raw             json.RawMessage
	SourceReference string
	EffectiveFrom   time.Time
	VerifyStatus    string
	ProposedBy      uuid.UUID
	Reason          string
	CreatedAt       time.Time
	// Decision is "", "approved" or "rejected".
	Decision       string
	DecidedBy      uuid.UUID
	DecidedAt      *time.Time
	DecisionReason string
}

// Service reads and writes configuration. It holds no state.
type Service struct {
	Audit *audit.Appender
	// Now defaults to time.Now.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Clock is the value of a regulatory clock key. Its source, status and effective
// date are the version's source_reference, verify_status and effective_from.
type Clock struct {
	Situation  string `json:"situation"`
	Duration   int    `json:"duration"`
	Unit       string `json:"unit"`
	StartEvent string `json:"start_event"`
	Calendar   string `json:"calendar"`
	Owner      string `json:"owner"`
}

var clockUnits = map[string]bool{"minutes": true, "hours": true, "working_days": true}

// parseClock validates a clock definition and returns it in canonical JSON.
func parseClock(raw json.RawMessage) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c Clock
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%w: expected an object with situation, duration, unit, start_event, calendar and owner", ErrInvalidValue)
	}
	switch {
	case strings.TrimSpace(c.Situation) == "" || len(c.Situation) > 200:
		return nil, fmt.Errorf("%w: situation is required (up to 200 characters)", ErrInvalidValue)
	case c.Duration < 1 || c.Duration > 365:
		return nil, fmt.Errorf("%w: duration must be between 1 and 365", ErrInvalidValue)
	case !clockUnits[c.Unit]:
		return nil, fmt.Errorf("%w: unit must be minutes, hours or working_days", ErrInvalidValue)
	case strings.TrimSpace(c.StartEvent) == "" || strings.TrimSpace(c.Calendar) == "" || strings.TrimSpace(c.Owner) == "":
		return nil, fmt.Errorf("%w: start_event, calendar and owner are required", ErrInvalidValue)
	}
	return json.Marshal(c)
}

func parseValue(def KeyDef, raw json.RawMessage) (int64, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return 0, ErrInvalidValue
	}
	n, ok := v.(json.Number)
	if !ok {
		return 0, fmt.Errorf("%w: expected an integer", ErrInvalidValue)
	}
	i, err := n.Int64()
	if err != nil {
		return 0, fmt.Errorf("%w: expected an integer", ErrInvalidValue)
	}
	if i < def.Min || i > def.Max {
		return 0, fmt.Errorf("%w: must be between %d and %d %s", ErrInvalidValue, def.Min, def.Max, def.Unit)
	}
	return i, nil
}

// Propose records a new version that is not yet in effect. tx must be a tenant
// transaction. The change and its audit record commit together.
func (s *Service) Propose(ctx context.Context, tx pgx.Tx, tenant, maker uuid.UUID, in ProposeInput) (Version, error) {
	def, ok := Lookup(in.Key)
	if !ok {
		return Version{}, ErrUnknownKey
	}
	var value int64
	var canonical json.RawMessage
	if def.Clock {
		c, err := parseClock(in.Value)
		if err != nil {
			return Version{}, err
		}
		canonical = c
	} else {
		v, err := parseValue(def, in.Value)
		if err != nil {
			return Version{}, err
		}
		value = v
		canonical, _ = json.Marshal(v)
	}
	now := s.now()
	switch {
	case maker == uuid.Nil || tenant == uuid.Nil:
		return Version{}, fmt.Errorf("%w: proposer and tenant are required", ErrInvalidInput)
	case strings.TrimSpace(in.SourceReference) == "":
		return Version{}, fmt.Errorf("%w: a source reference is required", ErrInvalidInput)
	case len(strings.TrimSpace(in.Reason)) < 10:
		return Version{}, fmt.Errorf("%w: a reason of at least 10 characters is required", ErrInvalidInput)
	case !verifyStatuses[in.VerifyStatus]:
		return Version{}, fmt.Errorf("%w: verify status must be confirmed, unverified, reported, draft or conflicting", ErrInvalidInput)
	case in.EffectiveFrom.IsZero():
		return Version{}, fmt.Errorf("%w: an effective date is required", ErrInvalidInput)
	case in.EffectiveFrom.Before(now.Add(-time.Minute)):
		return Version{}, fmt.Errorf("%w: a change cannot take effect in the past", ErrInvalidInput)
	case in.EffectiveFrom.After(now.AddDate(1, 0, 0)):
		return Version{}, fmt.Errorf("%w: a change cannot be scheduled more than a year ahead", ErrInvalidInput)
	}

	var next int
	if err := tx.QueryRow(ctx, `SELECT coalesce(max(version), 0) + 1 FROM config_versions WHERE tenant_id = $1 AND key = $2`,
		tenant, in.Key).Scan(&next); err != nil {
		return Version{}, fmt.Errorf("configsvc: next version: %w", err)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO config_versions (tenant_id, key, version, value, source_reference, effective_from, verify_status, proposed_by, reason, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		tenant, in.Key, next, canonical, strings.TrimSpace(in.SourceReference), in.EffectiveFrom.UTC(), in.VerifyStatus, maker, strings.TrimSpace(in.Reason), now)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return Version{}, ErrConflict
		}
		return Version{}, fmt.Errorf("configsvc: insert: %w", err)
	}

	if _, err := s.Audit.Append(ctx, tx, tenant, audit.Event{
		Actor: audit.ActorUser, ActorID: maker, Action: "config.proposed",
		SubjectType: "config", SubjectID: fmt.Sprintf("%s@%d", in.Key, next),
		Details: map[string]any{"key": in.Key, "version": next, "value": json.RawMessage(canonical), "verify_status": in.VerifyStatus,
			"source_reference": strings.TrimSpace(in.SourceReference), "effective_from": in.EffectiveFrom.UTC().Format(time.RFC3339)},
	}); err != nil {
		return Version{}, err
	}
	return Version{Key: in.Key, Version: next, Value: value, Raw: canonical, SourceReference: strings.TrimSpace(in.SourceReference),
		EffectiveFrom: in.EffectiveFrom.UTC(), VerifyStatus: in.VerifyStatus, ProposedBy: maker,
		Reason: strings.TrimSpace(in.Reason), CreatedAt: now}, nil
}

// Decide approves or rejects a proposed version. The checker must be a different
// person from the proposer. A version is decided once.
func (s *Service) Decide(ctx context.Context, tx pgx.Tx, tenant, checker uuid.UUID, key string, version int, approve bool, reason string) (Version, error) {
	if _, ok := Lookup(key); !ok {
		return Version{}, ErrUnknownKey
	}
	if checker == uuid.Nil {
		return Version{}, fmt.Errorf("%w: decider is required", ErrInvalidInput)
	}
	if !approve && strings.TrimSpace(reason) == "" {
		return Version{}, fmt.Errorf("%w: a reason is required to reject", ErrInvalidInput)
	}
	v, err := s.version(ctx, tx, tenant, key, version)
	if err != nil {
		return Version{}, err
	}
	if v.Decision != "" {
		return Version{}, ErrAlreadyDecided
	}
	if v.ProposedBy == checker {
		return Version{}, ErrSelfApproval
	}

	decision := "rejected"
	if approve {
		decision = "approved"
	}
	now := s.now()
	if _, err := tx.Exec(ctx, `
		INSERT INTO config_decisions (tenant_id, key, version, decision, decided_by, reason, decided_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, tenant, key, version, decision, checker, strings.TrimSpace(reason), now); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) {
			switch pg.Code {
			case "23505":
				return Version{}, ErrAlreadyDecided
			case "23001": // restrict_violation from the maker-checker trigger
				return Version{}, ErrSelfApproval
			}
		}
		return Version{}, fmt.Errorf("configsvc: decide: %w", err)
	}

	if _, err := s.Audit.Append(ctx, tx, tenant, audit.Event{
		Actor: audit.ActorUser, ActorID: checker, Action: "config." + decision,
		SubjectType: "config", SubjectID: fmt.Sprintf("%s@%d", key, version), ReasonCode: strings.TrimSpace(reason),
		Details: map[string]any{"key": key, "version": version, "proposed_by": v.ProposedBy.String()},
	}); err != nil {
		return Version{}, err
	}
	v.Decision, v.DecidedBy, v.DecidedAt, v.DecisionReason = decision, checker, &now, strings.TrimSpace(reason)
	return v, nil
}

const selectVersion = `
	SELECT v.key, v.version, v.value::text, v.source_reference, v.effective_from, v.verify_status,
	       v.proposed_by, v.reason, v.created_at,
	       coalesce(d.decision, ''), d.decided_by, d.decided_at, coalesce(d.reason, '')
	FROM config_versions v
	LEFT JOIN config_decisions d ON d.tenant_id = v.tenant_id AND d.key = v.key AND d.version = v.version`

func scanVersion(row pgx.Row) (Version, error) {
	var v Version
	var decidedBy *uuid.UUID
	var raw string
	if err := row.Scan(&v.Key, &v.Version, &raw, &v.SourceReference, &v.EffectiveFrom, &v.VerifyStatus,
		&v.ProposedBy, &v.Reason, &v.CreatedAt, &v.Decision, &decidedBy, &v.DecidedAt, &v.DecisionReason); err != nil {
		return Version{}, err
	}
	if decidedBy != nil {
		v.DecidedBy = *decidedBy
	}
	v.Raw = json.RawMessage(raw)
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		v.Value = n
	}
	v.EffectiveFrom, v.CreatedAt = v.EffectiveFrom.UTC(), v.CreatedAt.UTC()
	return v, nil
}

func (s *Service) version(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, key string, version int) (Version, error) {
	v, err := scanVersion(tx.QueryRow(ctx, selectVersion+` WHERE v.tenant_id = $1 AND v.key = $2 AND v.version = $3`, tenant, key, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	return v, err
}

// Effective returns the approved version in force for key at time at. When
// nothing is approved yet it returns ErrNotConfigured, and callers must not
// substitute a default.
func (s *Service) Effective(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, key string, at time.Time) (Version, error) {
	if _, ok := Lookup(key); !ok {
		return Version{}, ErrUnknownKey
	}
	v, err := scanVersion(tx.QueryRow(ctx, selectVersion+`
		WHERE v.tenant_id = $1 AND v.key = $2 AND d.decision = 'approved' AND v.effective_from <= $3
		ORDER BY v.effective_from DESC, v.version DESC LIMIT 1`, tenant, key, at.UTC()))
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotConfigured
	}
	return v, err
}

// History returns every version of key, newest first, with decisions.
func (s *Service) History(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, key string) ([]Version, error) {
	if _, ok := Lookup(key); !ok {
		return nil, ErrUnknownKey
	}
	rows, err := tx.Query(ctx, selectVersion+` WHERE v.tenant_id = $1 AND v.key = $2 ORDER BY v.version DESC`, tenant, key)
	if err != nil {
		return nil, fmt.Errorf("configsvc: history: %w", err)
	}
	defer rows.Close()
	var out []Version
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Rollback proposes the value of an earlier approved version again. It is an
// ordinary proposal: it needs the same approval and takes effect from
// effectiveFrom. Only versions that were approved can be rolled back to.
func (s *Service) Rollback(ctx context.Context, tx pgx.Tx, tenant, maker uuid.UUID, key string, toVersion int, effectiveFrom time.Time, reason string) (Version, error) {
	old, err := s.version(ctx, tx, tenant, key, toVersion)
	if err != nil {
		return Version{}, err
	}
	if old.Decision != "approved" {
		return Version{}, fmt.Errorf("%w: version %d was never approved", ErrInvalidInput, toVersion)
	}
	raw := old.Raw
	return s.Propose(ctx, tx, tenant, maker, ProposeInput{
		Key: key, Value: raw, SourceReference: fmt.Sprintf("rollback to version %d (%s)", toVersion, old.SourceReference),
		EffectiveFrom: effectiveFrom, VerifyStatus: old.VerifyStatus, Reason: reason,
	})
}

// EffectiveByPrefix returns the approved version in force at time at for every
// key that starts with prefix. It is how a whole family of keys, such as the
// calendar's, is read in one query.
func (s *Service) EffectiveByPrefix(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, prefix string, at time.Time) (map[string]Version, error) {
	if prefix == "" || len(prefix) > 60 {
		return nil, fmt.Errorf("%w: a prefix is required", ErrInvalidInput)
	}
	rows, err := tx.Query(ctx, selectVersion+`
		WHERE v.tenant_id = $1 AND starts_with(v.key, $2) AND d.decision = 'approved' AND v.effective_from <= $3
		ORDER BY v.key, v.effective_from DESC, v.version DESC`, tenant, prefix, at.UTC())
	if err != nil {
		return nil, fmt.Errorf("configsvc: read %s*: %w", prefix, err)
	}
	defer rows.Close()
	out := map[string]Version{}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		if _, seen := out[v.Key]; !seen { // rows are newest first within a key
			out[v.Key] = v
		}
	}
	return out, rows.Err()
}
