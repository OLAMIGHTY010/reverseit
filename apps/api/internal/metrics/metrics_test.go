package metrics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

const tok = "acct_0123456789abcdef0123456789abcdef"

var day = time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

func at(h, m, s int) time.Time {
	return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second)
}

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

func (r *rig) exec(sql string, args ...any) {
	r.t.Helper()
	if _, err := r.pool.Exec(context.Background(), sql, args...); err != nil {
		r.t.Fatalf("%v\n%s", err, sql)
	}
}

// txn stores a transaction whose debit is at debit and whose last event happened at
// lastEvent and was received at lastReceived.
func (r *rig) txn(ref string, debit, lastEvent, lastReceived time.Time) uuid.UUID {
	r.t.Helper()
	var id uuid.UUID
	send := func(typ string, ts, recv time.Time, i int) {
		svc := &txn.Service{Now: func() time.Time { return recv.Add(time.Hour) }}
		err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			res, err := svc.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: fmt.Sprintf("EV-%s-%d", ref, i), EventType: typ, SourceTS: ts,
				Txn: txn.Core{InstructionRef: ref, Channel: "MOBILE", Rail: "NIP", AmountMinor: 100000, Currency: "NGN", DebitAccountToken: tok}})
			id = res.TxnID
			return err
		})
		if err != nil {
			r.t.Fatal(err)
		}
	}
	send(txn.EvDebitPosted, debit, debit, 1)
	send(txn.EvCreditFailed, lastEvent, lastReceived, 2)
	// Make received_at what the scenario says, bypassing the append-only trigger.
	tx, err := r.pool.Begin(context.Background())
	if err != nil {
		r.t.Fatal(err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	if _, err := tx.Exec(context.Background(), `SET LOCAL session_replication_role = replica`); err != nil {
		r.t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(), `UPDATE transaction_events SET received_at = source_ts + interval '1 second' WHERE tenant_id = $1 AND txn_id = $2 AND event_type = 'DEBIT_POSTED'`, r.tenant, id); err != nil {
		r.t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(), `UPDATE transaction_events SET received_at = $3 WHERE tenant_id = $1 AND txn_id = $2 AND event_type = 'CREDIT_FAILED'`, r.tenant, id, lastReceived); err != nil {
		r.t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		r.t.Fatal(err)
	}
	return id
}

type caseSpec struct {
	class, source, status, root string
	txn                         *uuid.UUID
	exposure                    int64
	opened                      time.Time
	sla, resolved               *time.Time
}

func tp(t time.Time) *time.Time { return &t }

func (r *rig) kase(c caseSpec) uuid.UUID {
	r.t.Helper()
	r.n++
	id := uuid.New()
	r.exec(`INSERT INTO cases (tenant_id, id, case_ref, classification, status, priority, source, txn_id, exposure_minor, currency, root_cause, opened_at, sla_due_at, resolved_at)
		VALUES ($1,$2,$3,$4,$5,'MEDIUM',$6,$7,$8,'NGN',$9,$10,$11,$12)`, r.tenant, id, fmt.Sprintf("CASE-M-%04d", r.n), c.class, c.status, c.source, c.txn, c.exposure, c.root, c.opened, c.sla, c.resolved)
	return id
}

func (r *rig) investigated(caseID uuid.UUID, when time.Time) {
	r.exec(`INSERT INTO case_events (tenant_id, case_id, seq, at, actor_type, actor_id, kind, from_status, to_status, reason_code)
		VALUES ($1,$2,1,$3,'user',$4,'transition','DETECTED','UNDER_INVESTIGATION','STARTED')`, r.tenant, caseID, when, uuid.New())
}

func (r *rig) reversal(caseID, txnID uuid.UUID, by string, approval string, financial string, amount int64, submitted, verified time.Time) {
	r.t.Helper()
	r.n++
	var requester any
	if by == "user" {
		requester = uuid.New()
	}
	r.exec(`INSERT INTO reversals (tenant_id, reversal_ref, case_id, txn_id, idempotency_key, request_hash, requested_by, requested_by_type, amount_minor, currency, reason_code,
		approval_status, accepted_status, processing_status, financial_status, gate_result, submitted_at, verified_at, created_at)
		VALUES ($1,$2,$3,$4,$5,'h',$6,$7,$8,'NGN','TEST',$9,'ACCEPTED','COMPLETED',$10,'{}',$11,$12,$13)`,
		r.tenant, fmt.Sprintf("RV-M-%04d", r.n), caseID, txnID, fmt.Sprintf("key-m-%04d", r.n), requester, by, amount, approval, financial, submitted, verified, submitted)
}

func (r *rig) contact(caseID uuid.UUID, n int) {
	for i := 0; i < n; i++ {
		rid := uuid.New()
		cust := uuid.New()
		r.exec(`INSERT INTO customers (tenant_id, id, customer_ref) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, r.tenant, cust, "cust_"+fmt.Sprintf("%024x", r.n*100+i))
		r.exec(`INSERT INTO customer_reports (tenant_id, id, customer_id, idempotency_key, consent_at, consent_version, status) VALUES ($1,$2,$3,$4,now(),'v1','submitted')`, r.tenant, rid, cust, "k-"+rid.String())
		r.exec(`INSERT INTO case_contacts (tenant_id, case_id, report_id, customer_id) VALUES ($1,$2,$3,$4)`, r.tenant, caseID, rid, cust)
	}
}

func (r *rig) compute(from, to time.Time) map[string]Value {
	var rep Report
	if err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		rep, err = Compute(context.Background(), tx, r.tenant, from, to)
		return err
	}); err != nil {
		r.t.Fatal(err)
	}
	out := map[string]Value{}
	for _, m := range rep.Metrics {
		out[m.Name] = m
	}
	return out
}

func near(a *float64, want float64) bool { return a != nil && math.Abs(*a-want) < 1e-6 }

// A seeded day whose every number is worked out by hand in the comments.
func seed(r *rig) {
	t1 := r.txn("INS-M1", at(9, 0, 0), at(9, 0, 30), at(9, 0, 31))
	t2 := r.txn("INS-M2", at(10, 0, 0), at(10, 0, 0), at(10, 0, 1))
	t3 := r.txn("INS-M3", at(11, 0, 0), at(11, 0, 50), at(11, 0, 51))
	t4 := r.txn("INS-M4", at(12, 0, 0), at(12, 0, 0), at(12, 0, 1))
	t5 := r.txn("INS-M5", at(13, 0, 0), at(13, 0, 20), at(13, 0, 21))
	// A: opened 30 s after its last event; first looked at 4 min later; resolved after 30 min, inside its deadline.
	a := r.kase(caseSpec{"FAIL001", "bot", "RESOLVED", "beneficiary_side_failure", &t1, 1_000_000, at(9, 1, 0), tp(at(10, 1, 0)), tp(at(9, 31, 0))})
	r.investigated(a, at(9, 5, 0))
	// B: 90 s detection, looked at after 10 min, resolved after 60 min, after its deadline.
	b := r.kase(caseSpec{"FAIL001", "bot", "RESOLVED", "beneficiary_side_failure", &t2, 2_000_000, at(10, 1, 30), tp(at(10, 31, 30)), tp(at(11, 1, 30))})
	r.investigated(b, at(10, 11, 30))
	// C: 10 s detection, closed as nothing wrong after 2 min, never looked at by a person.
	r.kase(caseSpec{"UNK001", "bot", "CLOSED_NO_EXCEPTION", "switch_response_failure", &t3, 500_000, at(11, 1, 0), nil, tp(at(11, 3, 0))})
	// D: reported by a customer on a transaction the detector did not raise; two contacts; open.
	d := r.kase(caseSpec{"CUS001", "customer", "DETECTED", "", &t4, 700_000, at(12, 5, 0), nil, nil})
	r.contact(d, 2)
	// E: duplicate found by the bank, open.
	r.kase(caseSpec{"DUP001", "bot", "DETECTED", "duplicate_retry", &t5, 300_000, at(13, 0, 25), nil, nil})
	// Two verified reversals: A's was requested by the system, B's by a person and approved.
	r.reversal(a, t1, "system", "NOT_REQUIRED", "VERIFIED", 1_000_000, at(9, 20, 0), at(9, 30, 0))
	r.reversal(b, t2, "user", "APPROVED", "VERIFIED", 2_000_000, at(10, 50, 0), at(11, 0, 0))
}

func TestEveryMetricMatchesTheHandComputedValueOnSeededData(t *testing.T) {
	r := newRig(t)
	seed(r)
	m := r.compute(day, day.Add(24*time.Hour))

	if v := m["exception_volume"]; v.Samples != 5 || v.Breakdown["FAIL001/bot"] != 2 || v.Breakdown["UNK001/bot"] != 1 || v.Breakdown["CUS001/customer"] != 1 || v.Breakdown["DUP001/bot"] != 1 {
		t.Fatalf("volume: %+v", v)
	}
	// Detection latencies are 30, 90, 10 and 5 seconds for the four bot cases (C: 11:01:00 - 11:00:50 = 10; E: 13:00:25 - 13:00:20 = 5).
	// Sorted 5, 10, 30, 90: p50 = 20; p95 = 30 + 0.85 * (90 - 30) = 81.
	if v := m["detection_latency"]; v.Samples != 4 || !near(v.P50, 20) || !near(v.P95, 81) {
		t.Fatalf("detection latency: %+v p50=%v p95=%v", v, v.P50, v.P95)
	}
	// Looked at after 240 and 600 seconds: p50 = 420, p95 = 240 + 0.95 * 360 = 582.
	if v := m["time_to_first_investigation"]; v.Samples != 2 || !near(v.P50, 420) || !near(v.P95, 582) {
		t.Fatalf("first investigation: %+v", v)
	}
	// Resolved after 1800, 3600 and 120 seconds, sorted 120, 1800, 3600: p50 = 1800, p95 = 1800 + 0.9 * 1800 = 3420.
	if v := m["resolution_time"]; v.Samples != 3 || !near(v.P50, 1800) || !near(v.P95, 3420) {
		t.Fatalf("resolution: %+v", v)
	}
	// Debit to verification: A 09:00 to 09:30 = 1800, B 10:00 to 11:00 = 3600: p50 = 2700.
	if v := m["customer_wait"]; v.Samples != 2 || !near(v.P50, 2700) || !near(v.P95, 1800+0.95*1800) {
		t.Fatalf("customer wait: %+v", v)
	}
	// Four of the five cases (A, B, C, E) were found by the bank with no customer involved: D is customer-reported.
	if v := m["proactive_detection_rate"]; v.Samples != 5 || !near(v.Value, 0.8) {
		t.Fatalf("proactive: %+v", v)
	}
	// A met its deadline, B did not: one of two. C has no deadline and is not counted.
	if v := m["sla_compliance"]; v.Samples != 2 || !near(v.Value, 0.5) {
		t.Fatalf("sla: %+v", v)
	}
	// Both reversals were created, accepted, completed and verified.
	if v := m["reversal_rates"]; v.Samples != 2 || !near(v.Value, 1) || v.Breakdown["financial_status.VERIFIED"] != 2 || v.Breakdown["failure_rate"] != 0 {
		t.Fatalf("reversal rates: %+v", v)
	}
	// Exposure in naira: flagged 1.0m + 2.0m + 0.5m + 0.7m + 0.3m; submitted and reversed 3.0m; still open D + E = 1.0m.
	if v := m["exposure"]; v.ByCurrency["NGN"]["gross_flagged"] != 4_500_000 || v.ByCurrency["NGN"]["submitted"] != 3_000_000 || v.ByCurrency["NGN"]["reversed"] != 3_000_000 || v.ByCurrency["NGN"]["still_open"] != 1_000_000 {
		t.Fatalf("exposure: %+v", v.ByCurrency)
	}
	// Of three finished bot cases, one was closed because nothing was wrong.
	if v := m["false_positive_rate"]; v.Samples != 3 || !near(v.Value, 1.0/3) {
		t.Fatalf("false positives: %+v", v)
	}
	// One of two verified reversals was automatic; one of two created needed a person.
	if v := m["automation"]; !near(v.Value, 0.5) || v.Breakdown["manual_intervention_rate"] != 0.5 || v.Breakdown["verified"] != 2 {
		t.Fatalf("automation: %+v", v)
	}
	// Only D was contacted about, and twice.
	if v := m["repeat_contact_rate"]; v.Samples != 1 || !near(v.Value, 1) {
		t.Fatalf("repeat: %+v", v)
	}
	if v := m["recurring_root_causes"]; v.Breakdown["beneficiary_side_failure"] != 2 || v.Breakdown["switch_response_failure"] != 1 || v.Breakdown["duplicate_retry"] != 1 {
		t.Fatalf("root causes: %+v", v)
	}
}

func TestAPeriodWithNothingInItGivesNullsNotZeros(t *testing.T) {
	r := newRig(t)
	seed(r)
	m := r.compute(day.AddDate(0, 0, 10), day.AddDate(0, 0, 11))
	for _, name := range []string{"detection_latency", "time_to_first_investigation", "resolution_time", "customer_wait"} {
		if v := m[name]; v.Samples != 0 || v.P50 != nil || v.P95 != nil || v.Note == "" {
			t.Errorf("%s: %+v", name, v)
		}
	}
	for _, name := range []string{"proactive_detection_rate", "sla_compliance", "false_positive_rate", "repeat_contact_rate"} {
		if v := m[name]; v.Samples != 0 || v.Value != nil {
			t.Errorf("%s: %+v", name, v)
		}
	}
	if v := m["exception_volume"]; v.Samples != 0 {
		t.Fatalf("%+v", v)
	}
}

func TestOneInstitutionsFiguresNeverIncludeAnothers(t *testing.T) {
	r := newRig(t)
	seed(r)
	other := &rig{pool: r.pool, tenant: dbtest.Tenant(t, r.pool), t: t}
	m := other.compute(day, day.Add(24*time.Hour))
	if v := m["exception_volume"]; v.Samples != 0 {
		t.Fatalf("%+v", v)
	}
	if v := m["exposure"]; len(v.ByCurrency) != 0 {
		t.Fatalf("%+v", v.ByCurrency)
	}
}

func TestPeriodsAreValidated(t *testing.T) {
	r := newRig(t)
	for name, p := range map[string][2]time.Time{"reversed": {day.Add(time.Hour), day}, "empty": {day, day}, "too long": {day, day.Add(MaxPeriod + time.Hour)}} {
		err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			_, err := Compute(context.Background(), tx, r.tenant, p[0], p[1])
			return err
		})
		if !errors.Is(err, ErrPeriod) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestEveryComputedMetricHasACatalogueEntryWithItsDefinition(t *testing.T) {
	r := newRig(t)
	m := r.compute(day, day.Add(time.Hour))
	have := map[string]Definition{}
	for _, d := range Catalogue {
		if d.Definition == "" || d.Formula == "" || len(d.Sources) == 0 || d.Period == "" || d.Limitations == "" {
			t.Errorf("%s has an incomplete entry", d.Name)
		}
		have[d.Name] = d
	}
	for name := range m {
		if _, ok := have[name]; !ok {
			t.Errorf("%s is computed but not in the catalogue", name)
		}
	}
	if len(m) != len(Catalogue) {
		t.Errorf("%d computed, %d catalogued", len(m), len(Catalogue))
	}
}
