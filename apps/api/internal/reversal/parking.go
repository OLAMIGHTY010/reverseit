package reversal

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
)

// frozenFor reports the freeze that covers submitting this case's reversal, if
// any. An error means the switch could not be read, and the caller must not act.
func (s *Service) frozenFor(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, sub subject) (*killswitch.FrozenError, error) {
	err := s.Kill.Check(ctx, tx, tenant, actionFor(sub.Case.Classification, sub.Txn.Rail))
	var fe *killswitch.FrozenError
	switch {
	case err == nil:
		return nil, nil
	case errors.As(err, &fe):
		return fe, nil
	default:
		return nil, err
	}
}

// park holds an approved or authorised reversal back because a freeze covers it.
// The reversal is not failed and not cancelled: it keeps its approval and its
// remediation lock, and the case says why it has not moved. The first parking is
// recorded in the case history and the audit trail; repeating it changes nothing.
func (s *Service) park(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, rev Reversal, sub subject, fe *killswitch.FrozenError, a cases.Actor, out *Reversal) error {
	tag, err := tx.Exec(ctx, `UPDATE reversals SET parked_at = now(), parked_scope = $3, parked_target = $4, version = version + 1, updated_at = now()
		WHERE tenant_id = $1 AND id = $2 AND parked_at IS NULL`, tenant, rev.ID, string(fe.Scope), fe.Target)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		if err := s.event(ctx, tx, tenant, rev.CaseID, a, "reversal_parked", map[string]any{"reversal": rev.Ref, "scope": string(fe.Scope), "target": fe.Target}); err != nil {
			return err
		}
		if err := s.auditEvent(ctx, tx, tenant, a, "reversal.parked", rev.Ref, rev.Gate.PolicyVersion,
			map[string]any{"case": sub.Case.Ref, "scope": string(fe.Scope), "target": fe.Target}); err != nil {
			return err
		}
	}
	r, err := s.Get(ctx, tx, tenant, rev.ID.String())
	*out = r
	return err
}

// unpark records that a parked reversal passed its checks again and is being
// submitted. It does nothing for a reversal that was never parked.
func (s *Service) unpark(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, rev Reversal, a cases.Actor) error {
	tag, err := tx.Exec(ctx, `UPDATE reversals SET parked_at = NULL, parked_scope = '', parked_target = '', version = version + 1, updated_at = now()
		WHERE tenant_id = $1 AND id = $2 AND parked_at IS NOT NULL`, tenant, rev.ID)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	if err := s.event(ctx, tx, tenant, rev.CaseID, a, "reversal_resumed", map[string]any{"reversal": rev.Ref}); err != nil {
		return err
	}
	return s.auditEvent(ctx, tx, tenant, a, "reversal.resumed", rev.Ref, rev.Gate.PolicyVersion, map[string]any{"after": "unfreeze"})
}

// actionFor describes a reversal submission to the kill switch: its rail, its
// exception type and the policy that governs it. If either part of the policy
// identity is missing the policy is unknown, which every policy freeze covers.
func actionFor(code, rail string) killswitch.Action {
	id := ""
	if code != "" && rail != "" {
		id = PolicyID(code, rail)
	}
	return killswitch.Action{Rail: rail, ExceptionType: code, PolicyID: id, PolicyUnknown: id == ""}
}
