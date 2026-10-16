// Package customer holds ReverseIt's customers and their sign-in.
//
// A customer signs in to a different token audience from an operator, so a customer
// token never works on an operator route and an operator token never works on a
// customer route. The accounts a customer owns are recorded by the bank; nothing a
// customer types, pastes or uploads (an alert, an account number) is ever taken as
// proof of ownership.
//
// Sign-in is behind the Provider interface. DevProvider is for synthetic customers in
// development and test and is not wired in production; a production identity provider
// is an external dependency and replaces it without changing anything else here.
package customer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

// Role carried by a customer token. It grants nothing on operator routes.
const Role = "CUSTOMER"

// Errors.
var (
	ErrInvalidCredentials = errors.New("customer: invalid credentials")
	ErrNotFound           = errors.New("customer: no such customer")
)

const (
	maxAttempts = 5
	lockFor     = 15 * time.Minute
)

// Customer is a ReverseIt customer.
type Customer struct {
	ID          uuid.UUID
	Ref         string
	DisplayName string
}

// Store creates customers and links their accounts. It is used by setup, the demo and
// the bank's own onboarding; there is no customer-facing way to link an account.
type Store struct {
	Pool *db.Pool
}

// Create adds an active customer with a development passphrase. The passphrase is
// hashed with the same argon2id parameters as operator passwords.
func (s *Store) Create(ctx context.Context, tenant uuid.UUID, displayName, passphrase string) (Customer, error) {
	hash, err := identity.HashPassword(passphrase)
	if err != nil {
		return Customer{}, err
	}
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return Customer{}, err
	}
	c := Customer{Ref: "cust_" + hex.EncodeToString(b), DisplayName: displayName}
	err = s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO customers (tenant_id, customer_ref, display_name, password_hash) VALUES ($1,$2,$3,$4) RETURNING id`,
			tenant, c.Ref, displayName, hash).Scan(&c.ID)
	})
	return c, err
}

// LinkAccount records that a customer owns an account token. tail is the last four
// digits of the account number as the bank holds them (empty if not supplied); it lets
// a report that quotes an account tail be checked against the customer's own accounts.
// by says who established the link, for example "bank onboarding".
func (s *Store) LinkAccount(ctx context.Context, tenant, customerID uuid.UUID, accountToken, tail, by string) error {
	if tail != "" && !tailRe.MatchString(tail) {
		return errors.New("customer: the account tail must be four digits")
	}
	return s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO customer_accounts (tenant_id, customer_id, account_token, tail, linked_by) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
			tenant, customerID, accountToken, tail, by)
		return err
	})
}

var tailRe = regexp.MustCompile(`^[0-9]{4}$`)

// Account is an account a customer owns.
type Account struct {
	Token string
	Tail  string
}

// OwnedAccounts returns the customer's accounts with their tails.
func OwnedAccounts(ctx context.Context, tx pgx.Tx, tenant, customerID uuid.UUID) ([]Account, error) {
	rows, err := tx.Query(ctx, `SELECT account_token, tail FROM customer_accounts WHERE tenant_id = $1 AND customer_id = $2 ORDER BY account_token`, tenant, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.Token, &a.Tail); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Accounts returns the account tokens a customer owns.
func Accounts(ctx context.Context, tx pgx.Tx, tenant, customerID uuid.UUID) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT account_token FROM customer_accounts WHERE tenant_id = $1 AND customer_id = $2 ORDER BY account_token`, tenant, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Owns reports whether the customer owns the account token.
func Owns(ctx context.Context, tx pgx.Tx, tenant, customerID uuid.UUID, accountToken string) (bool, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM customer_accounts WHERE tenant_id = $1 AND customer_id = $2 AND account_token = $3`,
		tenant, customerID, accountToken).Scan(&n)
	return n > 0, err
}

// Get returns a customer of the tenant.
func Get(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (Customer, error) {
	var c Customer
	err := tx.QueryRow(ctx, `SELECT id, customer_ref, display_name FROM customers WHERE tenant_id = $1 AND id = $2 AND status = 'active'`, tenant, id).
		Scan(&c.ID, &c.Ref, &c.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// Provider authenticates a customer. It returns ErrInvalidCredentials for every
// failure so a caller cannot learn which customers exist.
type Provider interface {
	Authenticate(ctx context.Context, institution, customerRef, secret string) (tenant uuid.UUID, c Customer, err error)
}

// DevProvider checks a development passphrase. Failed attempts lock the customer for
// a time, and a locked, disabled or unknown customer costs the same time as a wrong
// passphrase and is not counted, so an attacker cannot extend someone else's lock.
type DevProvider struct {
	Pool  *db.Pool
	Audit *audit.Appender
	Now   func() time.Time
}

func (p *DevProvider) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

// Authenticate implements Provider.
func (p *DevProvider) Authenticate(ctx context.Context, institution, customerRef, secret string) (uuid.UUID, Customer, error) {
	var tenant uuid.UUID
	if err := p.Pool.QueryRow(ctx, `SELECT institution_id_by_code($1)`, institution).Scan(&tenant); err != nil || tenant == uuid.Nil {
		burn(secret)
		return uuid.Nil, Customer{}, ErrInvalidCredentials
	}
	customerRef = strings.TrimSpace(customerRef)
	now := p.now()
	var c Customer
	var hash, status string
	var locked *time.Time
	err := p.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id, customer_ref, display_name, password_hash, status, locked_until FROM customers WHERE tenant_id = $1 AND customer_ref = $2`,
			tenant, customerRef).Scan(&c.ID, &c.Ref, &c.DisplayName, &hash, &status, &locked)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		burn(secret)
		return uuid.Nil, Customer{}, ErrInvalidCredentials
	}
	if err != nil {
		return uuid.Nil, Customer{}, err
	}
	if status != "active" || (locked != nil && now.Before(*locked)) || hash == "" {
		burn(secret)
		return uuid.Nil, Customer{}, ErrInvalidCredentials
	}
	ok, _, verr := identity.VerifyPassword(secret, hash)
	if verr != nil || !ok {
		err := p.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE customers SET failed_attempts = failed_attempts + 1,
				locked_until = CASE WHEN failed_attempts + 1 >= $3 THEN $2::timestamptz + make_interval(secs => $4) ELSE locked_until END
				WHERE tenant_id = $1 AND id = $5`, tenant, now, maxAttempts, lockFor.Seconds(), c.ID)
			return err
		})
		if err != nil {
			return uuid.Nil, Customer{}, err
		}
		return uuid.Nil, Customer{}, ErrInvalidCredentials
	}
	// A sign-in that cannot be audited does not happen.
	err = p.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE customers SET failed_attempts = 0, locked_until = NULL WHERE tenant_id = $1 AND id = $2`, tenant, c.ID); err != nil {
			return err
		}
		if p.Audit == nil {
			return nil
		}
		_, err := p.Audit.Append(ctx, tx, tenant, audit.Event{Actor: audit.ActorUser, ActorID: c.ID, Action: "customer.login", SubjectType: "customer",
			SubjectID: c.Ref, Details: map[string]any{"provider": "development"}})
		return err
	})
	if err != nil {
		return uuid.Nil, Customer{}, fmt.Errorf("customer: sign-in not recorded: %w", err)
	}
	return tenant, c, nil
}

// burn spends about the time of a real check so a missing customer is not faster.
func burn(secret string) {
	h, err := identity.HashPassword("burn-" + strings.Repeat("x", 12))
	if err == nil {
		_, _, _ = identity.VerifyPassword(secret, h)
	}
}

// Login issues a customer-audience token for an authenticated customer.
type Login struct {
	Provider Provider
	Signer   *identity.Signer
}

// Result of a sign-in.
type Result struct {
	Token     string
	ExpiresAt time.Time
	Customer  Customer
	TenantID  uuid.UUID
}

// SignIn authenticates and issues a token whose audience is the customer audience.
func (l *Login) SignIn(ctx context.Context, institution, customerRef, secret string, now time.Time) (*Result, error) {
	tenant, c, err := l.Provider.Authenticate(ctx, institution, customerRef, secret)
	if err != nil {
		return nil, err
	}
	tok, exp, err := l.Signer.Issue(identity.Principal{UserID: c.ID, TenantID: tenant, Roles: []string{Role}}, identity.AudienceCustomer, now)
	if err != nil {
		return nil, err
	}
	return &Result{Token: tok, ExpiresAt: exp, Customer: c, TenantID: tenant}, nil
}
