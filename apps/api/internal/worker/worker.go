// Package worker runs the background processes: the outbox relay and a small
// health endpoint for orchestrators. Stream consumers are added by the phases
// that own them.
package worker

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/detect"
	"github.com/smartreversal/smartreversal/apps/api/internal/inbound"
	"github.com/smartreversal/smartreversal/apps/api/internal/outbox"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/redisx"
	"github.com/smartreversal/smartreversal/apps/api/internal/reversal"
	"github.com/smartreversal/smartreversal/apps/api/internal/server"
	"github.com/smartreversal/smartreversal/apps/api/internal/stream"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Deps are the worker's collaborators.
type Deps struct {
	Pool   *db.Pool
	Redis  *redisx.Client
	Logger *slog.Logger
	// HealthListener serves /v1/health and /v1/ready.
	HealthListener net.Listener
	Version        string

	OutboxInterval  time.Duration
	OutboxBatchSize int
	Timeouts        server.Timeouts

	// Detector, when set, consumes the transactions stream and runs the
	// stuck-transaction sweep every SweepInterval (default 30 seconds).
	Detector      *detect.Detector
	SweepInterval time.Duration
	// Reversal, when set, makes each sweep also run the simulated reversal
	// automation and advance reversals still in flight.
	Reversal *reversal.Service
	// Inbound, when set, consumes the inbound events stream that sources write to.
	Inbound *inbound.Handler
}

// Run starts every background task and blocks until ctx is cancelled or one
// task fails. A failure of one task stops the others, so an orchestrator can
// restart the whole process rather than leave it half alive.
func Run(ctx context.Context, d Deps) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	relay := &outbox.Relay{
		Pool:      d.Pool,
		Redis:     d.Redis.Client,
		Logger:    d.Logger,
		BatchSize: d.OutboxBatchSize,
	}
	health := server.NewRouter(server.Options{
		Logger:   d.Logger,
		Version:  d.Version,
		Service:  "worker",
		Checkers: []server.Checker{d.Pool, d.Redis},
	})

	tasks := 2
	errc := make(chan error, 5)
	if d.Detector != nil {
		tasks += 2
		consumer := &stream.Consumer{Pool: d.Pool, Redis: d.Redis.Client, Logger: d.Logger,
			Stream: txn.Stream, Group: "detector", Name: "worker-1", Handler: d.Detector.Handle}
		go func() { errc <- consumer.Run(ctx) }()
		go func() { errc <- sweepLoop(ctx, d) }()
	}
	if d.Inbound != nil {
		tasks++
		consumer := &stream.Consumer{Pool: d.Pool, Redis: d.Redis.Client, Logger: d.Logger,
			Stream: inbound.Stream, Group: inbound.Group, Name: "worker-1", Handler: d.Inbound.Handle}
		go func() { errc <- consumer.Run(ctx) }()
	}
	go func() {
		errc <- server.Serve(ctx, d.HealthListener, health, d.Logger, d.Timeouts)
	}()
	go func() {
		errc <- relay.Run(ctx, d.OutboxInterval)
	}()

	var first error
	for i := 0; i < tasks; i++ {
		err := <-errc
		if err != nil && first == nil && !errors.Is(err, context.Canceled) {
			first = err
		}
		cancel() // when one task ends, stop the other
	}
	return first
}

// sweepLoop runs the stuck-transaction sweep until ctx is cancelled. A failed
// sweep is logged and tried again on the next tick.
func sweepLoop(ctx context.Context, d Deps) error {
	every := d.SweepInterval
	if every <= 0 {
		every = 30 * time.Second
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if n, err := d.Detector.SweepAll(ctx, d.Pool); err != nil && ctx.Err() == nil {
				d.Logger.Error("sweep_failed", "error", err)
			} else if n > 0 {
				d.Logger.Info("sweep_opened_cases", "count", n)
			}
			// Detection a freeze held back is picked up again as soon as it is allowed.
			if n, err := d.Detector.ResumeAll(ctx, d.Pool); err != nil && ctx.Err() == nil {
				d.Logger.Error("resume_held_failed", "error", err)
			} else if n > 0 {
				d.Logger.Info("resumed_held_detection", "count", n)
			}
			if d.Reversal != nil {
				reverseAll(ctx, d)
			}
		}
	}
}

// reverseAll runs the reversal sweep for every institution. A failure for one
// institution is logged and does not stop the others.
func reverseAll(ctx context.Context, d Deps) {
	rows, err := d.Pool.Query(ctx, `SELECT id FROM institutions ORDER BY code`)
	if err != nil {
		d.Logger.Error("reversal_sweep_failed", "error", err)
		return
	}
	var tenants []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			d.Logger.Error("reversal_sweep_failed", "error", err)
			return
		}
		tenants = append(tenants, id)
	}
	rows.Close()
	for _, t := range tenants {
		res, err := d.Reversal.Sweep(ctx, t)
		if err != nil && ctx.Err() == nil {
			d.Logger.Error("reversal_sweep_failed", "tenant", t.String(), "error", err)
			continue
		}
		if len(res) > 0 {
			d.Logger.Info("reversal_sweep", "tenant", t.String(), "handled", len(res))
		}
	}
}
