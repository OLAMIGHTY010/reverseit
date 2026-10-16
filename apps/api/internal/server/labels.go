package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/labels"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

// listLabeledExamples serves GET /v1/labeled-examples?status=. Needs the full
// exception-queue scope. Only status "confirmed" examples may feed training or rule tuning.
func (s *Services) listLabeledExamples(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "view_exception_queue", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "full" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not see labelled examples")
		return
	}
	var out []labels.Example
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = labels.List(r.Context(), tx, p.TenantID, r.URL.Query().Get("status"))
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

type labelDecision struct {
	Confirm bool   `json:"confirm"`
	Note    string `json:"note"`
}

// decideLabeledExample serves POST /v1/labeled-examples/{id}/decision. Needs the approve
// scope of change_rules and MFA, because confirmed labels feed rule tuning. Whoever closed
// the case may not confirm its label. A rejection needs a note. The decision is a new
// version and is audited.
func (s *Services) decideLabeledExample(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorize(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "approve" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not decide labelled examples")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
		return
	}
	var req labelDecision
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "the body must be a JSON object with confirm and note")
		return
	}
	var next labels.Example
	err = s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		if next, err = labels.Decide(r.Context(), tx, p.TenantID, p.UserID, id, req.Confirm, req.Note, time.Now()); err != nil {
			return err
		}
		_, err = s.Audit.Append(r.Context(), tx, p.TenantID, audit.Event{Actor: audit.ActorUser, ActorID: p.UserID, Action: "labels." + next.Status, SubjectType: "labeled_example",
			SubjectID: next.ID.String(), IP: clientIP(r), Details: map[string]any{"case_id": next.CaseID.String(), "version": next.Version, "note": req.Note}})
		return err
	})
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, next)
	case errors.Is(err, labels.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
	case errors.Is(err, labels.ErrState):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "this example was already decided")
	case errors.Is(err, labels.ErrFourEyes):
		httpx.WriteError(w, r, http.StatusForbidden, "SELF_APPROVAL", "whoever closed the case may not confirm its label")
	default:
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
	}
}
