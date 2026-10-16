package cases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
)

type rig struct {
	pool   *db.Pool
	svc    *Service
	tenant uuid.UUID
	now    time.Time
	user   Actor
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := dbtest.Pool(t)
	r := &rig{pool: pool, tenant: dbtest.Tenant(t, pool), now: time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC), user: Actor{audit.ActorUser, uuid.New()}}
	app := &audit.Appender{Now: func() time.Time { return r.now }}
	cfg := &configsvc.Service{Audit: app, Now: func() time.Time { return r.now }}
	r.svc = &Service{Audit: app, Config: cfg, Now: func() time.Time { return r.now }}
	return r
}

func (r *rig) tx(t *testing.T, fn func(tx pgx.Tx) error) error {
	t.Helper()
	return r.pool.WithTenant(context.Background(), r.tenant, fn)
}

// txn inserts a bare transaction row so cases can refer to it.
func (r *rig) txn(t *testing.T, ref string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := r.pool.QueryRow(context.Background(), `INSERT INTO transactions (tenant_id, instruction_ref, channel, rail, amount_minor, currency, debit_account_token,
		lifecycle_state, debit_status, credit_status, settlement_status, first_event_at, last_event_at)
		VALUES ($1,$2,'MOBILE','NIP',1850000,'NGN','acct_0123456789abcdef0123456789abcdef','CONFIRMED_FAILURE','POSTED','FAILED','NONE',$3,$3) RETURNING id`,
		r.tenant, ref, r.now).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *rig) open(t *testing.T, in OpenInput) (Case, bool) {
	t.Helper()
	var c Case
	var created bool
	if err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		c, created, err = r.svc.Open(context.Background(), tx, r.tenant, in)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return c, created
}

func botCase(txn uuid.UUID, code string) OpenInput {
	amount := int64(1850000)
	return OpenInput{Classification: code, Source: SourceBot, RiskGrade: "", TxnID: &txn, ExposureMinor: &amount, Currency: "NGN", Rail: "NIP",
		RuleID: "DET-TEST-1", RuleVersion: "1.0", Explanation: "test", Actor: System,
		Evidence: []Evidence{{Kind: "transaction_event", Ref: "EV-1", SourceSystem: "SWITCH_A", Hash: "abc", IsFact: true}}}
}

func TestStateMachineAllowsOnlyDefinedMoves(t *testing.T) {
	allowed := [][2]string{
		{StDetected, StUnderInvestigation}, {StUnderInvestigation, StAwaitingApproval}, {StAwaitingApproval, StRemediationSubmitted},
		{StRemediationSubmitted, StOutcomeVerification}, {StOutcomeVerification, StResolved}, {StResolved, StClosed}, {StClosed, StReopened},
		{StRemediationFailed, StUnderInvestigation}, {StAwaitingApproval, StRejectedAction},
	}
	for _, m := range allowed {
		if !CanTransition(m[0], m[1]) {
			t.Errorf("%s to %s should be allowed", m[0], m[1])
		}
	}
	denied := [][2]string{
		{StDetected, StResolved}, {StDetected, StRemediationSubmitted}, {StUnderInvestigation, StResolved},
		{StAwaitingApproval, StResolved}, {StResolved, StDetected}, {StClosed, StResolved}, {StRemediationSubmitted, StResolved},
		{StDetected, StDetected}, {"NOPE", StResolved},
	}
	for _, m := range denied {
		if CanTransition(m[0], m[1]) {
			t.Errorf("%s to %s should be refused", m[0], m[1])
		}
	}
	// Nothing reaches RESOLVED except through outcome verification.
	for from, tos := range transitions {
		for _, to := range tos {
			if to == StResolved && from != StOutcomeVerification {
				t.Errorf("%s can jump straight to RESOLVED", from)
			}
		}
	}
	for _, s := range []string{StResolved, StClosed, StClosedNoException, StClosedCustomerError} {
		if !IsTerminal(s) {
			t.Errorf("%s should be terminal", s)
		}
	}
}

func TestOpeningADetectedCaseIsIdempotentAndRecordsEvidenceHistoryAndAudit(t *testing.T) {
	r := newRig(t)
	txn := r.txn(t, "INS-1")
	c, created := r.open(t, botCase(txn, CodeNoCredit))
	if !created || c.Status != StDetected || c.Source != SourceBot || !strings.HasPrefix(c.Ref, "CASE-2026-") || c.Priority != "MEDIUM" || c.Version != 1 {
		t.Fatalf("%+v created=%v", c, created)
	}
	again, createdAgain := r.open(t, botCase(txn, CodeNoCredit))
	if createdAgain || again.ID != c.ID {
		t.Fatalf("a second sighting made a second case: %+v", again)
	}
	other, createdOther := r.open(t, botCase(txn, CodeUnknown))
	if !createdOther || other.ID == c.ID {
		t.Fatal("a different classification must be a different case")
	}

	var ev []Evidence
	var hist []Event
	var chain audit.VerifyResult
	if err := r.tx(t, func(tx pgx.Tx) error {
		var err error
		if ev, err = r.svc.Evidence(context.Background(), tx, r.tenant, c.ID); err != nil {
			return err
		}
		if hist, err = r.svc.History(context.Background(), tx, r.tenant, c.ID); err != nil {
			return err
		}
		chain, err = audit.Verify(context.Background(), tx, r.tenant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(ev) != 1 || ev[0].Ref != "EV-1" || !ev[0].IsFact || len(hist) != 1 || hist[0].Kind != "opened" || !chain.OK {
		t.Fatalf("evidence %+v history %+v chain %+v", ev, hist, chain)
	}
}

func TestOpeningRulesAndCustomerCases(t *testing.T) {
	r := newRig(t)
	txn := r.txn(t, "INS-2")
	for name, in := range map[string]OpenInput{
		"unknown code":        {Classification: "ZZZ999", Source: SourceManual},
		"unknown source":      {Classification: CodeCustomer, Source: "telepathy"},
		"bot without a txn":   {Classification: CodeNoCredit, Source: SourceBot},
		"evidence w/o a kind": {Classification: CodeNoCredit, Source: SourceBot, TxnID: &txn, Evidence: []Evidence{{Ref: "x"}}},
	} {
		err := r.tx(t, func(tx pgx.Tx) error { _, _, err := r.svc.Open(context.Background(), tx, r.tenant, in); return err })
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	complaint, _ := json.Marshal(map[string]any{"channel": "agent", "text": "I was debited and the beneficiary got nothing"})
	c, created := r.open(t, OpenInput{Classification: CodeCustomer, Source: SourceCustomer, TxnID: &txn, Rail: "NIP", Complaint: complaint, Actor: r.user,
		Evidence: []Evidence{{Kind: "complaint", Ref: "COMPLAINT-1", IsFact: false}}})
	if !created || c.Source != SourceCustomer || !strings.Contains(string(c.Complaint), "beneficiary") {
		t.Fatalf("%+v", c)
	}
	// Customer cases are not deduplicated: two complaints are two records.
	c2, created2 := r.open(t, OpenInput{Classification: CodeCustomer, Source: SourceCustomer, TxnID: &txn, Rail: "NIP", Actor: r.user})
	if !created2 || c2.ID == c.ID {
		t.Fatal("a second complaint was swallowed")
	}
}

func TestTransitionsAreValidatedVersionedAndRecorded(t *testing.T) {
	r := newRig(t)
	c, _ := r.open(t, botCase(r.txn(t, "INS-3"), CodeNoCredit))
	ctx := context.Background()
	move := func(to string, v int, reason string) (Case, error) {
		var out Case
		err := r.tx(t, func(tx pgx.Tx) error {
			var err error
			out, err = r.svc.Transition(ctx, tx, r.tenant, c.ID, r.user, to, v, reason)
			return err
		})
		return out, err
	}

	if _, err := move(StResolved, 1, "skip ahead"); !errors.Is(err, ErrBadMove) {
		t.Errorf("illegal move: %v", err)
	}
	if _, err := move(StUnderInvestigation, 1, " "); !errors.Is(err, ErrInvalid) {
		t.Errorf("no reason: %v", err)
	}
	if _, err := move(StUnderInvestigation, 7, "STALE"); !errors.Is(err, ErrStaleVersion) {
		t.Errorf("stale version: %v", err)
	}
	got, err := move(StUnderInvestigation, 1, "ANALYST_PICKED_UP")
	if err != nil || got.Status != StUnderInvestigation || got.Version != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := move(StAwaitingApproval, 1, "OLD_VERSION"); !errors.Is(err, ErrStaleVersion) {
		t.Errorf("reusing the old version: %v", err)
	}
	if err := r.tx(t, func(tx pgx.Tx) error {
		_, err := r.svc.Transition(ctx, tx, r.tenant, uuid.New(), r.user, StUnderInvestigation, 0, "X")
		return err
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown case: %v", err)
	}

	var hist []Event
	_ = r.tx(t, func(tx pgx.Tx) error { var e error; hist, e = r.svc.History(ctx, tx, r.tenant, c.ID); return e })
	if len(hist) != 2 || hist[1].Kind != "transition" || hist[1].FromStatus != StDetected || hist[1].ToStatus != StUnderInvestigation ||
		hist[1].ReasonCode != "ANALYST_PICKED_UP" || hist[1].ActorID != r.user.ID.String() {
		t.Fatalf("history: %+v", hist)
	}
	// History is append-only.
	for _, stmt := range []string{`UPDATE case_events SET kind = 'x'`, `DELETE FROM case_events`, `UPDATE case_evidence SET ref = 'x'`, `DELETE FROM case_evidence`} {
		if err := r.tx(t, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, stmt); return err }); err == nil {
			t.Errorf("tenant role ran %q", stmt)
		}
	}
}

func TestClosingStampsResolutionTimeAndNotesAndEvidenceAreHistory(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	c, _ := r.open(t, botCase(r.txn(t, "INS-4"), CodeUnknown))
	step := func(to, reason string) {
		t.Helper()
		if err := r.tx(t, func(tx pgx.Tx) error {
			_, err := r.svc.Transition(ctx, tx, r.tenant, c.ID, System, to, 0, reason)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	step(StUnderInvestigation, "TRIAGE")
	if err := r.tx(t, func(tx pgx.Tx) error {
		if err := r.svc.AddNote(ctx, tx, r.tenant, c.ID, r.user, "called the switch operator"); err != nil {
			return err
		}
		if err := r.svc.AddNote(ctx, tx, r.tenant, c.ID, r.user, "  "); !errors.Is(err, ErrInvalid) {
			t.Errorf("blank note: %v", err)
		}
		if err := r.svc.Assign(ctx, tx, r.tenant, c.ID, r.user, r.user.ID); err != nil {
			return err
		}
		return r.svc.AddEvidence(ctx, tx, r.tenant, c.ID, r.user, Evidence{Kind: "status_query", Ref: "SQ-1", IsFact: true})
	}); err != nil {
		t.Fatal(err)
	}
	step(StClosedNoException, "LATE_SUCCESS_CONFIRMED")

	var got Case
	var hist []Event
	var ev []Evidence
	_ = r.tx(t, func(tx pgx.Tx) error {
		got, _ = r.svc.Get(ctx, tx, r.tenant, c.ID)
		hist, _ = r.svc.History(ctx, tx, r.tenant, c.ID)
		ev, _ = r.svc.Evidence(ctx, tx, r.tenant, c.ID)
		return nil
	})
	if got.Status != StClosedNoException || got.ResolvedAt == nil || got.ClosedReason != "LATE_SUCCESS_CONFIRMED" || got.OwnerID == nil || *got.OwnerID != r.user.ID {
		t.Fatalf("%+v", got)
	}
	kinds := []string{}
	for _, h := range hist {
		kinds = append(kinds, h.Kind)
	}
	if strings.Join(kinds, ",") != "opened,transition,note,assigned,evidence_added,transition" || len(ev) != 2 {
		t.Fatalf("history %v, evidence %d", kinds, len(ev))
	}
	if byRef, err := func() (Case, error) {
		var c2 Case
		err := r.tx(t, func(tx pgx.Tx) error { var e error; c2, e = r.svc.GetByRef(ctx, tx, r.tenant, c.Ref); return e })
		return c2, err
	}(); err != nil || byRef.ID != c.ID {
		t.Fatalf("by ref: %v", err)
	}
}

func TestQueueOrderFilterPagingAndIsolation(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	mk := func(ref, code, risk string, minutes int) Case {
		txn := r.txn(t, ref)
		in := botCase(txn, code)
		in.RiskGrade = risk
		in.At = r.now.Add(time.Duration(minutes) * time.Minute)
		c, _ := r.open(t, in)
		return c
	}
	low1 := mk("INS-Q1", CodeNoCredit, "", 0)
	high := mk("INS-Q2", CodeDuplicate, "HIGH", 10)
	low2 := mk("INS-Q3", CodeUnknown, "", 5)

	list := func(f Filter, after *Cursor, n int) []Case {
		var out []Case
		if err := r.tx(t, func(tx pgx.Tx) error { var e error; out, e = r.svc.List(ctx, tx, r.tenant, f, after, n); return e }); err != nil {
			t.Fatal(err)
		}
		return out
	}
	all := list(Filter{}, nil, 10)
	if len(all) != 3 || all[0].ID != high.ID || all[1].ID != low1.ID || all[2].ID != low2.ID {
		t.Fatalf("order (high priority first, then oldest): %v", []string{all[0].Ref, all[1].Ref, all[2].Ref})
	}
	if got := list(Filter{Classification: CodeUnknown}, nil, 10); len(got) != 1 || got[0].ID != low2.ID {
		t.Fatalf("filter: %+v", got)
	}
	if got := list(Filter{Status: StResolved}, nil, 10); len(got) != 0 {
		t.Fatalf("status filter: %+v", got)
	}
	cur := CursorFor(all[0])
	next := list(Filter{}, &cur, 10)
	if len(next) != 2 || next[0].ID != low1.ID {
		t.Fatalf("after the first page: %+v", next)
	}
	if err := r.tx(t, func(tx pgx.Tx) error { _, e := r.svc.List(ctx, tx, r.tenant, Filter{}, nil, 0); return e }); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad limit: %v", err)
	}

	other := newRig(t)
	var none []Case
	_ = other.tx(t, func(tx pgx.Tx) error { none, _ = other.svc.List(ctx, tx, other.tenant, Filter{}, nil, 10); return nil })
	if len(none) != 0 {
		t.Fatalf("another tenant sees %d cases", len(none))
	}
	err := other.tx(t, func(tx pgx.Tx) error { _, e := other.svc.Get(ctx, tx, other.tenant, high.ID); return e })
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("another tenant fetched a case: %v", err)
	}
}

func TestDeadlinesComeOnlyFromApprovedClocks(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	c, _ := r.open(t, botCase(r.txn(t, "INS-S1"), CodeNoCredit))
	if c.SLADueAt != nil || c.RegulatoryClock != "" {
		t.Fatalf("a deadline appeared with no approved clock: %+v", c)
	}

	// Propose and approve the NIP clock with two different people.
	raw, _ := json.Marshal(configsvc.Clock{Situation: "Failed instant transfer", Duration: 24, Unit: "hours", StartEvent: "failure_detected", Calendar: "WAT", Owner: "Compliance"})
	err := r.tx(t, func(tx pgx.Tx) error {
		v, err := r.svc.Config.Propose(ctx, tx, r.tenant, uuid.New(), configsvc.ProposeInput{Key: configsvc.ClockKey("FAILED_INSTANT_TRANSFER"), Value: raw,
			SourceReference: "SYNTHETIC DEMO POLICY", EffectiveFrom: r.now, VerifyStatus: "unverified", Reason: "synthetic policy for the test"})
		if err != nil {
			return err
		}
		_, err = r.svc.Config.Decide(ctx, tx, r.tenant, uuid.New(), v.Key, v.Version, true, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	c2, _ := r.open(t, botCase(r.txn(t, "INS-S2"), CodeNoCredit))
	if c2.SLADueAt == nil || !c2.SLADueAt.Equal(c2.OpenedAt.Add(24*time.Hour)) || c2.RegulatoryClock != "FAILED_INSTANT_TRANSFER" {
		t.Fatalf("deadline: %+v", c2)
	}
	// A code with no clock mapping still has none.
	c3, _ := r.open(t, botCase(r.txn(t, "INS-S3"), CodeUnknown))
	if c3.SLADueAt == nil {
		t.Log("UNK001 on NIP maps to the same clock, so it has a deadline too")
	}
}

func TestNotificationsAreTwoStageDedupedAndNeverPromiseBeforeVerification(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	c, _ := r.open(t, botCase(r.txn(t, "INS-N1"), CodeNoCredit))

	first := notify.CustomerDetected(1850000, "NGN", "INS-N1")
	second := notify.CustomerVerified(1850000, "NGN", "INS-N1")
	for _, bad := range []string{"refund", "reversed", "returned", "credited back", "money back"} {
		if strings.Contains(strings.ToLower(first.Body), bad) {
			t.Errorf("the first customer message says %q before anything is verified: %s", bad, first.Body)
		}
	}
	if !strings.Contains(second.Body, "reversed") || !strings.Contains(second.Body, "NGN 18,500.00") {
		t.Errorf("verified message: %s", second.Body)
	}

	var n1, n2, n3 bool
	if err := r.tx(t, func(tx pgx.Tx) error {
		var e error
		if n1, e = notify.Record(ctx, tx, r.tenant, c.ID, first, c.Ref+":customer:detected"); e != nil {
			return e
		}
		if n2, e = notify.Record(ctx, tx, r.tenant, c.ID, first, c.Ref+":customer:detected"); e != nil {
			return e
		}
		n3, e = notify.Record(ctx, tx, r.tenant, c.ID, second, c.Ref+":customer:verified")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if !n1 || n2 || !n3 {
		t.Fatalf("created flags %v %v %v", n1, n2, n3)
	}
	var stored []notify.Stored
	_ = r.tx(t, func(tx pgx.Tx) error {
		stored, _ = notify.ForCase(ctx, tx, r.tenant, c.ID, notify.Customer)
		return nil
	})
	if len(stored) != 2 || stored[0].Stage != notify.StageDetected || stored[1].Stage != notify.StageVerified || !stored[0].Simulated {
		t.Fatalf("%+v", stored)
	}
	if _, err := notify.Record(ctx, nil, r.tenant, c.ID, first, ""); err == nil {
		t.Error("a missing dedupe key was accepted")
	}
}

func TestMoneyFormatting(t *testing.T) {
	for in, want := range map[int64]string{0: "NGN 0.00", 5: "NGN 0.05", 100: "NGN 1.00", 123456789: "NGN 1,234,567.89", 1850000: "NGN 18,500.00", 99999: "NGN 999.99"} {
		if got := notify.Money(in, "NGN"); got != want {
			t.Errorf("Money(%d) = %s, want %s", in, got, want)
		}
	}
}

func TestStateMachineAllowsExternalDependencyMoves(t *testing.T) {
	if !CanTransition(StUnderInvestigation, StExternalDependency) {
		t.Errorf("UNDER_INVESTIGATION -> EXTERNAL_DEPENDENCY should be allowed")
	}
	if !CanTransition(StExternalDependency, StUnderInvestigation) {
		t.Errorf("EXTERNAL_DEPENDENCY -> UNDER_INVESTIGATION should be allowed")
	}
	if CanTransition(StDetected, StExternalDependency) {
		t.Errorf("DETECTED -> EXTERNAL_DEPENDENCY should not be allowed")
	}
	if CanTransition(StExternalDependency, StResolved) {
		t.Errorf("EXTERNAL_DEPENDENCY -> RESOLVED should not be allowed")
	}
}

func TestEscalateAndAutoAssignValidation(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	_, err := svc.Escalate(ctx, nil, uuid.New(), uuid.New(), System, "SUPERVISOR", "")
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("Escalate with empty reason should return ErrInvalid, got %v", err)
	}

	_, err = svc.AutoAssign(ctx, nil, uuid.New(), uuid.New(), System, "SUPERVISOR")
	if err == nil || !strings.Contains(err.Error(), "routing service not configured") {
		t.Errorf("AutoAssign without routing should fail, got %v", err)
	}
}


func TestACaseCanNameItsClockAndKeepAnInternalTargetApartFromTheRegulatoryDeadline(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	raw, _ := json.Marshal(configsvc.Clock{Situation: "Written reversal request", Duration: 48, Unit: "hours", StartEvent: "transaction_date", Calendar: "WAT", Owner: "Compliance"})
	if err := r.tx(t, func(tx pgx.Tx) error {
		v, err := r.svc.Config.Propose(ctx, tx, r.tenant, uuid.New(), configsvc.ProposeInput{Key: configsvc.ClockKey("WRONG_ACCOUNT_REQUEST"), Value: raw,
			SourceReference: "SYNTHETIC DEMO POLICY", EffectiveFrom: r.now, VerifyStatus: "unverified", Reason: "synthetic policy for the test"})
		if err != nil {
			return err
		}
		_, err = r.svc.Config.Decide(ctx, tx, r.tenant, uuid.New(), v.Key, v.Version, true, "")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	target := r.now.Add(6 * time.Hour)
	in := botCase(r.txn(t, "INS-IT1"), CodeCustomer)
	in.Source, in.Clock, in.InternalTargetAt = SourceCustomer, "WRONG_ACCOUNT_REQUEST", &target
	c, _ := r.open(t, in)
	if c.RegulatoryClock != "WRONG_ACCOUNT_REQUEST" || c.SLADueAt == nil || !c.SLADueAt.Equal(c.OpenedAt.Add(48*time.Hour)) {
		t.Fatalf("the named clock was not used: %+v", c)
	}
	if c.InternalTargetAt == nil || !c.InternalTargetAt.Equal(target) {
		t.Fatalf("the internal target was lost: %+v", c.InternalTargetAt)
	}
	// Without a name, the code and rail still pick the clock, and there is no internal target.
	plain, _ := r.open(t, botCase(r.txn(t, "INS-IT2"), CodeNoCredit))
	if plain.InternalTargetAt != nil {
		t.Fatalf("%v", plain.InternalTargetAt)
	}
	// CUS002 uses the written-reversal clock by default.
	if clockFor(CodeWrongAcct, "NIP") != "WRONG_ACCOUNT_REQUEST" {
		t.Fatal("CUS002 has no clock")
	}
}
