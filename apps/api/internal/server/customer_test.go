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

	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/customer"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

const customerPass = "synthetic customer passphrase"

type custRig struct {
	*auditRig
	store *customer.Store
	code  string
}

func newCustRig(t *testing.T, withLogin bool) *custRig {
	t.Helper()
	pool := dbtest.Pool(t)
	policy, err := authz.Load(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	priv, _, _ := identity.GenerateKey()
	signer, _ := identity.NewSigner("k1", priv, 10*time.Minute)
	ks, _ := identity.ParseKeySet("", map[string]ed25519.PublicKey{"k1": signer.PublicKey()})
	app := &audit.Appender{}
	logger, _ := quietLogger()
	svc := &Services{Pool: pool, Policy: policy, Audit: app}
	if withLogin {
		svc.CustomerLogin = &customer.Login{Provider: &customer.DevProvider{Pool: pool, Audit: app}, Signer: signer}
	}
	r := &custRig{store: &customer.Store{Pool: pool}, auditRig: &auditRig{pool: pool, signer: signer, tenant: dbtest.Tenant(t, pool), appender: app,
		handler: NewRouter(Options{Logger: logger, Verifier: identity.NewVerifier(ks, nil), Services: svc})}}
	if err := pool.QueryRow(context.Background(), `SELECT code FROM institutions WHERE id = $1`, r.tenant).Scan(&r.code); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *custRig) do(method, path, body, tok string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func (r *custRig) login(t *testing.T, ref, pass string) (int, string) {
	t.Helper()
	rec := r.do(http.MethodPost, "/v1/customer/auth/login", `{"institution":"`+r.code+`","customer_ref":"`+ref+`","passphrase":"`+pass+`"}`, "")
	var out customerLoginResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out.AccessToken
}

func TestACustomerSignsInGetsACustomerTokenAndSeesOnlyCustomerRoutes(t *testing.T) {
	r := newCustRig(t, true)
	c, err := r.store.Create(context.Background(), r.tenant, "Synthetic Customer", customerPass)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.store.LinkAccount(context.Background(), r.tenant, c.ID, "acct_0123456789abcdef0123456789abcdef", "", "bank onboarding")

	if code, _ := r.login(t, c.Ref, "wrong passphrase!"); code != http.StatusUnauthorized {
		t.Fatalf("wrong passphrase: %d", code)
	}
	if code, _ := r.login(t, "cust_000000000000000000000000", customerPass); code != http.StatusUnauthorized {
		t.Fatalf("unknown customer: %d", code)
	}
	if rec := r.do(http.MethodPost, "/v1/customer/auth/login", `{"institution":""}`, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty: %d", rec.Code)
	}
	code, tok := r.login(t, c.Ref, customerPass)
	if code != http.StatusOK || tok == "" {
		t.Fatalf("sign-in: %d", code)
	}
	rec := r.do(http.MethodGet, "/v1/customer/me", "", tok)
	var me customerMe
	_ = json.Unmarshal(rec.Body.Bytes(), &me)
	if rec.Code != http.StatusOK || me.CustomerRef != c.Ref || me.Accounts != 1 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodGet, "/v1/customer/me", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}

	// The two audiences never cross.
	if rec := r.do(http.MethodGet, "/v1/me", "", tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a customer token worked on an operator route: %d", rec.Code)
	}
	if rec := r.do(http.MethodGet, "/v1/audit", "", tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a customer token reached the audit trail: %d", rec.Code)
	}
	opTok := r.token(t, r.tenant, uuid.New(), "SUPERVISOR")
	if rec := r.do(http.MethodGet, "/v1/customer/me", "", opTok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("an operator token worked on a customer route: %d", rec.Code)
	}
}

func TestACustomerWhoIsDisabledAfterSigningInIsRefusedAtOnce(t *testing.T) {
	r := newCustRig(t, true)
	c, _ := r.store.Create(context.Background(), r.tenant, "Soon Disabled", customerPass)
	_, tok := r.login(t, c.Ref, customerPass)
	if _, err := r.pool.Exec(context.Background(), `UPDATE customers SET status = 'disabled' WHERE id = $1`, c.ID); err != nil {
		t.Fatal(err)
	}
	if rec := r.do(http.MethodGet, "/v1/customer/me", "", tok); rec.Code != http.StatusUnauthorized {
		t.Fatalf("%d", rec.Code)
	}
}

func TestWithNoCustomerIdentityProviderTheCustomerRoutesDoNotExist(t *testing.T) {
	r := newCustRig(t, false)
	if rec := r.do(http.MethodPost, "/v1/customer/auth/login", `{}`, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
	if rec := r.do(http.MethodGet, "/v1/customer/me", "", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
}

type recorder = httptest.ResponseRecorder

func newReq(method, path, body, tok string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return req
}

func serve(h http.Handler, req *http.Request) *recorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
