package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// listQuarantine serves GET /v1/quarantine. Needs manage_ingestion; viewing does not
// need MFA.
func (s *Services) listQuarantine(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "manage_ingestion", authz.Attrs{})
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var items []txn.Quarantined
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		items, err = txn.ListQuarantined(r.Context(), tx, p.TenantID, r.URL.Query().Get("status"), limit)
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

type quarantineAction struct {
	Reason string `json:"reason"`
}

// replayQuarantine serves POST /v1/quarantine/{id}/replay. Needs manage_ingestion with
// full scope and MFA, and a reason. The attempt and its result are audited in the
// same transaction. 200 with the result (replayed or refused again), 404 unknown id,
// 409 already resolved, 422 nothing to replay.
func (s *Services) replayQuarantine(w http.ResponseWriter, r *http.Request) {
	s.quarantineAct(w, r, "ingest.quarantine_replayed")
}

// discardQuarantine serves POST /v1/quarantine/{id}/discard.
func (s *Services) discardQuarantine(w http.ResponseWriter, r *http.Request) {
	s.quarantineAct(w, r, "ingest.quarantine_discarded")
}

func (s *Services) quarantineAct(w http.ResponseWriter, r *http.Request, action string) {
	p, g, ok := s.authorize(w, r, "manage_ingestion", authz.Attrs{})
	if !ok {
		return
	}
	if g.Scope != "full" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may view quarantined events but not change them")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
		return
	}
	var req quarantineAction
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil || len(req.Reason) < 5 || len(req.Reason) > 500 {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "a reason of 5 to 500 characters is required")
		return
	}
	var res txn.ReplayResult
	err = s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		details := map[string]any{"reason": req.Reason}
		if action == "ingest.quarantine_discarded" {
			if err := txn.Discard(r.Context(), tx, p.TenantID, id, req.Reason); err != nil {
				return err
			}
			res = txn.ReplayResult{ID: id, Outcome: "discarded"}
		} else {
			var err error
			if res, err = s.Txn.Replay(r.Context(), tx, p.TenantID, id); err != nil {
				return err
			}
			details["outcome"], details["replayed"] = res.Outcome, res.Replayed
		}
		_, err := s.Audit.Append(r.Context(), tx, p.TenantID, audit.Event{
			Actor: audit.ActorUser, ActorID: p.UserID, Action: action, SubjectType: "quarantined_event", SubjectID: id.String(),
			IP: clientIP(r), Details: details,
		})
		return err
	})
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, res)
	case errors.Is(err, txn.ErrQuarantineNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
	case errors.Is(err, txn.ErrNotOpen):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "this quarantined event was already resolved")
	case errors.Is(err, txn.ErrNotReplayable):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "this quarantined event kept no payload to replay; discard it instead")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
