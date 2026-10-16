package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/metrics"
)

func TestMetricsAreReadableByAggregateRolesAndTheCatalogueExplainsEachOne(t *testing.T) {
	r := newReportRig(t)
	get := func(path, role string) *recorder {
		tok := ""
		if role != "" {
			tok = r.token(t, r.tenant, uuid.New(), role)
		}
		return serve(r.handler, newReq("GET", path, "", tok))
	}
	if rec := get("/v1/metrics", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := get("/v1/metrics", "AGENT"); rec.Code != http.StatusForbidden {
		t.Fatalf("an agent read metrics: %d", rec.Code)
	}
	for _, role := range []string{"EXECUTIVE", "SUPERVISOR", "TXN_ANALYST"} {
		rec := get("/v1/metrics?from="+time.Now().AddDate(0, 0, -1).UTC().Format(time.RFC3339)+"&to="+time.Now().UTC().Format(time.RFC3339), role)
		var rep metrics.Report
		_ = json.Unmarshal(rec.Body.Bytes(), &rep)
		if rec.Code != http.StatusOK || !rep.Simulated || len(rep.Metrics) != len(metrics.Catalogue) {
			t.Fatalf("%s: %d %s", role, rec.Code, rec.Body.String())
		}
	}
	rec := get("/v1/metrics/catalogue", "EXECUTIVE")
	var cat struct{ Items []metrics.Definition }
	_ = json.Unmarshal(rec.Body.Bytes(), &cat)
	if rec.Code != http.StatusOK || len(cat.Items) != len(metrics.Catalogue) || cat.Items[0].Formula == "" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	for _, q := range []string{"?from=yesterday", "?to=nope", "?from=2026-10-14T00:00:00Z&to=2026-10-13T00:00:00Z", "?from=2024-01-01T00:00:00Z&to=2026-01-01T00:00:00Z"} {
		if rec := get("/v1/metrics"+q, "SUPERVISOR"); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", q, rec.Code)
		}
	}
	if rec := get("/v1/metrics", "SUPERVISOR"); rec.Code != http.StatusOK {
		t.Fatalf("the default period: %d", rec.Code)
	}
}
