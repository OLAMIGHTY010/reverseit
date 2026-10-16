package incidents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/alerts"
	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/routing"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

const tok = "acct_0123456789abcdef0123456789abcdef"

type rig struct {
	pool   *db.Pool
	tenant uuid.UUID
	now    time.Time
	svc    *Service
	al     *alerts.Service
	cfg    *configsvc.Service
	n      int
	t      *testing.T
}

func newRig(t *testing.T) *rig {
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), now: time.Now().UTC().Truncate(time.Second), t: t}
	clk := func() time.Time { return r.now }
	app := &audit.Appender{Now: clk}
	r.cfg = &configsvc.Service{Audit: app, Now: clk}
	r.al = &alerts.Service{Routing: &routing.Service{Audit: app, Now: clk}, Config: r.cfg, Audit: app, Now: clk}
	r.svc = &Service{Config: r.cfg, Alerts: r.al, Audit: app, Now: clk}
	return r
}

func (r *rig) in(fn func(tx pgx.Tx) error) error {
	return r.pool.WithTenant(context.Background(), r.tenant, fn)
}

func (r *rig) must(err error) {
	r.t.Helper()
	if err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) setConfig(key string, v int) {
	raw, _ := json.Marshal(v)
	r.must(r.in(func(tx pgx.Tx) error {
		ver, err := r.cfg.Propose(context.Background(), tx, r.tenant, uuid.New(), configsvc.ProposeInput{Key: key, Value: raw, SourceReference: "SYNTHETIC DEMO POLICY - not approved for production",
			EffectiveFrom: r.now, VerifyStatus: "unverified", Reason: "synthetic value for a test"})
		if err != nil {
			return err
		}
		_, err = r.cfg.Decide(context.Background(), tx, r.tenant, uuid.New(), key, ver.Version, true, "")
		return err
	}))
}

func (r *rig) policy() {
	r.setConfig("incident.group_window_minutes", 30)
	r.setConfig("incident.min_group_size", 5)
	r.now = r.now.Add(time.Second)
}

// kase opens a case on a new transaction on a rail and channel.
func (r *rig) kase(rail, channel, root, priority string, opened time.Time) uuid.UUID {
	r.t.Helper()
	r.n++
	svc := &txn.Service{Now: func() time.Time { return r.now }}
	var tid uuid.UUID
	r.must(r.in(func(tx pgx.Tx) error {
		res, err := svc.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: fmt.Sprintf("EV-IN-%d", r.n), EventType: txn.EvDebitPosted, SourceTS: r.now.Add(-time.Minute),
			Txn: txn.Core{InstructionRef: fmt.Sprintf("INS-IN%d", r.n), Channel: channel, Rail: rail, AmountMinor: 100000, Currency: "NGN", DebitAccountToken: tok}})
		tid = res.TxnID
		return err
	}))
	id := uuid.New()
	if _, err := r.pool.Exec(context.Background(), `INSERT INTO cases (tenant_id, id, case_ref, classification, status, priority, source, txn_id, exposure_minor, currency, root_cause, opened_at)
		VALUES ($1,$2,$3,'UNK001','DETECTED',$4,'bot',$5,100000,'NGN',$6,$7)`, r.tenant, id, fmt.Sprintf("CASE-I-%04d", r.n), priority, tid, root, opened); err != nil {
		r.t.Fatal(err)
	}
	return id
}

func (r *rig) group() Result {
	r.t.Helper()
	var res Result
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		res, err = r.svc.Group(context.Background(), tx, r.tenant)
		return err
	}))
	return res
}

func (r *rig) open() []Incident {
	var out []Incident
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		out, err = r.svc.List(context.Background(), tx, r.tenant, "")
		return err
	}))
	return out
}

func (r *rig) alertCount(status string) int {
	var n int
	r.must(r.in(func(tx pgx.Tx) error {
		l, err := r.al.List(context.Background(), tx, r.tenant, status)
		n = len(l)
		return err
	}))
	return n
}

func TestATimeoutSpikeIsOneIncidentAndOneAlertNotHundreds(t *testing.T) {
	r := newRig(t)
	r.policy()
	for i := 0; i < 40; i++ {
		r.kase("NIP", "MOBILE", "switch_response_failure", "HIGH", r.now.Add(-time.Duration(i)*30*time.Second))
	}
	res := r.group()
	if res.Opened != 1 || res.Linked != 40 {
		t.Fatalf("%+v", res)
	}
	inc := r.open()
	if len(inc) != 1 || inc[0].AffectedCount != 40 || inc[0].Rail != "NIP" || inc[0].Channel != "MOBILE" || inc[0].Signature != "switch_response_failure" ||
		inc[0].Severity != "HIGH" || inc[0].Exposure["NGN"] != 4_000_000 || inc[0].CausationStatus != "correlated" || inc[0].Status != "open" {
		t.Fatalf("%+v", inc)
	}
	// One alert for the incident.
	if r.alertCount("open") != 1 {
		t.Fatalf("%d alerts", r.alertCount("open"))
	}
	// The cases' own alerts are suppressed under the incident, not paged.
	r.must(r.in(func(tx pgx.Tx) error {
		_, err := r.al.Sweep(context.Background(), tx, r.tenant, Covering())
		return err
	}))
	if r.alertCount("open") != 1 || r.alertCount("suppressed") != 40 {
		t.Fatalf("open %d suppressed %d", r.alertCount("open"), r.alertCount("suppressed"))
	}
	// Running again changes nothing.
	again := r.group()
	if again.Opened != 0 || again.Linked != 0 || len(r.open()) != 1 || r.alertCount("open") != 1 {
		t.Fatalf("%+v", again)
	}
}

func TestOnlyExceptionsThatShareARailChannelAndSignatureAreGroupedAndOnlyAtTheApprovedSize(t *testing.T) {
	r := newRig(t)
	r.policy()
	for i := 0; i < 4; i++ { // one short of the approved size
		r.kase("NIP", "MOBILE", "switch_response_failure", "MEDIUM", r.now.Add(-time.Minute))
	}
	for i := 0; i < 5; i++ { // same signature, other channel: its own group, which reaches the size
		r.kase("NIP", "USSD", "switch_response_failure", "MEDIUM", r.now.Add(-time.Minute))
	}
	for i := 0; i < 3; i++ {
		r.kase("CARD", "MOBILE", "switch_response_failure", "MEDIUM", r.now.Add(-time.Minute))
	}
	for i := 0; i < 4; i++ { // same rail and channel, other signature
		r.kase("NIP", "MOBILE", "beneficiary_side_failure", "MEDIUM", r.now.Add(-time.Minute))
	}
	r.group()
	inc := r.open()
	if len(inc) != 1 || inc[0].Channel != "USSD" || inc[0].AffectedCount != 5 {
		t.Fatalf("%+v", inc)
	}
	// Cases older than the window are not grouped.
	r2 := newRig(t)
	r2.policy()
	for i := 0; i < 6; i++ {
		r2.kase("NIP", "MOBILE", "switch_response_failure", "MEDIUM", r2.now.Add(-2*time.Hour))
	}
	if res := r2.group(); res.Opened != 0 || len(r2.open()) != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestAnIncidentKeepsGrowingEvenWhenALaterBurstIsSmall(t *testing.T) {
	r := newRig(t)
	r.policy()
	for i := 0; i < 5; i++ {
		r.kase("NIP", "MOBILE", "switch_response_failure", "MEDIUM", r.now.Add(-time.Minute))
	}
	r.group()
	// Later, only two more arrive: fewer than the size, but there is an open incident of that scope.
	r.now = r.now.Add(10 * time.Minute)
	r.kase("NIP", "MOBILE", "switch_response_failure", "CRITICAL", r.now.Add(-time.Minute))
	r.kase("NIP", "MOBILE", "switch_response_failure", "MEDIUM", r.now.Add(-30*time.Second))
	res := r.group()
	inc := r.open()
	if res.Opened != 0 || res.Updated != 1 || len(inc) != 1 || inc[0].AffectedCount != 7 || inc[0].Severity != "CRITICAL" {
		t.Fatalf("%+v %+v", res, inc)
	}
}

func TestNothingIsGroupedUntilTheWindowAndSizeAreApproved(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 20; i++ {
		r.kase("NIP", "MOBILE", "switch_response_failure", "HIGH", r.now.Add(-time.Minute))
	}
	if res := r.group(); res.Skipped == "" || res.Opened != 0 {
		t.Fatalf("%+v", res)
	}
	r.setConfig("incident.group_window_minutes", 30)
	r.now = r.now.Add(time.Second)
	if res := r.group(); res.Skipped == "" || res.Opened != 0 {
		t.Fatalf("one of the two numbers is still missing: %+v", res)
	}
	r.setConfig("incident.min_group_size", 5)
	r.now = r.now.Add(time.Second)
	if res := r.group(); res.Opened != 1 {
		t.Fatalf("%+v", res)
	}
}

func TestCausationOnlyMovesUpAndResolvingNeedsAReviewNote(t *testing.T) {
	r := newRig(t)
	r.policy()
	for i := 0; i < 5; i++ {
		r.kase("NIP", "MOBILE", "switch_response_failure", "MEDIUM", r.now.Add(-time.Minute))
	}
	r.group()
	id := r.open()[0].ID
	by := uuid.New()
	call := func(fn func(tx pgx.Tx) error) error { return r.in(fn) }
	if err := call(func(tx pgx.Tx) error {
		_, err := r.svc.SetCausation(context.Background(), tx, r.tenant, by, id, "probable", "SWITCH_A degraded", "x")
		return err
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a note that says nothing: %v", err)
	}
	if err := call(func(tx pgx.Tx) error {
		_, err := r.svc.SetCausation(context.Background(), tx, r.tenant, by, id, "guess", "x", "enough words")
		return err
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("%v", err)
	}
	var got Incident
	if err := call(func(tx pgx.Tx) error {
		var err error
		got, err = r.svc.SetCausation(context.Background(), tx, r.tenant, by, id, "confirmed", "SWITCH_A degraded", "the switch operator confirmed an outage")
		return err
	}); err != nil || got.CausationStatus != "confirmed" || got.SuspectedCause != "SWITCH_A degraded" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := call(func(tx pgx.Tx) error {
		_, err := r.svc.SetCausation(context.Background(), tx, r.tenant, by, id, "probable", "", "changed my mind")
		return err
	}); !errors.Is(err, ErrState) {
		t.Fatalf("a confirmed cause was downgraded: %v", err)
	}
	if err := call(func(tx pgx.Tx) error {
		_, err := r.svc.Resolve(context.Background(), tx, r.tenant, by, id, "short")
		return err
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("%v", err)
	}
	if err := call(func(tx pgx.Tx) error {
		var err error
		got, err = r.svc.Resolve(context.Background(), tx, r.tenant, by, id, "The switch recovered at 14:10; no money was lost; add a freshness check.")
		return err
	}); err != nil || got.Status != "resolved" || got.ReviewNote == "" || got.ResolvedAt == nil {
		t.Fatalf("%+v %v", got, err)
	}
	if r.alertCount("open") != 0 {
		t.Fatal("the incident's alert is still open")
	}
	if err := call(func(tx pgx.Tx) error {
		_, err := r.svc.Resolve(context.Background(), tx, r.tenant, by, id, "resolved a second time please")
		return err
	}); !errors.Is(err, ErrState) {
		t.Fatalf("%v", err)
	}
	// A new burst after resolution is a new incident.
	for i := 0; i < 5; i++ {
		r.kase("NIP", "MOBILE", "switch_response_failure", "MEDIUM", r.now.Add(-time.Second))
	}
	res := r.group()
	open := r.open()
	if res.Opened != 1 || len(open) != 1 || open[0].ID == id || open[0].AffectedCount != 5 {
		// The earlier cases stay linked to the resolved incident; only the new ones join the new one.
		t.Fatalf("%+v %+v", res, open)
	}
}

func TestIncidentsAreTenantScopedAndCasesAreReachableFromThem(t *testing.T) {
	r := newRig(t)
	r.policy()
	var ids []uuid.UUID
	for i := 0; i < 5; i++ {
		ids = append(ids, r.kase("NIP", "MOBILE", "switch_response_failure", "MEDIUM", r.now.Add(-time.Minute)))
	}
	r.group()
	inc := r.open()[0]
	var got []uuid.UUID
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		got, err = r.svc.CaseIDs(context.Background(), tx, r.tenant, inc.ID)
		return err
	}))
	if len(got) != 5 {
		t.Fatalf("%d", len(got))
	}
	var gotRef Incident
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		gotRef, err = r.svc.Get(context.Background(), tx, r.tenant, inc.Ref)
		return err
	}))
	if gotRef.ID != inc.ID {
		t.Fatal("lookup by reference")
	}
	other := dbtest.Tenant(t, r.pool)
	err := r.pool.WithTenant(context.Background(), other, func(tx pgx.Tx) error { _, err := r.svc.Get(context.Background(), tx, other, inc.Ref); return err })
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
	_ = ids
}
