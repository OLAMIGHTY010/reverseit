// Package server builds the HTTP router and runs the server process.
package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/observability"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
)

// Options configures NewRouter.
type Options struct {
	Logger  *slog.Logger
	Version string
	// Service names the process in /v1/health. Default "api".
	Service string
	// Checkers are run by GET /v1/ready.
	Checkers []Checker
	// Verifier checks operator tokens. Nil means every protected route returns
	// 401, which is the safe default when identity is not configured.
	Verifier *identity.Verifier
	// Auth handles POST /v1/auth/login. Nil leaves the route out entirely.
	Auth *identity.Authenticator
	// ServiceVerifier checks assertions from other services. Nil leaves out the
	// machine routes such as event ingestion.
	ServiceVerifier *servauth.Verifier
	// Services enables the routes that need a database, a permission policy and
	// the audit trail. Nil leaves them out.
	Services *Services
}

type authRoutes struct {
	auth   *identity.Authenticator
	logger *slog.Logger
}

// NewRouter returns the API router. Routes live under /v1.
func NewRouter(o Options) http.Handler {
	r := chi.NewRouter()
	r.NotFound(httpx.NotFound)
	r.MethodNotAllowed(httpx.MethodNotAllowed)

	r.Use(observability.TraceMiddleware)
	r.Use(observability.RequestLogger(o.Logger))
	r.Use(recoverer(o.Logger))

	service := o.Service
	if service == "" {
		service = "api"
	}
	h := &health{service: service, version: o.Version, checkers: o.Checkers}
	r.Route("/v1", func(v1 chi.Router) {
		v1.Get("/health", h.live)
		v1.Get("/ready", h.ready)

		if o.Auth != nil {
			ar := &authRoutes{auth: o.Auth, logger: o.Logger}
			v1.Post("/auth/login", ar.login)
		}

		// Machine routes: a service assertion with the right scope, no operator token.
		if o.ServiceVerifier != nil && o.Services.ready() && o.Services.Txn != nil {
			v1.With(requireService(o.ServiceVerifier, "events:write")).Post("/events", o.Services.ingestEvent)
		}

		if o.ServiceVerifier != nil && o.Services.ready() && o.Services.Feed != nil {
			v1.With(requireService(o.ServiceVerifier, "events:write")).Post("/feeds/{mapping}/files", o.Services.importFeedFile)
		}

		if o.ServiceVerifier != nil && o.Services.ready() && o.Services.Correlate != nil {
			v1.With(requireService(o.ServiceVerifier, "events:write")).Post("/observations", o.Services.submitObservation)
		}

		// Everything below needs a valid operator token. Routes are added only
		// inside this group, so a new route cannot be public by accident.
		v1.Group(func(pr chi.Router) {
			pr.Use(requireAuth(o.Verifier, identity.AudienceOps))
			pr.Get("/me", me)
			if o.Services.ready() {
				pr.Get("/audit", o.Services.listAudit)
				if o.Services.Config != nil {
					pr.Get("/config/{key}", o.Services.readConfig)
					pr.Get("/config/{key}/versions", o.Services.configHistory)
					pr.Post("/config/{key}/proposals", o.Services.proposeConfig)
					pr.Post("/config/{key}/versions/{version}/decision", o.Services.decideConfig)
					pr.Get("/calendar/add-working-days", o.Services.addWorkingDays)
					pr.Get("/calendar/business-time", o.Services.businessTime)
					pr.Get("/calendar/next-cutoff", o.Services.nextCutoff)
				}
				if o.Services.Txn != nil {
					pr.Get("/transactions", o.Services.listTransactions)
					pr.Get("/transactions/{id}", o.Services.getTransaction)
					pr.Get("/pipeline", o.Services.getPipeline)
					pr.Get("/sources", o.Services.listSources)
					pr.Put("/sources/{system}/expectation", o.Services.setSourceExpectation)
					pr.Get("/quarantine", o.Services.listQuarantine)
					pr.Post("/quarantine/{id}/replay", o.Services.replayQuarantine)
					pr.Post("/quarantine/{id}/discard", o.Services.discardQuarantine)
				}
				if o.Services.Rules != nil {
					pr.Get("/rules", o.Services.listRules)
					pr.Post("/rules", o.Services.createRule)
					pr.Get("/rules/{id}", o.Services.getRule)
					pr.Post("/rules/{id}/rollback", o.Services.rollbackRule)
					pr.Post("/rules/{id}/disable", o.Services.disableRule)
					pr.Post("/rules/{id}/versions/{version}/shadow", o.Services.startRuleShadow)
					pr.Post("/rules/{id}/versions/{version}/backtest", o.Services.backtestRule)
					pr.Post("/rules/{id}/versions/{version}/activation", o.Services.requestRuleActivation)
					pr.Post("/rules/{id}/versions/{version}/decision", o.Services.decideRuleActivation)
				}
				if o.Services.Correlate != nil {
					pr.Get("/evidence", o.Services.listEvidence)
					pr.Post("/evidence/{id}/link", o.Services.linkEvidence)
					pr.Post("/evidence/{id}/discard", o.Services.discardEvidence)
				}
				if o.Services.Cases != nil {
					pr.Get("/exceptions", o.Services.listExceptions)
					pr.Get("/cases/{id}", o.Services.getCase)
					pr.Patch("/cases/{id}", o.Services.patchCase)
					pr.Post("/cases/{id}/notes", o.Services.addCaseNote)
					pr.Post("/cases/{id}/evidence", o.Services.addCaseEvidence)
					pr.Get("/cases/{id}/customer-status", o.Services.customerCaseStatus)
					pr.Post("/complaints", o.Services.submitComplaint)
				}
				if o.Services.Reversal != nil && o.Services.Cases != nil {
					pr.Post("/reversals", o.Services.requestReversal)
					pr.Get("/reversals/{id}", o.Services.getReversal)
					pr.Post("/reversals/{id}/advance", o.Services.advanceReversal)
					pr.Post("/reversals/{id}/reapproval", o.Services.requestReapproval)
					pr.Get("/cases/{id}/reversal-eligibility", o.Services.reversalEligibility)
					pr.Get("/approvals", o.Services.listApprovals)
					pr.Get("/review-items", o.Services.listReviewItems)
					pr.Post("/approvals/{id}/decision", o.Services.decideApproval)
					pr.Get("/reconciliation/reversals", o.Services.reconcileReversals)
					pr.Get("/reports/summary", o.Services.summaryReport)
				}
				if o.Services.KillSwitch != nil {
					pr.Get("/killswitch", o.Services.getKillSwitch)
					pr.Post("/killswitch", o.Services.changeKillSwitch)
				}
				if o.Services.Vault != nil {
					pr.Post("/accounts/unmask", o.Services.unmask)
				}
			}
		})
	})
	return r
}

// recoverer turns a panic into a 500 in the standard envelope. The panic value
// and stack go to the log only, never to the client.
func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				logger.ErrorContext(r.Context(), "panic_recovered",
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
					slog.String("trace_id", observability.TraceID(r.Context())),
				)
				httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
			}()
			next.ServeHTTP(w, r)
		})
	}
}
