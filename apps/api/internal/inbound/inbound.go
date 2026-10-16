// Package inbound lets a source deliver evidence by writing to a Redis stream
// instead of calling the API. A message is a stream entry with the same fields the
// outbox relay writes (tenant_id, event_id, type, payload), so the generic consumer
// supplies idempotency, retry and dead-lettering.
//
// What the handler adds is a policy about failure. A message that is wrong
// (invalid, contradicts its transaction, reuses a reference with different content)
// can never succeed, so retrying it five times and then dead-lettering it would only
// delay the answer. It is quarantined, kept whole for review and replay, and
// acknowledged. A message that failed for an infrastructure reason is returned as an
// error and retried by the consumer.
package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/correlate"
	"github.com/smartreversal/smartreversal/apps/api/internal/stream"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Stream is where sources write. Group is the consumer group, and so the scope of
// idempotency, for this adapter.
const (
	Stream = "events.inbound"
	Group  = "inbound"
)

// Message types.
const (
	TypeEvent       = "transaction.event"
	TypeObservation = "transaction.observation"
)

// Handler ingests inbound messages.
type Handler struct {
	Txn       *txn.Service
	Correlate *correlate.Matcher
}

// Handle is a stream.Handler.
func (h *Handler) Handle(ctx context.Context, tx pgx.Tx, m stream.Message) error {
	switch m.Type {
	case TypeEvent:
		return h.event(ctx, tx, m)
	case TypeObservation:
		return h.observation(ctx, tx, m)
	}
	// An unknown type cannot succeed on retry. Keep it where people look.
	return h.Txn.Quarantine(ctx, tx, m.TenantID, "", fmt.Sprintf("inbound message of unknown type %q", truncate(m.Type, 60)), hashOf(m.Payload))
}

func (h *Handler) event(ctx context.Context, tx pgx.Tx, m stream.Message) error {
	var env txn.Envelope
	if err := strictJSON(m.Payload, &env); err != nil {
		return h.Txn.Quarantine(ctx, tx, m.TenantID, "", "malformed event JSON: "+err.Error(), hashOf(m.Payload))
	}
	// The attempt runs in a savepoint, so a refusal leaves nothing behind.
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	_, err = h.Txn.Ingest(ctx, sp, m.TenantID, env)
	if err == nil {
		return sp.Commit(ctx)
	}
	_ = sp.Rollback(ctx)
	if isRefusal(err) {
		return h.Txn.QuarantineEnvelope(ctx, tx, m.TenantID, env, err.Error())
	}
	return err
}

func (h *Handler) observation(ctx context.Context, tx pgx.Tx, m stream.Message) error {
	if h.Correlate == nil {
		return errors.New("inbound: correlation is not configured")
	}
	var o correlate.Observation
	if err := strictJSON(m.Payload, &o); err != nil {
		return h.Txn.Quarantine(ctx, tx, m.TenantID, "", "malformed observation JSON: "+err.Error(), hashOf(m.Payload))
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	_, err = h.Correlate.Resolve(ctx, sp, m.TenantID, o)
	if err == nil {
		return sp.Commit(ctx)
	}
	_ = sp.Rollback(ctx)
	if errors.Is(err, correlate.ErrInvalid) || errors.Is(err, txn.ErrConflict) {
		return h.Txn.Quarantine(ctx, tx, m.TenantID, truncate(o.SourceSystem, 40), "observation refused: "+err.Error(), hashOf(m.Payload))
	}
	return err
}

func isRefusal(err error) bool {
	return errors.Is(err, txn.ErrInvalid) || errors.Is(err, txn.ErrCoreMismatch) || errors.Is(err, txn.ErrConflict)
}

// Entry builds the stream fields a source writes for one event or observation.
func Entry(tenant uuid.UUID, typ string, payload any) (map[string]any, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return map[string]any{"tenant_id": tenant.String(), "event_id": uuid.NewString(), "type": typ, "payload": string(b)}, nil
}
