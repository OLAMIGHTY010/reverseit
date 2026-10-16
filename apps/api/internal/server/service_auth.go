package server

import (
	"net/http"

	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
)

// requireService rejects a request unless it carries a valid assertion from a
// service that holds scope. It is the service-facing counterpart of requireAuth:
// operator tokens do not pass it, and service assertions do not pass requireAuth,
// because their audiences differ. Routes for machine callers, such as event
// ingestion, are mounted behind it.
func requireService(v *servauth.Verifier, scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r)
			if !ok || v == nil {
				unauthorized(w, r)
				return
			}
			p, err := v.Verify(r.Context(), raw, scope)
			if err != nil {
				unauthorized(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(servauth.WithPrincipal(r.Context(), p)))
		})
	}
}
