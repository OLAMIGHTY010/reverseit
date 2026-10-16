// Package pipeline reports how much work is waiting between the places evidence
// moves: the outbox in PostgreSQL, and the Redis streams consumers read.
//
// It reports measured numbers and decides nothing. What counts as too much backlog
// is for operators to set once they know their volumes, so no threshold is assumed
// here and the report carries no verdict.
package pipeline

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/inbound"
	"github.com/smartreversal/smartreversal/apps/api/internal/stream"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Outbox is the state of the institution's transactional outbox.
type Outbox struct {
	Unpublished   int     `json:"unpublished"`
	Failing       int     `json:"failing"`
	OldestAgeSecs float64 `json:"oldest_unpublished_age_secs"`
}

// Group is one consumer group on a stream.
type Group struct {
	Name      string `json:"name"`
	Consumers int64  `json:"consumers"`
	Pending   int64  `json:"pending"`
	// Lag is entries not yet delivered to the group. It is nil when Redis cannot
	// say (for example after the stream was trimmed past the group's position).
	Lag *int64 `json:"lag"`
}

// Stream is one Redis stream and its dead-letter stream.
type Stream struct {
	Name        string  `json:"name"`
	Length      int64   `json:"length"`
	DeadLetters int64   `json:"dead_letters"`
	Groups      []Group `json:"groups"`
}

// Report is the whole picture.
type Report struct {
	Outbox Outbox `json:"outbox"`
	// RedisAvailable is false when Redis was not configured or did not answer; the
	// stream list is then empty and that says nothing about the streams themselves.
	RedisAvailable bool     `json:"redis_available"`
	Streams        []Stream `json:"streams"`
}

// KnownStreams are always inspected, in addition to any stream the institution's
// outbox has published to.
var KnownStreams = []string{txn.Stream, inbound.Stream}

// Collect gathers the report. tx is a tenant transaction for the outbox, which is
// per institution. rc may be nil. extra names more streams to inspect. Streams are shared infrastructure, so their
// numbers are for the whole platform.
func Collect(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, rc *redis.Client, extra ...string) (Report, error) {
	var r Report
	if err := tx.QueryRow(ctx, `SELECT count(*)::int, count(*) FILTER (WHERE attempts > 0)::int,
			coalesce(extract(epoch FROM (now() - min(created_at))), 0)::float8
		FROM outbox_events WHERE tenant_id = $1 AND published_at IS NULL`, tenant).Scan(&r.Outbox.Unpublished, &r.Outbox.Failing, &r.Outbox.OldestAgeSecs); err != nil {
		return r, err
	}
	r.Streams = []Stream{}
	if rc == nil {
		return r, nil
	}
	names := map[string]bool{}
	for _, n := range append(append([]string{}, KnownStreams...), extra...) {
		names[n] = true
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT stream FROM outbox_events WHERE tenant_id = $1 AND created_at > now() - interval '7 days'`, tenant)
	if err != nil {
		return r, err
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return r, err
		}
		names[n] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return r, err
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	if err := rc.Ping(ctx).Err(); err != nil {
		return r, nil
	}
	r.RedisAvailable = true
	for _, n := range sorted {
		s, err := inspect(ctx, rc, n)
		if err != nil {
			r.RedisAvailable = false
			r.Streams = []Stream{}
			return r, nil
		}
		r.Streams = append(r.Streams, s)
	}
	return r, nil
}

func inspect(ctx context.Context, rc *redis.Client, name string) (Stream, error) {
	s := Stream{Name: name, Groups: []Group{}}
	n, err := rc.XLen(ctx, name).Result()
	if err != nil {
		return s, err
	}
	s.Length = n
	if d, err := rc.XLen(ctx, stream.DeadLetterStream(name)).Result(); err == nil {
		s.DeadLetters = d
	}
	groups, err := rc.XInfoGroups(ctx, name).Result()
	if err != nil {
		// A stream that does not exist yet has no groups.
		if strings.Contains(err.Error(), "no such key") || errors.Is(err, redis.Nil) {
			return s, nil
		}
		return s, err
	}
	for _, g := range groups {
		grp := Group{Name: g.Name, Consumers: g.Consumers, Pending: g.Pending}
		if g.Lag >= 0 {
			lag := g.Lag
			grp.Lag = &lag
		}
		s.Groups = append(s.Groups, grp)
	}
	sort.Slice(s.Groups, func(i, j int) bool { return s.Groups[i].Name < s.Groups[j].Name })
	return s, nil
}
