// Package mocks holds the Track H stand-ins for external systems.
//
// In B0 each mock only reports health and answers 501 on its interface routes.
// It does not simulate any behaviour yet: the status mock gets scripted
// outcomes in B5, the reversal mock in B5, and the notification mock in B7.
// Every response says it is simulated so nothing here can be mistaken for a
// live integration.
package mocks

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/observability"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

// Kind selects which interface a mock stands in for.
type Kind string

const (
	KindStatus   Kind = "status"
	KindReversal Kind = "reversal"
	KindNotify   Kind = "notify"
)

// route is one operation from the interface contract.
type route struct{ method, path string }

// routes mirror packages/api-contracts/interfaces. A test checks they match.
var routes = map[Kind][]route{
	KindStatus: {
		{http.MethodPost, "/status-query"},
	},
	KindReversal: {
		{http.MethodPost, "/reversals"},
		{http.MethodGet, "/reversals/{reference}"},
	},
	KindNotify: {
		{http.MethodPost, "/messages"},
		{http.MethodGet, "/messages/{id}"},
	},
}

// ParseKind validates a kind name.
func ParseKind(s string) (Kind, error) {
	k := Kind(s)
	if _, ok := routes[k]; !ok {
		return "", fmt.Errorf("unknown mock kind %q; use status, reversal or notify", s)
	}
	return k, nil
}

// Routes returns "METHOD /path" for every operation the mock stubs.
func Routes(k Kind) []string {
	var out []string
	for _, r := range routes[k] {
		out = append(out, r.method+" "+r.path)
	}
	return out
}

// NewHandler returns the stub handler for kind.
func NewHandler(k Kind, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.NotFound(httpx.NotFound)
	r.MethodNotAllowed(httpx.MethodNotAllowed)
	r.Use(observability.TraceMiddleware)
	r.Use(observability.RequestLogger(logger))

	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"service":   "mock-" + string(k),
			"simulated": true,
			"behaviour": "not implemented",
		})
	})

	notImplemented := func(w http.ResponseWriter, req *http.Request) {
		httpx.WriteError(w, req, http.StatusNotImplemented, httpx.CodeNotImplemented,
			"simulated "+string(k)+" service: behaviour is not implemented yet")
	}
	for _, rt := range routes[k] {
		r.MethodFunc(rt.method, rt.path, notImplemented)
	}
	return r
}
