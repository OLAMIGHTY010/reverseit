package server

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/feed"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
)

// importFeedFile serves POST /v1/feeds/{mapping}/files. The body is the file itself.
// A service holding events:write delivers it; the tenant comes from the service's
// account. 201 the file was imported (rows that were refused are listed and kept in
// quarantine), 200 these exact bytes were imported before, 404 no active mapping of
// that name, 413 too large, 422 the file cannot be read with the mapping.
func (s *Services) importFeedFile(w http.ResponseWriter, r *http.Request) {
	svc := servauth.FromContext(r.Context())
	if svc == nil {
		unauthorized(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, feed.MaxFileBytes))
	if err != nil {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "the file is too large")
		return
	}
	res, err := s.Feed.Import(r.Context(), svc.TenantID, chi.URLParam(r, "mapping"), body)
	switch {
	case err == nil:
	case errors.Is(err, feed.ErrNoMapping):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "there is no active mapping with that name")
		return
	case errors.Is(err, feed.ErrMapping), errors.Is(err, feed.ErrUnreadable):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
		return
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	code := http.StatusCreated
	if res.AlreadySeen {
		code = http.StatusOK
	}
	httpx.WriteJSON(w, code, res)
}
