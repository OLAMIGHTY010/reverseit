package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

type configVersionJSON struct {
	Key             string          `json:"key"`
	Version         int             `json:"version"`
	Value           json.RawMessage `json:"value"`
	SourceReference string          `json:"source_reference"`
	VerifyStatus    string          `json:"verify_status"`
	EffectiveFrom   time.Time       `json:"effective_from"`
	ProposedBy      string          `json:"proposed_by"`
	Reason          string          `json:"reason"`
	CreatedAt       time.Time       `json:"created_at"`
	Decision        string          `json:"decision"`
	DecidedBy       string          `json:"decided_by,omitempty"`
	DecidedAt       *time.Time      `json:"decided_at,omitempty"`
	DecisionReason  string          `json:"decision_reason,omitempty"`
}

func versionJSON(v configsvc.Version) configVersionJSON {
	out := configVersionJSON{
		Key: v.Key, Version: v.Version, Value: v.Raw, SourceReference: v.SourceReference, VerifyStatus: v.VerifyStatus,
		EffectiveFrom: v.EffectiveFrom, ProposedBy: v.ProposedBy.String(), Reason: v.Reason, CreatedAt: v.CreatedAt,
		Decision: v.Decision, DecidedAt: v.DecidedAt, DecisionReason: v.DecisionReason,
	}
	if out.Decision == "" {
		out.Decision = "pending"
	}
	if v.DecidedBy != uuid.Nil {
		out.DecidedBy = v.DecidedBy.String()
	}
	return out
}

// writeConfigError maps the service's errors to responses.
func writeConfigError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, configsvc.ErrUnknownKey):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "unknown configuration key")
	case errors.Is(err, configsvc.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such version")
	case errors.Is(err, configsvc.ErrNotConfigured):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_CONFIGURED", "no approved value exists for this key")
	case errors.Is(err, configsvc.ErrInvalidValue), errors.Is(err, configsvc.ErrInvalidInput):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
	case errors.Is(err, configsvc.ErrAlreadyDecided), errors.Is(err, configsvc.ErrConflict):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, configsvc.ErrSelfApproval):
		httpx.WriteError(w, r, http.StatusForbidden, "SELF_APPROVAL", "you cannot decide a change you proposed")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

// readConfig serves GET /v1/config/{key}, the value in force at a time.
func (s *Services) readConfig(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	at := time.Now()
	if v := r.URL.Query().Get("at"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "at must be an RFC 3339 time")
			return
		}
		at = t
	}
	var v configsvc.Version
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		v, err = s.Config.Effective(r.Context(), tx, p.TenantID, chi.URLParam(r, "key"), at)
		return err
	})
	if err != nil {
		writeConfigError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, versionJSON(v))
}

// configHistory serves GET /v1/config/{key}/versions.
func (s *Services) configHistory(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	var vs []configsvc.Version
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		vs, err = s.Config.History(r.Context(), tx, p.TenantID, chi.URLParam(r, "key"))
		return err
	})
	if err != nil {
		writeConfigError(w, r, err)
		return
	}
	items := make([]configVersionJSON, 0, len(vs))
	for _, v := range vs {
		items = append(items, versionJSON(v))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

type proposeRequest struct {
	Value           json.RawMessage `json:"value"`
	SourceReference string          `json:"source_reference"`
	EffectiveFrom   time.Time       `json:"effective_from"`
	VerifyStatus    string          `json:"verify_status"`
	Reason          string          `json:"reason"`
}

// proposeConfig serves POST /v1/config/{key}/proposals. Only the propose and
// draft scopes may do this. The approve scope may not: the person who approves
// does not also propose.
func (s *Services) proposeConfig(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "propose" && grant.Scope != "draft" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not propose configuration changes")
		return
	}
	var req proposeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "request body must be a JSON object with value, source_reference, effective_from, verify_status and reason")
		return
	}
	var v configsvc.Version
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		v, err = s.Config.Propose(r.Context(), tx, p.TenantID, p.UserID, configsvc.ProposeInput{
			Key: chi.URLParam(r, "key"), Value: req.Value, SourceReference: req.SourceReference,
			EffectiveFrom: req.EffectiveFrom, VerifyStatus: req.VerifyStatus, Reason: req.Reason,
		})
		return err
	})
	if err != nil {
		writeConfigError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, versionJSON(v))
}

type decisionRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// decideConfig serves POST /v1/config/{key}/versions/{version}/decision. It needs
// the approve scope and an MFA-backed token, and the decider must not be the
// proposer.
func (s *Services) decideConfig(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorize(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "approve" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not approve configuration changes")
		return
	}
	version, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || version < 1 {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "version must be a positive integer")
		return
	}
	var req decisionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil || (req.Decision != "approved" && req.Decision != "rejected") {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "request body must be a JSON object with decision (approved or rejected) and reason")
		return
	}
	var v configsvc.Version
	err = s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		v, err = s.Config.Decide(r.Context(), tx, p.TenantID, p.UserID, chi.URLParam(r, "key"), version, req.Decision == "approved", req.Reason)
		return err
	})
	if err != nil {
		writeConfigError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, versionJSON(v))
}
