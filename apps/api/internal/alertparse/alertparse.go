// Package alertparse turns a pasted debit alert into structured fields, with a
// confidence for each, and redacts it before anything is stored or logged.
//
// It is the backend side of the parser contract (A9). The real parser is owned by the
// AI developer. RuleParser here is the deterministic stand-in that the scope calls for
// first: regular expressions, no model, no network. A parser can be unavailable or
// unsure; the customer is then given the guided form and always confirms or edits the
// fields before anything is matched. A parsed field is a suggestion, never a fact.
//
// The alerts in tests are synthetic. Real bank alert formats vary and were not
// available, so the patterns here are examples, not a claim to read any bank's alerts.
package alertparse

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxAlertLength bounds what is parsed.
const MaxAlertLength = 2000

// ErrUnavailable means a parser could not be reached. The caller offers the guided
// form instead of failing the customer.
var ErrUnavailable = errors.New("alertparse: parser unavailable")

// ErrTooLong means the alert text is longer than MaxAlertLength.
var ErrTooLong = errors.New("alertparse: the alert text is too long")

// Field is one parsed value with how sure the parser is.
type Field struct {
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
}

// Parsed is the result of parsing one alert. Keys of Fields: amount_minor, currency,
// occurred_at (RFC 3339), account_tail, narration, reference, channel, direction.
type Parsed struct {
	Fields map[string]Field `json:"fields"`
	// Confidence is the lowest confidence among the fields that matching depends on
	// (amount, account tail, time). It is a guide for routing, not a verdict.
	Confidence float64 `json:"confidence"`
	// OtherBank is true when the text names another bank and none of ours.
	OtherBank bool `json:"other_bank"`
	// Flags are advisory observations such as mismatched amounts. They never block or
	// accuse; a person decides.
	Flags []string `json:"flags,omitempty"`
	// Parser names what produced this, for the audit trail.
	Parser string `json:"parser"`
}

// Parser is the contract the AI service's parser also meets.
type Parser interface {
	// Parse reads redacted alert text. It must never be given raw account numbers.
	Parse(ctx context.Context, redactedText string) (Parsed, error)
}

var (
	accountRun = regexp.MustCompile(`\d{10,}`)
	amountRe   = regexp.MustCompile(`(?i)(?:\bNGN|\bNAIRA|\bN|₦)\s?([0-9]{1,3}(?:,[0-9]{3})*(?:\.[0-9]{1,2})?|[0-9]+(?:\.[0-9]{1,2})?)`)
	tailRe     = regexp.MustCompile(`(?i)(?:\*{2,}|x{2,}|\.{3,}|#)\s?(\d{4})\b`)
	refRe      = regexp.MustCompile(`(?i)\b(?:ref(?:erence)?|session(?:\s?id)?|rrn|stan)\s*[:#=-]?\s*([A-Za-z0-9]{6,40})`)
	narrRe     = regexp.MustCompile(`(?i)\b(?:desc(?:ription)?|narration|narr|remark)\s*[:=-]\s*([^|;\n]{3,80})`)
	dateISO    = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})`)
	dateMon    = regexp.MustCompile(`(?i)(\d{1,2})[-/ ]([A-Za-z]{3})[a-z]*[-/ ](\d{4})[ ,]+(\d{1,2}):(\d{2})`)
	debitRe    = regexp.MustCompile(`(?i)\b(?:debit(?:ed)?|dr)\b`)
	creditRe   = regexp.MustCompile(`(?i)\b(?:credit(?:ed)?|cr)\b`)
)

// Redact masks every run of ten or more digits that stands alone, which is how account
// numbers, BVNs and phone numbers look, keeping only the last four. A long number that
// is part of a reference (written after "ref", "session", "rrn" or "stan") or inside an
// alphanumeric token such as NIP202610140912 is kept, because matching needs it and a
// reference is not an account. Short numbers such as amounts and times stay. Everything
// stored, logged or sent to another service is redacted first.
func Redact(text string) string {
	protected := map[int]bool{}
	for _, m := range refRe.FindAllStringSubmatchIndex(text, -1) {
		for i := m[2]; i < m[3]; i++ {
			protected[i] = true
		}
	}
	var b strings.Builder
	last := 0
	for _, loc := range accountRun.FindAllStringIndex(text, -1) {
		start, end := loc[0], loc[1]
		attached := (start > 0 && isAlnumByte(text[start-1])) || (end < len(text) && isAlnumByte(text[end]))
		if protected[start] || attached {
			continue
		}
		b.WriteString(text[last:start])
		b.WriteString(strings.Repeat("*", end-start-4) + text[end-4:end])
		last = end
	}
	b.WriteString(text[last:])
	return b.String()
}

func isAlnumByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// Tail returns the last four digits of the first run of ten or more digits, which a
// parser may use for the account tail before the text is redacted.
func Tail(text string) string {
	m := accountRun.FindString(text)
	if len(m) < 4 {
		return ""
	}
	return m[len(m)-4:]
}

var monthNum = map[string]time.Month{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}

var otherBanks = []string{"gtbank", "gtco", "zenith", "access bank", "uba", "first bank", "fcmb", "fidelity", "sterling", "stanbic", "union bank", "polaris", "keystone", "opay", "palmpay", "kuda", "moniepoint"}

// RuleParser is the deterministic parser.
type RuleParser struct {
	// OwnNames are names that identify this bank in an alert (for example its brand
	// and app names). They are configuration, not guessed.
	OwnNames []string
	// Zone is the zone alert times are written in, WAT for Nigerian banks.
	Zone *time.Location
	// Now is the clock used to notice a time in the future.
	Now func() time.Time
}

func (p *RuleParser) zone() *time.Location {
	if p.Zone != nil {
		return p.Zone
	}
	return time.FixedZone("WAT", 3600)
}

func (p *RuleParser) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// Parse implements Parser. It reads only the text it is given, so it can run on
// redacted text; when the account tail was masked by Redact, the last four digits are
// still there to read.
func (p *RuleParser) Parse(_ context.Context, text string) (Parsed, error) {
	if !utf8.ValidString(text) {
		return Parsed{}, errors.New("alertparse: the text is not valid UTF-8")
	}
	if len(text) > MaxAlertLength {
		return Parsed{}, ErrTooLong
	}
	out := Parsed{Fields: map[string]Field{}, Parser: "rule-based-v1"}
	set := func(k, v string, c float64) { out.Fields[k] = Field{Value: v, Confidence: c} }

	// Amount. More than one distinct amount lowers confidence and raises a flag, and
	// the first is offered, never the largest.
	seen := map[int64]bool{}
	var first int64
	for _, m := range amountRe.FindAllStringSubmatch(text, -1) {
		minor, ok := toMinor(m[1])
		if !ok || minor < 1 {
			continue
		}
		if len(seen) == 0 {
			first = minor
		}
		seen[minor] = true
	}
	switch len(seen) {
	case 0:
	case 1:
		set("amount_minor", strconv.FormatInt(first, 10), 0.9)
		set("currency", "NGN", 0.85)
	default:
		set("amount_minor", strconv.FormatInt(first, 10), 0.4)
		set("currency", "NGN", 0.7)
		out.Flags = append(out.Flags, "more than one amount appears in the alert")
	}

	if m := tailRe.FindStringSubmatch(text); m != nil {
		set("account_tail", m[1], 0.85)
	} else if run := accountRun.FindString(text); run != "" {
		set("account_tail", run[len(run)-4:], 0.7)
	}

	if t, ok := p.when(text); ok {
		set("occurred_at", t.UTC().Format(time.RFC3339), 0.85)
		if t.After(p.now().Add(5 * time.Minute)) {
			out.Flags = append(out.Flags, "the time in the alert is in the future")
		}
	}
	if m := refRe.FindStringSubmatch(text); m != nil {
		set("reference", m[1], 0.8)
	}
	if m := narrRe.FindStringSubmatch(text); m != nil {
		set("narration", strings.TrimSpace(m[1]), 0.7)
	}
	switch {
	case debitRe.MatchString(text) && !creditRe.MatchString(text):
		set("direction", "debit", 0.85)
	case creditRe.MatchString(text) && !debitRe.MatchString(text):
		set("direction", "credit", 0.85)
		out.Flags = append(out.Flags, "the alert looks like a credit, not a debit")
	}
	up := strings.ToUpper(text)
	for _, ch := range []string{"USSD", "ATM", "POS", "WEB", "MOBILE", "NIP"} {
		if strings.Contains(up, ch) {
			set("channel", ch, 0.6)
			break
		}
	}

	low := strings.ToLower(text)
	ours := false
	for _, n := range p.OwnNames {
		if n != "" && strings.Contains(low, strings.ToLower(n)) {
			ours = true
		}
	}
	if !ours {
		for _, b := range otherBanks {
			if strings.Contains(low, b) {
				out.OtherBank = true
				break
			}
		}
	}
	if len(out.Fields) < 2 {
		out.Flags = append(out.Flags, "too little could be read; the guided form is better")
	}

	out.Confidence = 1
	for _, k := range []string{"amount_minor", "account_tail", "occurred_at"} {
		if f, ok := out.Fields[k]; !ok {
			out.Confidence = 0
		} else if f.Confidence < out.Confidence {
			out.Confidence = f.Confidence
		}
	}
	return out, nil
}

func (p *RuleParser) when(text string) (time.Time, bool) {
	z := p.zone()
	if m := dateISO.FindStringSubmatch(text); m != nil {
		t, err := time.ParseInLocation("2006-01-02 15:04", m[1]+"-"+m[2]+"-"+m[3]+" "+m[4]+":"+m[5], z)
		return t, err == nil
	}
	if m := dateMon.FindStringSubmatch(text); m != nil {
		mon, ok := monthNum[strings.ToLower(m[2])]
		if !ok {
			return time.Time{}, false
		}
		day, _ := strconv.Atoi(m[1])
		year, _ := strconv.Atoi(m[3])
		h, _ := strconv.Atoi(m[4])
		mi, _ := strconv.Atoi(m[5])
		t := time.Date(year, mon, day, h, mi, 0, 0, z)
		if t.Day() != day { // 31 Feb and the like roll over: not a date
			return time.Time{}, false
		}
		return t, true
	}
	return time.Time{}, false
}

// toMinor converts "25,000.50" to minor units without rounding. More than two
// decimal places cannot occur because the pattern allows at most two.
func toMinor(s string) (int64, bool) {
	s = strings.ReplaceAll(s, ",", "")
	whole, frac, _ := strings.Cut(s, ".")
	for len(frac) < 2 {
		frac += "0"
	}
	n, err := strconv.ParseInt(whole+frac, 10, 64)
	if err != nil || n > 9_000_000_000_000_000 {
		return 0, false
	}
	return n, true
}
