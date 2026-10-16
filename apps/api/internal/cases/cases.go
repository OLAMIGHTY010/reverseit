// Package cases is the shared exception queue (B4). A case is opened by the
// detector, by a person, or from a customer complaint, and from then on it is
// handled the same way: one state machine, one history, one evidence list, one
// audit trail.
package cases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/bizcal"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/events"
	"github.com/smartreversal/smartreversal/apps/api/internal/routing"
)

// Classification codes. The first six come from the scope document. Others
// proposed in the v2 pack are not used until the bank confirms them.
const (
	CodeDuplicate   = "DUP001"
	CodeNoCredit    = "FAIL001"
	CodeUnknown     = "UNK001"
	CodeReversalErr = "REV001"
	CodeSettlement  = "STL001"
	CodeCustomer    = "CUS001"
	CodeWrongAcct   = "CUS002" // customer-reported wrong account or amount; separate written-reversal path
	CodeAtmFault    = "ATM001"
	CodeDisputedFee = "FEE001"
	CodeVendorDrop  = "VND001"
	CodeMismatch    = "MIS001"
	CodeIncomplete  = "INC001"
)

var knownCodes = map[string]bool{
	CodeDuplicate:   true,
	CodeNoCredit:    true,
	CodeUnknown:     true,
	CodeReversalErr: true,
	CodeSettlement:  true,
	CodeCustomer:    true,
	CodeWrongAcct:   true,
	CodeAtmFault:    true,
	CodeDisputedFee: true,
	CodeVendorDrop:  true,
	CodeMismatch:    true,
	CodeIncomplete:  true,
}

// Sources of a case.
const (
	SourceBot      = "bot"
	SourceCustomer = "customer"
	SourceManual   = "manual"
)

// Statuses (v2 section 6.2).
const (
	StDetected             = "DETECTED"
	StAwaitingEvidence     = "AWAITING_EVIDENCE"
	StUnderInvestigation   = "UNDER_INVESTIGATION"
	StAwaitingApproval     = "AWAITING_APPROVAL"
	StRemediationSubmitted = "REMEDIATION_SUBMITTED"
	StOutcomeVerification  = "OUTCOME_VERIFICATION"
	StResolved             = "RESOLVED"
	StClosed               = "CLOSED"
	StRejectedAction       = "REJECTED_ACTION"
	StRemediationFailed    = "REMEDIATION_FAILED"
	StExternalDependency   = "EXTERNAL_DEPENDENCY"
	StSuppressed           = "SUPPRESSED"
	StClosedNoException    = "CLOSED_NO_EXCEPTION"
	StClosedCustomerError  = "CLOSED_CUSTOMER_ERROR"
	StReopened             = "REOPENED"
)

// transitions is the state machine. Every move is validated against it.
var transitions = map[string][]string{
	StDetected:             {StAwaitingEvidence, StUnderInvestigation, StAwaitingApproval, StSuppressed, StClosedNoException},
	StAwaitingEvidence:     {StUnderInvestigation, StClosedNoException},
	StUnderInvestigation:   {StAwaitingEvidence, StAwaitingApproval, StRemediationSubmitted, StExternalDependency, StClosedNoException, StClosedCustomerError},
	StAwaitingApproval:     {StRemediationSubmitted, StRejectedAction, StUnderInvestigation},
	StRejectedAction:       {StUnderInvestigation},
	StRemediationSubmitted: {StOutcomeVerification, StRemediationFailed},
	StOutcomeVerification:  {StResolved, StRemediationFailed},
	StRemediationFailed:    {StUnderInvestigation},
	StExternalDependency:   {StUnderInvestigation},
	StSuppressed:           {StDetected, StUnderInvestigation},
	StResolved:             {StClosed, StReopened},
	StClosed:               {StReopened},
	StClosedNoException:    {StReopened},
	StClosedCustomerError:  {StReopened},
	StReopened:             {StUnderInvestigation},
}

// CanTransition reports whether from -> to is a legal move.
func CanTransition(from, to string) bool {
	for _, t := range transitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// IsTerminal reports whether no remediation is expected from a status.
func IsTerminal(status string) bool {
	switch status {
	case StResolved, StClosed, StClosedNoException, StClosedCustomerError:
		return true
	}
	return false
}

// Errors.
var (
	ErrInvalid      = errors.New("cases: invalid input")
	ErrNotFound     = errors.New("cases: no such case")
	ErrBadMove      = errors.New("cases: that status change is not allowed")
	ErrStaleVersion = errors.New("cases: the case changed since you read it")
)

// Actor says who made a change.
type Actor struct {
	Type audit.ActorType
	ID   uuid.UUID
}

// System is the actor for automatic changes.
var System = Actor{Type: audit.ActorSystem}

// Case is a stored case.
type Case struct {
	ID              uuid.UUID
	Ref             string
	Classification  string
	Status          string
	Priority        string
	RiskGrade       string
	Source          string
	TxnID           *uuid.UUID
	ExposureMinor   *int64
	Currency        string
	RootCause       string
	OwnerID         *uuid.UUID
	Queue           string
	RuleID          string
	RuleVersion     string
	RegulatoryClock string
	OpenedAt        time.Time
	SLADueAt        *time.Time
	// InternalTargetAt is the bank's own target, kept apart from the regulatory
	// deadline in SLADueAt. Nil when none was set.
	InternalTargetAt *time.Time
	ResolvedAt       *time.Time
	ClosedReason     string
	Complaint        json.RawMessage
	Version          int
}

// Evidence is something that supports a case.
type Evidence struct {
	ID           uuid.UUID `json:"id"`
	Kind         string    `json:"kind"`
	Ref          string    `json:"ref"`
	SourceSystem string    `json:"source_system,omitempty"`
	FetchedAt    time.Time `json:"fetched_at"`
	Hash         string    `json:"hash,omitempty"`
	IsFact       bool      `json:"is_fact"`
	Detail       any       `json:"detail,omitempty"`
}

// Event is one entry in a case's history.
type Event struct {
	Seq        int             `json:"seq"`
	At         time.Time       `json:"at"`
	ActorType  string          `json:"actor_type"`
	ActorID    string          `json:"actor_id,omitempty"`
	Kind       string          `json:"kind"`
	FromStatus string          `json:"from_status,omitempty"`
	ToStatus   string          `json:"to_status,omitempty"`
	ReasonCode string          `json:"reason_code,omitempty"`
	Detail     json.RawMessage `json:"detail,omitempty"`
}

// OpenInput describes a new case.
type OpenInput struct {
	Classification string
	Source         string
	RiskGrade      string
	TxnID          *uuid.UUID
	ExposureMinor  *int64
	Currency       string
	Rail           string // used to pick the regulatory clock
	// Clock names the regulatory clock explicitly (the deadline engine's choice for a
	// customer dispute). When empty the clock is picked from the code and the rail.
	Clock string
	// InternalTargetAt is the bank's own target, when one is known.
	InternalTargetAt *time.Time
	RootCause        string
	RuleID           string
	RuleVersion      string
	Explanation      string
	Evidence         []Evidence
	Complaint        json.RawMessage
	Actor            Actor
	At               time.Time // when the trigger happened; defaults to now
}

// Service creates and changes cases.
type Service struct {
	Audit   *audit.Appender
	Config  *configsvc.Service
	Routing *routing.Service
	Now     func() time.Time
	// OnClose, when set, is called inside the transition's transaction after a case
	// reaches a terminal status. It must not change the case. An error stops the
	// transition.
	OnClose func(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, c Case, by uuid.UUID) error
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// clockFor picks the regulatory clock that governs a case, from the code and the
// rail. The mapping is a modelling choice recorded here; the durations come from
// approved configuration only.
func clockFor(code, rail string) string {
	switch {
	case code == CodeDuplicate || code == CodeWrongAcct:
		return "WRONG_ACCOUNT_REQUEST"
	case (code == CodeNoCredit || code == CodeUnknown || code == CodeCustomer || code == CodeIncomplete) && (rail == "NIP" || rail == "INTERCHANGE"):
		return "FAILED_INSTANT_TRANSFER"
	case (code == CodeNoCredit || code == CodeAtmFault) && rail == "ATM":
		return "ATM_NOT_ON_US"
	case (code == CodeCustomer || code == CodeDisputedFee || code == CodeVendorDrop || code == CodeMismatch) && (rail == "POS" || rail == "WEB" || rail == "CARD" || rail == "MERCHANT"):
		return "POS_WEB_DISPUTE"
	}
	return ""
}

func priorityFor(code, risk string) string {
	switch {
	case risk == "HIGH":
		return "HIGH"
	case code == CodeReversalErr || code == CodeSettlement || code == CodeAtmFault || code == CodeMismatch:
		return "HIGH"
	case code == CodeNoCredit || code == CodeDuplicate || code == CodeIncomplete || code == CodeVendorDrop:
		return "MEDIUM"
	case code == CodeDisputedFee:
		return "LOW"
	}
	return "MEDIUM"
}

// deadline computes the SLA due time from an approved regulatory clock. It
// returns the clock name and a nil time when no approved clock applies, in which
// case the case simply has no regulatory deadline yet.
func (s *Service) deadline(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, clock string, from time.Time) (string, *time.Time, error) {
	if clock == "" || s.Config == nil {
		return "", nil, nil
	}
	v, err := s.Config.Effective(ctx, tx, tenant, configsvc.ClockKey(clock), from)
	if errors.Is(err, configsvc.ErrNotConfigured) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	var c configsvc.Clock
	if err := json.Unmarshal(v.Raw, &c); err != nil {
		return "", nil, err
	}
	var due time.Time
	switch c.Unit {
	case "minutes":
		due = from.Add(time.Duration(c.Duration) * time.Minute)
	case "hours":
		due = from.Add(time.Duration(c.Duration) * time.Hour)
	case "working_days":
		cal, err := bizcal.Load(ctx, tx, s.Config, tenant, from)
		if err != nil {
			return "", nil, err
		}
		if due, err = cal.AddWorkingDays(from, c.Duration); err != nil {
			return "", nil, err
		}
	}
	return clock, &due, nil
}

func (s *Service) addEvent(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, a Actor, kind, from, to, reason string, detail any) error {
	raw, err := json.Marshal(detail)
	if err != nil || detail == nil {
		raw = []byte("{}")
	}
	var actor any
	if a.ID != uuid.Nil {
		actor = a.ID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO case_events (tenant_id, case_id, seq, at, actor_type, actor_id, kind, from_status, to_status, reason_code, detail)
		VALUES ($1, $2, (SELECT coalesce(max(seq), 0) + 1 FROM case_events WHERE tenant_id = $1 AND case_id = $2),
		        $3, $4, $5, $6, $7, $8, $9, $10)`,
		tenant, caseID, s.now(), string(a.Type), actor, kind, from, to, reason, raw)
	if err != nil {
		return fmt.Errorf("cases: history: %w", err)
	}
	return nil
}

func (s *Service) audit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, a Actor, action string, c Case, extra map[string]any) error {
	d := map[string]any{"case_ref": c.Ref, "classification": c.Classification, "status": c.Status}
	for k, v := range extra {
		d[k] = v
	}
	_, err := s.Audit.Append(ctx, tx, tenant, audit.Event{
		Actor: a.Type, ActorID: a.ID, Action: action, SubjectType: "case", SubjectID: c.Ref, PolicyVersion: c.RuleVersion, Details: d,
	})
	return err
}

const caseColumns = `id, case_ref, classification, status, priority, risk_grade, source, txn_id, exposure_minor, coalesce(currency, ''),
	root_cause, owner_id, queue, rule_id, rule_version, regulatory_clock, opened_at, sla_due_at, resolved_at, closed_reason, complaint, version, internal_target_at`

func scanCase(row pgx.Row) (Case, error) {
	var c Case
	var complaint []byte
	if err := row.Scan(&c.ID, &c.Ref, &c.Classification, &c.Status, &c.Priority, &c.RiskGrade, &c.Source, &c.TxnID, &c.ExposureMinor,
		&c.Currency, &c.RootCause, &c.OwnerID, &c.Queue, &c.RuleID, &c.RuleVersion, &c.RegulatoryClock, &c.OpenedAt, &c.SLADueAt,
		&c.ResolvedAt, &c.ClosedReason, &complaint, &c.Version, &c.InternalTargetAt); err != nil {
		return Case{}, err
	}
	if complaint != nil {
		c.Complaint = complaint
	}
	c.OpenedAt = c.OpenedAt.UTC()
	return c, nil
}

// Open creates a case and returns it. For bot-detected cases it is idempotent:
// the same transaction and classification return the existing case with
// created=false, so a detector can safely see a transaction many times.
func (s *Service) Open(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, in OpenInput) (c Case, created bool, err error) {
	if !knownCodes[in.Classification] {
		return Case{}, false, fmt.Errorf("%w: unknown classification %q", ErrInvalid, in.Classification)
	}
	switch in.Source {
	case SourceBot, SourceCustomer, SourceManual:
	default:
		return Case{}, false, fmt.Errorf("%w: unknown source %q", ErrInvalid, in.Source)
	}
	if in.Source == SourceBot && in.TxnID == nil {
		return Case{}, false, fmt.Errorf("%w: a detected case needs a transaction", ErrInvalid)
	}
	if in.Actor.Type == "" {
		in.Actor = System
	}
	opened := in.At
	if opened.IsZero() {
		opened = s.now()
	}

	if in.Source == SourceBot {
		existing, err := scanCase(tx.QueryRow(ctx, `SELECT `+caseColumns+` FROM cases
			WHERE tenant_id = $1 AND txn_id = $2 AND classification = $3 AND source = 'bot'`, tenant, in.TxnID, in.Classification))
		if err == nil {
			return existing, false, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Case{}, false, err
		}
	}

	wanted := in.Clock
	if wanted == "" {
		wanted = clockFor(in.Classification, in.Rail)
	}
	clock, due, err := s.deadline(ctx, tx, tenant, wanted, opened)
	if err != nil {
		return Case{}, false, err
	}

	var seq int64
	if err := tx.QueryRow(ctx, `SELECT nextval('case_number_seq')`).Scan(&seq); err != nil {
		return Case{}, false, fmt.Errorf("cases: number: %w", err)
	}
	ref := fmt.Sprintf("CASE-%d-%06d", opened.Year(), seq)

	var currency any
	if in.Currency != "" {
		currency = in.Currency
	}
	id := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO cases (tenant_id, id, case_ref, classification, status, priority, risk_grade, source, txn_id, exposure_minor, currency,
			root_cause, rule_id, rule_version, regulatory_clock, opened_at, sla_due_at, complaint, internal_target_at)
		VALUES ($1,$2,$3,$4,'DETECTED',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		tenant, id, ref, in.Classification, priorityFor(in.Classification, in.RiskGrade), in.RiskGrade, in.Source, in.TxnID, in.ExposureMinor,
		currency, in.RootCause, in.RuleID, in.RuleVersion, clock, opened, due, nullJSON(in.Complaint), in.InternalTargetAt)
	if err != nil {
		return Case{}, false, fmt.Errorf("cases: insert: %w", err)
	}
	c, err = scanCase(tx.QueryRow(ctx, `SELECT `+caseColumns+` FROM cases WHERE tenant_id = $1 AND id = $2`, tenant, id))
	if err != nil {
		return Case{}, false, err
	}

	if err := s.addEvent(ctx, tx, tenant, id, in.Actor, "opened", "", StDetected, "", map[string]any{
		"source": in.Source, "rule_id": in.RuleID, "rule_version": in.RuleVersion, "explanation": in.Explanation, "root_cause": in.RootCause}); err != nil {
		return Case{}, false, err
	}
	for _, e := range in.Evidence {
		if err := s.attach(ctx, tx, tenant, id, e); err != nil {
			return Case{}, false, err
		}
	}
	if err := s.audit(ctx, tx, tenant, in.Actor, "case.opened", c, map[string]any{"source": in.Source}); err != nil {
		return Case{}, false, err
	}
	fields := map[string]any{"case_id": c.ID.String(), "case_ref": c.Ref, "classification": c.Classification, "source": c.Source,
		"priority": c.Priority, "status": c.Status}
	if c.TxnID != nil {
		fields["txn_id"] = c.TxnID.String()
	}
	if _, err := events.Emit(ctx, tx, tenant, events.Event{Type: events.CaseOpened, AggregateID: c.ID.String(), Key: c.ID.String(), Fields: fields, OccurredAt: opened}); err != nil {
		return Case{}, false, err
	}
	return c, true, nil
}

func nullJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return []byte(b)
}

func (s *Service) attach(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, e Evidence) error {
	if e.Kind == "" || e.Ref == "" {
		return fmt.Errorf("%w: evidence needs a kind and a reference", ErrInvalid)
	}
	fetched := e.FetchedAt
	if fetched.IsZero() {
		fetched = s.now()
	}
	detail, _ := json.Marshal(e.Detail)
	if e.Detail == nil {
		detail = []byte("{}")
	}
	_, err := tx.Exec(ctx, `INSERT INTO case_evidence (tenant_id, case_id, kind, ref, source_system, fetched_at, hash, is_fact, detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, tenant, caseID, e.Kind, e.Ref, e.SourceSystem, fetched, e.Hash, e.IsFact, detail)
	if err != nil {
		return fmt.Errorf("cases: evidence: %w", err)
	}
	return nil
}

// AddEvidence attaches evidence to an open case and records it in the history.
func (s *Service) AddEvidence(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, a Actor, e Evidence) error {
	c, err := s.Get(ctx, tx, tenant, caseID)
	if err != nil {
		return err
	}
	if err := s.attach(ctx, tx, tenant, caseID, e); err != nil {
		return err
	}
	return s.addEvent(ctx, tx, tenant, caseID, a, "evidence_added", c.Status, c.Status, "", map[string]any{"kind": e.Kind, "ref": e.Ref})
}

// AddNote records a note in the history.
func (s *Service) AddNote(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, a Actor, note string) error {
	note = strings.TrimSpace(note)
	if note == "" || len(note) > 2000 {
		return fmt.Errorf("%w: a note is 1 to 2000 characters", ErrInvalid)
	}
	c, err := s.Get(ctx, tx, tenant, caseID)
	if err != nil {
		return err
	}
	return s.addEvent(ctx, tx, tenant, caseID, a, "note", c.Status, c.Status, "", map[string]any{"note": note})
}

// Get loads one case by id.
func (s *Service) Get(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (Case, error) {
	c, err := scanCase(tx.QueryRow(ctx, `SELECT `+caseColumns+` FROM cases WHERE tenant_id = $1 AND id = $2`, tenant, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrNotFound
	}
	return c, err
}

// GetByRef loads one case by its reference, such as CASE-2026-000012.
func (s *Service) GetByRef(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ref string) (Case, error) {
	c, err := scanCase(tx.QueryRow(ctx, `SELECT `+caseColumns+` FROM cases WHERE tenant_id = $1 AND case_ref = $2`, tenant, ref))
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrNotFound
	}
	return c, err
}

// Transition moves a case to a new status. expectVersion must match the version
// the caller read (0 skips the check for automatic changes). The move must be
// allowed by the state machine. The change, its history entry and its audit
// record commit together.
func (s *Service) Transition(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, a Actor, to string, expectVersion int, reason string) (Case, error) {
	if strings.TrimSpace(reason) == "" {
		return Case{}, fmt.Errorf("%w: a reason code is required", ErrInvalid)
	}
	c, err := scanCase(tx.QueryRow(ctx, `SELECT `+caseColumns+` FROM cases WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenant, caseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrNotFound
	}
	if err != nil {
		return Case{}, err
	}
	if expectVersion != 0 && expectVersion != c.Version {
		return Case{}, ErrStaleVersion
	}
	if !CanTransition(c.Status, to) {
		return Case{}, fmt.Errorf("%w: %s to %s", ErrBadMove, c.Status, to)
	}
	from := c.Status
	now := s.now()
	var resolved any
	closedReason := c.ClosedReason
	if to == StResolved || to == StClosedNoException || to == StClosedCustomerError {
		resolved = now
		closedReason = strings.TrimSpace(reason)
	}

	if c.Status == StExternalDependency && c.SLADueAt != nil && to != StExternalDependency {
		var pausedAt time.Time
		err := tx.QueryRow(ctx, `SELECT at FROM case_events WHERE tenant_id = $1 AND case_id = $2 AND to_status = 'EXTERNAL_DEPENDENCY' ORDER BY seq DESC LIMIT 1`, tenant, caseID).Scan(&pausedAt)
		if err == nil && !pausedAt.IsZero() {
			pauseDuration := now.Sub(pausedAt)
			if pauseDuration > 0 {
				newDue := c.SLADueAt.Add(pauseDuration)
				c.SLADueAt = &newDue
				_, _ = tx.Exec(ctx, `UPDATE cases SET sla_due_at = $3 WHERE tenant_id = $1 AND id = $2`, tenant, caseID, newDue)
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE cases SET status = $3, version = version + 1, updated_at = $4,
			resolved_at = coalesce($5, resolved_at), closed_reason = $6 WHERE tenant_id = $1 AND id = $2`,
		tenant, caseID, to, now, resolved, closedReason); err != nil {
		return Case{}, fmt.Errorf("cases: transition: %w", err)
	}
	c.Status, c.Version = to, c.Version+1
	if err := s.addEvent(ctx, tx, tenant, caseID, a, "transition", from, to, strings.TrimSpace(reason), nil); err != nil {
		return Case{}, err
	}
	if err := s.audit(ctx, tx, tenant, a, "case.transitioned", c, map[string]any{"from": from, "to": to, "reason_code": strings.TrimSpace(reason)}); err != nil {
		return Case{}, err
	}
	if _, err := events.Emit(ctx, tx, tenant, events.Event{Type: events.CaseTransitioned, AggregateID: caseID.String(),
		Key: fmt.Sprintf("%s:%d", caseID, c.Version), OccurredAt: now,
		Fields: map[string]any{"case_id": caseID.String(), "case_ref": c.Ref, "classification": c.Classification, "from_status": from, "to_status": to, "version": c.Version}}); err != nil {
		return Case{}, err
	}
	out, err := scanCase(tx.QueryRow(ctx, `SELECT `+caseColumns+` FROM cases WHERE tenant_id = $1 AND id = $2`, tenant, caseID))
	if err != nil {
		return Case{}, err
	}
	if s.OnClose != nil && IsTerminal(to) {
		if err := s.OnClose(ctx, tx, tenant, out, a.ID); err != nil {
			return Case{}, err
		}
	}
	return out, nil
}

// Assign sets the owner of a case. If a routing service is configured and the assignee
// has active delegations, it routes the assignment to the active delegate up to max hops.
func (s *Service) Assign(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, a Actor, owner uuid.UUID) error {
	c, err := s.Get(ctx, tx, tenant, caseID)
	if err != nil {
		return err
	}
	effectiveOwner := owner
	var via []uuid.UUID
	if s.Routing != nil {
		if routed, err := s.Routing.Assignee(ctx, tx, tenant, owner, s.now()); err == nil {
			effectiveOwner = routed.Assignee
			via = routed.Via
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE cases SET owner_id = $3, version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, caseID, effectiveOwner); err != nil {
		return err
	}
	detail := map[string]any{"owner": effectiveOwner.String()}
	if len(via) > 0 {
		detail["delegated_from"] = owner.String()
		detail["delegation_hops"] = len(via)
	}
	if err := s.addEvent(ctx, tx, tenant, caseID, a, "assigned", c.Status, c.Status, "", detail); err != nil {
		return err
	}
	return s.audit(ctx, tx, tenant, a, "case.assigned", c, detail)
}

// AutoAssign routes a case to an active on-call staff member for the specified role.
// If nobody is on call, it moves the case to the role's queue.
func (s *Service) AutoAssign(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, a Actor, role string) (*Case, error) {
	if s.Routing == nil {
		return nil, errors.New("cases: routing service not configured")
	}
	c, err := s.Get(ctx, tx, tenant, caseID)
	if err != nil {
		return nil, err
	}
	onCall, err := s.Routing.OnCall(ctx, tx, tenant, role, s.now())
	if err != nil {
		return nil, fmt.Errorf("cases: check on-call: %w", err)
	}
	if len(onCall) == 0 {
		if _, err := tx.Exec(ctx, `UPDATE cases SET queue = $3, version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, caseID, role); err != nil {
			return nil, err
		}
		if err := s.addEvent(ctx, tx, tenant, caseID, a, "queue_assigned", c.Status, c.Status, "", map[string]any{"queue": role}); err != nil {
			return nil, err
		}
		updated, err := s.Get(ctx, tx, tenant, caseID)
		return &updated, err
	}
	if err := s.Assign(ctx, tx, tenant, caseID, a, onCall[0]); err != nil {
		return nil, err
	}
	updated, err := s.Get(ctx, tx, tenant, caseID)
	return &updated, err
}

// Escalate moves a case to a higher escalation tier (e.g. SUPERVISOR), bumps priority to CRITICAL,
// assigns the on-call supervisor if available, records history, and emits a case.escalated event.
func (s *Service) Escalate(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, a Actor, targetRole, reason string) (*Case, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("%w: escalation reason is required", ErrInvalid)
	}
	c, err := s.Get(ctx, tx, tenant, caseID)
	if err != nil {
		return nil, err
	}
	if targetRole == "" {
		targetRole = "SUPERVISOR"
	}

	var escalatedTo *uuid.UUID
	if s.Routing != nil {
		if onCall, err := s.Routing.OnCall(ctx, tx, tenant, targetRole, s.now()); err == nil && len(onCall) > 0 {
			escalatedTo = &onCall[0]
		}
	}

	now := s.now()
	query := `UPDATE cases SET priority = 'CRITICAL', queue = $3, version = version + 1, updated_at = $4`
	args := []any{tenant, caseID, targetRole, now}
	if escalatedTo != nil {
		query += `, owner_id = $5 WHERE tenant_id = $1 AND id = $2`
		args = append(args, *escalatedTo)
	} else {
		query += ` WHERE tenant_id = $1 AND id = $2`
	}

	if _, err := tx.Exec(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("cases: escalate update: %w", err)
	}

	eventDetail := map[string]any{"target_role": targetRole, "reason": reason}
	if escalatedTo != nil {
		eventDetail["escalated_to_user"] = escalatedTo.String()
	}
	if err := s.addEvent(ctx, tx, tenant, caseID, a, "escalated", c.Status, c.Status, "", eventDetail); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, tenant, a, "case.escalated", c, eventDetail); err != nil {
		return nil, err
	}

	emitFields := map[string]any{
		"case_id":           caseID.String(),
		"case_ref":          c.Ref,
		"escalated_to_role": targetRole,
		"reason":            reason,
	}
	if escalatedTo != nil {
		emitFields["escalated_to_user"] = escalatedTo.String()
	}
	_, _ = events.Emit(ctx, tx, tenant, events.Event{
		Type:        events.CaseEscalated,
		AggregateID: caseID.String(),
		Key:         fmt.Sprintf("%s:esc:%d", caseID, now.Unix()),
		Fields:      emitFields,
		OccurredAt:  now,
	})

	updated, err := s.Get(ctx, tx, tenant, caseID)
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

// SweepBreachedSLAs detects all open cases whose SLA deadline has passed, sets their priority
// to CRITICAL, triggers automated escalation to SUPERVISOR, and emits a case.sla_breached event.
func (s *Service) SweepBreachedSLAs(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]Case, error) {
	now := s.now()
	rows, err := tx.Query(ctx, `SELECT id FROM cases
		WHERE tenant_id = $1
		  AND status NOT IN ('RESOLVED', 'CLOSED', 'CLOSED_NO_EXCEPTION', 'CLOSED_CUSTOMER_ERROR', 'SUPPRESSED')
		  AND sla_due_at IS NOT NULL
		  AND sla_due_at < $2
		ORDER BY sla_due_at`, tenant, now)
	if err != nil {
		return nil, fmt.Errorf("cases: query breached slas: %w", err)
	}
	defer rows.Close()

	var caseIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		caseIDs = append(caseIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var breached []Case
	for _, id := range caseIDs {
		c, err := s.Get(ctx, tx, tenant, id)
		if err != nil {
			continue
		}
		if c.Priority != "CRITICAL" {
			_, _ = tx.Exec(ctx, `UPDATE cases SET priority = 'CRITICAL', version = version + 1, updated_at = $3 WHERE tenant_id = $1 AND id = $2`, tenant, id, now)
			c.Priority = "CRITICAL"
		}
		_ = s.addEvent(ctx, tx, tenant, id, System, "sla_breached", c.Status, c.Status, "", map[string]any{
			"sla_due_at":  c.SLADueAt.UTC().Format(time.RFC3339),
			"breached_at": now.UTC().Format(time.RFC3339),
		})
		_, _ = events.Emit(ctx, tx, tenant, events.Event{
			Type:        events.CaseSLABreached,
			AggregateID: id.String(),
			Key:         fmt.Sprintf("%s:sla:%d", id, now.Unix()),
			Fields: map[string]any{
				"case_id":        id.String(),
				"case_ref":       c.Ref,
				"classification": c.Classification,
				"priority":       c.Priority,
				"sla_due_at":     c.SLADueAt.UTC().Format(time.RFC3339),
				"breached_at":    now.UTC().Format(time.RFC3339),
			},
			OccurredAt: now,
		})
		if escalated, err := s.Escalate(ctx, tx, tenant, id, System, "SUPERVISOR", "Automated regulatory SLA breach escalation"); err == nil {
			breached = append(breached, *escalated)
		} else {
			breached = append(breached, c)
		}
	}
	return breached, nil
}

// History returns a case's history, oldest first.
func (s *Service) History(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID) ([]Event, error) {
	rows, err := tx.Query(ctx, `SELECT seq, at, actor_type, actor_id, kind, from_status, to_status, reason_code, detail
		FROM case_events WHERE tenant_id = $1 AND case_id = $2 ORDER BY seq`, tenant, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var actor *uuid.UUID
		var detail []byte
		if err := rows.Scan(&e.Seq, &e.At, &e.ActorType, &actor, &e.Kind, &e.FromStatus, &e.ToStatus, &e.ReasonCode, &detail); err != nil {
			return nil, err
		}
		if actor != nil {
			e.ActorID = actor.String()
		}
		e.Detail = detail
		e.At = e.At.UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// Evidence returns a case's evidence, oldest first.
func (s *Service) Evidence(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID) ([]Evidence, error) {
	rows, err := tx.Query(ctx, `SELECT id, kind, ref, source_system, fetched_at, hash, is_fact, detail FROM case_evidence
		WHERE tenant_id = $1 AND case_id = $2 ORDER BY fetched_at, id`, tenant, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Evidence{}
	for rows.Next() {
		var e Evidence
		var detail []byte
		if err := rows.Scan(&e.ID, &e.Kind, &e.Ref, &e.SourceSystem, &e.FetchedAt, &e.Hash, &e.IsFact, &detail); err != nil {
			return nil, err
		}
		e.FetchedAt = e.FetchedAt.UTC()
		e.Detail = json.RawMessage(detail)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Filter narrows List.
type Filter struct {
	Status         string
	Classification string
	Source         string
	TxnID          *uuid.UUID
}

// List returns cases for the queue: highest priority first, then oldest. After
// is the position of the last item of the previous page (seq-like cursor made of
// priority rank, opened time and id).
func (s *Service) List(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, f Filter, after *Cursor, limit int) ([]Case, error) {
	if limit < 1 || limit > 201 {
		return nil, fmt.Errorf("%w: limit out of range", ErrInvalid)
	}
	var rank, id any
	var openedAt any
	if after != nil {
		rank, openedAt, id = after.Rank, after.OpenedAt, after.ID
	}
	rows, err := tx.Query(ctx, `
		SELECT `+caseColumns+` FROM (
			SELECT *, CASE priority WHEN 'CRITICAL' THEN 0 WHEN 'HIGH' THEN 1 WHEN 'MEDIUM' THEN 2 ELSE 3 END AS rank FROM cases
			WHERE tenant_id = $1 AND ($2 = '' OR status = $2) AND ($3 = '' OR classification = $3) AND ($4 = '' OR source = $4)
			  AND ($5::uuid IS NULL OR txn_id = $5)) q
		WHERE ($6::int IS NULL OR (rank, opened_at, id::text) > ($6, $7, $8))
		ORDER BY rank, opened_at, id LIMIT $9`,
		tenant, f.Status, f.Classification, f.Source, f.TxnID, rank, openedAt, id, limit)
	if err != nil {
		return nil, fmt.Errorf("cases: list: %w", err)
	}
	defer rows.Close()
	var out []Case
	for rows.Next() {
		c, err := scanCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Cursor is a position in the queue.
type Cursor struct {
	Rank     int
	OpenedAt time.Time
	ID       string
}

// CursorFor returns the cursor that follows c.
func CursorFor(c Case) Cursor {
	rank := 3
	switch c.Priority {
	case "CRITICAL":
		rank = 0
	case "HIGH":
		rank = 1
	case "MEDIUM":
		rank = 2
	}
	return Cursor{Rank: rank, OpenedAt: c.OpenedAt, ID: c.ID.String()}
}

// RecordEvent adds a history entry that does not change the status. Other
// workflows use it to leave a trace on a case.
func (s *Service) RecordEvent(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, a Actor, kind, status string, detail any) error {
	return s.addEvent(ctx, tx, tenant, caseID, a, kind, status, status, "", detail)
}
