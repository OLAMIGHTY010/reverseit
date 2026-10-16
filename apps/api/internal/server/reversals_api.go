package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/reversal"
)

type tracksJSON struct {
	Accepted   string `json:"accepted"`
	Processing string `json:"processing"`
	Financial  string `json:"financial"`
}

type reversalJSON struct {
	ID              string `json:"id"`
	ReversalRef     string `json:"reversal_ref"`
	CaseID          string `json:"case_id"`
	TxnID           string `json:"txn_id"`
	AmountMinor     string `json:"amount_minor"`
	Currency        string `json:"currency"`
	ReasonCode      string `json:"reason_code"`
	RequestedByType string `json:"requested_by_type"`
	ApprovalStatus  string `json:"approval_status"`
	ApprovalID      string `json:"approval_id,omitempty"`
	// ApprovalExpiresAt is when the newest approval stops being valid.
	ApprovalExpiresAt *time.Time `json:"approval_expires_at,omitempty"`
	// ParkedAt is set while a freeze holds back submission of an approved reversal.
	ParkedAt      *time.Time    `json:"parked_at,omitempty"`
	Tracks        tracksJSON    `json:"tracks"`
	Gate          reversal.Gate `json:"gate"`
	PolicyID      string        `json:"policy_id"`
	PolicyVersion string        `json:"policy_version"`
	ProviderRef   string        `json:"provider_ref,omitempty"`
	FailureReason string        `json:"failure_reason,omitempty"`
	SubmittedAt   *time.Time    `json:"submitted_at,omitempty"`
	PostedAt      *time.Time    `json:"posted_at,omitempty"`
	VerifiedAt    *time.Time    `json:"verified_at,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	Simulated     bool          `json:"simulated"`
}

func toReversalJSON(r reversal.Reversal) reversalJSON {
	out := reversalJSON{ID: r.ID.String(), ReversalRef: r.Ref, CaseID: r.CaseID.String(), TxnID: r.TxnID.String(),
		AmountMinor: strconv.FormatInt(r.AmountMinor, 10), Currency: r.Currency, ReasonCode: r.ReasonCode, RequestedByType: r.RequestedByType,
		ApprovalStatus: r.ApprovalStatus, Tracks: tracksJSON{r.AcceptedStatus, r.ProcessingStatus, r.FinancialStatus}, Gate: r.Gate,
		ApprovalExpiresAt: r.ApprovalExpiresAt, ParkedAt: r.ParkedAt, PolicyID: r.PolicyID, PolicyVersion: r.PolicyVersion, ProviderRef: r.ProviderRef, FailureReason: r.FailureReason, SubmittedAt: r.SubmittedAt, PostedAt: r.PostedAt, VerifiedAt: r.VerifiedAt,
		CreatedAt: r.CreatedAt, Simulated: true}
	if r.ApprovalID != uuid.Nil {
		out.ApprovalID = r.ApprovalID.String()
	}
	return out
}

// reversalSummary is a reversal as shown inside a case.
type reversalSummary struct {
	ID               string `json:"id"`
	ReversalRef      string `json:"reversal_ref"`
	ApprovalStatus   string `json:"approval_status"`
	AcceptedStatus   string `json:"accepted_status"`
	ProcessingStatus string `json:"processing_status"`
	FinancialStatus  string `json:"financial_status"`
}

func (s *Services) reversalsForCase(r *http.Request, tx pgx.Tx, tenant, caseID uuid.UUID) ([]reversalSummary, error) {
	out := []reversalSummary{}
	if s.Reversal == nil {
		return out, nil
	}
	list, err := s.Reversal.ForCase(r.Context(), tx, tenant, caseID)
	if err != nil {
		return nil, err
	}
	for _, v := range list {
		out = append(out, reversalSummary{v.ID.String(), v.Ref, v.ApprovalStatus, v.AcceptedStatus, v.ProcessingStatus, v.FinancialStatus})
	}
	return out, nil
}

func authedPrincipal(r *http.Request) *identity.Principal { return identity.FromContext(r.Context()) }

func writeReversalError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, reversal.ErrNotFound), errors.Is(err, cases.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "not found")
	case errors.Is(err, reversal.ErrNoApprovalPolicy):
		httpx.WriteError(w, r, http.StatusConflict, "APPROVAL_POLICY_MISSING", "no approved approval validity period is configured, so nothing can be approved")
	case errors.Is(err, reversal.ErrApprovalExpired):
		httpx.WriteError(w, r, http.StatusConflict, "APPROVAL_EXPIRED", "the approval is no longer valid; request a fresh approval")
	case errors.Is(err, reversal.ErrApprovalInvalidated):
		httpx.WriteError(w, r, http.StatusConflict, "APPROVAL_INVALIDATED", "the policy changed since this was approved; request a fresh approval")
	case errors.Is(err, reversal.ErrKeyReuse):
		httpx.WriteError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", err.Error())
	case errors.Is(err, reversal.ErrNotPending), errors.Is(err, reversal.ErrBadState):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, authz.ErrMFARequired):
		httpx.WriteError(w, r, http.StatusForbidden, "MFA_REQUIRED", "multi-factor authentication is required to approve a reversal")
	case errors.Is(err, reversal.ErrSameActor):
		httpx.WriteError(w, r, http.StatusForbidden, "SELF_APPROVAL", "you cannot approve a reversal you requested")
	case errors.Is(err, authz.ErrConflictOfInterest):
		httpx.WriteError(w, r, http.StatusForbidden, "CONFLICT_OF_INTEREST", "you are linked to an account involved and cannot approve this")
	case errors.Is(err, reversal.ErrGateOverrideNotAllowed):
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, err.Error())
	case errors.Is(err, reversal.ErrGateOverrideSameActor):
		httpx.WriteError(w, r, http.StatusForbidden, "SELF_APPROVAL", err.Error())
	case errors.Is(err, reversal.ErrGateOverrideMFA):
		httpx.WriteError(w, r, http.StatusForbidden, "MFA_REQUIRED", err.Error())
	case errors.Is(err, reversal.ErrGateOverrideDecided):
		httpx.WriteError(w, r, http.StatusConflict, httpx.CodeConflict, err.Error())
	case errors.Is(err, authz.ErrForbidden):
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "you may not do this, or the amount is over your approval limit")
	case errors.Is(err, cases.ErrInvalid):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

type reversalRequest struct {
	CaseID      string   `json:"case_id"`
	ReasonCode  string   `json:"reason_code"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type gateFailedBody struct {
	Error        string             `json:"error"`
	Message      string             `json:"message"`
	FailedChecks []reversalCheckErr `json:"failed_checks"`
	Gate         reversal.Gate      `json:"gate"`
}

type reversalCheckErr struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// requestReversal serves POST /v1/reversals. It evaluates the eight checks. If
// they pass it records the reversal and takes the remediation lock; a person's
// request then waits for a different person to approve. Nothing is sent to the
// provider here. If a check fails the answer is 409 with the failed checks and
// nothing is created. A repeat with the same Idempotency-Key returns the same
// reversal.
func (s *Services) requestReversal(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "request_reversal", authz.Attrs{})
	if !ok {
		return
	}
	key, err := httpx.IdempotencyKey(r)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	var req reversalRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "body must be a JSON object with case_id and reason_code")
		return
	}
	caseID, err := uuid.Parse(req.CaseID)
	if err != nil { // a case reference is accepted as well
		var c cases.Case
		perr := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
			var e error
			c, e = s.Cases.GetByRef(r.Context(), tx, p.TenantID, req.CaseID)
			return e
		})
		if perr != nil {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "case_id must be the id or reference of an existing case")
			return
		}
		caseID = c.ID
	}
	prin := p
	out, err := s.Reversal.Request(r.Context(), p.TenantID, reversal.RequestInput{CaseID: caseID, IdempotencyKey: key,
		ReasonCode: req.ReasonCode, Who: reversal.Requester{Principal: &prin}})
	if err != nil {
		writeReversalError(w, r, err)
		return
	}
	if out.Blocked {
		body := gateFailedBody{Error: "GATE_FAILED", Message: "one or more reversal checks failed; nothing was done", Gate: out.Gate}
		for _, c := range out.Gate.Failed() {
			body.FailedChecks = append(body.FailedChecks, reversalCheckErr{c.ID, c.Name, c.Detail})
		}
		httpx.WriteJSON(w, http.StatusConflict, body)
		return
	}
	code := http.StatusAccepted
	if out.Existing {
		code = http.StatusOK
	}
	httpx.WriteJSON(w, code, toReversalJSON(out.Reversal))
}

// getReversal serves GET /v1/reversals/{id}: the three tracks, separately.
func (s *Services) getReversal(w http.ResponseWriter, r *http.Request) {
	p, ok := s.canReadQueue(w, r)
	if !ok {
		return
	}
	var rev reversal.Reversal
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var e error
		rev, e = s.Reversal.Get(r.Context(), tx, p.TenantID, pathParam(r, "id"))
		return e
	})
	if err != nil {
		writeReversalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toReversalJSON(rev))
}

type decisionBody struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// decideApproval serves POST /v1/approvals/{id}/decision. Approving needs the
// approve_reversal grant within the caller's limit, an MFA-backed token, and a
// caller who is not the requester. An approval then carries the reversal out
// against the simulated provider and verifies it against the ledger.
func (s *Services) decideApproval(w http.ResponseWriter, r *http.Request) {
	p := authedPrincipal(r)
	if p == nil {
		unauthorized(w, r)
		return
	}
	id, err := uuid.Parse(pathParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "not found")
		return
	}
	var body decisionBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil || (body.Decision != "approved" && body.Decision != "rejected") {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "body must be a JSON object with decision (approved or rejected) and reason")
		return
	}
	rev, err := s.Reversal.Decide(r.Context(), p.TenantID, p, id, body.Decision == "approved", body.Reason)
	if err != nil {
		writeReversalError(w, r, err)
		return
	}
	if body.Decision == "approved" {
		if rev, err = s.Reversal.Execute(r.Context(), p.TenantID, rev.ID); err != nil {
			writeReversalError(w, r, err)
			return
		}
		if rev, err = s.Reversal.Advance(r.Context(), p.TenantID, rev.ID); err != nil {
			writeReversalError(w, r, err)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, toReversalJSON(rev))
}

// advanceReversal serves POST /v1/reversals/{id}/advance: ask the provider where
// it stands and verify against the ledger when it says done. Safe to repeat.
func (s *Services) advanceReversal(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "request_reversal", authz.Attrs{})
	if !ok {
		return
	}
	var rev reversal.Reversal
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var e error
		rev, e = s.Reversal.Get(r.Context(), tx, p.TenantID, pathParam(r, "id"))
		return e
	})
	if err == nil {
		rev, err = s.Reversal.Advance(r.Context(), p.TenantID, rev.ID)
	}
	if err != nil {
		writeReversalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toReversalJSON(rev))
}

// reversalEligibility serves GET /v1/cases/{id}/reversal-eligibility: the eight
// checks as they stand now, with nothing changed.
func (s *Services) reversalEligibility(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "request_reversal", authz.Attrs{})
	if !ok {
		return
	}
	prin := p
	var g reversal.Gate
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		c, e := s.resolveCase(r, tx, p.TenantID, pathParam(r, "id"))
		if e != nil {
			return e
		}
		g, e = s.Reversal.Assess(r.Context(), tx, p.TenantID, c.ID, reversal.Requester{Principal: &prin})
		return e
	})
	if err != nil {
		writeReversalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, g)
}

type approvalJSON struct {
	ApprovalID string       `json:"approval_id"`
	Status     string       `json:"status"`
	Reason     string       `json:"reason"`
	CreatedAt  time.Time    `json:"created_at"`
	CaseRef    string       `json:"case_ref"`
	Reversal   reversalJSON `json:"reversal"`
	Simulated  bool         `json:"simulated"`
}

// listApprovals serves GET /v1/approvals, the approval queue.
func (s *Services) listApprovals(w http.ResponseWriter, r *http.Request) {
	p, ok := s.canReadQueue(w, r)
	if !ok {
		return
	}
	status := strings.ToUpper(r.URL.Query().Get("status"))
	if status == "" {
		status = "PENDING"
	}
	if status != "PENDING" && status != "APPROVED" && status != "REJECTED" {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "status must be PENDING, APPROVED or REJECTED")
		return
	}
	items := []approvalJSON{}
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT a.id, a.status, a.reason, a.created_at, r.id, c.case_ref FROM approvals a
			JOIN reversals r ON r.tenant_id = a.tenant_id AND r.id = a.reversal_id JOIN cases c ON c.tenant_id = r.tenant_id AND c.id = r.case_id
			WHERE a.status = $1 ORDER BY a.created_at LIMIT 200`, status)
		if err != nil {
			return err
		}
		type row struct {
			a     approvalJSON
			revID uuid.UUID
		}
		var list []row
		for rows.Next() {
			var x row
			var aid uuid.UUID
			if err := rows.Scan(&aid, &x.a.Status, &x.a.Reason, &x.a.CreatedAt, &x.revID, &x.a.CaseRef); err != nil {
				rows.Close()
				return err
			}
			x.a.ApprovalID, x.a.Simulated = aid.String(), true
			list = append(list, x)
		}
		rows.Close()
		for _, x := range list {
			rev, err := s.Reversal.Get(r.Context(), tx, p.TenantID, x.revID.String())
			if err != nil {
				return err
			}
			x.a.Reversal = toReversalJSON(rev)
			items = append(items, x.a)
		}
		return nil
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

type proposeOverrideRequest struct {
	Justification string `json:"justification"`
}

type decideOverrideRequest struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

type gateOverrideJSON struct {
	ID             string     `json:"id"`
	ReversalID     string     `json:"reversal_id"`
	ProposerID     string     `json:"proposer_id"`
	ApproverID     string     `json:"approver_id,omitempty"`
	Justification  string     `json:"justification"`
	Status         string     `json:"status"`
	DecisionReason string     `json:"decision_reason,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
}

func toGateOverrideJSON(o reversal.GateOverride) gateOverrideJSON {
	out := gateOverrideJSON{
		ID:             o.ID.String(),
		ReversalID:     o.ReversalID.String(),
		ProposerID:     o.ProposerID.String(),
		Justification:  o.Justification,
		Status:         o.Status,
		DecisionReason: o.DecisionReason,
		CreatedAt:      o.CreatedAt,
		DecidedAt:      o.DecidedAt,
	}
	if o.ApproverID != nil {
		out.ApproverID = o.ApproverID.String()
	}
	return out
}

// proposeGateOverride serves POST /v1/reversals/{id}/override-proposal.
func (s *Services) proposeGateOverride(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.scopeAtLeast(w, r, "override_gate", "override_dual")
	if !ok {
		return
	}
	revID, err := uuid.Parse(pathParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "id must be a UUID")
		return
	}
	var req proposeOverrideRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "invalid request body")
		return
	}
	res, err := s.Reversal.ProposeGateOverride(r.Context(), p.TenantID, revID, &p, req.Justification)
	if err != nil {
		writeReversalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toGateOverrideJSON(res))
}

// decideGateOverride serves POST /v1/reversals/overrides/{id}/decision.
func (s *Services) decideGateOverride(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.scopeAtLeast(w, r, "override_gate", "override_dual")
	if !ok {
		return
	}
	overrideID, err := uuid.Parse(pathParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "id must be a UUID")
		return
	}
	var req decideOverrideRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "invalid request body")
		return
	}
	res, err := s.Reversal.DecideGateOverride(r.Context(), p.TenantID, overrideID, &p, req.Approve, req.Reason)
	if err != nil {
		writeReversalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toGateOverrideJSON(res))
}

type recoverTimeoutsResponse struct {
	ProcessedCount int                       `json:"processed_count"`
	Results        []reversal.RecoveryResult `json:"results"`
}

// recoverTimeouts serves POST /v1/reversals/recover-timeouts.
func (s *Services) recoverTimeouts(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.scopeAtLeast(w, r, "view_exception_queue", "full")
	if !ok {
		return
	}
	results, err := s.Reversal.RecoverTimeouts(r.Context(), p.TenantID, 5*time.Minute)
	if err != nil {
		writeReversalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, recoverTimeoutsResponse{
		ProcessedCount: len(results),
		Results:        results,
	})
}

// requestReapproval serves POST /v1/reversals/{id}/reapproval, the safe route back
// after an approval lapsed. The checks run again, the requester becomes the maker,
// and a different person with the authority must approve. Nothing is submitted.
func (s *Services) requestReapproval(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "request_reversal", authz.Attrs{})
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "body must be a JSON object with a reason")
		return
	}
	var rev reversal.Reversal
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var e error
		rev, e = s.Reversal.Get(r.Context(), tx, p.TenantID, pathParam(r, "id"))
		return e
	})
	if err == nil {
		prin := p
		rev, err = s.Reversal.RequestReapproval(r.Context(), p.TenantID, &prin, rev.ID, body.Reason)
	}
	var blocked *reversal.BlockedError
	if errors.As(err, &blocked) {
		resp := gateFailedBody{Error: "GATE_FAILED", Message: "the checks no longer pass; nothing was done", Gate: blocked.Gate}
		for _, c := range blocked.Gate.Failed() {
			resp.FailedChecks = append(resp.FailedChecks, reversalCheckErr{c.ID, c.Name, c.Detail})
		}
		httpx.WriteJSON(w, http.StatusConflict, resp)
		return
	}
	if err != nil {
		writeReversalError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, toReversalJSON(rev))
}
