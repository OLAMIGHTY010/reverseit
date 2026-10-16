// Package creport is the heart of a ReverseIt customer's report: intake of a pasted
// alert, the customer's confirmation of the fields, and matching to the customer's own
// transactions.
//
// Rules this package keeps:
//   - A pasted alert, an account tail or a reference is a claim, not proof. Matching
//     looks only at transactions on accounts the bank recorded as the customer's own.
//     A reference that belongs to someone else's transaction is indistinguishable from
//     no match, so nothing leaks.
//   - The alert as pasted is never stored. Only the redacted text and a hash of the
//     original are kept, and the raw text is never logged.
//   - Parsed fields are suggestions. The customer confirms or edits every field and the
//     server validates each again before matching.
//   - A match is linked automatically only on an exact reference or session. Anything
//     weaker is offered as a ranked candidate with reasons and the customer chooses.
//   - Fuzzy matching needs an approved time window; without one it offers nothing.
//   - A report never moves money. Outcomes only decide which case the customer's
//     contact belongs to.
package creport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/alertparse"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/customer"
	"github.com/smartreversal/smartreversal/apps/api/internal/deadline"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Match statuses.
const (
	MatchCandidates      = "candidates"
	MatchExistingCase    = "matched_existing_case"
	MatchTransaction     = "matched_transaction"
	MatchVerifiedSuccess = "verified_successful"
	MatchAlreadyReversed = "already_reversed"
	MatchUnmatched       = "unmatched"
	MatchDuplicateReport = "duplicate_report"
	MatchOtherBank       = "other_bank"
)

// DefaultReportsPerHour caps reports per customer when no value is approved. It is an
// abuse guard, not a policy; configuration overrides it.
const DefaultReportsPerHour = 10

// Errors.
var (
	ErrNotFound    = errors.New("creport: no such report")
	ErrConsent     = errors.New("creport: consent is required")
	ErrInvalid     = errors.New("creport: invalid report")
	ErrSubmitted   = errors.New("creport: the report was already submitted")
	ErrNotAccepted = errors.New("creport: that transaction is not one of the candidates")
)

// RateLimitedError says when to try again.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "creport: too many reports; try again later" }

// Service handles reports.
type Service struct {
	Pool   *db.Pool
	Parser alertparse.Parser
	Config *configsvc.Service
	// Cases and Deadlines are needed to draft and submit.
	Cases     *cases.Service
	Deadlines *deadline.Engine
	Now       func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Report is a customer's report as the customer sees it.
type Report struct {
	ID           uuid.UUID         `json:"id"`
	Status       string            `json:"status"`
	ParserStatus string            `json:"parser_status"`
	Parsed       alertparse.Parsed `json:"parsed"`
	GuidedForm   bool              `json:"guided_form_required"`
	Confirmed    map[string]string `json:"confirmed,omitempty"`
	MatchStatus  string            `json:"match_status,omitempty"`
	Candidates   []Candidate       `json:"candidates,omitempty"`
	TxnID        *uuid.UUID        `json:"transaction_id,omitempty"`
	CaseID       *uuid.UUID        `json:"case_id,omitempty"`
	DisputeType  string            `json:"dispute_type,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	Version      int               `json:"version"`
	Outcome      *Outcome          `json:"outcome,omitempty"`
	Idempotent   bool              `json:"-"`
}

// CreateInput is a new report.
type CreateInput struct {
	IdempotencyKey string
	Text           string
	ImageRef       string
	Consent        bool
	ConsentVersion string
}

var (
	keyRe      = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,100}$`)
	imageRefRe = regexp.MustCompile(`^[A-Za-z0-9._:/-]{4,200}$`)
	versionRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,40}$`)
)

// Create records a report from pasted text or an image reference. Repeating the same
// idempotency key returns the same report. Consent is required and its time and
// version are kept. The text is redacted before it is parsed, stored or logged.
func (s *Service) Create(ctx context.Context, tenant, customerID uuid.UUID, in CreateInput) (Report, error) {
	if !keyRe.MatchString(in.IdempotencyKey) {
		return Report{}, fmt.Errorf("%w: Idempotency-Key must be 8 to 100 characters from A-Z a-z 0-9 . _ : -", ErrInvalid)
	}
	if !in.Consent {
		return Report{}, ErrConsent
	}
	if !versionRe.MatchString(in.ConsentVersion) {
		return Report{}, fmt.Errorf("%w: consent_version is required", ErrInvalid)
	}
	text := strings.TrimSpace(in.Text)
	if text == "" && in.ImageRef == "" {
		return Report{}, fmt.Errorf("%w: paste the alert text or give an image reference", ErrInvalid)
	}
	if in.ImageRef != "" && !imageRefRe.MatchString(in.ImageRef) {
		return Report{}, fmt.Errorf("%w: image_ref has unsupported characters", ErrInvalid)
	}
	if len(text) > alertparse.MaxAlertLength {
		return Report{}, fmt.Errorf("%w: the alert text is longer than %d characters", ErrInvalid, alertparse.MaxAlertLength)
	}

	var out Report
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenant.String()+"|report|"+customerID.String()); err != nil {
			return err
		}
		if existing, err := s.byKey(ctx, tx, tenant, customerID, in.IdempotencyKey); err == nil {
			out = existing
			out.Idempotent = true
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := s.checkRate(ctx, tx, tenant, customerID); err != nil {
			return err
		}
		redacted := alertparse.Redact(text)
		sum := sha256.Sum256([]byte(text))
		parsed := alertparse.Parsed{Fields: map[string]alertparse.Field{}}
		status := "parsed"
		switch {
		case text == "":
			status = "image_needs_parser" // reading an image belongs to the AI service; the customer fills the guided form
		default:
			p, err := s.Parser.Parse(ctx, redacted)
			switch {
			case errors.Is(err, alertparse.ErrUnavailable):
				status = "unavailable"
			case err != nil:
				status = "unreadable"
			default:
				parsed = p
			}
		}
		pj, _ := json.Marshal(parsed)
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO customer_reports (tenant_id, customer_id, idempotency_key, consent_at, consent_version, alert_redacted, alert_hash, image_ref, parser, parser_status, parsed, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$4,$4) RETURNING id`,
			tenant, customerID, in.IdempotencyKey, s.now(), in.ConsentVersion, redacted, hex.EncodeToString(sum[:]), in.ImageRef, parsed.Parser, status, pj).Scan(&id); err != nil {
			return err
		}
		var err error
		out, err = s.get(ctx, tx, tenant, customerID, id)
		return err
	})
	return out, err
}

func (s *Service) checkRate(ctx context.Context, tx pgx.Tx, tenant, customerID uuid.UUID) error {
	limit := int64(DefaultReportsPerHour)
	if v, err := s.Config.Effective(ctx, tx, tenant, "customer.reports_per_hour", s.now()); err == nil {
		limit = v.Value
	} else if !errors.Is(err, configsvc.ErrNotConfigured) {
		return err
	}
	var n int64
	var oldest *time.Time
	if err := tx.QueryRow(ctx, `SELECT count(*), min(created_at) FROM customer_reports WHERE tenant_id = $1 AND customer_id = $2 AND created_at > $3`,
		tenant, customerID, s.now().Add(-time.Hour)).Scan(&n, &oldest); err != nil {
		return err
	}
	if n >= limit {
		retry := time.Minute
		if oldest != nil {
			retry = oldest.Add(time.Hour).Sub(s.now())
			if retry < time.Second {
				retry = time.Second
			}
		}
		return &RateLimitedError{RetryAfter: retry}
	}
	return nil
}

const reportColumns = `id, status, parser_status, parsed, confirmed, match_status, candidates, txn_id, case_id, dispute_type, created_at, version`

func scanReport(row pgx.Row) (Report, error) {
	var r Report
	var parsed, confirmed, cands []byte
	if err := row.Scan(&r.ID, &r.Status, &r.ParserStatus, &parsed, &confirmed, &r.MatchStatus, &cands, &r.TxnID, &r.CaseID, &r.DisputeType, &r.CreatedAt, &r.Version); err != nil {
		return r, err
	}
	_ = json.Unmarshal(parsed, &r.Parsed)
	if r.Parsed.Fields == nil {
		r.Parsed.Fields = map[string]alertparse.Field{}
	}
	_ = json.Unmarshal(confirmed, &r.Confirmed)
	_ = json.Unmarshal(cands, &r.Candidates)
	r.CreatedAt = r.CreatedAt.UTC()
	r.GuidedForm = r.ParserStatus != "parsed" || r.Parsed.Confidence < 0.8 || len(r.Parsed.Fields) < 3
	return r, nil
}

func (s *Service) get(ctx context.Context, tx pgx.Tx, tenant, customerID, id uuid.UUID) (Report, error) {
	r, err := scanReport(tx.QueryRow(ctx, `SELECT `+reportColumns+` FROM customer_reports WHERE tenant_id = $1 AND customer_id = $2 AND id = $3`, tenant, customerID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	return r, err
}

func (s *Service) byKey(ctx context.Context, tx pgx.Tx, tenant, customerID uuid.UUID, key string) (Report, error) {
	r, err := scanReport(tx.QueryRow(ctx, `SELECT `+reportColumns+` FROM customer_reports WHERE tenant_id = $1 AND customer_id = $2 AND idempotency_key = $3`, tenant, customerID, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	return r, err
}

// Get returns one of the customer's reports. A report that belongs to someone else is
// reported as not found.
func (s *Service) Get(ctx context.Context, tenant, customerID, id uuid.UUID) (Report, error) {
	var r Report
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		r, err = s.get(ctx, tx, tenant, customerID, id)
		return err
	})
	return r, err
}

// List returns the customer's reports, newest first.
func (s *Service) List(ctx context.Context, tenant, customerID uuid.UUID) ([]Report, error) {
	out := []Report{}
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+reportColumns+` FROM customer_reports WHERE tenant_id = $1 AND customer_id = $2 ORDER BY created_at DESC, id LIMIT 100`, tenant, customerID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanReport(rows)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// ---- confirmation and matching ------------------------------------------------

// ConfirmInput is what the customer confirms or edits.
type ConfirmInput struct {
	AmountMinor    int64
	OccurredAt     time.Time
	AccountTail    string
	Reference      string
	Channel        string
	Narration      string
	CandidateTxnID *uuid.UUID
}

// Candidate is a transaction the report might be about, ranked with reasons.
type Candidate struct {
	TxnID          uuid.UUID `json:"transaction_id"`
	InstructionRef string    `json:"instruction_ref"`
	Confidence     float64   `json:"confidence"`
	Reasons        []string  `json:"reasons"`
	Exact          bool      `json:"exact_reference"`
	Lifecycle      string    `json:"lifecycle_state"`
	AmountMinor    string    `json:"amount_minor"`
	Currency       string    `json:"currency"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// Closure is what the customer is shown when the bank can already say what happened.
type Closure struct {
	Lifecycle string        `json:"lifecycle_state"`
	Events    []ClosureItem `json:"events"`
	Reversal  string        `json:"reversal_financial_status,omitempty"`
}

// ClosureItem is one verified step.
type ClosureItem struct {
	Type   string    `json:"event_type"`
	Source string    `json:"source_system"`
	At     time.Time `json:"at"`
}

// Outcome is what the match concluded.
type Outcome struct {
	MatchStatus      string      `json:"match_status"`
	TxnID            *uuid.UUID  `json:"transaction_id,omitempty"`
	CaseID           *uuid.UUID  `json:"case_id,omitempty"`
	CaseRef          string      `json:"case_ref,omitempty"`
	ExistingReportID *uuid.UUID  `json:"existing_report_id,omitempty"`
	RepeatContacts   int         `json:"repeat_contacts,omitempty"`
	Closure          *Closure    `json:"verified_evidence,omitempty"`
	Candidates       []Candidate `json:"candidates,omitempty"`
	NeedsSelection   bool        `json:"needs_selection"`
	Guidance         string      `json:"guidance"`
}

var channels = map[string]bool{"": true, "MOBILE": true, "INTERNET": true, "USSD": true, "POS": true, "ATM": true, "CARD": true, "WEB": true, "NIP": true, "GATEWAY": true, "MERCHANT": true}

var (
	tailRe = regexp.MustCompile(`^[0-9]{4}$`)
	refRe  = regexp.MustCompile(`^[A-Za-z0-9._:-]{6,64}$`)
)

func (in ConfirmInput) validate(now time.Time) error {
	switch {
	case in.AmountMinor < 1 || in.AmountMinor > txn.MaxAmountMinor:
		return fmt.Errorf("%w: amount_minor is out of range", ErrInvalid)
	case in.OccurredAt.IsZero():
		return fmt.Errorf("%w: occurred_at is required", ErrInvalid)
	case in.OccurredAt.After(now.Add(5 * time.Minute)):
		return fmt.Errorf("%w: occurred_at is in the future", ErrInvalid)
	case in.OccurredAt.Before(now.AddDate(-2, 0, 0)):
		return fmt.Errorf("%w: occurred_at is more than two years ago", ErrInvalid)
	case in.AccountTail != "" && !tailRe.MatchString(in.AccountTail):
		return fmt.Errorf("%w: account_tail must be four digits", ErrInvalid)
	case in.Reference != "" && !refRe.MatchString(in.Reference):
		return fmt.Errorf("%w: reference has unsupported characters", ErrInvalid)
	case !channels[strings.ToUpper(in.Channel)]:
		return fmt.Errorf("%w: channel is not recognised", ErrInvalid)
	case len(in.Narration) > 120:
		return fmt.Errorf("%w: narration is too long", ErrInvalid)
	}
	return nil
}

// Confirm validates what the customer confirmed, matches it against the customer's own
// transactions, and records the outcome. It can be repeated until the report is
// submitted, for example to pick a candidate.
func (s *Service) Confirm(ctx context.Context, tenant, customerID, reportID uuid.UUID, in ConfirmInput) (Report, error) {
	if err := in.validate(s.now()); err != nil {
		return Report{}, err
	}
	in.Channel = strings.ToUpper(in.Channel)
	var out Report
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM customer_reports WHERE tenant_id = $1 AND customer_id = $2 AND id = $3 FOR UPDATE`, tenant, customerID, reportID); err != nil {
			return err
		}
		cur, err := s.get(ctx, tx, tenant, customerID, reportID)
		if err != nil {
			return err
		}
		if cur.Status == "submitted" {
			return ErrSubmitted
		}
		accts, err := customer.OwnedAccounts(ctx, tx, tenant, customerID)
		if err != nil {
			return err
		}
		if in.AccountTail != "" && !tailMatchesAny(accts, in.AccountTail) && len(accts) > 0 && anyTail(accts) {
			return fmt.Errorf("%w: that account ending does not match any of your accounts", ErrInvalid)
		}
		cands, err := s.candidates(ctx, tx, tenant, accts, in)
		if err != nil {
			return err
		}
		o, err := s.decide(ctx, tx, tenant, customerID, reportID, cur, in, cands)
		if err != nil {
			return err
		}
		conf := map[string]string{"amount_minor": fmt.Sprint(in.AmountMinor), "occurred_at": in.OccurredAt.UTC().Format(time.RFC3339), "account_tail": in.AccountTail,
			"reference": in.Reference, "channel": in.Channel, "narration": in.Narration}
		cj, _ := json.Marshal(conf)
		cands2, _ := json.Marshal(o.Candidates)
		if _, err := tx.Exec(ctx, `UPDATE customer_reports SET status = 'confirmed', confirmed = $4, match_status = $5, candidates = $6, txn_id = $7, case_id = $8,
				updated_at = $9, version = version + 1 WHERE tenant_id = $1 AND customer_id = $2 AND id = $3`,
			tenant, customerID, reportID, cj, o.MatchStatus, cands2, o.TxnID, o.CaseID, s.now()); err != nil {
			return err
		}
		if o.CaseID != nil && o.MatchStatus == MatchExistingCase {
			if _, err := tx.Exec(ctx, `INSERT INTO case_contacts (tenant_id, case_id, report_id, customer_id) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, tenant, *o.CaseID, reportID, customerID); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT count(*)::int FROM case_contacts WHERE tenant_id = $1 AND case_id = $2`, tenant, *o.CaseID).Scan(&o.RepeatContacts); err != nil {
				return err
			}
		}
		out, err = s.get(ctx, tx, tenant, customerID, reportID)
		out.Outcome = &o
		return err
	})
	return out, err
}

func tailMatchesAny(accts []customer.Account, tail string) bool {
	for _, a := range accts {
		if a.Tail == tail {
			return true
		}
	}
	return false
}

func anyTail(accts []customer.Account) bool {
	for _, a := range accts {
		if a.Tail != "" {
			return true
		}
	}
	return false
}

type txnRow struct {
	id        uuid.UUID
	ref       string
	amount    int64
	currency  string
	channel   string
	rail      string
	state     string
	token     string
	debitedAt time.Time
}

// candidates finds the customer's own transactions that the report might be about.
func (s *Service) candidates(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, accts []customer.Account, in ConfirmInput) ([]Candidate, error) {
	if len(accts) == 0 {
		return nil, nil
	}
	tokens := make([]string, 0, len(accts))
	tailOf := map[string]string{}
	for _, a := range accts {
		tokens = append(tokens, a.Token)
		tailOf[a.Token] = a.Tail
	}
	var out []Candidate
	seen := map[uuid.UUID]bool{}
	scan := func(rows pgx.Rows) ([]txnRow, error) {
		defer rows.Close()
		var l []txnRow
		for rows.Next() {
			var t txnRow
			var fd *time.Time
			var first time.Time
			if err := rows.Scan(&t.id, &t.ref, &t.amount, &t.currency, &t.channel, &t.rail, &t.state, &t.token, &fd, &first); err != nil {
				return nil, err
			}
			t.debitedAt = first.UTC()
			if fd != nil {
				t.debitedAt = fd.UTC()
			}
			l = append(l, t)
		}
		return l, rows.Err()
	}
	const sel = `SELECT t.id, t.instruction_ref, t.amount_minor, t.currency, t.channel, t.rail, t.lifecycle_state, t.debit_account_token, t.first_debit_at, t.first_event_at FROM transactions t`

	// Exact reference or session first, among the customer's own transactions only.
	if in.Reference != "" {
		rows, err := tx.Query(ctx, sel+` WHERE t.tenant_id = $1 AND t.debit_account_token = ANY($2)
			AND (t.instruction_ref = $3 OR EXISTS (SELECT 1 FROM transaction_refs r WHERE r.tenant_id = t.tenant_id AND r.txn_id = t.id AND r.ref_value = $3))
			ORDER BY t.first_event_at DESC LIMIT 5`, tenant, tokens, in.Reference)
		if err != nil {
			return nil, err
		}
		list, err := scan(rows)
		if err != nil {
			return nil, err
		}
		for _, t := range list {
			seen[t.id] = true
			out = append(out, toCandidate(t, 1.0, []string{"the reference matches"}, true))
		}
	}
	if len(out) > 0 {
		return out, nil
	}

	// Fuzzy matching needs an approved window. Without one nothing is offered.
	win, err := s.Config.Effective(ctx, tx, tenant, "customer_match.window_seconds", s.now())
	if errors.Is(err, configsvc.ErrNotConfigured) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, sel+` WHERE t.tenant_id = $1 AND t.debit_account_token = ANY($2) AND t.amount_minor = $3
		AND coalesce(t.first_debit_at, t.first_event_at) BETWEEN $4 AND $5 ORDER BY t.first_event_at DESC LIMIT 50`,
		tenant, tokens, in.AmountMinor, in.OccurredAt.Add(-time.Duration(win.Value)*time.Second), in.OccurredAt.Add(time.Duration(win.Value)*time.Second))
	if err != nil {
		return nil, err
	}
	list, err := scan(rows)
	if err != nil {
		return nil, err
	}
	for _, t := range list {
		if seen[t.id] {
			continue
		}
		score := 0.5
		reasons := []string{"the amount matches", fmt.Sprintf("the time is within %d seconds", win.Value)}
		score += 0.25
		if in.AccountTail != "" && tailOf[t.token] == in.AccountTail {
			score += 0.15
			reasons = append(reasons, "the account ending matches")
		}
		if in.Channel != "" && strings.EqualFold(in.Channel, t.channel) {
			score += 0.1
			reasons = append(reasons, "the channel matches")
		}
		out = append(out, toCandidate(t, score, reasons, false))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].OccurredAt.After(out[j].OccurredAt)
	})
	return out, nil
}

func toCandidate(t txnRow, conf float64, reasons []string, exact bool) Candidate {
	return Candidate{TxnID: t.id, InstructionRef: t.ref, Confidence: conf, Reasons: reasons, Exact: exact, Lifecycle: t.state,
		AmountMinor: txn.Amount(t.amount).String(), Currency: t.currency, OccurredAt: t.debitedAt}
}

// decide turns the candidates and the customer's choice into an outcome.
func (s *Service) decide(ctx context.Context, tx pgx.Tx, tenant, customerID, reportID uuid.UUID, cur Report, in ConfirmInput, cands []Candidate) (Outcome, error) {
	o := Outcome{Candidates: cands}
	var chosen *Candidate
	switch {
	case in.CandidateTxnID != nil:
		for i := range cands {
			if cands[i].TxnID == *in.CandidateTxnID {
				chosen = &cands[i]
			}
		}
		if chosen == nil {
			return o, ErrNotAccepted
		}
	case len(cands) == 1 && cands[0].Exact:
		chosen = &cands[0]
	}
	if chosen == nil {
		if len(cands) > 0 {
			o.MatchStatus, o.NeedsSelection = MatchCandidates, true
			o.Guidance = "Please choose the transaction you mean, or say none of these is it."
			return o, nil
		}
		if cur.Parsed.OtherBank {
			o.MatchStatus = MatchOtherBank
			o.Guidance = "This alert appears to come from another bank, so we cannot find it here. Please raise it with that bank; you may still send us a general complaint."
			return o, nil
		}
		o.MatchStatus = MatchUnmatched
		o.Guidance = "We could not find this transaction on your accounts. You can send it to us as a claim and we will look into it."
		return o, nil
	}
	o.TxnID = &chosen.TxnID

	// The same customer already reported this transaction.
	var existing uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM customer_reports WHERE tenant_id = $1 AND customer_id = $2 AND txn_id = $3 AND status = 'submitted' AND id <> $4 ORDER BY submitted_at LIMIT 1`,
		tenant, customerID, chosen.TxnID, reportID).Scan(&existing)
	if err == nil {
		o.MatchStatus, o.ExistingReportID = MatchDuplicateReport, &existing
		var cid *uuid.UUID
		_ = tx.QueryRow(ctx, `SELECT case_id FROM customer_reports WHERE tenant_id = $1 AND id = $2`, tenant, existing).Scan(&cid)
		o.CaseID = cid
		if cid != nil {
			_ = tx.QueryRow(ctx, `SELECT count(*)::int FROM case_contacts WHERE tenant_id = $1 AND case_id = $2`, tenant, *cid).Scan(&o.RepeatContacts)
		}
		o.Guidance = "You have already reported this transaction. You can follow it under your cases, or add a note."
		return o, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return o, err
	}

	// An open case already exists for the transaction.
	var caseID uuid.UUID
	var caseRef string
	err = tx.QueryRow(ctx, `SELECT id, case_ref FROM cases WHERE tenant_id = $1 AND txn_id = $2
			AND status NOT IN ('RESOLVED', 'CLOSED', 'CLOSED_NO_EXCEPTION', 'CLOSED_CUSTOMER_ERROR') ORDER BY opened_at LIMIT 1`, tenant, chosen.TxnID).Scan(&caseID, &caseRef)
	if err == nil {
		o.MatchStatus, o.CaseID, o.CaseRef = MatchExistingCase, &caseID, caseRef
		o.Guidance = "We already know about this transaction and are looking into it. Your report has been added to the same case."
		return o, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return o, err
	}

	// Nothing open: can the bank already say what happened?
	var state, revStatus string
	if err := tx.QueryRow(ctx, `SELECT lifecycle_state FROM transactions WHERE tenant_id = $1 AND id = $2`, tenant, chosen.TxnID).Scan(&state); err != nil {
		return o, err
	}
	_ = tx.QueryRow(ctx, `SELECT financial_status FROM reversals WHERE tenant_id = $1 AND txn_id = $2 ORDER BY created_at DESC LIMIT 1`, tenant, chosen.TxnID).Scan(&revStatus)
	err = nil
	switch {
	case revStatus == "VERIFIED":
		o.MatchStatus = MatchAlreadyReversed
		o.Closure, err = s.closure(ctx, tx, tenant, chosen.TxnID, state, revStatus)
		o.Guidance = "This transaction was reversed and the reversal has been verified. If you did not receive the money, tell us and we will reopen it."
	case state == txn.StateConfirmedSuccess:
		o.MatchStatus = MatchVerifiedSuccess
		o.Closure, err = s.closure(ctx, tx, tenant, chosen.TxnID, state, "")
		o.Guidance = "Our records show this transaction completed. If you did not receive the value, tell us and we will look again."
	default:
		o.MatchStatus = MatchTransaction
		o.Guidance = "We found the transaction. You can send it to us as a report and we will look into it."
	}
	return o, err
}

func (s *Service) closure(ctx context.Context, tx pgx.Tx, tenant, txnID uuid.UUID, state, rev string) (*Closure, error) {
	rows, err := tx.Query(ctx, `SELECT event_type, source_system, source_ts FROM transaction_events WHERE tenant_id = $1 AND txn_id = $2 ORDER BY source_ts, id`, tenant, txnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c := &Closure{Lifecycle: state, Reversal: rev}
	for rows.Next() {
		var it ClosureItem
		if err := rows.Scan(&it.Type, &it.Source, &it.At); err != nil {
			return nil, err
		}
		it.At = it.At.UTC()
		c.Events = append(c.Events, it)
	}
	return c, rows.Err()
}

// ---- the customer's cases -------------------------------------------------------

// VisibleCaseIDs returns the cases a customer may see: those they reported, and those
// the bank detected on transactions of accounts the bank recorded as theirs, whether or
// not the customer ever reported them.
func (s *Service) VisibleCaseIDs(ctx context.Context, tx pgx.Tx, tenant, customerID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT c.id FROM cases c
		WHERE c.tenant_id = $1 AND (
		  EXISTS (SELECT 1 FROM customer_reports r WHERE r.tenant_id = c.tenant_id AND r.customer_id = $2 AND r.case_id = c.id)
		  OR EXISTS (SELECT 1 FROM transactions t JOIN customer_accounts a ON a.tenant_id = t.tenant_id AND a.account_token = t.debit_account_token
		             WHERE t.tenant_id = c.tenant_id AND t.id = c.txn_id AND a.customer_id = $2))
		ORDER BY c.opened_at DESC, c.id LIMIT 200`, tenant, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// CanSeeCase reports whether the case is one of the customer's. A case that is not is
// indistinguishable from one that does not exist.
func (s *Service) CanSeeCase(ctx context.Context, tx pgx.Tx, tenant, customerID, caseID uuid.UUID) (bool, error) {
	ids, err := s.VisibleCaseIDs(ctx, tx, tenant, customerID)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id == caseID {
			return true, nil
		}
	}
	return false, nil
}
