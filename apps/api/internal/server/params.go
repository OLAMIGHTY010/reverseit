package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// pathParam returns a route parameter.
func pathParam(r *http.Request, name string) string { return chi.URLParam(r, name) }
