package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/alerts"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/incidents"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

// listAlerts serves GET /v1/alerts?status=. Needs the full exception-queue scope.
func (s *Services) listAlerts(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "view_exception_queue", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "full" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not see alerts")
		return
	}
	var out []alerts.Alert
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = s.Alerts.List(r.Context(), tx, p.TenantID, r.URL.Query().Get("status"))
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

// acknowledgeAlert serves POST /v1/alerts/{id}/acknowledge. Whoever acknowledges must be
// routed the alert, or hold its target role or the role it was escalated to.
func (s *Services) acknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "view_exception_queue", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "full" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not act on alerts")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
		return
	}
	var out alerts.Alert
	denied := false
	err = s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		a, err := s.Alerts.Get(r.Context(), tx, p.TenantID, id)
		if err != nil {
			return err
		}
		if !mayActOn(p.UserID, p.Roles, a) {
			denied = true
			return nil
		}
		out, err = s.Alerts.Acknowledge(r.Context(), tx, p.TenantID, p.UserID, id)
		return err
	})
	switch {
	case denied:
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "this alert is not routed to you or your role")
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, out)
	case errors.Is(err, alerts.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
	case errors.Is(err, alerts.ErrState):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "this alert is no longer open")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

func mayActOn(user uuid.UUID, roles []string, a alerts.Alert) bool {
	for _, u := range a.RoutedTo {
		if u == user {
			return true
		}
	}
	for _, r := range roles {
		if r == a.TargetRole || (a.EscalatedRole != "" && r == a.EscalatedRole) {
			return true
		}
	}
	return false
}

type incidentDetail struct {
	incidents.Incident
	CaseIDs []uuid.UUID `json:"case_ids"`
}

// listIncidents serves GET /v1/incidents?status=.
func (s *Services) listIncidents(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "view_exception_queue", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "full" && grant.Scope != "metadata" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not see incidents")
		return
	}
	var out []incidents.Incident
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = s.Incidents.List(r.Context(), tx, p.TenantID, r.URL.Query().Get("status"))
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

// getIncident serves GET /v1/incidents/{id} (id or reference) with the cases in it, so a
// screen can go from the incident to its transactions and back.
func (s *Services) getIncident(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "view_exception_queue", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "full" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not see an incident's cases")
		return
	}
	var out incidentDetail
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		i, err := s.Incidents.Get(r.Context(), tx, p.TenantID, chi.URLParam(r, "id"))
		if err != nil {
			return err
		}
		out.Incident = i
		out.CaseIDs, err = s.Incidents.CaseIDs(r.Context(), tx, p.TenantID, i.ID)
		return err
	})
	writeIncident(w, r, err, out)
}

func writeIncident(w http.ResponseWriter, r *http.Request, err error, body any) {
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, body)
	case errors.Is(err, incidents.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
	case errors.Is(err, incidents.ErrState):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, incidents.ErrInvalid):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

type causationRequest struct {
	Status         string `json:"causation_status"`
	SuspectedCause string `json:"suspected_cause"`
	Note           string `json:"note"`
}

// setIncidentCausation serves POST /v1/incidents/{id}/causation. Needs the full
// notes-and-evidence scope. The judgement only moves up, with a note.
func (s *Services) setIncidentCausation(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.scopeAtLeast(w, r, "add_notes_evidence", "full")
	if !ok {
		return
	}
	var req causationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "the body must be a JSON object with causation_status and note")
		return
	}
	var out incidents.Incident
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		id, err := s.incidentID(r, tx, p.TenantID)
		if err != nil {
			return err
		}
		out, err = s.Incidents.SetCausation(r.Context(), tx, p.TenantID, p.UserID, id, req.Status, req.SuspectedCause, req.Note)
		return err
	})
	writeIncident(w, r, err, out)
}

type resolveIncidentRequest struct {
	ReviewNote string `json:"review_note"`
}

// resolveIncident serves POST /v1/incidents/{id}/resolve with a post-incident review note.
func (s *Services) resolveIncident(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.scopeAtLeast(w, r, "add_notes_evidence", "full")
	if !ok {
		return
	}
	var req resolveIncidentRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "the body must be a JSON object with review_note")
		return
	}
	var out incidents.Incident
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		id, err := s.incidentID(r, tx, p.TenantID)
		if err != nil {
			return err
		}
		out, err = s.Incidents.Resolve(r.Context(), tx, p.TenantID, p.UserID, id, req.ReviewNote)
		return err
	})
	writeIncident(w, r, err, out)
}

func (s *Services) incidentID(r *http.Request, tx pgx.Tx, tenant uuid.UUID) (uuid.UUID, error) {
	i, err := s.Incidents.Get(r.Context(), tx, tenant, chi.URLParam(r, "id"))
	return i.ID, err
}
