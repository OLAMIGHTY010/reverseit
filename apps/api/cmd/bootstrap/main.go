// Command bootstrap creates the first institution and operator of an
// environment. The password is read from BOOTSTRAP_PASSWORD, never from an
// argument, so it does not appear in the process list or shell history.
//
//	BOOTSTRAP_PASSWORD='...' bootstrap -institution DEMO-BANK -email ops@example.test -roles ADMIN_INTEGRATION
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/config"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "bootstrap:", err)
		os.Exit(1)
	}
}

func run() error {
	code := flag.String("institution", "", "institution code, capitals, digits and dashes")
	instName := flag.String("institution-name", "", "institution display name (default: the code)")
	email := flag.String("email", "", "operator email")
	name := flag.String("name", "", "operator display name (default: the email)")
	seedClocks := flag.Bool("seed-clocks", false, "propose the v2 candidate regulatory clocks as this operator; they stay pending until someone else approves them")
	roles := flag.String("roles", "", "comma-separated roles, for example ADMIN_INTEGRATION or REVERSAL_OFFICER,SUPERVISOR")
	enrolTOTP := flag.Bool("enrol-totp", false, "enrol a development authenticator for this operator and print its synthetic secret (D20)")
	flag.Parse()

	password := os.Getenv("BOOTSTRAP_PASSWORD")
	if password == "" {
		return fmt.Errorf("set BOOTSTRAP_PASSWORD")
	}
	var roleList []string
	for _, r := range strings.Split(*roles, ",") {
		if r = strings.TrimSpace(r); r != "" {
			roleList = append(roleList, r)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL(), db.Options{MaxConns: 2})
	if err != nil {
		return err
	}
	defer pool.Close()

	tenant, user, err := identity.Bootstrap(ctx, pool, identity.BootstrapSpec{
		InstitutionCode: *code, InstitutionName: *instName, Email: *email, DisplayName: *name,
		Password: password, Roles: roleList,
	})
	if err != nil {
		return err
	}
	fmt.Printf("institution %s (%s)\noperator %s\n", *code, tenant, user)
	if *enrolTOTP {
		secret, err := identity.NewTOTPSecret()
		if err != nil {
			return err
		}
		if err := (&identity.PGUserStore{Pool: pool}).EnrollTOTP(ctx, tenant, user, secret); err != nil {
			return err
		}
		fmt.Printf("DEVELOPMENT AUTHENTICATOR (synthetic, not a bank credential)\n%s\n", identity.OTPAuthURI(*code, *email, secret))
	}

	if *seedClocks {
		svc := &configsvc.Service{Audit: &audit.Appender{}}
		var proposed []configsvc.Version
		err := pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			var err error
			proposed, err = svc.SeedClockProposals(ctx, tx, tenant, user, time.Now())
			return err
		})
		if err != nil {
			return fmt.Errorf("seed clocks: %w", err)
		}
		fmt.Printf("proposed %d regulatory clocks; none is in force until another person approves it\n", len(proposed))
	}
	return nil
}
