package server

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/escalation"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

type customerCaseItem struct {
	ID         string     `json:"id"`
	CaseRef    string     `json:"case_ref"`
	DetectedBy string     `json:"origin"`
	Stage      string     `json:"stage"`
	Headline   string     `json:"headline"`
	OpenedAt   time.Time  `json:"opened_at"`
	BankTarget *time.Time `json:"bank_target,omitempty"`
	Verified   bool       `json:"outcome_verified"`
	Simulated  bool       `json:"simulated"`
}

// customerCases serves GET /v1/customer/cases: the customer's own cases, including
// ones the bank detected that the customer never reported. Nothing else is visible.
func (s *Services) customerCases(w http.ResponseWriter, r *http.Request) {
	p, ok := customerFrom(w, r)
	if !ok {
		return
	}
	items := []customerCaseItem{}
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		ids, err := s.Reports.VisibleCaseIDs(r.Context(), tx, p.TenantID, p.UserID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			c, err := s.Cases.Get(r.Context(), tx, p.TenantID, id)
			if err != nil {
				return err
			}
			stage, head := customerStage(c)
			origin := "reported_by_you"
			if c.Source == cases.SourceBot {
				origin = "found_by_the_bank"
			}
			target := earliest(c.SLADueAt, c.InternalTargetAt)
			items = append(items, customerCaseItem{ID: c.ID.String(), CaseRef: c.Ref, DetectedBy: origin, Stage: stage, Headline: head, OpenedAt: c.OpenedAt,
				BankTarget: target, Verified: c.Status == cases.StResolved || c.Status == cases.StClosed, Simulated: true})
		}
		return nil
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func earliest(a, b *time.Time) *time.Time {
	switch {
	case a == nil:
		return b
	case b == nil || a.Before(*b):
		return a
	}
	return b
}

// customerCaseOr404 loads a case only if it is one of the customer's.
func (s *Services) customerCase(w http.ResponseWriter, r *http.Request, fn func(tx pgx.Tx, c cases.Case, customerID uuid.UUID, tenant uuid.UUID) (any, error)) {
	p, ok := customerFrom(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(pathParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
		return
	}
	var out any
	notFound := false
	err = s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		seen, err := s.Reports.CanSeeCase(r.Context(), tx, p.TenantID, p.UserID, id)
		if err != nil {
			return err
		}
		if !seen {
			notFound = true
			return nil
		}
		c, err := s.Cases.Get(r.Context(), tx, p.TenantID, id)
		if err != nil {
			return err
		}
		out, err = fn(tx, c, p.UserID, p.TenantID)
		return err
	})
	switch {
	case notFound:
		httpx.WriteError(w, r, http.StatusNotFound, httpx.CodeNotFound, "resource not found")
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	default:
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

// customerCaseTimeline serves GET /v1/customer/cases/{id}/timeline: what a customer may
// be told. "Resolved" appears only after the financial outcome is verified.
func (s *Services) customerCaseTimeline(w http.ResponseWriter, r *http.Request) {
	s.customerCase(w, r, func(tx pgx.Tx, c cases.Case, _ uuid.UUID, tenant uuid.UUID) (any, error) {
		return s.buildCustomerStatus(r.Context(), tx, tenant, c)
	})
}

type escalationView struct {
	Due   bool              `json:"due"`
	Steps []escalation.Step `json:"steps"`
	Note  string            `json:"note,omitempty"`
}

// customerCaseEscalation serves GET /v1/customer/cases/{id}/escalation: empty until the
// case is overdue and unresolved, then the steps Compliance has approved. If none has
// been approved the answer says so and invents nothing.
func (s *Services) customerCaseEscalation(w http.ResponseWriter, r *http.Request) {
	s.customerCase(w, r, func(tx pgx.Tx, c cases.Case, _ uuid.UUID, tenant uuid.UUID) (any, error) {
		now := time.Now()
		if s.Now != nil {
			now = s.Now()
		}
		finished := cases.IsTerminal(c.Status)
		out := escalationView{Steps: []escalation.Step{}}
		if !escalation.Due(now, c.SLADueAt, c.InternalTargetAt, finished) {
			return out, nil
		}
		out.Due = true
		steps, err := escalation.Approved(r.Context(), tx, tenant)
		if err != nil {
			return nil, err
		}
		out.Steps = steps
		if len(steps) == 0 {
			out.Note = "This case is past its target. The bank has not yet published escalation steps; please contact the bank directly."
		}
		return out, nil
	})
}
