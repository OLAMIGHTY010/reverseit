package reversal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
)

// ErrApprovalExpired means the reversal's approval is no longer valid, so nothing
// was submitted. A fresh approval is requested with RequestReapproval.
var ErrApprovalExpired = errors.New("reversal: the approval is no longer valid; a fresh approval is needed")

// approvalStillValid reports whether the newest approval is still authority. An
// approval with no recorded expiry is not valid: nothing is trusted by default.
func (s *Service) approvalStillValid(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, rev Reversal) (bool, string, error) {
	var status string
	var expires *time.Time
	err := tx.QueryRow(ctx, `SELECT status, expires_at FROM approvals WHERE tenant_id = $1 AND reversal_id = $2
		ORDER BY created_at DESC, id DESC LIMIT 1`, tenant, rev.ID).Scan(&status, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "there is no approval on record", nil
	}
	if err != nil {
		return false, "", err
	}
	switch {
	case status != "APPROVED":
		return false, "the newest approval is " + strings.ToLower(status), nil
	case expires == nil:
		return false, "the approval has no recorded expiry", nil
	case !s.now().Before(*expires):
		return false, "the approval expired at " + expires.UTC().Format(time.RFC3339), nil
	}
	return true, "", nil
}

// expireApproval records that a reversal's approval has lapsed. The reversal keeps
// its remediation lock and its unsubmitted state; it simply cannot be submitted
// until a fresh approval is given. The lapse is written to the case history, the
// audit trail and the operations alerts, once.
func (s *Service) expireApproval(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, rev Reversal, sub subject, a cases.Actor, why string) error {
	return s.expireApprovalAs(ctx, tx, tenant, rev, sub, a, why, "approval_expired", "reversal.approval_expired")
}

// expireApprovalAs is expireApproval with the names used in the case history and
// the audit trail, so a lapse and a policy change are told apart.
func (s *Service) expireApprovalAs(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, rev Reversal, sub subject, a cases.Actor, why, caseKind, auditAction string) error {
	tag, err := tx.Exec(ctx, `UPDATE reversals SET approval_status = 'EXPIRED', version = version + 1, updated_at = now()
		WHERE tenant_id = $1 AND id = $2 AND approval_status = 'APPROVED'`, tenant, rev.ID)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	if err := s.event(ctx, tx, tenant, rev.CaseID, a, caseKind, map[string]any{"reversal": rev.Ref, "why": why}); err != nil {
		return err
	}
	if _, err := notify.Record(ctx, tx, tenant, rev.CaseID, notify.OpsApprovalExpired(sub.Case.Ref, rev.Ref, why), sub.Case.Ref+":ops:expired:"+rev.Ref+":"+fmt.Sprint(rev.Version)); err != nil {
		return err
	}
	return s.auditEvent(ctx, tx, tenant, a, auditAction, rev.Ref, rev.PolicyVersion, map[string]any{"case": sub.Case.Ref, "why": why})
}

// ExpireDue marks every approved but unsubmitted reversal whose approval has
// lapsed. It submits nothing and is not held back by a freeze, because recording a
// lapse is not an action on money. It returns how many it marked.
func (s *Service) ExpireDue(ctx context.Context, tenant uuid.UUID) (int, error) {
	var ids []uuid.UUID
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT r.id FROM reversals r WHERE r.tenant_id = $1 AND r.approval_status = 'APPROVED'
			AND r.accepted_status IN ('PENDING', 'UNKNOWN') AND r.financial_status = 'UNVERIFIED'`, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			rev, err := s.lockReversal(ctx, tx, tenant, id)
			if err != nil || rev.ApprovalStatus != "APPROVED" {
				return err
			}
			ok, why, err := s.approvalStillValid(ctx, tx, tenant, rev)
			if err != nil || ok {
				return err
			}
			sub, err := s.loadSubject(ctx, tx, tenant, rev.CaseID)
			if err != nil {
				return err
			}
			if err := s.expireApproval(ctx, tx, tenant, rev, sub, cases.System, why); err != nil {
				return err
			}
			n++
			return nil
		})
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// BlockedError is returned when the checks no longer allow a fresh approval to be
// requested. It carries the gate so callers can show the failed checks.
type BlockedError struct{ Gate Gate }

func (e *BlockedError) Error() string {
	return "reversal: the checks no longer pass, so a fresh approval cannot be requested"
}

// RequestReapproval asks for a fresh approval of a reversal whose approval has
// lapsed. It is the safe route back: the same reversal keeps its remediation lock,
// the checks are run again with the requester as maker, and a different person with
// the authority must approve. Nothing is submitted. Asking again while a fresh
// approval is waiting returns the same reversal.
func (s *Service) RequestReapproval(ctx context.Context, tenant uuid.UUID, who *identity.Principal, reversalID uuid.UUID, reason string) (Reversal, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Reversal{}, fmt.Errorf("%w: a reason is required", cases.ErrInvalid)
	}
	var out Reversal
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		rev, err := s.lockReversal(ctx, tx, tenant, reversalID)
		if err != nil {
			return err
		}
		out = rev
		if rev.ApprovalStatus == "PENDING" {
			return nil // a fresh approval is already waiting
		}
		if rev.ApprovalStatus != "EXPIRED" {
			return fmt.Errorf("%w: the approval is %s, not expired", ErrBadState, rev.ApprovalStatus)
		}
		if rev.AcceptedStatus != "PENDING" && rev.AcceptedStatus != "UNKNOWN" {
			return fmt.Errorf("%w: the reversal was already submitted", ErrBadState)
		}
		sub, err := s.loadSubject(ctx, tx, tenant, rev.CaseID)
		if err != nil {
			return err
		}
		gate, err := s.evaluateOwn(ctx, tx, tenant, sub, Requester{Principal: who}, rev.IdempotencyKey, false, rev.ID)
		if err != nil {
			return err
		}
		if gate.Verdict == Blocked {
			return &BlockedError{Gate: gate}
		}
		actor := cases.Actor{Type: "user", ID: who.UserID}
		if _, err := tx.Exec(ctx, `INSERT INTO approvals (tenant_id, reversal_id, maker, maker_type, reason) VALUES ($1,$2,$3,'user',$4)`,
			tenant, rev.ID, who.UserID, "Fresh approval: "+reason); err != nil {
			return fmt.Errorf("reversal: fresh approval: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE reversals SET approval_status = 'PENDING', version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID); err != nil {
			return err
		}
		if err := s.moveCase(ctx, tx, tenant, sub.Case, cases.StAwaitingApproval, actor, "FRESH_APPROVAL_REQUESTED"); err != nil {
			return err
		}
		if err := s.event(ctx, tx, tenant, rev.CaseID, actor, "reapproval_requested", map[string]any{"reversal": rev.Ref, "reason": reason}); err != nil {
			return err
		}
		if _, err := notify.Record(ctx, tx, tenant, rev.CaseID, notify.OpsAwaitingApproval(sub.Case.Ref, "fresh approval after expiry: "+reason),
			sub.Case.Ref+":ops:reapproval:"+rev.Ref+":"+fmt.Sprint(rev.Version)); err != nil {
			return err
		}
		if err := s.auditEvent(ctx, tx, tenant, actor, "reversal.reapproval_requested", rev.Ref, rev.PolicyVersion, map[string]any{"case": sub.Case.Ref, "reason": reason}); err != nil {
			return err
		}
		out, err = s.Get(ctx, tx, tenant, rev.ID.String())
		return err
	})
	return out, err
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
