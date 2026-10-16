// Package feed turns files that source systems drop (CSV, fixed-width, XML) into the
// canonical event envelope. A versioned mapping says which column, offset or element
// holds each canonical field, how the source's event codes are named, and how its
// times and amounts are written. Parsers only translate. Whether an event is valid
// is decided afterwards by the same checks as an event delivered by API.
//
// The layouts here are configuration examples for synthetic data. They are not any
// real institution's or scheme's file format.
package feed

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Formats.
const (
	CSV        = "csv"
	FixedWidth = "fixedwidth"
	XML        = "xml"
)

// Canonical fields a mapping may fill.
const (
	FSourceEventRef  = "source_event_ref"
	FEventType       = "event_type"
	FSourceTS        = "source_ts"
	FInstructionRef  = "instruction_ref"
	FChannel         = "channel"
	FRail            = "rail"
	FAmount          = "amount"
	FCurrency        = "currency"
	FDebitToken      = "debit_account_token"
	FBeneficiaryBank = "beneficiary_bank"
	FBeneficiaryTok  = "beneficiary_token"
	FSessionID       = "session_id"
	// External references used to correlate evidence from other sources.
	FRRN        = "rrn"
	FSTAN       = "stan"
	FLedgerID   = "ledger_id"
	FGatewayRef = "gateway_ref"
)

var knownFields = map[string]bool{
	FSourceEventRef: true, FEventType: true, FSourceTS: true, FInstructionRef: true, FChannel: true, FRail: true,
	FAmount: true, FCurrency: true, FDebitToken: true, FBeneficiaryBank: true, FBeneficiaryTok: true, FSessionID: true,
	FRRN: true, FSTAN: true, FLedgerID: true, FGatewayRef: true,
}

var required = []string{FSourceEventRef, FEventType, FSourceTS, FInstructionRef, FAmount, FDebitToken}

// Amount units.
const (
	UnitMinor = "minor" // whole minor units, such as 1850000
	UnitMajor = "major" // a decimal with at most two places, such as 18500.00
)

// Locator says where one canonical field is found. Which attributes apply depends on
// the mapping's format: Column for CSV (a header name), Start and Len for fixed
// width (zero-based, in bytes), Element for XML (a child element or attribute name).
type Locator struct {
	Column  string `json:"column,omitempty"`
	Start   int    `json:"start,omitempty"`
	Len     int    `json:"len,omitempty"`
	Element string `json:"element,omitempty"`
}

// Mapping is one version of one source's layout.
type Mapping struct {
	Name         string             `json:"name"`
	Version      int                `json:"version"`
	Format       string             `json:"format"`
	SourceSystem string             `json:"source_system"`
	Delimiter    string             `json:"delimiter,omitempty"`
	Fields       map[string]Locator `json:"fields"`
	// Defaults fill canonical fields the file does not carry, such as a constant rail.
	Defaults map[string]string `json:"defaults,omitempty"`
	// EventCodes names the source's own codes. A code that is not listed is rejected,
	// never guessed.
	EventCodes map[string]string `json:"event_codes"`
	TimeLayout string            `json:"time_layout"`
	TimeZone   string            `json:"time_zone"`
	AmountUnit string            `json:"amount_unit"`
	// RecordElement is the repeating XML element.
	RecordElement string `json:"record_element,omitempty"`
	// RecordLen is the fixed width of one line; a line of another length is rejected.
	RecordLen int `json:"record_len,omitempty"`
	// SkipHeader drops the first line of a fixed-width file.
	SkipHeader bool `json:"skip_header,omitempty"`

	loc *time.Location
}

// ErrMapping wraps every mapping problem.
var ErrMapping = errors.New("feed: invalid mapping")

// Validate checks the mapping and prepares it for use.
func (m *Mapping) Validate() error {
	bad := func(f string, a ...any) error { return fmt.Errorf("%w: "+f, append([]any{ErrMapping}, a...)...) }
	if strings.TrimSpace(m.Name) == "" || m.Version < 1 {
		return bad("a name and a version of 1 or more are required")
	}
	switch m.Format {
	case CSV, FixedWidth, XML:
	default:
		return bad("format must be csv, fixedwidth or xml")
	}
	if m.SourceSystem == "" {
		return bad("source_system is required")
	}
	for f := range m.Fields {
		if !knownFields[f] {
			return bad("%q is not a canonical field", f)
		}
	}
	for f := range m.Defaults {
		if !knownFields[f] {
			return bad("default for %q, which is not a canonical field", f)
		}
	}
	for _, f := range required {
		if _, ok := m.Fields[f]; !ok {
			if _, d := m.Defaults[f]; !d {
				return bad("field %q is required", f)
			}
		}
	}
	for f, l := range m.Fields {
		switch m.Format {
		case CSV:
			if l.Column == "" {
				return bad("field %q needs a column", f)
			}
		case FixedWidth:
			if l.Start < 0 || l.Len < 1 {
				return bad("field %q needs a start of 0 or more and a length of 1 or more", f)
			}
			if m.RecordLen > 0 && l.Start+l.Len > m.RecordLen {
				return bad("field %q runs past the record length", f)
			}
		case XML:
			if l.Element == "" {
				return bad("field %q needs an element", f)
			}
		}
	}
	if m.Format == CSV && len([]rune(m.Delimiter)) > 1 {
		return bad("delimiter must be one character")
	}
	if m.Format == FixedWidth && m.RecordLen < 1 {
		return bad("record_len is required for fixed width")
	}
	if m.Format == XML && m.RecordElement == "" {
		return bad("record_element is required for xml")
	}
	if len(m.EventCodes) == 0 {
		return bad("event_codes must map at least one source code")
	}
	codes := make([]string, 0, len(m.EventCodes))
	for src, canon := range m.EventCodes {
		if src == "" {
			return bad("an empty source event code")
		}
		codes = append(codes, canon)
	}
	sort.Strings(codes)
	for _, c := range codes {
		if !txn.KnownEventType(c) {
			return bad("event code maps to %q, which is not a canonical event type", c)
		}
	}
	if m.TimeLayout == "" {
		return bad("time_layout is required")
	}
	if m.TimeZone == "" {
		return bad("time_zone is required: a file time without a zone is ambiguous")
	}
	loc, err := time.LoadLocation(m.TimeZone)
	if err != nil {
		return bad("time_zone %q is unknown", m.TimeZone)
	}
	m.loc = loc
	if m.AmountUnit != UnitMinor && m.AmountUnit != UnitMajor {
		return bad("amount_unit must be minor or major")
	}
	return nil
}
