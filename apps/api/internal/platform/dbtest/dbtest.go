// Package dbtest gives integration tests a migrated database.
package dbtest

import (
	"strings"

	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"os"
	"testing"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/migrate"
	"github.com/smartreversal/smartreversal/apps/api/migrations"
)

// Pool opens TEST_DATABASE_URL, applies every pending migration and returns the
// pool. The test is skipped when the variable is not set. Use a disposable
// database: tests insert and delete rows.
func Pool(t testing.TB) *db.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, err := db.Open(ctx, url, db.Options{MaxConns: 8})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)

	ms, err := migrate.Load(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrate.New(p.Pool, ms).Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return p
}

// Tenant creates an institution for one test and removes it, and everything
// that refers to it, when the test ends. It uses the pool's own role, which is
// not subject to row-level security.
func Tenant(t testing.TB, p *db.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	code := "T-" + strings.ToUpper(uuid.NewString()[:12])
	var id uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO institutions (code, name) VALUES ($1, $1) RETURNING id`, code).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		// The audit trail refuses deletes, even from the owner. Tests clean up
		// as a superuser with triggers and foreign keys switched off for this
		// one transaction. This only works for a superuser, which test
		// databases use; production roles cannot do it.
		_ = p.WithTx(bg, func(tx pgx.Tx) error {
			if _, err := tx.Exec(bg, `SET LOCAL session_replication_role = replica`); err != nil {
				return err
			}
			for _, q := range []string{
				`DELETE FROM deferred_work WHERE tenant_id = $1`,
				`DELETE FROM automation_jobs WHERE tenant_id = $1`,
				`DELETE FROM sim_provider_requests WHERE tenant_id = $1`,
				`DELETE FROM sim_provider_script WHERE tenant_id = $1`,
				`DELETE FROM sim_ledger_entries WHERE tenant_id = $1`,
				`DELETE FROM sim_status_script WHERE tenant_id = $1`,
				`DELETE FROM approvals WHERE tenant_id = $1`,
				`DELETE FROM reversals WHERE tenant_id = $1`,
				`DELETE FROM notifications WHERE tenant_id = $1`,
				`DELETE FROM case_evidence WHERE tenant_id = $1`,
				`DELETE FROM case_events WHERE tenant_id = $1`,
				`DELETE FROM cases WHERE tenant_id = $1`,
				`DELETE FROM transaction_events WHERE tenant_id = $1`,
				`DELETE FROM quarantined_events WHERE tenant_id = $1`,
				`DELETE FROM transactions WHERE tenant_id = $1`,
				`DELETE FROM delegation_revocations WHERE tenant_id = $1`,
				`DELETE FROM delegations WHERE tenant_id = $1`,
				`DELETE FROM on_call_shifts WHERE tenant_id = $1`,
				`DELETE FROM escalation_steps WHERE tenant_id = $1`,
				`DELETE FROM kill_switch_events WHERE tenant_id = $1`,
				`DELETE FROM config_decisions WHERE tenant_id = $1`,
				`DELETE FROM config_versions WHERE tenant_id = $1`,
				`DELETE FROM audit_events WHERE tenant_id = $1`,
				`DELETE FROM audit_heads WHERE tenant_id = $1`,
			} {
				if _, err := tx.Exec(bg, q, id); err != nil {
					return err
				}
			}
			return nil
		})
		for _, q := range []string{
			`DELETE FROM service_assertion_uses WHERE tenant_id = $1`,
			`DELETE FROM service_accounts WHERE tenant_id = $1`,
			`DELETE FROM account_vault WHERE tenant_id = $1`,
			`DELETE FROM processed_events WHERE tenant_id = $1`,
			`DELETE FROM outbox_events WHERE tenant_id = $1`,
			`DELETE FROM users WHERE tenant_id = $1`,
			`DELETE FROM institutions WHERE id = $1`,
		} {
			_, _ = p.Exec(bg, q, id)
		}
	})
	return id
}
