package creport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/complaint"
	"github.com/smartreversal/smartreversal/apps/api/internal/deadline"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// ErrNotReady means the report cannot be drafted or submitted yet.
var ErrNotReady = errors.New("creport: the report is not ready")

// Disputes a customer may state, in the order shown.
var Disputes = []string{deadline.FailedTransfer, deadline.DuplicateDebit, deadline.PosWebFailure, deadline.AtmOnUs, deadline.AtmNotOnUs, deadline.WrongAccountOrAm, deadline.UnmatchedClaim}

// Classification is the proposed dispute type and whether a person should look.
type Classification struct {
	Proposed       string   `json:"proposed"`
	Allowed        []string `json:"allowed"`
	ReviewRequired bool     `json:"review_required"`
	Reason         string   `json:"reason,omitempty"`
}

// facts about the matched transaction that classification and the draft use.
type matched struct {
	txnID      uuid.UUID
	ref        string
	rail       string
	channel    string
	state      string
	currency   string
	amount     int64
	failureAt  time.Time
	firstDebit time.Time
	timeline   []complaint.TimelineItem
	caseClass  string
}

func (s *Service) loadMatched(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, id *uuid.UUID) (*matched, error) {
	if id == nil {
		return nil, nil
	}
	m := &matched{txnID: *id}
	var fd *time.Time
	var first time.Time
	if err := tx.QueryRow(ctx, `SELECT instruction_ref, rail, channel, lifecycle_state, currency, amount_minor, first_debit_at, first_event_at FROM transactions WHERE tenant_id = $1 AND id = $2`,
		tenant, *id).Scan(&m.ref, &m.rail, &m.channel, &m.state, &m.currency, &m.amount, &fd, &first); err != nil {
		return nil, err
	}
	m.firstDebit = first.UTC()
	if fd != nil {
		m.firstDebit = fd.UTC()
	}
	rows, err := tx.Query(ctx, `SELECT event_type, source_ts FROM transaction_events WHERE tenant_id = $1 AND txn_id = $2 ORDER BY source_ts, id`, tenant, *id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var typ string
		var at time.Time
		if err := rows.Scan(&typ, &at); err != nil {
			return nil, err
		}
		if what, ok := plain[typ]; ok {
			m.timeline = append(m.timeline, complaint.TimelineItem{At: at.UTC(), What: what})
		}
		if (typ == txn.EvCreditFailed || typ == txn.EvTransferTimeout) && m.failureAt.IsZero() {
			m.failureAt = at.UTC()
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	_ = tx.QueryRow(ctx, `SELECT classification FROM cases WHERE tenant_id = $1 AND txn_id = $2 AND status NOT IN ('RESOLVED','CLOSED','CLOSED_NO_EXCEPTION','CLOSED_CUSTOMER_ERROR') ORDER BY opened_at LIMIT 1`, tenant, *id).Scan(&m.caseClass)
	return m, nil
}

var plain = map[string]string{
	txn.EvDebitPosted:     "My account was debited",
	txn.EvTransferSent:    "The transfer was sent",
	txn.EvCreditConfirmed: "The credit to the other account was confirmed",
	txn.EvCreditFailed:    "The credit to the other account failed",
	txn.EvTransferTimeout: "The bank received no answer about the transfer",
	txn.EvReversalPosted:  "A reversal was posted",
}

// classify proposes a dispute type from what the bank knows. The clock is then chosen
// by the fixed mapping in the deadline engine, never here and never by a model.
func classify(m *matched, parserConfidence float64) Classification {
	c := Classification{Allowed: Disputes}
	switch {
	case m == nil:
		c.Proposed = deadline.UnmatchedClaim
	case m.caseClass == "DUP001":
		c.Proposed = deadline.DuplicateDebit
	case strings.EqualFold(m.rail, "ATM") || strings.EqualFold(m.channel, "ATM"):
		c.ReviewRequired, c.Reason = true, "Please say whether the machine belongs to this bank or another bank."
	case matchAny(m.rail, "POS", "WEB", "CARD", "MERCHANT") || matchAny(m.channel, "POS", "CARD", "MERCHANT"):
		c.Proposed = deadline.PosWebFailure
	default:
		c.Proposed = deadline.FailedTransfer
	}
	if parserConfidence > 0 && parserConfidence < 0.6 {
		c.ReviewRequired, c.Reason = true, "The alert could not be read with confidence, so a person will check the details."
	}
	return c
}

func matchAny(s string, opts ...string) bool {
	for _, o := range opts {
		if strings.EqualFold(s, o) {
			return true
		}
	}
	return false
}

func allowedDispute(d string) bool {
	for _, a := range Disputes {
		if a == d {
			return true
		}
	}
	return false
}

// Draft is the proposed dispute type, the deadline it implies and the complaint text.
type Draft struct {
	Classification Classification  `json:"classification"`
	Dispute        string          `json:"dispute_type"`
	Deadline       deadline.Result `json:"deadline"`
	Text           string          `json:"text"`
	Edited         bool            `json:"edited"`
	Facts          complaint.Facts `json:"-"`
}

func (s *Service) factsFor(r Report, dispute string, m *matched, c Classification, custRef string, dl deadline.Result) complaint.Facts {
	f := complaint.Facts{CustomerRef: custRef, Dispute: dispute, Deadline: &dl, AccountTail: r.Confirmed["account_tail"], Narration: r.Confirmed["narration"], Reference: r.Confirmed["reference"]}
	if v := r.Confirmed["amount_minor"]; v != "" {
		var n int64
		fmt.Sscan(v, &n)
		f.AmountMinor = n
	}
	f.Currency = "NGN"
	if t, err := time.Parse(time.RFC3339, r.Confirmed["occurred_at"]); err == nil {
		f.OccurredAt = t
	}
	if m != nil {
		f.Timeline = m.timeline
		if f.Reference == "" {
			f.Reference = m.ref
		}
		if m.currency != "" {
			f.Currency = m.currency
		}
	}
	f.Evidence = []string{"The alert I received, kept in redacted form"}
	if m != nil {
		f.Evidence = append(f.Evidence, "The bank's own record of transaction "+m.ref)
	}
	return f
}

// deadlineFacts picks the moments the clocks start from.
func deadlineFacts(r Report, m *matched, now time.Time) deadline.Facts {
	f := deadline.Facts{ReportedAt: now}
	if t, err := time.Parse(time.RFC3339, r.Confirmed["occurred_at"]); err == nil {
		f.TransactionDateAt, f.FailureAt = t, t
	}
	if m != nil {
		f.TransactionDateAt = m.firstDebit
		if !m.failureAt.IsZero() {
			f.FailureAt = m.failureAt
		} else {
			f.FailureAt = m.firstDebit
		}
	}
	return f
}

// MakeDraft proposes the dispute type and builds the complaint draft and deadline for a
// confirmed report. dispute may be empty to accept the proposal. It writes nothing.
func (s *Service) MakeDraft(ctx context.Context, tenant, customerID, reportID uuid.UUID, dispute string) (Draft, error) {
	var out Draft
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		out, _, _, err = s.draft(ctx, tx, tenant, customerID, reportID, dispute)
		return err
	})
	return out, err
}

func (s *Service) draft(ctx context.Context, tx pgx.Tx, tenant, customerID, reportID uuid.UUID, dispute string) (Draft, Report, *matched, error) {
	r, err := s.get(ctx, tx, tenant, customerID, reportID)
	if err != nil {
		return Draft{}, r, nil, err
	}
	if r.Status == "created" || len(r.Confirmed) == 0 {
		return Draft{}, r, nil, fmt.Errorf("%w: confirm the details first", ErrNotReady)
	}
	m, err := s.loadMatched(ctx, tx, tenant, r.TxnID)
	if err != nil {
		return Draft{}, r, nil, err
	}
	cls := classify(m, r.Parsed.Confidence)
	d := dispute
	if d == "" {
		d = r.DisputeType
	}
	if d == "" {
		d = cls.Proposed
	}
	if d == "" {
		return Draft{}, r, m, fmt.Errorf("%w: say which kind of problem this is", ErrNotReady)
	}
	if !allowedDispute(d) || (d == deadline.UnmatchedClaim) != (m == nil) {
		return Draft{}, r, m, fmt.Errorf("%w: that kind of problem does not fit this report", ErrInvalid)
	}
	if d != cls.Proposed && cls.Proposed != "" {
		cls.ReviewRequired, cls.Reason = true, "You chose a different kind of problem from the one we expected, so a person will check."
	}
	dl, err := s.Deadlines.Compute(ctx, tx, tenant, d, deadlineFacts(r, m, s.now()))
	if err != nil {
		return Draft{}, r, m, err
	}
	var custRef string
	_ = tx.QueryRow(ctx, `SELECT customer_ref FROM customers WHERE tenant_id = $1 AND id = $2`, tenant, customerID).Scan(&custRef)
	facts := s.factsFor(r, d, m, cls, custRef, dl)
	var edited string
	_ = tx.QueryRow(ctx, `SELECT edited FROM customer_reports WHERE tenant_id = $1 AND id = $2`, tenant, reportID).Scan(&edited)
	text := complaint.Draft(facts)
	if edited != "" {
		text = edited
	}
	return Draft{Classification: cls, Dispute: d, Deadline: dl, Text: text, Edited: edited != "", Facts: facts}, r, m, nil
}

// SubmitInput is what the customer sends to submit.
type SubmitInput struct {
	DisputeType string
	// Text is the customer's edited complaint. Empty means use the generated draft.
	Text string
	// DisputesVerifiedOutcome is for a customer who says they did not receive the value
	// the bank shows as completed or reversed. It opens a case flagged as a dispute.
	DisputesVerifiedOutcome bool
}

// Submitted is the result of a submission.
type Submitted struct {
	CaseID   uuid.UUID       `json:"case_id"`
	CaseRef  string          `json:"case_ref"`
	Created  bool            `json:"case_created"`
	Joined   bool            `json:"joined_existing_case"`
	Deadline deadline.Result `json:"deadline"`
	Dispute  string          `json:"dispute_type"`
	Review   bool            `json:"review_required"`
}

func customerActor(id uuid.UUID) cases.Actor { return cases.Actor{Type: audit.ActorUser, ID: id} }

// Submit turns a confirmed report into a case on the shared queue (CUS001, or CUS002
// for a wrong account or amount), or adds it to the case that already exists. The
// complaint text and the redacted alert are attached as evidence, the original draft is
// kept with any edits, the customer is acknowledged, and operations are alerted. It
// never moves money. Repeating it returns the same case.
func (s *Service) Submit(ctx context.Context, tenant, customerID, reportID uuid.UUID, in SubmitInput) (Submitted, error) {
	var out Submitted
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM customer_reports WHERE tenant_id = $1 AND customer_id = $2 AND id = $3 FOR UPDATE`, tenant, customerID, reportID); err != nil {
			return err
		}
		cur, err := s.get(ctx, tx, tenant, customerID, reportID)
		if err != nil {
			return err
		}
		if cur.Status == "submitted" {
			if cur.CaseID == nil {
				return ErrSubmitted
			}
			c, err := s.Cases.Get(ctx, tx, tenant, *cur.CaseID)
			if err != nil {
				return err
			}
			out = Submitted{CaseID: c.ID, CaseRef: c.Ref, Dispute: cur.DisputeType}
			return nil
		}
		switch cur.MatchStatus {
		case MatchCandidates, "":
			return fmt.Errorf("%w: choose the transaction first", ErrNotReady)
		case MatchOtherBank:
			return fmt.Errorf("%w: an alert from another bank cannot become a case here", ErrInvalid)
		case MatchDuplicateReport:
			return fmt.Errorf("%w: this transaction was already reported", ErrInvalid)
		case MatchVerifiedSuccess, MatchAlreadyReversed:
			if !in.DisputesVerifiedOutcome {
				return fmt.Errorf("%w: the bank shows this as complete; say you did not receive it to open a case", ErrInvalid)
			}
		}
		d, _, m, err := s.draft(ctx, tx, tenant, customerID, reportID, in.DisputeType)
		if err != nil {
			return err
		}
		// The customer's own words, within bounds. The original draft is kept beside them.
		final := d.Text
		original := complaint.Draft(d.Facts)
		if t := strings.TrimSpace(in.Text); t != "" {
			if len(t) > complaint.MaxLength {
				return fmt.Errorf("%w: the complaint is too long", ErrInvalid)
			}
			final = stripControl(t)
		}
		actor := customerActor(customerID)
		code := cases.CodeCustomer
		if d.Dispute == deadline.WrongAccountOrAm {
			code = cases.CodeWrongAcct
		}
		complaintJSON, _ := json.Marshal(map[string]any{"report_id": reportID.String(), "dispute_type": d.Dispute, "customer_id": customerID.String()})
		ev := []cases.Evidence{
			{Kind: "complaint", Ref: "COMPLAINT-" + reportID.String()[:8], IsFact: false, Detail: map[string]any{"text": final, "dispute_type": d.Dispute, "edited": final != original}},
			{Kind: "customer_alert", Ref: "ALERT-" + reportID.String()[:8], IsFact: false, Detail: map[string]any{"redacted": true, "parsed_confidence": cur.Parsed.Confidence, "flags": cur.Parsed.Flags}},
		}
		if d.Classification.ReviewRequired {
			ev = append(ev, cases.Evidence{Kind: "classification_review", Ref: "REVIEW-" + reportID.String()[:8], IsFact: false, Detail: map[string]any{"reason": d.Classification.Reason, "proposed": d.Classification.Proposed, "chosen": d.Dispute}})
		}
		if in.DisputesVerifiedOutcome {
			ev = append(ev, cases.Evidence{Kind: "customer_dispute", Ref: "DISPUTE-" + reportID.String()[:8], IsFact: false, Detail: map[string]any{"statement": "The customer says they did not receive the value the bank shows as complete."}})
		}

		var c cases.Case
		joined := false
		if cur.MatchStatus == MatchExistingCase && cur.CaseID != nil {
			c, err = s.Cases.Get(ctx, tx, tenant, *cur.CaseID)
			if err != nil {
				return err
			}
			for _, e := range ev {
				if err := s.Cases.AddEvidence(ctx, tx, tenant, c.ID, actor, e); err != nil {
					return err
				}
			}
			if err := s.Cases.AddNote(ctx, tx, tenant, c.ID, actor, "The customer contacted us again about this transaction."); err != nil {
				return err
			}
			joined = true
		} else {
			open := cases.OpenInput{Classification: code, Source: cases.SourceCustomer, Clock: clockOf(d.Dispute), RuleID: "CUSTOMER-REPORT-1", RuleVersion: "1.0",
				Explanation: "Reported by the customer through ReverseIt.", Complaint: complaintJSON, Actor: actor, Evidence: ev}
			if m != nil {
				open.TxnID, open.Rail, open.Currency = &m.txnID, m.rail, m.currency
				amt := m.amount
				open.ExposureMinor = &amt
			}
			if d.Deadline.Internal != nil {
				t := d.Deadline.Internal.Due
				open.InternalTargetAt = &t
			}
			c, _, err = s.Cases.Open(ctx, tx, tenant, open)
			if err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO case_contacts (tenant_id, case_id, report_id, customer_id) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, tenant, c.ID, reportID, customerID); err != nil {
			return err
		}
		var target *time.Time
		if d.Deadline.Binding != nil {
			t := *d.Deadline.Binding
			target = &t
		}
		if _, err := notify.Record(ctx, tx, tenant, c.ID, notify.CustomerReportReceived(c.Ref, target), c.Ref+":customer:received:"+reportID.String()); err != nil {
			return err
		}
		if _, err := notify.Record(ctx, tx, tenant, c.ID, notify.OpsCustomerReport(c.Ref, c.Classification, d.Dispute), c.Ref+":ops:customer-report:"+reportID.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE customer_reports SET status = 'submitted', case_id = $4, dispute_type = $5, draft = $6, edited = $7, submitted_at = $8, updated_at = $8, version = version + 1
			WHERE tenant_id = $1 AND customer_id = $2 AND id = $3`, tenant, customerID, reportID, c.ID, d.Dispute, original, editedOrEmpty(final, original), s.now()); err != nil {
			return err
		}
		out = Submitted{CaseID: c.ID, CaseRef: c.Ref, Created: !joined, Joined: joined, Deadline: d.Deadline, Dispute: d.Dispute, Review: d.Classification.ReviewRequired}
		return nil
	})
	return out, err
}

func editedOrEmpty(final, original string) string {
	if final == original {
		return ""
	}
	return final
}

func clockOf(dispute string) string {
	c, _ := deadline.ClockFor(dispute)
	return c
}

func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 32 && r != 127 {
			return r
		}
		return -1
	}, s)
}

// SaveEdit stores the customer's edited complaint text before submission. The original
// draft is always regenerated from the facts and kept at submission.
func (s *Service) SaveEdit(ctx context.Context, tenant, customerID, reportID uuid.UUID, text string) error {
	text = stripControl(strings.TrimSpace(text))
	if len(text) > complaint.MaxLength {
		return fmt.Errorf("%w: the complaint is too long", ErrInvalid)
	}
	return s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE customer_reports SET edited = $4, updated_at = $5, version = version + 1 WHERE tenant_id = $1 AND customer_id = $2 AND id = $3 AND status <> 'submitted'`,
			tenant, customerID, reportID, text, s.now())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			if _, err := s.get(ctx, tx, tenant, customerID, reportID); err != nil {
				return err
			}
			return ErrSubmitted
		}
		return nil
	})
}
