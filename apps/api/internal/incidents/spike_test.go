package incidents

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

var wat = time.FixedZone("WAT", 3600)

// kaseAt stores a transaction and a case that both happened at the given time.
func (r *rig) kaseAt(rail, channel string, at time.Time) {
	r.t.Helper()
	r.n++
	svc := &txn.Service{Now: func() time.Time { return at.Add(time.Minute) }}
	var tid uuid.UUID
	r.must(r.in(func(tx pgx.Tx) error {
		res, err := svc.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: fmt.Sprintf("EV-SP-%d", r.n), EventType: txn.EvDebitPosted, SourceTS: at,
			Txn: txn.Core{InstructionRef: fmt.Sprintf("INS-SP%d", r.n), Channel: channel, Rail: rail, AmountMinor: 100000, Currency: "NGN", DebitAccountToken: tok}})
		tid = res.TxnID
		return err
	}))
	if _, err := r.pool.Exec(context.Background(), `INSERT INTO cases (tenant_id, id, case_ref, classification, status, priority, source, txn_id, opened_at)
		VALUES ($1,$2,$3,'UNK001','DETECTED','MEDIUM','bot',$4,$5)`, r.tenant, uuid.New(), fmt.Sprintf("CASE-SP-%04d", r.n), tid, at); err != nil {
		r.t.Fatal(err)
	}
}

// spikeRig is a Wednesday morning with four earlier Wednesdays that had 2, 3, 2 and 3
// exceptions in the same hour: mean 2.5, population standard deviation 0.5.
func spikeRig(t *testing.T, current int) *rig {
	r := newRig(t)
	r.now = time.Date(2026, 10, 14, 10, 30, 0, 0, wat).UTC()
	for k, n := range []int{2, 3, 2, 3} {
		end := r.now.AddDate(0, 0, -7*(k+1))
		for i := 0; i < n; i++ {
			r.kaseAt("NIP", "MOBILE", end.Add(-time.Duration(i+1)*5*time.Minute))
		}
	}
	for i := 0; i < current; i++ {
		r.kaseAt("NIP", "MOBILE", r.now.Add(-time.Duration(i+1)*time.Minute))
	}
	return r
}

func (r *rig) spikePolicy(sigmaTenths, minWeeks, minCount int) {
	r.setConfig("incident.spike_sigma_tenths", sigmaTenths)
	r.setConfig("incident.spike_min_weeks", minWeeks)
	r.setConfig("incident.spike_min_count", minCount)
	r.now = r.now.Add(time.Second)
}

func (r *rig) spikes() SpikeResult {
	r.t.Helper()
	var res SpikeResult
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		res, err = r.svc.DetectSpikes(context.Background(), tx, r.tenant)
		return err
	}))
	return res
}

func TestAnHourFarAboveTheSameHourOfEarlierWeeksIsASpikeAndRaisesOneAlert(t *testing.T) {
	r := spikeRig(t, 12)
	r.spikePolicy(30, 4, 5)
	res := r.spikes()
	if len(res.Spikes) != 1 || res.Skipped != "" {
		t.Fatalf("%+v", res)
	}
	sp := res.Spikes[0]
	if sp.Rail != "NIP" || sp.Channel != "MOBILE" || sp.Count != 12 || sp.Weeks != 4 || sp.Mean != 2.5 || sp.StdDev != 0.5 || sp.Threshold != 4.0 {
		t.Fatalf("%+v", sp)
	}
	if r.alertCount("open") != 1 {
		t.Fatalf("%d alerts", r.alertCount("open"))
	}
	r.spikes() // again, same hour
	if r.alertCount("open") != 1 {
		t.Fatalf("a repeated check made %d alerts", r.alertCount("open"))
	}
	list := func() string {
		var body string
		_ = r.in(func(tx pgx.Tx) error {
			l, _ := r.al.List(context.Background(), tx, r.tenant, "open")
			body = l[0].Body
			return nil
		})
		return body
	}()
	if !strings.Contains(list, "12 exceptions") || !strings.Contains(list, "2.5") || !strings.Contains(list, "4 earlier weeks") {
		t.Fatalf("%s", list)
	}
}

func TestAnOrdinaryHourIsNotASpike(t *testing.T) {
	for _, n := range []int{3, 4} { // 4 equals the threshold of 4.0: not above it
		r := spikeRig(t, n)
		r.spikePolicy(30, 4, 3)
		if res := r.spikes(); len(res.Spikes) != 0 {
			t.Fatalf("%d exceptions: %+v", n, res)
		}
	}
	// Above the threshold but below the approved minimum count: still not called.
	r := spikeRig(t, 5)
	r.spikePolicy(30, 4, 6)
	if res := r.spikes(); len(res.Spikes) != 0 {
		t.Fatalf("%+v", res)
	}
	r2 := spikeRig(t, 5)
	r2.spikePolicy(30, 4, 5)
	if res := r2.spikes(); len(res.Spikes) != 1 {
		t.Fatalf("5 is above 4.0 and at the minimum: %+v", res)
	}
}

func TestNothingIsJudgedWithoutApprovedNumbersEnoughHistoryOrOnANonBusinessDay(t *testing.T) {
	r := spikeRig(t, 12)
	if res := r.spikes(); res.Skipped == "" || len(res.Spikes) != 0 {
		t.Fatalf("%+v", res)
	}
	r.spikePolicy(30, 6, 5) // six weeks are wanted and four exist
	if res := r.spikes(); res.Skipped == "" || len(res.Spikes) != 0 || !strings.Contains(res.Skipped, "comparable weeks") {
		t.Fatalf("%+v", res)
	}
	// A holiday on one of the four earlier Wednesdays removes that week from the baseline.
	h := spikeRig(t, 12)
	h.spikePolicy(30, 4, 5)
	holiday := h.now.AddDate(0, 0, -14).In(wat).Format("2006-01-02")
	h.setConfig("calendar.holiday."+holiday, 1)
	h.now = h.now.Add(time.Second)
	if res := h.spikes(); res.Skipped == "" {
		t.Fatalf("a holiday week still counted towards the baseline: %+v", res)
	}
	// Today a holiday: no judgement.
	d := spikeRig(t, 12)
	d.spikePolicy(30, 4, 5)
	d.setConfig("calendar.holiday."+d.now.In(wat).Format("2006-01-02"), 1)
	d.now = d.now.Add(time.Second)
	if res := d.spikes(); !strings.Contains(res.Skipped, "not a business day") {
		t.Fatalf("%+v", res)
	}
	// A Saturday is not a business day either.
	s := spikeRig(t, 12)
	s.spikePolicy(30, 4, 5)
	s.now = time.Date(2026, 10, 17, 10, 30, 0, 0, wat).UTC()
	if res := s.spikes(); !strings.Contains(res.Skipped, "not a business day") {
		t.Fatalf("%+v", res)
	}
}
