// Package escalation holds the ladder a customer is shown when their case is overdue
// and unresolved: the bank's dispute channel, its head-office complaints department, and
// the regulator's consumer protection department.
//
// The contact details and wording come from the bank and are reviewed by Compliance
// before any customer sees them. Nothing here supplies a name, number or address. A
// step is shown only once a different person from its author has approved it, and the
// ladder is returned only when the case is overdue and unresolved.
package escalation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Errors.
var (
	ErrInvalid  = errors.New("escalation: invalid step")
	ErrFourEyes = errors.New("escalation: the author of a step may not approve it")
	ErrNotFound = errors.New("escalation: no such step")
	ErrState    = errors.New("escalation: the step is not waiting for review")
)

// Step is one rung of the ladder.
type Step struct {
	No      int    `json:"step"`
	Label   string `json:"label"`
	Contact string `json:"contact"`
	Wording string `json:"wording"`
	Status  string `json:"review_status,omitempty"`
}

func (s Step) validate() error {
	switch {
	case s.No < 1 || s.No > 10:
		return fmt.Errorf("%w: step must be 1 to 10", ErrInvalid)
	case l(s.Label) < 3 || l(s.Label) > 80:
		return fmt.Errorf("%w: label must be 3 to 80 characters", ErrInvalid)
	case l(s.Contact) < 3 || l(s.Contact) > 400:
		return fmt.Errorf("%w: contact must be 3 to 400 characters", ErrInvalid)
	case l(s.Wording) < 10 || l(s.Wording) > 600:
		return fmt.Errorf("%w: wording must be 10 to 600 characters", ErrInvalid)
	}
	return nil
}

func l(s string) int { return len(strings.TrimSpace(s)) }

// Propose saves a step as a draft, replacing an earlier draft or rejected step of the
// same number. An approved step is changed by proposing a new one after it is withdrawn;
// here it is refused, so approved wording never changes silently.
func Propose(ctx context.Context, tx pgx.Tx, tenant, author uuid.UUID, s Step) error {
	if err := s.validate(); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO customer_escalation_steps (tenant_id, step_no, label, contact_text, wording, authored_by) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id, step_no) DO UPDATE SET label = EXCLUDED.label, contact_text = EXCLUDED.contact_text, wording = EXCLUDED.wording,
			authored_by = EXCLUDED.authored_by, review_status = 'draft', reviewed_by = NULL, reviewed_at = NULL, review_note = '', created_at = now()
		WHERE customer_escalation_steps.review_status <> 'approved'`,
		tenant, s.No, strings.TrimSpace(s.Label), strings.TrimSpace(s.Contact), strings.TrimSpace(s.Wording), author)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: step %d is approved and in use; withdraw it before proposing another", ErrState, s.No)
	}
	return nil
}

// Decide approves or rejects a draft. The reviewer must not be its author.
func Decide(ctx context.Context, tx pgx.Tx, tenant, reviewer uuid.UUID, stepNo int, approve bool, note string, now time.Time) error {
	var author uuid.UUID
	var status string
	err := tx.QueryRow(ctx, `SELECT authored_by, review_status FROM customer_escalation_steps WHERE tenant_id = $1 AND step_no = $2 FOR UPDATE`, tenant, stepNo).Scan(&author, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "draft" {
		return ErrState
	}
	if reviewer == author {
		return ErrFourEyes
	}
	if !approve && strings.TrimSpace(note) == "" {
		return fmt.Errorf("%w: a note is required to reject", ErrInvalid)
	}
	next := "approved"
	if !approve {
		next = "rejected"
	}
	_, err = tx.Exec(ctx, `UPDATE customer_escalation_steps SET review_status = $3, reviewed_by = $4, reviewed_at = $5, review_note = $6 WHERE tenant_id = $1 AND step_no = $2`,
		tenant, stepNo, next, reviewer, now, strings.TrimSpace(note))
	return err
}

// Withdraw takes an approved step out of use by returning it to draft with a note.
func Withdraw(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, stepNo int) error {
	tag, err := tx.Exec(ctx, `UPDATE customer_escalation_steps SET review_status = 'rejected', review_note = 'withdrawn' WHERE tenant_id = $1 AND step_no = $2 AND review_status = 'approved'`, tenant, stepNo)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Approved returns the steps a customer may be shown, in order.
func Approved(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]Step, error) {
	rows, err := tx.Query(ctx, `SELECT step_no, label, contact_text, wording FROM customer_escalation_steps WHERE tenant_id = $1 AND review_status = 'approved' ORDER BY step_no`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Step{}
	for rows.Next() {
		var s Step
		if err := rows.Scan(&s.No, &s.Label, &s.Contact, &s.Wording); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// All returns every step with its review status, for the people who manage the ladder.
func All(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]Step, error) {
	rows, err := tx.Query(ctx, `SELECT step_no, label, contact_text, wording, review_status FROM customer_escalation_steps WHERE tenant_id = $1 ORDER BY step_no`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Step{}
	for rows.Next() {
		var s Step
		if err := rows.Scan(&s.No, &s.Label, &s.Contact, &s.Wording, &s.Status); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Due says whether a case is overdue and unresolved: its binding deadline (the earlier
// of the regulatory deadline and the internal target) has passed and it is not
// finished. A case with no deadline is never overdue.
func Due(now time.Time, sla, internal *time.Time, finished bool) bool {
	if finished {
		return false
	}
	var due *time.Time
	for _, t := range []*time.Time{sla, internal} {
		if t != nil && (due == nil || t.Before(*due)) {
			due = t
		}
	}
	return due != nil && !now.Before(*due)
}
