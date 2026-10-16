package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/correlate"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// submitObservation serves POST /v1/observations. A service holding events:write
// delivers evidence that identifies a transaction only by its own references. 202
// the evidence was linked or queued, 200 it was seen before, 409 its reference was
// reused with different content, 422 it is invalid.
func (s *Services) submitObservation(w http.ResponseWriter, r *http.Request) {
	svc := servauth.FromContext(r.Context())
	if svc == nil {
		unauthorized(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxEventBody))
	if err != nil {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "the body is too large")
		return
	}
	var o correlate.Observation
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&o); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "the observation is not valid JSON for this schema: "+err.Error())
		return
	}
	var d correlate.Decision
	err = s.Pool.WithTenant(r.Context(), svc.TenantID, func(tx pgx.Tx) error {
		var err error
		d, err = s.Correlate.Resolve(r.Context(), tx, svc.TenantID, o)
		return err
	})
	switch {
	case err == nil:
	case errors.Is(err, correlate.ErrInvalid):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
		return
	case errors.Is(err, txn.ErrConflict):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "source_event_ref was already used with different content")
		return
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	code := http.StatusAccepted
	if d.Duplicate {
		code = http.StatusOK
	}
	httpx.WriteJSON(w, code, d)
}

// listEvidence serves GET /v1/evidence. Needs manage_ingestion; no MFA to view.
func (s *Services) listEvidence(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "manage_ingestion", authz.Attrs{})
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var items []correlate.Item
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		items, err = correlate.List(r.Context(), tx, p.TenantID, r.URL.Query().Get("status"), limit)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

type evidenceAction struct {
	TxnID  string `json:"txn_id"`
	Reason string `json:"reason"`
}

// linkEvidence serves POST /v1/evidence/{id}/link, and discardEvidence the matching
// discard. Both need manage_ingestion with full scope and MFA, and a reason, and are
// audited in the same transaction as the change.
func (s *Services) linkEvidence(w http.ResponseWriter, r *http.Request) { s.evidenceAct(w, r, true) }
func (s *Services) discardEvidence(w http.ResponseWriter, r *http.Request) {
	s.evidenceAct(w, r, false)
}

func (s *Services) evidenceAct(w http.ResponseWriter, r *http.Request, link bool) {
	p, g, ok := s.authorize(w, r, "manage_ingestion", authz.Attrs{})
	if !ok {
		return
	}
	if g.Scope != "full" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may view evidence but not change it")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
		return
	}
	var req evidenceAction
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil || len(req.Reason) < 5 || len(req.Reason) > 500 {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "a reason of 5 to 500 characters is required")
		return
	}
	var txnID uuid.UUID
	if link {
		if txnID, err = uuid.Parse(req.TxnID); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "txn_id must be a transaction id")
			return
		}
	}
	action := "ingest.evidence_discarded"
	err = s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		details := map[string]any{"reason": req.Reason}
		if link {
			action = "ingest.evidence_linked"
			details["txn_id"] = txnID.String()
			if _, err := s.Correlate.ManualLink(r.Context(), tx, p.TenantID, id, txnID, p.UserID, req.Reason); err != nil {
				return err
			}
		} else if err := correlate.Discard(r.Context(), tx, p.TenantID, id, p.UserID, req.Reason); err != nil {
			return err
		}
		_, err := s.Audit.Append(r.Context(), tx, p.TenantID, audit.Event{
			Actor: audit.ActorUser, ActorID: p.UserID, Action: action, SubjectType: "evidence", SubjectID: id.String(), IP: clientIP(r), Details: details,
		})
		return err
	})
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "outcome": map[bool]string{true: "linked", false: "discarded"}[link]})
	case errors.Is(err, correlate.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
	case errors.Is(err, correlate.ErrNotOpen):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "this evidence was already resolved")
	case errors.Is(err, correlate.ErrMismatch):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "the evidence and the transaction disagree on amount or currency, so they cannot be linked")
	case errors.Is(err, correlate.ErrInvalid):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
	case errors.Is(err, txn.ErrInvalid), errors.Is(err, txn.ErrCoreMismatch), errors.Is(err, txn.ErrConflict):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
