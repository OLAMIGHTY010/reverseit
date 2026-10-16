package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsRegistryAndHandler(t *testing.T) {
	reg := NewRegistry()

	reg.IncHTTPRequest("POST", "/v1/reversals", 202)
	reg.IncHTTPRequest("POST", "/v1/reversals", 202)
	reg.IncHTTPRequest("GET", "/v1/exceptions", 200)

	reg.IncTransaction("NIP")
	reg.IncTransaction("CARD")
	reg.IncTransaction("NIP")

	reg.IncException("ATM001")
	reg.IncException("FEE001")
	reg.IncException("ATM001")

	reg.IncReversal("VERIFIED")
	reg.IncReversal("FAILED")

	reg.IncSLABreach()
	reg.IncCircuitBreakerTrip("nip-switch")
	reg.IncAuditEvent()

	output := reg.Render()

	// Verify required Prometheus metrics are present
	checks := []string{
		`smartreversal_http_requests_total{method="POST",path="/v1/reversals",status="202"} 2`,
		`smartreversal_http_requests_total{method="GET",path="/v1/exceptions",status="200"} 1`,
		`smartreversal_transactions_ingested_total{rail="NIP"} 2`,
		`smartreversal_transactions_ingested_total{rail="CARD"} 1`,
		`smartreversal_exceptions_detected_total{code="ATM001"} 2`,
		`smartreversal_exceptions_detected_total{code="FEE001"} 1`,
		`smartreversal_reversals_executed_total{status="VERIFIED"} 1`,
		`smartreversal_reversals_executed_total{status="FAILED"} 1`,
		`smartreversal_sla_breaches_total 1`,
		`smartreversal_circuit_breaker_trips_total{name="nip-switch"} 1`,
		`smartreversal_audit_records_total 1`,
	}
	for _, expected := range checks {
		if !strings.Contains(output, expected) {
			t.Errorf("expected metrics output to contain %q, but got:\n%s", expected, output)
		}
	}

	// Verify HTTP Handler
	req := httptest.NewRequest(http.MethodGet, "/v1/metrics", nil)
	rec := httptest.NewRecorder()
	reg.Handler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("expected text/plain content type, got %s", rec.Header().Get("Content-Type"))
	}
}

func TestMetricsMiddleware(t *testing.T) {
	reg := NewRegistry()
	mw := MetricsMiddleware(reg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/cases/test/escalate", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	out := reg.Render()
	expected := `smartreversal_http_requests_total{method="POST",path="/v1/cases/test/escalate",status="202"} 1`
	if !strings.Contains(out, expected) {
		t.Errorf("expected %q in output, got:\n%s", expected, out)
	}
}

func TestW3CTraceparentMiddleware(t *testing.T) {
	handler := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := TraceID(r.Context())
		if id == "" {
			t.Error("expected non-empty trace ID in context")
		}
	}))

	// Case 1: incoming W3C traceparent
	incomingTrace := "4bf92f3577b34da6a3ce929d0e0e4736"
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("traceparent", "00-"+incomingTrace+"-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get(TraceHeader) != incomingTrace {
		t.Errorf("expected X-Trace-Id to match incoming trace %q, got %q", incomingTrace, rec.Header().Get(TraceHeader))
	}
	if !strings.HasPrefix(rec.Header().Get("traceparent"), "00-"+incomingTrace) {
		t.Errorf("expected response traceparent to match incoming trace, got %q", rec.Header().Get("traceparent"))
	}
}
