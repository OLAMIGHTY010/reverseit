package alerts

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/sources"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

const tok = "acct_0123456789abcdef0123456789abcdef"

func (r *rig) exec(sql string, args ...any) {
	r.t.Helper()
	if _, err := r.pool.Exec(context.Background(), sql, args...); err != nil {
		r.t.Fatalf("%v\n%s", err, sql)
	}
}

func (r *rig) txn(ref string) uuid.UUID {
	r.t.Helper()
	var id uuid.UUID
	svc := &txn.Service{Now: func() time.Time { return r.now }}
	r.must(r.in(func(tx pgx.Tx) error {
		res, err := svc.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: "EV-" + ref, EventType: txn.EvDebitPosted, SourceTS: r.now.Add(-time.Minute),
			Txn: txn.Core{InstructionRef: ref, Channel: "MOBILE", Rail: "NIP", AmountMinor: 5000000, Currency: "NGN", DebitAccountToken: tok}})
		id = res.TxnID
		return err
	}))
	return id
}

func (r *rig) kase(priority, status string, opened time.Time, due *time.Time, exposure int64) (uuid.UUID, string) {
	r.n++
	id := uuid.New()
	ref := fmt.Sprintf("CASE-A-%04d", r.n)
	r.exec(`INSERT INTO cases (tenant_id, id, case_ref, classification, status, priority, source, exposure_minor, currency, opened_at, sla_due_at)
		VALUES ($1,$2,$3,'FAIL001',$4,$5,'bot',$6,'NGN',$7,$8)`, r.tenant, id, ref, status, priority, exposure, opened, due)
	return id, ref
}

func (r *rig) sweep(covered Covered) SweepResult {
	r.t.Helper()
	var res SweepResult
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		res, err = r.svc.Sweep(context.Background(), tx, r.tenant, covered)
		return err
	}))
	return res
}

func (r *rig) kinds(status string) map[string]int {
	list, _ := r.list(status)
	out := map[string]int{}
	for _, a := range list {
		out[a.Kind]++
	}
	return out
}

func TestACriticalCaseIsOneAlertAcrossSweepsAndClearsWhenTheCaseDoes(t *testing.T) {
	r := newRig(t)
	high, _ := r.kase("HIGH", "DETECTED", r.now.Add(-time.Hour), nil, 5000000)
	r.kase("MEDIUM", "DETECTED", r.now.Add(-time.Hour), nil, 100)
	r.kase("HIGH", "RESOLVED", r.now.Add(-time.Hour), nil, 100) // finished: no alert

	first := r.sweep(nil)
	if first.Created != 1 || r.kinds(Open)[CriticalException] != 1 {
		t.Fatalf("%+v %v", first, r.kinds(Open))
	}
	again := r.sweep(nil)
	if again.Created != 0 || again.Repeated != 1 {
		t.Fatalf("%+v", again)
	}
	list, _ := r.list(Open)
	if list[0].Body == "" || list[0].SubjectType != "case" {
		t.Fatalf("%+v", list[0])
	}
	r.exec(`UPDATE cases SET status = 'RESOLVED' WHERE id = $1`, high)
	if done := r.sweep(nil); done.Resolved != 1 || len(r.kinds(Open)) != 0 {
		t.Fatalf("%+v %v", done, r.kinds(Open))
	}
}

func TestApprovalRequestsReversalFailuresAndLapsedApprovalsAreAlertsWhileTheyAreTrue(t *testing.T) {
	r := newRig(t)
	cid, _ := r.kase("MEDIUM", "AWAITING_APPROVAL", r.now.Add(-time.Hour), nil, 5000000)
	tid := r.txn("INS-AL1")
	r.exec(`INSERT INTO reversals (tenant_id, id, reversal_ref, case_id, txn_id, idempotency_key, request_hash, requested_by_type, amount_minor, currency, reason_code, approval_status, gate_result)
		VALUES ($1,$2,'RV-AL-1',$3,$4,'k-al-1','h','system',5000000,'NGN','T','PENDING','{}')`, r.tenant, uuid.New(), cid, tid)
	var rev uuid.UUID
	_ = r.pool.QueryRow(context.Background(), `SELECT id FROM reversals WHERE reversal_ref = 'RV-AL-1'`).Scan(&rev)
	r.exec(`INSERT INTO approvals (tenant_id, reversal_id, maker_type, reason) VALUES ($1,$2,'system','test')`, r.tenant, rev)

	r.sweep(nil)
	if k := r.kinds(Open); k[ApprovalRequest] != 1 {
		t.Fatalf("%v", k)
	}
	// Once decided, the alert clears.
	r.exec(`UPDATE approvals SET status = 'APPROVED', checker = $2, decided_at = now(), expires_at = now() + interval '1 hour', policy_id = 'p', policy_version = 'v' WHERE reversal_id = $1`, rev, uuid.New())
	r.exec(`UPDATE reversals SET approval_status = 'APPROVED' WHERE id = $1`, rev)
	r.sweep(nil)
	if k := r.kinds(Open); k[ApprovalRequest] != 0 {
		t.Fatalf("%v", k)
	}
	// A lapsed approval and a failed reversal each raise their own alert.
	r.exec(`SET session_replication_role = replica`)
	r.exec(`UPDATE reversals SET approval_status = 'EXPIRED' WHERE id = $1`, rev)
	r.sweep(nil)
	if k := r.kinds(Open); k[ApprovalExpired] != 1 {
		t.Fatalf("%v", k)
	}
	r.exec(`UPDATE reversals SET approval_status = 'APPROVED', financial_status = 'FAILED' WHERE id = $1`, rev)
	r.sweep(nil)
	if k := r.kinds(Open); k[ReversalFailure] != 1 || k[ApprovalExpired] != 0 {
		t.Fatalf("%v", k)
	}
}

func TestADownSourceIsAnOutageAlertForIntegrationAndClearsWhenItReports(t *testing.T) {
	r := newRig(t)
	svc := &txn.Service{Now: func() time.Time { return r.now }}
	r.must(r.in(func(tx pgx.Tx) error {
		_, err := svc.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: "EV-OUT-1", EventType: txn.EvDebitPosted, SourceTS: r.now,
			Txn: txn.Core{InstructionRef: "INS-OUT1", Channel: "MOBILE", Rail: "NIP", AmountMinor: 1000, Currency: "NGN", DebitAccountToken: tok}})
		return err
	}))
	r.must(r.in(func(tx pgx.Tx) error {
		return sources.Set(context.Background(), tx, r.tenant, uuid.New(), sources.Expectation{SourceSystem: "SWITCH_A", StaleAfterSec: 60, DownAfterSec: 300, MaxSkewSec: 30})
	}))
	r.sweep(nil)
	if k := r.kinds(Open); k[IntegrationOutage] != 0 {
		t.Fatalf("a fresh source raised an outage: %v", k)
	}
	r.now = r.now.Add(10 * time.Minute)
	r.sweep(nil)
	list, _ := r.list(Open)
	if len(list) != 1 || list[0].Kind != IntegrationOutage || list[0].TargetRole != "ADMIN_INTEGRATION" {
		t.Fatalf("%+v", list)
	}
	r.must(r.in(func(tx pgx.Tx) error {
		_, err := svc.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: "EV-OUT-2", EventType: txn.EvTransferSent, SourceTS: r.now,
			Txn: txn.Core{InstructionRef: "INS-OUT1", Channel: "MOBILE", Rail: "NIP", AmountMinor: 1000, Currency: "NGN", DebitAccountToken: tok}})
		return err
	}))
	if done := r.sweep(nil); done.Resolved != 1 {
		t.Fatalf("%+v", done)
	}
}

func TestDeadlineAndHighValueAlertsNeedApprovedThresholdsExceptABreach(t *testing.T) {
	r := newRig(t)
	opened := r.now.Add(-100 * time.Minute)
	due := r.now.Add(100 * time.Minute)
	_, nearRef := r.kase("MEDIUM", "UNDER_INVESTIGATION", opened, &due, 90_000_000)
	past := r.now.Add(-time.Minute)
	_, lateRef := r.kase("MEDIUM", "UNDER_INVESTIGATION", opened, &past, 100)
	r.sweep(nil)
	k := r.kinds(Open)
	if k[SLABreached] != 1 || k[SLAApproaching] != 0 || k[HighValueUnresolved] != 0 {
		t.Fatalf("without approved thresholds only a breach alerts: %v", k)
	}
	list, _ := r.list(Open)
	if list[0].SubjectRef != lateRef {
		t.Fatalf("%+v", list[0])
	}
	// 100 of 200 minutes used is 50 percent.
	r.setConfig("deadline.approaching_percent", 50)
	r.setConfig("alert.high_value_minor", 50_000_000)
	r.now = r.now.Add(time.Minute)
	r.sweep(nil)
	if k := r.kinds(Open); k[SLAApproaching] != 1 || k[HighValueUnresolved] != 0 {
		t.Fatalf("high value needs its age too: %v", k)
	}
	r.setConfig("alert.high_value_age_minutes", 60)
	r.sweep(nil)
	k = r.kinds(Open)
	if k[HighValueUnresolved] != 1 {
		t.Fatalf("%v", k)
	}
	for _, a := range func() []Alert { l, _ := r.list(Open); return l }() {
		if a.Kind == HighValueUnresolved && (a.SubjectRef != nearRef || a.TargetRole != "HEAD_OPS") {
			t.Fatalf("%+v", a)
		}
	}
}

func TestAKnownIncidentSuppressesTheAlertsForItsCases(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 5; i++ {
		r.kase("HIGH", "DETECTED", r.now.Add(-time.Hour), nil, 1000)
	}
	incident := uuid.New()
	covered := func(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID) (*uuid.UUID, error) {
		return &incident, nil
	}
	res := r.sweep(covered)
	if res.Suppressed != 5 || res.Created != 0 || len(r.kinds(Open)) != 0 || r.kinds(Suppressed)[CriticalException] != 5 {
		t.Fatalf("%+v open %v suppressed %v", res, r.kinds(Open), r.kinds(Suppressed))
	}
}
