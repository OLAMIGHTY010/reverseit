// Package servauth authenticates other services calling the API.
//
// A service signs a short-lived assertion with its own Ed25519 key and sends it
// as a bearer token. The API looks up the service's public key in the tenant named
// by the assertion, checks signature, audience, lifetime and scope, and records
// the assertion id so it cannot be used twice. The token proves who is calling;
// what they may do comes from the scopes stored with the account.
package servauth

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
)

const (
	// Audience is the "aud" claim of a service assertion. It differs from the
	// operator and customer audiences, so the three token kinds never mix.
	Audience = "smartreversal-service"
	// MaxLifetime is the longest an assertion may be valid.
	MaxLifetime = 5 * time.Minute

	leeway = 15 * time.Second
)

var (
	namePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{1,40}$`)
	scopePattern = regexp.MustCompile(`^[a-z][a-z_]{1,30}:[a-z][a-z_]{1,30}$`)
)

// ErrInvalid is returned for every authentication failure. The reason is not
// exposed to callers.
var ErrInvalid = errors.New("servauth: invalid service credentials")

// Principal is an authenticated calling service.
type Principal struct {
	ServiceID uuid.UUID
	TenantID  uuid.UUID
	Name      string
	Scopes    []string
}

// Has reports whether the service holds scope.
func (p *Principal) Has(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

type principalKey struct{}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext returns the service principal in ctx, or nil.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}

type claims struct {
	TenantID string `json:"tid"`
	jwt.RegisteredClaims
}

// NewAssertion signs an assertion for a service. It is what the calling side runs.
func NewAssertion(tenant, service uuid.UUID, key ed25519.PrivateKey, now time.Time, ttl time.Duration) (string, error) {
	if ttl <= 0 || ttl > MaxLifetime {
		return "", fmt.Errorf("servauth: lifetime must be between 1s and %s", MaxLifetime)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims{
		TenantID: tenant.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: service.String(), Subject: service.String(), Audience: jwt.ClaimStrings{Audience},
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)), ID: uuid.NewString(),
		},
	})
	tok.Header["kid"] = service.String()
	return tok.SignedString(key)
}

// Registry manages service accounts.
type Registry struct {
	Pool  *db.Pool
	Audit *audit.Appender
	Now   func() time.Time
}

func (g *Registry) now() time.Time {
	if g.Now != nil {
		return g.Now().UTC()
	}
	return time.Now().UTC()
}

// Register creates an active service account and returns its id. The scopes look
// like "events:write".
func (g *Registry) Register(ctx context.Context, tenant, actor uuid.UUID, name string, pub ed25519.PublicKey, scopes []string) (uuid.UUID, error) {
	switch {
	case !namePattern.MatchString(name):
		return uuid.Nil, errors.New("servauth: name must be lower case letters, digits and dashes")
	case len(pub) != ed25519.PublicKeySize:
		return uuid.Nil, errors.New("servauth: public key must be an Ed25519 key")
	case len(scopes) == 0 || len(scopes) > 20:
		return uuid.Nil, errors.New("servauth: between 1 and 20 scopes are required")
	case actor == uuid.Nil:
		return uuid.Nil, errors.New("servauth: actor is required")
	}
	for _, s := range scopes {
		if !scopePattern.MatchString(s) {
			return uuid.Nil, fmt.Errorf("servauth: scope %q must look like resource:action", s)
		}
	}
	id := uuid.New()
	err := g.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO service_accounts (tenant_id, id, name, public_key, scopes, created_by, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`, tenant, id, name, []byte(pub), scopes, actor, g.now()); err != nil {
			return fmt.Errorf("servauth: register: %w", err)
		}
		_, err := g.Audit.Append(ctx, tx, tenant, audit.Event{
			Actor: audit.ActorUser, ActorID: actor, Action: "service_account.registered",
			SubjectType: "service_account", SubjectID: id.String(),
			Details: map[string]any{"name": name, "scopes": scopes},
		})
		return err
	})
	return id, err
}

// Disable turns an account off. Its assertions stop working at once.
func (g *Registry) Disable(ctx context.Context, tenant, actor, id uuid.UUID) error {
	return g.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE service_accounts SET status = 'disabled' WHERE tenant_id = $1 AND id = $2 AND status = 'active'`, tenant, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("servauth: no active service account with that id")
		}
		_, err = g.Audit.Append(ctx, tx, tenant, audit.Event{
			Actor: audit.ActorUser, ActorID: actor, Action: "service_account.disabled",
			SubjectType: "service_account", SubjectID: id.String(),
		})
		return err
	})
}

// Verifier checks assertions.
type Verifier struct {
	Pool *db.Pool
	Now  func() time.Time
}

func (v *Verifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

// Verify authenticates raw and checks that the service holds wantScope.
func (v *Verifier) Verify(ctx context.Context, raw, wantScope string) (*Principal, error) {
	if raw == "" || len(raw) > 2048 {
		return nil, ErrInvalid
	}
	// Read the claimed tenant and service without trusting them, only to know
	// where to look up the key. Nothing is accepted until the signature checks
	// out against the key stored for exactly that tenant and service.
	var unverified claims
	if _, _, err := jwt.NewParser().ParseUnverified(raw, &unverified); err != nil {
		return nil, ErrInvalid
	}
	tenant, err := uuid.Parse(unverified.TenantID)
	if err != nil || tenant == uuid.Nil {
		return nil, ErrInvalid
	}
	service, err := uuid.Parse(unverified.Subject)
	if err != nil || service == uuid.Nil {
		return nil, ErrInvalid
	}

	var out *Principal
	err = v.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var name, status string
		var pub []byte
		var scopes []string
		err := tx.QueryRow(ctx, `SELECT name, public_key, scopes, status FROM service_accounts WHERE tenant_id = $1 AND id = $2`,
			tenant, service).Scan(&name, &pub, &scopes, &status)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != "active") {
			return ErrInvalid
		}
		if err != nil {
			return err
		}

		var c claims
		tok, err := jwt.ParseWithClaims(raw, &c,
			func(t *jwt.Token) (any, error) {
				if kid, _ := t.Header["kid"].(string); kid != service.String() {
					return nil, errors.New("key id does not match the service")
				}
				return ed25519.PublicKey(pub), nil
			},
			jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
			jwt.WithAudience(Audience), jwt.WithIssuer(service.String()),
			jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(leeway),
			jwt.WithTimeFunc(v.now),
		)
		if err != nil || !tok.Valid {
			return ErrInvalid
		}
		if c.IssuedAt == nil || c.NotBefore == nil || c.ExpiresAt.Sub(c.IssuedAt.Time) > MaxLifetime ||
			c.Subject != service.String() || c.TenantID != tenant.String() || len(c.ID) < 8 || len(c.ID) > 64 {
			return ErrInvalid
		}

		held := false
		for _, s := range scopes {
			if s == wantScope {
				held = true
			}
		}
		if !held {
			return ErrInvalid
		}

		// An assertion is good once. Rows for expired assertions are cleared on
		// the way, so the table stays small.
		if _, err := tx.Exec(ctx, `DELETE FROM service_assertion_uses WHERE tenant_id = $1 AND expires_at < $2`, tenant, v.now().Add(-time.Minute)); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO service_assertion_uses (tenant_id, service_id, jti, expires_at)
			VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, tenant, service, c.ID, c.ExpiresAt.Time.UTC().Add(leeway))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrInvalid // replay
		}
		out = &Principal{ServiceID: service, TenantID: tenant, Name: name, Scopes: scopes}
		return nil
	})
	if errors.Is(err, ErrInvalid) {
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("servauth: verify: %w", err)
	}
	return out, nil
}
