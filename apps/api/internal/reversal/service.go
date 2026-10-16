package reversal

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
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/events"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Errors.
var (
	ErrNotFound   = errors.New("reversal: not found")
	ErrKeyReuse   = errors.New("reversal: that idempotency key was used for a different request")
	ErrBadState   = errors.New("reversal: not in a state that allows this")
	ErrNotPending = errors.New("reversal: this approval was already decided")
	// ErrNoApprovalPolicy means no approved approval.validity_seconds exists, so an
	// approval cannot be given: without a validity period it would never expire.
	ErrNoApprovalPolicy = errors.New("reversal: no approved approval validity period, so nothing can be approved")
	ErrSameActor        = authz.ErrSameActor
)

// Service runs reversals.
type Service struct {
	Pool     *db.Pool
	Config   *configsvc.Service
	Kill     *killswitch.Service
	Cases    *cases.Service
	Audit    *audit.Appender
	Txn      *txn.Service
	Policy   *authz.Policy
	SoD      authz.SoD
	Status   StatusSource
	Ledger   LedgerSource
	Provider Provider
	Now      func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Reversal is a stored reversal with its three separate tracks.
type Reversal struct {
	ID               uuid.UUID  `json:"id"`
	Ref              string     `json:"reversal_ref"`
	CaseID           uuid.UUID  `json:"case_id"`
	TxnID            uuid.UUID  `json:"txn_id"`
	IdempotencyKey   string     `json:"idempotency_key"`
	RequestedBy      string     `json:"requested_by,omitempty"`
	RequestedByType  string     `json:"requested_by_type"`
	AmountMinor      int64      `json:"-"`
	Currency         string     `json:"currency"`
	ReasonCode       string     `json:"reason_code"`
	ApprovalStatus   string     `json:"approval_status"`
	AcceptedStatus   string     `json:"accepted_status"`
	ProcessingStatus string     `json:"processing_status"`
	FinancialStatus  string     `json:"financial_status"`
	Gate             Gate       `json:"gate"`
	ProviderRef      string     `json:"provider_ref,omitempty"`
	FailureReason    string     `json:"failure_reason,omitempty"`
	AttemptNo        int        `json:"attempt_no"`
	SubmittedAt      *time.Time `json:"submitted_at,omitempty"`
	PostedAt         *time.Time `json:"posted_at,omitempty"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	ApprovalID       uuid.UUID  `json:"approval_id,omitempty"`
	Version          int        `json:"version"`
	// ParkedAt is set while a freeze holds back submission of an approved reversal.
	ParkedAt *time.Time `json:"parked_at,omitempty"`
	// PolicyID names the policy that governed the decision and does not change.
	// PolicyVersion is the versions of the configuration in force when the
	// reversal was decided, and is never rewritten.
	PolicyID      string `json:"policy_id"`
	PolicyVersion string `json:"policy_version"`
	// ApprovalExpiresAt is when the newest approval stops being valid.
	ApprovalExpiresAt *time.Time `json:"approval_expires_at,omitempty"`
}

// PolicyID is the stable identity of the policy that governs reversals of one
// exception type on one rail. A policy-scope kill switch freeze targets it.
func PolicyID(code, rail string) string { return "reversal." + code + "." + rail }

const reversalColumns = `r.id, r.reversal_ref, r.case_id, r.txn_id, r.idempotency_key, coalesce(r.requested_by::text, ''), r.requested_by_type,
	r.amount_minor, r.currency, r.reason_code, r.approval_status, r.accepted_status, r.processing_status, r.financial_status, r.gate_result,
	r.provider_ref, r.failure_reason, r.attempt_no, r.submitted_at, r.posted_at, r.verified_at, r.created_at, coalesce(a.id, '00000000-0000-0000-0000-000000000000'), r.version, r.parked_at, r.policy_id, r.policy_version, a.expires_at`

func scanReversal(row pgx.Row) (Reversal, error) {
	var r Reversal
	var gate []byte
	if err := row.Scan(&r.ID, &r.Ref, &r.CaseID, &r.TxnID, &r.IdempotencyKey, &r.RequestedBy, &r.RequestedByType, &r.AmountMinor, &r.Currency,
		&r.ReasonCode, &r.ApprovalStatus, &r.AcceptedStatus, &r.ProcessingStatus, &r.FinancialStatus, &gate, &r.ProviderRef, &r.FailureReason,
		&r.AttemptNo, &r.SubmittedAt, &r.PostedAt, &r.VerifiedAt, &r.CreatedAt, &r.ApprovalID, &r.Version, &r.ParkedAt, &r.PolicyID, &r.PolicyVersion, &r.ApprovalExpiresAt); err != nil {
		return r, err
	}
	_ = json.Unmarshal(gate, &r.Gate)
	r.CreatedAt = r.CreatedAt.UTC()
	return r, nil
}

// A reversal may have had several approvals; the newest is the one that counts.
const reversalFrom = ` FROM reversals r LEFT JOIN LATERAL (SELECT id, expires_at FROM approvals WHERE tenant_id = r.tenant_id AND reversal_id = r.id
	ORDER BY created_at DESC, id DESC LIMIT 1) a ON true`

// Get loads a reversal by id or reference.
func (s *Service) Get(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, idOrRef string) (Reversal, error) {
	r, err := scanReversal(tx.QueryRow(ctx, `SELECT `+reversalColumns+reversalFrom+` WHERE r.tenant_id = $1 AND (r.id::text = $2 OR r.reversal_ref = $2)`, tenant, idOrRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return Reversal{}, ErrNotFound
	}
	return r, err
}

// ForCase lists a case's reversals.
func (s *Service) ForCase(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID) ([]Reversal, error) {
	rows, err := tx.Query(ctx, `SELECT `+reversalColumns+reversalFrom+` WHERE r.tenant_id = $1 AND r.case_id = $2 ORDER BY r.created_at`, tenant, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reversal{}
	for rows.Next() {
		r, err := scanReversal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) loadSubject(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID) (subject, error) {
	c, err := s.Cases.Get(ctx, tx, tenant, caseID)
	if err != nil {
		return subject{}, err
	}
	if c.TxnID == nil {
		return subject{}, fmt.Errorf("%w: the case has no transaction to reverse", ErrBadState)
	}
	t, err := loadTxn(ctx, tx, tenant, *c.TxnID)
	if err != nil {
		return subject{}, err
	}
	return subject{Case: c, Txn: t}, nil
}

// Assess evaluates the eight checks for a case without changing anything. It is
// what a screen shows as "reversal eligibility".
func (s *Service) Assess(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, who Requester) (Gate, error) {
	sub, err := s.loadSubject(ctx, tx, tenant, caseID)
	if err != nil {
		return Gate{}, err
	}
	return s.evaluate(ctx, tx, tenant, sub, who, "assessment", false)
}

// RequestInput describes a reversal request.
type RequestInput struct {
	CaseID         uuid.UUID
	IdempotencyKey string
	ReasonCode     string
	Who            Requester
}

// Outcome of Request.
type Outcome struct {
	Reversal Reversal
	Gate     Gate
	// Blocked is true when the gate failed. No reversal exists then.
	Blocked bool
	// Existing is true when the idempotency key already had a reversal.
	Existing bool
}

func requestHash(caseID uuid.UUID, reason string) string {
	sum := sha256.Sum256([]byte(caseID.String() + "|" + strings.TrimSpace(reason)))
	return hex.EncodeToString(sum[:])
}

func (s *Service) actor(who Requester) (cases.Actor, string, any) {
	if who.system() {
		return cases.System, "system", nil
	}
	return cases.Actor{Type: audit.ActorUser, ID: who.Principal.UserID}, "user", who.Principal.UserID
}

// Request evaluates the gate and, if it passes, records the reversal and takes
// the remediation lock. It does not call the provider. Repeating a request with
// the same idempotency key returns the same reversal.
func (s *Service) Request(ctx context.Context, tenant uuid.UUID, in RequestInput) (Outcome, error) {
	if strings.TrimSpace(in.ReasonCode) == "" {
		return Outcome{}, fmt.Errorf("%w: a reason code is required", cases.ErrInvalid)
	}
	var out Outcome
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		// Serialise requests for one case.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM cases WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenant, in.CaseID); err != nil {
			return err
		}
		hash := requestHash(in.CaseID, in.ReasonCode)
		var existingHash string
		var existingID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id, request_hash FROM reversals WHERE tenant_id = $1 AND idempotency_key = $2`, tenant, in.IdempotencyKey).Scan(&existingID, &existingHash)
		switch {
		case err == nil && existingHash == hash:
			r, gerr := s.Get(ctx, tx, tenant, existingID.String())
			out = Outcome{Reversal: r, Gate: r.Gate, Existing: true}
			return gerr
		case err == nil:
			return ErrKeyReuse
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}

		sub, err := s.loadSubject(ctx, tx, tenant, in.CaseID)
		if err != nil {
			return err
		}
		gate, err := s.evaluate(ctx, tx, tenant, sub, in.Who, in.IdempotencyKey, false)
		if err != nil {
			return err
		}
		actor, actorType, actorID := s.actor(in.Who)

		if gate.Verdict == Blocked {
			out = Outcome{Gate: gate, Blocked: true}
			return s.recordBlocked(ctx, tx, tenant, sub, actor, gate)
		}

		approval := "NOT_REQUIRED"
		if gate.Verdict == PassNeedsApproval {
			approval = "PENDING"
		}
		var seq int64
		if err := tx.QueryRow(ctx, `SELECT nextval('reversal_number_seq')`).Scan(&seq); err != nil {
			return err
		}
		ref := fmt.Sprintf("RV-%06d", seq)
		id := uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO reversals (tenant_id, id, reversal_ref, case_id, txn_id, idempotency_key, request_hash, requested_by, requested_by_type,
				amount_minor, currency, reason_code, approval_status, gate_result, policy_version, policy_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
			tenant, id, ref, in.CaseID, sub.Txn.ID, in.IdempotencyKey, hash, actorID, actorType, sub.Txn.Amount, sub.Txn.Currency,
			strings.TrimSpace(in.ReasonCode), approval, gate.json(), gate.PolicyVersion, PolicyID(sub.Case.Classification, sub.Txn.Rail))
		if err != nil {
			var pg *pgconn.PgError
			if errors.As(err, &pg) && pg.Code == "23505" && strings.Contains(pg.ConstraintName, "one_live") {
				// Another request got the remediation lock first.
				gate.Checks[4] = Check{ID: 5, Name: checkNames[5], Passed: false, Detail: "another reversal took the remediation lock for this transaction"}
				gate.Verdict = Blocked
				out = Outcome{Gate: gate, Blocked: true}
				return s.recordBlocked(ctx, tx, tenant, sub, actor, gate)
			}
			return fmt.Errorf("reversal: record: %w", err)
		}
		if approval == "PENDING" {
			if _, err := tx.Exec(ctx, `INSERT INTO approvals (tenant_id, reversal_id, maker, maker_type, reason) VALUES ($1,$2,$3,$4,$5)`,
				tenant, id, actorID, actorType, gate.ApprovalReason); err != nil {
				return fmt.Errorf("reversal: approval: %w", err)
			}
			if err := s.moveCase(ctx, tx, tenant, sub.Case, cases.StAwaitingApproval, actor, "APPROVAL_REQUIRED"); err != nil {
				return err
			}
			if _, err := notify.Record(ctx, tx, tenant, in.CaseID, notify.OpsAwaitingApproval(sub.Case.Ref, gate.ApprovalReason), sub.Case.Ref+":ops:awaiting:"+ref); err != nil {
				return err
			}
		}
		if err := s.event(ctx, tx, tenant, in.CaseID, actor, "reversal_requested", map[string]any{"reversal": ref, "verdict": gate.Verdict, "approval": approval, "checks_passed": len(gate.Checks)}); err != nil {
			return err
		}
		if err := s.auditEvent(ctx, tx, tenant, actor, "reversal.requested", ref, gate.PolicyVersion, map[string]any{"case": sub.Case.Ref, "approval": approval, "amount_minor": sub.Txn.Amount}); err != nil {
			return err
		}
		if err := s.emit(ctx, tx, tenant, events.ReversalRequested, id, nil); err != nil {
			return err
		}
		r, err := s.Get(ctx, tx, tenant, id.String())
		out = Outcome{Reversal: r, Gate: gate}
		return err
	})
	return out, err
}

func (s *Service) event(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID, a cases.Actor, kind string, detail any) error {
	c, err := s.Cases.Get(ctx, tx, tenant, caseID)
	if err != nil {
		return err
	}
	return s.Cases.RecordEvent(ctx, tx, tenant, caseID, a, kind, c.Status, detail)
}

func (s *Service) auditEvent(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, a cases.Actor, action, ref, policy string, details map[string]any) error {
	// Every audit record about a reversal names the policy that governed it.
	if strings.HasPrefix(ref, "RV-") {
		var id string
		if err := tx.QueryRow(ctx, `SELECT policy_id FROM reversals WHERE tenant_id = $1 AND reversal_ref = $2`, tenant, ref).Scan(&id); err == nil && id != "" {
			d := map[string]any{"policy_id": id}
			for k, v := range details {
				d[k] = v
			}
			details = d
		}
	}
	_, err := s.Audit.Append(ctx, tx, tenant, audit.Event{Actor: a.Type, ActorID: a.ID, Action: action, SubjectType: "reversal", SubjectID: ref, PolicyVersion: policy, Details: details})
	return err
}

// moveCase moves a case towards a status, taking the intermediate steps the state
// machine needs. It does nothing if the case is already there.
func (s *Service) moveCase(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, c cases.Case, to string, a cases.Actor, reason string) error {
	if c.Status == to {
		return nil
	}
	cur, err := s.Cases.Get(ctx, tx, tenant, c.ID)
	if err != nil {
		return err
	}
	if cur.Status == to {
		return nil
	}
	if !cases.CanTransition(cur.Status, to) {
		if cur.Status == cases.StDetected && cases.CanTransition(cases.StUnderInvestigation, to) {
			if _, err := s.Cases.Transition(ctx, tx, tenant, c.ID, a, cases.StUnderInvestigation, 0, "AUTO_TRIAGE"); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("%w: case %s cannot move from %s to %s", ErrBadState, c.Ref, cur.Status, to)
		}
	}
	_, err = s.Cases.Transition(ctx, tx, tenant, c.ID, a, to, 0, reason)
	return err
}

func (s *Service) recordBlocked(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, sub subject, a cases.Actor, g Gate) error {
	failed := g.Failed()
	ids := make([]string, 0, len(failed))
	reasons := make([]string, 0, len(failed))
	for _, c := range failed {
		ids = append(ids, fmt.Sprint(c.ID))
		reasons = append(reasons, fmt.Sprintf("check %d (%s): %s", c.ID, c.Name, c.Detail))
	}
	sig := strings.Join(ids, ",")
	if err := s.Cases.RecordEvent(ctx, tx, tenant, sub.Case.ID, a, "reversal_blocked", sub.Case.Status, map[string]any{"failed_checks": failed, "status_outcome": g.StatusOutcome}); err != nil {
		return err
	}
	if _, err := notify.Record(ctx, tx, tenant, sub.Case.ID, notify.OpsHeld(sub.Case.Ref, strings.Join(reasons, "; ")), sub.Case.Ref+":ops:held:"+sig); err != nil {
		return err
	}
	return s.auditEvent(ctx, tx, tenant, a, "reversal.blocked", sub.Case.Ref, g.PolicyVersion, map[string]any{"failed_checks": sig, "status_outcome": g.StatusOutcome, "policy_id": PolicyID(sub.Case.Classification, sub.Txn.Rail)})
}
