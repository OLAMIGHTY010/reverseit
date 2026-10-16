// Package migrate applies versioned SQL migrations to PostgreSQL.
//
// Files are named NNNN_description.up.sql and NNNN_description.down.sql. Every
// migration runs in its own transaction, so statements that cannot run inside
// a transaction (CREATE INDEX CONCURRENTLY) are not supported. Applied
// migrations are checksummed: editing an applied file is an error, never a
// silent change. A PostgreSQL advisory lock stops two runners from migrating
// at once.
package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lockKey is the advisory lock id. It is arbitrary but fixed.
const lockKey int64 = 7_351_204_118

var fileName = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.(up|down)\.sql$`)

// Migration is one versioned change.
type Migration struct {
	Version  int
	Name     string
	Up       string
	Down     string
	Checksum string // sha256 of the up script
}

// Load reads migrations from dir inside fsys. Every version needs an up file.
func Load(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("migrate: read %s: %w", dir, err)
	}

	byVersion := map[int]*Migration{}
	for _, e := range entries {
		if e.IsDir() || e.Name() == ".gitkeep" {
			continue
		}
		m := fileName.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("migrate: unexpected file name %q", e.Name())
		}
		version, _ := strconv.Atoi(m[1])
		body, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", e.Name(), err)
		}

		mg := byVersion[version]
		if mg == nil {
			mg = &Migration{Version: version, Name: m[2]}
			byVersion[version] = mg
		} else if mg.Name != m[2] {
			return nil, fmt.Errorf("migrate: version %04d has two names: %s and %s", version, mg.Name, m[2])
		}
		if m[3] == "up" {
			mg.Up = string(body)
			sum := sha256.Sum256(body)
			mg.Checksum = hex.EncodeToString(sum[:])
		} else {
			mg.Down = string(body)
		}
	}

	out := make([]Migration, 0, len(byVersion))
	for _, mg := range byVersion {
		if mg.Up == "" {
			return nil, fmt.Errorf("migrate: version %04d has no up file", mg.Version)
		}
		out = append(out, *mg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	for i, mg := range out {
		if mg.Version != i+1 {
			return nil, fmt.Errorf("migrate: versions must start at 0001 and have no gaps, found %04d at position %d", mg.Version, i+1)
		}
	}
	return out, nil
}

// Runner applies migrations.
type Runner struct {
	pool       *pgxpool.Pool
	migrations []Migration
}

// New returns a Runner.
func New(pool *pgxpool.Pool, migrations []Migration) *Runner {
	return &Runner{pool: pool, migrations: migrations}
}

// Status describes one migration.
type Status struct {
	Version int
	Name    string
	Applied bool
}

type appliedRow struct {
	version  int
	checksum string
}

// session holds the connection that owns the advisory lock.
func (r *Runner) session(ctx context.Context, fn func(conn *pgxpool.Conn) error) error {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrate: acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return fmt.Errorf("migrate: take lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, lockKey)
	}()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    integer     PRIMARY KEY,
		name       text        NOT NULL,
		checksum   text        NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("migrate: create schema_migrations: %w", err)
	}
	return fn(conn)
}

func (r *Runner) applied(ctx context.Context, conn *pgxpool.Conn) ([]appliedRow, error) {
	rows, err := conn.Query(ctx, `SELECT version, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("migrate: read applied: %w", err)
	}
	defer rows.Close()
	var out []appliedRow
	for rows.Next() {
		var a appliedRow
		if err := rows.Scan(&a.version, &a.checksum); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// verify checks that the applied set is a prefix of the known migrations and
// that no applied file has changed.
func (r *Runner) verify(applied []appliedRow) error {
	for i, a := range applied {
		if i >= len(r.migrations) || r.migrations[i].Version != a.version {
			return fmt.Errorf("migrate: database has migration %04d that is not in this build", a.version)
		}
		if r.migrations[i].Checksum != a.checksum {
			return fmt.Errorf("migrate: migration %04d_%s was edited after it was applied", a.version, r.migrations[i].Name)
		}
	}
	return nil
}

// Up applies every pending migration and returns the versions applied.
func (r *Runner) Up(ctx context.Context) ([]int, error) {
	var done []int
	err := r.session(ctx, func(conn *pgxpool.Conn) error {
		applied, err := r.applied(ctx, conn)
		if err != nil {
			return err
		}
		if err := r.verify(applied); err != nil {
			return err
		}
		for _, m := range r.migrations[len(applied):] {
			if err := runInTx(ctx, conn, m.Up, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
					m.Version, m.Name, m.Checksum)
				return err
			}); err != nil {
				return fmt.Errorf("migrate: apply %04d_%s: %w", m.Version, m.Name, err)
			}
			done = append(done, m.Version)
		}
		return nil
	})
	return done, err
}

// Down reverts the most recent steps migrations and returns the versions
// reverted. A migration without a down script cannot be reverted.
func (r *Runner) Down(ctx context.Context, steps int) ([]int, error) {
	if steps < 1 {
		return nil, errors.New("migrate: steps must be at least 1")
	}
	var done []int
	err := r.session(ctx, func(conn *pgxpool.Conn) error {
		applied, err := r.applied(ctx, conn)
		if err != nil {
			return err
		}
		if err := r.verify(applied); err != nil {
			return err
		}
		for i := 0; i < steps && len(applied) > 0; i++ {
			m := r.migrations[len(applied)-1]
			if m.Down == "" {
				return fmt.Errorf("migrate: %04d_%s has no down script", m.Version, m.Name)
			}
			if err := runInTx(ctx, conn, m.Down, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, m.Version)
				return err
			}); err != nil {
				return fmt.Errorf("migrate: revert %04d_%s: %w", m.Version, m.Name, err)
			}
			done = append(done, m.Version)
			applied = applied[:len(applied)-1]
		}
		return nil
	})
	return done, err
}

// Status lists every known migration and whether it is applied.
func (r *Runner) Status(ctx context.Context) ([]Status, error) {
	var out []Status
	err := r.session(ctx, func(conn *pgxpool.Conn) error {
		applied, err := r.applied(ctx, conn)
		if err != nil {
			return err
		}
		if err := r.verify(applied); err != nil {
			return err
		}
		for i, m := range r.migrations {
			out = append(out, Status{Version: m.Version, Name: m.Name, Applied: i < len(applied)})
		}
		return nil
	})
	return out, err
}

func runInTx(ctx context.Context, conn *pgxpool.Conn, script string, after func(pgx.Tx) error) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, script); err != nil {
		_ = tx.Rollback(context.WithoutCancel(ctx))
		return err
	}
	if err := after(tx); err != nil {
		_ = tx.Rollback(context.WithoutCancel(ctx))
		return err
	}
	return tx.Commit(ctx)
}
