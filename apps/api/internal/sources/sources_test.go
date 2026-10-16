package sources

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

var t0 = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

const tok = "acct_0123456789abcdef0123456789abcdef"

type rig struct {
	pool   *db.Pool
	tenant uuid.UUID
	n      int
	t      *testing.T
}

func newRig(t *testing.T) *rig {
	pool := dbtest.Pool(t)
	return &rig{pool: pool, tenant: dbtest.Tenant(t, pool), t: t}
}

// send stores one event from a source, received at the given time.
func (r *rig) send(source string, sourceTS, receivedAt time.Time) {
	r.t.Helper()
	r.n++
	svc := &txn.Service{Now: func() time.Time { return receivedAt }}
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		_, err := svc.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: source, SourceEventRef: "EV-" + source + "-" + uuid.NewString()[:8], EventType: txn.EvDebitPosted, SourceTS: sourceTS,
			Txn: txn.Core{InstructionRef: "INS-" + uuid.NewString()[:8], Channel: "MOBILE", Rail: "NIP", AmountMinor: 1000, Currency: "NGN", DebitAccountToken: tok}})
		return err
	})
	if err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) report(now time.Time) map[string]Status {
	r.t.Helper()
	out := map[string]Status{}
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		l, err := Report(context.Background(), tx, r.tenant, now)
		for _, s := range l {
			out[s.SourceSystem] = s
		}
		return err
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return out
}

func (r *rig) set(e Expectation) error {
	return r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error { return Set(context.Background(), tx, r.tenant, uuid.New(), e) })
}

func TestASourceWithNoExpectationIsUnconfiguredNeverFresh(t *testing.T) {
	r := newRig(t)
	r.send("SWITCH_A", t0, t0)
	s := r.report(t0.Add(time.Second))["SWITCH_A"]
	if s.Status != Unconfigured || s.Expectation != nil || s.Events != 1 {
		t.Fatalf("%+v", s)
	}
	if r.report(t0.Add(365 * 24 * time.Hour))["SWITCH_A"].Status != Unconfigured {
		t.Fatal("a source with no expectation changed status with time")
	}
}

func TestSilenceMovesASourceFromFreshToStaleToDownAtTheConfiguredBoundaries(t *testing.T) {
	r := newRig(t)
	r.send("SWITCH_A", t0, t0)
	if err := r.set(Expectation{SourceSystem: "SWITCH_A", StaleAfterSec: 300, DownAfterSec: 900, MaxSkewSec: 5}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		after time.Duration
		want  string
	}{{0, Fresh}, {299 * time.Second, Fresh}, {300 * time.Second, Stale}, {899 * time.Second, Stale}, {900 * time.Second, Down}, {24 * time.Hour, Down}} {
		if got := r.report(t0.Add(c.after))["SWITCH_A"]; got.Status != c.want {
			t.Errorf("after %s: %s, want %s", c.after, got.Status, c.want)
		}
	}
	// New traffic restores it.
	r.send("SWITCH_A", t0.Add(time.Hour), t0.Add(time.Hour))
	if got := r.report(t0.Add(time.Hour + time.Second))["SWITCH_A"]; got.Status != Fresh || got.SilenceSec != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestSilenceIsMeasuredFromReceiptNotFromTheTimeOnTheEvent(t *testing.T) {
	r := newRig(t)
	// A source replaying yesterday's events right now is talking, not silent.
	r.send("SWITCH_B", t0.Add(-24*time.Hour), t0)
	_ = r.set(Expectation{SourceSystem: "SWITCH_B", StaleAfterSec: 300, DownAfterSec: 900, MaxSkewSec: 5})
	if got := r.report(t0.Add(10 * time.Second))["SWITCH_B"]; got.Status != Fresh {
		t.Fatalf("%+v", got)
	}
}

func TestClockSkewIsMeasuredAndFlaggedAgainstTheExpectation(t *testing.T) {
	r := newRig(t)
	// Eleven events: ten arrive 2 seconds after they happened, one 60 seconds after.
	for i := 0; i < 10; i++ {
		at := t0.Add(time.Duration(i) * time.Minute)
		r.send("SWITCH_C", at, at.Add(2*time.Second))
	}
	r.send("SWITCH_C", t0.Add(20*time.Minute), t0.Add(20*time.Minute+60*time.Second))
	_ = r.set(Expectation{SourceSystem: "SWITCH_C", StaleAfterSec: 300, DownAfterSec: 900, MaxSkewSec: 30})
	s := r.report(t0.Add(21 * time.Minute))["SWITCH_C"]
	if s.SkewSamples != 11 || s.SkewP50Sec != 2 || s.SkewP95Sec != 60 || !s.SkewExceeded {
		t.Fatalf("%+v", s)
	}
	_ = r.set(Expectation{SourceSystem: "SWITCH_C", StaleAfterSec: 300, DownAfterSec: 900, MaxSkewSec: 120})
	if s := r.report(t0.Add(21 * time.Minute))["SWITCH_C"]; s.SkewExceeded {
		t.Fatalf("%+v", s)
	}
	// A source whose clock runs ahead has negative skew, and that is also drift.
	r.send("SWITCH_D", t0.Add(4*time.Minute), t0)
	_ = r.set(Expectation{SourceSystem: "SWITCH_D", StaleAfterSec: 300, DownAfterSec: 900, MaxSkewSec: 30})
	if s := r.report(t0.Add(time.Minute))["SWITCH_D"]; s.SkewP50Sec != -240 || !s.SkewExceeded {
		t.Fatalf("%+v", s)
	}
}

func TestExpectationsAreValidatedAndOnlyForSourcesThatHaveSpoken(t *testing.T) {
	r := newRig(t)
	r.send("SWITCH_A", t0, t0)
	for name, e := range map[string]Expectation{
		"no source":         {StaleAfterSec: 1, DownAfterSec: 2},
		"stale zero":        {SourceSystem: "SWITCH_A", StaleAfterSec: 0, DownAfterSec: 10},
		"down not after":    {SourceSystem: "SWITCH_A", StaleAfterSec: 10, DownAfterSec: 10},
		"down over a month": {SourceSystem: "SWITCH_A", StaleAfterSec: 10, DownAfterSec: 2592001},
		"negative skew":     {SourceSystem: "SWITCH_A", StaleAfterSec: 10, DownAfterSec: 20, MaxSkewSec: -1},
		"skew over a day":   {SourceSystem: "SWITCH_A", StaleAfterSec: 10, DownAfterSec: 20, MaxSkewSec: 86401},
		"never heard from":  {SourceSystem: "SWITCH_Z", StaleAfterSec: 10, DownAfterSec: 20},
	} {
		if err := r.set(e); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestOneInstitutionsSourcesAreInvisibleToAnother(t *testing.T) {
	r := newRig(t)
	r.send("SWITCH_A", t0, t0)
	other := &rig{pool: r.pool, tenant: dbtest.Tenant(t, r.pool), t: t}
	if got := other.report(t0); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	if err := other.set(Expectation{SourceSystem: "SWITCH_A", StaleAfterSec: 10, DownAfterSec: 20}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("%v", err)
	}
}

func TestPercentileUsesNearestRank(t *testing.T) {
	v := []float64{5, 1, 3, 2, 4}
	for p, want := range map[float64]float64{0.0: 1, 0.5: 3, 0.95: 5, 1.0: 5} {
		if got := percentile(v, p); got != want {
			t.Errorf("p%v = %v, want %v", p, got, want)
		}
	}
	if v[0] != 5 {
		t.Fatal("percentile sorted its input")
	}
}

func TestDegradedListsOnlyConfiguredSourcesThatAreStaleOrDown(t *testing.T) {
	r := newRig(t)
	r.send("QUIET_A", t0, t0)
	r.send("QUIET_B", t0, t0)
	r.send("NO_EXPECTATION", t0, t0)
	_ = r.set(Expectation{SourceSystem: "QUIET_A", StaleAfterSec: 300, DownAfterSec: 900, MaxSkewSec: 5})
	_ = r.set(Expectation{SourceSystem: "QUIET_B", StaleAfterSec: 60, DownAfterSec: 120, MaxSkewSec: 5})
	get := func(after time.Duration) map[string]string {
		var out map[string]string
		if err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			var err error
			out, err = Degraded(context.Background(), tx, r.tenant, t0.Add(after))
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := get(30 * time.Second); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	if got := get(90 * time.Second); len(got) != 1 || got["QUIET_B"] != Stale {
		t.Fatalf("%v", got)
	}
	got := get(time.Hour)
	if len(got) != 2 || got["QUIET_A"] != Down || got["QUIET_B"] != Down {
		t.Fatalf("%v", got)
	}
	if _, ok := got["NO_EXPECTATION"]; ok {
		t.Fatal("a source with no expectation was reported as degraded")
	}
}
