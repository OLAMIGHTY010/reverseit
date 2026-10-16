package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/rules"
)

type ruleVersionJSON struct {
	rules.Version
	Stats *rules.Stats `json:"shadow_and_backtest_stats,omitempty"`
}

func writeRuleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, rules.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such rule version")
	case errors.Is(err, rules.ErrInvalid), errors.Is(err, rules.ErrVersion):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
	case errors.Is(err, rules.ErrFourEyes):
		httpx.WriteError(w, r, http.StatusForbidden, "SELF_APPROVAL", "you cannot approve a rule you wrote or asked to activate")
	case errors.Is(err, rules.ErrState), errors.Is(err, rules.ErrNoEvidence):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

// listRules serves GET /v1/rules: the latest version of every rule.
func (s *Services) listRules(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	var out []rules.Version
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		out, err = s.Rules.Store.List(r.Context(), tx, p.TenantID, "")
		return err
	})
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

// getRule serves GET /v1/rules/{id}: every version of one rule, each with what it
// concluded in shadow and in backtests.
func (s *Services) getRule(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	var out []ruleVersionJSON
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		vs, err := s.Rules.Store.List(r.Context(), tx, p.TenantID, chi.URLParam(r, "id"))
		if err != nil {
			return err
		}
		for _, v := range vs {
			st, err := s.Rules.StatsFor(r.Context(), tx, p.TenantID, v.Rule.RuleID, v.Rule.Version)
			if err != nil {
				return err
			}
			out = append(out, ruleVersionJSON{Version: v, Stats: &st})
		}
		return nil
	})
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	if len(out) == 0 {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such rule")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

// createRule serves POST /v1/rules. The body is the rule. Needs the draft or propose
// scope. The rule is validated and saved as a draft.
func (s *Services) createRule(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "draft" && grant.Scope != "propose" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not write rules")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<10))
	if err != nil {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "the rule is too large")
		return
	}
	rl, err := rules.Parse(body)
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	var v rules.Version
	err = s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		v, err = s.Rules.Store.Create(r.Context(), tx, p.TenantID, p.UserID, rl)
		return err
	})
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, v)
}

func ruleVersionParam(w http.ResponseWriter, r *http.Request) (string, int, bool) {
	v, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || v < 1 {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "version must be a positive integer")
		return "", 0, false
	}
	return chi.URLParam(r, "id"), v, true
}

// startRuleShadow serves POST /v1/rules/{id}/versions/{version}/shadow.
func (s *Services) startRuleShadow(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "draft" && grant.Scope != "propose" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not run rules in shadow")
		return
	}
	id, ver, ok := ruleVersionParam(w, r)
	if !ok {
		return
	}
	var v rules.Version
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		v, err = s.Rules.Store.StartShadow(r.Context(), tx, p.TenantID, p.UserID, id, ver)
		return err
	})
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

// backtestRule serves POST /v1/rules/{id}/versions/{version}/backtest: evaluate the
// version against recent transactions and record the results. It opens no case.
func (s *Services) backtestRule(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "draft" && grant.Scope != "propose" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not test rules")
		return
	}
	id, ver, ok := ruleVersionParam(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var st rules.Stats
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		st, err = s.Rules.Backtest(r.Context(), tx, p.TenantID, id, ver, limit)
		return err
	})
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, st)
}

type ruleReasonRequest struct {
	Reason    string `json:"reason"`
	Approve   *bool  `json:"approve,omitempty"`
	ToVersion int    `json:"to_version,omitempty"`
}

func decodeRuleReason(w http.ResponseWriter, r *http.Request) (ruleReasonRequest, bool) {
	var req ruleReasonRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "the body must be a JSON object with a reason")
		return req, false
	}
	return req, true
}

// requestRuleActivation serves POST /v1/rules/{id}/versions/{version}/activation.
// Needs the propose scope. The version must have run in shadow and been evaluated.
func (s *Services) requestRuleActivation(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "propose" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "only the propose scope may ask for a rule to be activated")
		return
	}
	id, ver, ok := ruleVersionParam(w, r)
	if !ok {
		return
	}
	req, ok := decodeRuleReason(w, r)
	if !ok {
		return
	}
	var v rules.Version
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		v, err = s.Rules.Store.RequestActivation(r.Context(), tx, p.TenantID, p.UserID, id, ver, req.Reason)
		return err
	})
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

// decideRuleActivation serves POST /v1/rules/{id}/versions/{version}/decision with
// {"approve": true|false, "reason": "..."}. Needs the approve scope and MFA. The
// decider must be neither the author nor the requester.
func (s *Services) decideRuleActivation(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorize(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "approve" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not approve rules")
		return
	}
	id, ver, ok := ruleVersionParam(w, r)
	if !ok {
		return
	}
	req, ok := decodeRuleReason(w, r)
	if !ok || req.Approve == nil {
		if ok {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "approve (true or false) is required")
		}
		return
	}
	var v rules.Version
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		v, err = s.Rules.Store.Decide(r.Context(), tx, p.TenantID, p.UserID, id, ver, *req.Approve, req.Reason)
		return err
	})
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

// rollbackRule serves POST /v1/rules/{id}/rollback with {"to_version": n, "reason": "..."}.
// It asks for an earlier version to become live again; approval is still needed.
func (s *Services) rollbackRule(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "propose" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "only the propose scope may ask for a rollback")
		return
	}
	req, ok := decodeRuleReason(w, r)
	if !ok {
		return
	}
	if req.ToVersion < 1 {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "to_version is required")
		return
	}
	var v rules.Version
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		v, err = s.Rules.Store.Rollback(r.Context(), tx, p.TenantID, p.UserID, chi.URLParam(r, "id"), req.ToVersion, req.Reason)
		return err
	})
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

// disableRule serves POST /v1/rules/{id}/disable with {"reason": "..."}. It takes the
// live version out of use at once. Needs the propose scope.
func (s *Services) disableRule(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "change_rules", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "propose" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "only the propose scope may disable a rule")
		return
	}
	req, ok := decodeRuleReason(w, r)
	if !ok {
		return
	}
	var v rules.Version
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		v, err = s.Rules.Store.Disable(r.Context(), tx, p.TenantID, p.UserID, chi.URLParam(r, "id"), req.Reason)
		return err
	})
	if err != nil {
		writeRuleError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}
