package txn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Errors from replay.
var (
	ErrQuarantineNotFound = errors.New("txn: no such quarantined event")
	ErrNotOpen            = errors.New("txn: the quarantined event was already resolved")
	ErrNotReplayable      = errors.New("txn: the quarantined event kept no payload to replay")
)

// QuarantineEnvelope records an event that was refused, keeping the envelope so it
// can be replayed once the cause is fixed. The payload holds only what the
// envelope holds: account tokens, never account numbers.
func (s *Service) QuarantineEnvelope(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, env Envelope, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	source := env.SourceSystem
	if len(source) > 40 {
		source = source[:40]
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO quarantined_events (tenant_id, source_system, reason, payload_hash, payload) VALUES ($1,$2,$3,$4,$5)`,
		tenant, source, reason, env.Hash(), payload)
	return err
}

// Quarantined is one refused event.
type Quarantined struct {
	ID          uuid.UUID  `json:"id"`
	Source      string     `json:"source_system"`
	Reason      string     `json:"reason"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	Replayable  bool       `json:"replayable"`
	CreatedAt   time.Time  `json:"created_at"`
	LastAttempt *time.Time `json:"last_attempt_at,omitempty"`
	LastReason  string     `json:"last_reason,omitempty"`
}

// ListQuarantined returns events of one status (open when empty), oldest first.
func ListQuarantined(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, status string, limit int) ([]Quarantined, error) {
	if status == "" {
		status = "open"
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := tx.Query(ctx, `SELECT id, source_system, reason, status, attempts, payload IS NOT NULL, created_at, last_attempt_at, last_reason
		FROM quarantined_events WHERE tenant_id = $1 AND status = $2 ORDER BY created_at, id LIMIT $3`, tenant, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Quarantined{}
	for rows.Next() {
		var q Quarantined
		if err := rows.Scan(&q.ID, &q.Source, &q.Reason, &q.Status, &q.Attempts, &q.Replayable, &q.CreatedAt, &q.LastAttempt, &q.LastReason); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// ReplayResult says what a replay did.
type ReplayResult struct {
	ID       uuid.UUID `json:"id"`
	Replayed bool      `json:"replayed"`
	Outcome  string    `json:"outcome"`
	Reason   string    `json:"reason,omitempty"`
	TxnID    uuid.UUID `json:"txn_id,omitempty"`
}

// Replay offers a quarantined event to ingestion again, in the caller's tenant
// transaction. It goes through exactly the same checks as a new event. If it is
// accepted, or turns out to repeat an event already stored, the quarantine row is
// marked replayed. If it is refused again the row stays open with the new reason and
// nothing is stored. The row is locked, so two replays of one event cannot both run.
func (s *Service) Replay(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (ReplayResult, error) {
	var status string
	var payload []byte
	err := tx.QueryRow(ctx, `SELECT status, payload FROM quarantined_events WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenant, id).Scan(&status, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReplayResult{}, ErrQuarantineNotFound
	}
	if err != nil {
		return ReplayResult{}, err
	}
	if status != "open" {
		return ReplayResult{}, ErrNotOpen
	}
	if payload == nil {
		return ReplayResult{}, ErrNotReplayable
	}
	var env Envelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return ReplayResult{}, fmt.Errorf("%w: %v", ErrNotReplayable, err)
	}

	// A refused attempt must leave nothing behind, so it runs in a savepoint.
	sp, err := tx.Begin(ctx)
	if err != nil {
		return ReplayResult{}, err
	}
	res, ierr := s.Ingest(ctx, sp, tenant, env)
	if ierr != nil && (errors.Is(ierr, ErrInvalid) || errors.Is(ierr, ErrCoreMismatch) || errors.Is(ierr, ErrConflict)) {
		if err := sp.Rollback(ctx); err != nil {
			return ReplayResult{}, err
		}
		reason := ierr.Error()
		if len(reason) > 500 {
			reason = reason[:500]
		}
		if _, err := tx.Exec(ctx, `UPDATE quarantined_events SET attempts = attempts + 1, last_attempt_at = now(), last_reason = $3
			WHERE tenant_id = $1 AND id = $2`, tenant, id, reason); err != nil {
			return ReplayResult{}, err
		}
		return ReplayResult{ID: id, Outcome: "refused_again", Reason: reason}, nil
	}
	if ierr != nil {
		_ = sp.Rollback(ctx)
		return ReplayResult{}, ierr
	}
	if err := sp.Commit(ctx); err != nil {
		return ReplayResult{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE quarantined_events SET status = 'replayed', attempts = attempts + 1, last_attempt_at = now(), resolved_at = now(), last_reason = ''
		WHERE tenant_id = $1 AND id = $2`, tenant, id); err != nil {
		return ReplayResult{}, err
	}
	return ReplayResult{ID: id, Replayed: true, Outcome: res.Outcome, TxnID: res.TxnID}, nil
}

// Discard marks an open quarantined event as not to be replayed, with the reason
// kept on the row. The event itself is never deleted.
func Discard(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID, reason string) error {
	tag, err := tx.Exec(ctx, `UPDATE quarantined_events SET status = 'discarded', resolved_at = now(), last_reason = $3
		WHERE tenant_id = $1 AND id = $2 AND status = 'open'`, tenant, id, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM quarantined_events WHERE tenant_id = $1 AND id = $2`, tenant, id).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ErrQuarantineNotFound
		}
		return ErrNotOpen
	}
	return nil
}
