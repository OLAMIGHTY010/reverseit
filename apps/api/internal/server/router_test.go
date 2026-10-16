package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeChecker struct {
	name  string
	err   error
	block bool
}

func (f fakeChecker) Name() string { return f.name }
func (f fakeChecker) Check(ctx context.Context) error {
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

func quietLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestHealth(t *testing.T) {
	logger, _ := quietLogger()
	h := NewRouter(Options{Logger: logger, Version: "1.2.3"})
	rec := do(h, http.MethodGet, "/v1/health")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body liveBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Version != "1.2.3" {
		t.Fatalf("unexpected body: %+v", body)
	}
	if rec.Header().Get("X-Trace-Id") == "" {
		t.Fatal("trace id header missing")
	}
}

func TestReadyReportsFailureWithoutLeakingErrors(t *testing.T) {
	logger, _ := quietLogger()
	h := NewRouter(Options{Logger: logger, Checkers: []Checker{
		fakeChecker{name: "postgres"},
		fakeChecker{name: "redis", err: errors.New("dial tcp 10.0.0.5:6379 password=hunter2")},
	}})
	rec := do(h, http.MethodGet, "/v1/ready")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var body readyBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Checks["postgres"] != "ok" || body.Checks["redis"] != "failed" {
		t.Fatalf("unexpected checks: %+v", body.Checks)
	}
	if strings.Contains(rec.Body.String(), "hunter2") || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Fatalf("error text leaked: %s", rec.Body.String())
	}
}

func TestReadyOKWithAllHealthyAndWithNoCheckers(t *testing.T) {
	logger, _ := quietLogger()
	for _, cs := range [][]Checker{nil, {fakeChecker{name: "postgres"}}} {
		rec := do(NewRouter(Options{Logger: logger, Checkers: cs}), http.MethodGet, "/v1/ready")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d with %d checkers", rec.Code, len(cs))
		}
	}
}

func TestReadyTimesOutSlowChecker(t *testing.T) {
	logger, _ := quietLogger()
	h := NewRouter(Options{Logger: logger, Checkers: []Checker{fakeChecker{name: "slow", block: true}}})

	start := time.Now()
	rec := do(h, http.MethodGet, "/v1/ready")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if time.Since(start) > readyTimeout+time.Second {
		t.Fatalf("ready took %s", time.Since(start))
	}
}

func TestUnknownRouteAndMethodUseEnvelope(t *testing.T) {
	logger, _ := quietLogger()
	h := NewRouter(Options{Logger: logger})

	rec := do(h, http.MethodGet, "/v1/nope")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"error":"NOT_FOUND"`) {
		t.Fatalf("not found: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(h, http.MethodPost, "/v1/health")
	if rec.Code != http.StatusMethodNotAllowed || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("method: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPanicBecomesInternalErrorWithoutLeak(t *testing.T) {
	logger, logs := quietLogger()
	h := recoverer(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("db password is hunter2")
	}))
	rec := do(h, http.MethodGet, "/")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatalf("panic text leaked to client: %s", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "panic_recovered") {
		t.Fatalf("panic not logged: %s", logs.String())
	}
}

func TestHealthReportsConfiguredServiceName(t *testing.T) {
	logger, _ := quietLogger()
	for service, want := range map[string]string{"": "api", "worker": "worker"} {
		rec := do(NewRouter(Options{Logger: logger, Service: service}), http.MethodGet, "/v1/health")
		var body liveBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Service != want {
			t.Errorf("service %q reported as %q, want %q", service, body.Service, want)
		}
	}
}
