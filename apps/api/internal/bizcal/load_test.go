package bizcal_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/bizcal"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

var lagos, _ = time.LoadLocation("Africa/Lagos")

func at(y int, m time.Month, d, hh, mm int) time.Time { return time.Date(y, m, d, hh, mm, 0, 0, lagos) }

func TestCalendarIsBuiltFromApprovedConfigurationOnly(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	tenant := dbtest.Tenant(t, pool)
	now := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	svc := &configsvc.Service{Audit: &audit.Appender{Now: func() time.Time { return now }}, Now: func() time.Time { return now }}
	maker, checker := uuid.New(), uuid.New()

	run := func(fn func(tx pgx.Tx) error) {
		t.Helper()
		if err := pool.WithTenant(ctx, tenant, fn); err != nil {
			t.Fatal(err)
		}
	}
	set := func(key string, value int, approve bool, effective time.Time, reason string) {
		t.Helper()
		raw, _ := json.Marshal(value)
		run(func(tx pgx.Tx) error {
			v, err := svc.Propose(ctx, tx, tenant, maker, configsvc.ProposeInput{
				Key: key, Value: raw, SourceReference: "test fixture", EffectiveFrom: effective,
				VerifyStatus: "unverified", Reason: reason})
			if err != nil || !approve {
				return err
			}
			_, err = svc.Decide(ctx, tx, tenant, checker, key, v.Version, true, "")
			return err
		})
	}

	set("calendar.holiday.2026-10-14", 1, true, now, "Test holiday on a Wednesday")
	set("calendar.holiday.2026-10-15", 1, false, now, "Proposed but never approved")          // pending: not in force
	set("calendar.holiday.2026-10-16", 1, true, now.AddDate(0, 0, 30), "Scheduled for later") // not yet in force
	set("calendar.cutoff_minute.NIP", 16*60, true, now, "NIP cut-off 16:00 for the test")
	set("calendar.settlement_lag_days.NIP", 0, true, now, "NIP settles the same day in the test")
	set("calendar.cutoff_minute.POS", 17*60, true, now, "POS cut-off with no lag configured")
	// A holiday is then cancelled by a later approved version with value 0.
	set("calendar.holiday.2026-10-20", 1, true, now, "Holiday that is later cancelled")
	later := now.Add(48 * time.Hour)
	set("calendar.holiday.2026-10-20", 0, true, later, "Cancelled: it is a working day after all")

	load := func(when time.Time) *bizcal.Calendar {
		t.Helper()
		var cal *bizcal.Calendar
		run(func(tx pgx.Tx) error {
			var err error
			cal, err = bizcal.Load(ctx, tx, svc, tenant, when)
			return err
		})
		return cal
	}

	cal := load(now.Add(time.Hour))
	d := func(day int) bizcal.Date { return bizcal.Date{Year: 2026, Month: 10, Day: day} }
	if cal.IsBusinessDay(d(14)) {
		t.Error("the approved holiday is not honoured")
	}
	if !cal.IsBusinessDay(d(15)) {
		t.Error("a proposal that was never approved changed the calendar")
	}
	if !cal.IsBusinessDay(d(16)) {
		t.Error("a holiday scheduled for the future is already in force")
	}
	if cal.IsBusinessDay(d(20)) {
		t.Error("the holiday that is later cancelled should still stand today")
	}
	if cal.Location().String() != "Africa/Lagos" {
		t.Errorf("time zone %s", cal.Location())
	}

	if got, err := cal.NextCutoff("NIP", at(2026, 10, 13, 10, 0)); err != nil || !got.Equal(at(2026, 10, 13, 16, 0)) {
		t.Errorf("NIP cut-off: %s %v", got, err)
	}
	if got, err := cal.SettlementDate("NIP", at(2026, 10, 13, 17, 0)); err != nil || got != d(15) {
		t.Errorf("settlement across the holiday: %s %v", got, err)
	}
	// POS has a cut-off but no lag, so it has no usable rule.
	if _, err := cal.NextCutoff("POS", at(2026, 10, 13, 10, 0)); !errors.Is(err, bizcal.ErrNoCutoff) {
		t.Errorf("half-configured rail: %v", err)
	}

	// Later, the schedule and the cancellation have both taken effect.
	cal = load(later.Add(31 * 24 * time.Hour))
	if cal.IsBusinessDay(d(16)) {
		t.Error("the scheduled holiday did not take effect")
	}
	if !cal.IsBusinessDay(d(20)) {
		t.Error("the cancellation did not take effect")
	}
}

func TestAnInstitutionWithNothingConfiguredHasNoHolidaysAndNoCutoffs(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	tenant := dbtest.Tenant(t, pool)
	var cal *bizcal.Calendar
	err := pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		cal, err = bizcal.Load(ctx, tx, &configsvc.Service{}, tenant, time.Now())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cal.NextCutoff("NIP", time.Now()); !errors.Is(err, bizcal.ErrNoCutoff) {
		t.Errorf("no cut-off should be assumed: %v", err)
	}
	if !cal.IsBusinessDay(bizcal.Date{Year: 2026, Month: 10, Day: 14}) {
		t.Error("a holiday appeared from nowhere")
	}
	if cal.IsBusinessDay(bizcal.Date{Year: 2026, Month: 10, Day: 17}) {
		t.Error("Saturday is not a weekend day")
	}
}

func TestRegistryAcceptsOnlyWellFormedCalendarKeys(t *testing.T) {
	for _, k := range []string{"calendar.holiday.2026-10-01", "calendar.cutoff_minute.NIP", "calendar.settlement_lag_days.CARD"} {
		if _, ok := configsvc.Lookup(k); !ok {
			t.Errorf("%s not accepted", k)
		}
	}
	for _, k := range []string{"calendar.holiday.tomorrow", "calendar.holiday.2026-10-1", "calendar.cutoff_minute.nip", "calendar.cutoff_minute."} {
		if _, ok := configsvc.Lookup(k); ok {
			t.Errorf("%s accepted", k)
		}
	}
}
