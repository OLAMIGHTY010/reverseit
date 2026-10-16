// Command sim generates synthetic transaction evidence with ground-truth labels, and
// plays it through the real ingestion and detection code.
//
//	sim list                              the eighteen scenarios
//	sim generate -seed 7 [-demo50] [-scenarios a,b] -out DIR
//	                                      write events.jsonl and truth.jsonl; no database
//	sim play -seed 7 [-demo50] [-scenarios a,b]
//	                                      play a run in a fresh synthetic institution
//	                                      SIM-<seed> and print the score as JSON
//
// Everything is synthetic and simulated. play uses a simulated clock, so the
// latencies in its report are rule and scenario delays, not performance. It refuses
// to run when ENVIRONMENT is production.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/config"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/sim"
	"github.com/smartreversal/smartreversal/apps/api/internal/simrun"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "sim:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: sim list|generate|play")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	seed := fs.Uint64("seed", 1, "seed; the same seed always gives the same run")
	demo50 := fs.Bool("demo50", false, "the fifty-transaction demonstration mix")
	scenarios := fs.String("scenarios", "", "comma-separated scenario names; default one of each")
	out := fs.String("out", "", "output directory for generate")
	if args[0] != "list" {
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
	}
	switch args[0] {
	case "list":
		for _, sc := range sim.Scenarios {
			fmt.Printf("%-28s %s\n", sc.Name, sc.Description)
		}
		return nil
	case "generate":
		r, err := build(*seed, *demo50, *scenarios)
		if err != nil {
			return err
		}
		if *out == "" {
			return fmt.Errorf("-out is required")
		}
		if err := os.MkdirAll(*out, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*out, "events.jsonl"), r.EventsJSONL(), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*out, "truth.jsonl"), r.TruthJSONL(), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %d events for %d transactions to %s\n", len(r.Events), r.Transactions(), *out)
		return nil
	case "play":
		return play(*seed, *demo50, *scenarios)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func build(seed uint64, demo50 bool, scenarios string) (sim.Run, error) {
	if demo50 {
		return sim.Demo50(seed, time.Time{})
	}
	var plan []string
	if scenarios != "" {
		plan = strings.Split(scenarios, ",")
	}
	return sim.Generate(sim.Options{Seed: seed, Plan: plan})
}

func play(seed uint64, demo50 bool, scenarios string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	if cfg.Environment == config.EnvProduction {
		return fmt.Errorf("refusing to run: the simulator must never write into production")
	}
	r, err := build(seed, demo50, scenarios)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Open(ctx, cfg.DatabaseURL(), db.Options{MaxConns: 4})
	if err != nil {
		return err
	}
	defer pool.Close()
	code := fmt.Sprintf("SIM-%d", seed)
	var tenant uuid.UUID
	var n int
	if err := pool.QueryRow(ctx, `INSERT INTO institutions (code, name) VALUES ($1, $1) ON CONFLICT (code) DO UPDATE SET code = EXCLUDED.code RETURNING id`, code).Scan(&tenant); err != nil {
		return err
	}
	if err := pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM transactions`).Scan(&n)
	}); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("institution %s already holds a played run; choose another seed", code)
	}
	h := simrun.NewHarness(pool, tenant, r.Start.Add(-time.Minute))
	if err := h.InstallPolicies(ctx); err != nil {
		return err
	}
	if err := h.Play(ctx, r, simrun.Options{}); err != nil {
		return err
	}
	got, err := h.Observe(ctx)
	if err != nil {
		return err
	}
	rep := simrun.Score(r.Truth, got)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"institution": code, "seed": seed, "synthetic": true, "simulated_clock": true, "report": rep})
}
