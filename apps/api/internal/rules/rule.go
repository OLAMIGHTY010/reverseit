// Package rules holds declarative detection rules: data, not code, so a rule can be
// reviewed, tested against labelled cases, run in shadow, approved by a second person,
// activated, and rolled back.
//
// A rule says "when a transaction's facts satisfy all of these conditions, open a case
// of this code". It cannot do anything else. In particular it cannot reverse money, and
// validation refuses a rule that would call an unknown outcome a confirmed failure.
package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Limits on a rule's size, so a rule stays reviewable.
const (
	MaxConditions = 12
	MaxListItems  = 20
	MaxFresh      = 5
)

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("rules: invalid rule")

func bad(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, a...)...)
}

// Field kinds.
type kind int

const (
	text kind = iota
	number
)

// Facts a rule may read. Anything not listed here cannot be tested by a rule.
var fields = map[string]kind{
	"lifecycle_state":           text,
	"credit_status":             text,
	"debit_status":              text,
	"rail":                      text,
	"channel":                   text,
	"currency":                  text,
	"amount_minor":              number,
	"event_count":               number,
	"seconds_since_last_event":  number,
	"seconds_since_first_debit": number,
}

// Allowed values for the fields that have a fixed set.
var enums = map[string]map[string]bool{
	"lifecycle_state": set("INITIATED", "DEBITED", "PENDING", "CONFIRMED_SUCCESS", "CONFIRMED_FAILURE", "UNKNOWN", "CONFLICTING", "REVERSED"),
	"credit_status":   set("NONE", "PENDING", "CONFIRMED", "FAILED", "UNKNOWN", "CONFLICTING"),
	"debit_status":    set("NONE", "POSTED"),
}

func set(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range v {
		m[s] = true
	}
	return m
}

// Codes a declarative rule may open. Duplicate detection compares transactions with
// each other and stays in code. Reversal, settlement and customer codes do not come
// from detection rules.
var (
	failureFamily = set("FAIL001", "ATM001", "FEE001", "VND001", "MIS001", "INC001")
	allowedCodes  = set("FAIL001", "ATM001", "FEE001", "VND001", "MIS001", "INC001", "UNK001")
)

var (
	ruleIDRe = regexp.MustCompile(`^[A-Z][A-Z0-9-]{2,39}$`)
	rootRe   = regexp.MustCompile(`^[a-z][a-z0-9_]{2,59}$`)
	valueRe  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,40}$`)
	sourceRe = regexp.MustCompile(`^[A-Z0-9_-]{2,40}$`)
)

// Condition is one test of one fact.
type Condition struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

// Rule is one version of one detection rule.
type Rule struct {
	RuleID      string      `json:"rule_id"`
	Version     int         `json:"version"`
	Code        string      `json:"code"`
	Risk        string      `json:"risk"`
	RootCause   string      `json:"root_cause"`
	Explanation string      `json:"explanation"`
	All         []Condition `json:"all"`
	// RequiresFresh names sources that must be fresh for the rule to run. If any is
	// stale, down or has no expectation set, the rule is held back and records why,
	// because absence of an event from a silent source is not evidence.
	RequiresFresh []string `json:"requires_fresh,omitempty"`
}

// Parse reads and validates a rule from JSON, refusing unknown fields.
func Parse(b []byte) (Rule, error) {
	var r Rule
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Rule{}, bad("%v", err)
	}
	if err := r.Validate(); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// Validate checks everything that can be checked without running the rule.
func (r *Rule) Validate() error {
	switch {
	case !ruleIDRe.MatchString(r.RuleID):
		return bad("rule_id must be 3 to 40 capitals, digits or dashes, starting with a capital")
	case r.Version < 1:
		return bad("version must be 1 or more")
	case !allowedCodes[r.Code]:
		return bad("code %q cannot be opened by a rule; allowed: %s", r.Code, strings.Join(sortedKeys(allowedCodes), ", "))
	case r.Risk != "" && r.Risk != "LOW" && r.Risk != "MEDIUM" && r.Risk != "HIGH":
		return bad("risk must be LOW, MEDIUM, HIGH or empty")
	case !rootRe.MatchString(r.RootCause):
		return bad("root_cause must be lower case words joined by underscores")
	case len(r.Explanation) < 10 || len(r.Explanation) > 300 || strings.ContainsAny(r.Explanation, "\r\n"):
		return bad("explanation must be 10 to 300 characters on one line")
	case len(r.All) == 0:
		return bad("a rule needs at least one condition; a rule that matches everything is refused")
	case len(r.All) > MaxConditions:
		return bad("at most %d conditions", MaxConditions)
	case len(r.RequiresFresh) > MaxFresh:
		return bad("at most %d sources in requires_fresh", MaxFresh)
	}
	for _, s := range r.RequiresFresh {
		if !sourceRe.MatchString(s) {
			return bad("requires_fresh names %q, which is not a source system name", s)
		}
	}
	seen := map[string]bool{}
	for i, c := range r.All {
		if err := c.validate(); err != nil {
			return bad("condition %d: %v", i+1, err)
		}
		key := c.Field + "/" + c.Op
		if seen[key] {
			return bad("condition %d repeats %s %s; combine them into one", i+1, c.Field, c.Op)
		}
		seen[key] = true
	}
	return r.checkSafety()
}

func (c Condition) validate() error {
	k, ok := fields[c.Field]
	if !ok {
		return fmt.Errorf("%q is not a field a rule can read", c.Field)
	}
	switch c.Op {
	case "eq", "ne":
		if k == number {
			n, err := c.number()
			if err != nil {
				return err
			}
			_ = n
			return nil
		}
		v, err := c.text()
		if err != nil {
			return err
		}
		return checkEnum(c.Field, v)
	case "in", "not_in":
		if k != text {
			return fmt.Errorf("%s works on text fields only", c.Op)
		}
		list, err := c.list()
		if err != nil {
			return err
		}
		for _, v := range list {
			if err := checkEnum(c.Field, v); err != nil {
				return err
			}
		}
		return nil
	case "gt", "gte", "lt", "lte":
		if k != number {
			return fmt.Errorf("%s works on number fields only", c.Op)
		}
		n, err := c.number()
		if err != nil {
			return err
		}
		if n < 0 {
			return errors.New("a threshold may not be negative")
		}
		return nil
	}
	return fmt.Errorf("op %q is not one of eq, ne, in, not_in, gt, gte, lt, lte", c.Op)
}

func checkEnum(field, v string) error {
	if !valueRe.MatchString(v) {
		return fmt.Errorf("value %q has unsupported characters", v)
	}
	if e, ok := enums[field]; ok && !e[v] {
		return fmt.Errorf("%q is not a value of %s", v, field)
	}
	return nil
}

func (c Condition) text() (string, error) {
	var s string
	if err := json.Unmarshal(c.Value, &s); err != nil {
		return "", errors.New("value must be a string")
	}
	return s, nil
}

func (c Condition) number() (int64, error) {
	var n int64
	if err := json.Unmarshal(c.Value, &n); err != nil {
		return 0, errors.New("value must be a whole number")
	}
	return n, nil
}

func (c Condition) list() ([]string, error) {
	var l []string
	if err := json.Unmarshal(c.Value, &l); err != nil || len(l) == 0 {
		return nil, errors.New("value must be a non-empty list of strings")
	}
	if len(l) > MaxListItems {
		return nil, fmt.Errorf("at most %d items in a list", MaxListItems)
	}
	return l, nil
}

// checkSafety refuses rules that could treat an unknown outcome as a confirmed
// failure. A failure-family code must be guarded by lifecycle_state equal to
// CONFIRMED_FAILURE. An unknown-family rule may not claim a confirmed state.
func (r *Rule) checkSafety() error {
	var lifecycleEq string
	for _, c := range r.All {
		if c.Field == "lifecycle_state" && c.Op == "eq" {
			lifecycleEq, _ = c.text()
		}
	}
	if failureFamily[r.Code] && lifecycleEq != "CONFIRMED_FAILURE" {
		return bad("a %s rule must require lifecycle_state eq CONFIRMED_FAILURE: an unknown or unconfirmed outcome is never a failure", r.Code)
	}
	if r.Code == "UNK001" {
		for _, c := range r.All {
			if c.Field != "lifecycle_state" {
				continue
			}
			var vals []string
			switch c.Op {
			case "eq":
				v, _ := c.text()
				vals = []string{v}
			case "in":
				vals, _ = c.list()
			}
			for _, v := range vals {
				if v == "CONFIRMED_FAILURE" || v == "CONFIRMED_SUCCESS" || v == "REVERSED" {
					return bad("a UNK001 rule cannot be about a transaction whose outcome is already confirmed (%s)", v)
				}
			}
		}
	}
	return nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Facts are what a rule can see about one transaction.
type Facts struct {
	LifecycleState         string
	CreditStatus           string
	DebitStatus            string
	Rail                   string
	Channel                string
	Currency               string
	AmountMinor            int64
	EventCount             int64
	SecondsSinceLastEvent  int64
	SecondsSinceFirstDebit int64
}

func (f Facts) text(field string) string {
	switch field {
	case "lifecycle_state":
		return f.LifecycleState
	case "credit_status":
		return f.CreditStatus
	case "debit_status":
		return f.DebitStatus
	case "rail":
		return f.Rail
	case "channel":
		return f.Channel
	case "currency":
		return f.Currency
	}
	return ""
}

func (f Facts) number(field string) int64 {
	switch field {
	case "amount_minor":
		return f.AmountMinor
	case "event_count":
		return f.EventCount
	case "seconds_since_last_event":
		return f.SecondsSinceLastEvent
	case "seconds_since_first_debit":
		return f.SecondsSinceFirstDebit
	}
	return 0
}

// Matches reports whether every condition holds. The rule must have been validated.
func (r *Rule) Matches(f Facts) bool {
	for _, c := range r.All {
		if !c.holds(f) {
			return false
		}
	}
	return true
}

func (c Condition) holds(f Facts) bool {
	if fields[c.Field] == number {
		want, _ := c.number()
		have := f.number(c.Field)
		switch c.Op {
		case "eq":
			return have == want
		case "ne":
			return have != want
		case "gt":
			return have > want
		case "gte":
			return have >= want
		case "lt":
			return have < want
		case "lte":
			return have <= want
		}
		return false
	}
	have := f.text(c.Field)
	switch c.Op {
	case "eq":
		want, _ := c.text()
		return have == want
	case "ne":
		want, _ := c.text()
		return have != want
	case "in", "not_in":
		list, _ := c.list()
		in := false
		for _, v := range list {
			if v == have {
				in = true
			}
		}
		return in == (c.Op == "in")
	}
	return false
}
