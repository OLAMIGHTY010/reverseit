package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

func (r *ingestRig) opToken(t *testing.T, mfa bool, roles ...string) string {
	t.Helper()
	tok, _, err := r.signer.Issue(identity.Principal{UserID: uuid.New(), TenantID: r.tenant, Roles: roles, MFA: mfa}, identity.AudienceOps, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (r *ingestRig) do(method, path, body, tok string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func TestQuarantinedEventsAreListedReplayedAndDiscardedUnderControl(t *testing.T) {
	r := newIngestRig(t)
	// A source whose clock runs ahead: the event is refused and kept.
	future := ev("INS-600001", "DEBIT_POSTED", "EV-600001", time.Now().Add(2*time.Hour))
	if rec := r.post(t, "EV-600001", future, "assertion"); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}

	admin := r.opToken(t, true, "ADMIN_INTEGRATION")
	rec := r.do(http.MethodGet, "/v1/quarantine", "", admin)
	var list struct{ Items []txn.Quarantined }
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list.Items) != 1 || !list.Items[0].Replayable || list.Items[0].Status != "open" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	id := list.Items[0].ID.String()
	body := `{"reason":"checking the source clock"}`

	// Who may and may not.
	if rec := r.do(http.MethodGet, "/v1/quarantine", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := r.do(http.MethodGet, "/v1/quarantine", "", r.opToken(t, true, "TXN_ANALYST")); rec.Code != http.StatusForbidden {
		t.Fatalf("an analyst listed quarantine: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/quarantine/"+id+"/replay", body, r.opToken(t, false, "ADMIN_INTEGRATION")); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "MFA_REQUIRED") {
		t.Fatalf("replay without MFA: %d %s", rec.Code, rec.Body.String())
	}
	head := r.opToken(t, true, "HEAD_OPS")
	if rec := r.do(http.MethodGet, "/v1/quarantine", "", head); rec.Code != http.StatusOK {
		t.Fatalf("view-only role could not view: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/quarantine/"+id+"/replay", body, head); rec.Code != http.StatusForbidden {
		t.Fatalf("view-only role replayed: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/quarantine/"+id+"/replay", `{"reason":"x"}`, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("no real reason: %d", rec.Code)
	}

	// The cause is still there, so the replay is refused again and stored nothing.
	rec = r.do(http.MethodPost, "/v1/quarantine/"+id+"/replay", body, admin)
	var res txn.ReplayResult
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if rec.Code != http.StatusOK || res.Replayed || res.Outcome != "refused_again" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodGet, "/v1/transactions?reference=INS-600001", "", r.opToken(t, false, "TXN_ANALYST")); !strings.Contains(rec.Body.String(), `"items":[]`) && strings.Contains(rec.Body.String(), "INS-600001") {
		t.Fatalf("a refused replay stored a transaction: %s", rec.Body.String())
	}

	if rec := r.do(http.MethodPost, "/v1/quarantine/"+uuid.NewString()+"/replay", body, admin); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/quarantine/"+id+"/discard", body, admin); rec.Code != http.StatusOK {
		t.Fatalf("discard: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodPost, "/v1/quarantine/"+id+"/replay", body, admin); rec.Code != http.StatusConflict {
		t.Fatalf("replay after discard: %d", rec.Code)
	}
	var audited int
	_ = r.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events WHERE tenant_id = $1 AND action LIKE 'ingest.quarantine_%'`, r.tenant).Scan(&audited)
	if audited != 2 {
		t.Fatalf("%d audit records, want the replay attempt and the discard", audited)
	}
}
