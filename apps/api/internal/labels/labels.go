// Package labels keeps labelled examples that come from cases the detector missed.
//
// When operations close a case a customer reported on a transaction no detector had
// raised, that is a lesson: the evidence looked like a problem to the customer and to
// the person who resolved it. The example is recorded as pending. Only an example a
// person has confirmed, and who is not the person who closed the case, may be used for
// training or rule tuning. Every decision is a new version; earlier ones are kept as
// superseded, so the history of what was believed is never lost.
package labels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
)

// Errors.
var (
	ErrNotFound = errors.New("labels: no such example")
	ErrState    = errors.New("labels: the example is not pending")
	ErrFourEyes = errors.New("labels: whoever closed the case may not confirm its label")
)

// Example is a labelled example.
type Example struct {
	ID        uuid.UUID       `json:"id"`
	CaseID    uuid.UUID       `json:"case_id"`
	TxnID     uuid.UUID       `json:"transaction_id"`
	Version   int             `json:"version"`
	Label     json.RawMessage `json:"label"`
	Status    string          `json:"status"`
	DecidedAt *time.Time      `json:"decided_at,omitempty"`
	Note      string          `json:"note,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// OnClose is a cases.Service close hook. It records a pending example when a case that
// a customer reported, on a transaction with no detector-raised case, is closed. By is
// who closed it, kept so the same person cannot confirm the label.
func OnClose(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, c cases.Case, by uuid.UUID) error {
	if c.Source != cases.SourceCustomer || c.TxnID == nil {
		return nil
	}
	var bot int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM cases WHERE tenant_id = $1 AND txn_id = $2 AND source = 'bot'`, tenant, *c.TxnID).Scan(&bot); err != nil {
		return err
	}
	if bot > 0 {
		return nil // the detector had it
	}
	label, _ := json.Marshal(map[string]any{"classification": c.Classification, "root_cause": c.RootCause, "closed_status": c.Status, "closed_reason": c.ClosedReason,
		"kind": "missed_by_detector", "closed_by": by.String()})
	_, err := tx.Exec(ctx, `INSERT INTO labeled_examples (tenant_id, case_id, txn_id, label) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, tenant, c.ID, *c.TxnID, label)
	return err
}

// List returns examples of a status (pending when empty), newest first.
func List(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, status string) ([]Example, error) {
	if status == "" {
		status = "pending"
	}
	rows, err := tx.Query(ctx, `SELECT id, case_id, txn_id, version, label, status, decided_at, note, created_at FROM labeled_examples
		WHERE tenant_id = $1 AND status = $2 ORDER BY created_at DESC, id LIMIT 200`, tenant, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Example{}
	for rows.Next() {
		var e Example
		if err := rows.Scan(&e.ID, &e.CaseID, &e.TxnID, &e.Version, &e.Label, &e.Status, &e.DecidedAt, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Decide confirms or rejects a pending example as a new version; the pending one is
// kept as superseded. Whoever closed the case may not confirm its label.
func Decide(ctx context.Context, tx pgx.Tx, tenant, by, id uuid.UUID, confirm bool, note string, now time.Time) (Example, error) {
	var e Example
	err := tx.QueryRow(ctx, `SELECT id, case_id, txn_id, version, label, status, decided_at, note, created_at FROM labeled_examples WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenant, id).
		Scan(&e.ID, &e.CaseID, &e.TxnID, &e.Version, &e.Label, &e.Status, &e.DecidedAt, &e.Note, &e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	if err != nil {
		return e, err
	}
	if e.Status != "pending" {
		return e, ErrState
	}
	var meta struct {
		ClosedBy string `json:"closed_by"`
	}
	_ = json.Unmarshal(e.Label, &meta)
	if confirm && meta.ClosedBy == by.String() {
		return e, ErrFourEyes
	}
	if !confirm && strings.TrimSpace(note) == "" {
		return e, fmt.Errorf("labels: a note is required to reject")
	}
	status := "confirmed"
	if !confirm {
		status = "rejected"
	}
	if _, err := tx.Exec(ctx, `UPDATE labeled_examples SET status = 'superseded' WHERE tenant_id = $1 AND id = $2`, tenant, id); err != nil {
		return e, err
	}
	var next Example
	err = tx.QueryRow(ctx, `INSERT INTO labeled_examples (tenant_id, case_id, txn_id, version, label, status, decided_by, decided_at, note)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, case_id, txn_id, version, label, status, decided_at, note, created_at`,
		tenant, e.CaseID, e.TxnID, e.Version+1, e.Label, status, by, now, strings.TrimSpace(note)).
		Scan(&next.ID, &next.CaseID, &next.TxnID, &next.Version, &next.Label, &next.Status, &next.DecidedAt, &next.Note, &next.CreatedAt)
	return next, err
}

// Confirmed returns the examples that may be used for training or rule tuning: the
// confirmed ones, and no others.
func Confirmed(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]Example, error) {
	return List(ctx, tx, tenant, "confirmed")
}
