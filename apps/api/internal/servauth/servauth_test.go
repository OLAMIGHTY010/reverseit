package servauth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type rig struct {
	pool    *db.Pool
	reg     *Registry
	ver     *Verifier
	tenant  uuid.UUID
	admin   uuid.UUID
	now     time.Time
	pub     ed25519.PublicKey
	priv    ed25519.PrivateKey
	service uuid.UUID
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), admin: uuid.New(), now: time.Now().UTC().Truncate(time.Second)}
	r.reg = &Registry{Pool: pool, Audit: &audit.Appender{}, Now: func() time.Time { return r.now }}
	r.ver = &Verifier{Pool: pool, Now: func() time.Time { return r.now }}
	var err error
	r.pub, r.priv, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	r.service, err = r.reg.Register(context.Background(), r.tenant, r.admin, "simulator", r.pub, []string{"events:write", "status:read"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *rig) assertion(t *testing.T) string {
	t.Helper()
	tok, err := NewAssertion(r.tenant, r.service, r.priv, r.now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestAServiceAuthenticatesWithItsOwnKeyAndScope(t *testing.T) {
	r := newRig(t)
	p, err := r.ver.Verify(context.Background(), r.assertion(t), "events:write")
	if err != nil {
		t.Fatal(err)
	}
	if p.ServiceID != r.service || p.TenantID != r.tenant || p.Name != "simulator" || !p.Has("events:write") || !p.Has("status:read") || p.Has("reversal:submit") {
		t.Fatalf("principal: %+v", p)
	}
}

func TestAnAssertionWorksOnce(t *testing.T) {
	r := newRig(t)
	tok := r.assertion(t)
	if _, err := r.ver.Verify(context.Background(), tok, "events:write"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ver.Verify(context.Background(), tok, "events:write"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("replay accepted: %v", err)
	}
	// A fresh assertion is fine.
	if _, err := r.ver.Verify(context.Background(), r.assertion(t), "events:write"); err != nil {
		t.Fatalf("a new assertion was refused: %v", err)
	}
}

func TestAFailedCheckDoesNotUseUpTheAssertion(t *testing.T) {
	r := newRig(t)
	tok := r.assertion(t)
	if _, err := r.ver.Verify(context.Background(), tok, "reversal:submit"); !errors.Is(err, ErrInvalid) {
		t.Fatal("a scope the service lacks was accepted")
	}
	if _, err := r.ver.Verify(context.Background(), tok, "events:write"); err != nil {
		t.Fatalf("the assertion was consumed by a refused check: %v", err)
	}
}

func TestTheServiceCannotClaimAScopeItWasNotGiven(t *testing.T) {
	r := newRig(t)
	for _, scope := range []string{"reversal:submit", "events:read", "", "events:write "} {
		if _, err := r.ver.Verify(context.Background(), r.assertion(t), scope); !errors.Is(err, ErrInvalid) {
			t.Errorf("scope %q: %v", scope, err)
		}
	}
}

func TestExpiredAndFutureAssertionsAreRefused(t *testing.T) {
	r := newRig(t)
	old, _ := NewAssertion(r.tenant, r.service, r.priv, r.now.Add(-time.Hour), time.Minute)
	if _, err := r.ver.Verify(context.Background(), old, "events:write"); !errors.Is(err, ErrInvalid) {
		t.Errorf("expired: %v", err)
	}
	future, _ := NewAssertion(r.tenant, r.service, r.priv, r.now.Add(time.Hour), time.Minute)
	if _, err := r.ver.Verify(context.Background(), future, "events:write"); !errors.Is(err, ErrInvalid) {
		t.Errorf("not yet valid: %v", err)
	}
	if _, err := NewAssertion(r.tenant, r.service, r.priv, r.now, MaxLifetime+time.Second); err == nil {
		t.Error("an over-long assertion could be created")
	}
	if _, err := NewAssertion(r.tenant, r.service, r.priv, r.now, 0); err == nil {
		t.Error("a zero-length assertion could be created")
	}
}

func rawClaims(tenant, service uuid.UUID, now time.Time) claims {
	return claims{TenantID: tenant.String(), RegisteredClaims: jwt.RegisteredClaims{
		Issuer: service.String(), Subject: service.String(), Audience: jwt.ClaimStrings{Audience},
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)), ID: uuid.NewString()}}
}

func sign(t *testing.T, method jwt.SigningMethod, key any, kid string, c claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, c)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestForgedAndMalformedAssertionsAreRefused(t *testing.T) {
	r := newRig(t)
	_, otherKey, _ := ed25519.GenerateKey(rand.Reader)
	good := rawClaims(r.tenant, r.service, r.now)

	mut := func(f func(*claims)) claims { c := rawClaims(r.tenant, r.service, r.now); f(&c); return c }
	cases := map[string]string{
		"signed by another key":          sign(t, jwt.SigningMethodEdDSA, otherKey, r.service.String(), good),
		"wrong audience":                 sign(t, jwt.SigningMethodEdDSA, r.priv, r.service.String(), mut(func(c *claims) { c.Audience = jwt.ClaimStrings{"smartreversal-ops"} })),
		"operator audience":              sign(t, jwt.SigningMethodEdDSA, r.priv, r.service.String(), mut(func(c *claims) { c.Audience = jwt.ClaimStrings{"smartreversal-ops"} })),
		"issuer is someone else":         sign(t, jwt.SigningMethodEdDSA, r.priv, r.service.String(), mut(func(c *claims) { c.Issuer = uuid.NewString() })),
		"kid names another service":      sign(t, jwt.SigningMethodEdDSA, r.priv, uuid.NewString(), good),
		"no kid":                         sign(t, jwt.SigningMethodEdDSA, r.priv, "", good),
		"no expiry":                      sign(t, jwt.SigningMethodEdDSA, r.priv, r.service.String(), mut(func(c *claims) { c.ExpiresAt = nil })),
		"lifetime over the limit":        sign(t, jwt.SigningMethodEdDSA, r.priv, r.service.String(), mut(func(c *claims) { c.ExpiresAt = jwt.NewNumericDate(r.now.Add(time.Hour)) })),
		"no issued-at":                   sign(t, jwt.SigningMethodEdDSA, r.priv, r.service.String(), mut(func(c *claims) { c.IssuedAt = nil })),
		"short jti":                      sign(t, jwt.SigningMethodEdDSA, r.priv, r.service.String(), mut(func(c *claims) { c.ID = "x" })),
		"no jti":                         sign(t, jwt.SigningMethodEdDSA, r.priv, r.service.String(), mut(func(c *claims) { c.ID = "" })),
		"HMAC keyed with the public key": sign(t, jwt.SigningMethodHS256, []byte(r.pub), r.service.String(), good),
		"unsigned":                       sign(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, r.service.String(), good),
		"garbage":                        "not.a.token",
		"empty":                          "",
		"huge":                           strings.Repeat("a", 3000),
	}
	for name, tok := range cases {
		if _, err := r.ver.Verify(context.Background(), tok, "events:write"); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := r.ver.Verify(context.Background(), sign(t, jwt.SigningMethodEdDSA, r.priv, r.service.String(), good), "events:write"); err != nil {
		t.Fatalf("the well-formed control was refused: %v", err)
	}
}

func TestAServiceCannotActInAnotherTenant(t *testing.T) {
	a, b := newRig(t), newRig(t)
	// Service A claims tenant B. B has no such service id, so there is no key to check against.
	tok, _ := NewAssertion(b.tenant, a.service, a.priv, a.now, time.Minute)
	if _, err := b.ver.Verify(context.Background(), tok, "events:write"); !errors.Is(err, ErrInvalid) {
		t.Errorf("service A authenticated in tenant B: %v", err)
	}
	// Service A's key under tenant B's service id fails the signature check.
	tok2, _ := NewAssertion(b.tenant, b.service, a.priv, a.now, time.Minute)
	if _, err := b.ver.Verify(context.Background(), tok2, "events:write"); !errors.Is(err, ErrInvalid) {
		t.Errorf("tenant A's key was accepted for tenant B's service: %v", err)
	}
	// Unknown service ids and tenants.
	tok3, _ := NewAssertion(uuid.New(), uuid.New(), a.priv, a.now, time.Minute)
	if _, err := a.ver.Verify(context.Background(), tok3, "events:write"); !errors.Is(err, ErrInvalid) {
		t.Errorf("unknown service: %v", err)
	}
}

func TestDisablingAnAccountStopsItImmediately(t *testing.T) {
	r := newRig(t)
	if _, err := r.ver.Verify(context.Background(), r.assertion(t), "events:write"); err != nil {
		t.Fatal(err)
	}
	if err := r.reg.Disable(context.Background(), r.tenant, r.admin, r.service); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ver.Verify(context.Background(), r.assertion(t), "events:write"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a disabled service authenticated: %v", err)
	}
	if err := r.reg.Disable(context.Background(), r.tenant, r.admin, r.service); err == nil {
		t.Error("disabling twice succeeded")
	}
}

func TestRegistrationIsValidatedAndAudited(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	for name, c := range map[string]struct {
		name   string
		key    ed25519.PublicKey
		scopes []string
	}{
		"upper case name": {"Simulator", pub, []string{"a_b:c_d"}},
		"name with space": {"my service", pub, []string{"events:write"}},
		"short key":       {"svc-one", pub[:10], []string{"events:write"}},
		"no scopes":       {"svc-one", pub, nil},
		"bad scope":       {"svc-one", pub, []string{"everything"}},
		"scope with caps": {"svc-one", pub, []string{"Events:Write"}},
		"too many scopes": {"svc-one", pub, make([]string, 21)},
		"duplicate name":  {"simulator", pub, []string{"events:write"}},
	} {
		if _, err := r.reg.Register(ctx, r.tenant, r.admin, c.name, c.key, c.scopes); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := r.reg.Register(ctx, r.tenant, uuid.Nil, "svc-two", pub, []string{"events:write"}); err == nil {
		t.Error("registration without an actor accepted")
	}

	var actions []string
	var res audit.VerifyResult
	_ = r.pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error {
		recs, _ := audit.List(ctx, tx, r.tenant, audit.Filter{SubjectType: "service_account"}, 0, 20)
		for _, rec := range recs {
			actions = append(actions, rec.Action)
		}
		res, _ = audit.Verify(ctx, tx, r.tenant)
		return nil
	})
	if len(actions) != 1 || actions[0] != "service_account.registered" || !res.OK {
		t.Fatalf("audit: %v %+v", actions, res)
	}
}

func TestExpiredAssertionRecordsAreCleanedUp(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := r.ver.Verify(ctx, r.assertion(t), "events:write"); err != nil {
			t.Fatal(err)
		}
	}
	r.now = r.now.Add(10 * time.Minute)
	if _, err := r.ver.Verify(ctx, r.assertion(t), "events:write"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM service_assertion_uses WHERE tenant_id = $1`, r.tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d assertion records remain, want 1", n)
	}
}

func TestPrincipalContextHelpers(t *testing.T) {
	if FromContext(context.Background()) != nil {
		t.Fatal("empty context has a principal")
	}
	p := &Principal{Name: "x", Scopes: []string{"a:b"}}
	if got := FromContext(WithPrincipal(context.Background(), p)); got != p || !got.Has("a:b") || got.Has("a:c") {
		t.Fatal("round trip failed")
	}
}
