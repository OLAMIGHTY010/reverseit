package server

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type auditRig struct {
	pool     *db.Pool
	handler  http.Handler
	signer   *identity.Signer
	tenant   uuid.UUID
	appender *audit.Appender
}

func newAuditRig(t *testing.T) *auditRig {
	t.Helper()
	pool := dbtest.Pool(t)
	policy, err := authz.Load(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	priv, _, _ := identity.GenerateKey()
	signer, err := identity.NewSigner("k1", priv, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ks, _ := identity.ParseKeySet("", map[string]ed25519.PublicKey{"k1": signer.PublicKey()})
	logger, _ := quietLogger()
	app := &audit.Appender{}
	return &auditRig{
		pool: pool, signer: signer, tenant: dbtest.Tenant(t, pool), appender: app,
		handler: NewRouter(Options{
			Logger: logger, Verifier: identity.NewVerifier(ks, nil),
			Services: &Services{Pool: pool, Policy: policy, Audit: app},
		}),
	}
}

func (r *auditRig) token(t *testing.T, tenant uuid.UUID, user uuid.UUID, roles ...string) string {
	t.Helper()
	tok, _, err := r.signer.Issue(identity.Principal{UserID: user, TenantID: tenant, Roles: roles}, identity.AudienceOps, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (r *auditRig) seed(t *testing.T, tenant uuid.UUID, e audit.Event) {
	t.Helper()
	err := r.pool.WithTenant(context.Background(), tenant, func(tx pgx.Tx) error {
		_, err := r.appender.Append(context.Background(), tx, tenant, e)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (r *auditRig) call(token, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/audit"+query, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) auditListResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out auditListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func userEvent(user uuid.UUID, action string) audit.Event {
	return audit.Event{Actor: audit.ActorUser, ActorID: user, Action: action, SubjectType: "case", SubjectID: "C-1"}
}

func TestAuditRequiresAuthentication(t *testing.T) {
	r := newAuditRig(t)
	if rec := r.call("", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestAuditScopesDecideWhatIsVisible(t *testing.T) {
	r := newAuditRig(t)
	alice, bob := uuid.New(), uuid.New()
	r.seed(t, r.tenant, userEvent(alice, "case.opened"))
	r.seed(t, r.tenant, userEvent(bob, "case.opened"))
	r.seed(t, r.tenant, audit.Event{Actor: audit.ActorSystem, Action: "scheduler.tick"})
	r.seed(t, r.tenant, audit.Event{Actor: audit.ActorService, ActorID: uuid.New(), Action: "ingest.accepted"})

	actors := func(l auditListResponse, action string) int {
		n := 0
		for _, it := range l.Items {
			if it.Action == action {
				n++
			}
		}
		return n
	}

	// own: only the caller's records.
	own := decodeList(t, r.call(r.token(t, r.tenant, alice, "TXN_ANALYST"), ""))
	for _, it := range own.Items {
		if it.ActorID != alice.String() {
			t.Errorf("own scope returned a record by %s", it.ActorID)
		}
	}
	if actors(own, "case.opened") != 1 {
		t.Errorf("own scope: %+v", own.Items)
	}

	// all_ops and all: everything in the tenant.
	for _, role := range []string{"HEAD_OPS", "RISK_COMPLIANCE"} {
		l := decodeList(t, r.call(r.token(t, r.tenant, uuid.New(), role), ""))
		if actors(l, "case.opened") != 2 || actors(l, "scheduler.tick") != 1 || actors(l, "ingest.accepted") != 1 {
			t.Errorf("%s did not see everything: %+v", role, l.Items)
		}
	}

	// system: only system and service actors.
	sys := decodeList(t, r.call(r.token(t, r.tenant, uuid.New(), "ADMIN_INTEGRATION"), ""))
	for _, it := range sys.Items {
		if it.ActorType == "user" {
			t.Errorf("system scope returned a user record: %+v", it)
		}
	}
	if actors(sys, "scheduler.tick") != 1 || actors(sys, "ingest.accepted") != 1 {
		t.Errorf("system scope missed records: %+v", sys.Items)
	}
}

func TestAuditDeniedRolesAndUnavailableScopes(t *testing.T) {
	r := newAuditRig(t)
	for _, role := range []string{"EXECUTIVE", "AGENT"} {
		if rec := r.call(r.token(t, r.tenant, uuid.New(), role), ""); rec.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", role, rec.Code)
		}
	}
	rec := r.call(r.token(t, r.tenant, uuid.New(), "SUPERVISOR"), "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "not available yet") {
		t.Errorf("team scope: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.call(r.token(t, r.tenant, uuid.New(), "NO_SUCH_ROLE"), ""); rec.Code != http.StatusForbidden {
		t.Errorf("unknown role: %d", rec.Code)
	}
	// An analyst may not ask for someone else's records.
	me := uuid.New()
	if rec := r.call(r.token(t, r.tenant, me, "TXN_ANALYST"), "?actor_id="+uuid.NewString()); rec.Code != http.StatusForbidden {
		t.Errorf("own scope reading another actor: %d", rec.Code)
	}
	if rec := r.call(r.token(t, r.tenant, me, "TXN_ANALYST"), "?actor_id="+me.String()); rec.Code != http.StatusOK {
		t.Errorf("own scope reading itself: %d", rec.Code)
	}
}

func TestAuditNeverCrossesTenants(t *testing.T) {
	r := newAuditRig(t)
	other := dbtest.Tenant(t, r.pool)
	r.seed(t, r.tenant, userEvent(uuid.New(), "case.opened"))
	r.seed(t, other, userEvent(uuid.New(), "secret.other.tenant"))

	l := decodeList(t, r.call(r.token(t, r.tenant, uuid.New(), "RISK_COMPLIANCE"), ""))
	for _, it := range l.Items {
		if it.Action == "secret.other.tenant" {
			t.Fatal("a record from another tenant was returned")
		}
	}
}

func TestReadingAuditIsAudited(t *testing.T) {
	r := newAuditRig(t)
	reader := uuid.New()
	tok := r.token(t, r.tenant, reader, "RISK_COMPLIANCE")
	decodeList(t, r.call(tok, "?subject_id=C-1"))

	l := decodeList(t, r.call(tok, "?action=audit.read"))
	if len(l.Items) < 1 {
		t.Fatal("the first read left no audit record")
	}
	first := l.Items[0]
	if first.ActorID != reader.String() || !first.Sensitive || first.Details["scope"] != "all" || first.Details["subject_id"] != "C-1" {
		t.Fatalf("read record: %+v", first)
	}
}

func TestAuditVerifyFieldReportsAGoodChainAndATamperedOne(t *testing.T) {
	r := newAuditRig(t)
	for i := 0; i < 3; i++ {
		r.seed(t, r.tenant, userEvent(uuid.New(), "case.noted"))
	}
	tok := r.token(t, r.tenant, uuid.New(), "RISK_COMPLIANCE")

	l := decodeList(t, r.call(tok, "?verify=true"))
	if l.Chain == nil || !l.Chain.Verified || l.Chain.Checked < 3 || l.Chain.HeadHash == "" {
		t.Fatalf("clean chain: %+v", l.Chain)
	}
	if without := decodeList(t, r.call(tok, "")); without.Chain != nil {
		t.Error("chain verification ran without being asked")
	}

	// Tamper as a superuser would, then verify again.
	err := r.pool.WithTx(context.Background(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `SET LOCAL session_replication_role = replica`); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `UPDATE audit_events SET action = 'case.erased' WHERE tenant_id = $1 AND chain_seq = 2`, r.tenant)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	l = decodeList(t, r.call(tok, "?verify=true"))
	if l.Chain == nil || l.Chain.Verified || l.Chain.BadSeq != 2 || l.Chain.Reason == "" {
		t.Fatalf("tampered chain not reported: %+v", l.Chain)
	}

	// Only the all scope may ask for verification.
	if rec := r.call(r.token(t, r.tenant, uuid.New(), "HEAD_OPS"), "?verify=true"); rec.Code != http.StatusForbidden {
		t.Errorf("all_ops verify: %d", rec.Code)
	}
}

func TestAuditPaging(t *testing.T) {
	r := newAuditRig(t)
	for i := 0; i < 5; i++ {
		r.seed(t, r.tenant, audit.Event{Actor: audit.ActorSystem, Action: fmt.Sprintf("tick.n%d", i)})
	}
	tok := r.token(t, r.tenant, uuid.New(), "HEAD_OPS")
	// The audit.read records these calls make are ignored by prefix.
	var seen []string
	cursor := ""
	for page := 0; page < 10; page++ {
		url := "?limit=2"
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		l := decodeList(t, r.call(tok, url))
		for _, it := range l.Items {
			if strings.HasPrefix(it.Action, "tick.") {
				seen = append(seen, it.Action)
			}
		}
		if l.NextCursor == "" {
			break
		}
		cursor = l.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("paging saw %v", seen)
	}
}

func TestAuditRejectsBadParameters(t *testing.T) {
	r := newAuditRig(t)
	tok := r.token(t, r.tenant, uuid.New(), "RISK_COMPLIANCE")
	for _, q := range []string{"?limit=0", "?limit=201", "?limit=x", "?cursor=!!", "?cursor=" + "bm90LWEtbnVtYmVy", "?actor_id=nope"} {
		if rec := r.call(tok, q); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", q, rec.Code)
		}
	}
}

func TestAuditRouteIsAbsentWithoutServices(t *testing.T) {
	logger, _ := quietLogger()
	h := NewRouter(Options{Logger: logger})
	req := httptest.NewRequest(http.MethodGet, "/v1/audit", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}
