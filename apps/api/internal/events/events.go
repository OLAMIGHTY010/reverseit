// Package events defines the domain events the platform publishes about cases,
// reversals and notifications, and the one safe way to emit them.
//
// An event is written to the transactional outbox in the same database
// transaction as the change it describes, so a change that rolls back never
// publishes, and a change that commits always does. The relay then delivers it
// to Redis Streams at least once. Delivery is not exactly once: consumers must
// dedupe on event_id, which is stable. Emitting the same event twice (the same
// type, tenant and dedupe key) stores it once.
//
// Payloads carry identifiers and non-sensitive metadata only. Each event type
// lists the fields it may carry, and Emit refuses any other field.
package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/outbox"
)

// SchemaVersion is carried in every payload. Increase it when a field changes
// meaning; adding an optional field does not need a new version.
const SchemaVersion = 1

// Streams the events are published to.
const (
	StreamCases         = "cases"
	StreamReversals     = "reversals"
	StreamNotifications = "notifications"
)

// Event types.
const (
	CaseOpened           = "case.opened"
	CaseTransitioned     = "case.transitioned"
	CaseSLABreached      = "case.sla_breached"
	CaseEscalated        = "case.escalated"
	ReversalRequested    = "reversal.requested"
	ReversalApproved     = "reversal.approved"
	ReversalSubmitted    = "reversal.submitted"
	ReversalVerified     = "reversal.verified"
	ReversalFailed       = "reversal.failed"
	NotificationRecorded = "notification.recorded"
)

type definition struct {
	stream string
	fields []string
}

var reversalFields = []string{"reversal_id", "reversal_ref", "case_id", "txn_id", "amount_minor", "currency", "approval_status",
	"accepted_status", "processing_status", "financial_status", "requested_by_type", "policy_id"}

// registry is the complete list of events and the fields each may carry.
var registry = map[string]definition{
	CaseOpened:           {StreamCases, []string{"case_id", "case_ref", "classification", "source", "priority", "status", "txn_id"}},
	CaseTransitioned:     {StreamCases, []string{"case_id", "case_ref", "classification", "from_status", "to_status", "version"}},
	CaseSLABreached:      {StreamCases, []string{"case_id", "case_ref", "classification", "priority", "sla_due_at", "breached_at"}},
	CaseEscalated:        {StreamCases, []string{"case_id", "case_ref", "escalated_to_role", "escalated_to_user", "reason"}},
	ReversalRequested:    {StreamReversals, reversalFields},
	ReversalApproved:     {StreamReversals, reversalFields},
	ReversalSubmitted:    {StreamReversals, reversalFields},
	ReversalVerified:     {StreamReversals, reversalFields},
	ReversalFailed:       {StreamReversals, append([]string{"failure_code"}, reversalFields...)},
	NotificationRecorded: {StreamNotifications, []string{"notification_id", "case_id", "audience", "stage", "template_id"}},
}

// Types lists every defined event type.
func Types() []string {
	out := make([]string, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	return out
}

// Fields returns the fields an event type may carry, or nil if it is unknown.
func Fields(eventType string) []string {
	d, ok := registry[eventType]
	if !ok {
		return nil
	}
	return append([]string(nil), d.fields...)
}

// Event is one domain event to emit.
type Event struct {
	Type string
	// AggregateID is the case or reversal the event is about. Events for one
	// aggregate are published in order.
	AggregateID string
	// Key identifies this occurrence. Emitting the same type and key again for a
	// tenant stores nothing, which is how repeated requests avoid repeated events.
	Key string
	// Fields are the payload values. Values must be strings, integers or bools.
	Fields map[string]any
	// OccurredAt defaults to the current time.
	OccurredAt time.Time
	TraceID    string
}

var namespace = uuid.MustParse("6f1d2c4e-5a1b-4c3d-9e8f-2b7a6d5c4e31")

// ID returns the stable event id for a tenant, type and key.
func ID(tenant uuid.UUID, eventType, key string) uuid.UUID {
	return uuid.NewSHA1(namespace, []byte(tenant.String()+"|"+eventType+"|"+key))
}

// ErrInvalid is returned for an event that breaks the rules.
var ErrInvalid = errors.New("events: invalid event")

func (e Event) validate() (definition, error) {
	d, ok := registry[e.Type]
	if !ok {
		return d, fmt.Errorf("%w: unknown type %q", ErrInvalid, e.Type)
	}
	if e.AggregateID == "" || e.Key == "" {
		return d, fmt.Errorf("%w: aggregate id and key are required", ErrInvalid)
	}
	allowed := map[string]bool{}
	for _, f := range d.fields {
		allowed[f] = true
	}
	for k, v := range e.Fields {
		if !allowed[k] {
			return d, fmt.Errorf("%w: %s may not carry %q", ErrInvalid, e.Type, k)
		}
		switch x := v.(type) {
		case string:
			if len(x) > 200 {
				return d, fmt.Errorf("%w: field %q is too long", ErrInvalid, k)
			}
		case int, int32, int64, bool:
		default:
			return d, fmt.Errorf("%w: field %q must be a string, integer or bool", ErrInvalid, k)
		}
	}
	return d, nil
}

// Emit writes e to the outbox using tx, the transaction that holds the change.
// It reports whether a new event was stored: false means this occurrence had
// already been emitted.
func Emit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, e Event) (bool, error) {
	d, err := e.validate()
	if err != nil {
		return false, err
	}
	at := e.OccurredAt
	if at.IsZero() {
		at = time.Now()
	}
	payload := map[string]any{"schema_version": SchemaVersion, "tenant_id": tenant.String(), "occurred_at": at.UTC().Format(time.RFC3339Nano)}
	for k, v := range e.Fields {
		payload[k] = v
	}
	return outbox.EnqueueOnce(ctx, tx, outbox.Event{TenantID: tenant, ID: ID(tenant, e.Type, e.Key), Stream: d.stream, Type: e.Type,
		AggregateID: e.AggregateID, Payload: payload, TraceID: e.TraceID})
}
