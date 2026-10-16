package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

// relayLockKey makes exactly one relay publish at a time, which keeps
// per-aggregate order. It is arbitrary but fixed.
const relayLockKey int64 = 7_351_204_119

const (
	defaultBatchSize = 100
	defaultMaxLen    = 100_000
	maxErrorLen      = 200
)

// Relay publishes unpublished outbox rows to Redis Streams.
//
// Delivery is at least once. If the process stops after a stream write and
// before the row is marked published, the event is published again on the
// next pass. Consumers must treat event_id as an idempotency key.
type Relay struct {
	Pool   *db.Pool
	Redis  *redis.Client
	Logger *slog.Logger

	// BatchSize is the most rows published per pass. Default 100.
	BatchSize int
	// MaxLen is the approximate cap on each stream's length. Default 100000.
	MaxLen int64
}

// PublishPending publishes one batch and returns how many events were
// published. It stops at the first stream error so order is preserved, records
// the failed attempt, and returns the error.
func (r *Relay) PublishPending(ctx context.Context) (int, error) {
	batch := r.BatchSize
	if batch <= 0 {
		batch = defaultBatchSize
	}
	maxLen := r.MaxLen
	if maxLen <= 0 {
		maxLen = defaultMaxLen
	}

	published := 0
	var publishErr error

	err := r.Pool.WithTx(ctx, func(tx pgx.Tx) error {
		var got bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, relayLockKey).Scan(&got); err != nil {
			return err
		}
		if !got {
			return nil // another relay is publishing
		}

		rows, err := tx.Query(ctx, `
			SELECT id, tenant_id::text, event_id::text, stream, event_type, aggregate_id, payload::text, trace_id
			FROM outbox_events
			WHERE published_at IS NULL
			ORDER BY id
			LIMIT $1
			FOR UPDATE`, batch)
		if err != nil {
			return err
		}
		type row struct {
			id                                                        int64
			tenantID, eventID, stream, typ, aggregate, payload, trace string
		}
		var pending []row
		for rows.Next() {
			var x row
			if err := rows.Scan(&x.id, &x.tenantID, &x.eventID, &x.stream, &x.typ, &x.aggregate, &x.payload, &x.trace); err != nil {
				rows.Close()
				return err
			}
			pending = append(pending, x)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		var done []int64
		for _, x := range pending {
			err := r.Redis.XAdd(ctx, &redis.XAddArgs{
				Stream: x.stream,
				MaxLen: maxLen,
				Approx: true,
				Values: map[string]any{
					"tenant_id":    x.tenantID,
					"event_id":     x.eventID,
					"type":         x.typ,
					"aggregate_id": x.aggregate,
					"payload":      x.payload,
					"trace_id":     x.trace,
				},
			}).Err()
			if err != nil {
				publishErr = fmt.Errorf("outbox: publish event %s: %w", x.eventID, err)
				if _, uerr := tx.Exec(ctx, `UPDATE outbox_events SET attempts = attempts + 1, last_error = $2 WHERE id = $1`,
					x.id, truncate(err.Error(), maxErrorLen)); uerr != nil {
					return uerr
				}
				break
			}
			done = append(done, x.id)
		}

		if len(done) > 0 {
			if _, err := tx.Exec(ctx, `UPDATE outbox_events SET published_at = now(), attempts = attempts + 1, last_error = '' WHERE id = ANY($1)`, done); err != nil {
				return err
			}
		}
		published = len(done)
		return nil
	})
	if err != nil {
		return published, err
	}
	return published, publishErr
}

// Run publishes on every tick until ctx is cancelled. Errors are logged and the
// loop continues, because a Redis outage must not stop the process.
func (r *Relay) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("outbox: relay interval must be positive")
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		n, err := r.PublishPending(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			r.Logger.Error("outbox_publish_failed", slog.Int("published", n), slog.Any("error", err))
		case n > 0:
			r.Logger.Debug("outbox_published", slog.Int("count", n))
		}
		if n == r.batchSize() && err == nil {
			continue // more may be waiting
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

func (r *Relay) batchSize() int {
	if r.BatchSize <= 0 {
		return defaultBatchSize
	}
	return r.BatchSize
}

// Requeue marks every row with an id of sinceID or higher as unpublished so the
// relay sends it again. It is the recovery path after Redis loses stream data.
// Consumers dedupe on event_id, so replays are safe.
func Requeue(ctx context.Context, pool *db.Pool, sinceID int64) (int64, error) {
	tag, err := pool.Exec(ctx, `UPDATE outbox_events SET published_at = NULL WHERE id >= $1 AND published_at IS NOT NULL`, sinceID)
	if err != nil {
		return 0, fmt.Errorf("outbox: requeue: %w", err)
	}
	return tag.RowsAffected(), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
