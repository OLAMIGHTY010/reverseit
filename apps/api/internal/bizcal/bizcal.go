// Package bizcal does business-day arithmetic for deadlines and settlement.
//
// Clocks such as "14 working days" and rail cut-offs depend on weekends and
// public holidays, in the institution's own time zone (WAT, Africa/Lagos, which
// has no daylight saving). The calendar holds no built-in holidays and no
// built-in cut-offs: both are configuration with a source (see configsvc), and a
// rail without a configured cut-off is an error, not a guess.
package bizcal

import (
	"errors"
	"fmt"
	"time"
)

// MaxWorkingDays bounds arithmetic so a bad input cannot loop for long.
const MaxWorkingDays = 3650

// Errors.
var (
	ErrNoCutoff = errors.New("bizcal: no cut-off is configured for this rail")
	ErrRange    = errors.New("bizcal: value out of range")
)

// Date is a calendar day with no time zone.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// ParseDate reads YYYY-MM-DD.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, fmt.Errorf("bizcal: %q is not a YYYY-MM-DD date", s)
	}
	return Date{t.Year(), t.Month(), t.Day()}, nil
}

// String returns YYYY-MM-DD.
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day) }

// Cutoff is a rail's daily cut-off and settlement lag.
type Cutoff struct {
	// MinuteOfDay is the local cut-off, in minutes after midnight (0 to 1439).
	// Work received at or after it is treated as received the next business day.
	MinuteOfDay int
	// LagDays is how many business days after the cut-off date settlement lands.
	LagDays int
}

// Calendar is an immutable set of rules. Build one with New.
type Calendar struct {
	loc      *time.Location
	weekend  map[time.Weekday]bool
	holidays map[Date]string
	cutoffs  map[string]Cutoff
}

// Options builds a Calendar.
type Options struct {
	// Location defaults to Africa/Lagos.
	Location *time.Location
	// Weekend defaults to Saturday and Sunday.
	Weekend []time.Weekday
	// Holidays maps a date to its name. A holiday is not a business day.
	Holidays map[Date]string
	// Cutoffs maps a rail to its cut-off.
	Cutoffs map[string]Cutoff
}

// New returns a Calendar.
func New(o Options) (*Calendar, error) {
	loc := o.Location
	if loc == nil {
		var err error
		if loc, err = time.LoadLocation("Africa/Lagos"); err != nil {
			return nil, fmt.Errorf("bizcal: load Africa/Lagos: %w", err)
		}
	}
	weekend := map[time.Weekday]bool{}
	days := o.Weekend
	if days == nil {
		days = []time.Weekday{time.Saturday, time.Sunday}
	}
	for _, d := range days {
		weekend[d] = true
	}
	if len(weekend) == 7 {
		return nil, errors.New("bizcal: every day is a weekend day")
	}
	for rail, c := range o.Cutoffs {
		if c.MinuteOfDay < 0 || c.MinuteOfDay > 1439 || c.LagDays < 0 || c.LagDays > 10 {
			return nil, fmt.Errorf("bizcal: cut-off for %s is out of range", rail)
		}
	}
	hol := make(map[Date]string, len(o.Holidays))
	for d, n := range o.Holidays {
		hol[d] = n
	}
	cut := make(map[string]Cutoff, len(o.Cutoffs))
	for r, c := range o.Cutoffs {
		cut[r] = c
	}
	return &Calendar{loc: loc, weekend: weekend, holidays: hol, cutoffs: cut}, nil
}

// Location returns the calendar's time zone.
func (c *Calendar) Location() *time.Location { return c.loc }

func (c *Calendar) dateOf(t time.Time) Date {
	l := t.In(c.loc)
	return Date{l.Year(), l.Month(), l.Day()}
}

func (c *Calendar) midnight(d Date) time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, c.loc)
}

func (c *Calendar) weekday(d Date) time.Weekday { return c.midnight(d).Weekday() }

func (c *Calendar) nextDate(d Date) Date {
	return c.dateOf(c.midnight(d).AddDate(0, 0, 1))
}

// IsBusinessDay reports whether d is neither a weekend day nor a holiday.
func (c *Calendar) IsBusinessDay(d Date) bool {
	if c.weekend[c.weekday(d)] {
		return false
	}
	_, hol := c.holidays[d]
	return !hol
}

// HolidayName returns the holiday on d, if there is one.
func (c *Calendar) HolidayName(d Date) (string, bool) {
	n, ok := c.holidays[d]
	return n, ok
}

// nextBusinessDay returns the first business day strictly after d.
func (c *Calendar) nextBusinessDay(d Date) Date {
	for i := 0; i < 400; i++ { // a year of holidays and weekends cannot run longer
		d = c.nextDate(d)
		if c.IsBusinessDay(d) {
			return d
		}
	}
	return d
}

// AddWorkingDays returns the moment n business days after t, at the same local
// time of day. Counting starts from t's local date, which is day 0 whether or not
// it is a business day, and each following business day adds one. Weekends and
// holidays in between are skipped. n of zero returns t unchanged.
//
// This counting convention (the start day is not counted) is an assumption. A
// regulator's wording for a given clock may count differently, so the convention
// is documented and should be confirmed with Compliance.
func (c *Calendar) AddWorkingDays(t time.Time, n int) (time.Time, error) {
	if n < 0 || n > MaxWorkingDays {
		return time.Time{}, fmt.Errorf("%w: days must be between 0 and %d", ErrRange, MaxWorkingDays)
	}
	if n == 0 {
		return t, nil
	}
	local := t.In(c.loc)
	d := c.dateOf(t)
	for i := 0; i < n; i++ {
		d = c.nextBusinessDay(d)
	}
	return time.Date(d.Year, d.Month, d.Day, local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), c.loc), nil
}

// WorkingDaysBetween counts the business days after from's date up to and
// including to's date. It is the inverse of AddWorkingDays for whole days. It is
// negative when to is before from.
func (c *Calendar) WorkingDaysBetween(from, to time.Time) (int, error) {
	a, b := c.dateOf(from), c.dateOf(to)
	sign := 1
	if c.midnight(b).Before(c.midnight(a)) {
		a, b, sign = b, a, -1
	}
	n := 0
	for d := a; d != b; {
		d = c.nextDate(d)
		if c.IsBusinessDay(d) {
			n++
		}
		if n > MaxWorkingDays*2 {
			return 0, fmt.Errorf("%w: span is too long", ErrRange)
		}
	}
	return sign * n, nil
}

// ElapsedBusinessTime returns how much of the span from..to falls on business
// days, in the calendar's time zone. Time on weekends and holidays is not
// counted. A span that ends before it starts is an error.
func (c *Calendar) ElapsedBusinessTime(from, to time.Time) (time.Duration, error) {
	if to.Before(from) {
		return 0, fmt.Errorf("%w: end is before start", ErrRange)
	}
	if to.Sub(from) > time.Duration(MaxWorkingDays)*24*time.Hour {
		return 0, fmt.Errorf("%w: span is too long", ErrRange)
	}
	var total time.Duration
	d := c.dateOf(from)
	last := c.dateOf(to)
	for {
		if c.IsBusinessDay(d) {
			start, end := c.midnight(d), c.midnight(c.nextDate(d))
			if from.After(start) {
				start = from
			}
			if to.Before(end) {
				end = to
			}
			if end.After(start) {
				total += end.Sub(start)
			}
		}
		if d == last {
			return total, nil
		}
		d = c.nextDate(d)
	}
}

func (c *Calendar) cutoff(rail string) (Cutoff, error) {
	co, ok := c.cutoffs[rail]
	if !ok {
		return Cutoff{}, fmt.Errorf("%w: %s", ErrNoCutoff, rail)
	}
	return co, nil
}

func (c *Calendar) cutoffInstant(d Date, co Cutoff) time.Time {
	return time.Date(d.Year, d.Month, d.Day, co.MinuteOfDay/60, co.MinuteOfDay%60, 0, 0, c.loc)
}

// NextCutoff returns the next cut-off for rail at or after t. If t falls on a
// business day before the cut-off, that is today's cut-off. Otherwise it is the
// cut-off of the next business day. A moment exactly at the cut-off has already
// reached it, so the next one is returned.
func (c *Calendar) NextCutoff(rail string, t time.Time) (time.Time, error) {
	co, err := c.cutoff(rail)
	if err != nil {
		return time.Time{}, err
	}
	d := c.dateOf(t)
	if c.IsBusinessDay(d) && t.Before(c.cutoffInstant(d, co)) {
		return c.cutoffInstant(d, co), nil
	}
	return c.cutoffInstant(c.nextBusinessDay(d), co), nil
}

// SettlementDate returns the business date on which work received at t settles
// on rail. Work received on a business day before the cut-off belongs to that
// day; anything later, or on a weekend or holiday, belongs to the next business
// day. The rail's lag in business days is then added.
func (c *Calendar) SettlementDate(rail string, t time.Time) (Date, error) {
	co, err := c.cutoff(rail)
	if err != nil {
		return Date{}, err
	}
	d := c.dateOf(t)
	if !c.IsBusinessDay(d) || !t.Before(c.cutoffInstant(d, co)) {
		d = c.nextBusinessDay(d)
	}
	for i := 0; i < co.LagDays; i++ {
		d = c.nextBusinessDay(d)
	}
	return d, nil
}
