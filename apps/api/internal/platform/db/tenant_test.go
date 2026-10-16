package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type tenants struct {
	pool   *db.Pool
	a, b   uuid.UUID
	userA  uuid.UUID
	userB  uuid.UUID
	emailA string
	emailB string
}

// newTenants creates two institutions with one user each, using the
// connection's own (privileged) role, then removes them afterwards.
func newTenants(t *testing.T) *tenants {
	t.Helper()
	p := dbtest.Pool(t)
	ctx := context.Background()
	x := &tenants{pool: p, emailA: "a@example.test", emailB: "b@example.test"}

	suffix := strings.ToUpper(uuid.NewString()[:8])
	mk := func(code, email string) (uuid.UUID, uuid.UUID) {
		var inst, user uuid.UUID
		if err := p.QueryRow(ctx, `INSERT INTO institutions (code, name) VALUES ($1, $2) RETURNING id`,
			code, code).Scan(&inst); err != nil {
			t.Fatal(err)
		}
		if err := p.QueryRow(ctx, `INSERT INTO users (tenant_id, email, display_name, password_hash)
			VALUES ($1, $2, 'Test User', 'x') RETURNING id`, inst, email).Scan(&user); err != nil {
			t.Fatal(err)
		}
		return inst, user
	}
	x.a, x.userA = mk("TEST-A-"+suffix, x.emailA)
	x.b, x.userB = mk("TEST-B-"+suffix, x.emailB)

	t.Cleanup(func() {
		for _, id := range []uuid.UUID{x.a, x.b} {
			_, _ = p.Exec(context.Background(), `DELETE FROM users WHERE tenant_id = $1`, id)
			_, _ = p.Exec(context.Background(), `DELETE FROM institutions WHERE id = $1`, id)
		}
	})
	return x
}

func TestWithTenantRejectsZeroID(t *testing.T) {
	p := dbtest.Pool(t)
	err := p.WithTenant(context.Background(), uuid.Nil, func(pgx.Tx) error { return nil })
	if err == nil {
		t.Fatal("zero tenant id accepted")
	}
}

func TestTenantSeesOnlyItsOwnRows(t *testing.T) {
	x := newTenants(t)
	ctx := context.Background()

	for name, tc := range map[string]struct {
		tenant uuid.UUID
		want   string
	}{"A": {x.a, x.emailA}, "B": {x.b, x.emailB}} {
		var emails []string
		err := x.pool.WithTenant(ctx, tc.tenant, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT email FROM users WHERE email IN ($1, $2)`, x.emailA, x.emailB)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var e string
				if err := rows.Scan(&e); err != nil {
					return err
				}
				emails = append(emails, e)
			}
			return rows.Err()
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(emails) != 1 || emails[0] != tc.want {
			t.Errorf("tenant %s saw %v, want only %s", name, emails, tc.want)
		}
	}
}

func TestTenantCannotReadAnotherTenantsRowByID(t *testing.T) {
	x := newTenants(t)
	ctx := context.Background()
	err := x.pool.WithTenant(ctx, x.a, func(tx pgx.Tx) error {
		var email string
		err := tx.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, x.userB).Scan(&email)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("reading tenant B's user as tenant A: err=%v email=%q", err, email)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTenantCannotInsertForAnotherTenant(t *testing.T) {
	x := newTenants(t)
	ctx := context.Background()
	err := x.pool.WithTenant(ctx, x.a, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO users (tenant_id, email, display_name, password_hash)
			VALUES ($1, 'intruder@example.test', 'Intruder', 'x')`, x.b)
		return err
	})
	if err == nil {
		t.Fatal("tenant A inserted a row for tenant B")
	}
}

func TestTenantCannotUpdateOrDeleteAnotherTenantsRows(t *testing.T) {
	x := newTenants(t)
	ctx := context.Background()
	err := x.pool.WithTenant(ctx, x.a, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET display_name = 'hacked' WHERE id = $1`, x.userB)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 0 {
			t.Errorf("tenant A updated %d of tenant B's rows", tag.RowsAffected())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var name string
	if err := x.pool.QueryRow(ctx, `SELECT display_name FROM users WHERE id = $1`, x.userB).Scan(&name); err != nil || name != "Test User" {
		t.Fatalf("tenant B's user changed: name=%q err=%v", name, err)
	}
}

func TestTenantCannotMoveItsRowToAnotherTenant(t *testing.T) {
	x := newTenants(t)
	ctx := context.Background()
	err := x.pool.WithTenant(ctx, x.a, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE users SET tenant_id = $1 WHERE id = $2`, x.b, x.userA)
		return err
	})
	if err == nil {
		t.Fatal("tenant A moved its user into tenant B")
	}
}

func TestInstitutionVisibleOnlyToItself(t *testing.T) {
	x := newTenants(t)
	ctx := context.Background()
	err := x.pool.WithTenant(ctx, x.a, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM institutions WHERE id IN ($1, $2)`, x.a, x.b).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("tenant A can see %d institutions, want 1", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Without WithTenant a connection has no tenant, so the role sees nothing.
func TestNoTenantContextSeesNothing(t *testing.T) {
	x := newTenants(t)
	ctx := context.Background()
	err := x.pool.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+db.TenantRole); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("tenant role without context sees %d users, want 0", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The tenant setting must not survive the transaction on a pooled connection.
func TestTenantSettingDoesNotLeakAcrossTransactions(t *testing.T) {
	x := newTenants(t)
	ctx := context.Background()
	if err := x.pool.WithTenant(ctx, x.a, func(pgx.Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		var setting string
		if err := x.pool.QueryRow(ctx, `SELECT coalesce(current_setting('app.tenant_id', true), '')`).Scan(&setting); err != nil {
			t.Fatal(err)
		}
		if setting != "" {
			t.Fatalf("app.tenant_id leaked as %q", setting)
		}
	}
}

func TestLoginLookupFunctionReturnsOnlyTheID(t *testing.T) {
	x := newTenants(t)
	ctx := context.Background()
	var code string
	if err := x.pool.QueryRow(ctx, `SELECT code FROM institutions WHERE id = $1`, x.a).Scan(&code); err != nil {
		t.Fatal(err)
	}
	err := x.pool.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+db.TenantRole); err != nil {
			return err
		}
		var got uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT institution_id_by_code($1)`, code).Scan(&got); err != nil {
			return err
		}
		if got != x.a {
			t.Errorf("lookup returned %s, want %s", got, x.a)
		}
		var none *uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT institution_id_by_code('NO-SUCH-CODE')`).Scan(&none); err != nil {
			return err
		}
		if none != nil {
			t.Errorf("unknown code returned %s", none)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
