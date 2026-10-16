// Package complaint writes the complaint a customer sends with a report.
//
// The text is built from structured facts by a fixed template, so it contains facts
// only, in neutral words, with "not provided" wherever a fact is missing. A language
// model may be offered the draft to improve its phrasing, but whatever it returns must
// pass ValidateRephrase: every number, date and reference in it has to be one that is
// already in the draft. If the model is down, or its text is refused, the template text
// is used. The customer owns the final words; the original draft is kept with their edits.
package complaint

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/deadline"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
)

// MaxLength bounds a complaint.
const MaxLength = 4000

// ErrRephrase wraps every reason a rephrased text is refused.
var ErrRephrase = errors.New("complaint: the rephrased text is not acceptable")

// TimelineItem is one dated step in what happened.
type TimelineItem struct {
	At   time.Time
	What string
}

// Facts are everything a complaint may say.
type Facts struct {
	CaseRef     string
	CustomerRef string
	AccountTail string
	AmountMinor int64
	Currency    string
	OccurredAt  time.Time
	Narration   string
	Reference   string
	SessionID   string
	Dispute     string
	Timeline    []TimelineItem
	// Deadline is the computed deadline, if any. The complaint states the applicable
	// timeline with the status of its source, never as settled fact when unverified.
	Deadline *deadline.Result
	Evidence []string
}

var wat = time.FixedZone("WAT", 3600)

const missing = "not provided"

func or(s string) string {
	if strings.TrimSpace(s) == "" {
		return missing
	}
	return s
}

var disputeLabel = map[string]string{
	deadline.FailedTransfer:   "a transfer that did not complete after my account was debited",
	deadline.DuplicateDebit:   "my account being debited more than once for the same payment",
	deadline.PosWebFailure:    "a card or web payment that failed after my account was debited",
	deadline.AtmOnUs:          "an ATM withdrawal that did not dispense cash but debited my account",
	deadline.AtmNotOnUs:       "an ATM withdrawal at another bank's machine that did not dispense cash but debited my account",
	deadline.WrongAccountOrAm: "a payment sent to the wrong account or for the wrong amount",
	deadline.UnmatchedClaim:   "a debit on my account that I cannot match to a transaction",
}

var action = map[string]string{
	deadline.FailedTransfer:   "Please confirm the outcome of this transfer and return the amount to my account if it was not completed.",
	deadline.DuplicateDebit:   "Please check whether I was debited twice and correct my account if so.",
	deadline.PosWebFailure:    "Please check this payment and return the amount to my account if it was not completed.",
	deadline.AtmOnUs:          "Please check the machine record and return the amount to my account if no cash was dispensed.",
	deadline.AtmNotOnUs:       "Please check this withdrawal with the other bank and return the amount to my account if no cash was dispensed.",
	deadline.WrongAccountOrAm: "Please treat this as my written request to reverse the payment.",
	deadline.UnmatchedClaim:   "Please look into this debit and tell me what it relates to.",
}

// Draft builds the complaint from facts. It is deterministic: the same facts give the
// same text.
func Draft(f Facts) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	label, ok := disputeLabel[f.Dispute]
	if !ok {
		label = "a transaction on my account"
	}
	w("Complaint")
	w("")
	w("Case reference: %s", orRef(f.CaseRef))
	w("Customer: %s", or(f.CustomerRef))
	w("Account ending: %s", or(f.AccountTail))
	if f.AmountMinor > 0 {
		w("Amount: %s", notify.Money(f.AmountMinor, or2(f.Currency, "NGN")))
	} else {
		w("Amount: %s", missing)
	}
	if f.OccurredAt.IsZero() {
		w("Date and time: %s", missing)
	} else {
		w("Date and time: %s", f.OccurredAt.In(wat).Format("2 Jan 2006 15:04")+" WAT")
	}
	w("Narration: %s", or(f.Narration))
	w("Reference: %s", or(f.Reference))
	w("Session: %s", or(f.SessionID))
	w("")
	w("What happened")
	w("I am writing about %s.", label)
	if len(f.Timeline) == 0 {
		w("I have no further timeline to add.")
	}
	for _, t := range f.Timeline {
		w("- %s: %s", t.At.In(wat).Format("2 Jan 2006 15:04")+" WAT", t.What)
	}
	if f.Dispute == deadline.WrongAccountOrAm {
		w("")
		w("This is a written reversal request. I understand that recovery depends on the funds still being available.")
	}
	w("")
	w("Timeline that applies")
	switch {
	case f.Deadline != nil && f.Deadline.Regulatory != nil:
		r := f.Deadline.Regulatory
		w("%s: %d %s, from %s. The status of this timeline is %s and it has not been verified by the bank unless it says confirmed.", r.Situation, r.Duration, unit(r.Unit), r.StartEvent, r.Status)
		if f.Deadline.Binding != nil {
			w("The bank's target for this case is %s.", f.Deadline.Binding.In(wat).Format("2 Jan 2006 15:04")+" WAT")
		}
	case f.Deadline != nil && f.Deadline.Binding != nil:
		w("No regulatory timeline has been confirmed for this situation. The bank's target for this case is %s.", f.Deadline.Binding.In(wat).Format("2 Jan 2006 15:04")+" WAT")
	default:
		w("No regulatory timeline has been confirmed for this situation.")
	}
	w("")
	w("What I ask")
	if a, ok := action[f.Dispute]; ok {
		w("%s", a)
	} else {
		w("Please look into this and tell me the outcome.")
	}
	w("")
	w("Evidence attached")
	if len(f.Evidence) == 0 {
		w("- %s", missing)
	}
	for _, e := range f.Evidence {
		w("- %s", e)
	}
	return strings.TrimRight(b.String(), "\n")
}

func orRef(s string) string {
	if s == "" {
		return "to be assigned"
	}
	return s
}

func or2(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func unit(u string) string {
	if u == "working_days" {
		return "working days"
	}
	return u
}

var (
	tokenRe = regexp.MustCompile(`[A-Za-z0-9]*[0-9][A-Za-z0-9.,:/_-]*`)
	urlRe   = regexp.MustCompile(`(?i)(https?://|www\.|\S+@\S+\.\S+)`)
)

// tokens returns every token that contains a digit, with trailing punctuation removed.
func tokens(text string) map[string]bool {
	out := map[string]bool{}
	for _, t := range tokenRe.FindAllString(text, -1) {
		out[strings.TrimRight(t, ".,:;-/_")] = true
	}
	return out
}

// ValidateRephrase checks text a model returned for the draft of f. Every number, date
// and reference in it must already be in the template draft, and it may not add a link
// or an address. It does not judge style.
func ValidateRephrase(text string, f Facts) error {
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return fmt.Errorf("%w: it is empty", ErrRephrase)
	case len(text) > MaxLength:
		return fmt.Errorf("%w: it is too long", ErrRephrase)
	case urlRe.MatchString(text):
		return fmt.Errorf("%w: it contains a link or an address", ErrRephrase)
	}
	allowed := tokens(Draft(f))
	for t := range tokens(text) {
		if !allowed[t] {
			return fmt.Errorf("%w: it contains %q, which is not in the facts", ErrRephrase, t)
		}
	}
	return nil
}

// Final chooses the text to use: the model's rephrasing if it passes validation, else
// the template draft.
func Final(f Facts, rephrased string) (text string, usedModel bool) {
	if rephrased != "" && ValidateRephrase(rephrased, f) == nil {
		return strings.TrimSpace(rephrased), true
	}
	return Draft(f), false
}
