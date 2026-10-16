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
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/datashield"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

const reasonOK = "customer disputes a missing credit, ticket 4411"

type unmaskRig struct {
	*auditRig
	vault *datashield.Vault
	now   time.Time
	token string // account token for 0123456789
}

func newUnmaskRig(t *testing.T, limit int) *unmaskRig {
	t.Helper()
	pool := dbtest.Pool(t)
	policy, err := authz.Load(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	priv, _, _ := identity.GenerateKey()
	signer, _ := identity.NewSigner("k1", priv, 10*time.Minute)
	ks, _ := identity.ParseKeySet("", map[string]ed25519.PublicKey{"k1": signer.PublicKey()})

	tk, _ := datashield.GenerateKey()
	ek, _ := datashield.GenerateKey()
	tokenizer, _ := datashield.NewTokenizer(tk)
	sealer, _ := datashield.NewSealer("k1", ek)
	vault := &datashield.Vault{Tokenizer: tokenizer, Sealer: sealer}

	rig := &unmaskRig{vault: vault, now: time.Now()}
	app := &audit.Appender{}
	logger, _ := quietLogger()
	rig.auditRig = &auditRig{
		pool: pool, signer: signer, tenant: dbtest.Tenant(t, pool), appender: app,
		handler: NewRouter(Options{
			Logger: logger, Verifier: identity.NewVerifier(ks, nil),
			Services: &Services{Pool: pool, Policy: policy, Audit: app, Vault: vault,
				UnmaskLimit: limit, UnmaskWindow: time.Hour, Now: func() time.Time { return rig.now }},
		}),
	}
	err = pool.WithTenant(context.Background(), rig.tenant, func(tx pgx.Tx) error {
		st, err := vault.Store(context.Background(), tx, rig.tenant, "0123456789")
		rig.token = st.Token
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return rig
}

func (r *unmaskRig) tokenFor(t *testing.T, user uuid.UUID, mfa bool, roles ...string) string {
	t.Helper()
	tok, _, err := r.signer.Issue(identity.Principal{UserID: user, TenantID: r.tenant, Roles: roles, MFA: mfa}, identity.AudienceOps, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (r *unmaskRig) post(tok, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/accounts/unmask", strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func (r *unmaskRig) body(token, reason string) string {
	b, _ := json.Marshal(map[string]string{"token": token, "reason": reason})
	return string(b)
}

func (r *unmaskRig) records(t *testing.T) []audit.Record {
	t.Helper()
	var out []audit.Record
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		out, err = audit.List(context.Background(), tx, r.tenant, audit.Filter{Action: "account.unmask"}, 0, 100)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUnmaskRevealsTheNumberAndAuditsTheReveal(t *testing.T) {
	r := newUnmaskRig(t, 10)
	user := uuid.New()
	rec := r.post(r.tokenFor(t, user, true, "TXN_ANALYST"), r.body(r.token, reasonOK))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("an unmasked response is cacheable")
	}
	var out unmaskResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.AccountNumber != "0123456789" || out.Masked != "****6789" || out.Token != r.token {
		t.Fatalf("response: %+v", out)
	}

	recs := r.records(t)
	if len(recs) != 1 {
		t.Fatalf("%d audit records", len(recs))
	}
	got := recs[0]
	if got.ActorID != user || !got.Sensitive || got.SubjectID != r.token || got.Details["reason"] != reasonOK || got.Details["outcome"] != "revealed" {
		t.Fatalf("audit record: %+v", got)
	}
	// The audit trail must not hold the number it protects.
	if strings.Contains(strings.Join([]string{got.SubjectID, got.IP, got.Details["reason"].(string)}, " "), "0123456789") {
		t.Error("the audit record contains the account number")
	}
}

func TestUnmaskIsRefusedWithoutTheGrantMFAOrAReason(t *testing.T) {
	r := newUnmaskRig(t, 10)
	cases := map[string]struct {
		tok  string
		body string
		want int
	}{
		"no token":           {"", r.body(r.token, reasonOK), http.StatusUnauthorized},
		"role without grant": {r.tokenFor(t, uuid.New(), true, "EXECUTIVE"), r.body(r.token, reasonOK), http.StatusForbidden},
		"head of ops":        {r.tokenFor(t, uuid.New(), true, "HEAD_OPS"), r.body(r.token, reasonOK), http.StatusForbidden},
		"admin":              {r.tokenFor(t, uuid.New(), true, "ADMIN_INTEGRATION"), r.body(r.token, reasonOK), http.StatusForbidden},
		"no MFA":             {r.tokenFor(t, uuid.New(), false, "TXN_ANALYST"), r.body(r.token, reasonOK), http.StatusForbidden},
		"short reason":       {r.tokenFor(t, uuid.New(), true, "TXN_ANALYST"), r.body(r.token, "because"), http.StatusForbidden},
		"blank reason":       {r.tokenFor(t, uuid.New(), true, "TXN_ANALYST"), r.body(r.token, "   "), http.StatusUnprocessableEntity},
		"no token field":     {r.tokenFor(t, uuid.New(), true, "TXN_ANALYST"), `{"reason":"` + reasonOK + `"}`, http.StatusUnprocessableEntity},
		"bad json":           {r.tokenFor(t, uuid.New(), true, "TXN_ANALYST"), `{`, http.StatusBadRequest},
		"unknown field":      {r.tokenFor(t, uuid.New(), true, "TXN_ANALYST"), `{"token":"x","reason":"` + reasonOK + `","admin":true}`, http.StatusBadRequest},
	}
	for name, c := range cases {
		rec := r.post(c.tok, c.body)
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", name, rec.Code, c.want, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "0123456789") {
			t.Errorf("%s: the response contains the number", name)
		}
	}
	if n := len(r.records(t)); n != 0 {
		t.Errorf("refused requests left %d audit records; they were stopped before any lookup", n)
	}
}

func TestMFARequiredErrorIsDistinct(t *testing.T) {
	r := newUnmaskRig(t, 10)
	rec := r.post(r.tokenFor(t, uuid.New(), false, "TXN_ANALYST"), r.body(r.token, reasonOK))
	if !strings.Contains(rec.Body.String(), "MFA_REQUIRED") {
		t.Fatalf("%s", rec.Body.String())
	}
}

func TestUnknownTokenIsNotFoundAndStillAudited(t *testing.T) {
	r := newUnmaskRig(t, 10)
	user := uuid.New()
	rec := r.post(r.tokenFor(t, user, true, "TXN_ANALYST"), r.body("acct_"+strings.Repeat("0", 32), reasonOK))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	recs := r.records(t)
	if len(recs) != 1 || recs[0].Details["outcome"] != "not_found" {
		t.Fatalf("probing must leave a trace: %+v", recs)
	}
}

func TestUnmaskIsRateLimitedPerCaller(t *testing.T) {
	r := newUnmaskRig(t, 3)
	alice, bob := uuid.New(), uuid.New()
	a, b := r.tokenFor(t, alice, true, "TXN_ANALYST"), r.tokenFor(t, bob, true, "TXN_ANALYST")

	for i := 0; i < 3; i++ {
		if rec := r.post(a, r.body(r.token, reasonOK)); rec.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i+1, rec.Code)
		}
	}
	rec := r.post(a, r.body(r.token, reasonOK))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || strings.Contains(rec.Body.String(), "0123456789") {
		t.Fatalf("fourth request: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.post(b, r.body(r.token, reasonOK)); rec.Code != http.StatusOK {
		t.Fatalf("another caller was limited by Alice's use: %d", rec.Code)
	}
	if n := len(r.records(t)); n != 4 {
		t.Fatalf("%d audit records, want 3 for Alice and 1 for Bob (the limited attempt writes none)", n)
	}

	// The window moves on.
	r.now = r.now.Add(2 * time.Hour)
	if rec := r.post(a, r.body(r.token, reasonOK)); rec.Code != http.StatusOK {
		t.Fatalf("after the window: %d", rec.Code)
	}
}

func TestNothingIsReleasedWhenTheAuditWriteFails(t *testing.T) {
	r := newUnmaskRig(t, 10)
	// Make every audit append fail by breaking the chain head for this tenant.
	err := r.pool.WithTx(context.Background(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `SET LOCAL session_replication_role = replica`); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `INSERT INTO audit_heads (tenant_id, last_seq, last_hash) VALUES ($1, 7, repeat('a', 64)) ON CONFLICT (tenant_id) DO UPDATE SET last_seq = 7`, r.tenant); err != nil {
			return err
		}
		// A row at seq 8 makes the next append collide on the primary key.
		_, err := tx.Exec(context.Background(), `INSERT INTO audit_events (tenant_id, chain_seq, id, occurred_at, actor_type, action, prev_hash, record_hash)
			VALUES ($1, 8, gen_random_uuid(), now(), 'system', 'x.y', repeat('a', 64), repeat('b', 64))`, r.tenant)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := r.post(r.tokenFor(t, uuid.New(), true, "TXN_ANALYST"), r.body(r.token, reasonOK))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "0123456789") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestOtherTenantsCannotUnmask(t *testing.T) {
	r := newUnmaskRig(t, 10)
	other := dbtest.Tenant(t, r.pool)
	tok, _, _ := r.signer.Issue(identity.Principal{UserID: uuid.New(), TenantID: other, Roles: []string{"TXN_ANALYST"}, MFA: true}, identity.AudienceOps, time.Now())
	rec := r.post(tok, r.body(r.token, reasonOK))
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "0123456789") {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUnmaskRouteIsAbsentWithoutAVault(t *testing.T) {
	logger, _ := quietLogger()
	h := NewRouter(Options{Logger: logger})
	req := httptest.NewRequest(http.MethodPost, "/v1/accounts/unmask", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("route answered")
	}
}
