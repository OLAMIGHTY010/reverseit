package customer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

const (
	tokA = "acct_0123456789abcdef0123456789abcdef"
	tokB = "acct_fedcba9876543210fedcba9876543210"
)

const pass = "synthetic customer passphrase"

type rig struct {
	pool   *db.Pool
	tenant uuid.UUID
	code   string
	store  *Store
	prov   *DevProvider
	now    time.Time
	t      *testing.T
}

func newRig(t *testing.T) *rig {
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), now: time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC), t: t}
	if err := pool.QueryRow(context.Background(), `SELECT code FROM institutions WHERE id = $1`, r.tenant).Scan(&r.code); err != nil {
		t.Fatal(err)
	}
	r.store = &Store{Pool: pool}
	r.prov = &DevProvider{Pool: pool, Audit: &audit.Appender{Now: func() time.Time { return r.now }}, Now: func() time.Time { return r.now }}
	return r
}

func (r *rig) auth(ref, secret string) (uuid.UUID, Customer, error) {
	return r.prov.Authenticate(context.Background(), r.code, ref, secret)
}

func TestACustomerSignsInWithTheRightPassphraseAndEverythingElseLooksTheSame(t *testing.T) {
	r := newRig(t)
	c, err := r.store.Create(context.Background(), r.tenant, "Synthetic Customer", pass)
	if err != nil {
		t.Fatal(err)
	}
	tenant, got, err := r.auth(c.Ref, pass)
	if err != nil || tenant != r.tenant || got.ID != c.ID || got.Ref != c.Ref {
		t.Fatalf("%v %+v", err, got)
	}
	for name, try := range map[string]func() error{
		"wrong passphrase": func() error { _, _, err := r.auth(c.Ref, "not the passphrase"); return err },
		"unknown customer": func() error { _, _, err := r.auth("cust_000000000000000000000000", pass); return err },
		"unknown institution": func() error {
			_, _, err := r.prov.Authenticate(context.Background(), "NO-SUCH-BANK", c.Ref, pass)
			return err
		},
		"empty passphrase": func() error { _, _, err := r.auth(c.Ref, ""); return err },
		"another institution": func() error {
			other := dbtest.Tenant(t, r.pool)
			var code string
			_ = r.pool.QueryRow(context.Background(), `SELECT code FROM institutions WHERE id = $1`, other).Scan(&code)
			_, _, err := r.prov.Authenticate(context.Background(), code, c.Ref, pass)
			return err
		},
	} {
		if err := try(); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: %v", name, err)
		}
	}
	var n int
	_ = r.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE tenant_id = $1 AND action = 'customer.login'`, r.tenant).Scan(&n)
	if n != 1 {
		t.Fatalf("%d sign-ins audited, want the one success", n)
	}
}

func TestRepeatedWrongPassphrasesLockTheCustomerAndALockedCustomerIsNotCounted(t *testing.T) {
	r := newRig(t)
	c, _ := r.store.Create(context.Background(), r.tenant, "Locked Customer", pass)
	for i := 0; i < maxAttempts; i++ {
		if _, _, err := r.auth(c.Ref, "wrong passphrase!"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatal(err)
		}
	}
	// Locked: even the right passphrase is refused, and further attempts add nothing.
	if _, _, err := r.auth(c.Ref, pass); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a locked customer signed in: %v", err)
	}
	var attempts int
	_ = r.pool.QueryRow(context.Background(), `SELECT failed_attempts FROM customers WHERE id = $1`, c.ID).Scan(&attempts)
	for i := 0; i < 3; i++ {
		_, _, _ = r.auth(c.Ref, "wrong passphrase!")
	}
	var after int
	_ = r.pool.QueryRow(context.Background(), `SELECT failed_attempts FROM customers WHERE id = $1`, c.ID).Scan(&after)
	if after != attempts {
		t.Fatalf("attempts against a locked customer were counted: %d to %d", attempts, after)
	}
	r.now = r.now.Add(lockFor + time.Second)
	if _, _, err := r.auth(c.Ref, pass); err != nil {
		t.Fatalf("the lock should have expired: %v", err)
	}
	var reset int
	_ = r.pool.QueryRow(context.Background(), `SELECT failed_attempts FROM customers WHERE id = $1`, c.ID).Scan(&reset)
	if reset != 0 {
		t.Fatalf("%d", reset)
	}
}

func TestADisabledCustomerAndOneWithNoPassphraseCannotSignIn(t *testing.T) {
	r := newRig(t)
	c, _ := r.store.Create(context.Background(), r.tenant, "Disabled Customer", pass)
	if _, err := r.pool.Exec(context.Background(), `UPDATE customers SET status = 'disabled' WHERE id = $1`, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.auth(c.Ref, pass); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("%v", err)
	}
	d, _ := r.store.Create(context.Background(), r.tenant, "No Passphrase", pass)
	if _, err := r.pool.Exec(context.Background(), `UPDATE customers SET password_hash = '' WHERE id = $1`, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.auth(d.Ref, ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("%v", err)
	}
	if _, err := r.store.Create(context.Background(), r.tenant, "Short", "short"); err == nil {
		t.Fatal("a short passphrase was accepted")
	}
}

func TestOwnershipIsWhatTheBankRecordedAndNothingElse(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	alice, _ := r.store.Create(ctx, r.tenant, "Alice", pass)
	bob, _ := r.store.Create(ctx, r.tenant, "Bob", pass)
	if err := r.store.LinkAccount(ctx, r.tenant, alice.ID, tokA, "", "bank onboarding"); err != nil {
		t.Fatal(err)
	}
	if err := r.store.LinkAccount(ctx, r.tenant, alice.ID, tokA, "", "bank onboarding"); err != nil {
		t.Fatalf("linking twice should be harmless: %v", err)
	}
	if err := r.store.LinkAccount(ctx, r.tenant, alice.ID, "0123456789", "", "bank onboarding"); err == nil {
		t.Fatal("a raw account number was linked; only tokens are")
	}
	check := func(c Customer, token string) (owns bool) {
		_ = r.pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error {
			var err error
			owns, err = Owns(ctx, tx, r.tenant, c.ID, token)
			return err
		})
		return
	}
	if !check(alice, tokA) || check(alice, tokB) || check(bob, tokA) {
		t.Fatal("ownership is wrong")
	}
	// Another institution sees nothing.
	other := dbtest.Tenant(t, r.pool)
	var n int
	_ = r.pool.WithTenant(ctx, other, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM customer_accounts`).Scan(&n)
	})
	if n != 0 {
		t.Fatalf("%d links visible to another institution", n)
	}
}
