package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

type caseJSON struct {
	ID              string     `json:"id"`
	CaseRef         string     `json:"case_ref"`
	Classification  string     `json:"classification"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	RiskGrade       string     `json:"risk_grade,omitempty"`
	Source          string     `json:"source"`
	TxnID           string     `json:"txn_id,omitempty"`
	ExposureMinor   string     `json:"exposure_minor,omitempty"`
	Currency        string     `json:"currency,omitempty"`
	RootCause       string     `json:"root_cause,omitempty"`
	OwnerID         string     `json:"owner_id,omitempty"`
	Queue           string     `json:"queue"`
	RuleID          string     `json:"rule_id,omitempty"`
	RuleVersion     string     `json:"rule_version,omitempty"`
	RegulatoryClock string     `json:"regulatory_clock,omitempty"`
	OpenedAt        time.Time  `json:"opened_at"`
	SLADueAt        *time.Time `json:"sla_due_at,omitempty"`
	InternalTarget  *time.Time `json:"internal_target_at,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	ClosedReason    string     `json:"closed_reason,omitempty"`
	Version         int        `json:"version"`
	Simulated       bool       `json:"simulated"`
}

func toCaseJSON(c cases.Case) caseJSON {
	out := caseJSON{ID: c.ID.String(), CaseRef: c.Ref, Classification: c.Classification, Status: c.Status, Priority: c.Priority,
		RiskGrade: c.RiskGrade, Source: c.Source, Currency: c.Currency, RootCause: c.RootCause, Queue: c.Queue, RuleID: c.RuleID,
		RuleVersion: c.RuleVersion, RegulatoryClock: c.RegulatoryClock, OpenedAt: c.OpenedAt, SLADueAt: c.SLADueAt, InternalTarget: c.InternalTargetAt, ResolvedAt: c.ResolvedAt,
		ClosedReason: c.ClosedReason, Version: c.Version, Simulated: true}
	if c.TxnID != nil {
		out.TxnID = c.TxnID.String()
	}
	if c.ExposureMinor != nil {
		out.ExposureMinor = strconv.FormatInt(*c.ExposureMinor, 10)
	}
	if c.OwnerID != nil {
		out.OwnerID = c.OwnerID.String()
	}
	if c.SLADueAt != nil {
		t := c.SLADueAt.UTC()
		out.SLADueAt = &t
	}
	if c.ResolvedAt != nil {
		t := c.ResolvedAt.UTC()
		out.ResolvedAt = &t
	}
	return out
}

func writeCaseError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, cases.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "no such case")
	case errors.Is(err, cases.ErrInvalid):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
	case errors.Is(err, cases.ErrBadMove):
		httpx.WriteError(w, r, http.StatusConflict, "BAD_TRANSITION", err.Error())
	case errors.Is(err, cases.ErrStaleVersion):
		httpx.WriteError(w, r, http.StatusPreconditionFailed, "STALE_VERSION", "the case changed since you read it; fetch it again")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

func (s *Services) canReadQueue(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	p, grant, ok := s.authorizeNoMFA(w, r, "view_exception_queue", authz.Attrs{})
	if !ok {
		return p, false
	}
	switch grant.Scope {
	case "full", "metadata", "own_customer":
		return p, true
	}
	httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not view cases")
	return p, false
}

func actorOf(p identity.Principal) cases.Actor {
	return cases.Actor{Type: audit.ActorUser, ID: p.UserID}
}

// listExceptions serves GET /v1/exceptions, the shared queue.
func (s *Services) listExceptions(w http.ResponseWriter, r *http.Request) {
	p, ok := s.canReadQueue(w, r)
	if !ok {
		return
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
		return
	}
	var after *cases.Cursor
	if page.Cursor != "" {
		parts := strings.SplitN(page.Cursor, "|", 3)
		rank, e1 := strconv.Atoi(parts[0])
		if len(parts) != 3 || e1 != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "cursor is invalid")
			return
		}
		at, e2 := time.Parse(time.RFC3339Nano, parts[1])
		if e2 != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "cursor is invalid")
			return
		}
		after = &cases.Cursor{Rank: rank, OpenedAt: at, ID: parts[2]}
	}
	q := r.URL.Query()
	f := cases.Filter{Status: q.Get("status"), Classification: q.Get("classification"), Source: q.Get("source")}
	if v := q.Get("txn_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "txn_id must be a UUID")
			return
		}
		f.TxnID = &id
	}
	var list []cases.Case
	err = s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		list, err = s.Cases.List(r.Context(), tx, p.TenantID, f, after, page.Limit+1)
		return err
	})
	if err != nil {
		writeCaseError(w, r, err)
		return
	}
	items := make([]caseJSON, 0, len(list))
	for _, c := range list {
		items = append(items, toCaseJSON(c))
	}
	byID := map[string]cases.Case{}
	for _, c := range list {
		byID[c.ID.String()] = c
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(items, page.Limit, func(last caseJSON) string {
		cur := cases.CursorFor(byID[last.ID])
		return strconv.Itoa(cur.Rank) + "|" + cur.OpenedAt.UTC().Format(time.RFC3339Nano) + "|" + cur.ID
	}))
}

// resolveCase finds a case by uuid or by reference such as CASE-2026-000012.
func (s *Services) resolveCase(r *http.Request, tx pgx.Tx, tenant uuid.UUID, idOrRef string) (cases.Case, error) {
	if id, err := uuid.Parse(idOrRef); err == nil {
		return s.Cases.Get(r.Context(), tx, tenant, id)
	}
	return s.Cases.GetByRef(r.Context(), tx, tenant, idOrRef)
}

type caseDetail struct {
	Case          caseJSON          `json:"case"`
	Evidence      []cases.Evidence  `json:"evidence"`
	History       []cases.Event     `json:"history"`
	Transaction   *transactionJSON  `json:"transaction,omitempty"`
	Notifications []notify.Stored   `json:"notifications"`
	Reversals     []reversalSummary `json:"reversals"`
}

// getCase serves GET /v1/cases/{id}.
func (s *Services) getCase(w http.ResponseWriter, r *http.Request) {
	p, ok := s.canReadQueue(w, r)
	if !ok {
		return
	}
	var out caseDetail
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		c, err := s.resolveCase(r, tx, p.TenantID, pathParam(r, "id"))
		if err != nil {
			return err
		}
		out.Case = toCaseJSON(c)
		if out.Evidence, err = s.Cases.Evidence(r.Context(), tx, p.TenantID, c.ID); err != nil {
			return err
		}
		if out.History, err = s.Cases.History(r.Context(), tx, p.TenantID, c.ID); err != nil {
			return err
		}
		if out.Notifications, err = notify.ForCase(r.Context(), tx, p.TenantID, c.ID, ""); err != nil {
			return err
		}
		if c.TxnID != nil {
			t, err := scanTxn(tx.QueryRow(r.Context(), `SELECT `+txnColumns+` FROM transactions WHERE id = $1`, *c.TxnID))
			if err == nil {
				out.Transaction = &t
			}
		}
		out.Reversals, err = s.reversalsForCase(r, tx, p.TenantID, c.ID)
		return err
	})
	if err != nil {
		writeCaseError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type patchCaseRequest struct {
	Status     string `json:"status"`
	ReasonCode string `json:"reason_code"`
	OwnerID    string `json:"owner_id"`
}

// Statuses that only the reversal workflow may set, because each one claims a
// fact about money that a person's say-so cannot establish.
var reversalOnly = map[string]bool{
	cases.StRemediationSubmitted: true,
	cases.StOutcomeVerification:  true,
	cases.StResolved:             true,
	cases.StRemediationFailed:    true,
}

// patchCase serves PATCH /v1/cases/{id}. If-Match carries the version the caller
// read. Statuses that assert a financial outcome cannot be set here.
func (s *Services) patchCase(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.scopeAtLeast(w, r, "add_notes_evidence", "full")
	if !ok {
		return
	}
	version, err := strconv.Atoi(strings.Trim(r.Header.Get("If-Match"), `"`))
	if err != nil || version < 1 {
		httpx.WriteError(w, r, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "If-Match must carry the version you read")
		return
	}
	var req patchCaseRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "body must be a JSON object with status and reason_code, or owner_id")
		return
	}
	if reversalOnly[req.Status] {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "this status is set only by the reversal workflow, after the outcome is verified")
		return
	}
	var out cases.Case
	err = s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		c, err := s.resolveCase(r, tx, p.TenantID, pathParam(r, "id"))
		if err != nil {
			return err
		}
		out = c
		if req.OwnerID != "" {
			owner, perr := uuid.Parse(req.OwnerID)
			if perr != nil {
				return cases.ErrInvalid
			}
			if c.Version != version {
				return cases.ErrStaleVersion
			}
			if err := s.Cases.Assign(r.Context(), tx, p.TenantID, c.ID, actorOf(p), owner); err != nil {
				return err
			}
			out, err = s.Cases.Get(r.Context(), tx, p.TenantID, c.ID)
			if err != nil {
				return err
			}
			version = out.Version
		}
		if req.Status != "" {
			out, err = s.Cases.Transition(r.Context(), tx, p.TenantID, c.ID, actorOf(p), req.Status, version, req.ReasonCode)
		}
		return err
	})
	if err != nil {
		writeCaseError(w, r, err)
		return
	}
	w.Header().Set("ETag", `"`+strconv.Itoa(out.Version)+`"`)
	httpx.WriteJSON(w, http.StatusOK, toCaseJSON(out))
}

// scopeAtLeast refuses callers whose grant for capability is narrower than want.
func (s *Services) scopeAtLeast(w http.ResponseWriter, r *http.Request, capability, want string) (identity.Principal, authz.Grant, bool) {
	p, g, ok := s.authorizeNoMFA(w, r, capability, authz.Attrs{})
	if ok && g.Scope != want {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not do this")
		return p, g, false
	}
	return p, g, ok
}

type noteRequest struct {
	Note string `json:"note"`
}

// addCaseNote serves POST /v1/cases/{id}/notes.
func (s *Services) addCaseNote(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "add_notes_evidence", authz.Attrs{})
	if !ok {
		return
	}
	var req noteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "body must be a JSON object with note")
		return
	}
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		c, err := s.resolveCase(r, tx, p.TenantID, pathParam(r, "id"))
		if err != nil {
			return err
		}
		return s.Cases.AddNote(r.Context(), tx, p.TenantID, c.ID, actorOf(p), req.Note)
	})
	if err != nil {
		writeCaseError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]string{"status": "recorded"})
}

type evidenceRequest struct {
	Kind         string          `json:"kind"`
	Ref          string          `json:"ref"`
	SourceSystem string          `json:"source_system"`
	Detail       json.RawMessage `json:"detail"`
}

// addCaseEvidence serves POST /v1/cases/{id}/evidence. Evidence added by a person
// is marked as not independently verified.
func (s *Services) addCaseEvidence(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.scopeAtLeast(w, r, "add_notes_evidence", "full")
	if !ok {
		return
	}
	var req evidenceRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "body must be a JSON object with kind and ref")
		return
	}
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		c, err := s.resolveCase(r, tx, p.TenantID, pathParam(r, "id"))
		if err != nil {
			return err
		}
		return s.Cases.AddEvidence(r.Context(), tx, p.TenantID, c.ID, actorOf(p), cases.Evidence{
			Kind: req.Kind, Ref: req.Ref, SourceSystem: req.SourceSystem, IsFact: false, Detail: req.Detail})
	})
	if err != nil {
		writeCaseError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]string{"status": "recorded"})
}

type complaintRequest struct {
	InstructionRef string `json:"instruction_ref"`
	Text           string `json:"text"`
	ContactRef     string `json:"contact_ref"`
}

type complaintResponse struct {
	Case    caseJSON `json:"case"`
	Matched bool     `json:"matched_existing_case"`
	Linked  bool     `json:"linked_to_transaction"`
}

// submitComplaint serves POST /v1/complaints, the customer-reported path (CUS001).
// An agent or operator enters what the customer reports. The complaint is matched
// to a transaction by its instruction reference. When the detector already opened
// a case for that transaction, the complaint joins it instead of starting a second
// one. Otherwise a CUS001 case is opened on the same queue, with an unmatched
// claim when no transaction is found. A complaint alone never triggers money
// movement.
func (s *Services) submitComplaint(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.authorizeNoMFA(w, r, "add_notes_evidence", authz.Attrs{})
	if !ok {
		return
	}
	var req complaintRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "body must be a JSON object with instruction_ref and text")
		return
	}
	req.Text, req.InstructionRef = strings.TrimSpace(req.Text), strings.TrimSpace(req.InstructionRef)
	if len(req.Text) < 5 || len(req.Text) > 2000 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "text must be 5 to 2000 characters")
		return
	}
	var resp complaintResponse
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var txnID *uuid.UUID
		var rail, currency string
		var amount int64
		if req.InstructionRef != "" {
			var id uuid.UUID
			err := tx.QueryRow(r.Context(), `SELECT id, rail, currency, amount_minor FROM transactions WHERE instruction_ref = $1`, req.InstructionRef).
				Scan(&id, &rail, &currency, &amount)
			if err == nil {
				txnID = &id
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		complaint, _ := json.Marshal(map[string]any{"text": req.Text, "instruction_ref": req.InstructionRef, "contact_ref": req.ContactRef, "entered_by": p.UserID.String()})
		evidence := cases.Evidence{Kind: "complaint", Ref: "COMPLAINT-" + uuid.NewString()[:8], IsFact: false, Detail: json.RawMessage(complaint)}

		// Join an existing detected case for the same transaction.
		if txnID != nil {
			existing, err := s.Cases.List(r.Context(), tx, p.TenantID, cases.Filter{TxnID: txnID, Source: cases.SourceBot}, nil, 20)
			if err != nil {
				return err
			}
			for _, c := range existing {
				if cases.IsTerminal(c.Status) {
					continue
				}
				if err := s.Cases.AddEvidence(r.Context(), tx, p.TenantID, c.ID, actorOf(p), evidence); err != nil {
					return err
				}
				if err := s.Cases.AddNote(r.Context(), tx, p.TenantID, c.ID, actorOf(p), "Customer contacted us about this transaction."); err != nil {
					return err
				}
				resp = complaintResponse{Case: toCaseJSON(c), Matched: true, Linked: true}
				return nil
			}
		}

		in := cases.OpenInput{Classification: cases.CodeCustomer, Source: cases.SourceCustomer, TxnID: txnID, Rail: rail, Currency: currency,
			RuleID: "INTAKE-1", RuleVersion: "1.0", Explanation: "Reported by the customer through an agent.", Complaint: complaint,
			Actor: actorOf(p), Evidence: []cases.Evidence{evidence}}
		if txnID != nil {
			in.ExposureMinor = &amount
		}
		c, _, err := s.Cases.Open(r.Context(), tx, p.TenantID, in)
		if err != nil {
			return err
		}
		if txnID != nil {
			if _, err := notify.Record(r.Context(), tx, p.TenantID, c.ID, notify.CustomerDetected(amount, currency, req.InstructionRef), c.Ref+":customer:detected"); err != nil {
				return err
			}
		}
		resp = complaintResponse{Case: toCaseJSON(c), Linked: txnID != nil}
		return nil
	})
	if err != nil {
		writeCaseError(w, r, err)
		return
	}
	code := http.StatusCreated
	if resp.Matched {
		code = http.StatusOK
	}
	httpx.WriteJSON(w, code, resp)
}

type customerStatus struct {
	CaseRef         string          `json:"case_ref"`
	Stage           string          `json:"stage"`
	Headline        string          `json:"headline"`
	OutcomeVerified bool            `json:"outcome_verified"`
	Deadline        *time.Time      `json:"regulatory_deadline,omitempty"`
	BankTarget      *time.Time      `json:"bank_target,omitempty"`
	Timeline        []timelineEntry `json:"timeline"`
	Messages        []notify.Stored `json:"messages"`
	Simulated       bool            `json:"simulated"`
}

type timelineEntry struct {
	At    time.Time `json:"at"`
	Label string    `json:"label"`
}

// customerCaseStatus serves GET /v1/cases/{id}/customer-status: only what a
// customer may be told. It says funds were returned only once the outcome has
// been verified.
func (s *Services) customerCaseStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := s.canReadQueue(w, r)
	if !ok {
		return
	}
	var out customerStatus
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		c, err := s.resolveCase(r, tx, p.TenantID, pathParam(r, "id"))
		if err != nil {
			return err
		}
		out, err = s.buildCustomerStatus(r.Context(), tx, p.TenantID, c)
		return err
	})
	if err != nil {
		writeCaseError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// buildCustomerStatus is the customer-safe view of a case, shared by the operator route
// and the customer's own.
func (s *Services) buildCustomerStatus(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, c cases.Case) (customerStatus, error) {
	var out customerStatus
	hist, err := s.Cases.History(ctx, tx, tenant, c.ID)
	if err != nil {
		return out, err
	}
	out.CaseRef, out.Deadline, out.Simulated = c.Ref, c.SLADueAt, true
	out.BankTarget = c.InternalTargetAt
	out.OutcomeVerified = c.Status == cases.StResolved || c.Status == cases.StClosed
	out.Stage, out.Headline = customerStage(c)
	out.Timeline = []timelineEntry{}
	for _, h := range hist {
		if label := customerLabel(h); label != "" {
			out.Timeline = append(out.Timeline, timelineEntry{At: h.At, Label: label})
		}
	}
	out.Messages, err = notify.ForCase(ctx, tx, tenant, c.ID, notify.Customer)
	return out, err
}

func customerStage(c cases.Case) (stage, headline string) {
	switch c.Status {
	case cases.StResolved, cases.StClosed:
		return "resolved", "Resolved. The reversal was confirmed in your account records."
	case cases.StRemediationSubmitted, cases.StOutcomeVerification:
		return "reversal_in_progress", "A reversal has been sent and we are confirming it. We will tell you when it is confirmed."
	case cases.StClosedNoException:
		return "closed", "We checked and found nothing wrong with this transaction."
	case cases.StClosedCustomerError:
		return "closed", "We looked into this and have closed it. Please contact us for details."
	default:
		return "investigating", "We found a problem and are checking it. You do not need to do anything."
	}
}

func customerLabel(h cases.Event) string {
	switch {
	case h.Kind == "opened":
		return "Problem detected"
	case h.Kind != "transition":
		return ""
	}
	switch h.ToStatus {
	case cases.StUnderInvestigation:
		return "Under investigation"
	case cases.StRemediationSubmitted:
		return "Reversal sent"
	case cases.StOutcomeVerification:
		return "Confirming the reversal"
	case cases.StResolved:
		return "Reversal confirmed"
	case cases.StClosedNoException:
		return "Checked, nothing wrong"
	}
	return ""
}

type escalateCaseRequest struct {
	TargetRole string `json:"target_role"`
	Reason     string `json:"reason"`
}

// escalateCase serves POST /v1/cases/{id}/escalate.
func (s *Services) escalateCase(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.scopeAtLeast(w, r, "add_notes_evidence", "full")
	if !ok {
		return
	}
	var req escalateCaseRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, "escalation reason is required")
		return
	}
	var out *cases.Case
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		c, err := s.resolveCase(r, tx, p.TenantID, pathParam(r, "id"))
		if err != nil {
			return err
		}
		out, err = s.Cases.Escalate(r.Context(), tx, p.TenantID, c.ID, actorOf(p), req.TargetRole, req.Reason)
		return err
	})
	if err != nil {
		writeCaseError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCaseJSON(*out))
}

type sweepSLAResponse struct {
	BreachedCount int        `json:"breached_count"`
	Cases         []caseJSON `json:"cases"`
}

// sweepSLAs serves POST /v1/cases/sweep-sla.
func (s *Services) sweepSLAs(w http.ResponseWriter, r *http.Request) {
	p, _, ok := s.scopeAtLeast(w, r, "view_exception_queue", "full")
	if !ok {
		return
	}
	var breached []cases.Case
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		var err error
		breached, err = s.Cases.SweepBreachedSLAs(r.Context(), tx, p.TenantID)
		return err
	})
	if err != nil {
		writeCaseError(w, r, err)
		return
	}
	items := make([]caseJSON, 0, len(breached))
	for _, c := range breached {
		items = append(items, toCaseJSON(c))
	}
	httpx.WriteJSON(w, http.StatusOK, sweepSLAResponse{
		BreachedCount: len(breached),
		Cases:         items,
	})
}
