package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

// ErrNotFound is returned when a tenant or user does not exist.
var ErrNotFound = errors.New("identity: not found")

// User is an operator account.
type User struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	Email          string
	DisplayName    string
	PasswordHash   string
	Status         string
	FailedAttempts int
	LockedUntil    *time.Time
	Roles          []string
}

// UserStore is where operator accounts live. The PostgreSQL implementation is
// below. An external identity provider would replace it (ADR 0003, D6).
type UserStore interface {
	// TenantIDByCode resolves an institution code. It returns ErrNotFound for
	// an unknown code.
	TenantIDByCode(ctx context.Context, code string) (uuid.UUID, error)
	// FindByEmail returns ErrNotFound for an unknown email.
	FindByEmail(ctx context.Context, tenant uuid.UUID, email string) (*User, error)
	// RecordFailure counts a failed login and locks the account for lockFor
	// once it reaches maxAttempts.
	RecordFailure(ctx context.Context, tenant, user uuid.UUID, now time.Time, maxAttempts int, lockFor time.Duration) error
	// RecordSuccess clears the failure count and lock, and stores newHash when
	// it is not empty.
	RecordSuccess(ctx context.Context, tenant, user uuid.UUID, newHash string) error
}

// PGUserStore keeps users in PostgreSQL under row-level security.
type PGUserStore struct {
	Pool *db.Pool
}

var _ UserStore = (*PGUserStore)(nil)

// NormalizeEmail lower-cases and trims an email address.
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// TenantIDByCode resolves a code through a function that returns only the id.
func (s *PGUserStore) TenantIDByCode(ctx context.Context, code string) (uuid.UUID, error) {
	var id *uuid.UUID
	err := s.Pool.QueryRow(ctx, `SELECT institution_id_by_code($1)`, code).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("identity: resolve institution: %w", err)
	}
	if id == nil {
		return uuid.Nil, ErrNotFound
	}
	return *id, nil
}

// FindByEmail loads a user and their roles inside the tenant.
func (s *PGUserStore) FindByEmail(ctx context.Context, tenant uuid.UUID, email string) (*User, error) {
	var u *User
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		found := User{}
		err := tx.QueryRow(ctx, `
			SELECT id, tenant_id, email, display_name, password_hash, status, failed_attempts, locked_until
			FROM users WHERE email = $1`, NormalizeEmail(email)).
			Scan(&found.ID, &found.TenantID, &found.Email, &found.DisplayName, &found.PasswordHash,
				&found.Status, &found.FailedAttempts, &found.LockedUntil)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT role FROM user_roles WHERE user_id = $1 ORDER BY role`, found.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r string
			if err := rows.Scan(&r); err != nil {
				return err
			}
			found.Roles = append(found.Roles, r)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		u = &found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

// RecordFailure increments the counter in one statement so concurrent failures
// cannot lose a count.
func (s *PGUserStore) RecordFailure(ctx context.Context, tenant, user uuid.UUID, now time.Time, maxAttempts int, lockFor time.Duration) error {
	return s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE users SET
				failed_attempts = failed_attempts + 1,
				locked_until = CASE WHEN failed_attempts + 1 >= $3 THEN $2::timestamptz + make_interval(secs => $4) ELSE locked_until END
			WHERE id = $1`, user, now, maxAttempts, lockFor.Seconds())
		return err
	})
}

// RecordSuccess clears failures and, when newHash is set, upgrades the hash.
func (s *PGUserStore) RecordSuccess(ctx context.Context, tenant, user uuid.UUID, newHash string) error {
	return s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE users SET failed_attempts = 0, locked_until = NULL,
				password_hash = CASE WHEN $2 <> '' THEN $2 ELSE password_hash END
			WHERE id = $1`, user, newHash)
		return err
	})
}

// Create adds an active operator. It is used by setup and test code. Creating
// users through the API needs dual control and arrives with the admin screens.
func (s *PGUserStore) Create(ctx context.Context, tenant uuid.UUID, email, displayName, password string, roles []string) (uuid.UUID, error) {
	if err := validateRoles(roles); err != nil {
		return uuid.Nil, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (tenant_id, email, display_name, password_hash)
			VALUES ($1, $2, $3, $4) RETURNING id`, tenant, NormalizeEmail(email), displayName, hash).Scan(&id); err != nil {
			return err
		}
		for _, r := range roles {
			if _, err := tx.Exec(ctx, `INSERT INTO user_roles (tenant_id, user_id, role) VALUES ($1, $2, $3)`, tenant, id, r); err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}
