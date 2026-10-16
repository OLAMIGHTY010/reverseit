package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/sources"
)

func TestSourceHealthIsReadByOperatorsAndExpectationsAreSetUnderControl(t *testing.T) {
	r := newIngestRig(t)
	if rec := r.post(t, "EV-800001", ev("INS-800001", "DEBIT_POSTED", "EV-800001", time.Now()), "assertion"); rec.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	read := func(tok string) (int, []sources.Status) {
		rec := r.do(http.MethodGet, "/v1/sources", "", tok)
		var out struct{ Items []sources.Status }
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out.Items
	}
	if code, _ := read(""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", code)
	}
	if code, _ := read(r.opToken(t, false, "EXECUTIVE")); code != http.StatusForbidden {
		t.Fatalf("aggregate-only role read source health: %d", code)
	}
	code, items := read(r.opToken(t, false, "TXN_ANALYST"))
	if code != http.StatusOK || len(items) != 1 || items[0].SourceSystem != "SWITCH_A" || items[0].Status != "unconfigured" {
		t.Fatalf("%d %+v", code, items)
	}

	body := `{"stale_after_secs":300,"down_after_secs":900,"max_skew_secs":30}`
	path := "/v1/sources/SWITCH_A/expectation"
	if rec := r.do(http.MethodPut, path, body, r.opToken(t, false, "ADMIN_INTEGRATION")); rec.Code != http.StatusForbidden {
		t.Fatalf("no MFA: %d", rec.Code)
	}
	if rec := r.do(http.MethodPut, path, body, r.opToken(t, true, "HEAD_OPS")); rec.Code != http.StatusForbidden {
		t.Fatalf("view-only: %d", rec.Code)
	}
	admin := r.opToken(t, true, "ADMIN_INTEGRATION")
	if rec := r.do(http.MethodPut, path, `{"stale_after_secs":900,"down_after_secs":300}`, admin); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("inverted thresholds: %d", rec.Code)
	}
	if rec := r.do(http.MethodPut, "/v1/sources/NEVER_SEEN/expectation", body, admin); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown source: %d", rec.Code)
	}
	if rec := r.do(http.MethodPut, path, body, admin); rec.Code != http.StatusOK {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	if _, items = read(r.opToken(t, false, "TXN_ANALYST")); items[0].Status != "fresh" || items[0].Expectation == nil {
		t.Fatalf("%+v", items)
	}
	var audited int
	_ = r.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events WHERE tenant_id = $1 AND action = 'ingest.source_expectation_set'`, r.tenant).Scan(&audited)
	if audited != 1 {
		t.Fatalf("%d audit records", audited)
	}
}
