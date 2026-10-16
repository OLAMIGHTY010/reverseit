package migrations_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/migrate"
	"github.com/smartreversal/smartreversal/apps/api/migrations"
)

func TestEmbeddedMigrationsLoad(t *testing.T) {
	ms, err := migrate.Load(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 {
		t.Fatal("no migrations embedded")
	}
	for _, m := range ms {
		if m.Down == "" {
			t.Errorf("migration %04d_%s has no down script", m.Version, m.Name)
		}
	}
}

// TestEmbeddedMigrationsApplyAndRevert runs the real migrations against a
// disposable database: up, down to empty, up again.
func TestEmbeddedMigrationsApplyAndRevert(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Run in a private schema so this test cannot disturb tables that other
	// packages' tests use in the shared database.
	const schema = "sr_migrations_embedded_test"
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE; CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	ms, err := migrate.Load(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	r := migrate.New(pool, ms)

	if done, err := r.Up(ctx); err != nil || len(done) != len(ms) {
		t.Fatalf("up: %v %v", done, err)
	}
	if done, err := r.Down(ctx, len(ms)); err != nil || len(done) != len(ms) {
		t.Fatalf("down: %v %v", done, err)
	}
	if done, err := r.Up(ctx); err != nil || len(done) != len(ms) {
		t.Fatalf("re-up: %v %v", done, err)
	}
}
