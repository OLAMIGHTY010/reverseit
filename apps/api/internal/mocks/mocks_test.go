package mocks

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestParseKind(t *testing.T) {
	for _, ok := range []string{"status", "reversal", "notify"} {
		if _, err := ParseKind(ok); err != nil {
			t.Errorf("%s rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Status", "sms", "../x"} {
		if _, err := ParseKind(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestHealthSaysSimulatedAndNotImplemented(t *testing.T) {
	for _, k := range []Kind{KindStatus, KindReversal, KindNotify} {
		rec := httptest.NewRecorder()
		NewHandler(k, quiet()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", k, rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["simulated"] != true || body["behaviour"] != "not implemented" || body["service"] != "mock-"+string(k) {
			t.Errorf("%s: unexpected body %v", k, body)
		}
	}
}

func TestInterfaceRoutesReturnNotImplemented(t *testing.T) {
	for _, k := range []Kind{KindStatus, KindReversal, KindNotify} {
		for _, op := range Routes(k) {
			parts := strings.SplitN(op, " ", 2)
			path := strings.NewReplacer("{reference}", "R-1", "{id}", "M-1").Replace(parts[1])
			rec := httptest.NewRecorder()
			NewHandler(k, quiet()).ServeHTTP(rec, httptest.NewRequest(parts[0], path, nil))
			if rec.Code != http.StatusNotImplemented {
				t.Errorf("%s %s: status %d, want 501", k, op, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "NOT_IMPLEMENTED") || !strings.Contains(rec.Body.String(), "simulated") {
				t.Errorf("%s %s: body does not say simulated and not implemented: %s", k, op, rec.Body.String())
			}
		}
	}
}

func TestUnknownRouteIsNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(KindStatus, quiet()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reversals", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}
