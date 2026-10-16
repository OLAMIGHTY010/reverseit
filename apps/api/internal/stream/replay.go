package stream

import (
	"context"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// ReplayDeadLetters moves up to limit entries from the dead-letter stream of
// stream back onto stream, without the dlq_ annotations, and removes them from
// the dead-letter stream. It returns how many were moved.
//
// An entry is written back before it is deleted, so a crash in between can
// only cause a duplicate. Consumers dedupe on event_id, so that is safe. Fix
// the cause of the failure before replaying, or the entries will return here.
func ReplayDeadLetters(ctx context.Context, rc *redis.Client, stream string, limit int64) (int, error) {
	if limit < 1 {
		limit = 100
	}
	dlq := DeadLetterStream(stream)
	entries, err := rc.XRangeN(ctx, dlq, "-", "+", limit).Result()
	if err != nil {
		return 0, fmt.Errorf("stream: read dead letters: %w", err)
	}

	moved := 0
	for _, e := range entries {
		values := make(map[string]any, len(e.Values))
		for k, v := range e.Values {
			if !strings.HasPrefix(k, "dlq_") {
				values[k] = v
			}
		}
		if err := rc.XAdd(ctx, &redis.XAddArgs{Stream: stream, Values: values}).Err(); err != nil {
			return moved, fmt.Errorf("stream: replay write: %w", err)
		}
		if err := rc.XDel(ctx, dlq, e.ID).Err(); err != nil {
			return moved, fmt.Errorf("stream: replay delete: %w", err)
		}
		moved++
	}
	return moved, nil
}
