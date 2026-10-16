package migrate

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func mapFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys["migrations/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func TestLoadOrdersAndChecksums(t *testing.T) {
	ms, err := Load(mapFS(map[string]string{
		"0002_second.up.sql":   "SELECT 2;",
		"0001_first.up.sql":    "SELECT 1;",
		"0001_first.down.sql":  "SELECT -1;",
		"0002_second.down.sql": "SELECT -2;",
	}), "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 2 || ms[0].Version != 1 || ms[1].Name != "second" {
		t.Fatalf("unexpected: %+v", ms)
	}
	if ms[0].Down != "SELECT -1;" || ms[0].Checksum == "" || ms[0].Checksum == ms[1].Checksum {
		t.Fatalf("down or checksum wrong: %+v", ms)
	}
}

func TestLoadRejectsBadInput(t *testing.T) {
	cases := map[string]map[string]string{
		"bad name":     {"first.up.sql": "x"},
		"no up":        {"0001_a.down.sql": "x"},
		"gap":          {"0001_a.up.sql": "x", "0003_c.up.sql": "x"},
		"starts at 2":  {"0002_b.up.sql": "x"},
		"name differs": {"0001_a.up.sql": "x", "0001_b.down.sql": "x"},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(mapFS(files), "migrations"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadEmptyDirIsFine(t *testing.T) {
	fsys := fstest.MapFS{"migrations/.gitkeep": &fstest.MapFile{}}
	ms, err := Load(fsys, "migrations")
	if err != nil || len(ms) != 0 {
		t.Fatalf("ms=%v err=%v", ms, err)
	}
}

// testPool returns a pool whose search_path is a private schema, so these tests
// never touch the tables other packages' tests rely on.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const schema = "sr_migrate_pkg_test"
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE; CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a, err := pgxpool.New(context.Background(), url)
		if err == nil {
			_, _ = a.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
			a.Close()
		}
	})

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// cleanup removes everything the test created, including the bookkeeping table.
func cleanup(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	drop := func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS mig_a, mig_b, schema_migrations`)
	}
	drop()
	t.Cleanup(drop)
}

func testMigrations(t *testing.T) []Migration {
	t.Helper()
	ms, err := Load(mapFS(map[string]string{
		"0001_a.up.sql":   "CREATE TABLE mig_a (id int PRIMARY KEY); INSERT INTO mig_a VALUES (1);",
		"0001_a.down.sql": "DROP TABLE mig_a;",
		"0002_b.up.sql":   "CREATE TABLE mig_b (id int PRIMARY KEY);",
		"0002_b.down.sql": "DROP TABLE mig_b;",
	}), "migrations")
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestUpDownUp(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	ctx := context.Background()
	r := New(pool, testMigrations(t))

	done, err := r.Up(ctx)
	if err != nil || len(done) != 2 {
		t.Fatalf("up: %v %v", done, err)
	}
	if !tableExists(t, pool, "mig_a") || !tableExists(t, pool, "mig_b") {
		t.Fatal("tables missing after up")
	}

	done, err = r.Up(ctx)
	if err != nil || len(done) != 0 {
		t.Fatalf("second up should do nothing: %v %v", done, err)
	}

	done, err = r.Down(ctx, 1)
	if err != nil || len(done) != 1 || done[0] != 2 {
		t.Fatalf("down 1: %v %v", done, err)
	}
	if tableExists(t, pool, "mig_b") || !tableExists(t, pool, "mig_a") {
		t.Fatal("down reverted the wrong migration")
	}

	st, err := r.Status(ctx)
	if err != nil || !st[0].Applied || st[1].Applied {
		t.Fatalf("status: %+v %v", st, err)
	}

	if _, err := r.Down(ctx, 5); err != nil {
		t.Fatalf("down all: %v", err)
	}
	if tableExists(t, pool, "mig_a") {
		t.Fatal("mig_a still present")
	}

	if done, err = r.Up(ctx); err != nil || len(done) != 2 {
		t.Fatalf("re-apply: %v %v", done, err)
	}
}

func TestFailedMigrationRollsBackCompletely(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	ms, err := Load(mapFS(map[string]string{
		"0001_a.up.sql": "CREATE TABLE mig_a (id int PRIMARY KEY); SELECT 1/0;",
	}), "migrations")
	if err != nil {
		t.Fatal(err)
	}
	r := New(pool, ms)

	if _, err := r.Up(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	if tableExists(t, pool, "mig_a") {
		t.Fatal("partial migration left a table behind")
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("version recorded for failed migration: n=%d err=%v", n, err)
	}
}

func TestEditedMigrationIsRejected(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	ctx := context.Background()

	if _, err := New(pool, testMigrations(t)).Up(ctx); err != nil {
		t.Fatal(err)
	}

	edited, err := Load(mapFS(map[string]string{
		"0001_a.up.sql":   "CREATE TABLE mig_a (id int PRIMARY KEY, extra int);",
		"0001_a.down.sql": "DROP TABLE mig_a;",
		"0002_b.up.sql":   "CREATE TABLE mig_b (id int PRIMARY KEY);",
		"0002_b.down.sql": "DROP TABLE mig_b;",
	}), "migrations")
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(pool, edited).Up(ctx)
	if err == nil || !strings.Contains(err.Error(), "edited after it was applied") {
		t.Fatalf("expected checksum error, got %v", err)
	}
}

func TestUnknownAppliedMigrationIsRejected(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	ctx := context.Background()

	if _, err := New(pool, testMigrations(t)).Up(ctx); err != nil {
		t.Fatal(err)
	}
	older := testMigrations(t)[:1]
	if _, err := New(pool, older).Up(ctx); err == nil {
		t.Fatal("database ahead of build was accepted")
	}
}

func TestDownWithoutScriptFails(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	ctx := context.Background()
	ms, err := Load(mapFS(map[string]string{"0001_a.up.sql": "CREATE TABLE mig_a (id int);"}), "migrations")
	if err != nil {
		t.Fatal(err)
	}
	r := New(pool, ms)
	if _, err := r.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Down(ctx, 1); err == nil {
		t.Fatal("expected error")
	}
	if _, err := r.Down(ctx, 0); err == nil {
		t.Fatal("zero steps should be rejected")
	}
}

func TestConcurrentRunnersApplyOnce(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	ctx := context.Background()
	ms := testMigrations(t)

	type result struct {
		n   int
		err error
	}
	res := make(chan result, 4)
	for i := 0; i < 4; i++ {
		go func() {
			done, err := New(pool, ms).Up(ctx)
			res <- result{len(done), err}
		}()
	}
	total := 0
	for i := 0; i < 4; i++ {
		r := <-res
		if r.err != nil {
			t.Fatalf("runner failed: %v", r.err)
		}
		total += r.n
	}
	if total != 2 {
		t.Fatalf("migrations applied %d times in total, want 2", total)
	}
}

func TestLoadFromRootDirectory(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_a.up.sql":   &fstest.MapFile{Data: []byte("SELECT 1;")},
		"0001_a.down.sql": &fstest.MapFile{Data: []byte("SELECT 2;")},
	}
	ms, err := Load(fsys, ".")
	if err != nil || len(ms) != 1 || ms[0].Down != "SELECT 2;" {
		t.Fatalf("ms=%+v err=%v", ms, err)
	}
}
