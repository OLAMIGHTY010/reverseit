package creport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/alertparse"
	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/customer"
	"github.com/smartreversal/smartreversal/apps/api/internal/deadline"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

var wat = time.FixedZone("WAT", 3600)

const (
	tokAlice = "acct_0123456789abcdef0123456789abcdef"
	tokBob   = "acct_fedcba9876543210fedcba9876543210"
	tokBen   = "acct_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

type rig struct {
	pool   *db.Pool
	tenant uuid.UUID
	now    time.Time
	svc    *Service
	cfg    *configsvc.Service
	cases  *cases.Service
	ing    *txn.Service
	alice  customer.Customer
	bob    customer.Customer
	n      int
	t      *testing.T
}

type brokenParser struct{}

func (brokenParser) Parse(context.Context, string) (alertparse.Parsed, error) {
	return alertparse.Parsed{}, alertparse.ErrUnavailable
}

func newRig(t *testing.T) *rig {
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), now: time.Date(2026, 10, 15, 12, 0, 0, 0, wat), t: t}
	clk := func() time.Time { return r.now }
	app := &audit.Appender{Now: clk}
	r.cfg = &configsvc.Service{Audit: app, Now: clk}
	r.cases = &cases.Service{Audit: app, Config: r.cfg, Now: clk}
	r.ing = &txn.Service{Now: clk}
	r.svc = &Service{Pool: pool, Config: r.cfg, Now: clk, Cases: r.cases, Deadlines: &deadline.Engine{Config: r.cfg, Now: clk},
		Parser: &alertparse.RuleParser{OwnNames: []string{"Wema"}, Now: clk}}
	cs := &customer.Store{Pool: pool}
	var err error
	if r.alice, err = cs.Create(context.Background(), r.tenant, "Alice", "synthetic customer passphrase"); err != nil {
		t.Fatal(err)
	}
	if r.bob, err = cs.Create(context.Background(), r.tenant, "Bob", "synthetic customer passphrase"); err != nil {
		t.Fatal(err)
	}
	_ = cs.LinkAccount(context.Background(), r.tenant, r.alice.ID, tokAlice, "6789", "bank onboarding")
	_ = cs.LinkAccount(context.Background(), r.tenant, r.bob.ID, tokBob, "4321", "bank onboarding")
	return r
}

func (r *rig) in(fn func(tx pgx.Tx) error) {
	r.t.Helper()
	if err := r.pool.WithTenant(context.Background(), r.tenant, fn); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) setConfig(key string, v int) {
	r.t.Helper()
	raw, _ := json.Marshal(v)
	r.in(func(tx pgx.Tx) error {
		ver, err := r.cfg.Propose(context.Background(), tx, r.tenant, uuid.New(), configsvc.ProposeInput{Key: key, Value: raw, SourceReference: "SYNTHETIC DEMO POLICY - not approved for production",
			EffectiveFrom: r.now, VerifyStatus: "unverified", Reason: "synthetic value for a test"})
		if err != nil {
			return err
		}
		_, err = r.cfg.Decide(context.Background(), tx, r.tenant, uuid.New(), key, ver.Version, true, "")
		return err
	})
}

// transfer ingests a debit (and optionally more events) on an account, returning the txn id.
func (r *rig) transfer(token, ref string, amount int64, at time.Time, more ...string) uuid.UUID {
	r.t.Helper()
	var id uuid.UUID
	types := append([]string{txn.EvDebitPosted}, more...)
	for i, typ := range types {
		r.n++
		r.in(func(tx pgx.Tx) error {
			res, err := r.ing.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: fmt.Sprintf("EV-%s-%d", ref, r.n), EventType: typ,
				SourceTS: at.Add(time.Duration(i) * time.Second), Txn: txn.Core{InstructionRef: ref, Channel: "MOBILE", Rail: "NIP", AmountMinor: txn.Amount(amount), Currency: "NGN",
					DebitAccountToken: token, BeneficiaryToken: tokBen, SessionID: "SES-" + ref},
				Refs: txn.Refs{"RRN": "RRN" + ref[len(ref)-6:]}})
			id = res.TxnID
			return err
		})
	}
	return id
}

func (r *rig) openCase(txnID uuid.UUID) cases.Case {
	r.t.Helper()
	var c cases.Case
	r.in(func(tx pgx.Tx) error {
		var err error
		c, _, err = r.cases.Open(context.Background(), tx, r.tenant, cases.OpenInput{Classification: cases.CodeNoCredit, Source: cases.SourceBot, TxnID: &txnID, Rail: "NIP",
			RuleID: "DET-FAIL-1", RuleVersion: "1.0", Explanation: "test", Actor: cases.System})
		return err
	})
	return c
}

func (r *rig) create(c customer.Customer, key, text string) Report {
	r.t.Helper()
	rep, err := r.svc.Create(context.Background(), r.tenant, c.ID, CreateInput{IdempotencyKey: key, Text: text, Consent: true, ConsentVersion: "v1"})
	if err != nil {
		r.t.Fatal(err)
	}
	return rep
}

func (r *rig) confirm(c customer.Customer, id uuid.UUID, in ConfirmInput) (Report, error) {
	return r.svc.Confirm(context.Background(), r.tenant, c.ID, id, in)
}

const alert = "Wema Alert: Acct 0123456789 debited NGN 25,000.00 on 2026-10-14 09:12. Desc: NIP TRANSFER | Ref: NIP202610140912"

func TestAReportKeepsOnlyTheRedactedAlertItsHashAndTheConsent(t *testing.T) {
	r := newRig(t)
	rep := r.create(r.alice, "report-key-0001", alert)
	if rep.Status != "created" || rep.ParserStatus != "parsed" || rep.Parsed.Fields["amount_minor"].Value != "2500000" {
		t.Fatalf("%+v", rep)
	}
	var redacted, hash string
	var consent time.Time
	var version string
	if err := r.pool.QueryRow(context.Background(), `SELECT alert_redacted, alert_hash, consent_at, consent_version FROM customer_reports WHERE id = $1`, rep.ID).Scan(&redacted, &hash, &consent, &version); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(alert))
	if strings.Contains(redacted, "0123456789") || !strings.Contains(redacted, "6789") || hash != hex.EncodeToString(sum[:]) || version != "v1" || !consent.Equal(r.now) {
		t.Fatalf("redacted %q hash %s", redacted, hash)
	}
	var raw int
	_ = r.pool.QueryRow(context.Background(), `SELECT count(*) FROM customer_reports WHERE tenant_id = $1 AND (alert_redacted LIKE '%0123456789%' OR parsed::text LIKE '%0123456789%')`, r.tenant).Scan(&raw)
	if raw != 0 {
		t.Fatal("the raw account number was stored")
	}
}

func TestCreateIsIdempotentNeedsConsentAndValidatesItsInput(t *testing.T) {
	r := newRig(t)
	a := r.create(r.alice, "report-key-0002", alert)
	b := r.create(r.alice, "report-key-0002", alert)
	if a.ID != b.ID || !b.Idempotent {
		t.Fatalf("%v %v", a.ID, b.ID)
	}
	if other := r.create(r.bob, "report-key-0002", alert); other.ID == a.ID {
		t.Fatal("two customers shared an idempotency key")
	}
	ctx := context.Background()
	for name, in := range map[string]CreateInput{
		"no consent":         {IdempotencyKey: "report-key-0003", Text: alert, ConsentVersion: "v1"},
		"no consent version": {IdempotencyKey: "report-key-0003", Text: alert, Consent: true},
		"no text or image":   {IdempotencyKey: "report-key-0003", Consent: true, ConsentVersion: "v1"},
		"short key":          {IdempotencyKey: "k", Text: alert, Consent: true, ConsentVersion: "v1"},
		"odd image ref":      {IdempotencyKey: "report-key-0003", ImageRef: "a b;c", Consent: true, ConsentVersion: "v1"},
		"too long":           {IdempotencyKey: "report-key-0003", Text: strings.Repeat("a", alertparse.MaxAlertLength+1), Consent: true, ConsentVersion: "v1"},
	} {
		_, err := r.svc.Create(ctx, r.tenant, r.alice.ID, in)
		if name == "no consent" && !errors.Is(err, ErrConsent) || name != "no consent" && !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAParserThatIsDownOrAnImageGivesTheGuidedFormNotAFailure(t *testing.T) {
	r := newRig(t)
	r.svc.Parser = brokenParser{}
	rep := r.create(r.alice, "report-key-0004", alert)
	if rep.ParserStatus != "unavailable" || !rep.GuidedForm || len(rep.Parsed.Fields) != 0 {
		t.Fatalf("%+v", rep)
	}
	img, err := r.svc.Create(context.Background(), r.tenant, r.alice.ID, CreateInput{IdempotencyKey: "report-key-0005", ImageRef: "uploads/alerts/abc123.png", Consent: true, ConsentVersion: "v1"})
	if err != nil || img.ParserStatus != "image_needs_parser" || !img.GuidedForm {
		t.Fatalf("%+v %v", img, err)
	}
}

func TestReportsAreRateLimitedPerCustomerWithAnApprovableLimit(t *testing.T) {
	r := newRig(t)
	for i := 0; i < DefaultReportsPerHour; i++ {
		r.create(r.alice, fmt.Sprintf("report-rate-%04d", i), alert)
	}
	_, err := r.svc.Create(context.Background(), r.tenant, r.alice.ID, CreateInput{IdempotencyKey: "report-rate-9999", Text: alert, Consent: true, ConsentVersion: "v1"})
	var rl *RateLimitedError
	if !errors.As(err, &rl) || rl.RetryAfter <= 0 || rl.RetryAfter > time.Hour {
		t.Fatalf("%v", err)
	}
	r.create(r.bob, "report-rate-0000", alert) // one customer's limit is not another's
	r.setConfig("customer.reports_per_hour", 12)
	r.create(r.alice, "report-rate-9999", alert)
	r.now = r.now.Add(61 * time.Minute)
	r.setConfig("customer.reports_per_hour", 1)
	r.create(r.alice, "report-rate-late1", alert)
	if _, err := r.svc.Create(context.Background(), r.tenant, r.alice.ID, CreateInput{IdempotencyKey: "report-rate-late2", Text: alert, Consent: true, ConsentVersion: "v1"}); !errors.As(err, &rl) {
		t.Fatalf("an approved lower limit was not applied: %v", err)
	}
}

func TestAnExactReferenceLinksOnlyToTheCustomersOwnTransaction(t *testing.T) {
	r := newRig(t)
	mine := r.transfer(tokAlice, "NIP202610140912", 2500000, time.Date(2026, 10, 14, 9, 12, 0, 0, wat))
	theirs := r.transfer(tokBob, "NIP202610140999", 2500000, time.Date(2026, 10, 14, 9, 12, 0, 0, wat))
	_ = theirs
	rep := r.create(r.alice, "report-key-0010", alert)
	in := ConfirmInput{AmountMinor: 2500000, OccurredAt: time.Date(2026, 10, 14, 9, 12, 0, 0, wat), AccountTail: "6789", Reference: "NIP202610140912"}
	out, err := r.confirm(r.alice, rep.ID, in)
	if err != nil || out.Outcome.MatchStatus != MatchTransaction || out.Outcome.TxnID == nil || *out.Outcome.TxnID != mine || out.Outcome.NeedsSelection {
		t.Fatalf("%+v %v", out.Outcome, err)
	}
	// Quoting someone else's reference finds nothing, exactly as if it did not exist.
	rep2 := r.create(r.alice, "report-key-0011", alert)
	in.Reference = "NIP202610140999"
	out, err = r.confirm(r.alice, rep2.ID, in)
	if err != nil || out.Outcome.MatchStatus != MatchUnmatched || out.Outcome.TxnID != nil || len(out.Outcome.Candidates) != 0 {
		t.Fatalf("someone else's reference leaked: %+v %v", out.Outcome, err)
	}
	in.Reference = "NIP202610149999"
	rep3 := r.create(r.alice, "report-key-0012", alert)
	out2, _ := r.confirm(r.alice, rep3.ID, in)
	if out2.Outcome.MatchStatus != out.Outcome.MatchStatus || out2.Outcome.Guidance != out.Outcome.Guidance {
		t.Fatal("a real reference of someone else and a made-up one are distinguishable")
	}
}

func TestFuzzyMatchingNeedsAnApprovedWindowThenOffersRankedCandidatesTheCustomerChooses(t *testing.T) {
	r := newRig(t)
	at := time.Date(2026, 10, 14, 9, 12, 0, 0, wat)
	near := r.transfer(tokAlice, "NIP202610140101", 2500000, at.Add(40*time.Second))
	far := r.transfer(tokAlice, "NIP202610140202", 2500000, at.Add(50*time.Minute))
	other := r.transfer(tokBob, "NIP202610140303", 2500000, at)
	_, _ = far, other
	rep := r.create(r.alice, "report-key-0020", alert)
	in := ConfirmInput{AmountMinor: 2500000, OccurredAt: at, AccountTail: "6789", Channel: "mobile"}

	out, err := r.confirm(r.alice, rep.ID, in)
	if err != nil || out.Outcome.MatchStatus != MatchUnmatched || len(out.Outcome.Candidates) != 0 {
		t.Fatalf("without an approved window nothing may be offered: %+v %v", out.Outcome, err)
	}
	r.setConfig("customer_match.window_seconds", 300)
	out, err = r.confirm(r.alice, rep.ID, in)
	if err != nil || out.Outcome.MatchStatus != MatchCandidates || !out.Outcome.NeedsSelection || len(out.Outcome.Candidates) != 1 {
		t.Fatalf("%+v %v", out.Outcome, err)
	}
	c := out.Outcome.Candidates[0]
	if c.TxnID != near || c.Exact || c.Confidence != 1.0 || len(c.Reasons) < 4 {
		t.Fatalf("one transaction in the window, on the right account, with the right amount, tail and channel: %+v", c)
	}
	if out.Outcome.TxnID != nil {
		t.Fatal("a fuzzy match was linked without the customer choosing")
	}
	// Choosing something that was not offered is refused, including another customer's transaction.
	var bobs uuid.UUID
	_ = r.pool.QueryRow(context.Background(), `SELECT id FROM transactions WHERE instruction_ref = 'NIP202610140303'`).Scan(&bobs)
	for _, id := range []uuid.UUID{far, bobs, uuid.New()} {
		id := id
		in2 := in
		in2.CandidateTxnID = &id
		if _, err := r.confirm(r.alice, rep.ID, in2); !errors.Is(err, ErrNotAccepted) {
			t.Fatalf("%v", err)
		}
	}
	in.CandidateTxnID = &near
	out, err = r.confirm(r.alice, rep.ID, in)
	if err != nil || out.Outcome.MatchStatus != MatchTransaction || *out.Outcome.TxnID != near || out.TxnID == nil || *out.TxnID != near {
		t.Fatalf("%+v %v", out.Outcome, err)
	}
}

func TestAnOpenCaseTakesTheReportAsARepeatContactAndEachReportCountsOnce(t *testing.T) {
	r := newRig(t)
	at := time.Date(2026, 10, 14, 9, 12, 0, 0, wat)
	id := r.transfer(tokAlice, "NIP202610140912", 2500000, at, txn.EvTransferSent, txn.EvCreditFailed)
	c := r.openCase(id)
	in := ConfirmInput{AmountMinor: 2500000, OccurredAt: at, AccountTail: "6789", Reference: "NIP202610140912"}
	a := r.create(r.alice, "report-key-0030", alert)
	out, err := r.confirm(r.alice, a.ID, in)
	if err != nil || out.Outcome.MatchStatus != MatchExistingCase || out.Outcome.CaseID == nil || *out.Outcome.CaseID != c.ID || out.Outcome.RepeatContacts != 1 {
		t.Fatalf("%+v %v", out.Outcome, err)
	}
	if again, _ := r.confirm(r.alice, a.ID, in); again.Outcome.RepeatContacts != 1 {
		t.Fatalf("confirming the same report again counted a second contact: %d", again.Outcome.RepeatContacts)
	}
	b := r.create(r.alice, "report-key-0031", alert)
	if out, _ = r.confirm(r.alice, b.ID, in); out.Outcome.RepeatContacts != 2 {
		t.Fatalf("%d", out.Outcome.RepeatContacts)
	}
}

func TestATransactionTheBankCanAlreadyExplainIsShownWithItsEvidence(t *testing.T) {
	r := newRig(t)
	at := time.Date(2026, 10, 14, 9, 12, 0, 0, wat)
	r.transfer(tokAlice, "NIP202610140912", 2500000, at, txn.EvTransferSent, txn.EvCreditConfirmed)
	rep := r.create(r.alice, "report-key-0040", alert)
	out, err := r.confirm(r.alice, rep.ID, ConfirmInput{AmountMinor: 2500000, OccurredAt: at, Reference: "NIP202610140912"})
	if err != nil || out.Outcome.MatchStatus != MatchVerifiedSuccess || out.Outcome.Closure == nil || out.Outcome.Closure.Lifecycle != txn.StateConfirmedSuccess || len(out.Outcome.Closure.Events) != 3 {
		t.Fatalf("%+v %v", out.Outcome, err)
	}
}

func TestTheSameTransactionReportedTwiceLinksToTheFirstReport(t *testing.T) {
	r := newRig(t)
	at := time.Date(2026, 10, 14, 9, 12, 0, 0, wat)
	r.transfer(tokAlice, "NIP202610140912", 2500000, at, txn.EvTransferSent, txn.EvCreditConfirmed)
	in := ConfirmInput{AmountMinor: 2500000, OccurredAt: at, Reference: "NIP202610140912"}
	first := r.create(r.alice, "report-key-0050", alert)
	if _, err := r.confirm(r.alice, first.ID, in); err != nil {
		t.Fatal(err)
	}
	if _, err := r.pool.Exec(context.Background(), `UPDATE customer_reports SET status = 'submitted', submitted_at = now() WHERE id = $1`, first.ID); err != nil {
		t.Fatal(err)
	}
	second := r.create(r.alice, "report-key-0051", alert)
	out, err := r.confirm(r.alice, second.ID, in)
	if err != nil || out.Outcome.MatchStatus != MatchDuplicateReport || out.Outcome.ExistingReportID == nil || *out.Outcome.ExistingReportID != first.ID {
		t.Fatalf("%+v %v", out.Outcome, err)
	}
	if _, err := r.confirm(r.alice, first.ID, in); !errors.Is(err, ErrSubmitted) {
		t.Fatalf("a submitted report was changed: %v", err)
	}
}

func TestAnAlertFromAnotherBankIsExplainedAndOtherCustomersReportsAreInvisible(t *testing.T) {
	r := newRig(t)
	other := r.create(r.alice, "report-key-0060", "GTBank Alert: Acct 0987654321 debited NGN 5,000.00 on 2026-10-14 10:00. Ref: GTB99887766")
	if !other.Parsed.OtherBank {
		t.Fatalf("%+v", other.Parsed)
	}
	out, err := r.confirm(r.alice, other.ID, ConfirmInput{AmountMinor: 500000, OccurredAt: time.Date(2026, 10, 14, 10, 0, 0, 0, wat)})
	if err != nil || out.Outcome.MatchStatus != MatchOtherBank || !strings.Contains(out.Outcome.Guidance, "another bank") {
		t.Fatalf("%+v %v", out.Outcome, err)
	}
	// Bob can neither read nor change Alice's report, and gets the same answer as for a report that does not exist.
	if _, err := r.svc.Get(context.Background(), r.tenant, r.bob.ID, other.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if _, err := r.confirm(r.bob, other.ID, ConfirmInput{AmountMinor: 500000, OccurredAt: time.Date(2026, 10, 14, 10, 0, 0, 0, wat)}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if list, _ := r.svc.List(context.Background(), r.tenant, r.bob.ID); len(list) != 0 {
		t.Fatalf("%d reports listed for another customer", len(list))
	}
}

func TestConfirmationRevalidatesEveryField(t *testing.T) {
	r := newRig(t)
	rep := r.create(r.alice, "report-key-0070", alert)
	ok := ConfirmInput{AmountMinor: 2500000, OccurredAt: time.Date(2026, 10, 14, 9, 12, 0, 0, wat), AccountTail: "6789"}
	for name, mut := range map[string]func(*ConfirmInput){
		"zero amount":         func(c *ConfirmInput) { c.AmountMinor = 0 },
		"huge amount":         func(c *ConfirmInput) { c.AmountMinor = txn.MaxAmountMinor + 1 },
		"no time":             func(c *ConfirmInput) { c.OccurredAt = time.Time{} },
		"time in the future":  func(c *ConfirmInput) { c.OccurredAt = r.now.Add(time.Hour) },
		"three years ago":     func(c *ConfirmInput) { c.OccurredAt = r.now.AddDate(-3, 0, 0) },
		"long tail":           func(c *ConfirmInput) { c.AccountTail = "123456" },
		"someone else's tail": func(c *ConfirmInput) { c.AccountTail = "4321" },
		"odd reference":       func(c *ConfirmInput) { c.Reference = "a b" },
		"odd channel":         func(c *ConfirmInput) { c.Channel = "FAX" },
		"long narration":      func(c *ConfirmInput) { c.Narration = strings.Repeat("n", 121) },
	} {
		in := ok
		mut(&in)
		if _, err := r.confirm(r.alice, rep.ID, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := r.confirm(r.alice, rep.ID, ok); err != nil {
		t.Fatal(err)
	}
}
