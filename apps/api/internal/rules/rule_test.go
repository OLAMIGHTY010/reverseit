package rules

import (
	"errors"
	"strings"
	"testing"
)

const goodATM = `{"rule_id":"DET-ATM-2","version":1,"code":"ATM001","risk":"HIGH","root_cause":"atm_hardware_fault",
 "explanation":"Cash was not dispensed after the debit posted.",
 "all":[{"field":"lifecycle_state","op":"eq","value":"CONFIRMED_FAILURE"},{"field":"channel","op":"in","value":["ATM"]},{"field":"amount_minor","op":"gte","value":100}]}`

func TestAWellFormedRuleParsesAndMatchesOnlyWhatItSays(t *testing.T) {
	r, err := Parse([]byte(goodATM))
	if err != nil {
		t.Fatal(err)
	}
	f := Facts{LifecycleState: "CONFIRMED_FAILURE", Channel: "ATM", AmountMinor: 100}
	if !r.Matches(f) {
		t.Fatal("should match")
	}
	for name, mut := range map[string]func(*Facts){
		"another channel":    func(f *Facts) { f.Channel = "POS" },
		"amount too small":   func(f *Facts) { f.AmountMinor = 99 },
		"an unknown outcome": func(f *Facts) { f.LifecycleState = "UNKNOWN" },
	} {
		g := f
		mut(&g)
		if r.Matches(g) {
			t.Errorf("%s matched", name)
		}
	}
}

func TestEveryOperatorBehaves(t *testing.T) {
	f := Facts{Rail: "NIP", Channel: "MOBILE", LifecycleState: "UNKNOWN", AmountMinor: 500, SecondsSinceLastEvent: 600}
	cases := []struct {
		c    string
		want bool
	}{
		{`{"field":"rail","op":"eq","value":"NIP"}`, true}, {`{"field":"rail","op":"eq","value":"ATM"}`, false},
		{`{"field":"rail","op":"ne","value":"ATM"}`, true}, {`{"field":"rail","op":"ne","value":"NIP"}`, false},
		{`{"field":"rail","op":"in","value":["ATM","NIP"]}`, true}, {`{"field":"rail","op":"in","value":["ATM"]}`, false},
		{`{"field":"rail","op":"not_in","value":["ATM"]}`, true}, {`{"field":"rail","op":"not_in","value":["NIP"]}`, false},
		{`{"field":"amount_minor","op":"gt","value":499}`, true}, {`{"field":"amount_minor","op":"gt","value":500}`, false},
		{`{"field":"amount_minor","op":"gte","value":500}`, true}, {`{"field":"amount_minor","op":"gte","value":501}`, false},
		{`{"field":"amount_minor","op":"lt","value":501}`, true}, {`{"field":"amount_minor","op":"lt","value":500}`, false},
		{`{"field":"amount_minor","op":"lte","value":500}`, true}, {`{"field":"amount_minor","op":"lte","value":499}`, false},
		{`{"field":"amount_minor","op":"eq","value":500}`, true}, {`{"field":"amount_minor","op":"ne","value":500}`, false},
		{`{"field":"seconds_since_last_event","op":"gte","value":300}`, true},
	}
	for _, c := range cases {
		doc := `{"rule_id":"DET-T-1","version":1,"code":"UNK001","root_cause":"switch_response_failure","explanation":"Waiting for a final answer.","all":[` + c.c + `]}`
		r, err := Parse([]byte(doc))
		if err != nil {
			t.Fatalf("%s: %v", c.c, err)
		}
		if got := r.Matches(f); got != c.want {
			t.Errorf("%s = %v, want %v", c.c, got, c.want)
		}
	}
}

func TestValidationRefusesWhatCouldGoWrong(t *testing.T) {
	cond := `{"field":"lifecycle_state","op":"eq","value":"CONFIRMED_FAILURE"}`
	wrap := func(code, extra string, conds ...string) string {
		return `{"rule_id":"DET-X-1","version":1,"code":"` + code + `","root_cause":"some_cause","explanation":"A long enough explanation."` + extra + `,"all":[` + strings.Join(conds, ",") + `]}`
	}
	cases := map[string]string{
		"no conditions: would match everything":    wrap("UNK001", ""),
		"duplicate detection is not a rule":        wrap("DUP001", "", cond),
		"reversal code is not a detection code":    wrap("REV001", "", cond),
		"unknown code":                             wrap("XYZ999", "", cond),
		"failure rule without the confirmed guard": wrap("FAIL001", "", `{"field":"rail","op":"eq","value":"NIP"}`),
		"failure rule guarded by the wrong state":  wrap("FAIL001", "", `{"field":"lifecycle_state","op":"eq","value":"UNKNOWN"}`),
		"failure rule that allows unknown":         wrap("FAIL001", "", `{"field":"lifecycle_state","op":"in","value":["CONFIRMED_FAILURE","UNKNOWN"]}`),
		"unknown rule about a confirmed outcome":   wrap("UNK001", "", `{"field":"lifecycle_state","op":"eq","value":"CONFIRMED_FAILURE"}`),
		"unknown rule listing a confirmed outcome": wrap("UNK001", "", `{"field":"lifecycle_state","op":"in","value":["UNKNOWN","REVERSED"]}`),
		"field the rule may not read":              wrap("UNK001", "", `{"field":"debit_account_token","op":"eq","value":"x"}`),
		"unknown operator":                         wrap("UNK001", "", `{"field":"rail","op":"like","value":"N%"}`),
		"ordering on text":                         wrap("UNK001", "", `{"field":"rail","op":"gt","value":"A"}`),
		"in on a number":                           wrap("UNK001", "", `{"field":"amount_minor","op":"in","value":["1"]}`),
		"text value for a number":                  wrap("UNK001", "", `{"field":"amount_minor","op":"gt","value":"5"}`),
		"negative threshold":                       wrap("UNK001", "", `{"field":"amount_minor","op":"gt","value":-1}`),
		"not a lifecycle value":                    wrap("UNK001", "", `{"field":"lifecycle_state","op":"eq","value":"MAYBE"}`),
		"empty list":                               wrap("UNK001", "", `{"field":"rail","op":"in","value":[]}`),
		"repeated field and operator":              wrap("UNK001", "", `{"field":"rail","op":"eq","value":"NIP"}`, `{"field":"rail","op":"eq","value":"ATM"}`),
		"odd characters in a value":                wrap("UNK001", "", `{"field":"rail","op":"eq","value":"N IP;"}`),
		"too many fresh sources":                   wrap("UNK001", `,"requires_fresh":["AA","BB","CC","DD","EE","FF"]`, `{"field":"rail","op":"eq","value":"NIP"}`),
		"bad source name":                          wrap("UNK001", `,"requires_fresh":["lower"]`, `{"field":"rail","op":"eq","value":"NIP"}`),
		"unknown field in the document":            wrap("UNK001", `,"auto_reverse":true`, `{"field":"rail","op":"eq","value":"NIP"}`),
		"bad risk":                                 wrap("UNK001", `,"risk":"SEVERE"`, `{"field":"rail","op":"eq","value":"NIP"}`),
		"explanation with a newline":               strings.Replace(wrap("UNK001", "", `{"field":"rail","op":"eq","value":"NIP"}`), "A long enough explanation.", "line one\\nline two xx", 1),
		"short explanation":                        strings.Replace(wrap("UNK001", "", `{"field":"rail","op":"eq","value":"NIP"}`), "A long enough explanation.", "short", 1),
		"version zero":                             strings.Replace(wrap("UNK001", "", `{"field":"rail","op":"eq","value":"NIP"}`), `"version":1`, `"version":0`, 1),
		"lower case id":                            strings.Replace(wrap("UNK001", "", `{"field":"rail","op":"eq","value":"NIP"}`), "DET-X-1", "det-x-1", 1),
		"not JSON":                                 `{`,
	}
	for name, doc := range cases {
		if _, err := Parse([]byte(doc)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The count limit is checked directly, because distinct field and operator pairs run out first.
	r := Rule{RuleID: "DET-X-1", Version: 1, Code: "UNK001", RootCause: "some_cause", Explanation: "A long enough explanation.", All: make([]Condition, MaxConditions+1)}
	if err := r.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("%v", err)
	}
}

func TestTheFailureGuardCannotBeBypassedByAnyCombination(t *testing.T) {
	// Every failure-family code, with the guard, is accepted; without it, refused.
	for code := range failureFamily {
		guarded := `{"rule_id":"DET-G-1","version":1,"code":"` + code + `","root_cause":"some_cause","explanation":"A long enough explanation.","all":[{"field":"lifecycle_state","op":"eq","value":"CONFIRMED_FAILURE"}]}`
		if _, err := Parse([]byte(guarded)); err != nil {
			t.Errorf("%s: %v", code, err)
		}
		unguarded := strings.Replace(guarded, `"op":"eq","value":"CONFIRMED_FAILURE"`, `"op":"ne","value":"CONFIRMED_SUCCESS"`, 1)
		if _, err := Parse([]byte(unguarded)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s accepted without the guard: %v", code, err)
		}
	}
}
