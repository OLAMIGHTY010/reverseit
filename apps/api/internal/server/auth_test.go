package server

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/datashield"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/reversal"
	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

const testPassword = "a long enough passphrase"

// memStore is an in-memory identity.UserStore for handler tests.
type memStore struct {
	tenant uuid.UUID
	user   *identity.User
}

func (m *memStore) TenantIDByCode(_ context.Context, code string) (uuid.UUID, error) {
	if code != "DEMO-BANK" {
		return uuid.Nil, identity.ErrNotFound
	}
	return m.tenant, nil
}
func (m *memStore) FindByEmail(_ context.Context, _ uuid.UUID, email string) (*identity.User, error) {
	if identity.NormalizeEmail(email) != m.user.Email {
		return nil, identity.ErrNotFound
	}
	u := *m.user
	return &u, nil
}
func (m *memStore) RecordFailure(context.Context, uuid.UUID, uuid.UUID, time.Time, int, time.Duration) error {
	return nil
}
func (m *memStore) RecordSuccess(context.Context, uuid.UUID, uuid.UUID, string) error { return nil }

type authRig struct {
	handler  http.Handler
	signer   *identity.Signer
	verifier *identity.Verifier
	user     *identity.User
}

func newAuthRig(t *testing.T) *authRig {
	t.Helper()
	priv, _, err := identity.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := identity.NewSigner("k1", priv, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ks, _ := identity.ParseKeySet("", map[string]ed25519.PublicKey{"k1": signer.PublicKey()})
	verifier := identity.NewVerifier(ks, nil)

	hash, err := identity.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	tenant := uuid.New()
	user := &identity.User{ID: uuid.New(), TenantID: tenant, Email: "officer@example.test", PasswordHash: hash,
		Status: "active", Roles: []string{"REVERSAL_OFFICER"}}

	logger, _ := quietLogger()
	return &authRig{
		signer: signer, verifier: verifier, user: user,
		handler: NewRouter(Options{
			Logger: logger, Verifier: verifier,
			Auth: &identity.Authenticator{Store: &memStore{tenant: tenant, user: user}, Signer: signer},
			// Mounted so the route-policy test below walks the data routes too.
			Services:        &Services{Pool: &db.Pool{}, Policy: &authz.Policy{}, Audit: &audit.Appender{}, Vault: &datashield.Vault{}, Config: &configsvc.Service{}, KillSwitch: &killswitch.Service{}, Txn: &txn.Service{}, Cases: &cases.Service{}, Reversal: &reversal.Service{}},
			ServiceVerifier: &servauth.Verifier{},
		}),
	}
}

func (r *authRig) token(t *testing.T, audience string, at time.Time) string {
	t.Helper()
	tok, _, err := r.signer.Issue(identity.Principal{UserID: r.user.ID, TenantID: r.user.TenantID, Roles: r.user.Roles}, audience, at)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func get(h http.Handler, path, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestProtectedRouteRejectsBadCredentials(t *testing.T) {
	r := newAuthRig(t)
	good := r.token(t, identity.AudienceOps, time.Now())

	cases := map[string]string{
		"no header":         "",
		"wrong scheme":      "Basic " + good,
		"scheme only":       "Bearer",
		"empty token":       "Bearer ",
		"token with space":  "Bearer " + good + " extra",
		"garbage":           "Bearer not.a.token",
		"customer audience": "Bearer " + r.token(t, identity.AudienceCustomer, time.Now()),
		"expired":           "Bearer " + r.token(t, identity.AudienceOps, time.Now().Add(-time.Hour)),
	}
	for name, h := range cases {
		rec := get(r.handler, "/v1/me", h)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, rec.Code)
		}
		if rec.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s: no WWW-Authenticate header", name)
		}
		if !strings.Contains(rec.Body.String(), `"error":"UNAUTHORIZED"`) {
			t.Errorf("%s: body %s", name, rec.Body.String())
		}
	}
}

func TestMeReturnsTheCallersIdentity(t *testing.T) {
	r := newAuthRig(t)
	rec := get(r.handler, "/v1/me", "bearer "+r.token(t, identity.AudienceOps, time.Now()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body meResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.UserID != r.user.ID.String() || body.TenantID != r.user.TenantID.String() ||
		len(body.Roles) != 1 || body.Roles[0] != "REVERSAL_OFFICER" || body.MFA {
		t.Fatalf("unexpected body: %+v", body)
	}
}

// Every route except the explicit public list must refuse an unauthenticated
// request. The walk covers routes added later, so a new route that forgets the
// auth group fails this test.
func TestEveryNonPublicRouteRequiresAuthentication(t *testing.T) {
	r := newAuthRig(t)
	public := map[string]bool{
		"GET /v1/health":      true,
		"GET /v1/ready":       true,
		"GET /v1/metrics":     true,
		"POST /v1/auth/login": true,
	}
	routes, ok := r.handler.(chi.Routes)
	if !ok {
		t.Fatal("router does not expose routes")
	}
	seen := 0
	err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		seen++
		if public[method+" "+route] {
			return nil
		}
		path := strings.NewReplacer("{id}", "x", "{reference}", "x", "{key}", "duplicate.window_seconds", "{version}", "1").Replace(route)
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		r.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s answered %d without a token, want 401", method, route, rec.Code)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 34 {
		t.Fatalf("walk found only %d routes", seen)
	}
}

func TestProtectedRoutesFailClosedWithoutAVerifier(t *testing.T) {
	logger, _ := quietLogger()
	h := NewRouter(Options{Logger: logger}) // identity not configured
	r := newAuthRig(t)
	rec := get(h, "/v1/me", "Bearer "+r.token(t, identity.AudienceOps, time.Now()))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
	if rec = get(h, "/v1/health", ""); rec.Code != http.StatusOK {
		t.Fatalf("health must stay public: %d", rec.Code)
	}
}

func TestLoginRouteIsAbsentWhenIdentityIsNotConfigured(t *testing.T) {
	logger, _ := quietLogger()
	h := NewRouter(Options{Logger: logger})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("login answered %d with no authenticator", rec.Code)
	}
}

func postLogin(h http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLoginIssuesATokenThatWorksOnProtectedRoutes(t *testing.T) {
	r := newAuthRig(t)
	rec := postLogin(r.handler, `{"institution":"DEMO-BANK","email":"Officer@example.test","password":"`+testPassword+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("login response is cacheable")
	}
	var body loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TokenType != "Bearer" || body.AccessToken == "" || body.UserID != r.user.ID.String() {
		t.Fatalf("unexpected body: %+v", body)
	}
	if rec := get(r.handler, "/v1/me", "Bearer "+body.AccessToken); rec.Code != http.StatusOK {
		t.Fatalf("issued token rejected: %d", rec.Code)
	}
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	r := newAuthRig(t)
	bodies := []string{
		`{"institution":"DEMO-BANK","email":"officer@example.test","password":"the wrong password"}`,
		`{"institution":"DEMO-BANK","email":"nobody@example.test","password":"` + testPassword + `"}`,
		`{"institution":"OTHER-BANK","email":"officer@example.test","password":"` + testPassword + `"}`,
	}
	var first string
	for i, b := range bodies {
		rec := postLogin(r.handler, b)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("case %d: status %d", i, rec.Code)
		}
		var e struct{ Error, Message string }
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		sig := e.Error + "|" + e.Message
		if i == 0 {
			first = sig
		} else if sig != first {
			t.Errorf("case %d answers %q, differs from %q", i, sig, first)
		}
		if strings.Contains(rec.Body.String(), "access_token") {
			t.Errorf("case %d returned a token", i)
		}
	}
}

func TestLoginRejectsMalformedRequests(t *testing.T) {
	r := newAuthRig(t)
	cases := map[string]struct {
		body string
		want int
	}{
		"not json":      {`nope`, http.StatusBadRequest},
		"empty":         {``, http.StatusBadRequest},
		"unknown field": {`{"institution":"A","email":"b","password":"c","admin":true}`, http.StatusBadRequest},
		"two objects":   {`{} {}`, http.StatusBadRequest},
		"missing field": {`{"institution":"DEMO-BANK","email":"officer@example.test"}`, http.StatusUnprocessableEntity},
		"empty fields":  {`{"institution":"","email":"","password":""}`, http.StatusUnprocessableEntity},
	}
	for name, c := range cases {
		if rec := postLogin(r.handler, c.body); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d", name, rec.Code, c.want)
		}
	}
}
