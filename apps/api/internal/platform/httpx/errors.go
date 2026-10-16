// Package httpx holds HTTP helpers shared by every handler: the error
// envelope, JSON responses and request decoding.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/smartreversal/smartreversal/apps/api/internal/observability"
)

// Error codes used across the API. The code is a stable machine-readable
// string, the message is for people.
const (
	CodeBadRequest     = "BAD_REQUEST"
	CodeUnauthorized   = "UNAUTHORIZED"
	CodeForbidden      = "FORBIDDEN"
	CodeNotFound       = "NOT_FOUND"
	CodeConflict       = "CONFLICT"
	CodeValidation     = "VALIDATION_FAILED"
	CodeRateLimited    = "RATE_LIMITED"
	CodeNotImplemented = "NOT_IMPLEMENTED"
	CodeInternal       = "INTERNAL_ERROR"
	CodeUnavailable    = "UNAVAILABLE"
)

// maxBodyBytes caps JSON request bodies.
const maxBodyBytes = 1 << 20

// ErrorBody is the single error shape returned by the API (decision D16).
// Extra fields such as failed_checks are added by specific endpoints.
type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	TraceID string `json:"trace_id,omitempty"`
	Details []any  `json:"details,omitempty"`
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes the error envelope. Messages must never contain secrets,
// account numbers or internal error text.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details ...any) {
	WriteJSON(w, status, ErrorBody{
		Error:   code,
		Message: message,
		TraceID: observability.TraceID(r.Context()),
		Details: details,
	})
}

// DecodeJSON reads a single JSON object from the request body. Unknown fields,
// trailing data and bodies over 1 MiB are rejected.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

// NotFound and MethodNotAllowed give the router's default responses the
// standard envelope.
func NotFound(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, http.StatusNotFound, CodeNotFound, "resource not found")
}

func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	WriteError(w, r, http.StatusMethodNotAllowed, CodeBadRequest, "method not allowed")
}
