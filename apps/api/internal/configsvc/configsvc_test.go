package configsvc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type rig struct {
	pool   *db.Pool
	svc    *Service
	tenant uuid.UUID
	maker  uuid.UUID
	check  uuid.UUID
	now    time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), maker: uuid.New(), check: uuid.New(),
		now: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)}
	r.svc = &Service{Audit: &audit.Appender{Now: func() time.Time { return r.now }}, Now: func() time.Time { return r.now }}
	return r
}

func (r *rig) tx(t *testing.T, fn func(tx pgx.Tx) error) error {
	t.Helper()
	return r.pool.WithTenant(context.Background(), r.tenant, fn)
}

func (r *rig) propose(t *testing.T, key string, value any, effective time.Time) Version {
	t.Helper()
	raw, _ := json.Marshal(value)
	var v Version
	err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		v, err = r.svc.Propose(context.Background(), tx, r.tenant, r.maker, ProposeInput{
			Key: key, Value: raw, SourceReference: "Scope FR-04", EffectiveFrom: effective,
			VerifyStatus: "unverified", Reason: "initial value from the scope document",
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (r *rig) decide(t *testing.T, key string, version int, approve bool, reason string) error {
	t.Helper()
	return r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.Decide(context.Background(), tx, r.tenant, r.check, key, version, approve, reason)
		return err
	})
}

func (r *rig) effective(t *testing.T, key string, at time.Time) (Version, error) {
	t.Helper()
	var v Version
	err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		v, err = r.svc.Effective(context.Background(), tx, r.tenant, key, at)
		return err
	})
	return v, err
}

const key = "duplicate.window_seconds"

func TestNothingIsConfiguredUntilSomeoneApproves(t *testing.T) {
	r := newRig(t)
	if _, err := r.effective(t, key, r.now); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("empty: %v", err)
	}
	v := r.propose(t, key, 30, r.now)
	if v.Version != 1 || v.Value != 30 || v.Decision != "" {
		t.Fatalf("proposal: %+v", v)
	}
	if _, err := r.effective(t, key, r.now.Add(time.Hour)); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("a proposal must not be in force: %v", err)
	}
	if err := r.decide(t, key, 1, true, ""); err != nil {
		t.Fatal(err)
	}
	got, err := r.effective(t, key, r.now.Add(time.Hour))
	if err != nil || got.Value != 30 || got.Decision != "approved" || got.DecidedBy != r.check || got.SourceReference != "Scope FR-04" || got.VerifyStatus != "unverified" {
		t.Fatalf("effective: %+v %v", got, err)
	}
}

func TestRejectedChangesNeverTakeEffectAndNeedAReason(t *testing.T) {
	r := newRig(t)
	r.propose(t, key, 45, r.now)
	if err := r.decide(t, key, 1, false, "   "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reject without reason: %v", err)
	}
	if err := r.decide(t, key, 1, false, "not supported by the policy source"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.effective(t, key, r.now.Add(time.Hour)); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("rejected value in force: %v", err)
	}
	h := r.history(t)
	if len(h) != 1 || h[0].Decision != "rejected" || h[0].DecisionReason == "" {
		t.Fatalf("history: %+v", h)
	}
}

func (r *rig) history(t *testing.T) []Version {
	t.Helper()
	var out []Version
	err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		out, err = r.svc.History(context.Background(), tx, r.tenant, key)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTheProposerCanNeverDecideTheirOwnChange(t *testing.T) {
	r := newRig(t)
	r.propose(t, key, 30, r.now)

	err := r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.Decide(context.Background(), tx, r.tenant, r.maker, key, 1, true, "")
		return err
	})
	if !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("service: %v", err)
	}

	// The database refuses it too, even if the service were bypassed.
	err = r.tx(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO config_decisions (tenant_id, key, version, decision, decided_by) VALUES ($1, $2, 1, 'approved', $3)`,
			r.tenant, key, r.maker)
		return err
	})
	if err == nil {
		t.Fatal("the database accepted a self-approval")
	}
	if _, e := r.effective(t, key, r.now.Add(time.Hour)); !errors.Is(e, ErrNotConfigured) {
		t.Fatal("a self-approved change took effect")
	}
}

func TestAVersionIsDecidedOnceAndUnknownVersionsAreRefused(t *testing.T) {
	r := newRig(t)
	r.propose(t, key, 30, r.now)
	if err := r.decide(t, key, 1, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := r.decide(t, key, 1, false, "changed my mind"); !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("second decision: %v", err)
	}
	if err := r.decide(t, key, 9, true, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown version: %v", err)
	}
	if err := r.decide(t, "no.such.key", 1, true, ""); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unknown key: %v", err)
	}
}

func TestValuesAreValidatedAgainstTheKeyRegistry(t *testing.T) {
	r := newRig(t)
	propose := func(k string, raw string) error {
		return r.tx(t, func(tx pgx.Tx) error {
			_, err := r.svc.Propose(context.Background(), tx, r.tenant, r.maker, ProposeInput{
				Key: k, Value: json.RawMessage(raw), SourceReference: "src", EffectiveFrom: r.now,
				VerifyStatus: "draft", Reason: "a sufficient reason",
			})
			return err
		})
	}
	for name, c := range map[string]struct {
		key, value string
		err        error
	}{
		"unknown key":         {"made.up", "1", ErrUnknownKey},
		"string":              {key, `"30"`, ErrInvalidValue},
		"float":               {key, `30.5`, ErrInvalidValue},
		"bool":                {key, `true`, ErrInvalidValue},
		"null":                {key, `null`, ErrInvalidValue},
		"object":              {key, `{"a":1}`, ErrInvalidValue},
		"not json":            {key, `{`, ErrInvalidValue},
		"zero where min is 1": {key, `0`, ErrInvalidValue},
		"negative":            {key, `-5`, ErrInvalidValue},
		"over the maximum":    {key, `3601`, ErrInvalidValue},
		"huge":                {key, `99999999999999999999`, ErrInvalidValue},
		"bad rail name":       {"waiting_threshold_seconds.nip", `60`, ErrUnknownKey},
	} {
		if err := propose(c.key, c.value); !errors.Is(err, c.err) {
			t.Errorf("%s: %v, want %v", name, err, c.err)
		}
	}
	for name, c := range map[string]struct{ key, value string }{
		"duplicate window":      {key, `30`},
		"lowest allowed":        {key, `1`},
		"highest allowed":       {key, `3600`},
		"waiting threshold NIP": {"waiting_threshold_seconds.NIP", `300`},
		"zero wait is allowed":  {"waiting_threshold_seconds.POS", `0`},
		"role limit":            {"approval.limit_minor.REVERSAL_OFFICER", `100000000`},
		"auto limit":            {"approval.auto_limit_minor", `0`},
	} {
		if err := propose(c.key, c.value); err != nil {
			t.Errorf("%s rejected: %v", name, err)
		}
	}
}

func TestProposalInputIsValidated(t *testing.T) {
	r := newRig(t)
	good := ProposeInput{Key: key, Value: json.RawMessage(`30`), SourceReference: "src", EffectiveFrom: r.now,
		VerifyStatus: "confirmed", Reason: "a sufficient reason"}
	for name, mutate := range map[string]func(*ProposeInput){
		"no source":         func(p *ProposeInput) { p.SourceReference = "  " },
		"short reason":      func(p *ProposeInput) { p.Reason = "short" },
		"bad verify status": func(p *ProposeInput) { p.VerifyStatus = "probably" },
		"no effective date": func(p *ProposeInput) { p.EffectiveFrom = time.Time{} },
		"effective in past": func(p *ProposeInput) { p.EffectiveFrom = r.now.Add(-time.Hour) },
		"over a year ahead": func(p *ProposeInput) { p.EffectiveFrom = r.now.AddDate(1, 0, 1) },
	} {
		in := good
		mutate(&in)
		err := r.tx(t, func(tx pgx.Tx) error {
			_, err := r.svc.Propose(context.Background(), tx, r.tenant, r.maker, in)
			return err
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: %v", name, err)
		}
	}
	err := r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.Propose(context.Background(), tx, r.tenant, uuid.Nil, good)
		return err
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("no proposer: %v", err)
	}
}

func TestScheduledChangesTakeEffectOnTheirDateAndTheLatestWins(t *testing.T) {
	r := newRig(t)
	r.propose(t, key, 30, r.now)
	r.decide(t, key, 1, true, "")
	nextWeek := r.now.AddDate(0, 0, 7)
	r.propose(t, key, 60, nextWeek)
	r.decide(t, key, 2, true, "")

	for at, want := range map[time.Time]int64{
		r.now:                         30,
		r.now.Add(6 * 24 * time.Hour): 30,
		nextWeek:                      60,
		nextWeek.Add(time.Hour):       60,
	} {
		got, err := r.effective(t, key, at)
		if err != nil || got.Value != want {
			t.Errorf("at %s: %+v %v, want %d", at.Format(time.RFC3339), got, err, want)
		}
	}
	if _, err := r.effective(t, key, r.now.Add(-time.Hour)); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("before the first value: %v", err)
	}
}

func TestRollbackIsAnApprovedProposalOfAnOlderValue(t *testing.T) {
	r := newRig(t)
	r.propose(t, key, 30, r.now)
	r.decide(t, key, 1, true, "")
	r.propose(t, key, 90, r.now)
	r.decide(t, key, 2, true, "")
	r.propose(t, key, 45, r.now)
	r.decide(t, key, 3, false, "no evidence for this value")

	var rb Version
	err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		rb, err = r.svc.Rollback(context.Background(), tx, r.tenant, r.maker, key, 1, r.now, "version 2 was wrong")
		return err
	})
	if err != nil || rb.Version != 4 || rb.Value != 30 || rb.Decision != "" {
		t.Fatalf("rollback: %+v %v", rb, err)
	}
	if got, _ := r.effective(t, key, r.now.Add(time.Minute)); got.Value != 90 {
		t.Fatalf("rollback took effect before approval: %+v", got)
	}
	r.decide(t, key, 4, true, "")
	if got, _ := r.effective(t, key, r.now.Add(time.Minute)); got.Value != 30 || got.Version != 4 {
		t.Fatalf("after approval: %+v", got)
	}

	// A version that was rejected, or never existed, cannot be rolled back to.
	for _, v := range []int{3, 99} {
		err := r.tx(t, func(tx pgx.Tx) error {
			_, err := r.svc.Rollback(context.Background(), tx, r.tenant, r.maker, key, v, r.now, "a sufficient reason")
			return err
		})
		if err == nil {
			t.Errorf("rolled back to version %d", v)
		}
	}
}

func TestVersionsAndDecisionsCannotBeChanged(t *testing.T) {
	r := newRig(t)
	r.propose(t, key, 30, r.now)
	r.decide(t, key, 1, true, "")
	for _, stmt := range []string{
		`UPDATE config_versions SET value = '999'`, `DELETE FROM config_versions`,
		`UPDATE config_decisions SET decision = 'rejected'`, `DELETE FROM config_decisions`,
	} {
		if err := r.tx(t, func(tx pgx.Tx) error { _, err := tx.Exec(context.Background(), stmt); return err }); err == nil {
			t.Errorf("tenant role ran %q", stmt)
		}
		if _, err := r.pool.Exec(context.Background(), stmt); err == nil {
			t.Errorf("owner ran %q", stmt)
		}
	}
	if got, err := r.effective(t, key, r.now.Add(time.Minute)); err != nil || got.Value != 30 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestEveryChangeIsAuditedAndTheChainStaysIntact(t *testing.T) {
	r := newRig(t)
	r.propose(t, key, 30, r.now)
	r.decide(t, key, 1, true, "")
	r.propose(t, key, 45, r.now)
	r.decide(t, key, 2, false, "no source for 45")

	var recs []audit.Record
	var res audit.VerifyResult
	err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		if recs, err = audit.List(context.Background(), tx, r.tenant, audit.Filter{SubjectType: "config"}, 0, 50); err != nil {
			return err
		}
		res, err = audit.Verify(context.Background(), tx, r.tenant)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, rec := range recs {
		actions = append(actions, rec.Action)
	}
	want := []string{"config.proposed", "config.approved", "config.proposed", "config.rejected"}
	if len(actions) != 4 {
		t.Fatalf("actions: %v", actions)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("actions: %v, want %v", actions, want)
		}
	}
	if recs[0].ActorID != r.maker || recs[1].ActorID != r.check || recs[3].ReasonCode != "no source for 45" {
		t.Errorf("attribution: %+v", recs)
	}
	if !res.OK {
		t.Fatalf("chain: %+v", res)
	}
}

func TestAFailedProposalLeavesNothingBehind(t *testing.T) {
	r := newRig(t)
	boom := errors.New("later step failed")
	err := r.tx(t, func(tx pgx.Tx) error {
		if _, err := r.svc.Propose(context.Background(), tx, r.tenant, r.maker, ProposeInput{
			Key: key, Value: json.RawMessage(`30`), SourceReference: "src", EffectiveFrom: r.now,
			VerifyStatus: "draft", Reason: "a sufficient reason"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if h := r.history(t); len(h) != 0 {
		t.Fatalf("rolled back proposal persisted: %+v", h)
	}
}

func TestConfigIsIsolatedBetweenTenants(t *testing.T) {
	a, b := newRig(t), newRig(t)
	a.propose(t, key, 30, a.now)
	a.decide(t, key, 1, true, "")
	if _, err := b.effective(t, key, b.now.Add(time.Hour)); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("tenant B saw tenant A's value: %v", err)
	}
	if h := b.history(t); len(h) != 0 {
		t.Fatalf("tenant B history: %+v", h)
	}
}

func TestConcurrentProposalsGetDistinctVersionsOrAClearConflict(t *testing.T) {
	r := newRig(t)
	const n = 8
	var wg sync.WaitGroup
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raw, _ := json.Marshal(10 + i)
			results <- r.tx(t, func(tx pgx.Tx) error {
				_, err := r.svc.Propose(context.Background(), tx, r.tenant, r.maker, ProposeInput{
					Key: key, Value: raw, SourceReference: "src", EffectiveFrom: r.now,
					VerifyStatus: "draft", Reason: "a sufficient reason"})
				return err
			})
		}(i)
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrConflict):
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	h := r.history(t)
	if ok == 0 || len(h) != ok {
		t.Fatalf("%d succeeded but history holds %d", ok, len(h))
	}
	seen := map[int]bool{}
	for _, v := range h {
		if seen[v.Version] {
			t.Fatalf("version %d issued twice", v.Version)
		}
		seen[v.Version] = true
	}
}

func TestRegistryPatterns(t *testing.T) {
	for _, k := range []string{"duplicate.window_seconds", "status_query.freshness_seconds", "waiting_threshold_seconds.NIP",
		"approval.auto_limit_minor", "approval.limit_minor.SUPERVISOR", "sla.pause_max_hours"} {
		if _, ok := Lookup(k); !ok {
			t.Errorf("%s not in the registry", k)
		}
	}
	for _, k := range []string{"", "duplicate.window_seconds.extra", "approval.limit_minor.lower", "approval.limit_minor.", "../etc/passwd", "DUPLICATE.WINDOW_SECONDS"} {
		if _, ok := Lookup(k); ok {
			t.Errorf("%q matched the registry", k)
		}
	}
}

// ---- regulatory clocks ------------------------------------------------------

func TestSeededClocksArePendingUntilSomeoneElseApproves(t *testing.T) {
	r := newRig(t)
	var vs []Version
	if err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		vs, err = r.svc.SeedClockProposals(context.Background(), tx, r.tenant, r.maker, r.now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(vs) != len(SeedClocks) || len(vs) != 7 {
		t.Fatalf("seeded %d clocks", len(vs))
	}
	for _, v := range vs {
		if v.Decision != "" {
			t.Errorf("%s was decided at seeding", v.Key)
		}
		if _, err := r.effective(t, v.Key, r.now.Add(time.Hour)); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("%s is in force before any approval: %v", v.Key, err)
		}
	}

	// The status the pack gave each clock is kept, and the conflicting one is not hidden.
	byKey := map[string]Version{}
	for _, v := range vs {
		byKey[v.Key] = v
	}
	if v := byKey[ClockKey("POS_WEB_DISPUTE")]; v.VerifyStatus != "conflicting" || !strings.Contains(string(v.Raw), `"duration":72`) {
		t.Errorf("POS clock: %+v", v)
	}
	if v := byKey[ClockKey("ATM_ON_US_MANUAL_REFUND")]; v.VerifyStatus != "draft" {
		t.Errorf("ATM on-us clock: %+v", v)
	}
	for _, v := range vs {
		if v.VerifyStatus == "confirmed" {
			t.Errorf("%s claims to be confirmed: press reports have not been verified", v.Key)
		}
		if !strings.Contains(v.SourceReference, "section 5.3") {
			t.Errorf("%s lacks a source: %q", v.Key, v.SourceReference)
		}
	}

	// A person other than the seeder can approve one, and it then reads back.
	if err := r.decide(t, ClockKey("ATM_NOT_ON_US"), 1, true, ""); err != nil {
		t.Fatal(err)
	}
	got, err := r.effective(t, ClockKey("ATM_NOT_ON_US"), r.now.Add(time.Hour))
	var c Clock
	if err != nil || json.Unmarshal(got.Raw, &c) != nil || c.Duration != 48 || c.Unit != "hours" || got.VerifyStatus != "reported" {
		t.Fatalf("approved clock: %+v %+v %v", got, c, err)
	}
	// The seeder cannot approve their own seeding.
	err = r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.Decide(context.Background(), tx, r.tenant, r.maker, ClockKey("POS_WEB_DISPUTE"), 1, true, "")
		return err
	})
	if !errors.Is(err, ErrSelfApproval) {
		t.Errorf("seeder approving: %v", err)
	}
}

func TestClockValuesAreValidated(t *testing.T) {
	r := newRig(t)
	propose := func(raw string) error {
		return r.tx(t, func(tx pgx.Tx) error {
			_, err := r.svc.Propose(context.Background(), tx, r.tenant, r.maker, ProposeInput{
				Key: ClockKey("TEST_CLOCK"), Value: json.RawMessage(raw), SourceReference: "src", EffectiveFrom: r.now,
				VerifyStatus: "draft", Reason: "a sufficient reason"})
			return err
		})
	}
	good := `{"situation":"s","duration":2,"unit":"hours","start_event":"e","calendar":"WAT","owner":"o"}`
	if err := propose(good); err != nil {
		t.Fatalf("good clock rejected: %v", err)
	}
	for name, raw := range map[string]string{
		"an integer":     `30`,
		"not an object":  `"24 hours"`,
		"unknown field":  `{"situation":"s","duration":2,"unit":"hours","start_event":"e","calendar":"WAT","owner":"o","x":1}`,
		"no situation":   `{"situation":" ","duration":2,"unit":"hours","start_event":"e","calendar":"WAT","owner":"o"}`,
		"zero duration":  `{"situation":"s","duration":0,"unit":"hours","start_event":"e","calendar":"WAT","owner":"o"}`,
		"huge duration":  `{"situation":"s","duration":366,"unit":"hours","start_event":"e","calendar":"WAT","owner":"o"}`,
		"bad unit":       `{"situation":"s","duration":2,"unit":"weeks","start_event":"e","calendar":"WAT","owner":"o"}`,
		"no start event": `{"situation":"s","duration":2,"unit":"hours","start_event":"","calendar":"WAT","owner":"o"}`,
		"no calendar":    `{"situation":"s","duration":2,"unit":"hours","start_event":"e","owner":"o"}`,
		"no owner":       `{"situation":"s","duration":2,"unit":"hours","start_event":"e","calendar":"WAT"}`,
		"fractional":     `{"situation":"s","duration":1.5,"unit":"hours","start_event":"e","calendar":"WAT","owner":"o"}`,
	} {
		if err := propose(raw); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A clock id outside the pattern is not a key at all.
	if _, ok := Lookup("regulatory_clock.lower"); ok {
		t.Error("lower-case clock id accepted")
	}
}

func TestEveryRegulatoryClockSeedIsAValidClock(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range SeedClocks {
		if seen[c.ID] {
			t.Errorf("duplicate seed id %s", c.ID)
		}
		seen[c.ID] = true
		raw, _ := json.Marshal(c.Clock)
		if _, err := parseClock(raw); err != nil {
			t.Errorf("%s: %v", c.ID, err)
		}
		if _, ok := Lookup(ClockKey(c.ID)); !ok {
			t.Errorf("%s does not match the registry", c.ID)
		}
		if c.Source == "" || c.Note == "" {
			t.Errorf("%s lacks a source or note", c.ID)
		}
	}
}
