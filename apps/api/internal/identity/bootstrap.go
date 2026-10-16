package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

// BootstrapSpec describes the first institution and operator of an environment.
type BootstrapSpec struct {
	InstitutionCode string
	InstitutionName string
	Email           string
	DisplayName     string
	Password        string
	Roles           []string
}

// Bootstrap creates an institution if it does not exist and one operator in it.
// It uses the pool's own role to create the institution, because no tenant
// exists yet to run under. It is meant for setup and the demo, not for a
// request path. Normal user management needs dual control and arrives later.
func Bootstrap(ctx context.Context, pool *db.Pool, spec BootstrapSpec) (tenant, user uuid.UUID, err error) {
	if spec.InstitutionCode == "" || spec.Email == "" || spec.Password == "" {
		return uuid.Nil, uuid.Nil, errors.New("identity: institution code, email and password are required")
	}
	name := spec.InstitutionName
	if name == "" {
		name = spec.InstitutionCode
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO institutions (code, name) VALUES ($1, $2)
		ON CONFLICT (code) DO UPDATE SET code = EXCLUDED.code
		RETURNING id`, spec.InstitutionCode, name).Scan(&tenant); err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("identity: create institution: %w", err)
	}
	display := spec.DisplayName
	if display == "" {
		display = spec.Email
	}
	user, err = (&PGUserStore{Pool: pool}).Create(ctx, tenant, spec.Email, display, spec.Password, spec.Roles)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return tenant, user, nil
}
