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
	"github.com/smartreversal/smartreversal/apps/api/internal/automation"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type ksRig struct{ *auditRig }

func newKSRig(t *testing.T) *ksRig {
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
	return &ksRig{&auditRig{
		pool: pool, signer: signer, tenant: dbtest.Tenant(t, pool), appender: app,
		handler: NewRouter(Options{Logger: logger, Verifier: identity.NewVerifier(ks, nil),
			Services: &Services{Pool: pool, Policy: policy, Audit: app, KillSwitch: &killswitch.Service{Audit: app}}}),
	}}
}

func (r *ksRig) tok(t *testing.T, user uuid.UUID, mfa bool, roles ...string) string {
	t.Helper()
	tok, _, err := r.signer.Issue(identity.Principal{UserID: user, TenantID: r.tenant, Roles: roles, MFA: mfa}, identity.AudienceOps, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (r *ksRig) call(method, tok, body string) (int, killswitchState, string) {
	req := httptest.NewRequest(method, "/v1/killswitch", strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	var st killswitchState
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	return rec.Code, st, rec.Body.String()
}

func TestFreezeNeedsNoMFAAndShowsInTheState(t *testing.T) {
	r := newKSRig(t)
	officer := r.tok(t, uuid.New(), false, "REVERSAL_OFFICER") // no MFA: freezing must still work
	code, st, body := r.call(http.MethodPost, officer, `{"action":"freeze","scope":"rail","target":"NIP","reason":"duplicate postings seen"}`)
	if code != http.StatusOK || len(st.Frozen) != 1 || st.Frozen[0].Scope != "rail" || st.Frozen[0].Target != "NIP" {
		t.Fatalf("%d %s", code, body)
	}
	// Repeating it is harmless.
	code, st, _ = r.call(http.MethodPost, officer, `{"action":"freeze","scope":"rail","target":"NIP","reason":"duplicate postings seen"}`)
	if code != http.StatusOK || len(st.Frozen) != 1 {
		t.Fatalf("repeat: %d %+v", code, st)
	}
	// Any signed-in operator can see it, including one who cannot change it.
	code, st, _ = r.call(http.MethodGet, r.tok(t, uuid.New(), false, "EXECUTIVE"), "")
	if code != http.StatusOK || len(st.Frozen) != 1 {
		t.Fatalf("read: %d %+v", code, st)
	}
}

func TestOnlyRolesWithTheGrantMayChangeTheSwitch(t *testing.T) {
	r := newKSRig(t)
	for _, role := range []string{"TXN_ANALYST", "RECON_ANALYST", "EXECUTIVE", "AGENT"} {
		code, _, _ := r.call(http.MethodPost, r.tok(t, uuid.New(), true, role), `{"action":"freeze","scope":"global","reason":"not allowed"}`)
		if code != http.StatusForbidden {
			t.Errorf("%s: %d", role, code)
		}
	}
	for _, role := range []string{"REVERSAL_OFFICER", "SUPERVISOR", "HEAD_OPS", "RISK_COMPLIANCE", "ADMIN_INTEGRATION"} {
		r2 := newKSRig(t)
		code, _, body := r2.call(http.MethodPost, r2.tok(t, uuid.New(), false, role), `{"action":"freeze","scope":"global","reason":"incident in progress"}`)
		if code != http.StatusOK {
			t.Errorf("%s: %d %s", role, code, body)
		}
	}
	if code, _, _ := r.call(http.MethodPost, "", `{}`); code != http.StatusUnauthorized {
		t.Errorf("no token: %d", code)
	}
	if code, _, _ := r.call(http.MethodGet, "", ""); code != http.StatusUnauthorized {
		t.Errorf("no token on read: %d", code)
	}
}

func TestLiftingNeedsMFAAndTwoPeople(t *testing.T) {
	r := newKSRig(t)
	alice, bob := uuid.New(), uuid.New()
	r.call(http.MethodPost, r.tok(t, alice, true, "SUPERVISOR"), `{"action":"freeze","scope":"global","reason":"incident in progress"}`)

	// No MFA: refused, for each lifting action.
	for _, action := range []string{"request_unfreeze", "approve_unfreeze", "reject_unfreeze"} {
		code, _, body := r.call(http.MethodPost, r.tok(t, alice, false, "SUPERVISOR"), `{"action":"`+action+`","scope":"global","reason":"resume please","request_id":1}`)
		if code != http.StatusForbidden || !strings.Contains(body, "MFA_REQUIRED") {
			t.Errorf("%s without MFA: %d %s", action, code, body)
		}
	}

	code, st, body := r.call(http.MethodPost, r.tok(t, alice, true, "SUPERVISOR"), `{"action":"request_unfreeze","scope":"global","reason":"root cause fixed"}`)
	if code != http.StatusOK || len(st.Pending) != 1 || len(st.Frozen) != 1 {
		t.Fatalf("request: %d %s", code, body)
	}
	id := st.Pending[0].RequestID
	approve := `{"action":"approve_unfreeze","reason":"verified the fix","request_id":` + itoa(id) + `}`

	code, _, body = r.call(http.MethodPost, r.tok(t, alice, true, "SUPERVISOR"), approve)
	if code != http.StatusForbidden || !strings.Contains(body, "SELF_APPROVAL") {
		t.Fatalf("self approval: %d %s", code, body)
	}
	code, st, body = r.call(http.MethodPost, r.tok(t, bob, true, "HEAD_OPS"), approve)
	if code != http.StatusOK || len(st.Frozen) != 0 || len(st.Pending) != 0 {
		t.Fatalf("approval: %d %s", code, body)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestRejectingKeepsTheFreeze(t *testing.T) {
	r := newKSRig(t)
	alice, bob := uuid.New(), uuid.New()
	r.call(http.MethodPost, r.tok(t, alice, true, "SUPERVISOR"), `{"action":"freeze","scope":"exception_type","target":"DUP001","reason":"false positives"}`)
	_, st, _ := r.call(http.MethodPost, r.tok(t, alice, true, "SUPERVISOR"), `{"action":"request_unfreeze","scope":"exception_type","target":"DUP001","reason":"tuned the rule"}`)
	code, st, body := r.call(http.MethodPost, r.tok(t, bob, true, "RISK_COMPLIANCE"),
		`{"action":"reject_unfreeze","reason":"needs more evidence","request_id":`+itoa(st.Pending[0].RequestID)+`}`)
	if code != http.StatusOK || len(st.Frozen) != 1 || len(st.Pending) != 0 {
		t.Fatalf("%d %s", code, body)
	}
}

func TestKillSwitchRequestValidationAndConflicts(t *testing.T) {
	r := newKSRig(t)
	sup := r.tok(t, uuid.New(), true, "SUPERVISOR")
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"not json":                    {`nope`, http.StatusBadRequest},
		"unknown field":               {`{"action":"freeze","scope":"global","reason":"incident","x":1}`, http.StatusBadRequest},
		"unknown action":              {`{"action":"explode","scope":"global","reason":"incident"}`, http.StatusUnprocessableEntity},
		"unknown scope":               {`{"action":"freeze","scope":"galaxy","reason":"incident"}`, http.StatusUnprocessableEntity},
		"global with a target":        {`{"action":"freeze","scope":"global","target":"NIP","reason":"incident"}`, http.StatusUnprocessableEntity},
		"rail without a target":       {`{"action":"freeze","scope":"rail","reason":"incident"}`, http.StatusUnprocessableEntity},
		"no reason":                   {`{"action":"freeze","scope":"global"}`, http.StatusUnprocessableEntity},
		"unfreeze what is not frozen": {`{"action":"request_unfreeze","scope":"global","reason":"resume please"}`, http.StatusConflict},
		"decide an unknown request":   {`{"action":"approve_unfreeze","reason":"looks fine to me","request_id":9999}`, http.StatusNotFound},
	} {
		if code, _, body := r.call(http.MethodPost, sup, c.body); code != c.want {
			t.Errorf("%s: %d, want %d (%s)", name, code, c.want, body)
		}
	}
	r.call(http.MethodPost, sup, `{"action":"freeze","scope":"global","reason":"incident in progress"}`)
	r.call(http.MethodPost, sup, `{"action":"request_unfreeze","scope":"global","reason":"root cause fixed"}`)
	if code, _, _ := r.call(http.MethodPost, r.tok(t, uuid.New(), true, "HEAD_OPS"), `{"action":"request_unfreeze","scope":"global","reason":"a second request"}`); code != http.StatusConflict {
		t.Errorf("second open request: %d", code)
	}
}

func TestKillSwitchRoutesAreAbsentWithoutTheService(t *testing.T) {
	logger, _ := quietLogger()
	h := NewRouter(Options{Logger: logger})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/killswitch", nil))
	if rec.Code == http.StatusOK {
		t.Fatal("route answered")
	}
}

func TestTheStateShowsWhatAFreezeIsHoldingBackAndTheJobs(t *testing.T) {
	r := newKSRig(t)
	ctx := context.Background()
	officer := r.tok(t, uuid.New(), false, "REVERSAL_OFFICER")
	// Nothing held yet: the fields are present and empty.
	code, st, body := r.call(http.MethodGet, officer, "")
	if code != http.StatusOK || st.DeferredWork == nil || st.Jobs == nil || st.ParkedReversals != 0 || len(st.DeferredWork) != 0 || len(st.Jobs) != 0 {
		t.Fatalf("%d %s", code, body)
	}
	if err := r.pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error {
		for i := 0; i < 2; i++ {
			if err := automation.Defer(ctx, tx, r.tenant, automation.KindDetect, uuid.New(), &killswitch.FrozenError{Scope: killswitch.ScopeGlobal}); err != nil {
				return err
			}
		}
		return automation.RecordJob(ctx, tx, r.tenant, automation.JobReversalSweep, true, "global", map[string]any{"held_new_requests": 2})
	}); err != nil {
		t.Fatal(err)
	}
	code, st, body = r.call(http.MethodGet, officer, "")
	if code != http.StatusOK || st.DeferredWork["detect"] != 2 || len(st.Jobs) != 1 || st.Jobs[0].Job != "reversal_sweep" || st.Jobs[0].State != "paused" || st.Jobs[0].Frozen != "global" {
		t.Fatalf("%d %s", code, body)
	}
	// Another institution sees none of it.
	other := dbtest.Tenant(t, r.pool)
	tok, _, _ := r.signer.Issue(identity.Principal{UserID: uuid.New(), TenantID: other, Roles: []string{"REVERSAL_OFFICER"}}, identity.AudienceOps, time.Now())
	if _, st2, _ := r.call(http.MethodGet, tok, ""); len(st2.DeferredWork) != 0 || len(st2.Jobs) != 0 {
		t.Fatalf("another institution sees %+v", st2)
	}
}
