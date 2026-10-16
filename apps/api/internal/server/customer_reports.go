package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/creport"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

const maxReportBody = 16 << 10

func writeReportError(w http.ResponseWriter, r *http.Request, err error) {
	var rl *creport.RateLimitedError
	switch {
	case errors.As(err, &rl):
		secs := int(rl.RetryAfter.Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		httpx.WriteError(w, r, http.StatusTooManyRequests, httpx.CodeRateLimited, "You have sent several reports recently. Please try again in about "+strconv.Itoa((secs+59)/60)+" minute(s).")
	case errors.Is(err, creport.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
	case errors.Is(err, creport.ErrConsent):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "CONSENT_REQUIRED", "We need your consent to read the alert you share.")
	case errors.Is(err, creport.ErrSubmitted):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, "this report was already submitted")
	case errors.Is(err, creport.ErrNotAccepted), errors.Is(err, creport.ErrInvalid):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

func customerFrom(w http.ResponseWriter, r *http.Request) (*identity.Principal, bool) {
	p := identity.FromContext(r.Context())
	if p == nil {
		unauthorized(w, r)
		return nil, false
	}
	return p, true
}

type createReportRequest struct {
	Text           string `json:"text"`
	ImageRef       string `json:"image_ref"`
	Consent        bool   `json:"consent"`
	ConsentVersion string `json:"consent_version"`
}

// createCustomerReport serves POST /v1/customer/reports. 201 a new report, 200 the same
// Idempotency-Key was used before, 422 invalid or no consent, 429 too many reports.
func (s *Services) createCustomerReport(w http.ResponseWriter, r *http.Request) {
	p, ok := customerFrom(w, r)
	if !ok {
		return
	}
	key, err := httpx.IdempotencyKey(r)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	var req createReportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBody)).Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "the body must be a JSON object with text or image_ref, consent and consent_version")
		return
	}
	rep, err := s.Reports.Create(r.Context(), p.TenantID, p.UserID, creport.CreateInput{IdempotencyKey: key, Text: req.Text, ImageRef: req.ImageRef, Consent: req.Consent, ConsentVersion: req.ConsentVersion})
	if err != nil {
		writeReportError(w, r, err)
		return
	}
	code := http.StatusCreated
	if rep.Idempotent {
		code = http.StatusOK
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, code, rep)
}

// listCustomerReports serves GET /v1/customer/reports.
func (s *Services) listCustomerReports(w http.ResponseWriter, r *http.Request) {
	p, ok := customerFrom(w, r)
	if !ok {
		return
	}
	list, err := s.Reports.List(r.Context(), p.TenantID, p.UserID)
	if err != nil {
		writeReportError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": list})
}

func reportID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
		return uuid.Nil, false
	}
	return id, true
}

// getCustomerReport serves GET /v1/customer/reports/{id}. Another customer's report is a 404.
func (s *Services) getCustomerReport(w http.ResponseWriter, r *http.Request) {
	p, ok := customerFrom(w, r)
	if !ok {
		return
	}
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	rep, err := s.Reports.Get(r.Context(), p.TenantID, p.UserID, id)
	if err != nil {
		writeReportError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, rep)
}

type confirmReportRequest struct {
	AmountMinor          txn.Amount `json:"amount_minor"`
	OccurredAt           time.Time  `json:"occurred_at"`
	AccountTail          string     `json:"account_tail"`
	Reference            string     `json:"reference"`
	Channel              string     `json:"channel"`
	Narration            string     `json:"narration"`
	CandidateTransaction string     `json:"candidate_transaction_id"`
}

// confirmCustomerReport serves POST /v1/customer/reports/{id}/confirm. The customer
// confirms or edits the fields, and the server validates each again before matching.
// Repeating it with a chosen candidate_transaction_id picks one of the offered
// candidates.
func (s *Services) confirmCustomerReport(w http.ResponseWriter, r *http.Request) {
	p, ok := customerFrom(w, r)
	if !ok {
		return
	}
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxReportBody))
	if err != nil {
		httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "the body is too large")
		return
	}
	var req confirmReportRequest
	if err := json.Unmarshal(body, &req); err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "the fields are not valid: "+err.Error())
		return
	}
	in := creport.ConfirmInput{AmountMinor: int64(req.AmountMinor), OccurredAt: req.OccurredAt, AccountTail: req.AccountTail, Reference: req.Reference, Channel: req.Channel, Narration: req.Narration}
	if req.CandidateTransaction != "" {
		cid, err := uuid.Parse(req.CandidateTransaction)
		if err != nil {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "candidate_transaction_id must be an id")
			return
		}
		in.CandidateTxnID = &cid
	}
	rep, err := s.Reports.Confirm(r.Context(), p.TenantID, p.UserID, id, in)
	if err != nil {
		writeReportError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, rep)
}

// customerReportDraft serves GET /v1/customer/reports/{id}/draft?dispute_type=. It
// proposes the kind of problem, shows the deadline that implies and the complaint
// text. It writes nothing. An ATM report must say whose machine it was.
func (s *Services) customerReportDraft(w http.ResponseWriter, r *http.Request) {
	p, ok := customerFrom(w, r)
	if !ok {
		return
	}
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	d, err := s.Reports.MakeDraft(r.Context(), p.TenantID, p.UserID, id, r.URL.Query().Get("dispute_type"))
	if err != nil {
		writeReportError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, d)
}

// customerReportDeadline serves GET /v1/customer/reports/{id}/deadline: the clock and
// countdown, with the regulatory deadline and the bank's own target kept apart, the
// source of each and the registry status of the clock.
func (s *Services) customerReportDeadline(w http.ResponseWriter, r *http.Request) {
	p, ok := customerFrom(w, r)
	if !ok {
		return
	}
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	d, err := s.Reports.MakeDraft(r.Context(), p.TenantID, p.UserID, id, r.URL.Query().Get("dispute_type"))
	if err != nil {
		writeReportError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, d.Deadline)
}

type complaintRequestBody struct {
	DisputeType             string `json:"dispute_type"`
	Text                    string `json:"text"`
	DisputesVerifiedOutcome bool   `json:"disputes_verified_outcome"`
}

// editCustomerComplaint serves PUT /v1/customer/reports/{id}/complaint: save the
// customer's own wording before submitting. The original draft is always kept.
func (s *Services) editCustomerComplaint(w http.ResponseWriter, r *http.Request) {
	p, ok := customerFrom(w, r)
	if !ok {
		return
	}
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	var req complaintRequestBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBody)).Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "the body must be a JSON object with text")
		return
	}
	if err := s.Reports.SaveEdit(r.Context(), p.TenantID, p.UserID, id, req.Text); err != nil {
		writeReportError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// submitCustomerComplaint serves POST /v1/customer/reports/{id}/complaint: send the
// report to the bank. It opens CUS001, or CUS002 for a wrong account or amount, on the
// shared queue, or adds the report to the case that already exists. Repeating it
// returns the same case. 409 NOT_READY until the details are confirmed and a match chosen.
func (s *Services) submitCustomerComplaint(w http.ResponseWriter, r *http.Request) {
	p, ok := customerFrom(w, r)
	if !ok {
		return
	}
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	var req complaintRequestBody
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBody)).Decode(&req); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "the body must be a JSON object")
			return
		}
	}
	out, err := s.Reports.Submit(r.Context(), p.TenantID, p.UserID, id, creport.SubmitInput{DisputeType: req.DisputeType, Text: req.Text, DisputesVerifiedOutcome: req.DisputesVerifiedOutcome})
	if errors.Is(err, creport.ErrNotReady) {
		httpx.WriteError(w, r, http.StatusConflict, "NOT_READY", err.Error())
		return
	}
	if err != nil {
		writeReportError(w, r, err)
		return
	}
	code := http.StatusCreated
	if !out.Created && !out.Joined {
		code = http.StatusOK
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, code, out)
}

type feedbackRequest struct {
	Helpful         *bool  `json:"helpful"`
	Comment         string `json:"comment"`
	DisputesOutcome bool   `json:"disputes_outcome"`
}

// customerFeedback serves POST /v1/customer/reports/{id}/feedback. Idempotency-Key is
// required. Saying the value was not received reopens a closed case; it changes no
// financial fact.
func (s *Services) customerFeedback(w http.ResponseWriter, r *http.Request) {
	p, ok := customerFrom(w, r)
	if !ok {
		return
	}
	id, ok := reportID(w, r)
	if !ok {
		return
	}
	key, err := httpx.IdempotencyKey(r)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	var req feedbackRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReportBody)).Decode(&req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "the body must be a JSON object with helpful, comment or disputes_outcome")
		return
	}
	out, err := s.Reports.Feedback(r.Context(), p.TenantID, p.UserID, id, creport.FeedbackInput{IdempotencyKey: key, Helpful: req.Helpful, Comment: req.Comment, DisputesOutcome: req.DisputesOutcome})
	if errors.Is(err, creport.ErrNotReady) {
		httpx.WriteError(w, r, http.StatusConflict, "NOT_READY", err.Error())
		return
	}
	if err != nil {
		writeReportError(w, r, err)
		return
	}
	code := http.StatusCreated
	if out.Repeat {
		code = http.StatusOK
	}
	httpx.WriteJSON(w, code, out)
}
