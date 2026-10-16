package reversal

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/events"
)

// emit writes a domain event for a reversal in the caller's transaction, after
// the state it describes has been stored. The payload is read back from the
// stored reversal, so an event always reports what the database holds: for
// instance reversal.verified is emitted only from the ledger-verified path, and
// never because a provider said it was done. Each type is emitted at most once
// per reversal.
func (s *Service) emit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, typ string, id uuid.UUID, extra map[string]any) error {
	return s.emitKeyed(ctx, tx, tenant, typ, id, "", extra)
}

// emitKeyed is emit with an explicit occurrence key, for events that can happen
// more than once to a reversal (a fresh approval after the first lapsed).
func (s *Service) emitKeyed(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, typ string, id uuid.UUID, key string, extra map[string]any) error {
	rev, err := s.Get(ctx, tx, tenant, id.String())
	if err != nil {
		return err
	}
	fields := map[string]any{"reversal_id": rev.ID.String(), "reversal_ref": rev.Ref, "case_id": rev.CaseID.String(), "txn_id": rev.TxnID.String(),
		"amount_minor": strconv.FormatInt(rev.AmountMinor, 10), "currency": rev.Currency, "approval_status": rev.ApprovalStatus,
		"accepted_status": rev.AcceptedStatus, "processing_status": rev.ProcessingStatus, "financial_status": rev.FinancialStatus,
		"requested_by_type": rev.RequestedByType, "policy_id": rev.PolicyID}
	for k, v := range extra {
		fields[k] = v
	}
	_, err = events.Emit(ctx, tx, tenant, events.Event{Type: typ, AggregateID: rev.ID.String(), Key: orDefault(key, rev.ID.String()), Fields: fields})
	return err
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
