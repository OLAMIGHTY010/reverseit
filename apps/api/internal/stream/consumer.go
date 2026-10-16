// Package stream consumes Redis Streams with exactly-once effect.
//
// Redis delivers at least once. The consumer turns that into an idempotent
// effect by recording the event id in PostgreSQL in the same transaction as the
// handler's writes. A message is acknowledged only after that transaction
// commits. Messages that keep failing move to a dead-letter stream, and
// messages left behind by a crashed consumer are claimed by a live one.
package stream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

const (
	defaultBatch         = 50
	defaultBlock         = time.Second
	defaultClaimIdle     = 30 * time.Second
	defaultMaxDeliveries = 5
	maxErrorLen          = 200
)

// Message is one decoded stream entry.
type Message struct {
	// ID is the Redis stream entry id.
	ID          string
	Stream      string
	TenantID    uuid.UUID
	EventID     uuid.UUID
	Type        string
	AggregateID string
	TraceID     string
	Payload     []byte
	// Deliveries counts how many times Redis has delivered this entry.
	Deliveries int64
}

// Handler performs the consumer's work using tx. Returning an error rolls tx
// back and leaves the message to be retried. The handler must not commit or
// roll back tx itself.
type Handler func(ctx context.Context, tx pgx.Tx, m Message) error

// Consumer reads one stream as one member of a consumer group.
type Consumer struct {
	Pool   *db.Pool
	Redis  *redis.Client
	Logger *slog.Logger

	Stream string
	// Group is the consumer group, also used as the idempotency scope in
	// processed_events. All members of one group share it.
	Group string
	// Name identifies this member within the group.
	Name    string
	Handler Handler

	// BatchSize is the most entries fetched per read. Default 50.
	BatchSize int64
	// Block is how long a read waits for new entries. Default 1s.
	Block time.Duration
	// ClaimIdle is how long an entry must sit unacknowledged before this
	// consumer claims it. It is also the retry delay after a handler error.
	// Default 30s.
	ClaimIdle time.Duration
	// MaxDeliveries is the delivery count after which an entry is dead-lettered
	// instead of retried. Default 5.
	MaxDeliveries int64
}

// DeadLetterStream returns the name of the dead-letter stream for stream.
func DeadLetterStream(stream string) string { return stream + ".dlq" }

func (c *Consumer) batch() int64 {
	if c.BatchSize > 0 {
		return c.BatchSize
	}
	return defaultBatch
}

func (c *Consumer) block() time.Duration {
	if c.Block > 0 {
		return c.Block
	}
	return defaultBlock
}

func (c *Consumer) claimIdle() time.Duration {
	if c.ClaimIdle > 0 {
		return c.ClaimIdle
	}
	return defaultClaimIdle
}

func (c *Consumer) maxDeliveries() int64 {
	if c.MaxDeliveries > 0 {
		return c.MaxDeliveries
	}
	return defaultMaxDeliveries
}

// Setup creates the consumer group, and the stream if it does not exist.
func (c *Consumer) Setup(ctx context.Context) error {
	if c.Stream == "" || c.Group == "" || c.Name == "" || c.Handler == nil {
		return errors.New("stream: Stream, Group, Name and Handler are required")
	}
	err := c.Redis.XGroupCreateMkStream(ctx, c.Stream, c.Group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("stream: create group: %w", err)
	}
	return nil
}

// Run processes entries until ctx is cancelled. Errors from Redis are logged
// and retried after a short pause so an outage does not stop the process.
func (c *Consumer) Run(ctx context.Context) error {
	if err := c.Setup(ctx); err != nil {
		return err
	}
	for ctx.Err() == nil {
		if _, err := c.RunOnce(ctx); err != nil && ctx.Err() == nil {
			c.Logger.Error("stream_consumer_error", slog.String("stream", c.Stream), slog.Any("error", err))
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
	}
	return nil
}

// RunOnce claims stale entries, reads new ones, and processes them. It returns
// how many entries it handled (including duplicates and dead-lettered ones).
func (c *Consumer) RunOnce(ctx context.Context) (int, error) {
	handled := 0

	claimed, _, err := c.Redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   c.Stream,
		Group:    c.Group,
		Consumer: c.Name,
		MinIdle:  c.claimIdle(),
		Start:    "0-0",
		Count:    c.batch(),
	}).Result()
	if err != nil {
		return handled, fmt.Errorf("stream: claim: %w", err)
	}
	for _, m := range claimed {
		n, err := c.deliveryCount(ctx, m.ID)
		if err != nil {
			return handled, err
		}
		if err := c.process(ctx, m, n); err != nil {
			return handled, err
		}
		handled++
	}

	res, err := c.Redis.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.Group,
		Consumer: c.Name,
		Streams:  []string{c.Stream, ">"},
		Count:    c.batch(),
		Block:    c.block(),
	}).Result()
	if errors.Is(err, redis.Nil) {
		return handled, nil
	}
	if err != nil {
		return handled, fmt.Errorf("stream: read: %w", err)
	}
	for _, s := range res {
		for _, m := range s.Messages {
			if err := c.process(ctx, m, 1); err != nil {
				return handled, err
			}
			handled++
		}
	}
	return handled, nil
}

func (c *Consumer) deliveryCount(ctx context.Context, id string) (int64, error) {
	p, err := c.Redis.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: c.Stream, Group: c.Group, Start: id, End: id, Count: 1,
	}).Result()
	if err != nil {
		return 0, fmt.Errorf("stream: pending info: %w", err)
	}
	if len(p) == 0 {
		return 1, nil
	}
	return p[0].RetryCount, nil
}

// process handles one entry. It returns an error only for infrastructure
// problems that should stop this pass. Handler failures are logged and left
// for redelivery.
func (c *Consumer) process(ctx context.Context, raw redis.XMessage, deliveries int64) error {
	msg, err := decode(c.Stream, raw, deliveries)
	if err != nil {
		return c.deadLetter(ctx, raw, deliveries, "malformed message: "+err.Error())
	}

	if deliveries > c.maxDeliveries() {
		return c.deadLetter(ctx, raw, deliveries, "exceeded maximum deliveries")
	}

	duplicate := false
	herr := c.Pool.WithTenant(ctx, msg.TenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`INSERT INTO processed_events (tenant_id, consumer, event_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			msg.TenantID, c.Group, msg.EventID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			duplicate = true
			return nil
		}
		return c.Handler(ctx, tx, msg)
	})
	if herr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.Logger.Warn("stream_handler_failed",
			slog.String("stream", c.Stream),
			slog.String("event_id", msg.EventID.String()),
			slog.Int64("deliveries", deliveries),
			slog.String("trace_id", msg.TraceID),
			slog.Any("error", herr),
		)
		return nil // not acknowledged: redelivered after ClaimIdle
	}
	if duplicate {
		c.Logger.Debug("stream_duplicate_skipped", slog.String("event_id", msg.EventID.String()))
	}
	if err := c.Redis.XAck(ctx, c.Stream, c.Group, raw.ID).Err(); err != nil {
		return fmt.Errorf("stream: ack: %w", err)
	}
	return nil
}

// deadLetter copies the entry to the dead-letter stream with the reason, then
// acknowledges the original so it stops being redelivered.
func (c *Consumer) deadLetter(ctx context.Context, raw redis.XMessage, deliveries int64, reason string) error {
	values := map[string]any{}
	for k, v := range raw.Values {
		values[k] = v
	}
	values["dlq_reason"] = truncate(reason, maxErrorLen)
	values["dlq_original_id"] = raw.ID
	values["dlq_source_stream"] = c.Stream
	values["dlq_group"] = c.Group
	values["dlq_deliveries"] = strconv.FormatInt(deliveries, 10)
	values["dlq_at"] = time.Now().UTC().Format(time.RFC3339Nano)

	if err := c.Redis.XAdd(ctx, &redis.XAddArgs{Stream: DeadLetterStream(c.Stream), Values: values}).Err(); err != nil {
		return fmt.Errorf("stream: dead-letter write: %w", err)
	}
	if err := c.Redis.XAck(ctx, c.Stream, c.Group, raw.ID).Err(); err != nil {
		return fmt.Errorf("stream: ack after dead-letter: %w", err)
	}
	c.Logger.Error("stream_dead_lettered",
		slog.String("stream", c.Stream), slog.String("entry_id", raw.ID), slog.String("reason", reason))
	return nil
}

func decode(stream string, raw redis.XMessage, deliveries int64) (Message, error) {
	get := func(k string) (string, bool) {
		s, ok := raw.Values[k].(string)
		return s, ok && s != ""
	}
	tenantStr, ok := get("tenant_id")
	if !ok {
		return Message{}, errors.New("tenant_id missing")
	}
	tenant, err := uuid.Parse(tenantStr)
	if err != nil || tenant == uuid.Nil {
		return Message{}, errors.New("tenant_id is not a UUID")
	}
	idStr, ok := get("event_id")
	if !ok {
		return Message{}, errors.New("event_id missing")
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return Message{}, errors.New("event_id is not a UUID")
	}
	typ, ok := get("type")
	if !ok {
		return Message{}, errors.New("type missing")
	}
	payload, ok := get("payload")
	if !ok {
		return Message{}, errors.New("payload missing")
	}
	agg, _ := get("aggregate_id")
	trace, _ := get("trace_id")
	return Message{
		ID: raw.ID, Stream: stream, TenantID: tenant, EventID: id, Type: typ, AggregateID: agg,
		TraceID: trace, Payload: []byte(payload), Deliveries: deliveries,
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
