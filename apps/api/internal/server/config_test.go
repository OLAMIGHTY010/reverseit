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
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type configRig struct {
	*auditRig
}

func newConfigRig(t *testing.T) *configRig {
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
	return &configRig{&auditRig{
		pool: pool, signer: signer, tenant: dbtest.Tenant(t, pool), appender: app,
		handler: NewRouter(Options{Logger: logger, Verifier: identity.NewVerifier(ks, nil),
			Services: &Services{Pool: pool, Policy: policy, Audit: app, Config: &configsvc.Service{Audit: app}}}),
	}}
}

func (r *configRig) tokenMFA(t *testing.T, user uuid.UUID, mfa bool, roles ...string) string {
	t.Helper()
	tok, _, err := r.signer.Issue(identity.Principal{UserID: user, TenantID: r.tenant, Roles: roles, MFA: mfa}, identity.AudienceOps, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (r *configRig) do(method, path, tok, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func proposalBody(value int, effective time.Time) string {
	b, _ := json.Marshal(map[string]any{
		"value": value, "source_reference": "Scope FR-04", "effective_from": effective.UTC().Format(time.RFC3339),
		"verify_status": "unverified", "reason": "initial window from the scope document",
	})
	return string(b)
}

func decodeVersion(t *testing.T, rec *httptest.ResponseRecorder, want int) configVersionJSON {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status %d, want %d: %s", rec.Code, want, rec.Body.String())
	}
	var v configVersionJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

const cfgKey = "duplicate.window_seconds"

func TestSupervisorProposesRiskApprovesAndTheValueTakesEffect(t *testing.T) {
	r := newConfigRig(t)
	sup := r.tokenMFA(t, uuid.New(), true, "SUPERVISOR")
	risk := r.tokenMFA(t, uuid.New(), true, "RISK_COMPLIANCE")
	path := "/v1/config/" + cfgKey

	if rec := r.do(http.MethodGet, path, risk, ""); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "NOT_CONFIGURED") {
		t.Fatalf("before anything: %d %s", rec.Code, rec.Body.String())
	}

	v := decodeVersion(t, r.do(http.MethodPost, path+"/proposals", sup, proposalBody(30, time.Now())), http.StatusCreated)
	if v.Version != 1 || v.Decision != "pending" || string(v.Value) != "30" || v.VerifyStatus != "unverified" {
		t.Fatalf("proposal: %+v", v)
	}
	if rec := r.do(http.MethodGet, path, risk, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("a pending proposal is in force: %d", rec.Code)
	}

	d := decodeVersion(t, r.do(http.MethodPost, path+"/versions/1/decision", risk, `{"decision":"approved"}`), http.StatusOK)
	if d.Decision != "approved" || d.DecidedBy == "" {
		t.Fatalf("decision: %+v", d)
	}
	got := decodeVersion(t, r.do(http.MethodGet, path, sup, ""), http.StatusOK)
	if string(got.Value) != "30" || got.Decision != "approved" || got.SourceReference != "Scope FR-04" {
		t.Fatalf("effective: %+v", got)
	}

	rec := r.do(http.MethodGet, path+"/versions", sup, "")
	var h struct{ Items []configVersionJSON }
	_ = json.Unmarshal(rec.Body.Bytes(), &h)
	if rec.Code != http.StatusOK || len(h.Items) != 1 {
		t.Fatalf("history: %d %s", rec.Code, rec.Body.String())
	}
}

func TestOnlyTheRightRolesMayProposeAndDecide(t *testing.T) {
	r := newConfigRig(t)
	path := "/v1/config/" + cfgKey
	for _, role := range []string{"TXN_ANALYST", "RECON_ANALYST", "REVERSAL_OFFICER", "EXECUTIVE", "AGENT"} {
		tok := r.tokenMFA(t, uuid.New(), true, role)
		if rec := r.do(http.MethodPost, path+"/proposals", tok, proposalBody(30, time.Now())); rec.Code != http.StatusForbidden {
			t.Errorf("%s proposed: %d", role, rec.Code)
		}
		if rec := r.do(http.MethodGet, path, tok, ""); rec.Code != http.StatusForbidden {
			t.Errorf("%s read config: %d", role, rec.Code)
		}
	}
	// The approver does not also propose.
	risk := r.tokenMFA(t, uuid.New(), true, "RISK_COMPLIANCE")
	if rec := r.do(http.MethodPost, path+"/proposals", risk, proposalBody(30, time.Now())); rec.Code != http.StatusForbidden {
		t.Errorf("the approver proposed: %d", rec.Code)
	}
	// Proposers do not approve, and admin only drafts.
	for _, role := range []string{"SUPERVISOR", "HEAD_OPS", "ADMIN_INTEGRATION"} {
		tok := r.tokenMFA(t, uuid.New(), true, role)
		if rec := r.do(http.MethodPost, path+"/versions/1/decision", tok, `{"decision":"approved"}`); rec.Code != http.StatusForbidden {
			t.Errorf("%s approved: %d", role, rec.Code)
		}
	}
	// Admin can draft a proposal, which still needs an approval.
	admin := r.tokenMFA(t, uuid.New(), true, "ADMIN_INTEGRATION")
	if rec := r.do(http.MethodPost, path+"/proposals", admin, proposalBody(30, time.Now())); rec.Code != http.StatusCreated {
		t.Errorf("admin draft: %d %s", rec.Code, rec.Body.String())
	}
}

func TestApprovingNeedsMFAAndASecondPerson(t *testing.T) {
	r := newConfigRig(t)
	path := "/v1/config/" + cfgKey
	maker := uuid.New()
	// One person who can act as proposer and as approver (two role contexts, one
	// user id) still cannot approve their own change.
	asProposer := r.tokenMFA(t, maker, true, "SUPERVISOR")
	both := r.tokenMFA(t, maker, true, "RISK_COMPLIANCE")
	decodeVersion(t, r.do(http.MethodPost, path+"/proposals", asProposer, proposalBody(30, time.Now())), http.StatusCreated)

	rec := r.do(http.MethodPost, path+"/versions/1/decision", both, `{"decision":"approved"}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "SELF_APPROVAL") {
		t.Fatalf("self approval: %d %s", rec.Code, rec.Body.String())
	}

	noMFA := r.tokenMFA(t, uuid.New(), false, "RISK_COMPLIANCE")
	rec = r.do(http.MethodPost, path+"/versions/1/decision", noMFA, `{"decision":"approved"}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "MFA_REQUIRED") {
		t.Fatalf("no MFA: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodGet, path, both, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("nothing should be in force: %d", rec.Code)
	}
}

func TestDecisionsAreFinalAndRejectionsNeedAReason(t *testing.T) {
	r := newConfigRig(t)
	path := "/v1/config/" + cfgKey
	sup := r.tokenMFA(t, uuid.New(), true, "SUPERVISOR")
	risk := r.tokenMFA(t, uuid.New(), true, "RISK_COMPLIANCE")
	decodeVersion(t, r.do(http.MethodPost, path+"/proposals", sup, proposalBody(30, time.Now())), http.StatusCreated)

	if rec := r.do(http.MethodPost, path+"/versions/1/decision", risk, `{"decision":"rejected"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("reject without reason: %d", rec.Code)
	}
	decodeVersion(t, r.do(http.MethodPost, path+"/versions/1/decision", risk, `{"decision":"rejected","reason":"no source for this value"}`), http.StatusOK)
	if rec := r.do(http.MethodPost, path+"/versions/1/decision", risk, `{"decision":"approved"}`); rec.Code != http.StatusConflict {
		t.Errorf("second decision: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, path+"/versions/7/decision", risk, `{"decision":"approved"}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown version: %d", rec.Code)
	}
}

func TestConfigRequestValidation(t *testing.T) {
	r := newConfigRig(t)
	sup := r.tokenMFA(t, uuid.New(), true, "SUPERVISOR")
	risk := r.tokenMFA(t, uuid.New(), true, "RISK_COMPLIANCE")

	cases := map[string]struct {
		method, path, tok, body string
		want                    int
	}{
		"no token":        {http.MethodGet, "/v1/config/" + cfgKey, "", "", http.StatusUnauthorized},
		"unknown key":     {http.MethodGet, "/v1/config/made.up", risk, "", http.StatusNotFound},
		"bad at":          {http.MethodGet, "/v1/config/" + cfgKey + "?at=yesterday", risk, "", http.StatusBadRequest},
		"propose junk":    {http.MethodPost, "/v1/config/" + cfgKey + "/proposals", sup, `nope`, http.StatusBadRequest},
		"propose extra":   {http.MethodPost, "/v1/config/" + cfgKey + "/proposals", sup, `{"value":1,"admin":true}`, http.StatusBadRequest},
		"propose string":  {http.MethodPost, "/v1/config/" + cfgKey + "/proposals", sup, strings.Replace(proposalBody(30, time.Now()), `"value":30`, `"value":"30"`, 1), http.StatusUnprocessableEntity},
		"propose range":   {http.MethodPost, "/v1/config/" + cfgKey + "/proposals", sup, proposalBody(99999, time.Now()), http.StatusUnprocessableEntity},
		"propose unknown": {http.MethodPost, "/v1/config/made.up/proposals", sup, proposalBody(1, time.Now()), http.StatusNotFound},
		"propose past":    {http.MethodPost, "/v1/config/" + cfgKey + "/proposals", sup, proposalBody(30, time.Now().AddDate(0, 0, -3)), http.StatusUnprocessableEntity},
		"bad version":     {http.MethodPost, "/v1/config/" + cfgKey + "/versions/zero/decision", risk, `{"decision":"approved"}`, http.StatusBadRequest},
		"bad decision":    {http.MethodPost, "/v1/config/" + cfgKey + "/versions/1/decision", risk, `{"decision":"maybe"}`, http.StatusBadRequest},
	}
	for name, c := range cases {
		if rec := r.do(c.method, c.path, c.tok, c.body); rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (%s)", name, rec.Code, c.want, rec.Body.String())
		}
	}
}

func TestConfigRoutesAreAbsentWithoutTheService(t *testing.T) {
	logger, _ := quietLogger()
	h := NewRouter(Options{Logger: logger})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/config/"+cfgKey, nil))
	if rec.Code == http.StatusOK {
		t.Fatal("route answered")
	}
}

func TestReadingAndProposingNeedNoMFAButApprovingDoes(t *testing.T) {
	r := newConfigRig(t)
	path := "/v1/config/" + cfgKey
	sup := r.tokenMFA(t, uuid.New(), false, "SUPERVISOR")
	risk := r.tokenMFA(t, uuid.New(), false, "RISK_COMPLIANCE")

	if rec := r.do(http.MethodGet, path, risk, ""); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "NOT_CONFIGURED") {
		t.Fatalf("read without MFA: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodGet, path+"/versions", risk, ""); rec.Code != http.StatusOK {
		t.Fatalf("history without MFA: %d", rec.Code)
	}
	decodeVersion(t, r.do(http.MethodPost, path+"/proposals", sup, proposalBody(30, time.Now())), http.StatusCreated)

	rec := r.do(http.MethodPost, path+"/versions/1/decision", risk, `{"decision":"approved"}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "MFA_REQUIRED") {
		t.Fatalf("approving without MFA: %d %s", rec.Code, rec.Body.String())
	}
}
