package identity

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	// Issuer is the "iss" claim of every token.
	Issuer = "smartreversal-api"
	// AudienceOps is for operators: analysts, officers, supervisors, admins.
	AudienceOps = "smartreversal-ops"
	// AudienceCustomer is for ReverseIt customers. A customer token never
	// passes an operator route, and an operator token never passes a customer
	// route (ADR 0003, D8).
	AudienceCustomer = "smartreversal-customer"

	// MaxTokenLifetime bounds how long any token may live, whatever the
	// issuer was configured with.
	MaxTokenLifetime = 24 * time.Hour

	leeway   = 30 * time.Second
	maxRoles = 20
)

var rolePattern = regexp.MustCompile(`^[A-Z_]{3,40}$`)

// Principal is who is calling. It is built only from a verified token.
type Principal struct {
	UserID    uuid.UUID
	TenantID  uuid.UUID
	Roles     []string
	MFA       bool
	Audience  string
	TokenID   string
	ExpiresAt time.Time
}

// HasRole reports whether the principal holds role.
func (p *Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type claims struct {
	TenantID string   `json:"tid"`
	Roles    []string `json:"roles"`
	MFA      bool     `json:"mfa,omitempty"`
	jwt.RegisteredClaims
}

// Issue signs a token for p. The audience must be one of the two known values.
// ExpiresAt, TokenID and Audience on p are ignored for the claims they set:
// the signer decides lifetime and id.
func (s *Signer) Issue(p Principal, audience string, now time.Time) (string, time.Time, error) {
	if audience != AudienceOps && audience != AudienceCustomer {
		return "", time.Time{}, errors.New("identity: unknown audience")
	}
	if p.UserID == uuid.Nil || p.TenantID == uuid.Nil {
		return "", time.Time{}, errors.New("identity: user id and tenant id are required")
	}
	if err := validateRoles(p.Roles); err != nil {
		return "", time.Time{}, err
	}
	exp := now.Add(s.ttl)
	c := claims{
		TenantID: p.TenantID.String(),
		Roles:    p.Roles,
		MFA:      p.MFA,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   p.UserID.String(),
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        uuid.NewString(),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
	tok.Header["kid"] = s.kid
	signed, err := tok.SignedString(s.key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("identity: sign: %w", err)
	}
	return signed, exp, nil
}

func validateRoles(roles []string) error {
	if len(roles) > maxRoles {
		return errors.New("identity: too many roles")
	}
	for _, r := range roles {
		if !rolePattern.MatchString(r) {
			return errors.New("identity: invalid role name")
		}
	}
	return nil
}

// Verifier checks tokens against a KeySet.
type Verifier struct {
	keys *KeySet
	now  func() time.Time
}

// NewVerifier returns a Verifier. now may be nil to use the system clock.
func NewVerifier(keys *KeySet, now func() time.Time) *Verifier {
	if now == nil {
		now = time.Now
	}
	return &Verifier{keys: keys, now: now}
}

// ErrInvalidToken is returned for every verification failure. The reason is
// deliberately not exposed to callers, so responses do not help an attacker.
var ErrInvalidToken = errors.New("identity: invalid token")

// Verify checks signature, key id, algorithm, issuer, audience, expiry, not
// before, lifetime and claim formats. wantAudience is the audience the route
// accepts.
func (v *Verifier) Verify(raw, wantAudience string) (*Principal, error) {
	var c claims
	tok, err := jwt.ParseWithClaims(raw, &c,
		func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			key, ok := v.keys.get(kid)
			if !ok {
				return nil, errors.New("unknown key id")
			}
			return key, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(Issuer),
		jwt.WithAudience(wantAudience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(leeway),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil || !tok.Valid {
		return nil, ErrInvalidToken
	}

	if c.IssuedAt == nil || c.NotBefore == nil || c.ExpiresAt.Time.Sub(c.IssuedAt.Time) > MaxTokenLifetime {
		return nil, ErrInvalidToken
	}
	user, err := uuid.Parse(c.Subject)
	if err != nil || user == uuid.Nil {
		return nil, ErrInvalidToken
	}
	tenant, err := uuid.Parse(c.TenantID)
	if err != nil || tenant == uuid.Nil {
		return nil, ErrInvalidToken
	}
	if validateRoles(c.Roles) != nil || c.ID == "" {
		return nil, ErrInvalidToken
	}
	return &Principal{
		UserID:    user,
		TenantID:  tenant,
		Roles:     c.Roles,
		MFA:       c.MFA,
		Audience:  wantAudience,
		TokenID:   c.ID,
		ExpiresAt: c.ExpiresAt.Time,
	}, nil
}

type principalKey struct{}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext returns the principal stored in ctx, or nil.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}
