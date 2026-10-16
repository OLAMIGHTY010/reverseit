package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

// requireAuth rejects a request unless it carries a valid token for audience.
// It fails closed: any problem with the token is a 401 with no detail. On
// success the principal is stored in the request context.
func requireAuth(v *identity.Verifier, audience string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r)
			if !ok || v == nil {
				unauthorized(w, r)
				return
			}
			p, err := v.Verify(raw, audience)
			if err != nil {
				unauthorized(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(identity.WithPrincipal(r.Context(), p)))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	scheme, token, found := strings.Cut(h, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t") {
		return "", false
	}
	return token, true
}

func unauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="smartreversal"`)
	httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
}

type loginRequest struct {
	Institution string `json:"institution"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	// OTP is the six-digit code from the development authenticator. Only a
	// login with a valid code yields a token that can approve.
	OTP string `json:"otp,omitempty"`
}

type loginResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	UserID      string    `json:"user_id"`
	Roles       []string  `json:"roles"`
	MFA         bool      `json:"mfa"`
}

// login exchanges credentials for an operator token.
func (s *authRoutes) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "request body must be a JSON object with institution, email and password")
		return
	}
	if req.Institution == "" || req.Email == "" || req.Password == "" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "institution, email and password are required")
		return
	}

	res, err := s.auth.LoginOTP(r.Context(), req.Institution, req.Email, req.Password, req.OTP)
	switch {
	case errors.Is(err, identity.ErrInvalidCredentials):
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteError(w, r, http.StatusUnauthorized, httpx.CodeUnauthorized, "invalid credentials")
		return
	case err != nil:
		s.logger.ErrorContext(r.Context(), "login_failed", "error", err)
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, loginResponse{
		AccessToken: res.Token,
		TokenType:   "Bearer",
		ExpiresAt:   res.ExpiresAt,
		UserID:      res.UserID.String(),
		Roles:       res.Roles,
		MFA:         res.MFA,
	})
}

type meResponse struct {
	UserID    string    `json:"user_id"`
	TenantID  string    `json:"tenant_id"`
	Roles     []string  `json:"roles"`
	MFA       bool      `json:"mfa"`
	ExpiresAt time.Time `json:"expires_at"`
}

// me returns the caller's own identity.
func me(w http.ResponseWriter, r *http.Request) {
	p := identity.FromContext(r.Context())
	if p == nil { // unreachable behind requireAuth; fail closed anyway
		unauthorized(w, r)
		return
	}
	roles := p.Roles
	if roles == nil {
		roles = []string{}
	}
	httpx.WriteJSON(w, http.StatusOK, meResponse{
		UserID: p.UserID.String(), TenantID: p.TenantID.String(), Roles: roles, MFA: p.MFA, ExpiresAt: p.ExpiresAt,
	})
}
