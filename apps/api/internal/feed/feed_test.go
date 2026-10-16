package feed

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func switchA() *Mapping {
	return &Mapping{
		Name: "switch-a-csv", Version: 1, Format: CSV, SourceSystem: "SWITCH_A",
		Fields: map[string]Locator{
			FSourceEventRef: {Column: "event_id"}, FEventType: {Column: "code"}, FSourceTS: {Column: "ts"}, FInstructionRef: {Column: "instr"},
			FChannel: {Column: "channel"}, FRail: {Column: "rail"}, FAmount: {Column: "amt"}, FCurrency: {Column: "ccy"},
			FDebitToken: {Column: "debit_acct"}, FBeneficiaryBank: {Column: "bene_bank"}, FBeneficiaryTok: {Column: "bene_acct"},
			FSessionID: {Column: "session"}, FRRN: {Column: "rrn"}, FSTAN: {Column: "stan"},
		},
		EventCodes: map[string]string{"DR": "DEBIT_POSTED", "TO": "TRANSFER_RESPONSE_TIMEOUT", "OK": "CREDIT_CONFIRMED"},
		TimeLayout: "2006-01-02 15:04:05", TimeZone: "Africa/Lagos", AmountUnit: UnitMajor,
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMappingValidation(t *testing.T) {
	if err := switchA().Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(m *Mapping){
		"no name":             func(m *Mapping) { m.Name = "" },
		"version zero":        func(m *Mapping) { m.Version = 0 },
		"unknown format":      func(m *Mapping) { m.Format = "json" },
		"unknown field":       func(m *Mapping) { m.Fields["balance"] = Locator{Column: "b"} },
		"missing required":    func(m *Mapping) { delete(m.Fields, FDebitToken) },
		"column missing":      func(m *Mapping) { m.Fields[FRail] = Locator{} },
		"no event codes":      func(m *Mapping) { m.EventCodes = nil },
		"non canonical code":  func(m *Mapping) { m.EventCodes["ZZ"] = "SOMETHING_ELSE" },
		"no time zone":        func(m *Mapping) { m.TimeZone = "" },
		"bad time zone":       func(m *Mapping) { m.TimeZone = "Mars/Base" },
		"bad amount unit":     func(m *Mapping) { m.AmountUnit = "cents" },
		"two char delimiter":  func(m *Mapping) { m.Delimiter = "||" },
		"default not a field": func(m *Mapping) { m.Defaults = map[string]string{"colour": "red"} },
		"no source system":    func(m *Mapping) { m.SourceSystem = "" },
		"no layout":           func(m *Mapping) { m.TimeLayout = "" },
	}
	for name, mut := range cases {
		m := switchA()
		mut(m)
		if err := m.Validate(); !errors.Is(err, ErrMapping) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCSVTranslatesRowsAndReportsTheOnesItCannot(t *testing.T) {
	recs, errs, err := switchA().Parse(fixture(t, "switch_a.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 good rows, got %d", len(recs))
	}
	e := recs[0].Envelope
	if e.SourceSystem != "SWITCH_A" || e.EventType != "DEBIT_POSTED" || e.Txn.AmountMinor != 1850000 || e.Txn.InstructionRef != "INS-1001" {
		t.Fatalf("%+v", e)
	}
	// 09:00 in Lagos (UTC+1) is 08:00 UTC.
	if !e.SourceTS.Equal(time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC)) || e.SourceTS.Location() != time.UTC {
		t.Fatalf("the file time was not converted using the mapping's zone: %s", e.SourceTS)
	}
	if recs[0].Refs["RRN"] != "RRN000000001" || recs[0].Refs["STAN"] != "000101" || recs[0].Line != 2 {
		t.Fatalf("%+v", recs[0])
	}
	want := map[int]string{4: "event code", 5: "does not match the layout", 6: "more than two decimal places", 7: "columns"}
	if len(errs) != len(want) {
		t.Fatalf("%+v", errs)
	}
	for _, re := range errs {
		if !strings.Contains(re.Reason, want[re.Line]) {
			t.Errorf("line %d: %q", re.Line, re.Reason)
		}
	}
}

func TestCSVRefusesAFileWhoseHeaderLacksAMappedColumn(t *testing.T) {
	if _, _, err := switchA().Parse([]byte("event_id,code\nEV-1,DR\n")); err == nil || !strings.Contains(err.Error(), "no column") {
		t.Fatalf("%v", err)
	}
	if _, _, err := switchA().Parse(nil); err == nil {
		t.Fatal("an empty file was accepted")
	}
}

func TestAmountsAreExactNeverRounded(t *testing.T) {
	m := switchA()
	for in, want := range map[string]int64{"18500.00": 1850000, "18500.5": 1850050, "18500": 1850000, "0.01": 1} {
		if got, err := m.parseAmount(in); err != nil || got != want {
			t.Errorf("%s: %d %v", in, got, err)
		}
	}
	for _, in := range []string{"", "1.234", "-5.00", "1,000.00", "abc", "0.00", ".50", "1e3"} {
		if got, err := m.parseAmount(in); err == nil {
			t.Errorf("%q accepted as %d", in, got)
		}
	}
	m.AmountUnit = UnitMinor
	if got, err := m.parseAmount("1850000"); err != nil || got != 1850000 {
		t.Fatalf("%d %v", got, err)
	}
	if _, err := m.parseAmount("18500.00"); err == nil {
		t.Fatal("a decimal was accepted as minor units")
	}
}

func TestDefaultsFillWhatTheFileDoesNotCarry(t *testing.T) {
	m := switchA()
	delete(m.Fields, FRail)
	delete(m.Fields, FChannel)
	m.Defaults = map[string]string{FRail: "NIP", FChannel: "API"}
	recs, _, err := m.Parse(fixture(t, "switch_a.csv"))
	if err != nil || recs[0].Envelope.Txn.Rail != "NIP" || recs[0].Envelope.Txn.Channel != "API" {
		t.Fatalf("%v %+v", err, recs)
	}
}

func TestCSVWithByteOrderMarkAndOtherDelimiter(t *testing.T) {
	m := switchA()
	m.Delimiter = "|"
	data := "\xef\xbb\xbf" + strings.ReplaceAll(strings.SplitN(string(fixture(t, "switch_a.csv")), "\n", 3)[0]+"\n"+strings.SplitN(string(fixture(t, "switch_a.csv")), "\n", 3)[1]+"\n", ",", "|")
	recs, errs, err := m.Parse([]byte(data))
	if err != nil || len(recs) != 1 || len(errs) != 0 {
		t.Fatalf("%v %v %v", err, recs, errs)
	}
}

func switchB() *Mapping {
	return &Mapping{
		Name: "switch-b-fixed", Version: 1, Format: FixedWidth, SourceSystem: "SWITCH_B", RecordLen: 106, SkipHeader: true,
		Fields: map[string]Locator{
			FSourceEventRef: {Start: 0, Len: 10}, FEventType: {Start: 10, Len: 2}, FSourceTS: {Start: 12, Len: 19}, FInstructionRef: {Start: 31, Len: 10},
			FChannel: {Start: 41, Len: 8}, FRail: {Start: 49, Len: 5}, FAmount: {Start: 54, Len: 12}, FCurrency: {Start: 66, Len: 3},
			FDebitToken: {Start: 69, Len: 37},
		},
		EventCodes: map[string]string{"DR": "DEBIT_POSTED", "OK": "CREDIT_CONFIRMED"},
		TimeLayout: "2006-01-02 15:04:05", TimeZone: "UTC", AmountUnit: UnitMinor,
	}
}

func TestFixedWidthReadsByOffsetAndRejectsShiftedLines(t *testing.T) {
	m := switchB()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	recs, errs, err := m.Parse(fixture(t, "switch_b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || len(errs) != 2 {
		t.Fatalf("%d records, errors %+v", len(recs), errs)
	}
	e := recs[1].Envelope
	if e.SourceEventRef != "EV-2002" || e.EventType != "CREDIT_CONFIRMED" || e.Txn.AmountMinor != 1850000 || e.Txn.Channel != "MOBILE" || e.Txn.DebitAccountToken != "acct_0123456789abcdef0123456789abcdef" {
		t.Fatalf("%+v", e)
	}
	if errs[0].Line != 4 || errs[1].Line != 5 || !strings.Contains(errs[1].Reason, "bytes") {
		t.Fatalf("%+v", errs)
	}
}

func TestFixedWidthMappingMustFitItsRecord(t *testing.T) {
	m := switchB()
	m.RecordLen = 100
	if err := m.Validate(); !errors.Is(err, ErrMapping) {
		t.Fatalf("%v", err)
	}
	m = switchB()
	m.RecordLen = 0
	if err := m.Validate(); !errors.Is(err, ErrMapping) {
		t.Fatalf("%v", err)
	}
	if _, _, err := switchB().Parse([]byte("  \n")); err == nil {
		t.Fatal("an empty file was accepted")
	}
}

func gateway() *Mapping {
	return &Mapping{
		Name: "gateway-xml", Version: 1, Format: XML, SourceSystem: "GATEWAY_X", RecordElement: "Event",
		Fields: map[string]Locator{
			FSourceEventRef: {Element: "id"}, FEventType: {Element: "Code"}, FSourceTS: {Element: "Time"}, FInstructionRef: {Element: "Instr"},
			FChannel: {Element: "Channel"}, FRail: {Element: "Rail"}, FAmount: {Element: "Amount"}, FCurrency: {Element: "Ccy"},
			FDebitToken: {Element: "Debit"}, FGatewayRef: {Element: "GwRef"},
		},
		EventCodes: map[string]string{"DR": "DEBIT_POSTED", "OK": "CREDIT_CONFIRMED"},
		TimeLayout: time.RFC3339, TimeZone: "UTC", AmountUnit: UnitMajor,
	}
}

func TestXMLReadsAttributesAndChildrenAndReportsBadRecords(t *testing.T) {
	recs, errs, err := gateway().Parse(fixture(t, "gateway.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || len(errs) != 1 || !strings.Contains(errs[0].Reason, "record 3") {
		t.Fatalf("%d %+v", len(recs), errs)
	}
	e := recs[0].Envelope
	if e.SourceEventRef != "EV-3001" || e.EventType != "DEBIT_POSTED" || e.Txn.AmountMinor != 1850000 || recs[0].Refs["GATEWAY_REF"] != "GW-3001" {
		t.Fatalf("%+v %+v", e, recs[0].Refs)
	}
}

func TestXMLRefusesDTDsMalformedAndEmptyDocuments(t *testing.T) {
	for name, doc := range map[string]string{
		"entity declaration": `<?xml version="1.0"?><!DOCTYPE Events [<!ENTITY x "boom">]><Events><Event id="EV-1"><Code>DR</Code></Event></Events>`,
		"not well formed":    `<Events><Event id="EV-1"><Code>DR</Event></Events>`,
		"no records":         `<Events></Events>`,
		"empty":              ``,
	} {
		if recs, _, err := gateway().Parse([]byte(doc)); err == nil || len(recs) != 0 {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
}
