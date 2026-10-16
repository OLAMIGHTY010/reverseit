package correlate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Errors from the queue.
var (
	ErrNotFound = errors.New("correlate: no such evidence")
	ErrNotOpen  = errors.New("correlate: the evidence was already resolved")
	// ErrMismatch means a person tried to link evidence to a transaction whose amount
	// or currency differs. Such a link would hide a real disagreement between sources.
	ErrMismatch = errors.New("correlate: the evidence and the transaction disagree on amount or currency")
)

// Item is one row of the correlation record.
type Item struct {
	ID          uuid.UUID         `json:"id"`
	Source      string            `json:"source_system"`
	SourceRef   string            `json:"source_event_ref"`
	EventType   string            `json:"event_type"`
	Refs        map[string]string `json:"refs"`
	AmountMinor string            `json:"amount_minor"`
	Currency    string            `json:"currency"`
	Reason      string            `json:"reason"`
	Candidates  []Candidate       `json:"candidates"`
	Status      string            `json:"status"`
	LinkedTxn   *uuid.UUID        `json:"linked_txn_id,omitempty"`
	Method      string            `json:"link_method,omitempty"`
	Confidence  *float64          `json:"confidence,omitempty"`
	Attempts    int               `json:"attempts"`
	CreatedAt   time.Time         `json:"created_at"`
}

// List returns evidence of one status (open when empty), oldest first.
func List(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, status string, limit int) ([]Item, error) {
	if status == "" {
		status = "open"
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := tx.Query(ctx, `SELECT id, source_system, source_event_ref, observation, reason, candidates, status, linked_txn_id, link_method, confidence::float8, attempts, created_at
		FROM evidence_correlations WHERE tenant_id = $1 AND status = $2 ORDER BY created_at, id LIMIT $3`, tenant, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		var obs, cands []byte
		if err := rows.Scan(&it.ID, &it.Source, &it.SourceRef, &obs, &it.Reason, &cands, &it.Status, &it.LinkedTxn, &it.Method, &it.Confidence, &it.Attempts, &it.CreatedAt); err != nil {
			return nil, err
		}
		var o Observation
		if err := json.Unmarshal(obs, &o); err != nil {
			return nil, err
		}
		it.EventType, it.Refs, it.AmountMinor, it.Currency = o.EventType, o.Refs, o.AmountMinor.String(), o.Currency
		_ = json.Unmarshal(cands, &it.Candidates)
		out = append(out, it)
	}
	return out, rows.Err()
}

// lockOpen loads and locks an open item's observation.
func lockOpen(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (Observation, error) {
	var status string
	var obs []byte
	err := tx.QueryRow(ctx, `SELECT status, observation FROM evidence_correlations WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenant, id).Scan(&status, &obs)
	if errors.Is(err, pgx.ErrNoRows) {
		return Observation{}, ErrNotFound
	}
	if err != nil {
		return Observation{}, err
	}
	if status != "open" {
		return Observation{}, ErrNotOpen
	}
	var o Observation
	return o, json.Unmarshal(obs, &o)
}

// ManualLink attaches open evidence to a transaction a person has chosen. The
// amount and currency must agree. The link is recorded with who made it and why;
// the caller audits it in the same transaction.
func (m *Matcher) ManualLink(ctx context.Context, tx pgx.Tx, tenant, id, txnID, by uuid.UUID, reason string) (txn.Result, error) {
	o, err := lockOpen(ctx, tx, tenant, id)
	if err != nil {
		return txn.Result{}, err
	}
	var amount int64
	var currency string
	err = tx.QueryRow(ctx, `SELECT amount_minor, currency FROM transactions WHERE tenant_id = $1 AND id = $2`, tenant, txnID).Scan(&amount, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return txn.Result{}, fmt.Errorf("%w: no such transaction", ErrInvalid)
	}
	if err != nil {
		return txn.Result{}, err
	}
	if amount != int64(o.AmountMinor) || currency != o.Currency {
		return txn.Result{}, ErrMismatch
	}
	res, err := m.attach(ctx, tx, tenant, txnID, o)
	if err != nil {
		return txn.Result{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE evidence_correlations SET status = 'linked', linked_txn_id = $3, link_method = 'manual', resolved_by = $4, resolved_reason = $5, resolved_at = now()
		WHERE tenant_id = $1 AND id = $2`, tenant, id, txnID, by, reason)
	return res, err
}

// Discard closes open evidence as not belonging to any transaction. It is kept.
func Discard(ctx context.Context, tx pgx.Tx, tenant, id, by uuid.UUID, reason string) error {
	if _, err := lockOpen(ctx, tx, tenant, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE evidence_correlations SET status = 'discarded', resolved_by = $3, resolved_reason = $4, resolved_at = now()
		WHERE tenant_id = $1 AND id = $2`, tenant, id, by, reason)
	return err
}

// Retry looks at every open item again, because the transaction it belongs to may
// have arrived since. Items that now match confidently are linked automatically.
// It returns how many were linked.
func (m *Matcher) Retry(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (int, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM evidence_correlations WHERE tenant_id = $1 AND status = 'open' ORDER BY created_at, id`, tenant)
	if err != nil {
		return 0, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	linked := 0
	for _, id := range ids {
		o, err := lockOpen(ctx, tx, tenant, id)
		if errors.Is(err, ErrNotOpen) {
			continue
		}
		if err != nil {
			return linked, err
		}
		d, err := m.decide(ctx, tx, tenant, o)
		if err != nil {
			return linked, err
		}
		cands, _ := json.Marshal(d.Candidates)
		if d.Linked {
			linked++
			_, err = tx.Exec(ctx, `UPDATE evidence_correlations SET status = 'linked', linked_txn_id = $3, link_method = 'automatic', confidence = $4, candidates = $5, reason = '',
				attempts = attempts + 1, resolved_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, id, d.TxnID, d.Confidence, cands)
		} else {
			_, err = tx.Exec(ctx, `UPDATE evidence_correlations SET reason = $3, candidates = $4, attempts = attempts + 1 WHERE tenant_id = $1 AND id = $2`, tenant, id, d.Reason, cands)
		}
		if err != nil {
			return linked, err
		}
	}
	return linked, nil
}
