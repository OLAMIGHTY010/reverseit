package creport

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/customer"
	"github.com/smartreversal/smartreversal/apps/api/internal/deadline"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

func (r *rig) setClock(id string, c configsvc.Clock) {
	r.t.Helper()
	r.setConfigValue(configsvc.ClockKey(id), c, "reported")
}

func (r *rig) setConfigValue(key string, v any, verify string) {
	r.t.Helper()
	raw, _ := json.Marshal(v)
	r.in(func(tx pgx.Tx) error {
		ver, err := r.cfg.Propose(context.Background(), tx, r.tenant, uuid.New(), configsvc.ProposeInput{Key: key, Value: raw, SourceReference: "SYNTHETIC DEMO POLICY - not approved for production",
			EffectiveFrom: r.now, VerifyStatus: verify, Reason: "synthetic value for a test"})
		if err != nil {
			return err
		}
		_, err = r.cfg.Decide(context.Background(), tx, r.tenant, uuid.New(), key, ver.Version, true, "")
		return err
	})
}

func (r *rig) policies() {
	r.setClock("FAILED_INSTANT_TRANSFER", configsvc.Clock{Situation: "Failed instant transfer", Duration: 24, Unit: "hours", StartEvent: "dispute_reported", Calendar: "WAT", Owner: "Compliance"})
	r.setClock("WRONG_ACCOUNT_REQUEST", configsvc.Clock{Situation: "Written reversal request", Duration: 14, Unit: "working_days", StartEvent: "transaction_date", Calendar: "WAT", Owner: "Compliance"})
	r.setConfigValue("internal_target_hours.FAILED_INSTANT_TRANSFER", 12, "unverified")
	r.setConfigValue("customer.ack_target_hours", 4, "unverified")
	r.setConfigValue("customer_match.window_seconds", 300, "unverified")
}

var occurred = time.Date(2026, 10, 14, 9, 12, 0, 0, wat)

func (r *rig) confirmed(c customer.Customer, key string, in ConfirmInput) Report {
	r.t.Helper()
	rep := r.create(c, key, alert)
	out, err := r.confirm(c, rep.ID, in)
	if err != nil {
		r.t.Fatal(err)
	}
	return out
}

func (r *rig) notes(caseID uuid.UUID, audience string) []string {
	var out []string
	r.in(func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT body FROM notifications WHERE tenant_id = $1 AND case_id = $2 AND audience = $3 ORDER BY created_at`, r.tenant, caseID, audience)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b string
			if err := rows.Scan(&b); err != nil {
				return err
			}
			out = append(out, b)
		}
		return rows.Err()
	})
	return out
}

func TestAMatchedFailedTransferBecomesACustomerCaseWithTheRightClocksAndOneAcknowledgement(t *testing.T) {
	r := newRig(t)
	r.policies()
	id := r.transfer(tokAlice, "NIP202610140912", 2500000, occurred, txn.EvTransferSent, txn.EvCreditFailed)
	rep := r.confirmed(r.alice, "report-sub-0001", ConfirmInput{AmountMinor: 2500000, OccurredAt: occurred, AccountTail: "6789", Reference: "NIP202610140912"})
	if rep.Outcome.MatchStatus != MatchTransaction {
		t.Fatalf("%+v", rep.Outcome)
	}

	d, err := r.svc.MakeDraft(context.Background(), r.tenant, r.alice.ID, rep.ID, "")
	if err != nil || d.Dispute != deadline.FailedTransfer || d.Classification.ReviewRequired || d.Deadline.Regulatory == nil || d.Deadline.Internal == nil {
		t.Fatalf("%+v %v", d, err)
	}
	for _, want := range []string{"Account ending: 6789", "NGN 25,000.00", "My account was debited", "The credit to the other account failed", "has not been verified"} {
		if !strings.Contains(d.Text, want) {
			t.Errorf("the draft lacks %q:\n%s", want, d.Text)
		}
	}

	r.now = r.now.Add(30 * time.Minute)
	sub, err := r.svc.Submit(context.Background(), r.tenant, r.alice.ID, rep.ID, SubmitInput{})
	if err != nil || !sub.Created || sub.Joined {
		t.Fatalf("%+v %v", sub, err)
	}
	var c cases.Case
	r.in(func(tx pgx.Tx) error {
		var err error
		c, err = r.cases.Get(context.Background(), tx, r.tenant, sub.CaseID)
		return err
	})
	if c.Classification != cases.CodeCustomer || c.Source != cases.SourceCustomer || c.TxnID == nil || *c.TxnID != id || c.RegulatoryClock != "FAILED_INSTANT_TRANSFER" {
		t.Fatalf("%+v", c)
	}
	if c.SLADueAt == nil || !c.SLADueAt.Equal(c.OpenedAt.Add(24*time.Hour)) {
		t.Fatalf("regulatory deadline: %v", c.SLADueAt)
	}
	// The internal target runs from the failure (12 hours), not from the report.
	if c.InternalTargetAt == nil || !c.InternalTargetAt.Equal(occurred.Add(2*time.Second).Add(12*time.Hour)) {
		t.Fatalf("internal target: %v", c.InternalTargetAt)
	}
	cust := r.notes(c.ID, "customer")
	if len(cust) != 1 || !strings.Contains(cust[0], "We have received your report") || !strings.Contains(cust[0], c.Ref) {
		t.Fatalf("%v", cust)
	}
	for _, bad := range []string{"refund", "reversed", "returned", "money back"} {
		if strings.Contains(strings.ToLower(cust[0]), bad) {
			t.Fatalf("the acknowledgement promises money: %s", cust[0])
		}
	}
	if ops := r.notes(c.ID, "operations"); len(ops) != 1 || !strings.Contains(ops[0], "CUS001") {
		t.Fatalf("%v", ops)
	}

	// Submitting again changes nothing.
	again, err := r.svc.Submit(context.Background(), r.tenant, r.alice.ID, rep.ID, SubmitInput{})
	if err != nil || again.CaseID != sub.CaseID || again.Created {
		t.Fatalf("%+v %v", again, err)
	}
	if len(r.notes(c.ID, "customer")) != 1 {
		t.Fatal("a repeat submission sent a second acknowledgement")
	}
	var contacts int
	_ = r.pool.QueryRow(context.Background(), `SELECT count(*) FROM case_contacts WHERE case_id = $1`, c.ID).Scan(&contacts)
	if contacts != 1 {
		t.Fatalf("%d contacts", contacts)
	}
	if _, err := r.confirm(r.alice, rep.ID, ConfirmInput{AmountMinor: 2500000, OccurredAt: occurred}); !errors.Is(err, ErrSubmitted) {
		t.Fatalf("a submitted report was changed: %v", err)
	}
}

func TestTheCustomersEditsAreKeptBesideTheOriginalDraft(t *testing.T) {
	r := newRig(t)
	r.policies()
	r.transfer(tokAlice, "NIP202610140912", 2500000, occurred, txn.EvTransferSent, txn.EvCreditFailed)
	rep := r.confirmed(r.alice, "report-sub-0002", ConfirmInput{AmountMinor: 2500000, OccurredAt: occurred, Reference: "NIP202610140912"})
	original, _ := r.svc.MakeDraft(context.Background(), r.tenant, r.alice.ID, rep.ID, "")
	if err := r.svc.SaveEdit(context.Background(), r.tenant, r.alice.ID, rep.ID, "My own words.\x00 Please help."); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.SaveEdit(context.Background(), r.tenant, r.alice.ID, rep.ID, strings.Repeat("a", 5000)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("%v", err)
	}
	if err := r.svc.SaveEdit(context.Background(), r.tenant, r.bob.ID, rep.ID, "hello"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another customer edited the report: %v", err)
	}
	sub, err := r.svc.Submit(context.Background(), r.tenant, r.alice.ID, rep.ID, SubmitInput{})
	if err != nil {
		t.Fatal(err)
	}
	var draft, edited string
	_ = r.pool.QueryRow(context.Background(), `SELECT draft, edited FROM customer_reports WHERE id = $1`, rep.ID).Scan(&draft, &edited)
	if draft != original.Text && !strings.Contains(draft, "Complaint") || edited != "My own words. Please help." {
		t.Fatalf("draft %q edited %q", draft, edited)
	}
	var evidence string
	_ = r.pool.QueryRow(context.Background(), `SELECT detail::text FROM case_evidence WHERE case_id = $1 AND kind = 'complaint'`, sub.CaseID).Scan(&evidence)
	if !strings.Contains(evidence, "My own words.") {
		t.Fatalf("the case carries %s", evidence)
	}
}

func TestAnUnmatchedClaimAndAWrongAccountReportGetTheirOwnClocksAndCodes(t *testing.T) {
	r := newRig(t)
	r.policies()
	un := r.confirmed(r.alice, "report-sub-0003", ConfirmInput{AmountMinor: 777700, OccurredAt: occurred})
	if un.Outcome.MatchStatus != MatchUnmatched {
		t.Fatalf("%+v", un.Outcome)
	}
	sub, err := r.svc.Submit(context.Background(), r.tenant, r.alice.ID, un.ID, SubmitInput{})
	if err != nil || sub.Dispute != deadline.UnmatchedClaim || sub.Deadline.Regulatory != nil || sub.Deadline.Internal == nil || !sub.Deadline.Internal.Due.Equal(r.now.Add(4*time.Hour)) {
		t.Fatalf("an unmatched claim has an acknowledgement target and no regulatory clock: %+v %v", sub, err)
	}
	var c cases.Case
	r.in(func(tx pgx.Tx) error {
		var err error
		c, err = r.cases.Get(context.Background(), tx, r.tenant, sub.CaseID)
		return err
	})
	if c.Classification != cases.CodeCustomer || c.TxnID != nil || c.SLADueAt != nil {
		t.Fatalf("%+v", c)
	}

	r.transfer(tokAlice, "NIP202610140555", 900000, occurred, txn.EvTransferSent, txn.EvCreditConfirmed)
	wrong := r.confirmed(r.alice, "report-sub-0004", ConfirmInput{AmountMinor: 900000, OccurredAt: occurred, Reference: "NIP202610140555"})
	// The bank shows it completed. The customer says it went to the wrong account: that is a different problem from "I did not receive it".
	if _, err := r.svc.Submit(context.Background(), r.tenant, r.alice.ID, wrong.ID, SubmitInput{DisputeType: deadline.WrongAccountOrAm}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a completed transfer needs the customer to say what is wrong: %v", err)
	}
	r.setConfigValue("customer_match.window_seconds", 600, "unverified")
	sub, err = r.svc.Submit(context.Background(), r.tenant, r.alice.ID, wrong.ID, SubmitInput{DisputeType: deadline.WrongAccountOrAm, DisputesVerifiedOutcome: true})
	if err != nil || sub.Dispute != deadline.WrongAccountOrAm || !sub.Review {
		t.Fatalf("%+v %v", sub, err)
	}
	r.in(func(tx pgx.Tx) error {
		var err error
		c, err = r.cases.Get(context.Background(), tx, r.tenant, sub.CaseID)
		return err
	})
	if c.Classification != cases.CodeWrongAcct || c.RegulatoryClock != "WRONG_ACCOUNT_REQUEST" || c.SLADueAt == nil {
		t.Fatalf("%+v", c)
	}
}

func TestAReportOnAnOpenCaseJoinsItInsteadOfStartingAnother(t *testing.T) {
	r := newRig(t)
	r.policies()
	id := r.transfer(tokAlice, "NIP202610140912", 2500000, occurred, txn.EvTransferSent, txn.EvCreditFailed)
	existing := r.openCase(id)
	rep := r.confirmed(r.alice, "report-sub-0005", ConfirmInput{AmountMinor: 2500000, OccurredAt: occurred, Reference: "NIP202610140912"})
	sub, err := r.svc.Submit(context.Background(), r.tenant, r.alice.ID, rep.ID, SubmitInput{})
	if err != nil || !sub.Joined || sub.Created || sub.CaseID != existing.ID {
		t.Fatalf("%+v %v", sub, err)
	}
	var n int
	_ = r.pool.QueryRow(context.Background(), `SELECT count(*) FROM cases WHERE tenant_id = $1`, r.tenant).Scan(&n)
	if n != 1 {
		t.Fatalf("%d cases", n)
	}
	var notes int
	_ = r.pool.QueryRow(context.Background(), `SELECT count(*) FROM case_events WHERE case_id = $1 AND kind = 'note'`, existing.ID).Scan(&notes)
	if notes != 1 {
		t.Fatalf("%d notes", notes)
	}
}

func TestSubmissionRefusesWhatCannotBecomeACase(t *testing.T) {
	r := newRig(t)
	r.policies()
	r.transfer(tokAlice, "NIP202610140912", 2500000, occurred, txn.EvTransferSent, txn.EvCreditConfirmed)
	created := r.create(r.alice, "report-sub-0006", alert)
	if _, err := r.svc.Submit(context.Background(), r.tenant, r.alice.ID, created.ID, SubmitInput{}); !errors.Is(err, ErrNotReady) {
		t.Fatalf("an unconfirmed report was submitted: %v", err)
	}
	other := r.create(r.alice, "report-sub-0007", "GTBank Alert: Acct 0987654321 debited NGN 5,000.00 on 2026-10-14 10:00. Ref: GTB99887766")
	if _, err := r.confirm(r.alice, other.ID, ConfirmInput{AmountMinor: 500000, OccurredAt: time.Date(2026, 10, 14, 10, 0, 0, 0, wat)}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Submit(context.Background(), r.tenant, r.alice.ID, other.ID, SubmitInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("another bank's alert became a case: %v", err)
	}
	// Another customer cannot draft or submit Alice's report.
	if _, err := r.svc.MakeDraft(context.Background(), r.tenant, r.bob.ID, created.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if _, err := r.svc.Submit(context.Background(), r.tenant, r.bob.ID, created.ID, SubmitInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	// An unresolved choice between candidates must be made first.
	r.transfer(tokAlice, "NIP202610140601", 111100, occurred.Add(10*time.Second), txn.EvTransferSent, txn.EvCreditConfirmed)
	pick := r.confirmed(r.alice, "report-sub-0008", ConfirmInput{AmountMinor: 111100, OccurredAt: occurred, AccountTail: "6789"})
	if pick.Outcome.MatchStatus != MatchCandidates {
		t.Fatalf("%+v", pick.Outcome)
	}
	if _, err := r.svc.Submit(context.Background(), r.tenant, r.alice.ID, pick.ID, SubmitInput{}); !errors.Is(err, ErrNotReady) {
		t.Fatalf("%v", err)
	}
}

func TestAnAtmTransactionNeedsTheCustomerToSayWhoseMachineItWas(t *testing.T) {
	r := newRig(t)
	r.policies()
	r.setClock("ATM_NOT_ON_US", configsvc.Clock{Situation: "Failed ATM, not-on-us", Duration: 48, Unit: "hours", StartEvent: "failure_detected", Calendar: "WAT", Owner: "Compliance"})
	r.n++
	r.in(func(tx pgx.Tx) error {
		for i, typ := range []string{txn.EvDebitPosted, txn.EvCreditFailed} {
			if _, err := r.ing.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "ATM_SWITCH", SourceEventRef: "EV-ATM-" + typ, EventType: typ, SourceTS: occurred.Add(time.Duration(i) * time.Second),
				Txn: txn.Core{InstructionRef: "ATM202610140001", Channel: "ATM", Rail: "ATM", AmountMinor: 2000000, Currency: "NGN", DebitAccountToken: tokAlice}}); err != nil {
				return err
			}
		}
		return nil
	})
	rep := r.confirmed(r.alice, "report-sub-0009", ConfirmInput{AmountMinor: 2000000, OccurredAt: occurred, Reference: "ATM202610140001"})
	d, err := r.svc.MakeDraft(context.Background(), r.tenant, r.alice.ID, rep.ID, "")
	if err == nil || !errors.Is(err, ErrNotReady) || !strings.Contains(err.Error(), "which kind") {
		t.Fatalf("an ATM report must not guess whose machine it was: %+v %v", d, err)
	}
	d, err = r.svc.MakeDraft(context.Background(), r.tenant, r.alice.ID, rep.ID, deadline.AtmNotOnUs)
	if err != nil || d.Dispute != deadline.AtmNotOnUs || !d.Classification.ReviewRequired || d.Deadline.Regulatory == nil || d.Deadline.Regulatory.Duration != 48 {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := r.svc.MakeDraft(context.Background(), r.tenant, r.alice.ID, rep.ID, deadline.UnmatchedClaim); !errors.Is(err, ErrInvalid) {
		t.Fatalf("an unmatched-claim type on a matched transaction: %v", err)
	}
	if _, err := r.svc.MakeDraft(context.Background(), r.tenant, r.alice.ID, rep.ID, "NOT_A_TYPE"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("%v", err)
	}
}

func TestFeedbackIsRecordedOnceAndADisputeOfAClosedOutcomeReopensTheCaseWithoutTouchingMoney(t *testing.T) {
	r := newRig(t)
	r.policies()
	r.transfer(tokAlice, "NIP202610140912", 2500000, occurred, txn.EvTransferSent, txn.EvCreditFailed)
	rep := r.confirmed(r.alice, "report-fb-0001", ConfirmInput{AmountMinor: 2500000, OccurredAt: occurred, Reference: "NIP202610140912"})
	yes := true

	// Feedback before submission is refused.
	if _, err := r.svc.Feedback(context.Background(), r.tenant, r.alice.ID, rep.ID, FeedbackInput{IdempotencyKey: "feedback-0001", Helpful: &yes}); !errors.Is(err, ErrNotReady) {
		t.Fatalf("%v", err)
	}
	sub, err := r.svc.Submit(context.Background(), r.tenant, r.alice.ID, rep.ID, SubmitInput{})
	if err != nil {
		t.Fatal(err)
	}
	fb, err := r.svc.Feedback(context.Background(), r.tenant, r.alice.ID, rep.ID, FeedbackInput{IdempotencyKey: "feedback-0001", Helpful: &yes, Comment: "Thanks"})
	if err != nil || fb.Reopened || fb.CaseRef != sub.CaseRef {
		t.Fatalf("%+v %v", fb, err)
	}
	if again, _ := r.svc.Feedback(context.Background(), r.tenant, r.alice.ID, rep.ID, FeedbackInput{IdempotencyKey: "feedback-0001", Helpful: &yes}); !again.Repeat || again.ID != fb.ID {
		t.Fatalf("%+v", again)
	}

	// While the case is open a dispute adds a note and reopens nothing.
	open, err := r.svc.Feedback(context.Background(), r.tenant, r.alice.ID, rep.ID, FeedbackInput{IdempotencyKey: "feedback-0002", DisputesOutcome: true, Comment: "Still not received"})
	if err != nil || open.Reopened || !strings.Contains(open.Message, "still open") {
		t.Fatalf("%+v %v", open, err)
	}

	// Once the case is closed, the same statement reopens it.
	closeCase := func() {
		r.in(func(tx pgx.Tx) error {
			for _, to := range []string{cases.StUnderInvestigation, cases.StClosedNoException} {
				c, _ := r.cases.Get(context.Background(), tx, r.tenant, sub.CaseID)
				if c.Status == to || cases.IsTerminal(c.Status) {
					continue
				}
				if _, err := r.cases.Transition(context.Background(), tx, r.tenant, sub.CaseID, cases.System, to, 0, "CHECKED"); err != nil {
					return err
				}
			}
			return nil
		})
	}
	closeCase()
	re, err := r.svc.Feedback(context.Background(), r.tenant, r.alice.ID, rep.ID, FeedbackInput{IdempotencyKey: "feedback-0003", DisputesOutcome: true})
	if err != nil || !re.Reopened || !strings.Contains(re.Message, "reopened") {
		t.Fatalf("%+v %v", re, err)
	}
	var status string
	_ = r.pool.QueryRow(context.Background(), `SELECT status FROM cases WHERE id = $1`, sub.CaseID).Scan(&status)
	if status != cases.StReopened {
		t.Fatalf("case is %s", status)
	}
	var reversals int
	_ = r.pool.QueryRow(context.Background(), `SELECT count(*) FROM reversals WHERE tenant_id = $1`, r.tenant).Scan(&reversals)
	if reversals != 0 {
		t.Fatal("feedback created a reversal")
	}

	// Validation, and another customer cannot give feedback on this report.
	for name, in := range map[string]FeedbackInput{
		"short key":    {IdempotencyKey: "k", Helpful: &yes},
		"nothing said": {IdempotencyKey: "feedback-0004"},
		"long comment": {IdempotencyKey: "feedback-0005", Comment: strings.Repeat("c", 1001)},
	} {
		if _, err := r.svc.Feedback(context.Background(), r.tenant, r.alice.ID, rep.ID, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := r.svc.Feedback(context.Background(), r.tenant, r.bob.ID, rep.ID, FeedbackInput{IdempotencyKey: "feedback-0006", Helpful: &yes}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
}
