package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/customer"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

type customerLoginRequest struct {
	Institution string `json:"institution"`
	CustomerRef string `json:"customer_ref"`
	Passphrase  string `json:"passphrase"`
}

type customerLoginResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	CustomerRef string    `json:"customer_ref"`
	Simulated   bool      `json:"simulated"`
}

// customerLogin serves POST /v1/customer/auth/login. It exists only when a customer
// identity provider is configured, which in this build means the development one for
// synthetic customers. The token has the customer audience and works on no operator
// route.
func (s *Services) customerLogin(w http.ResponseWriter, r *http.Request) {
	var req customerLoginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil || req.Institution == "" || req.CustomerRef == "" || req.Passphrase == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "the body must be a JSON object with institution, customer_ref and passphrase")
		return
	}
	res, err := s.CustomerLogin.SignIn(r.Context(), req.Institution, req.CustomerRef, req.Passphrase, time.Now())
	switch {
	case err == nil:
	case errors.Is(err, customer.ErrInvalidCredentials):
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "invalid credentials")
		return
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, customerLoginResponse{AccessToken: res.Token, TokenType: "Bearer", ExpiresAt: res.ExpiresAt, CustomerRef: res.Customer.Ref, Simulated: true})
}

type customerMe struct {
	CustomerRef string `json:"customer_ref"`
	DisplayName string `json:"display_name"`
	Accounts    int    `json:"linked_accounts"`
}

// customerMe serves GET /v1/customer/me.
func (s *Services) customerMe(w http.ResponseWriter, r *http.Request) {
	p := identity.FromContext(r.Context())
	if p == nil {
		unauthorized(w, r)
		return
	}
	var out customerMe
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		c, err := customer.Get(r.Context(), tx, p.TenantID, p.UserID)
		if err != nil {
			return err
		}
		accts, err := customer.Accounts(r.Context(), tx, p.TenantID, p.UserID)
		out = customerMe{CustomerRef: c.Ref, DisplayName: c.DisplayName, Accounts: len(accts)}
		return err
	})
	if errors.Is(err, customer.ErrNotFound) {
		unauthorized(w, r) // a token for a customer who is no longer active
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
