package authz

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/google/uuid"

	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

var allRoles = []string{"TXN_ANALYST", "RECON_ANALYST", "REVERSAL_OFFICER", "SUPERVISOR", "HEAD_OPS",
	"RISK_COMPLIANCE", "ADMIN_INTEGRATION", "EXECUTIVE", "AGENT"}

var allCapabilities = []string{"view_exception_queue", "view_unmasked_account", "add_notes_evidence",
	"request_reversal", "approve_reversal", "override_gate", "change_rules", "trigger_kill_switch",
	"view_audit", "manage_users", "export_data"}

// matrix is the v2 prompt pack section 6.4 table, written out independently of
// the migration so that a typo in either shows up as a difference. Columns are
// in allRoles order. "" means no access.
var matrix = map[string][]string{
	"view_exception_queue":  {"full", "full", "full", "full", "full", "full", "metadata", "aggregate", "own_customer"},
	"view_unmasked_account": {"reason_gated", "reason_gated", "reason_gated", "reason_gated", "", "reason_gated", "", "", ""},
	"add_notes_evidence":    {"full", "full", "full", "full", "", "full", "", "", "notes_only"},
	"request_reversal":      {"", "", "full", "full", "", "", "", "", ""},
	"approve_reversal":      {"", "", "within_limit", "within_limit", "higher_limit", "", "", "", ""},
	"override_gate":         {"", "", "", "override_dual", "override_dual", "", "", "", ""},
	"change_rules":          {"", "", "", "propose", "propose", "approve", "draft", "", ""},
	"trigger_kill_switch":   {"", "", "full", "full", "full", "full", "full", "", ""},
	"view_audit":            {"own", "own", "own", "team", "all_ops", "all", "system", "", ""},
	"manage_users":          {"", "", "", "", "", "review", "dual_control", "", ""},
	"export_data":           {"", "masked", "", "masked", "masked", "controlled", "", "aggregate", ""},
}

func TestMatrixInDatabaseMatchesTheV2TableCellForCell(t *testing.T) {
	pol, err := Load(context.Background(), dbtest.Pool(t))
	if err != nil {
		t.Fatal(err)
	}
	cells := 0
	for _, capability := range allCapabilities {
		for i, role := range allRoles {
			cells++
			want := matrix[capability][i]
			got := pol.grants[role][capability]
			if got != want {
				t.Errorf("%s / %s: database has %q, v2 table says %q", role, capability, got, want)
			}
		}
	}
	if cells != 99 {
		t.Fatalf("checked %d cells, want 99", cells)
	}
	for _, r := range allRoles {
		if !pol.roles[r] {
			t.Errorf("role %s missing from the database", r)
		}
	}
	extra := 0
	for _, caps := range pol.grants {
		extra += len(caps)
	}
	want := 0
	for _, row := range matrix {
		for _, v := range row {
			if v != "" {
				want++
			}
		}
	}
	// manage_ingestion is an addition to the v2 table (migration 0023): ADMIN_INTEGRATION full, HEAD_OPS metadata.
	want += 2
	if extra != want {
		t.Errorf("database holds %d grants, the v2 table plus manage_ingestion has %d", extra, want)
	}
}

func TestPrivilegedCapabilitiesRequireMFAInTheDatabase(t *testing.T) {
	pol, err := Load(context.Background(), dbtest.Pool(t))
	if err != nil {
		t.Fatal(err)
	}
	var mfa []string
	for c, m := range pol.mfa {
		if m {
			mfa = append(mfa, c)
		}
	}
	sort.Strings(mfa)
	want := []string{"approve_reversal", "change_rules", "manage_ingestion", "manage_users", "override_gate", "view_unmasked_account"}
	if len(mfa) != len(want) {
		t.Fatalf("MFA required for %v, want %v", mfa, want)
	}
	for i := range want {
		if mfa[i] != want[i] {
			t.Fatalf("MFA required for %v, want %v", mfa, want)
		}
	}
	if pol.mfa["trigger_kill_switch"] {
		t.Error("freezing automation must not require MFA: it is the safe action")
	}
}

// ---- rule behaviour, on an in-memory policy -------------------------------

func policy() *Policy {
	var grants []Grant
	for _, c := range allCapabilities {
		for i, r := range allRoles {
			if s := matrix[c][i]; s != "" {
				grants = append(grants, Grant{Role: r, Capability: c, Scope: s})
			}
		}
	}
	mfa := map[string]bool{"approve_reversal": true, "change_rules": true, "manage_users": true,
		"override_gate": true, "view_unmasked_account": true}
	for _, c := range allCapabilities {
		if _, ok := mfa[c]; !ok {
			mfa[c] = false
		}
	}
	return NewPolicy(grants, mfa, allRoles)
}

func who(mfa bool, roles ...string) *identity.Principal {
	return &identity.Principal{UserID: uuid.New(), TenantID: uuid.New(), Roles: roles, MFA: mfa}
}

func i64(v int64) *int64 { return &v }

func TestEveryRoleAndCapabilityPairFollowsTheMatrix(t *testing.T) {
	pol := policy()
	for _, c := range allCapabilities {
		for i, role := range allRoles {
			want := matrix[c][i]
			// Give every attribute so only the matrix decides.
			p := who(true, role)
			a := Attrs{AmountMinor: i64(1), LimitMinor: i64(10), OwnerID: p.UserID,
				Reason: "a long enough reason", SecondApprover: uuid.New()}
			g, err := pol.Check(p, c, a)
			if want == "" {
				if !errors.Is(err, ErrForbidden) {
					t.Errorf("%s / %s: want forbidden, got %v %v", role, c, g, err)
				}
				continue
			}
			if err != nil || g.Scope != want || g.Role != role || g.Capability != c {
				t.Errorf("%s / %s: got %+v err=%v, want scope %q", role, c, g, err, want)
			}
		}
	}
}

func TestAdminHasNoImplicitAccess(t *testing.T) {
	pol := policy()
	p := who(true, "ADMIN_INTEGRATION")
	for _, c := range []string{"request_reversal", "approve_reversal", "override_gate", "view_unmasked_account"} {
		a := Attrs{AmountMinor: i64(1), LimitMinor: i64(10), Reason: "a long enough reason", SecondApprover: uuid.New()}
		if _, err := pol.Check(p, c, a); !errors.Is(err, ErrForbidden) {
			t.Errorf("admin was allowed %s: %v", c, err)
		}
	}
}

func TestUnknownRoleCapabilityAndNilInputsAreDenied(t *testing.T) {
	pol := policy()
	if _, err := pol.Check(who(true, "SUPER_USER"), "request_reversal", Attrs{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("unknown role: %v", err)
	}
	if _, err := pol.Check(who(true, "SUPERVISOR"), "launch_missiles", Attrs{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("unknown capability: %v", err)
	}
	if _, err := pol.Check(who(true), "request_reversal", Attrs{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("no roles: %v", err)
	}
	if _, err := pol.Check(nil, "request_reversal", Attrs{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("nil principal: %v", err)
	}
	var nilPol *Policy
	if _, err := nilPol.Check(who(true, "SUPERVISOR"), "request_reversal", Attrs{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("nil policy: %v", err)
	}
}

func TestApprovalLimits(t *testing.T) {
	pol := policy()
	p := who(true, "REVERSAL_OFFICER")
	cases := map[string]struct {
		attrs Attrs
		ok    bool
	}{
		"under limit":         {Attrs{AmountMinor: i64(500), LimitMinor: i64(1000)}, true},
		"exactly at limit":    {Attrs{AmountMinor: i64(1000), LimitMinor: i64(1000)}, true},
		"over limit":          {Attrs{AmountMinor: i64(1001), LimitMinor: i64(1000)}, false},
		"no limit configured": {Attrs{AmountMinor: i64(1)}, false},
		"no amount":           {Attrs{LimitMinor: i64(1000)}, false},
		"negative amount":     {Attrs{AmountMinor: i64(-1), LimitMinor: i64(1000)}, false},
		"zero limit":          {Attrs{AmountMinor: i64(1), LimitMinor: i64(0)}, false},
	}
	for name, c := range cases {
		_, err := pol.Check(p, "approve_reversal", c.attrs)
		if c.ok != (err == nil) {
			t.Errorf("%s: err=%v, want ok=%v", name, err, c.ok)
		}
	}
	// Head of ops has a higher scope but the same check applies.
	if _, err := pol.Check(who(true, "HEAD_OPS"), "approve_reversal", Attrs{AmountMinor: i64(5), LimitMinor: i64(1)}); !errors.Is(err, ErrForbidden) {
		t.Errorf("head of ops over limit: %v", err)
	}
}

func TestOwnScopeNeedsTheCallerToBeTheOwner(t *testing.T) {
	pol := policy()
	p := who(true, "TXN_ANALYST")
	if _, err := pol.Check(p, "view_audit", Attrs{OwnerID: p.UserID}); err != nil {
		t.Errorf("own records denied: %v", err)
	}
	if _, err := pol.Check(p, "view_audit", Attrs{OwnerID: uuid.New()}); !errors.Is(err, ErrForbidden) {
		t.Errorf("someone else's records allowed: %v", err)
	}
	if _, err := pol.Check(p, "view_audit", Attrs{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("no owner given allowed: %v", err)
	}
}

func TestReasonGatedNeedsARealReason(t *testing.T) {
	pol := policy()
	p := who(true, "TXN_ANALYST")
	for reason, ok := range map[string]bool{
		"customer called about a missing credit": true,
		"":                                       false,
		"   \t\n  ":                              false,
		"short":                                  false,
		"  too short ":                           false,
	} {
		_, err := pol.Check(p, "view_unmasked_account", Attrs{Reason: reason})
		if ok != (err == nil) {
			t.Errorf("reason %q: err=%v, want ok=%v", reason, err, ok)
		}
	}
}

func TestDualControlNeedsADifferentSecondApproverAndAReason(t *testing.T) {
	pol := policy()
	p := who(true, "SUPERVISOR")
	reason := "gate blocked a verified failure"
	cases := map[string]struct {
		a  Attrs
		ok bool
	}{
		"second approver and reason": {Attrs{Reason: reason, SecondApprover: uuid.New()}, true},
		"no second approver":         {Attrs{Reason: reason}, false},
		"self as second approver":    {Attrs{Reason: reason, SecondApprover: p.UserID}, false},
		"no reason":                  {Attrs{SecondApprover: uuid.New()}, false},
	}
	for name, c := range cases {
		_, err := pol.Check(p, "override_gate", c.a)
		if c.ok != (err == nil) {
			t.Errorf("%s: err=%v, want ok=%v", name, err, c.ok)
		}
	}
}

func TestMFAIsRequiredForPrivilegedActionsOnly(t *testing.T) {
	pol := policy()
	a := Attrs{AmountMinor: i64(1), LimitMinor: i64(10)}
	if _, err := pol.Check(who(false, "REVERSAL_OFFICER"), "approve_reversal", a); !errors.Is(err, ErrMFARequired) {
		t.Errorf("approve without MFA: %v", err)
	}
	if _, err := pol.Check(who(true, "REVERSAL_OFFICER"), "approve_reversal", a); err != nil {
		t.Errorf("approve with MFA: %v", err)
	}
	if _, err := pol.Check(who(false, "REVERSAL_OFFICER"), "trigger_kill_switch", Attrs{}); err != nil {
		t.Errorf("freezing automation needs no MFA: %v", err)
	}
	// Someone without the capability learns nothing about MFA.
	if _, err := pol.Check(who(false, "AGENT"), "approve_reversal", a); !errors.Is(err, ErrForbidden) {
		t.Errorf("agent without MFA got %v, want plain forbidden", err)
	}
}

func TestMultipleRolesTakeTheWidestApplicableScope(t *testing.T) {
	pol := policy()
	p := who(true, "TXN_ANALYST", "HEAD_OPS", "RECON_ANALYST")
	g, err := pol.Check(p, "view_audit", Attrs{OwnerID: p.UserID})
	if err != nil || g.Scope != "all_ops" || g.Role != "HEAD_OPS" {
		t.Fatalf("got %+v err=%v, want HEAD_OPS all_ops", g, err)
	}
	// Two approval roles: the wider scope wins when both rules pass.
	q := who(true, "REVERSAL_OFFICER", "HEAD_OPS")
	g, err = pol.Check(q, "approve_reversal", Attrs{AmountMinor: i64(5), LimitMinor: i64(10)})
	if err != nil || g.Scope != "higher_limit" {
		t.Fatalf("got %+v err=%v", g, err)
	}
}

func TestGrantForSkipsOnlyTheMFARule(t *testing.T) {
	pol := policy()
	noMFA := who(false, "SUPERVISOR")
	if g, err := pol.GrantFor(noMFA, "change_rules", Attrs{}); err != nil || g.Scope != "propose" {
		t.Fatalf("grant without MFA: %+v %v", g, err)
	}
	if _, err := pol.Check(noMFA, "change_rules", Attrs{}); !errors.Is(err, ErrMFARequired) {
		t.Fatalf("Check must still demand MFA: %v", err)
	}
	// Everything else is unchanged: no grant, no access.
	if _, err := pol.GrantFor(who(false, "AGENT"), "change_rules", Attrs{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("agent: %v", err)
	}
	if _, err := pol.GrantFor(who(false, "TXN_ANALYST"), "view_unmasked_account", Attrs{Reason: "short"}); !errors.Is(err, ErrForbidden) {
		t.Errorf("reason gate must still apply: %v", err)
	}
	if _, err := pol.GrantFor(nil, "change_rules", Attrs{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("nil principal: %v", err)
	}
}
