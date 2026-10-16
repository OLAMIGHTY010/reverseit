// Command demo seeds and replays the demonstration scenario against the
// configured database. Everything is synthetic and every money movement is
// simulated. It refuses to run when ENVIRONMENT is production.
//
//	demo seed    create the demo institution, operators, authenticators and policies
//	demo play    reset the scenario data, then run the scenario once
//	demo reset   remove the scenario data
//	demo code    print the current one-time code for an operator (officer|supervisor)
//
// The password comes from DEMO_PASSWORD. Authenticator secrets are printed by
// seed only, and are synthetic.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/config"
	"github.com/smartreversal/smartreversal/apps/api/internal/demo"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

const institution = "DEMO-BANK"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "demo:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: demo seed|play|reset|code")
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	if cfg.Environment == config.EnvProduction {
		return fmt.Errorf("refusing to run: the demonstration data is synthetic and must never be loaded into production")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Open(ctx, cfg.DatabaseURL(), db.Options{MaxConns: 4})
	if err != nil {
		return err
	}
	defer pool.Close()
	env, err := demo.NewEnv(ctx, pool)
	if err != nil {
		return err
	}
	password := os.Getenv("DEMO_PASSWORD")
	switch os.Args[1] {
	case "seed":
		if len(password) < 12 {
			return fmt.Errorf("set DEMO_PASSWORD (at least 12 characters)")
		}
		s, err := demo.Seed(ctx, env, institution, password)
		if err != nil {
			return err
		}
		fmt.Printf("institution %s (%s)\n", s.Institution, s.Tenant)
		for _, o := range []demo.Operator{s.Officer, s.Supervisor} {
			fmt.Printf("%s  roles=%v\n  authenticator (SYNTHETIC, development only): %s\n", o.Email, o.Roles, o.OTPAuth)
		}
		fmt.Printf("policies installed under the label %q\n", demo.PolicyLabel)
	case "reset", "play":
		tenant, err := env.Store.TenantIDByCode(ctx, institution)
		if err != nil {
			return fmt.Errorf("run `demo seed` first: %w", err)
		}
		if err := demo.Reset(ctx, pool, tenant); err != nil {
			return err
		}
		if os.Args[1] == "reset" {
			fmt.Println("scenario data removed")
			return nil
		}
		rep, err := demo.Play(ctx, env, tenant)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	case "code":
		if len(os.Args) < 3 {
			return fmt.Errorf("usage: demo code officer|supervisor")
		}
		email := map[string]string{"officer": "officer@demo.example", "supervisor": "supervisor@demo.example"}[os.Args[2]]
		if email == "" {
			return fmt.Errorf("unknown operator %q", os.Args[2])
		}
		tenant, err := env.Store.TenantIDByCode(ctx, institution)
		if err != nil {
			return err
		}
		u, err := env.Store.FindByEmail(ctx, tenant, email)
		if err != nil {
			return err
		}
		secret, err := env.Store.TOTPSecret(ctx, tenant, u.ID)
		if err != nil {
			return err
		}
		fmt.Println(identity.TOTPCode(secret, time.Now()))
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
	return nil
}
