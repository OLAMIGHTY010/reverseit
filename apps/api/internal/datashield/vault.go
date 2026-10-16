package datashield

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrUnknownToken is returned when the vault holds no account for a token.
var ErrUnknownToken = errors.New("datashield: unknown account token")

// Vault keeps sealed account numbers in PostgreSQL.
type Vault struct {
	Tokenizer *Tokenizer
	Sealer    *Sealer
}

// Stored is what callers keep after storing an account.
type Stored struct {
	Token  string
	Masked string
}

// Store seals account and returns its token. Storing the same account again is
// harmless and returns the same token. tx must be a tenant transaction.
func (v *Vault) Store(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, account string) (Stored, error) {
	token, err := v.Tokenizer.Token(tenant, account)
	if err != nil {
		return Stored{}, err
	}
	sealed, err := v.Sealer.Seal(tenant, token, account)
	if err != nil {
		return Stored{}, err
	}
	masked := Mask(account)
	if _, err := tx.Exec(ctx, `
		INSERT INTO account_vault (tenant_id, token, masked_display, ciphertext, key_id)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (tenant_id, token) DO NOTHING`,
		tenant, token, masked, sealed, v.Sealer.KeyID()); err != nil {
		return Stored{}, fmt.Errorf("datashield: store: %w", err)
	}
	return Stored{Token: token, Masked: masked}, nil
}

// Masked returns the display form for a token without opening the vault.
func (v *Vault) Masked(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, token string) (string, error) {
	var m string
	err := tx.QueryRow(ctx, `SELECT masked_display FROM account_vault WHERE tenant_id = $1 AND token = $2`, tenant, token).Scan(&m)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUnknownToken
	}
	return m, err
}

// Reveal decrypts the account number for a token. It applies no permission
// check: callers must have authorised and audited the request first.
func (v *Vault) Reveal(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, token string) (string, error) {
	var sealed []byte
	err := tx.QueryRow(ctx, `SELECT ciphertext FROM account_vault WHERE tenant_id = $1 AND token = $2`, tenant, token).Scan(&sealed)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUnknownToken
	}
	if err != nil {
		return "", fmt.Errorf("datashield: read vault: %w", err)
	}
	return v.Sealer.Open(tenant, token, sealed)
}
