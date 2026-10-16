package server

import (
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/pipeline"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

// getPipeline serves GET /v1/pipeline: how much work is waiting in the institution's
// outbox and on the Redis streams. Needs manage_ingestion; viewing needs no MFA. It
// reports numbers and passes no verdict, because what is too much is for operators to
// decide. When Redis cannot be reached, redis_available is false and the stream list
// is empty, which says nothing about the streams themselves.
func (s *Services) getPipeline(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "manage_ingestion", authz.Attrs{})
	if !ok {
		return
	}
	var rep pipeline.Report
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		rep, err = pipeline.Collect(r.Context(), tx, p.TenantID, s.Redis)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, rep)
}
