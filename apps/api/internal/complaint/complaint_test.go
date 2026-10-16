package complaint

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/deadline"
)

var at = time.Date(2026, 10, 14, 9, 12, 0, 0, wat)

func facts() Facts {
	due := time.Date(2026, 10, 15, 9, 12, 0, 0, wat)
	return Facts{CustomerRef: "cust_0123456789abcdef01234567", AccountTail: "6789", AmountMinor: 2500000, Currency: "NGN", OccurredAt: at,
		Narration: "NIP TRANSFER", Reference: "NIP202610140912", Dispute: deadline.FailedTransfer,
		Timeline: []TimelineItem{{at, "My account was debited"}, {at.Add(30 * time.Second), "The transfer did not complete"}},
		Deadline: &deadline.Result{Regulatory: &deadline.Regulatory{Situation: "Failed instant transfer", Duration: 24, Unit: "hours", StartEvent: "dispute_reported", Status: "reported"}, Binding: &due},
		Evidence: []string{"Debit record NIP202610140912"}}
}

func TestTheDraftStatesFactsInNeutralWordsAndNothingElse(t *testing.T) {
	d := Draft(facts())
	for _, want := range []string{"Account ending: 6789", "Amount: NGN 25,000.00", "14 Oct 2026 09:12 WAT", "Reference: NIP202610140912", "a transfer that did not complete",
		"My account was debited", "24 hours", "reported", "has not been verified", "15 Oct 2026 09:12 WAT", "Debit record NIP202610140912", "Case reference: to be assigned"} {
		if !strings.Contains(d, want) {
			t.Errorf("the draft lacks %q:\n%s", want, d)
		}
	}
	for _, never := range []string{"fraud", "negligen", "sue", "lawyer", "demand", "refunded", "has been reversed", "guarantee"} {
		if strings.Contains(strings.ToLower(d), never) {
			t.Errorf("the draft contains %q", never)
		}
	}
	if Draft(facts()) != d {
		t.Fatal("the draft is not deterministic")
	}
}

func TestMissingFactsAreSaidToBeMissingNotInvented(t *testing.T) {
	d := Draft(Facts{Dispute: deadline.UnmatchedClaim})
	if strings.Count(d, "not provided") < 6 || !strings.Contains(d, "No regulatory timeline has been confirmed") || !strings.Contains(d, "Amount: not provided") {
		t.Fatalf("%s", d)
	}
}

func TestAWrongAccountComplaintIsAWrittenRequestThatSaysRecoveryDependsOnFunds(t *testing.T) {
	f := facts()
	f.Dispute = deadline.WrongAccountOrAm
	d := Draft(f)
	if !strings.Contains(d, "written reversal request") || !strings.Contains(d, "depends on the funds still being available") {
		t.Fatalf("%s", d)
	}
}

func TestARephrasingMayNotAddANumberDateReferenceLinkOrAddress(t *testing.T) {
	f := facts()
	ok := "I am writing about a transfer of NGN 25,000.00 on 14 Oct 2026 09:12 WAT that did not complete although my account was debited. Reference NIP202610140912, account ending 6789. Please return the amount."
	if err := ValidateRephrase(ok, f); err != nil {
		t.Fatalf("a faithful rephrasing was refused: %v", err)
	}
	for name, bad := range map[string]string{
		"a different amount":    "A transfer of NGN 250,000.00 on 14 Oct 2026 09:12 WAT. Reference NIP202610140912",
		"a different date":      "A transfer of NGN 25,000.00 on 13 Oct 2026 09:12 WAT. Reference NIP202610140912",
		"an invented reference": "A transfer. Reference NIP202610149999",
		"a new number":          "A transfer of NGN 25,000.00, the 3rd time this has happened. Reference NIP202610140912",
		"a link":                "A transfer of NGN 25,000.00. See https://example.com. Reference NIP202610140912",
		"an address":            "A transfer of NGN 25,000.00. Write to me at someone@example.com",
		"empty":                 "   ",
		"too long":              strings.Repeat("a ", MaxLength),
	} {
		if err := ValidateRephrase(bad, f); !errors.Is(err, ErrRephrase) {
			t.Errorf("%s was accepted: %v", name, err)
		}
	}
}

func TestFinalFallsBackToTheTemplateWhenThereIsNoModelOrItsTextIsRefused(t *testing.T) {
	f := facts()
	if text, used := Final(f, ""); used || text != Draft(f) {
		t.Fatal("no model output must give the template")
	}
	if text, used := Final(f, "Invented amount NGN 99,999.00"); used || text != Draft(f) {
		t.Fatal("a refused rephrasing must give the template")
	}
	good := "A transfer of NGN 25,000.00 on 14 Oct 2026 09:12 WAT did not complete. Reference NIP202610140912."
	if text, used := Final(f, good); !used || text != good {
		t.Fatal("a faithful rephrasing should be used")
	}
}
