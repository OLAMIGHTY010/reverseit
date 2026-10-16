package contract

import (
	"io"
	"log/slog"
	"net/http"
	"sort"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/correlate"
	"github.com/smartreversal/smartreversal/apps/api/internal/datashield"
	"github.com/smartreversal/smartreversal/apps/api/internal/feed"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/reversal"
	"github.com/smartreversal/smartreversal/apps/api/internal/rules"
	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
	"github.com/smartreversal/smartreversal/apps/api/internal/server"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// routerOperations lists "METHOD /path" for every route the real router serves.
func routerOperations(t *testing.T) map[string]bool {
	t.Helper()
	h := server.NewRouter(server.Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		// A non-nil authenticator mounts the login route.
		Auth: &identity.Authenticator{},
		// Non-nil services mount the routes that need a database and a policy.
		Services:        &server.Services{Pool: &db.Pool{}, Policy: &authz.Policy{}, Audit: &audit.Appender{}, Vault: &datashield.Vault{}, Config: &configsvc.Service{}, KillSwitch: &killswitch.Service{}, Txn: &txn.Service{}, Feed: &feed.Importer{}, Correlate: &correlate.Matcher{}, Rules: &rules.Engine{}, Cases: &cases.Service{}, Reversal: &reversal.Service{}},
		ServiceVerifier: &servauth.Verifier{},
	})
	routes, ok := h.(chi.Routes)
	if !ok {
		t.Fatal("router does not expose its routes")
	}
	got := map[string]bool{}
	err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func specOperations(t *testing.T, status string) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	for path, item := range loadSpec(t).Paths.Map() {
		for method, op := range item.Operations() {
			if s, _ := op.Extensions["x-status"].(string); s == status {
				got[method+" "+path] = true
			}
		}
	}
	return got
}

func diff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// A route that is served but not documented is an undeclared contract.
func TestEveryServedRouteIsInTheSpecAsImplemented(t *testing.T) {
	if missing := diff(routerOperations(t), specOperations(t, "implemented")); len(missing) > 0 {
		t.Errorf("routes served but not marked implemented in openapi.yaml: %v", missing)
	}
}

// An operation marked implemented must exist, or clients are told something false.
func TestEveryImplementedOperationIsServed(t *testing.T) {
	if missing := diff(specOperations(t, "implemented"), routerOperations(t)); len(missing) > 0 {
		t.Errorf("operations marked implemented in openapi.yaml but not served: %v", missing)
	}
}

// A planned operation that is already served should be promoted.
func TestNoPlannedOperationIsAlreadyServed(t *testing.T) {
	served := routerOperations(t)
	for _, op := range diff(specOperations(t, "planned"), map[string]bool{}) {
		if served[op] {
			t.Errorf("%s is served but still marked planned", op)
		}
	}
}

// Guards against the comparisons above passing because both sides are empty.
func TestRouteDiscoveryFindsKnownRoutes(t *testing.T) {
	served := routerOperations(t)
	for _, want := range []string{"GET /v1/health", "GET /v1/ready", "POST /v1/auth/login", "GET /v1/me", "GET /v1/audit", "POST /v1/accounts/unmask", "GET /v1/config/{key}", "POST /v1/config/{key}/versions/{version}/decision", "GET /v1/calendar/next-cutoff", "GET /v1/killswitch", "POST /v1/killswitch", "POST /v1/events", "POST /v1/observations", "GET /v1/evidence", "POST /v1/evidence/{id}/link", "POST /v1/evidence/{id}/discard", "POST /v1/feeds/{mapping}/files", "GET /v1/rules", "POST /v1/rules", "GET /v1/rules/{id}", "POST /v1/rules/{id}/rollback", "POST /v1/rules/{id}/disable", "POST /v1/rules/{id}/versions/{version}/shadow", "POST /v1/rules/{id}/versions/{version}/backtest", "POST /v1/rules/{id}/versions/{version}/activation", "POST /v1/rules/{id}/versions/{version}/decision", "GET /v1/pipeline", "GET /v1/sources", "PUT /v1/sources/{system}/expectation", "GET /v1/quarantine", "POST /v1/quarantine/{id}/replay", "POST /v1/quarantine/{id}/discard", "GET /v1/transactions", "GET /v1/transactions/{id}", "GET /v1/exceptions", "GET /v1/cases/{id}", "PATCH /v1/cases/{id}", "POST /v1/complaints", "POST /v1/reversals", "POST /v1/reversals/{id}/reapproval", "GET /v1/reversals/{id}", "POST /v1/approvals/{id}/decision", "GET /v1/approvals", "GET /v1/review-items", "GET /v1/reconciliation/reversals", "GET /v1/reports/summary", "GET /v1/cases/{id}/reversal-eligibility"} {
		if !served[want] {
			t.Errorf("route discovery did not find %s; found %v", want, served)
		}
	}
	if !specOperations(t, "implemented")["GET /v1/health"] {
		t.Error("spec discovery did not find GET /v1/health")
	}
}
