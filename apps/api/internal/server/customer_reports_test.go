package server

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/alertparse"
	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/creport"
	"github.com/smartreversal/smartreversal/apps/api/internal/customer"
	"github.com/smartreversal/smartreversal/apps/api/internal/deadline"
	"github.com/smartreversal/smartreversal/apps/api/internal/escalation"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/labels"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

type reportRig struct {
	*custRig
	ing          *txn.Service
	alice, bob   customer.Customer
	aliceT, bobT string
}

func newReportRig(t *testing.T) *reportRig {
	t.Helper()
	pool := dbtest.Pool(t)
	policy, err := authz.Load(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	priv, _, _ := identity.GenerateKey()
	signer, _ := identity.NewSigner("k1", priv, 10*time.Minute)
	ks, _ := identity.ParseKeySet("", map[string]ed25519.PublicKey{"k1": signer.PublicKey()})
	app := &audit.Appender{}
	cfg := &configsvc.Service{Audit: app}
	logger, _ := quietLogger()
	cs := &cases.Service{Audit: app, Config: cfg}
	svc := &Services{Pool: pool, Policy: policy, Audit: app, Config: cfg, Txn: &txn.Service{}, Cases: cs,
		CustomerLogin: &customer.Login{Provider: &customer.DevProvider{Pool: pool, Audit: app}, Signer: signer},
		Reports:       &creport.Service{Pool: pool, Config: cfg, Cases: cs, Deadlines: &deadline.Engine{Config: cfg}, Parser: &alertparse.RuleParser{OwnNames: []string{"Wema"}}}}
	cr := &custRig{store: &customer.Store{Pool: pool}, auditRig: &auditRig{pool: pool, signer: signer, tenant: dbtest.Tenant(t, pool), appender: app,
		handler: NewRouter(Options{Logger: logger, Verifier: identity.NewVerifier(ks, nil), Services: svc})}}
	if err := pool.QueryRow(context.Background(), `SELECT code FROM institutions WHERE id = $1`, cr.tenant).Scan(&cr.code); err != nil {
		t.Fatal(err)
	}
	r := &reportRig{custRig: cr, ing: &txn.Service{}}
	r.alice, _ = cr.store.Create(context.Background(), cr.tenant, "Alice", customerPass)
	r.bob, _ = cr.store.Create(context.Background(), cr.tenant, "Bob", customerPass)
	_ = cr.store.LinkAccount(context.Background(), cr.tenant, r.alice.ID, tokA, "6789", "bank onboarding")
	_ = cr.store.LinkAccount(context.Background(), cr.tenant, r.bob.ID, tokB, "4321", "bank onboarding")
	_, r.aliceT = cr.login(t, r.alice.Ref, customerPass)
	_, r.bobT = cr.login(t, r.bob.Ref, customerPass)
	return r
}

func (r *reportRig) post(path, body, tok, key string) *recorder {
	req := newReq(http.MethodPost, path, body, tok)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	return serve(r.handler, req)
}

func (r *reportRig) transfer(t *testing.T, token, ref string, amount int64, at time.Time, types ...string) {
	t.Helper()
	for i, typ := range types {
		err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			_, err := r.ing.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: fmt.Sprintf("EV-%s-%d", ref, i), EventType: typ,
				SourceTS: at.Add(time.Duration(i) * time.Second), Txn: txn.Core{InstructionRef: ref, Channel: "MOBILE", Rail: "NIP", AmountMinor: txn.Amount(amount), Currency: "NGN", DebitAccountToken: token}})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

const reportAlert = "Wema Alert: Acct 0123456789 debited NGN 18,500.00 on 2026-10-14 09:12. Desc: NIP TRANSFER | Ref: NIP202610140912"

func TestACustomerCreatesAReportAndConfirmsItOverHTTPAndNobodyElseCanSeeIt(t *testing.T) {
	r := newReportRig(t)
	at := time.Date(2026, 10, 14, 8, 12, 0, 0, time.UTC)
	r.transfer(t, tokA, "NIP202610140912", 1850000, at, "DEBIT_POSTED", "TRANSFER_SENT", "CREDIT_CONFIRMED")

	body, _ := json.Marshal(map[string]any{"text": reportAlert, "consent": true, "consent_version": "v1"})
	if rec := r.post("/v1/customer/reports", string(body), "", "rep-http-0001"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := r.post("/v1/customer/reports", string(body), r.token(t, r.tenant, uuid.New(), "SUPERVISOR"), "rep-http-0001"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("an operator token: %d", rec.Code)
	}
	if rec := r.post("/v1/customer/reports", string(body), r.aliceT, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("no idempotency key: %d", rec.Code)
	}
	noConsent, _ := json.Marshal(map[string]any{"text": reportAlert, "consent": false, "consent_version": "v1"})
	if rec := r.post("/v1/customer/reports", string(noConsent), r.aliceT, "rep-http-0002"); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "CONSENT_REQUIRED") {
		t.Fatalf("no consent: %d %s", rec.Code, rec.Body.String())
	}

	rec := r.post("/v1/customer/reports", string(body), r.aliceT, "rep-http-0001")
	var rep creport.Report
	_ = json.Unmarshal(rec.Body.Bytes(), &rep)
	if rec.Code != http.StatusCreated || rep.Parsed.Fields["amount_minor"].Value != "1850000" || rep.Parsed.Fields["account_tail"].Value != "6789" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "0123456789") {
		t.Fatal("the raw account number came back")
	}
	if again := r.post("/v1/customer/reports", string(body), r.aliceT, "rep-http-0001"); again.Code != http.StatusOK {
		t.Fatalf("repeat: %d", again.Code)
	}

	confirm := `{"amount_minor":1850000,"occurred_at":"2026-10-14T09:12:00+01:00","account_tail":"6789","reference":"NIP202610140912"}`
	path := "/v1/customer/reports/" + rep.ID.String()
	rec = r.post(path+"/confirm", confirm, r.aliceT, "")
	var out creport.Report
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != http.StatusOK || out.Outcome == nil || out.Outcome.MatchStatus != creport.MatchVerifiedSuccess || out.Outcome.Closure == nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := r.post(path+"/confirm", `{"amount_minor":"1850.00","occurred_at":"2026-10-14T09:12:00+01:00"}`, r.aliceT, ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a fractional amount: %d", rec.Code)
	}
	if rec := r.post(path+"/confirm", `{"amount_minor":1850000}`, r.aliceT, ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing time: %d", rec.Code)
	}

	// Bob cannot see, list, read or confirm Alice's report.
	for _, c := range []struct{ method, p, body string }{{"GET", path, ""}, {"POST", path + "/confirm", confirm}} {
		var rec *recorder
		if c.method == "GET" {
			rec = serve(r.handler, newReq(c.method, c.p, "", r.bobT))
		} else {
			rec = r.post(c.p, c.body, r.bobT, "")
		}
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s as another customer: %d", c.method, c.p, rec.Code)
		}
	}
	list := serve(r.handler, newReq("GET", "/v1/customer/reports", "", r.bobT))
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), rep.ID.String()) {
		t.Fatalf("another customer's report was listed: %s", list.Body.String())
	}
	if rec := serve(r.handler, newReq("GET", "/v1/customer/reports/not-an-id", "", r.aliceT)); rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
}

func TestTooManyReportsGetAFriendlyRetryAfter(t *testing.T) {
	r := newReportRig(t)
	body, _ := json.Marshal(map[string]any{"text": reportAlert, "consent": true, "consent_version": "v1"})
	for i := 0; i < creport.DefaultReportsPerHour; i++ {
		if rec := r.post("/v1/customer/reports", string(body), r.aliceT, fmt.Sprintf("rep-rate-%04d", i)); rec.Code != http.StatusCreated {
			t.Fatalf("%d: %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec := r.post("/v1/customer/reports", string(body), r.aliceT, "rep-rate-9999")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), "try again") {
		t.Fatalf("%d %s %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if rec := r.post("/v1/customer/reports", string(body), r.bobT, "rep-rate-0000"); rec.Code != http.StatusCreated {
		t.Fatalf("one customer's limit applied to another: %d", rec.Code)
	}
}

// Scope acceptance 5: a pasted sample alert is parsed, classified, given the right
// countdown, and queued as CUS001 on the same queue operators work from.
func TestAPastedAlertBecomesACustomerCaseWithTheRightCountdownOnTheSharedQueue(t *testing.T) {
	r := newReportRig(t)
	// Synthetic policy, approved the normal way: a 24 hour clock and a 12 hour internal target.
	setCfg := func(key string, v any, verify string) {
		raw, _ := json.Marshal(v)
		err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			svc := &configsvc.Service{Audit: r.appender}
			ver, err := svc.Propose(context.Background(), tx, r.tenant, uuid.New(), configsvc.ProposeInput{Key: key, Value: raw, SourceReference: "SYNTHETIC DEMO POLICY - not approved for production",
				EffectiveFrom: time.Now(), VerifyStatus: verify, Reason: "synthetic value for a test"})
			if err != nil {
				return err
			}
			_, err = svc.Decide(context.Background(), tx, r.tenant, uuid.New(), key, ver.Version, true, "")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	setCfg(configsvc.ClockKey("FAILED_INSTANT_TRANSFER"), configsvc.Clock{Situation: "Failed instant transfer", Duration: 24, Unit: "hours", StartEvent: "dispute_reported", Calendar: "WAT", Owner: "Compliance"}, "reported")
	setCfg("internal_target_hours.FAILED_INSTANT_TRANSFER", 12, "unverified")
	time.Sleep(10 * time.Millisecond)

	at := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Minute)
	r.transfer(t, tokA, "NIP202610140912", 1850000, at, "DEBIT_POSTED", "TRANSFER_SENT", "CREDIT_FAILED")
	alert := "Wema Alert: Acct 0123456789 debited NGN 18,500.00 on " + at.In(time.FixedZone("WAT", 3600)).Format("2006-01-02 15:04") + ". Desc: NIP TRANSFER | Ref: NIP202610140912"
	body, _ := json.Marshal(map[string]any{"text": alert, "consent": true, "consent_version": "v1"})
	rec := r.post("/v1/customer/reports", string(body), r.aliceT, "rep-accept-0001")
	var rep creport.Report
	_ = json.Unmarshal(rec.Body.Bytes(), &rep)
	if rec.Code != http.StatusCreated || rep.Parsed.Fields["amount_minor"].Value != "1850000" || rep.GuidedForm {
		t.Fatalf("parsed: %d %s", rec.Code, rec.Body.String())
	}
	path := "/v1/customer/reports/" + rep.ID.String()
	confirm := `{"amount_minor":1850000,"occurred_at":"` + at.Format(time.RFC3339) + `","account_tail":"6789","reference":"NIP202610140912"}`
	if rec = r.post(path+"/confirm", confirm, r.aliceT, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"matched_transaction"`) {
		t.Fatalf("confirm: %d %s", rec.Code, rec.Body.String())
	}

	rec = serve(r.handler, newReq("GET", path+"/deadline", "", r.aliceT))
	var dl deadline.Result
	_ = json.Unmarshal(rec.Body.Bytes(), &dl)
	if rec.Code != http.StatusOK || dl.Dispute != deadline.FailedTransfer || dl.Regulatory == nil || dl.Regulatory.Status != "reported" || dl.Internal == nil || dl.Binding == nil {
		t.Fatalf("deadline: %d %s", rec.Code, rec.Body.String())
	}
	if !dl.Internal.Due.Equal(at.Add(2 * time.Second).Add(12 * time.Hour)) {
		t.Fatalf("internal target %v", dl.Internal.Due)
	}
	if dl.State != deadline.OnTrack || dl.RemainingSec <= 0 {
		t.Fatalf("%+v", dl)
	}

	if rec = serve(r.handler, newReq("GET", path+"/draft", "", r.aliceT)); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "NGN 18,500.00") {
		t.Fatalf("draft: %d %s", rec.Code, rec.Body.String())
	}
	if rec = r.post(path+"/complaint", `{}`, r.bobT, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("another customer submitted: %d", rec.Code)
	}
	rec = r.post(path+"/complaint", `{}`, r.aliceT, "")
	var sub creport.Submitted
	_ = json.Unmarshal(rec.Body.Bytes(), &sub)
	if rec.Code != http.StatusCreated || !sub.Created || sub.CaseRef == "" || sub.Dispute != deadline.FailedTransfer {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body.String())
	}
	if again := r.post(path+"/complaint", `{}`, r.aliceT, ""); again.Code != http.StatusOK {
		t.Fatalf("repeat: %d", again.Code)
	}

	// The same queue operators work from now shows it as CUS001, from a customer, with both deadlines apart.
	ops := r.token(t, r.tenant, uuid.New(), "TXN_ANALYST")
	rec = serve(r.handler, newReq("GET", "/v1/exceptions", "", ops))
	var list struct{ Items []caseJSON }
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	var found *caseJSON
	for i := range list.Items {
		if list.Items[i].CaseRef == sub.CaseRef {
			found = &list.Items[i]
		}
	}
	if rec.Code != http.StatusOK || found == nil || found.Classification != "CUS001" || found.Source != "customer" || found.SLADueAt == nil || found.InternalTarget == nil || found.RegulatoryClock != "FAILED_INSTANT_TRANSFER" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if !found.InternalTarget.Before(*found.SLADueAt) {
		t.Fatalf("the bank's own target should come first: %v %v", found.InternalTarget, found.SLADueAt)
	}
}

func TestACustomerSeesTheirOwnCasesIncludingOnesTheBankFoundAndNobodyElses(t *testing.T) {
	r := newReportRig(t)
	at := time.Now().Add(-time.Hour).UTC()
	r.transfer(t, tokA, "NIP202610140001", 1850000, at, "DEBIT_POSTED", "TRANSFER_SENT", "CREDIT_FAILED")
	r.transfer(t, tokB, "NIP202610140002", 990000, at, "DEBIT_POSTED", "TRANSFER_SENT", "CREDIT_FAILED")
	cs := &cases.Service{Audit: r.appender}
	open := func(ref string) cases.Case {
		var c cases.Case
		_ = r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			var id uuid.UUID
			if err := tx.QueryRow(context.Background(), `SELECT id FROM transactions WHERE instruction_ref = $1`, ref).Scan(&id); err != nil {
				return err
			}
			var err error
			c, _, err = cs.Open(context.Background(), tx, r.tenant, cases.OpenInput{Classification: cases.CodeNoCredit, Source: cases.SourceBot, TxnID: &id, Rail: "NIP", RuleID: "DET-FAIL-1", RuleVersion: "1.0", Explanation: "test", Actor: cases.System})
			return err
		})
		return c
	}
	aliceCase, bobCase := open("NIP202610140001"), open("NIP202610140002")

	rec := serve(r.handler, newReq("GET", "/v1/customer/cases", "", r.aliceT))
	var list struct{ Items []customerCaseItem }
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list.Items) != 1 || list.Items[0].CaseRef != aliceCase.Ref || list.Items[0].DetectedBy != "found_by_the_bank" || list.Items[0].Verified {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), bobCase.Ref) {
		t.Fatal("another customer's case was listed")
	}

	path := "/v1/customer/cases/" + aliceCase.ID.String()
	rec = serve(r.handler, newReq("GET", path+"/timeline", "", r.aliceT))
	var st customerStatus
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if rec.Code != http.StatusOK || st.CaseRef != aliceCase.Ref || st.OutcomeVerified || st.Stage != "investigating" || len(st.Timeline) == 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	for _, bad := range []string{"refund", "returned", "money back"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), bad) {
			t.Fatalf("the timeline promises money before it is verified: %s", rec.Body.String())
		}
	}
	for _, p := range []string{path + "/timeline", path + "/escalation", "/v1/customer/cases/" + uuid.NewString() + "/timeline", "/v1/customer/cases/not-an-id/timeline"} {
		if rec := serve(r.handler, newReq("GET", p, "", r.bobT)); rec.Code != http.StatusNotFound {
			t.Errorf("%s as another customer: %d", p, rec.Code)
		}
	}
	if rec := serve(r.handler, newReq("GET", path+"/timeline", "", "")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("%d", rec.Code)
	}

	// The escalation ladder is empty until the case is overdue and unresolved.
	esc := func() escalationView {
		var v escalationView
		rec := serve(r.handler, newReq("GET", path+"/escalation", "", r.aliceT))
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
		if rec.Code != http.StatusOK {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		return v
	}
	if v := esc(); v.Due || len(v.Steps) != 0 {
		t.Fatalf("%+v", v)
	}
	if _, err := r.pool.Exec(context.Background(), `UPDATE cases SET sla_due_at = now() - interval '1 minute' WHERE id = $1`, aliceCase.ID); err != nil {
		t.Fatal(err)
	}
	if v := esc(); !v.Due || len(v.Steps) != 0 || v.Note == "" {
		t.Fatalf("overdue with no approved steps must say so and invent nothing: %+v", v)
	}
	author, reviewer := uuid.New(), uuid.New()
	_ = r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		if err := escalation.Propose(context.Background(), tx, r.tenant, author, escalation.Step{No: 1, Label: "Bank dispute channel", Contact: "SYNTHETIC PLACEHOLDER", Wording: "Please contact the bank's dispute channel about this case."}); err != nil {
			return err
		}
		return escalation.Decide(context.Background(), tx, r.tenant, reviewer, 1, true, "checked", time.Now())
	})
	if v := esc(); !v.Due || len(v.Steps) != 1 || v.Steps[0].Label != "Bank dispute channel" {
		t.Fatalf("%+v", v)
	}
	if _, err := r.pool.Exec(context.Background(), `UPDATE cases SET status = 'RESOLVED' WHERE id = $1`, aliceCase.ID); err != nil {
		t.Fatal(err)
	}
	if v := esc(); v.Due || len(v.Steps) != 0 {
		t.Fatalf("a resolved case still shows the ladder: %+v", v)
	}
}

func (r *reportRig) opMFA(t *testing.T, roles ...string) string {
	t.Helper()
	tok, _, err := r.signer.Issue(identity.Principal{UserID: uuid.New(), TenantID: r.tenant, Roles: roles, MFA: true}, identity.AudienceOps, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestFeedbackOverHTTPAndLabelledExamplesFromMissedCases(t *testing.T) {
	r := newReportRig(t)
	app := r.appender
	// Close hook as the API wires it.
	svcCases := &cases.Service{Audit: app, OnClose: labels.OnClose}
	r.handler = rebuild(t, r, svcCases)

	at := time.Now().Add(-time.Hour).UTC().Truncate(time.Minute)
	r.transfer(t, tokA, "NIP202610140777", 777700, at, "DEBIT_POSTED", "TRANSFER_SENT", "CREDIT_FAILED")
	body, _ := json.Marshal(map[string]any{"text": "Wema Alert: Acct 0123456789 debited NGN 7,777.00 on " + at.In(time.FixedZone("WAT", 3600)).Format("2006-01-02 15:04") + ". Ref: NIP202610140777", "consent": true, "consent_version": "v1"})
	var rep creport.Report
	_ = json.Unmarshal(r.post("/v1/customer/reports", string(body), r.aliceT, "rep-fb-http-1").Body.Bytes(), &rep)
	path := "/v1/customer/reports/" + rep.ID.String()
	if rec := r.post(path+"/feedback", `{"helpful":true}`, r.aliceT, "fb-http-0001"); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "NOT_READY") {
		t.Fatalf("before submission: %d %s", rec.Code, rec.Body.String())
	}
	confirm := `{"amount_minor":777700,"occurred_at":"` + at.Format(time.RFC3339) + `","account_tail":"6789","reference":"NIP202610140777"}`
	if rec := r.post(path+"/confirm", confirm, r.aliceT, ""); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var sub creport.Submitted
	_ = json.Unmarshal(r.post(path+"/complaint", `{}`, r.aliceT, "").Body.Bytes(), &sub)
	if sub.CaseRef == "" {
		t.Fatal("not submitted")
	}
	if rec := r.post(path+"/feedback", `{"helpful":true,"comment":"Thanks"}`, r.aliceT, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("no key: %d", rec.Code)
	}
	if rec := r.post(path+"/feedback", `{"helpful":true,"comment":"Thanks"}`, r.aliceT, "fb-http-0002"); rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := r.post(path+"/feedback", `{"helpful":true,"comment":"Thanks"}`, r.aliceT, "fb-http-0002"); rec.Code != http.StatusOK {
		t.Fatalf("repeat: %d", rec.Code)
	}
	if rec := r.post(path+"/feedback", `{"helpful":true}`, r.bobT, "fb-http-0003"); rec.Code != http.StatusNotFound {
		t.Fatalf("another customer: %d", rec.Code)
	}
	if rec := r.post(path+"/feedback", `{}`, r.aliceT, "fb-http-0004"); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("nothing said: %d", rec.Code)
	}

	// Operations close the customer-reported case (the detector never raised it), which records a pending label.
	var caseID uuid.UUID
	_ = r.pool.QueryRow(context.Background(), `SELECT id FROM cases WHERE case_ref = $1`, sub.CaseRef).Scan(&caseID)
	closer := uuid.New()
	_ = r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		for _, to := range []string{cases.StUnderInvestigation, cases.StClosedNoException} {
			if _, err := svcCases.Transition(context.Background(), tx, r.tenant, caseID, cases.Actor{Type: audit.ActorUser, ID: closer}, to, 0, "CHECKED"); err != nil {
				return err
			}
		}
		return nil
	})
	analyst := r.token(t, r.tenant, uuid.New(), "TXN_ANALYST")
	rec := serve(r.handler, newReq("GET", "/v1/labeled-examples", "", analyst))
	var list struct{ Items []labels.Example }
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list.Items) != 1 || list.Items[0].Status != "pending" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(r.handler, newReq("GET", "/v1/labeled-examples", "", r.aliceT)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a customer saw labels: %d", rec.Code)
	}
	id := list.Items[0].ID.String()
	d := `{"confirm":true,"note":"root cause clear"}`
	if rec := r.post("/v1/labeled-examples/"+id+"/decision", d, analyst, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("an analyst decided: %d", rec.Code)
	}
	if rec := r.post("/v1/labeled-examples/"+id+"/decision", d, r.token(t, r.tenant, uuid.New(), "RISK_COMPLIANCE"), ""); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "MFA_REQUIRED") {
		t.Fatalf("no MFA: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.post("/v1/labeled-examples/"+id+"/decision", d, r.opMFA(t, "RISK_COMPLIANCE"), ""); rec.Code != http.StatusOK {
		t.Fatalf("decide: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.post("/v1/labeled-examples/"+id+"/decision", d, r.opMFA(t, "RISK_COMPLIANCE"), ""); rec.Code != http.StatusNotFound && rec.Code != http.StatusConflict {
		t.Fatalf("the superseded version was decided again: %d", rec.Code)
	}
	rec = serve(r.handler, newReq("GET", "/v1/labeled-examples?status=confirmed", "", analyst))
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Items) != 1 || list.Items[0].Version != 2 {
		t.Fatalf("%s", rec.Body.String())
	}
}

// rebuild returns a router like the rig's but whose case service has the close hook.
func rebuild(t *testing.T, r *reportRig, cs *cases.Service) http.Handler {
	t.Helper()
	policy, err := authz.Load(context.Background(), r.pool)
	if err != nil {
		t.Fatal(err)
	}
	ks, _ := identity.ParseKeySet("", map[string]ed25519.PublicKey{"k1": r.signer.PublicKey()})
	cfg := &configsvc.Service{Audit: r.appender}
	logger, _ := quietLogger()
	svc := &Services{Pool: r.pool, Policy: policy, Audit: r.appender, Config: cfg, Txn: &txn.Service{}, Cases: cs,
		CustomerLogin: &customer.Login{Provider: &customer.DevProvider{Pool: r.pool, Audit: r.appender}, Signer: r.signer},
		Reports:       &creport.Service{Pool: r.pool, Config: cfg, Cases: cs, Deadlines: &deadline.Engine{Config: cfg}, Parser: &alertparse.RuleParser{OwnNames: []string{"Wema"}}}}
	return NewRouter(Options{Logger: logger, Verifier: identity.NewVerifier(ks, nil), Services: svc})
}
