package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/routing"
)

type rig struct {
	pool   *db.Pool
	tenant uuid.UUID
	now    time.Time
	svc    *Service
	rt     *routing.Service
	cfg    *configsvc.Service
	users  *identity.PGUserStore
	n      int
	t      *testing.T
}

func newRig(t *testing.T) *rig {
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), now: time.Now().UTC().Truncate(time.Second), users: &identity.PGUserStore{Pool: pool}, t: t}
	clk := func() time.Time { return r.now }
	app := &audit.Appender{Now: clk}
	r.rt = &routing.Service{Audit: app, Now: clk}
	r.cfg = &configsvc.Service{Audit: app, Now: clk}
	r.svc = &Service{Routing: r.rt, Config: r.cfg, Audit: app, Now: clk}
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

func (r *rig) user(role string) uuid.UUID {
	r.n++
	id, err := r.users.Create(context.Background(), r.tenant, fmt.Sprintf("alert%d@example.test", r.n), "User", "a long enough passphrase", []string{role})
	r.must(err)
	return id
}

func (r *rig) onCall(role string, u uuid.UUID) {
	r.must(r.in(func(tx pgx.Tx) error {
		_, err := r.rt.AddShift(context.Background(), tx, r.tenant, u, role, u, r.now.Add(-time.Hour), r.now.Add(8*time.Hour))
		return err
	}))
}

func (r *rig) raise(in Input) (Alert, string) {
	r.t.Helper()
	var a Alert
	var out string
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		a, out, err = r.svc.Raise(context.Background(), tx, r.tenant, in)
		return err
	}))
	return a, out
}

func in(key string) Input {
	return Input{Kind: CriticalException, Severity: "HIGH", DedupeKey: key, SubjectType: "case", SubjectRef: "CASE-2026-000001", Body: "Duplicate Transaction. Reference: NIP202610001234. Amount: NGN 50,000.", TargetRole: "SUPERVISOR"}
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

func TestAConditionThatIsStillTrueIsOneAlertWithACountNotAFlood(t *testing.T) {
	r := newRig(t)
	sup := r.user("SUPERVISOR")
	r.onCall("SUPERVISOR", sup)
	first, outcome := r.raise(in("case:1:critical"))
	if outcome != Created || first.Status != Open || first.Occurrences != 1 || len(first.RoutedTo) != 1 || first.RoutedTo[0] != sup || first.Unrouted {
		t.Fatalf("%s %+v", outcome, first)
	}
	for i := 0; i < 9; i++ {
		a, outcome := r.raise(in("case:1:critical"))
		if outcome != Repeated || a.ID != first.ID {
			t.Fatalf("%s", outcome)
		}
	}
	var n, occ int
	_ = r.pool.QueryRow(context.Background(), `SELECT count(*), max(occurrences) FROM alerts WHERE tenant_id = $1`, r.tenant).Scan(&n, &occ)
	if n != 1 || occ != 10 {
		t.Fatalf("%d alerts, %d occurrences", n, occ)
	}
	// Once resolved, the next occurrence is a new alert.
	r.must(r.in(func(tx pgx.Tx) error { return r.svc.Resolve(context.Background(), tx, r.tenant, "case:1:critical") }))
	if again, outcome := r.raise(in("case:1:critical")); outcome != Created || again.ID == first.ID {
		t.Fatalf("%s", outcome)
	}
}

func TestConcurrentRaisesOfOneConditionMakeOneAlert(t *testing.T) {
	r := newRig(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.in(func(tx pgx.Tx) error {
				_, _, err := r.svc.Raise(context.Background(), tx, r.tenant, in("case:2:critical"))
				return err
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var n, occ int
	_ = r.pool.QueryRow(context.Background(), `SELECT count(*), max(occurrences) FROM alerts WHERE tenant_id = $1`, r.tenant).Scan(&n, &occ)
	if n != 1 || occ != 8 {
		t.Fatalf("%d alerts, %d occurrences", n, occ)
	}
}

func TestAnAlertCoveredByAKnownIncidentIsRecordedAsSuppressedAndPagesNoOne(t *testing.T) {
	r := newRig(t)
	sup := r.user("SUPERVISOR")
	r.onCall("SUPERVISOR", sup)
	incident := uuid.New()
	a := in("case:3:critical")
	a.IncidentID = &incident
	got, outcome := r.raise(a)
	if outcome != Suppression || got.Status != Suppressed || len(got.RoutedTo) != 0 || got.SuppressedBy == nil || *got.SuppressedBy != incident {
		t.Fatalf("%s %+v", outcome, got)
	}
	if open, _ := r.list(Open); len(open) != 0 {
		t.Fatalf("a suppressed alert is in the open list: %+v", open)
	}
	if sup, _ := r.list(Suppressed); len(sup) != 1 {
		t.Fatal("the suppressed alert is not kept")
	}
}

func (r *rig) list(status string) ([]Alert, error) {
	var out []Alert
	err := r.in(func(tx pgx.Tx) error {
		var err error
		out, err = r.svc.List(context.Background(), tx, r.tenant, status)
		return err
	})
	return out, err
}

func TestRoutingFollowsOnCallThenEscalationAndNeverLosesTheAlert(t *testing.T) {
	r := newRig(t)
	head := r.user("HEAD_OPS")
	// Nobody is on call for the role, and no escalation is set: the alert is visible and marked unrouted.
	a, _ := r.raise(in("case:4:critical"))
	if !a.Unrouted || len(a.RoutedTo) != 0 || a.Status != Open {
		t.Fatalf("%+v", a)
	}
	// With an escalation step to a role that has someone on call, the alert goes there.
	r.must(r.in(func(tx pgx.Tx) error {
		return r.rt.SetEscalation(context.Background(), tx, r.tenant, uuid.New(), "SUPERVISOR", "HEAD_OPS")
	}))
	r.onCall("HEAD_OPS", head)
	b, _ := r.raise(in("case:5:critical"))
	if b.Unrouted || len(b.RoutedTo) != 1 || b.RoutedTo[0] != head {
		t.Fatalf("%+v", b)
	}
	if _, _, err := r.raiseErr(Input{Kind: "nope", Severity: "HIGH", DedupeKey: "k", Body: "b", TargetRole: "SUPERVISOR"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("%v", err)
	}
	for name, bad := range map[string]Input{
		"severity": {Kind: CriticalException, Severity: "SEVERE", DedupeKey: "k", Body: "b", TargetRole: "SUPERVISOR"},
		"no key":   {Kind: CriticalException, Severity: "HIGH", Body: "b", TargetRole: "SUPERVISOR"},
		"no body":  {Kind: CriticalException, Severity: "HIGH", DedupeKey: "k", TargetRole: "SUPERVISOR"},
		"no role":  {Kind: CriticalException, Severity: "HIGH", DedupeKey: "k", Body: "b"},
	} {
		if _, _, err := r.raiseErr(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func (r *rig) raiseErr(in Input) (Alert, string, error) {
	var a Alert
	var out string
	err := r.in(func(tx pgx.Tx) error {
		var err error
		a, out, err = r.svc.Raise(context.Background(), tx, r.tenant, in)
		return err
	})
	return a, out, err
}

func TestAcknowledgingStopsEscalationAndOnlyAnOpenAlertCanBeAcknowledged(t *testing.T) {
	r := newRig(t)
	sup, head := r.user("SUPERVISOR"), r.user("HEAD_OPS")
	r.onCall("SUPERVISOR", sup)
	r.onCall("HEAD_OPS", head)
	r.must(r.in(func(tx pgx.Tx) error {
		return r.rt.SetEscalation(context.Background(), tx, r.tenant, uuid.New(), "SUPERVISOR", "HEAD_OPS")
	}))
	a, _ := r.raise(in("case:6:critical"))

	// With no approved wait, nothing escalates by itself.
	r.now = r.now.Add(48 * time.Hour)
	var n int
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		n, err = r.svc.Escalate(context.Background(), tx, r.tenant)
		return err
	}))
	if n != 0 {
		t.Fatalf("an alert escalated with no approved wait: %d", n)
	}

	r.setConfig("alert.escalate_after_minutes", 15)
	r.now = r.now.Add(time.Minute)
	r.onCall("SUPERVISOR", sup) // the first shifts have ended by now
	r.onCall("HEAD_OPS", head)
	b, _ := r.raise(in("case:7:critical"))
	r.now = r.now.Add(14 * time.Minute)
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		n, err = r.svc.Escalate(context.Background(), tx, r.tenant)
		return err
	}))
	// The first alert has waited far longer; the second only 14 minutes.
	if n != 1 {
		t.Fatalf("escalated %d, want only the old one", n)
	}
	var esc Alert
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		esc, err = r.svc.Get(context.Background(), tx, r.tenant, a.ID)
		return err
	}))
	if esc.EscalationLevel != 1 || esc.EscalatedRole != "HEAD_OPS" || len(esc.RoutedTo) != 1 || esc.RoutedTo[0] != head {
		t.Fatalf("%+v", esc)
	}
	// Acknowledging the second stops it from escalating.
	r.must(r.in(func(tx pgx.Tx) error {
		_, err := r.svc.Acknowledge(context.Background(), tx, r.tenant, sup, b.ID)
		return err
	}))
	r.now = r.now.Add(time.Hour)
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		n, err = r.svc.Escalate(context.Background(), tx, r.tenant)
		return err
	}))
	var after Alert
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		after, err = r.svc.Get(context.Background(), tx, r.tenant, b.ID)
		return err
	}))
	if after.EscalationLevel != 0 || after.Status != Acknowledged || after.AcknowledgedBy == nil || *after.AcknowledgedBy != sup {
		t.Fatalf("%+v", after)
	}
	// Acknowledging twice, or acknowledging something that does not exist, is refused.
	err := r.in(func(tx pgx.Tx) error {
		_, err := r.svc.Acknowledge(context.Background(), tx, r.tenant, sup, b.ID)
		return err
	})
	if !errors.Is(err, ErrState) {
		t.Fatalf("%v", err)
	}
	err = r.in(func(tx pgx.Tx) error {
		_, err := r.svc.Acknowledge(context.Background(), tx, r.tenant, sup, uuid.New())
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
}

func TestEscalationStopsAtTheTopOfTheLadderWithoutLosingTheAlert(t *testing.T) {
	r := newRig(t)
	sup := r.user("SUPERVISOR")
	r.onCall("SUPERVISOR", sup)
	r.setConfig("alert.escalate_after_minutes", 10)
	r.must(r.in(func(tx pgx.Tx) error {
		return r.rt.SetEscalation(context.Background(), tx, r.tenant, uuid.New(), "SUPERVISOR", "HEAD_OPS")
	}))
	a, _ := r.raise(in("case:8:critical"))
	// Nobody is on call in HEAD_OPS, so there is nowhere to escalate: it stays open and is marked.
	r.now = r.now.Add(time.Hour)
	var n int
	r.must(r.in(func(tx pgx.Tx) error {
		var err error
		n, err = r.svc.Escalate(context.Background(), tx, r.tenant)
		return err
	}))
	got, _ := r.list(Open)
	if n != 0 || len(got) != 1 || got[0].ID != a.ID || !got[0].Unrouted || got[0].EscalationLevel != 0 {
		t.Fatalf("%d %+v", n, got)
	}
}

func TestAlertsAreTenantScoped(t *testing.T) {
	r := newRig(t)
	r.raise(in("case:9:critical"))
	other := &rig{pool: r.pool, tenant: dbtest.Tenant(t, r.pool), now: r.now, t: t}
	other.svc = &Service{Now: func() time.Time { return other.now }}
	if got, _ := other.list(Open); len(got) != 0 {
		t.Fatalf("%d alerts visible to another institution", len(got))
	}
	// The same dedupe key is a different condition in another institution.
	if _, outcome := other.raise(in("case:9:critical")); outcome != Created {
		t.Fatal("a dedupe key leaked across institutions")
	}
}
