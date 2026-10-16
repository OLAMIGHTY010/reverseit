// Command worker runs the SMARTREVERSAL background processes.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/config"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/correlate"
	"github.com/smartreversal/smartreversal/apps/api/internal/detect"
	"github.com/smartreversal/smartreversal/apps/api/internal/inbound"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/observability"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/redisx"
	"github.com/smartreversal/smartreversal/apps/api/internal/reversal"
	"github.com/smartreversal/smartreversal/apps/api/internal/rules"
	"github.com/smartreversal/smartreversal/apps/api/internal/server"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
	"github.com/smartreversal/smartreversal/apps/api/internal/worker"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	logger := observability.NewLogger(cfg.Environment, cfg.LogLevel)
	logger.Info("worker_starting", slog.String("version", version), slog.Any("config", cfg))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL(), db.Options{
		MaxConns:        int32(cfg.DBMaxConns),
		MaxConnLifetime: cfg.DBConnMaxLifetime,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	rc, err := redisx.Open(ctx, redisx.Options{Addr: cfg.RedisAddr(), Password: cfg.RedisPassword, DB: cfg.RedisDB})
	if err != nil {
		return err
	}
	defer rc.Close()

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.WorkerHealthPort))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	app := &audit.Appender{}
	cfgSvc := &configsvc.Service{Audit: app}
	killSvc := &killswitch.Service{Audit: app}
	detector := &detect.Detector{Cases: &cases.Service{Audit: app, Config: cfgSvc}, Config: cfgSvc, Kill: killSvc,
		Rules: &rules.Engine{Store: &rules.Store{Audit: app}}}

	caseSvc := &cases.Service{Audit: app, Config: cfgSvc}
	policy, err := authz.Load(ctx, pool)
	if err != nil {
		return fmt.Errorf("load permissions: %w", err)
	}
	// Execution is simulated: the sources below are the synthetic ones.
	txnSvc := &txn.Service{}
	revSvc := &reversal.Service{Pool: pool, Config: cfgSvc, Kill: killSvc, Cases: caseSvc, Audit: app, Txn: txnSvc,
		Policy: policy, SoD: authz.SoD{Conflicts: &authz.PGConflictChecker{Pool: pool}},
		Status: reversal.SimStatus{}, Ledger: reversal.SimLedger{}, Provider: reversal.SimProvider{}}

	return worker.Run(ctx, worker.Deps{
		Detector:        detector,
		Inbound:         &inbound.Handler{Txn: txnSvc, Correlate: &correlate.Matcher{Txn: txnSvc}},
		Reversal:        revSvc,
		Pool:            pool,
		Redis:           rc,
		Logger:          logger,
		HealthListener:  ln,
		Version:         version,
		OutboxInterval:  cfg.OutboxPollInterval,
		OutboxBatchSize: cfg.OutboxBatchSize,
		Timeouts: server.Timeouts{
			Read:     cfg.ReadTimeout,
			Write:    cfg.WriteTimeout,
			Idle:     cfg.IdleTimeout,
			Shutdown: cfg.ShutdownTimeout,
		},
	})
}
