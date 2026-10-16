package feed

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Record is one parsed row: the envelope, the external references it carries for
// correlation, and where in the file it came from.
type Record struct {
	Line     int
	Envelope txn.Envelope
	Refs     map[string]string
}

// RowError is a row that could not be turned into an envelope. The row is reported
// and the rest of the file is still read.
type RowError struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

// MaxRows bounds one file. A larger file is refused whole, so a mistaken upload
// cannot exhaust memory.
const MaxRows = 100_000

// get returns the raw text of a canonical field for the current row.
type get func(field string) (string, bool)

// build turns one row into a Record using the mapping's rules.
func (m *Mapping) build(line int, g get) (Record, error) {
	val := func(f string) string {
		if v, ok := g(f); ok {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
		return m.Defaults[f]
	}
	code := val(FEventType)
	canon, ok := m.EventCodes[code]
	if !ok {
		return Record{}, fmt.Errorf("event code %q is not in the mapping", code)
	}
	ts, err := m.parseTime(val(FSourceTS))
	if err != nil {
		return Record{}, err
	}
	amount, err := m.parseAmount(val(FAmount))
	if err != nil {
		return Record{}, err
	}
	rec := Record{Line: line, Refs: map[string]string{}}
	rec.Envelope = txn.Envelope{
		SourceSystem: m.SourceSystem, SourceEventRef: val(FSourceEventRef), EventType: canon, SourceTS: ts,
		Txn: txn.Core{InstructionRef: val(FInstructionRef), Channel: val(FChannel), Rail: val(FRail), AmountMinor: txn.Amount(amount),
			Currency: val(FCurrency), DebitAccountToken: val(FDebitToken), BeneficiaryBank: val(FBeneficiaryBank),
			BeneficiaryToken: val(FBeneficiaryTok), SessionID: val(FSessionID)},
	}
	for f, key := range map[string]string{FRRN: "RRN", FSTAN: "STAN", FLedgerID: "LEDGER_ID", FGatewayRef: "GATEWAY_REF"} {
		if v := val(f); v != "" {
			rec.Refs[key] = v
		}
	}
	if len(rec.Refs) > 0 {
		rec.Envelope.Refs = rec.Refs
	}
	return rec, nil
}

func (m *Mapping) parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("source_ts is empty")
	}
	t, err := time.ParseInLocation(m.TimeLayout, s, m.loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("source_ts %q does not match the layout %q", s, m.TimeLayout)
	}
	return t.UTC(), nil
}

// parseAmount returns minor units. A major amount must have at most two decimal
// places, so nothing is rounded.
func (m *Mapping) parseAmount(s string) (int64, error) {
	if m.AmountUnit == UnitMinor {
		// Fixed-width files pad numbers with zeros.
		return txn.ParseAmount(strings.TrimLeft(s, "0"))
	}
	whole, frac, _ := strings.Cut(s, ".")
	if len(frac) > 2 {
		return 0, fmt.Errorf("amount %q has more than two decimal places", s)
	}
	for len(frac) < 2 {
		frac += "0"
	}
	if whole == "" || strings.ContainsAny(whole+frac, "+-, ") {
		return 0, fmt.Errorf("amount %q is not a plain decimal", s)
	}
	n, err := strconv.ParseInt(whole+frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %q is not a plain decimal", s)
	}
	return txn.ParseAmount(strconv.FormatInt(n, 10))
}

// ErrTooManyRows is returned when a file exceeds MaxRows.
var ErrTooManyRows = fmt.Errorf("the file has more than %d rows", MaxRows)
