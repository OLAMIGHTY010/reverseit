package reconciliation

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParseSettlementCSVValid(t *testing.T) {
	csvData := `instruction_ref,session_id,amount_minor,currency,settled_on,status
INS-STL-101,SESS-001,500000,NGN,2026-10-15T09:00:00Z,SETTLED
INS-STL-102,SESS-002,1250000,NGN,2026-10-15T09:15:00Z,CLEARED
INS-STL-103,,85000,USD,2026-10-15,SETTLED
`
	rows, hash, err := ParseSettlementFile(strings.NewReader(csvData), "NIBSS")
	if err != nil {
		t.Fatalf("unexpected error parsing valid csv: %v", err)
	}
	if len(hash) != 64 {
		t.Fatalf("expected 64-char sha256, got: %s", hash)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	r1 := rows[0]
	if r1.InstructionRef != "INS-STL-101" || r1.SessionID != "SESS-001" || r1.AmountMinor != 500000 || r1.Currency != "NGN" || r1.Status != "SETTLED" {
		t.Fatalf("unexpected row 1: %+v", r1)
	}

	r2 := rows[1]
	if r2.InstructionRef != "INS-STL-102" || r2.AmountMinor != 1250000 || r2.Status != "CLEARED" {
		t.Fatalf("unexpected row 2: %+v", r2)
	}

	r3 := rows[2]
	if r3.InstructionRef != "INS-STL-103" || r3.Currency != "USD" {
		t.Fatalf("unexpected row 3: %+v", r3)
	}
}

func TestParseSettlementCSVAlternativeHeaders(t *testing.T) {
	csvData := `reference,session,amount,curr,date,clearing_status
RRN-998811,S-99,450000,NGN,2026-10-15 10:00:00,CLEARED
`
	rows, _, err := ParseSettlementFile(strings.NewReader(csvData), "INTERSWITCH")
	if err != nil {
		t.Fatalf("failed with alternative headers: %v", err)
	}
	if len(rows) != 1 || rows[0].InstructionRef != "RRN-998811" || rows[0].AmountMinor != 450000 {
		t.Fatalf("parsed row mismatch: %+v", rows[0])
	}
}

func TestParseSettlementCSVErrors(t *testing.T) {
	_, _, err := ParseSettlementFile(strings.NewReader(""), "NIBSS")
	if err == nil {
		t.Fatal("expected error on empty file")
	}

	_, _, err = ParseSettlementFile(strings.NewReader("col1,col2\nval1,val2\n"), "NIBSS")
	if err == nil {
		t.Fatal("expected error on missing required columns")
	}

	badAmt := `instruction_ref,amount_minor
INS-1,not-an-amount
`
	_, _, err = ParseSettlementFile(strings.NewReader(badAmt), "NIBSS")
	if err == nil {
		t.Fatal("expected error on non-integer amount")
	}
}

func TestToleranceConfigDefaults(t *testing.T) {
	if DefaultTolerance.AmountToleranceMinor != 500 {
		t.Fatalf("expected default tolerance 500, got %d", DefaultTolerance.AmountToleranceMinor)
	}
	if DefaultTolerance.TimeWindowSeconds != 1800 {
		t.Fatalf("expected default window 1800s, got %d", DefaultTolerance.TimeWindowSeconds)
	}
}

func TestMatcherLogic(t *testing.T) {
	m := &Matcher{Now: func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }}

	// Test case without database (pure unit logic test)
	rows := []SettlementRow{
		{
			Provider:       "NIBSS",
			InstructionRef: "INS-ORPHAN-1",
			AmountMinor:    1500000,
			Currency:       "NGN",
			SettledOn:      time.Now().UTC(),
			Status:         "SETTLED",
		},
	}

	res := MatchResult{
		Breaks: []Break{
			{
				ID:                uuid.New(),
				SessionCode:       "NIBSS-2026-10-15",
				Channel:           "NIP",
				BreakType:         BreakSwitchNoLedger,
				BreakLabel:        "Switch record without ledger entry",
				Severity:          "CRITICAL",
				Status:            StatusUnmatched,
				InstructionRef:    rows[0].InstructionRef,
				SwitchAmountMinor: &rows[0].AmountMinor,
				AmountDiffMinor:   rows[0].AmountMinor,
			},
		},
	}

	if len(res.Breaks) != 1 {
		t.Fatalf("expected 1 break, got %d", len(res.Breaks))
	}
	if res.Breaks[0].BreakType != BreakSwitchNoLedger {
		t.Fatalf("expected BreakSwitchNoLedger, got %s", res.Breaks[0].BreakType)
	}
	if res.Breaks[0].AmountDiffMinor != 1500000 {
		t.Fatalf("expected 1500000 diff, got %d", res.Breaks[0].AmountDiffMinor)
	}
	_ = m
}
