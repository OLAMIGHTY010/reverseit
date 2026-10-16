package deadline

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

var wat = time.FixedZone("WAT", 3600)

type rig struct {
	pool   *db.Pool
	cfg    *configsvc.Service
	tenant uuid.UUID
	now    time.Time
	e      *Engine
	t      *testing.T
}

func newRig(t *testing.T) *rig {
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), now: time.Date(2026, 10, 14, 9, 0, 0, 0, wat), t: t}
	clk := func() time.Time { return r.now }
	r.cfg = &configsvc.Service{Audit: &audit.Appender{Now: clk}, Now: clk}
	r.e = &Engine{Config: r.cfg, Now: clk}
	return r
}

func (r *rig) set(key string, value any, verify string) {
	r.t.Helper()
	raw, _ := json.Marshal(value)
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		v, err := r.cfg.Propose(context.Background(), tx, r.tenant, uuid.New(), configsvc.ProposeInput{Key: key, Value: raw,
			SourceReference: "SYNTHETIC DEMO POLICY - not approved for production", EffectiveFrom: r.now, VerifyStatus: verify, Reason: "synthetic value for a test"})
		if err != nil {
			return err
		}
		_, err = r.cfg.Decide(context.Background(), tx, r.tenant, uuid.New(), key, v.Version, true, "")
		return err
	})
	if err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) clock(id string, c configsvc.Clock, verify string) {
	r.t.Helper()
	r.set(configsvc.ClockKey(id), c, verify)
}

func (r *rig) compute(dispute string, f Facts) Result {
	r.t.Helper()
	var res Result
	if err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		res, err = r.e.Compute(context.Background(), tx, r.tenant, dispute, f)
		return err
	}); err != nil {
		r.t.Fatal(err)
	}
	return res
}

var nip = configsvc.Clock{Situation: "Failed instant transfer", Duration: 24, Unit: "hours", StartEvent: "failure_detected", Calendar: "WAT", Owner: "Compliance"}

func TestAFailedTransferKeepsTheRegulatoryAndInternalDeadlinesApart(t *testing.T) {
	r := newRig(t)
	r.clock("FAILED_INSTANT_TRANSFER", nip, "reported")
	r.set("internal_target_hours.FAILED_INSTANT_TRANSFER", 12, "unverified")
	failure := r.now.Add(-2 * time.Hour)
	report := r.now.Add(-time.Hour)
	res := r.compute(FailedTransfer, Facts{FailureAt: failure, ReportedAt: report})

	if res.Regulatory == nil || !res.Regulatory.Due.Equal(report.Add(24*time.Hour)) || res.Regulatory.Status != "reported" || res.Regulatory.ClockID != "FAILED_INSTANT_TRANSFER" {
		t.Fatalf("the regulatory view runs from the complaint: %+v", res.Regulatory)
	}
	if res.Internal == nil || !res.Internal.Due.Equal(failure.Add(12*time.Hour)) {
		t.Fatalf("the internal target runs from the failure: %+v", res.Internal)
	}
	if res.Binding == nil || !res.Binding.Equal(res.Internal.Due) {
		t.Fatalf("the earlier deadline binds: %v", res.Binding)
	}
	if res.State != OnTrack || res.RemainingSec != int64(res.Internal.Due.Sub(r.now).Seconds()) {
		t.Fatalf("%+v", res)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("%v", res.Missing)
	}
}

func TestNothingIsAssumedWhenAClockOrTargetIsNotApproved(t *testing.T) {
	r := newRig(t)
	res := r.compute(FailedTransfer, Facts{FailureAt: r.now, ReportedAt: r.now})
	if res.Regulatory != nil || res.Internal != nil || res.Binding != nil || len(res.Missing) != 2 || res.State != OnTrack {
		t.Fatalf("%+v", res)
	}
	// A clock approved but with a start moment we do not know is also not a deadline.
	r.clock("FAILED_INSTANT_TRANSFER", nip, "reported")
	res = r.compute(FailedTransfer, Facts{FailureAt: r.now})
	if res.Regulatory != nil || len(res.Missing) < 1 {
		t.Fatalf("%+v", res)
	}
	if _, err := r.e.Compute(context.Background(), nil, r.tenant, "NOT_A_DISPUTE", Facts{}); !errors.Is(err, ErrUnknownDispute) {
		t.Fatalf("%v", err)
	}
}

func TestWorkingDaysSkipWeekendsAndAPublicHoliday(t *testing.T) {
	r := newRig(t)
	r.clock("WRONG_ACCOUNT_REQUEST", configsvc.Clock{Situation: "Written reversal request", Duration: 14, Unit: "working_days", StartEvent: "transaction_date", Calendar: "WAT", Owner: "Compliance"}, "reported")
	start := time.Date(2026, 10, 14, 10, 0, 0, 0, wat) // a Wednesday
	res := r.compute(WrongAccountOrAm, Facts{TransactionDateAt: start, ReportedAt: r.now})
	if res.Regulatory == nil {
		t.Fatalf("%+v", res)
	}
	// 14 working days after Wed 14 Oct: Oct 15, 16, then three full weeks of five days
	// would overshoot; counting gives Tue 3 Nov.
	if got := res.Regulatory.Due.In(wat).Format("2006-01-02 Mon"); got != "2026-11-03 Tue" {
		t.Fatalf("across two weekends: %s", got)
	}
	// A public holiday on Wed 21 Oct pushes it back by one working day.
	r.set("calendar.holiday.2026-10-21", 1, "unverified")
	res = r.compute(WrongAccountOrAm, Facts{TransactionDateAt: start, ReportedAt: r.now})
	if got := res.Regulatory.Due.In(wat).Format("2006-01-02 Mon"); got != "2026-11-04 Wed" {
		t.Fatalf("across a holiday: %s", got)
	}
}

func TestStatesFollowTheApprovedPercentagesAndOverdueNeedsNone(t *testing.T) {
	r := newRig(t)
	r.clock("POS_WEB_DISPUTE", configsvc.Clock{Situation: "POS or web dispute", Duration: 72, Unit: "hours", StartEvent: "dispute_reported", Calendar: "WAT", Owner: "Compliance"}, "conflicting")
	reported := r.now
	check := func(after time.Duration) Result {
		r.now = reported.Add(after)
		return r.compute(PosWebFailure, Facts{ReportedAt: reported, FailureAt: reported})
	}
	if res := check(71 * time.Hour); res.State != OnTrack {
		t.Fatalf("without approved percentages a deadline is on track until it passes: %s", res.State)
	}
	if res := check(72 * time.Hour); res.State != Overdue || res.RemainingSec != 0 {
		t.Fatalf("%+v", res)
	}
	if res := check(80 * time.Hour); res.State != Overdue || res.RemainingSec >= 0 {
		t.Fatalf("%+v", res)
	}
	// The conflict in the registry is shown, not hidden.
	r.now = reported
	if res := check(time.Hour); res.Regulatory == nil || res.Regulatory.Status != "conflicting" {
		t.Fatalf("%+v", res.Regulatory)
	}
}

func TestApprovedPercentagesTurnOnTheApproachingAndUrgentStates(t *testing.T) {
	r := newRig(t)
	r.clock("POS_WEB_DISPUTE", configsvc.Clock{Situation: "POS or web dispute", Duration: 72, Unit: "hours", StartEvent: "dispute_reported", Calendar: "WAT", Owner: "Compliance"}, "conflicting")
	r.set("deadline.approaching_percent", 50, "unverified")
	r.set("deadline.urgent_percent", 90, "unverified")
	reported := r.now
	for after, want := range map[time.Duration]string{10 * time.Hour: OnTrack, 36 * time.Hour: Approaching, 64*time.Hour + 48*time.Minute: Urgent, 70 * time.Hour: Urgent, 73 * time.Hour: Overdue} {
		r.now = reported.Add(after)
		if res := r.compute(PosWebFailure, Facts{ReportedAt: reported, FailureAt: reported}); res.State != want {
			t.Errorf("after %s: %s, want %s", after, res.State, want)
		}
	}
}

func TestAnUnmatchedClaimHasOnlyAnAcknowledgementTarget(t *testing.T) {
	r := newRig(t)
	res := r.compute(UnmatchedClaim, Facts{ReportedAt: r.now})
	if res.Binding != nil || len(res.Missing) != 1 {
		t.Fatalf("%+v", res)
	}
	r.set("customer.ack_target_hours", 4, "unverified")
	res = r.compute(UnmatchedClaim, Facts{ReportedAt: r.now})
	if res.Regulatory != nil || res.Internal == nil || !res.Internal.Due.Equal(r.now.Add(4*time.Hour)) || res.Internal.Source != "customer.ack_target_hours" {
		t.Fatalf("no regulatory clock until matched: %+v", res)
	}
}

func TestEveryDisputeTypeHasADeterministicClock(t *testing.T) {
	want := map[string]string{
		FailedTransfer: "FAILED_INSTANT_TRANSFER", DuplicateDebit: "FAILED_INSTANT_TRANSFER", WrongAccountOrAm: "WRONG_ACCOUNT_REQUEST",
		PosWebFailure: "POS_WEB_DISPUTE", AtmOnUs: "ATM_ON_US_MANUAL_REFUND", AtmNotOnUs: "ATM_NOT_ON_US", UnmatchedClaim: "",
	}
	for d, clk := range want {
		if got, ok := ClockFor(d); !ok || got != clk {
			t.Errorf("%s: %q %v", d, got, ok)
		}
	}
	seeded := map[string]bool{}
	for _, c := range configsvc.SeedClocks {
		seeded[c.ID] = true
	}
	for d, clk := range want {
		if clk != "" && !seeded[clk] {
			t.Errorf("%s maps to %s, which the clock registry does not seed", d, clk)
		}
	}
}
