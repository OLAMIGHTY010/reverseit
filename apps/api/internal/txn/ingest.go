package txn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/outbox"
)

// Outcomes of Ingest.
const (
	Accepted  = "accepted"
	Duplicate = "duplicate"
)

// Errors.
var (
	// ErrConflict means the source reused an event reference for different
	// content (HTTP 409). It is a data-quality problem on the source's side.
	ErrConflict = errors.New("txn: event reference already used with different content")
	// ErrCoreMismatch means the event disagrees with facts the transaction
	// already has, such as its amount (HTTP 422).
	ErrCoreMismatch = errors.New("txn: event conflicts with the transaction it refers to")
)

// Stream is the outbox stream that carries transaction updates.
const Stream = "transactions"

// Result describes what Ingest did.
type Result struct {
	Outcome   string
	EventID   uuid.UUID
	TxnID     uuid.UUID
	Lifecycle string
}

// Service ingests events.
type Service struct {
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Ingest stores one event and updates the transaction, in the caller's tenant
// transaction. The event, the projection and the outbox message commit together.
// An exact repeat is reported as a duplicate and changes nothing.
func (s *Service) Ingest(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, env Envelope) (Result, error) {
	now := s.now()
	if err := env.Validate(now); err != nil {
		return Result{}, err
	}
	hash := env.Hash()

	if res, found, err := s.existing(ctx, tx, tenant, env, hash); err != nil || found {
		return res, err
	}

	c := env.Txn
	// Create the transaction if this is its first event. Two events racing to
	// create it both end up on the same row.
	if _, err := tx.Exec(ctx, `
		INSERT INTO transactions (tenant_id, instruction_ref, channel, rail, amount_minor, currency, debit_account_token,
			beneficiary_bank, beneficiary_token, session_id, lifecycle_state, debit_status, credit_status, settlement_status,
			first_event_at, last_event_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'INITIATED','NONE','NONE','NONE',$11,$11)
		ON CONFLICT (tenant_id, instruction_ref) DO NOTHING`,
		tenant, c.InstructionRef, c.Channel, c.Rail, int64(c.AmountMinor), c.Currency, c.DebitAccountToken,
		c.BeneficiaryBank, c.BeneficiaryToken, c.SessionID, env.SourceTS.UTC()); err != nil {
		return Result{}, fmt.Errorf("txn: create transaction: %w", err)
	}

	var id uuid.UUID
	var have Core
	var amount int64
	err := tx.QueryRow(ctx, `SELECT id, channel, rail, amount_minor, currency, debit_account_token, beneficiary_bank, beneficiary_token, session_id
		FROM transactions WHERE tenant_id = $1 AND instruction_ref = $2 FOR UPDATE`, tenant, c.InstructionRef).
		Scan(&id, &have.Channel, &have.Rail, &amount, &have.Currency, &have.DebitAccountToken, &have.BeneficiaryBank, &have.BeneficiaryToken, &have.SessionID)
	if err != nil {
		return Result{}, fmt.Errorf("txn: lock transaction: %w", err)
	}
	// Another delivery of this same event may have committed while this one waited
	// for the transaction's lock, so look again now that the lock is held.
	if res, found, err := s.existing(ctx, tx, tenant, env, hash); err != nil || found {
		return res, err
	}
	have.AmountMinor = Amount(amount)
	if have.Rail != c.Rail || have.AmountMinor != c.AmountMinor || have.Currency != c.Currency ||
		have.DebitAccountToken != c.DebitAccountToken {
		return Result{}, fmt.Errorf("%w: rail, amount, currency and debit account are fixed by the first event", ErrCoreMismatch)
	}
	// Later events may supply details the first one lacked.
	if have.BeneficiaryToken == "" && c.BeneficiaryToken != "" || have.BeneficiaryBank == "" && c.BeneficiaryBank != "" || have.SessionID == "" && c.SessionID != "" {
		if _, err := tx.Exec(ctx, `UPDATE transactions SET
				beneficiary_token = CASE WHEN beneficiary_token = '' THEN $3 ELSE beneficiary_token END,
				beneficiary_bank  = CASE WHEN beneficiary_bank  = '' THEN $4 ELSE beneficiary_bank END,
				session_id        = CASE WHEN session_id        = '' THEN $5 ELSE session_id END
			WHERE tenant_id = $1 AND id = $2`, tenant, id, c.BeneficiaryToken, c.BeneficiaryBank, c.SessionID); err != nil {
			return Result{}, err
		}
	}

	payload, _ := json.Marshal(env)
	eventID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO transaction_events (tenant_id, id, txn_id, source_system, source_event_ref, event_type, source_ts, received_at, payload_hash, payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		tenant, eventID, id, env.SourceSystem, env.SourceEventRef, env.EventType, env.SourceTS.UTC(), now, hash, payload); err != nil {
		return Result{}, fmt.Errorf("txn: store event: %w", err)
	}

	if err := storeRefs(ctx, tx, tenant, id, env); err != nil {
		return Result{}, err
	}

	p, err := s.reproject(ctx, tx, tenant, id)
	if err != nil {
		return Result{}, err
	}

	if _, err := outbox.Enqueue(ctx, tx, outbox.Event{
		TenantID: tenant, Stream: Stream, Type: "transaction.updated", AggregateID: id.String(),
		Payload: map[string]any{"txn_id": id.String(), "instruction_ref": c.InstructionRef, "event_type": env.EventType, "lifecycle_state": p.Lifecycle},
	}); err != nil {
		return Result{}, err
	}
	return Result{Outcome: Accepted, EventID: eventID, TxnID: id, Lifecycle: p.Lifecycle}, nil
}

// existing reports whether the source's reference is already stored. The same
// content is a duplicate; different content is a conflict.
func (s *Service) existing(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, env Envelope, hash string) (Result, bool, error) {
	var existingID, existingTxn uuid.UUID
	var existingHash string
	err := tx.QueryRow(ctx, `SELECT id, txn_id, payload_hash FROM transaction_events
		WHERE tenant_id = $1 AND source_system = $2 AND source_event_ref = $3`,
		tenant, env.SourceSystem, env.SourceEventRef).Scan(&existingID, &existingTxn, &existingHash)
	switch {
	case err == nil && existingHash == hash:
		state, serr := s.lifecycle(ctx, tx, tenant, existingTxn)
		return Result{Outcome: Duplicate, EventID: existingID, TxnID: existingTxn, Lifecycle: state}, true, serr
	case err == nil:
		return Result{}, true, ErrConflict
	case !errors.Is(err, pgx.ErrNoRows):
		return Result{}, true, fmt.Errorf("txn: look up event: %w", err)
	}
	return Result{}, false, nil
}

func (s *Service) lifecycle(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (string, error) {
	var st string
	err := tx.QueryRow(ctx, `SELECT lifecycle_state FROM transactions WHERE tenant_id = $1 AND id = $2`, tenant, id).Scan(&st)
	return st, err
}

// reproject recomputes a transaction from all of its events and stores it.
func (s *Service) reproject(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (Projection, error) {
	rows, err := tx.Query(ctx, `SELECT event_type, source_ts FROM transaction_events WHERE tenant_id = $1 AND txn_id = $2`, tenant, id)
	if err != nil {
		return Projection{}, fmt.Errorf("txn: read events: %w", err)
	}
	var seen []Seen
	for rows.Next() {
		var e Seen
		if err := rows.Scan(&e.Type, &e.At); err != nil {
			rows.Close()
			return Projection{}, err
		}
		seen = append(seen, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Projection{}, err
	}
	p := Project(seen)
	if _, err := tx.Exec(ctx, `UPDATE transactions SET lifecycle_state = $3, debit_status = $4, credit_status = $5, settlement_status = $6,
			reversal_status = $7, first_debit_at = $8, first_event_at = $9, last_event_at = $10, event_count = $11, updated_at = now()
		WHERE tenant_id = $1 AND id = $2`,
		tenant, id, p.Lifecycle, p.DebitStatus, p.CreditStatus, p.SettlementState, p.ReversalStatus, p.FirstDebitAt,
		p.FirstEventAt, p.LastEventAt, p.EventCount); err != nil {
		return Projection{}, fmt.Errorf("txn: update projection: %w", err)
	}
	return p, nil
}

// Quarantine records an event that failed validation so it can be inspected.
func (s *Service) Quarantine(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, sourceSystem, reason, payloadHash string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	if len(sourceSystem) > 40 {
		sourceSystem = sourceSystem[:40]
	}
	_, err := tx.Exec(ctx, `INSERT INTO quarantined_events (tenant_id, source_system, reason, payload_hash) VALUES ($1,$2,$3,$4)`,
		tenant, sourceSystem, reason, payloadHash)
	return err
}

// storeRefs records the external references an event carries, and the session id,
// against its transaction. A reference seen before is left as it is.
func storeRefs(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, env Envelope) error {
	refs := map[string]string{}
	for k, v := range env.Refs {
		refs[k] = v
	}
	if env.Txn.SessionID != "" {
		refs["SESSION_ID"] = env.Txn.SessionID
	}
	for k, v := range refs {
		if _, err := tx.Exec(ctx, `INSERT INTO transaction_refs (tenant_id, txn_id, ref_type, ref_value, source_system) VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT DO NOTHING`, tenant, id, k, v, env.SourceSystem); err != nil {
			return fmt.Errorf("txn: store reference: %w", err)
		}
	}
	return nil
}
