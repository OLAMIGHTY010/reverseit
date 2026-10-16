package bizcal

import (
	"errors"
	"testing"
	"time"
)

var lagos, _ = time.LoadLocation("Africa/Lagos")

func at(y int, m time.Month, d, hh, mm int) time.Time { return time.Date(y, m, d, hh, mm, 0, 0, lagos) }

func date(y int, m time.Month, d int) Date { return Date{y, m, d} }

// October 2026: the 1st is a Thursday. The tests use made-up holidays so no
// claim is made about real public holidays.
func newCal(t *testing.T) *Calendar {
	t.Helper()
	c, err := New(Options{
		Holidays: map[Date]string{
			date(2026, 10, 1):  "Test holiday A (Thursday)",
			date(2026, 10, 14): "Test holiday B (Wednesday)",
		},
		Cutoffs: map[string]Cutoff{
			"NIP": {MinuteOfDay: 16 * 60, LagDays: 0},
			"POS": {MinuteOfDay: 17*60 + 30, LagDays: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestBusinessDays(t *testing.T) {
	c := newCal(t)
	for d, want := range map[Date]bool{
		date(2026, 10, 2):  true,  // Friday
		date(2026, 10, 3):  false, // Saturday
		date(2026, 10, 4):  false, // Sunday
		date(2026, 10, 5):  true,  // Monday
		date(2026, 10, 1):  false, // holiday on a Thursday
		date(2026, 10, 14): false, // holiday on a Wednesday
		date(2026, 10, 15): true,
	} {
		if got := c.IsBusinessDay(d); got != want {
			t.Errorf("%s: %v, want %v", d, got, want)
		}
	}
	if n, ok := c.HolidayName(date(2026, 10, 1)); !ok || n == "" {
		t.Error("holiday name lost")
	}
}

func TestAddWorkingDays(t *testing.T) {
	c := newCal(t)
	cases := []struct {
		name string
		from time.Time
		n    int
		want time.Time
	}{
		{"zero days is a no-op", at(2026, 10, 7, 9, 30), 0, at(2026, 10, 7, 9, 30)},
		{"one day midweek", at(2026, 10, 7, 9, 30), 1, at(2026, 10, 8, 9, 30)},
		{"over a weekend", at(2026, 10, 9, 15, 0), 1, at(2026, 10, 12, 15, 0)},
		{"from a Saturday counts from the next business day", at(2026, 10, 3, 10, 0), 1, at(2026, 10, 5, 10, 0)},
		{"skips a mid-week holiday", at(2026, 10, 13, 8, 0), 1, at(2026, 10, 15, 8, 0)},
		{"five days is a calendar week", at(2026, 10, 5, 12, 0), 5, at(2026, 10, 12, 12, 0)},
		{"five days across a holiday", at(2026, 10, 12, 12, 0), 5, at(2026, 10, 20, 12, 0)},
		{"14 working days skipping a holiday", at(2026, 10, 5, 9, 0), 14, at(2026, 10, 26, 9, 0)},
		{"ending on a holiday-free Monday", at(2026, 10, 15, 23, 59), 3, at(2026, 10, 20, 23, 59)},
	}
	for _, tc := range cases {
		got, err := c.AddWorkingDays(tc.from, tc.n)
		if err != nil || !got.Equal(tc.want) {
			t.Errorf("%s: got %s (%v), want %s", tc.name, got, err, tc.want)
		}
	}
}

func TestAddWorkingDaysUsesTheLocalDateNotUTC(t *testing.T) {
	c := newCal(t)
	// 23:30 UTC on Thursday 8 Oct is 00:30 Friday 9 Oct in Lagos.
	from := time.Date(2026, 10, 8, 23, 30, 0, 0, time.UTC)
	got, err := c.AddWorkingDays(from, 1)
	want := at(2026, 10, 12, 0, 30) // the next business day after Friday is Monday
	if err != nil || !got.Equal(want) {
		t.Fatalf("got %s (%v), want %s", got, err, want)
	}
}

func TestAddWorkingDaysRejectsBadCounts(t *testing.T) {
	c := newCal(t)
	for _, n := range []int{-1, MaxWorkingDays + 1} {
		if _, err := c.AddWorkingDays(at(2026, 10, 7, 9, 0), n); !errors.Is(err, ErrRange) {
			t.Errorf("n=%d: %v", n, err)
		}
	}
	if _, err := c.AddWorkingDays(at(2026, 10, 7, 9, 0), MaxWorkingDays); err != nil {
		t.Errorf("the maximum should be allowed: %v", err)
	}
}

func TestWorkingDaysBetween(t *testing.T) {
	c := newCal(t)
	cases := []struct {
		from, to time.Time
		want     int
	}{
		{at(2026, 10, 7, 9, 0), at(2026, 10, 7, 17, 0), 0},
		{at(2026, 10, 7, 9, 0), at(2026, 10, 8, 1, 0), 1},
		{at(2026, 10, 9, 9, 0), at(2026, 10, 12, 9, 0), 1},
		{at(2026, 10, 9, 9, 0), at(2026, 10, 11, 9, 0), 0},
		{at(2026, 10, 13, 9, 0), at(2026, 10, 15, 9, 0), 1},
		{at(2026, 10, 12, 9, 0), at(2026, 10, 7, 9, 0), -3},
	}
	for _, tc := range cases {
		got, err := c.WorkingDaysBetween(tc.from, tc.to)
		if err != nil || got != tc.want {
			t.Errorf("%s -> %s: %d (%v), want %d", tc.from, tc.to, got, err, tc.want)
		}
	}
	// It undoes AddWorkingDays on a business day.
	for n := 1; n <= 20; n++ {
		from := at(2026, 10, 5, 9, 0)
		to, _ := c.AddWorkingDays(from, n)
		if got, _ := c.WorkingDaysBetween(from, to); got != n {
			t.Errorf("round trip for %d gave %d", n, got)
		}
	}
}

func TestElapsedBusinessTime(t *testing.T) {
	c := newCal(t)
	h := time.Hour
	cases := []struct {
		name     string
		from, to time.Time
		want     time.Duration
	}{
		{"within a day", at(2026, 10, 7, 9, 0), at(2026, 10, 7, 17, 0), 8 * h},
		{"overnight midweek", at(2026, 10, 7, 18, 0), at(2026, 10, 8, 6, 0), 12 * h},
		{"Friday evening to Monday morning counts only the edges", at(2026, 10, 9, 18, 0), at(2026, 10, 12, 6, 0), 12 * h},
		{"a whole weekend counts nothing", at(2026, 10, 3, 0, 0), at(2026, 10, 5, 0, 0), 0},
		{"a holiday counts nothing", at(2026, 10, 13, 12, 0), at(2026, 10, 15, 12, 0), 24 * h},
		{"zero length", at(2026, 10, 7, 9, 0), at(2026, 10, 7, 9, 0), 0},
	}
	for _, tc := range cases {
		got, err := c.ElapsedBusinessTime(tc.from, tc.to)
		if err != nil || got != tc.want {
			t.Errorf("%s: %s (%v), want %s", tc.name, got, err, tc.want)
		}
	}
	if _, err := c.ElapsedBusinessTime(at(2026, 10, 8, 0, 0), at(2026, 10, 7, 0, 0)); !errors.Is(err, ErrRange) {
		t.Errorf("reversed span: %v", err)
	}
	if _, err := c.ElapsedBusinessTime(at(2010, 1, 1, 0, 0), at(2026, 1, 1, 0, 0)); !errors.Is(err, ErrRange) {
		t.Errorf("huge span: %v", err)
	}
}

func TestNextCutoff(t *testing.T) {
	c := newCal(t)
	cases := []struct {
		name string
		rail string
		from time.Time
		want time.Time
	}{
		{"before the cut-off today", "NIP", at(2026, 10, 7, 10, 0), at(2026, 10, 7, 16, 0)},
		{"one minute before", "NIP", at(2026, 10, 7, 15, 59), at(2026, 10, 7, 16, 0)},
		{"exactly at the cut-off has passed", "NIP", at(2026, 10, 7, 16, 0), at(2026, 10, 8, 16, 0)},
		{"after the cut-off", "NIP", at(2026, 10, 7, 20, 0), at(2026, 10, 8, 16, 0)},
		{"Friday after cut-off goes to Monday", "NIP", at(2026, 10, 9, 20, 0), at(2026, 10, 12, 16, 0)},
		{"on a weekend", "NIP", at(2026, 10, 4, 9, 0), at(2026, 10, 5, 16, 0)},
		{"on a holiday morning", "NIP", at(2026, 10, 14, 8, 0), at(2026, 10, 15, 16, 0)},
		{"another rail has its own time", "POS", at(2026, 10, 7, 10, 0), at(2026, 10, 7, 17, 30)},
	}
	for _, tc := range cases {
		got, err := c.NextCutoff(tc.rail, tc.from)
		if err != nil || !got.Equal(tc.want) {
			t.Errorf("%s: %s (%v), want %s", tc.name, got, err, tc.want)
		}
	}
	if _, err := c.NextCutoff("USSD", at(2026, 10, 7, 10, 0)); !errors.Is(err, ErrNoCutoff) {
		t.Errorf("unconfigured rail: %v", err)
	}
}

func TestSettlementDate(t *testing.T) {
	c := newCal(t)
	cases := []struct {
		name string
		rail string
		from time.Time
		want Date
	}{
		{"before the cut-off settles the same day", "NIP", at(2026, 10, 7, 10, 0), date(2026, 10, 7)},
		{"after the cut-off settles the next business day", "NIP", at(2026, 10, 7, 17, 0), date(2026, 10, 8)},
		{"a holiday on the settlement day pushes it on", "NIP", at(2026, 10, 13, 17, 0), date(2026, 10, 15)},
		{"received on the holiday itself", "NIP", at(2026, 10, 14, 9, 0), date(2026, 10, 15)},
		{"Friday after cut-off", "NIP", at(2026, 10, 9, 18, 0), date(2026, 10, 12)},
		{"weekend receipt", "NIP", at(2026, 10, 3, 9, 0), date(2026, 10, 5)},
		{"T+1 skips the weekend", "POS", at(2026, 10, 9, 10, 0), date(2026, 10, 12)},
		{"T+1 skips a holiday", "POS", at(2026, 10, 13, 10, 0), date(2026, 10, 15)},
		{"T+1 after cut-off", "POS", at(2026, 10, 7, 18, 0), date(2026, 10, 9)},
	}
	for _, tc := range cases {
		got, err := c.SettlementDate(tc.rail, tc.from)
		if err != nil || got != tc.want {
			t.Errorf("%s: %s (%v), want %s", tc.name, got, err, tc.want)
		}
	}
	if _, err := c.SettlementDate("ATM", at(2026, 10, 7, 10, 0)); !errors.Is(err, ErrNoCutoff) {
		t.Errorf("unconfigured rail: %v", err)
	}
}

func TestYearBoundaryAndLeapDay(t *testing.T) {
	c, err := New(Options{Holidays: map[Date]string{date(2027, 1, 1): "Test New Year"}})
	if err != nil {
		t.Fatal(err)
	}
	// Thursday 31 Dec 2026 + 1 working day skips Friday 1 Jan (holiday) and the weekend.
	got, _ := c.AddWorkingDays(at(2026, 12, 31, 9, 0), 1)
	if want := at(2027, 1, 4, 9, 0); !got.Equal(want) {
		t.Errorf("year boundary: %s, want %s", got, want)
	}
	// 2028 is a leap year: Monday 28 Feb 2028 + 1 working day is Tuesday 29 Feb.
	got, _ = c.AddWorkingDays(at(2028, 2, 28, 9, 0), 1)
	if want := at(2028, 2, 29, 9, 0); !got.Equal(want) {
		t.Errorf("leap day: %s, want %s", got, want)
	}
	if d, _ := ParseDate("2028-02-29"); d != date(2028, 2, 29) {
		t.Errorf("ParseDate: %v", d)
	}
}

func TestOptionsValidation(t *testing.T) {
	if _, err := New(Options{Weekend: []time.Weekday{0, 1, 2, 3, 4, 5, 6}}); err == nil {
		t.Error("a calendar with no business days was accepted")
	}
	for name, c := range map[string]Cutoff{
		"negative minute": {MinuteOfDay: -1}, "minute too large": {MinuteOfDay: 1440},
		"negative lag": {MinuteOfDay: 0, LagDays: -1}, "huge lag": {MinuteOfDay: 0, LagDays: 11},
	} {
		if _, err := New(Options{Cutoffs: map[string]Cutoff{"X": c}}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for _, bad := range []string{"", "2026-13-01", "2026-02-30", "01/02/2026", "2026-1-5x"} {
		if _, err := ParseDate(bad); err == nil {
			t.Errorf("ParseDate(%q) accepted", bad)
		}
	}
	// A different weekend (Friday and Saturday) is honoured.
	c, _ := New(Options{Weekend: []time.Weekday{time.Friday, time.Saturday}})
	if c.IsBusinessDay(date(2026, 10, 9)) || !c.IsBusinessDay(date(2026, 10, 11)) {
		t.Error("custom weekend ignored")
	}
}

func TestCalendarIsNotAffectedByLaterChangesToItsInputs(t *testing.T) {
	hol := map[Date]string{date(2026, 10, 7): "x"}
	c, _ := New(Options{Holidays: hol})
	delete(hol, date(2026, 10, 7))
	if c.IsBusinessDay(date(2026, 10, 7)) {
		t.Fatal("the calendar changed when its input map did")
	}
}
