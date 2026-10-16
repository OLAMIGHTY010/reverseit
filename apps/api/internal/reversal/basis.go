package reversal

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
)

// ErrApprovalInvalidated means the policy an approval relied on has changed and the
// reversal still needs a person's approval, so the earlier one no longer counts.
var ErrApprovalInvalidated = errors.New("reversal: the policy changed since this was approved; a fresh approval is needed")

// authorityBasis lists the versions of every setting that decides who may authorise
// a reversal and for how long: the approval limits and validity period, whether the
// exception type may be reversed automatically, and the execution flag. A setting
// with no approved value is simply absent, so adding or removing one also changes
// the basis. Settings that only describe the evidence (freshness, waiting times) are
// not part of it: they are checked afresh by the gate on every submission.
func (s *Service) authorityBasis(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, code string) (string, error) {
	at := s.now()
	found, err := s.Config.EffectiveByPrefix(ctx, tx, tenant, "approval.", at)
	if err != nil {
		return "", err
	}
	for _, key := range []string{"reversal.auto_eligible." + code, "reversal.simulated_execution_enabled"} {
		v, err := s.Config.Effective(ctx, tx, tenant, key, at)
		if errors.Is(err, configsvc.ErrNotConfigured) || errors.Is(err, configsvc.ErrUnknownKey) {
			continue
		}
		if err != nil {
			return "", err
		}
		found[key] = v
	}
	parts := make([]string, 0, len(found))
	for k, v := range found {
		parts = append(parts, fmt.Sprintf("%s@%d", k, v.Version))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";"), nil
}

// approvalBasis returns the authority basis the newest approval was given under.
func (s *Service) approvalBasis(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, revID uuid.UUID) (string, error) {
	var basis string
	err := tx.QueryRow(ctx, `SELECT authority_basis FROM approvals WHERE tenant_id = $1 AND reversal_id = $2 AND status = 'APPROVED'
		ORDER BY created_at DESC, id DESC LIMIT 1`, tenant, revID).Scan(&basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return basis, err
}

// escalateToApproval is for a reversal that policy let automation submit but which
// has not been submitted, when policy now says a person must approve it. Nobody
// approved it, so there is no earlier authority to carry over. It becomes a
// reversal awaiting approval, with the system as maker, and keeps its remediation
// lock. Repeating it changes nothing.
func (s *Service) escalateToApproval(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, rev Reversal, sub subject, a cases.Actor, reason string) error {
	tag, err := tx.Exec(ctx, `UPDATE reversals SET approval_status = 'PENDING', parked_at = NULL, parked_scope = '', parked_target = '', version = version + 1, updated_at = now()
		WHERE tenant_id = $1 AND id = $2 AND approval_status = 'NOT_REQUIRED'`, tenant, rev.ID)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO approvals (tenant_id, reversal_id, maker, maker_type, reason) VALUES ($1,$2,NULL,'system',$3)`,
		tenant, rev.ID, "Policy changed after this was recorded: "+reason); err != nil {
		return fmt.Errorf("reversal: escalate: %w", err)
	}
	if err := s.moveCase(ctx, tx, tenant, sub.Case, cases.StAwaitingApproval, a, "POLICY_NOW_REQUIRES_APPROVAL"); err != nil {
		return err
	}
	if err := s.event(ctx, tx, tenant, rev.CaseID, a, "approval_now_required", map[string]any{"reversal": rev.Ref, "why": reason}); err != nil {
		return err
	}
	if _, err := notify.Record(ctx, tx, tenant, rev.CaseID, notify.OpsAwaitingApproval(sub.Case.Ref, reason), sub.Case.Ref+":ops:escalated:"+rev.Ref); err != nil {
		return err
	}
	return s.auditEvent(ctx, tx, tenant, a, "reversal.escalated_to_approval", rev.Ref, rev.PolicyVersion, map[string]any{"case": sub.Case.Ref, "why": reason})
}
