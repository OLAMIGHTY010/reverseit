package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/metrics"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

func (s *Services) canReadMetrics(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	p, grant, ok := s.authorizeNoMFA(w, r, "view_exception_queue", authz.Attrs{})
	if !ok {
		return uuid.Nil, false
	}
	switch grant.Scope {
	case "full", "metadata", "aggregate":
		return p.TenantID, true
	}
	httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not read metrics")
	return uuid.Nil, false
}

// metricsCatalogue serves GET /v1/metrics/catalogue: what each metric means, how it is
// computed, which tables it reads, and what it cannot tell you.
func (s *Services) metricsCatalogue(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.canReadMetrics(w, r); !ok {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": metrics.Catalogue})
}

// metricsReport serves GET /v1/metrics?from=&to=. The period defaults to the last seven
// days and may not exceed 366. Values are measured from stored records, are null when
// there is nothing to measure, and describe synthetic data only. Roles limited to
// aggregates get the same figures: none of them identifies a person or a case.
func (s *Services) metricsReport(w http.ResponseWriter, r *http.Request) {
	t, ok := s.canReadMetrics(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	from, to := now.AddDate(0, 0, -7), now
	var err error
	if v := r.URL.Query().Get("from"); v != "" {
		if from, err = time.Parse(time.RFC3339, v); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "from must be an RFC 3339 time")
			return
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if to, err = time.Parse(time.RFC3339, v); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "to must be an RFC 3339 time")
			return
		}
	}
	tenant := t
	var rep metrics.Report
	err = s.Pool.WithTenant(r.Context(), tenant, func(tx pgx.Tx) error {
		var err error
		rep, err = metrics.Compute(r.Context(), tx, tenant, from, to)
		return err
	})
	switch {
	case err == nil:
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteJSON(w, http.StatusOK, rep)
	case errors.Is(err, metrics.ErrPeriod):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
