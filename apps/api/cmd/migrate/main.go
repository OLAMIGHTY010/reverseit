// Command migrate applies, reverts and reports database migrations.
//
//	migrate up
//	migrate down [steps]   (default 1)
//	migrate status
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/smartreversal/smartreversal/apps/api/internal/config"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/migrate"
	"github.com/smartreversal/smartreversal/apps/api/migrations"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: migrate up | down [steps] | status")
	}
	cmd := args[0]
	steps := 1
	switch cmd {
	case "up", "status":
		if len(args) != 1 {
			return fmt.Errorf("%s takes no arguments", cmd)
		}
	case "down":
		if len(args) > 2 {
			return fmt.Errorf("usage: migrate down [steps]")
		}
		if len(args) == 2 {
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 1 {
				return fmt.Errorf("steps must be a positive integer")
			}
			steps = n
		}
	default:
		return fmt.Errorf("unknown command %q; use up, down or status", cmd)
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

	ms, err := migrate.Load(migrations.FS, ".")
	if err != nil {
		return err
	}
	r := migrate.New(pool.Pool, ms)

	switch cmd {
	case "up":
		done, err := r.Up(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("applied %d migration(s)\n", len(done))
	case "down":
		done, err := r.Down(ctx, steps)
		if err != nil {
			return err
		}
		fmt.Printf("reverted %d migration(s)\n", len(done))
	case "status":
		st, err := r.Status(ctx)
		if err != nil {
			return err
		}
		for _, s := range st {
			mark := "pending"
			if s.Applied {
				mark = "applied"
			}
			fmt.Printf("%04d  %-8s  %s\n", s.Version, mark, s.Name)
		}
	}
	return nil
}
