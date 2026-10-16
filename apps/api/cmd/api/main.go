// Command api runs the SMARTREVERSAL HTTP API.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"

	"github.com/smartreversal/smartreversal/apps/api/internal/alertparse"
	"github.com/smartreversal/smartreversal/apps/api/internal/alerts"
	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/config"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/correlate"
	"github.com/smartreversal/smartreversal/apps/api/internal/creport"
	"github.com/smartreversal/smartreversal/apps/api/internal/customer"
	"github.com/smartreversal/smartreversal/apps/api/internal/datashield"
	"github.com/smartreversal/smartreversal/apps/api/internal/deadline"
	"github.com/smartreversal/smartreversal/apps/api/internal/feed"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/incidents"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/labels"
	"github.com/smartreversal/smartreversal/apps/api/internal/observability"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/reconciliation"
	"github.com/smartreversal/smartreversal/apps/api/internal/reversal"
	"github.com/smartreversal/smartreversal/apps/api/internal/routing"
	"github.com/smartreversal/smartreversal/apps/api/internal/rules"
	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
	"github.com/smartreversal/smartreversal/apps/api/internal/server"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}

	logger := observability.NewLogger(cfg.Environment, cfg.LogLevel)
	logger.Info("api_starting", slog.String("version", version), slog.Any("config", cfg))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.APIPort))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	pool, err := db.Open(ctx, cfg.DatabaseURL(), db.Options{
		MaxConns:        int32(cfg.DBMaxConns),
		MaxConnLifetime: cfg.DBConnMaxLifetime,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	// The pipeline report reads stream backlog. The client connects lazily, so an
	// unreachable Redis does not stop the API; the report says it is unavailable.
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr(), Password: cfg.RedisPassword, DB: cfg.RedisDB})
	defer redisClient.Close()

	signer, verifier, err := identity.Load(cfg.JWTKeyID, cfg.JWTPrivateKey, cfg.JWTPublicKeys, cfg.JWTTokenTTL)
	if err != nil {
		return err
	}

	policy, err := authz.Load(ctx, pool)
	if err != nil {
		return err
	}
	appender := &audit.Appender{}

	var vault *datashield.Vault
	if cfg.AccountTokenKey != "" && cfg.AccountEncKey != "" {
		tokenizer, err := datashield.NewTokenizer(cfg.AccountTokenKey)
		if err != nil {
			return err
		}
		sealer, err := datashield.NewSealer(cfg.AccountEncKeyID, cfg.AccountEncKey)
		if err != nil {
			return err
		}
		vault = &datashield.Vault{Tokenizer: tokenizer, Sealer: sealer}
	} else {
		logger.Warn("account_vault_disabled", "reason", "ACCOUNT_TOKEN_KEY and ACCOUNT_ENC_KEY are not both set; the unmask route is not mounted")
	}

	configService := &configsvc.Service{Audit: appender}
	killService := &killswitch.Service{Audit: appender}
	txnService := &txn.Service{}
	routingService := &routing.Service{Audit: appender}
	reconService := &reconciliation.Service{Pool: pool, Audit: appender}
	caseService := &cases.Service{Audit: appender, Config: configService, Routing: routingService, OnClose: labels.OnClose}
	// Execution is simulated: the sources below are the synthetic ones. No real
	// provider adapter exists in this build.
	var customerLogin *customer.Login
	if cfg.CustomerDevLoginEnabled {
		customerLogin = &customer.Login{Provider: &customer.DevProvider{Pool: pool, Audit: appender}, Signer: signer}
	}
	reports := &creport.Service{Pool: pool, Config: configService, Cases: caseService, Deadlines: &deadline.Engine{Config: configService}, Parser: &alertparse.RuleParser{OwnNames: cfg.OwnBankNames}}
	alertService := &alerts.Service{Routing: &routing.Service{Audit: appender}, Config: configService, Audit: appender}
	incidentService := &incidents.Service{Config: configService, Alerts: alertService, Audit: appender}
	reversalService := &reversal.Service{Pool: pool, Config: configService, Kill: killService, Cases: caseService, Audit: appender, Txn: txnService,
		Policy: policy, SoD: authz.SoD{Conflicts: &authz.PGConflictChecker{Pool: pool}},
		Status: reversal.SimStatus{}, Ledger: reversal.SimLedger{}, Provider: reversal.SimProvider{}}

	handler := server.NewRouter(server.Options{
		Logger:   logger,
		Version:  version,
		Checkers: []server.Checker{pool},
		Verifier: verifier,
		Auth: &identity.Authenticator{
			Store:    &identity.PGUserStore{Pool: pool},
			Signer:   signer,
			Observer: &identity.AuditLoginObserver{Pool: pool, Appender: appender, Logger: logger},
		},
		Services:        &server.Services{Pool: pool, Policy: policy, Audit: appender, Vault: vault, Config: configService, KillSwitch: killService, Txn: txnService, Feed: &feed.Importer{Pool: pool, Txn: txnService}, Redis: redisClient, Alerts: alertService, Incidents: incidentService, CustomerLogin: customerLogin, Reports: reports, Rules: &rules.Engine{Store: &rules.Store{Audit: appender}}, Correlate: &correlate.Matcher{Txn: txnService}, Cases: caseService, Reversal: reversalService, Reconciliation: reconService, Routing: routingService},
		ServiceVerifier: &servauth.Verifier{Pool: pool},
	})
	return server.Serve(ctx, ln, handler, logger, server.Timeouts{
		Read:     cfg.ReadTimeout,
		Write:    cfg.WriteTimeout,
		Idle:     cfg.IdleTimeout,
		Shutdown: cfg.ShutdownTimeout,
	})
}
