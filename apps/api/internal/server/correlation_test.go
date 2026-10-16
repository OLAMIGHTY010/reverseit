package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/correlate"
	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
)

func (r *ingestRig) postObs(t *testing.T, body any, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	j, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/observations", strings.NewReader(string(j)))
	if bearer == "assertion" {
		a, err := servauth.NewAssertion(r.tenant, r.svcID, r.svcKey, time.Now(), time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		bearer = a
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func observation(ref string, refs map[string]string, amount string, at time.Time) map[string]any {
	return map[string]any{"source_system": "SWITCH_X", "source_event_ref": ref, "event_type": "CREDIT_CONFIRMED",
		"source_ts": at.UTC().Format(time.RFC3339Nano), "refs": refs, "amount_minor": amount, "currency": "NGN"}
}

func TestEvidenceIsLinkedQueuedAndResolvedByAPersonThroughTheAPI(t *testing.T) {
	r := newIngestRig(t)
	now := time.Now()
	e := ev("INS-700001", "DEBIT_POSTED", "EV-700001", now)
	e["refs"] = map[string]string{"RRN": "RRN700001", "GATEWAY_REF": "GW700001"}
	if rec := r.post(t, "EV-700001", e, "assertion"); rec.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}

	if rec := r.postObs(t, observation("OB-700001", map[string]string{"GATEWAY_REF": "GW700001"}, "1850000", now.Add(time.Second)), ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no assertion: %d", rec.Code)
	}
	rec := r.postObs(t, observation("OB-700001", map[string]string{"GATEWAY_REF": "GW700001"}, "1850000", now.Add(time.Second)), "assertion")
	var d correlate.Decision
	_ = json.Unmarshal(rec.Body.Bytes(), &d)
	if rec.Code != http.StatusAccepted || !d.Linked {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := r.postObs(t, observation("OB-700001", map[string]string{"GATEWAY_REF": "GW700001"}, "1850000", now.Add(time.Second)), "assertion"); rec.Code != http.StatusOK {
		t.Fatalf("repeat: %d", rec.Code)
	}
	if rec := r.postObs(t, observation("OB-700001", map[string]string{"GATEWAY_REF": "GW700001"}, "5", now.Add(time.Second)), "assertion"); rec.Code != http.StatusConflict {
		t.Fatalf("reused reference: %d", rec.Code)
	}
	if rec := r.postObs(t, map[string]any{"source_system": "S"}, "assertion"); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid: %d", rec.Code)
	}

	// Weak evidence (the RRN alone) is queued, not linked.
	rec = r.postObs(t, observation("OB-700002", map[string]string{"RRN": "RRN700001"}, "1850000", now.Add(2*time.Second)), "assertion")
	var q correlate.Decision
	_ = json.Unmarshal(rec.Body.Bytes(), &q)
	if rec.Code != http.StatusAccepted || q.Linked || q.Queued.String() == "" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}

	admin := r.opToken(t, true, "ADMIN_INTEGRATION")
	rec = r.do(http.MethodGet, "/v1/evidence", "", admin)
	var list struct{ Items []correlate.Item }
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list.Items) != 1 || list.Items[0].SourceRef != "OB-700002" || len(list.Items[0].Candidates) != 1 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	txnID := list.Items[0].Candidates[0].TxnID.String()
	id := q.Queued.String()
	body := `{"txn_id":"` + txnID + `","reason":"RRN and amount agree, confirmed with the switch team"}`

	if rec := r.do(http.MethodGet, "/v1/evidence", "", r.opToken(t, true, "TXN_ANALYST")); rec.Code != http.StatusForbidden {
		t.Fatalf("analyst: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/evidence/"+id+"/link", body, r.opToken(t, false, "ADMIN_INTEGRATION")); rec.Code != http.StatusForbidden {
		t.Fatalf("no MFA: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/evidence/"+id+"/link", body, r.opToken(t, true, "HEAD_OPS")); rec.Code != http.StatusForbidden {
		t.Fatalf("view-only: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/evidence/"+id+"/link", `{"txn_id":"`+txnID+`","reason":"x"}`, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("no reason: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/evidence/"+id+"/link", body, admin); rec.Code != http.StatusOK {
		t.Fatalf("link: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodPost, "/v1/evidence/"+id+"/link", body, admin); rec.Code != http.StatusConflict {
		t.Fatalf("link twice: %d", rec.Code)
	}
	var audited int
	_ = r.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events WHERE tenant_id = $1 AND action = 'ingest.evidence_linked'`, r.tenant).Scan(&audited)
	if audited != 1 {
		t.Fatalf("%d audit records", audited)
	}

	// A disagreeing amount cannot be forced through.
	rec = r.postObs(t, observation("OB-700003", map[string]string{"RRN": "RRN700001"}, "99", now.Add(3*time.Second)), "assertion")
	var bad correlate.Decision
	_ = json.Unmarshal(rec.Body.Bytes(), &bad)
	if rec := r.do(http.MethodPost, "/v1/evidence/"+bad.Queued.String()+"/link", body, admin); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mismatch: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodPost, "/v1/evidence/"+bad.Queued.String()+"/discard", `{"reason":"source reported the wrong amount"}`, admin); rec.Code != http.StatusOK {
		t.Fatalf("discard: %d", rec.Code)
	}
}
