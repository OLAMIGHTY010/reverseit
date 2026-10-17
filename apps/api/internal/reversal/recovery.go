package reversal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/events"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
)

var (
	ErrGateOverrideNotAllowed = errors.New("reversal: gate override not permitted for your role")
	ErrGateOverrideSameActor  = errors.New("reversal: the proposer cannot also approve the gate override")
	ErrGateOverrideMFA        = errors.New("reversal: approving a gate override requires MFA")
	ErrGateOverrideDecided    = errors.New("reversal: gate override has already been decided")
)

// GateOverride represents a dual-control authorization to bypass a blocked reversal gate.
type GateOverride struct {
	ID             uuid.UUID  `json:"id"`
	ReversalID     uuid.UUID  `json:"reversal_id"`
	ProposerID     uuid.UUID  `json:"proposer_id"`
	ApproverID     *uuid.UUID `json:"approver_id,omitempty"`
	Justification  string     `json:"justification"`
	Status         string     `json:"status"` // PENDING, APPROVED, REJECTED
	DecisionReason string     `json:"decision_reason,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
}

// RecoverySubcase links an unresolved/timed-out reversal parent case to its manual resolution subcase.
type RecoverySubcase struct {
	ID           uuid.UUID  `json:"id"`
	ParentCaseID uuid.UUID  `json:"parent_case_id"`
	SubcaseID    uuid.UUID  `json:"subcase_id"`
	ReversalID   uuid.UUID  `json:"reversal_id"`
	Reason       string     `json:"reason"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
}

// RecoveryResult describes the action taken by the timeout recovery worker for one reversal.
type RecoveryResult struct {
	ReversalID  uuid.UUID  `json:"reversal_id"`
	ReversalRef string     `json:"reversal_ref"`
	Outcome     string     `json:"outcome"` // "completed", "rejected", "retried", "escalated_to_subcase"
	SubcaseID   *uuid.UUID `json:"subcase_id,omitempty"`
	Detail      string     `json:"detail"`
}

// ProposeGateOverride records an initial dual-control request to bypass a blocked safety gate.
func (s *Service) ProposeGateOverride(ctx context.Context, tenant, reversalID uuid.UUID, proposer *identity.Principal, justification string) (GateOverride, error) {
	justification = strings.TrimSpace(justification)
	if len(justification) < 10 {
		return GateOverride{}, fmt.Errorf("%w: justification must be at least 10 characters", cases.ErrInvalid)
	}
	if proposer == nil {
		return GateOverride{}, fmt.Errorf("%w: authenticated operator required", cases.ErrInvalid)
	}

	// Proposer must hold override_gate capability (Supervisor or Head of Ops).
	if _, err := s.Policy.Check(proposer, "override_gate", authz.Attrs{}); err != nil {
		return GateOverride{}, fmt.Errorf("%w: %v", ErrGateOverrideNotAllowed, err)
	}

	var out GateOverride
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		rev, err := s.Get(ctx, tx, tenant, reversalID.String())
		if err != nil {
			return err
		}
		if rev.ApprovalStatus == "APPROVED" {
			return fmt.Errorf("%w: reversal is already approved", ErrBadState)
		}

		out = GateOverride{
			ID:            uuid.New(),
			ReversalID:    reversalID,
			ProposerID:    proposer.UserID,
			Justification: justification,
			Status:        "PENDING",
			CreatedAt:     s.now(),
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO gate_overrides (tenant_id, id, reversal_id, proposer_id, justification, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			tenant, out.ID, out.ReversalID, out.ProposerID, out.Justification, out.Status, out.CreatedAt); err != nil {
			return fmt.Errorf("reversal: insert gate override: %w", err)
		}

		actor := cases.Actor{Type: "user", ID: proposer.UserID}
		if err := s.event(ctx, tx, tenant, rev.CaseID, actor, "gate_override_proposed", map[string]any{
			"override_id":   out.ID.String(),
			"reversal_ref":  rev.Ref,
			"justification": justification,
		}); err != nil {
			return err
		}

		return s.auditEvent(ctx, tx, tenant, actor, "reversal.gate_override_proposed", rev.Ref, rev.Gate.PolicyVersion, map[string]any{
			"override_id":   out.ID.String(),
			"justification": justification,
		})
	})
	return out, err
}

// DecideGateOverride performs the secondary dual-control approval or rejection of a gate override.
func (s *Service) DecideGateOverride(ctx context.Context, tenant, overrideID uuid.UUID, decider *identity.Principal, approve bool, reason string) (GateOverride, error) {
	reason = strings.TrimSpace(reason)
	if !approve && reason == "" {
		return GateOverride{}, fmt.Errorf("%w: rejection reason is required", cases.ErrInvalid)
	}
	if decider == nil {
		return GateOverride{}, fmt.Errorf("%w: authenticated operator required", cases.ErrInvalid)
	}

	// Decider must hold override_gate capability.
	if _, err := s.Policy.Check(decider, "override_gate", authz.Attrs{}); err != nil {
		return GateOverride{}, fmt.Errorf("%w: %v", ErrGateOverrideNotAllowed, err)
	}
	// Decider must have MFA.
	if !decider.MFA {
		return GateOverride{}, ErrGateOverrideMFA
	}

	var out GateOverride
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var revID, proposerID uuid.UUID
		var currentStatus, justification string
		var createdAt time.Time
		err := tx.QueryRow(ctx, `
			SELECT reversal_id, proposer_id, justification, status, created_at
			FROM gate_overrides WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenant, overrideID).
			Scan(&revID, &proposerID, &justification, &currentStatus, &createdAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if currentStatus != "PENDING" {
			return ErrGateOverrideDecided
		}
		if decider.UserID == proposerID {
			return ErrGateOverrideSameActor
		}

		rev, err := s.Get(ctx, tx, tenant, revID.String())
		if err != nil {
			return err
		}

		now := s.now()
		newStatus := "REJECTED"
		if approve {
			newStatus = "APPROVED"
		}

		if _, err := tx.Exec(ctx, `
			UPDATE gate_overrides
			SET status = $3, approver_id = $4, decision_reason = $5, decided_at = $6
			WHERE tenant_id = $1 AND id = $2`,
			tenant, overrideID, newStatus, decider.UserID, reason, now); err != nil {
			return fmt.Errorf("reversal: update gate override: %w", err)
		}

		actor := cases.Actor{Type: "user", ID: decider.UserID}
		if approve {
			// Update reversal approval_status to APPROVED so Execute can proceed.
			if _, err := tx.Exec(ctx, `
				UPDATE reversals
				SET approval_status = 'APPROVED', version = version + 1, updated_at = now()
				WHERE tenant_id = $1 AND id = $2`, tenant, revID); err != nil {
				return err
			}
			if err := s.event(ctx, tx, tenant, rev.CaseID, actor, "gate_override_approved", map[string]any{
				"override_id": overrideID.String(),
				"reversal":    rev.Ref,
				"reason":      reason,
			}); err != nil {
				return err
			}
			if err := s.auditEvent(ctx, tx, tenant, actor, "reversal.gate_override_approved", rev.Ref, rev.Gate.PolicyVersion, map[string]any{
				"override_id": overrideID.String(),
				"proposer_id": proposerID.String(),
				"approver_id": decider.UserID.String(),
				"reason":      reason,
			}); err != nil {
				return err
			}
			_ = s.emit(ctx, tx, tenant, events.ReversalApproved, revID, nil)
		} else {
			if err := s.event(ctx, tx, tenant, rev.CaseID, actor, "gate_override_rejected", map[string]any{
				"override_id": overrideID.String(),
				"reversal":    rev.Ref,
				"reason":      reason,
			}); err != nil {
				return err
			}
			if err := s.auditEvent(ctx, tx, tenant, actor, "reversal.gate_override_rejected", rev.Ref, rev.Gate.PolicyVersion, map[string]any{
				"override_id": overrideID.String(),
				"reason":      reason,
			}); err != nil {
				return err
			}
		}

		out = GateOverride{
			ID:             overrideID,
			ReversalID:     revID,
			ProposerID:     proposerID,
			ApproverID:     &decider.UserID,
			Justification:  justification,
			Status:         newStatus,
			DecisionReason: reason,
			CreatedAt:      createdAt,
			DecidedAt:      &now,
		}
		return nil
	})
	return out, err
}

// GetGateOverride retrieves the active or past gate override for a reversal.
func (s *Service) GetGateOverride(ctx context.Context, tx pgx.Tx, tenant, reversalID uuid.UUID) (*GateOverride, error) {
	var o GateOverride
	var appID *uuid.UUID
	var decAt *time.Time
	err := tx.QueryRow(ctx, `
		SELECT id, reversal_id, proposer_id, approver_id, justification, status, decision_reason, created_at, decided_at
		FROM gate_overrides WHERE tenant_id = $1 AND reversal_id = $2`, tenant, reversalID).
		Scan(&o.ID, &o.ReversalID, &o.ProposerID, &appID, &o.Justification, &o.Status, &o.DecisionReason, &o.CreatedAt, &decAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o.ApproverID = appID
	o.DecidedAt = decAt
	return &o, nil
}

// RecoverTimeouts scans for submitted reversals whose outcome is indeterminate (UNKNOWN or IN_PROGRESS past olderThan)
// and executes automated status queries or escalates to a manual recovery sub-case.
func (s *Service) RecoverTimeouts(ctx context.Context, tenant uuid.UUID, olderThan time.Duration) ([]RecoveryResult, error) {
	if olderThan <= 0 {
		olderThan = 5 * time.Minute
	}
	cutoff := s.now().Add(-olderThan)

	var candidates []uuid.UUID
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id FROM reversals
			WHERE tenant_id = $1
			  AND (accepted_status = 'UNKNOWN' OR processing_status = 'IN_PROGRESS')
			  AND financial_status = 'UNVERIFIED'
			  AND submitted_at IS NOT NULL
			  AND submitted_at <= $2
			ORDER BY submitted_at`, tenant, cutoff)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			candidates = append(candidates, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("reversal: query timeout candidates: %w", err)
	}

	var results []RecoveryResult
	for _, id := range candidates {
		res, err := s.recoverSingleTimeout(ctx, tenant, id)
		if err != nil {
			results = append(results, RecoveryResult{
				ReversalID: id,
				Outcome:    "error",
				Detail:     err.Error(),
			})
			continue
		}
		results = append(results, res)
	}
	return results, nil
}

func (s *Service) recoverSingleTimeout(ctx context.Context, tenant, reversalID uuid.UUID) (RecoveryResult, error) {
	var res RecoveryResult
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		rev, err := s.lockReversal(ctx, tx, tenant, reversalID)
		if err != nil {
			return err
		}
		res.ReversalID = rev.ID
		res.ReversalRef = rev.Ref

		// Increment attempt counter.
		newAttempt := rev.AttemptNo + 1
		if _, err := tx.Exec(ctx, `UPDATE reversals SET attempt_no = $3, version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, newAttempt); err != nil {
			return err
		}

		ack, found, perr := s.Provider.Status(ctx, tx, tenant, rev.IdempotencyKey)
		now := s.now()

		if perr == nil && found {
			switch {
			case ack.Accepted == "REJECTED":
				if _, err := tx.Exec(ctx, `UPDATE reversals SET accepted_status = 'REJECTED', processing_status = 'FAILED', financial_status = 'FAILED', failure_reason = $3, provider_ref = $4,
					version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, ack.Reason, ack.ProviderRef); err != nil {
					return err
				}
				sub, _ := s.loadSubject(ctx, tx, tenant, rev.CaseID)
				_ = s.failedReversal(ctx, tx, tenant, rev, sub, cases.System, "PROVIDER_REJECTED", ack.Reason)
				res.Outcome = "rejected"
				res.Detail = "Provider confirmed failure: " + ack.Reason
				return nil

			case ack.Processing == "COMPLETED":
				if _, err := tx.Exec(ctx, `UPDATE reversals SET accepted_status = 'ACCEPTED', processing_status = 'COMPLETED', provider_ref = $3, posted_at = $4,
					version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, ack.ProviderRef, now); err != nil {
					return err
				}
				sub, _ := s.loadSubject(ctx, tx, tenant, rev.CaseID)
				if _, err := s.verify(ctx, tx, tenant, rev, sub, cases.System); err != nil {
					return err
				}
				res.Outcome = "completed"
				res.Detail = "Provider verified completion via async status query"
				return nil
			}
		}

		// If still indeterminate after 3 or more attempts, open a manual recovery sub-case.
		if newAttempt >= 3 {
			c, err := s.Cases.Get(ctx, tx, tenant, rev.CaseID)
			if err != nil {
				return err
			}
			amount := rev.AmountMinor
			tid := rev.TxnID
			subCaseInput := cases.OpenInput{
				Classification: cases.CodeReversalErr,
				Source:         cases.SourceManual,
				TxnID:          &tid,
				ExposureMinor:  &amount,
				Currency:       rev.Currency,
				Rail:           "NIP",
				RootCause:      "timeout_indeterminate",
				RuleID:         "REC-REC-1",
				RuleVersion:    "1.0",
				Explanation:    fmt.Sprintf("Execution timeout recovery: reversal %s for case %s remained indeterminate after %d polling attempts", rev.Ref, c.Ref, newAttempt),
				Actor:          cases.System,
				Evidence: []cases.Evidence{
					{
						Kind:         "reversal_timeout",
						Ref:          rev.Ref,
						SourceSystem: "RECOVERY_WORKER",
						IsFact:       true,
						Detail: map[string]any{
							"attempts":     newAttempt,
							"submitted_at": rev.SubmittedAt,
						},
					},
				},
			}
			subCase, _, err := s.Cases.Open(ctx, tx, tenant, subCaseInput)
			if err != nil {
				return fmt.Errorf("reversal: open recovery subcase: %w", err)
			}

			// Link in recovery_subcases table.
			if _, err := tx.Exec(ctx, `
				INSERT INTO recovery_subcases (tenant_id, parent_case_id, subcase_id, reversal_id, reason)
				VALUES ($1, $2, $3, $4, $5)`,
				tenant, rev.CaseID, subCase.ID, rev.ID, "Unresolved payment rail execution timeout"); err != nil {
				return fmt.Errorf("reversal: link recovery subcase: %w", err)
			}

			// Mark reversal failed so the transaction lock can be handled by the subcase.
			failureReason := fmt.Sprintf("Execution timeout unrecovered after %d attempts; escalated to manual recovery subcase %s", newAttempt, subCase.Ref)
			_, _ = tx.Exec(ctx, `
				UPDATE reversals
				SET processing_status = 'FAILED', financial_status = 'FAILED', failure_reason = $3,
				    version = version + 1, updated_at = now()
				WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, failureReason)

			actor := cases.System
			_ = s.event(ctx, tx, tenant, rev.CaseID, actor, "timeout_subcase_opened", map[string]any{
				"reversal":    rev.Ref,
				"subcase_id":  subCase.ID.String(),
				"subcase_ref": subCase.Ref,
				"attempt_no":  newAttempt,
			})
			_ = s.auditEvent(ctx, tx, tenant, actor, "reversal.timeout_subcase_opened", rev.Ref, rev.Gate.PolicyVersion, map[string]any{
				"subcase_id":  subCase.ID.String(),
				"subcase_ref": subCase.Ref,
				"attempt_no":  newAttempt,
			})
			_ = s.emit(ctx, tx, tenant, events.ReversalFailed, rev.ID, map[string]any{
				"failure_code": "TIMEOUT_MANUAL_RECOVERY",
			})
			_, _ = notify.Record(ctx, tx, tenant, rev.CaseID,
				notify.OpsReversalFailed(c.Ref, rev.Ref, failureReason),
				c.Ref+":ops:timeout:"+rev.Ref)

			res.Outcome = "escalated_to_subcase"
			res.SubcaseID = &subCase.ID
			res.Detail = fmt.Sprintf("Escalated to recovery subcase %s after %d polling attempts", subCase.Ref, newAttempt)
			return nil
		}

		res.Outcome = "retried"
		res.Detail = fmt.Sprintf("Polling attempt %d recorded; status remains indeterminate", newAttempt)
		return nil
	})
	return res, err
}

// RecoverySubcases returns all manual recovery sub-cases linked to a parent case.
func (s *Service) RecoverySubcases(ctx context.Context, tx pgx.Tx, tenant, parentCaseID uuid.UUID) ([]RecoverySubcase, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, parent_case_id, subcase_id, reversal_id, reason, status, created_at, resolved_at
		FROM recovery_subcases WHERE tenant_id = $1 AND parent_case_id = $2 ORDER BY created_at`, tenant, parentCaseID)
	if err != nil {
		return nil, fmt.Errorf("reversal: query recovery subcases: %w", err)
	}
	defer rows.Close()

	var out []RecoverySubcase
	for rows.Next() {
		var sub RecoverySubcase
		var resAt *time.Time
		if err := rows.Scan(&sub.ID, &sub.ParentCaseID, &sub.SubcaseID, &sub.ReversalID, &sub.Reason, &sub.Status, &sub.CreatedAt, &resAt); err != nil {
			return nil, err
		}
		sub.ResolvedAt = resAt
		out = append(out, sub)
	}
	return out, rows.Err()
}
