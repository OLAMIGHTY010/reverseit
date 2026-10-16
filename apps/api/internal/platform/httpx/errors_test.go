package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartreversal/smartreversal/apps/api/internal/observability"
)

func TestWriteErrorUsesEnvelopeAndTraceID(t *testing.T) {
	h := observability.TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusConflict, CodeConflict, "already exists")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(observability.TraceHeader, "trace-abc-12345")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type = %q", ct)
	}
	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != CodeConflict || body.Message != "already exists" || body.TraceID != "trace-abc-12345" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestWriteErrorOmitsEmptyDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusBadRequest, CodeBadRequest, "bad")
	if strings.Contains(rec.Body.String(), "details") {
		t.Fatalf("empty details should be omitted: %s", rec.Body.String())
	}
}

type payload struct {
	Name string `json:"name"`
}

func decode(t *testing.T, body string) error {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	var p payload
	return DecodeJSON(rec, req, &p)
}

func TestDecodeJSON(t *testing.T) {
	if err := decode(t, `{"name":"a"}`); err != nil {
		t.Fatalf("valid body rejected: %v", err)
	}
	for name, body := range map[string]string{
		"unknown field": `{"name":"a","extra":1}`,
		"trailing data": `{"name":"a"}{"name":"b"}`,
		"malformed":     `{`,
		"empty":         ``,
		"too large":     `{"name":"` + strings.Repeat("x", maxBodyBytes+1) + `"}`,
	} {
		if err := decode(t, body); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestDefaultHandlersUseEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	NotFound(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), CodeNotFound) {
		t.Fatalf("not found: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	MethodNotAllowed(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method not allowed: %d", rec.Code)
	}
}
