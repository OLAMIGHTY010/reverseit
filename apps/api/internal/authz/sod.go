package authz

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

// Segregation of duties (v2 section 6.4). The same person can never request
// and approve, whatever role they hold, and an approver cannot be related to the
// account involved.

// ErrSameActor is returned when the maker and the checker are the same person.
var ErrSameActor = errors.New("authz: the maker cannot also be the checker")

// ErrConflictOfInterest is returned when the approver is linked to the account.
var ErrConflictOfInterest = errors.New("authz: the approver is related to this account")

// Subject describes the item being approved.
type Subject struct {
	// AccountTokens are the tokens of the accounts the item touches.
	AccountTokens []string
}

// ConflictChecker answers whether an approver is related to a subject. It is an
// interface so a bank can plug in its own HR or relationship data.
type ConflictChecker interface {
	Conflicts(ctx context.Context, tenant, approver uuid.UUID, s Subject) (bool, error)
}

// SoD enforces maker-checker separation.
type SoD struct {
	Conflicts ConflictChecker
}

// CanApprove returns nil only when checker may approve something made by maker.
// A failure of the conflict check denies: when the system cannot tell, the
// answer is no.
func (s SoD) CanApprove(ctx context.Context, checker *identity.Principal, maker uuid.UUID, subject Subject) error {
	if checker == nil || checker.UserID == uuid.Nil || maker == uuid.Nil {
		return ErrForbidden
	}
	if checker.UserID == maker {
		return ErrSameActor
	}
	if s.Conflicts == nil {
		return fmt.Errorf("authz: no conflict checker configured: %w", ErrForbidden)
	}
	conflict, err := s.Conflicts.Conflicts(ctx, checker.TenantID, checker.UserID, subject)
	if err != nil {
		return fmt.Errorf("authz: conflict check failed: %w", ErrForbidden)
	}
	if conflict {
		return ErrConflictOfInterest
	}
	return nil
}

// PGConflictChecker looks for a staff link to any of the subject's accounts.
type PGConflictChecker struct {
	Pool *db.Pool
}

var _ ConflictChecker = (*PGConflictChecker)(nil)

// Conflicts reports whether approver is linked to any account in s.
func (c *PGConflictChecker) Conflicts(ctx context.Context, tenant, approver uuid.UUID, s Subject) (bool, error) {
	if len(s.AccountTokens) == 0 {
		return false, nil
	}
	var linked bool
	err := c.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM staff_account_links WHERE user_id = $1 AND account_token = ANY($2))`,
			approver, s.AccountTokens).Scan(&linked)
	})
	return linked, err
}
