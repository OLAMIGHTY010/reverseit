package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/sources"
)

// listSources serves GET /v1/sources: the freshness and clock skew of every source
// that has sent events. Any operator who may see the exception queue may read it,
// because screens must be able to say that evidence may be missing.
func (s *Services) listSources(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "view_exception_queue", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "full" && grant.Scope != "metadata" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not read source health")
		return
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	var out []sources.Status
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = sources.Report(r.Context(), tx, p.TenantID, now.UTC())
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	if out == nil {
		out = []sources.Status{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out, "as_of": now.UTC()})
}

// setSourceExpectation serves PUT /v1/sources/{system}/expectation. Needs
// manage_ingestion with full scope and MFA. The change is audited in the same
// transaction.
func (s *Services) setSourceExpectation(w http.ResponseWriter, r *http.Request) {
	p, g, ok := s.authorize(w, r, "manage_ingestion", authz.Attrs{})
	if !ok {
		return
	}
	if g.Scope != "full" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may view source health but not change it")
		return
	}
	var e sources.Expectation
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&e); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "the body is not valid JSON for this schema")
		return
	}
	e.SourceSystem = chi.URLParam(r, "system")
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		if err := sources.Set(r.Context(), tx, p.TenantID, p.UserID, e); err != nil {
			return err
		}
		_, err := s.Audit.Append(r.Context(), tx, p.TenantID, audit.Event{
			Actor: audit.ActorUser, ActorID: p.UserID, Action: "ingest.source_expectation_set", SubjectType: "source", SubjectID: e.SourceSystem, IP: clientIP(r),
			Details: map[string]any{"stale_after_secs": e.StaleAfterSec, "down_after_secs": e.DownAfterSec, "max_skew_secs": e.MaxSkewSec},
		})
		return err
	})
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, e)
	case errors.Is(err, sources.ErrInvalid):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
