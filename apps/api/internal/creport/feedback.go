package creport

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
)

// FeedbackInput is a customer's feedback on a report that was submitted.
type FeedbackInput struct {
	IdempotencyKey string
	Helpful        *bool
	Comment        string
	// DisputesOutcome says the customer did not receive what the bank shows as done.
	DisputesOutcome bool
}

// FeedbackResult says what the feedback did.
type FeedbackResult struct {
	ID       uuid.UUID `json:"id"`
	Reopened bool      `json:"reopened"`
	CaseRef  string    `json:"case_ref"`
	Message  string    `json:"message"`
	Repeat   bool      `json:"-"`
}

// Feedback records a customer's feedback. A customer who disputes a verified outcome
// reopens the case: the case moves to REOPENED with a note, operations are alerted and
// the customer is acknowledged. Reopening changes no financial fact. The bank's records
// of what was posted stay as they are, and any further remediation goes through the
// same eight-check gate as any other. Repeating a key returns the first result.
func (s *Service) Feedback(ctx context.Context, tenant, customerID, reportID uuid.UUID, in FeedbackInput) (FeedbackResult, error) {
	if !keyRe.MatchString(in.IdempotencyKey) {
		return FeedbackResult{}, fmt.Errorf("%w: Idempotency-Key must be 8 to 100 characters from A-Z a-z 0-9 . _ : -", ErrInvalid)
	}
	in.Comment = stripControl(strings.TrimSpace(in.Comment))
	if len(in.Comment) > 1000 {
		return FeedbackResult{}, fmt.Errorf("%w: the comment is longer than 1000 characters", ErrInvalid)
	}
	if in.Helpful == nil && !in.DisputesOutcome && in.Comment == "" {
		return FeedbackResult{}, fmt.Errorf("%w: say whether this helped, add a comment, or say you did not receive it", ErrInvalid)
	}
	var out FeedbackResult
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM customer_reports WHERE tenant_id = $1 AND customer_id = $2 AND id = $3 FOR UPDATE`, tenant, customerID, reportID); err != nil {
			return err
		}
		r, err := s.get(ctx, tx, tenant, customerID, reportID)
		if err != nil {
			return err
		}
		var prior uuid.UUID
		var reopened bool
		err = tx.QueryRow(ctx, `SELECT id, reopened FROM customer_feedback WHERE tenant_id = $1 AND customer_id = $2 AND idempotency_key = $3`, tenant, customerID, in.IdempotencyKey).Scan(&prior, &reopened)
		if err == nil {
			out = FeedbackResult{ID: prior, Reopened: reopened, Repeat: true}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if r.Status != "submitted" || r.CaseID == nil {
			return fmt.Errorf("%w: feedback is for a report that was sent to the bank", ErrNotReady)
		}
		c, err := s.Cases.Get(ctx, tx, tenant, *r.CaseID)
		if err != nil {
			return err
		}
		actor := customerActor(customerID)
		reopen := false
		if in.DisputesOutcome {
			note := "The customer says they did not receive the value the bank shows as complete."
			if in.Comment != "" {
				note += " They wrote: " + in.Comment
			}
			if cases.IsTerminal(c.Status) {
				if _, err := s.Cases.Transition(ctx, tx, tenant, c.ID, actor, cases.StReopened, 0, "CUSTOMER_DISPUTES_OUTCOME"); err != nil {
					return err
				}
				reopen = true
			}
			if err := s.Cases.AddNote(ctx, tx, tenant, c.ID, actor, note); err != nil {
				return err
			}
			if _, err := notify.Record(ctx, tx, tenant, c.ID, notify.OpsCustomerReport(c.Ref, c.Classification, "customer disputes the outcome"), c.Ref+":ops:dispute:"+in.IdempotencyKey); err != nil {
				return err
			}
		}
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO customer_feedback (tenant_id, report_id, customer_id, idempotency_key, helpful, comment, disputes_outcome, reopened, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, tenant, reportID, customerID, in.IdempotencyKey, in.Helpful, in.Comment, in.DisputesOutcome, reopen, s.now()).Scan(&id); err != nil {
			return err
		}
		msg := "Thank you for telling us."
		switch {
		case reopen:
			msg = "We have reopened your case and will look at it again. We will tell you what we find."
		case in.DisputesOutcome:
			msg = "Your case is still open, so we have added what you said to it."
		}
		out = FeedbackResult{ID: id, Reopened: reopen, CaseRef: c.Ref, Message: msg}
		return nil
	})
	return out, err
}
