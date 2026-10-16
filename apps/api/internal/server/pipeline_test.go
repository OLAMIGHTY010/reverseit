package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/pipeline"
)

func TestPipelineReportIsForOperatorsWithTheIngestionGrantAndWorksWithoutRedis(t *testing.T) {
	r := newIngestRig(t)
	if rec := r.post(t, "EV-900001", ev("INS-900001", "DEBIT_POSTED", "EV-900001", timeNow()), "assertion"); rec.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodGet, "/v1/pipeline", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := r.do(http.MethodGet, "/v1/pipeline", "", r.opToken(t, false, "TXN_ANALYST")); rec.Code != http.StatusForbidden {
		t.Fatalf("analyst: %d", rec.Code)
	}
	rec := r.do(http.MethodGet, "/v1/pipeline", "", r.opToken(t, false, "HEAD_OPS"))
	var rep pipeline.Report
	_ = json.Unmarshal(rec.Body.Bytes(), &rep)
	if rec.Code != http.StatusOK || rep.RedisAvailable || rep.Streams == nil || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	// The accepted event is waiting in this institution's outbox, since no relay runs in this test.
	if rep.Outbox.Unpublished != 1 {
		t.Fatalf("%+v", rep.Outbox)
	}
}

func timeNow() time.Time { return time.Now() }
