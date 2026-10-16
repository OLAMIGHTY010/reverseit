package datashield_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/datashield"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

func newVault(t *testing.T) *datashield.Vault {
	t.Helper()
	tk, err := datashield.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	ek, _ := datashield.GenerateKey()
	tokenizer, err := datashield.NewTokenizer(tk)
	if err != nil {
		t.Fatal(err)
	}
	sealer, err := datashield.NewSealer("k1", ek)
	if err != nil {
		t.Fatal(err)
	}
	return &datashield.Vault{Tokenizer: tokenizer, Sealer: sealer}
}

func TestVaultStoresMasksAndRevealsPerTenant(t *testing.T) {
	pool := dbtest.Pool(t)
	v := newVault(t)
	ctx := context.Background()
	a, b := dbtest.Tenant(t, pool), dbtest.Tenant(t, pool)

	var stored datashield.Stored
	err := pool.WithTenant(ctx, a, func(tx pgx.Tx) error {
		var err error
		stored, err = v.Store(ctx, tx, a, "0123 456 789")
		if err != nil {
			return err
		}
		again, err := v.Store(ctx, tx, a, "0123456789")
		if err != nil {
			return err
		}
		if again != stored {
			t.Errorf("storing twice gave %v then %v", stored, again)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Masked != "****6789" || !strings.HasPrefix(stored.Token, "acct_") {
		t.Fatalf("stored: %+v", stored)
	}

	// What is in the table never contains the number.
	var raw string
	var keyID string
	if err := pool.QueryRow(ctx, `SELECT encode(ciphertext, 'hex'), key_id FROM account_vault WHERE tenant_id = $1`, a).Scan(&raw, &keyID); err != nil {
		t.Fatal(err)
	}
	if keyID != "k1" || strings.Contains(raw, "30313233343536373839") { // hex of "0123456789"
		t.Fatalf("vault row looks unsealed: key=%s", keyID)
	}

	err = pool.WithTenant(ctx, a, func(tx pgx.Tx) error {
		got, err := v.Reveal(ctx, tx, a, stored.Token)
		if err != nil || got != "0123456789" {
			t.Errorf("reveal: %q %v", got, err)
		}
		m, err := v.Masked(ctx, tx, a, stored.Token)
		if err != nil || m != "****6789" {
			t.Errorf("masked: %q %v", m, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Another tenant can neither see nor open it.
	err = pool.WithTenant(ctx, b, func(tx pgx.Tx) error {
		if _, err := v.Reveal(ctx, tx, b, stored.Token); !errors.Is(err, datashield.ErrUnknownToken) {
			t.Errorf("tenant B reveal: %v", err)
		}
		if _, err := v.Reveal(ctx, tx, a, stored.Token); !errors.Is(err, datashield.ErrUnknownToken) {
			t.Errorf("tenant B revealing as tenant A: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestVaultRowsCannotBeChangedByTenantCode(t *testing.T) {
	pool := dbtest.Pool(t)
	v := newVault(t)
	ctx := context.Background()
	tenant := dbtest.Tenant(t, pool)
	if err := pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := v.Store(ctx, tx, tenant, "0123456789")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{`UPDATE account_vault SET masked_display = '****0000'`, `DELETE FROM account_vault`} {
		err := pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, stmt)
			return err
		})
		if err == nil {
			t.Errorf("tenant role ran %q", stmt)
		}
	}
}

func TestVaultRejectsBadInputAndUnknownTokens(t *testing.T) {
	pool := dbtest.Pool(t)
	v := newVault(t)
	ctx := context.Background()
	tenant := dbtest.Tenant(t, pool)
	err := pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := v.Store(ctx, tx, tenant, "bad!"); err == nil {
			t.Error("bad account stored")
		}
		if _, err := v.Reveal(ctx, tx, tenant, "acct_"+strings.Repeat("0", 32)); !errors.Is(err, datashield.ErrUnknownToken) {
			t.Errorf("unknown token: %v", err)
		}
		if _, err := v.Masked(ctx, tx, uuid.New(), "acct_"+strings.Repeat("0", 32)); !errors.Is(err, datashield.ErrUnknownToken) {
			t.Errorf("masked unknown: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
