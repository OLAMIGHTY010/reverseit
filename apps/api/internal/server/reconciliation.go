package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/reconciliation"
)

// maxSettlementFileSize is 10 MB.
const maxSettlementFileSize = 10 << 20

func (s *Services) uploadSettlementFile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.canReadQueue(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSettlementFileSize)
	if err := r.ParseMultipartForm(maxSettlementFileSize); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "settlement file exceeds 10MB limit")
			return
		}
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "invalid multipart form: "+err.Error())
		return
	}

	provider := r.FormValue("provider")
	kind := r.FormValue("kind")
	periodStart := r.FormValue("period_start")
	periodEnd := r.FormValue("period_end")
	mappingVersion := r.FormValue("mapping_version")

	if provider == "" || kind == "" || periodStart == "" || periodEnd == "" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "provider, kind, period_start, and period_end are required")
		return
	}

	if kind != "SETTLEMENT" && kind != "LEDGER_STATEMENT" && kind != "SWITCH_RESPONSE" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "kind must be SETTLEMENT, LEDGER_STATEMENT, or SWITCH_RESPONSE")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "file field is required in multipart form")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "failed reading uploaded file: "+err.Error())
		return
	}

	summary, err := s.Reconciliation.UploadAndReconcile(r.Context(), p.TenantID, reconciliation.UploadInput{
		Provider:       provider,
		Kind:           kind,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		MappingVersion: mappingVersion,
		Filename:       header.Filename,
		Content:        content,
		Tolerance:      reconciliation.DefaultTolerance,
	})
	if err != nil {
		switch {
		case errors.Is(err, reconciliation.ErrDuplicateFile):
			httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "duplicate settlement file for this provider and period")
			return
		case errors.Is(err, reconciliation.ErrEmptyFile), errors.Is(err, reconciliation.ErrInvalidHeader), errors.Is(err, reconciliation.ErrInvalidRecord):
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
			return
		default:
			httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "reconciliation error: "+err.Error())
			return
		}
	}

	httpx.WriteJSON(w, http.StatusAccepted, summary)
}

func (s *Services) listReconBreaks(w http.ResponseWriter, r *http.Request) {
	p, ok := s.canReadQueue(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	breakType := r.URL.Query().Get("break_type")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	breaks, err := s.Reconciliation.ListBreaks(r.Context(), p.TenantID, status, breakType, limit)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "failed querying reconciliation breaks: "+err.Error())
		return
	}
	if breaks == nil {
		breaks = []reconciliation.Break{}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items":     breaks,
		"count":     len(breaks),
		"simulated": true,
	})
}

func (s *Services) pairReconBreak(w http.ResponseWriter, r *http.Request) {
	p, ok := s.canReadQueue(w, r)
	if !ok {
		return
	}

	idStr := chi.URLParam(r, "id")
	breakID, err := uuid.Parse(idStr)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "invalid break id")
		return
	}

	var body struct {
		Notes string `json:"notes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Notes == "" {
		body.Notes = "Manual pairing confirmed by analyst"
	}

	updated, err := s.Reconciliation.PairBreak(r.Context(), p.TenantID, breakID, p.UserID.String(), body.Notes)
	if err != nil {
		if errors.Is(err, reconciliation.ErrBreakNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "break not found")
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "failed pairing break: "+err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, updated)
}
