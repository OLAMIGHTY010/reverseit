package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
)

type svcRig struct {
	handler http.Handler
	tenant  uuid.UUID
	service uuid.UUID
	priv    ed25519.PrivateKey
	signer  *identity.Signer
	verify  *identity.Verifier
}

func newSvcRig(t *testing.T) *svcRig {
	t.Helper()
	pool := dbtest.Pool(t)
	r := &svcRig{tenant: dbtest.Tenant(t, pool)}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	r.priv = priv
	reg := &servauth.Registry{Pool: pool, Audit: &audit.Appender{}}
	var err error
	r.service, err = reg.Register(t.Context(), r.tenant, uuid.New(), "simulator", pub, []string{"events:write"})
	if err != nil {
		t.Fatal(err)
	}

	opPriv, _, _ := identity.GenerateKey()
	r.signer, _ = identity.NewSigner("k1", opPriv, 10*time.Minute)
	ks, _ := identity.ParseKeySet("", map[string]ed25519.PublicKey{"k1": r.signer.PublicKey()})
	r.verify = identity.NewVerifier(ks, nil)

	sv := &servauth.Verifier{Pool: pool}
	mux := chi.NewRouter()
	mux.With(requireService(sv, "events:write")).Post("/machine", func(w http.ResponseWriter, req *http.Request) {
		p := servauth.FromContext(req.Context())
		_, _ = w.Write([]byte(p.Name))
	})
	mux.With(requireService(sv, "reversal:submit")).Post("/machine-other-scope", func(w http.ResponseWriter, req *http.Request) {})
	mux.With(requireAuth(r.verify, identity.AudienceOps)).Get("/human", func(w http.ResponseWriter, req *http.Request) {})
	r.handler = mux
	return r
}

func (r *svcRig) do(method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func (r *svcRig) assertion(t *testing.T) string {
	t.Helper()
	tok, err := servauth.NewAssertion(r.tenant, r.service, r.priv, time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestAServiceReachesMachineRoutesWithAValidAssertion(t *testing.T) {
	r := newSvcRig(t)
	rec := r.do(http.MethodPost, "/machine", r.assertion(t))
	if rec.Code != http.StatusOK || rec.Body.String() != "simulator" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestMachineRoutesRefuseEverythingElse(t *testing.T) {
	r := newSvcRig(t)
	good := r.assertion(t)
	opTok, _, _ := r.signer.Issue(identity.Principal{UserID: uuid.New(), TenantID: r.tenant, Roles: []string{"SUPERVISOR"}}, identity.AudienceOps, time.Now())

	for name, c := range map[string]struct{ path, token string }{
		"no token":          {"/machine", ""},
		"garbage":           {"/machine", "nope"},
		"an operator token": {"/machine", opTok},
		"a scope not held":  {"/machine-other-scope", r.assertion(t)},
	} {
		rec := r.do(http.MethodPost, c.path, c.token)
		if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if rec := r.do(http.MethodPost, "/machine", good); rec.Code != http.StatusOK {
		t.Fatalf("control: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/machine", good); rec.Code != http.StatusUnauthorized {
		t.Errorf("replay: %d", rec.Code)
	}
}

func TestServiceAssertionsDoNotWorkOnOperatorRoutes(t *testing.T) {
	r := newSvcRig(t)
	if rec := r.do(http.MethodGet, "/human", r.assertion(t)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a service assertion passed an operator route: %d", rec.Code)
	}
}

func TestMachineRoutesFailClosedWithoutAVerifier(t *testing.T) {
	h := requireService(nil, "events:write")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer anything")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("%d", rec.Code)
	}
}
