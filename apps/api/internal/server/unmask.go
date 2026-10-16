package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/datashield"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

const (
	defaultUnmaskLimit  = 10
	defaultUnmaskWindow = 10 * time.Minute
	unmaskAction        = "account.unmask"
)

type unmaskRequest struct {
	Token  string `json:"token"`
	Reason string `json:"reason"`
}

type unmaskResponse struct {
	Token         string `json:"token"`
	AccountNumber string `json:"account_number"`
	Masked        string `json:"masked"`
}

// unmask reveals one account number. The request must come from a role with the
// view_unmasked_account grant, carry a real reason, and pass the MFA check. Each
// call, successful or not, is written to the audit chain before the number is
// released, and a caller is limited to a few reveals per window.
func (s *Services) unmask(w http.ResponseWriter, r *http.Request) {
	var req unmaskRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "request body must be a JSON object with token and reason")
		return
	}
	req.Token, req.Reason = strings.TrimSpace(req.Token), strings.TrimSpace(req.Reason)
	if req.Token == "" || req.Reason == "" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "token and reason are required")
		return
	}

	// The policy decides before anything is looked up, so a caller without the
	// grant learns nothing about whether a token exists.
	p, _, ok := s.authorize(w, r, "view_unmasked_account", authz.Attrs{Reason: req.Reason})
	if !ok {
		return
	}

	limit, window := s.UnmaskLimit, s.UnmaskWindow
	if limit <= 0 {
		limit = defaultUnmaskLimit
	}
	if window <= 0 {
		window = defaultUnmaskWindow
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}

	var (
		account string
		masked  string
		outcome string
	)
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		n, err := audit.CountSince(r.Context(), tx, p.TenantID, p.UserID, unmaskAction, now.Add(-window))
		if err != nil {
			return err
		}
		if n >= limit {
			outcome = "rate_limited" // nothing is revealed and the attempt is not counted again
			return nil
		}

		var rerr error
		account, rerr = s.Vault.Reveal(r.Context(), tx, p.TenantID, req.Token)
		switch {
		case errors.Is(rerr, datashield.ErrUnknownToken):
			outcome = "not_found"
		case rerr != nil:
			outcome = "cannot_open"
		default:
			outcome = "revealed"
			masked = datashield.Mask(account)
		}

		// Written before the response, in the same transaction. If this fails
		// the transaction rolls back and nothing is released.
		_, err = s.Audit.Append(r.Context(), tx, p.TenantID, audit.Event{
			Actor: audit.ActorUser, ActorID: p.UserID, Action: unmaskAction,
			SubjectType: "account", SubjectID: req.Token, Sensitive: true, IP: clientIP(r),
			Details: map[string]any{"reason": req.Reason, "outcome": outcome},
		})
		return err
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}

	switch outcome {
	case "rate_limited":
		w.Header().Set("Retry-After", "60")
		httpx.WriteError(w, r, http.StatusTooManyRequests, httpx.CodeRateLimited, "too many unmask requests; try again later")
	case "not_found":
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such account token")
	case "revealed":
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteJSON(w, http.StatusOK, unmaskResponse{Token: req.Token, AccountNumber: account, Masked: masked})
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
