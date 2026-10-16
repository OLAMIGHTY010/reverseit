package server

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/detect"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

type caseRig struct {
	*auditRig
	ing *txn.Service
	det *detect.Detector
	n   int
}

func newCaseRig(t *testing.T) *caseRig {
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
	cs := &cases.Service{Audit: app, Config: cfg}
	logger, _ := quietLogger()
	return &caseRig{
		ing: &txn.Service{}, det: &detect.Detector{Cases: cs, Config: cfg},
		auditRig: &auditRig{pool: pool, signer: signer, tenant: dbtest.Tenant(t, pool), appender: app,
			handler: NewRouter(Options{Logger: logger, Verifier: identity.NewVerifier(ks, nil),
				Services: &Services{Pool: pool, Policy: policy, Audit: app, Txn: &txn.Service{}, Cases: cs, Config: cfg}})},
	}
}

func (r *caseRig) event(t *testing.T, ref, typ string, at time.Time) uuid.UUID {
	t.Helper()
	r.n++
	var res txn.Result
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		var err error
		res, err = r.ing.Ingest(context.Background(), tx, r.tenant, txn.Envelope{SourceSystem: "SWITCH_A", SourceEventRef: fmt.Sprintf("EV-%s-%d", ref, r.n),
			EventType: typ, SourceTS: at, Txn: txn.Core{InstructionRef: ref, Channel: "MOBILE", Rail: "NIP", AmountMinor: 1850000, Currency: "NGN",
				DebitAccountToken: tokA, BeneficiaryToken: tokB, SessionID: "S-" + ref}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return res.TxnID
}

// failed ingests a debit whose credit failed, runs the detector, and returns the case id.
func (r *caseRig) failed(t *testing.T, ref string) (txnID uuid.UUID, c cases.Case) {
	t.Helper()
	at := time.Now().Add(-10 * time.Minute)
	r.event(t, ref, "DEBIT_POSTED", at)
	r.event(t, ref, "TRANSFER_SENT", at.Add(time.Second))
	txnID = r.event(t, ref, "CREDIT_FAILED", at.Add(2*time.Second))
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		fs, err := r.det.Evaluate(context.Background(), tx, r.tenant, txnID)
		if err == nil && len(fs) == 1 {
			c = fs[0].Case
		}
		return err
	})
	if err != nil || c.ID == uuid.Nil {
		t.Fatalf("detector: %v %+v", err, c)
	}
	return
}

func (r *caseRig) call(method, path, tok, body string, hdr ...string) (int, []byte) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func (r *caseRig) as(t *testing.T, roles ...string) string {
	return r.token(t, r.tenant, uuid.New(), roles...)
}

func TestTheQueueShowsDetectedCasesInPriorityOrderWithFiltersAndPaging(t *testing.T) {
	r := newCaseRig(t)
	for i := 0; i < 3; i++ {
		r.failed(t, fmt.Sprintf("INS-Q%d", i))
	}
	tok := r.as(t, "TXN_ANALYST")

	var page struct {
		Items      []caseJSON `json:"items"`
		NextCursor string     `json:"next_cursor"`
	}
	code, body := r.call(http.MethodGet, "/v1/exceptions?limit=2", tok, "")
	_ = json.Unmarshal(body, &page)
	if code != http.StatusOK || len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("%d %s", code, body)
	}
	c := page.Items[0]
	if c.Classification != "FAIL001" || c.Status != "DETECTED" || c.Source != "bot" || c.ExposureMinor != "1850000" || !c.Simulated || c.Version != 1 || c.RuleID == "" {
		t.Fatalf("%+v", c)
	}
	_, body = r.call(http.MethodGet, "/v1/exceptions?limit=2&cursor="+page.NextCursor, tok, "")
	var next struct{ Items []caseJSON }
	_ = json.Unmarshal(body, &next)
	if len(next.Items) != 1 {
		t.Fatalf("second page: %s", body)
	}
	seen := map[string]bool{}
	for _, it := range append(page.Items, next.Items...) {
		seen[it.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("paging saw %d of 3", len(seen))
	}
	_, body = r.call(http.MethodGet, "/v1/exceptions?classification=DUP001", tok, "")
	var none struct{ Items []caseJSON }
	_ = json.Unmarshal(body, &none)
	if len(none.Items) != 0 {
		t.Fatalf("filter: %s", body)
	}
	for _, q := range []string{"?cursor=zzz", "?cursor=" + "MXx4eHw5", "?txn_id=nope", "?limit=0"} {
		if code, _ := r.call(http.MethodGet, "/v1/exceptions"+q, tok, ""); code != http.StatusBadRequest {
			t.Errorf("%s: %d", q, code)
		}
	}
}

func TestQueueAccessByRoleAndTenant(t *testing.T) {
	r := newCaseRig(t)
	_, c := r.failed(t, "INS-A1")
	if code, _ := r.call(http.MethodGet, "/v1/exceptions", "", ""); code != http.StatusUnauthorized {
		t.Errorf("no token: %d", code)
	}
	for _, role := range []string{"TXN_ANALYST", "RECON_ANALYST", "REVERSAL_OFFICER", "SUPERVISOR", "HEAD_OPS", "RISK_COMPLIANCE", "ADMIN_INTEGRATION", "AGENT"} {
		if code, _ := r.call(http.MethodGet, "/v1/exceptions", r.as(t, role), ""); code != http.StatusOK {
			t.Errorf("%s: %d", role, code)
		}
	}
	if code, _ := r.call(http.MethodGet, "/v1/exceptions", r.as(t, "EXECUTIVE"), ""); code != http.StatusForbidden {
		t.Errorf("executive sees case rows: %d", code)
	}
	other := newCaseRig(t)
	if code, _ := other.call(http.MethodGet, "/v1/cases/"+c.ID.String(), other.as(t, "SUPERVISOR"), ""); code != http.StatusNotFound {
		t.Errorf("another tenant fetched the case: %d", code)
	}
	_, body := other.call(http.MethodGet, "/v1/exceptions", other.as(t, "SUPERVISOR"), "")
	var l struct{ Items []caseJSON }
	_ = json.Unmarshal(body, &l)
	if len(l.Items) != 0 {
		t.Fatalf("another tenant sees %d cases", len(l.Items))
	}
}

func TestCaseDetailCarriesEvidenceHistoryTransactionAndNotifications(t *testing.T) {
	r := newCaseRig(t)
	_, c := r.failed(t, "INS-D1")
	code, body := r.call(http.MethodGet, "/v1/cases/"+c.Ref, r.as(t, "SUPERVISOR"), "") // by reference
	var d caseDetail
	_ = json.Unmarshal(body, &d)
	if code != http.StatusOK || d.Case.ID != c.ID.String() || len(d.Evidence) != 3 || len(d.History) != 1 || d.Transaction == nil ||
		d.Transaction.LifecycleState != "CONFIRMED_FAILURE" || len(d.Notifications) != 2 || d.Reversals == nil {
		t.Fatalf("%d %s", code, body)
	}
	if code, _ := r.call(http.MethodGet, "/v1/cases/"+uuid.NewString(), r.as(t, "SUPERVISOR"), ""); code != http.StatusNotFound {
		t.Errorf("unknown id: %d", code)
	}
	if code, _ := r.call(http.MethodGet, "/v1/cases/CASE-1999-000001", r.as(t, "SUPERVISOR"), ""); code != http.StatusNotFound {
		t.Errorf("unknown ref: %d", code)
	}
}

func TestPatchingACaseIsVersionedValidatedAndCannotClaimAFinancialOutcome(t *testing.T) {
	r := newCaseRig(t)
	_, c := r.failed(t, "INS-P1")
	sup := r.as(t, "SUPERVISOR")
	path := "/v1/cases/" + c.ID.String()

	if code, _ := r.call(http.MethodPatch, path, sup, `{"status":"UNDER_INVESTIGATION","reason_code":"PICKED_UP"}`); code != http.StatusPreconditionRequired {
		t.Errorf("no If-Match: %d", code)
	}
	if code, _ := r.call(http.MethodPatch, path, sup, `{"status":"UNDER_INVESTIGATION","reason_code":"PICKED_UP"}`, "If-Match", `"9"`); code != http.StatusPreconditionFailed {
		t.Errorf("stale: %d", code)
	}
	for _, st := range []string{"RESOLVED", "REMEDIATION_SUBMITTED", "OUTCOME_VERIFICATION", "REMEDIATION_FAILED"} {
		if code, body := r.call(http.MethodPatch, path, sup, `{"status":"`+st+`","reason_code":"I SAY SO"}`, "If-Match", `"1"`); code != http.StatusForbidden {
			t.Errorf("%s: %d %s", st, code, body)
		}
	}
	if code, _ := r.call(http.MethodPatch, path, sup, `{"status":"CLOSED","reason_code":"X"}`, "If-Match", `"1"`); code != http.StatusConflict {
		t.Errorf("an illegal move: %d", code)
	}
	if code, _ := r.call(http.MethodPatch, path, sup, `{"status":"UNDER_INVESTIGATION"}`, "If-Match", `"1"`); code != http.StatusUnprocessableEntity {
		t.Errorf("no reason: %d", code)
	}

	code, body := r.call(http.MethodPatch, path, sup, `{"status":"UNDER_INVESTIGATION","reason_code":"PICKED_UP"}`, "If-Match", `"1"`)
	var got caseJSON
	_ = json.Unmarshal(body, &got)
	if code != http.StatusOK || got.Status != "UNDER_INVESTIGATION" || got.Version != 2 {
		t.Fatalf("%d %s", code, body)
	}
	owner := uuid.NewString()
	code, body = r.call(http.MethodPatch, path, sup, `{"owner_id":"`+owner+`"}`, "If-Match", `"2"`)
	_ = json.Unmarshal(body, &got)
	if code != http.StatusOK || got.OwnerID != owner || got.Version != 3 {
		t.Fatalf("assign: %d %s", code, body)
	}

	for _, role := range []string{"AGENT", "EXECUTIVE"} {
		if code, _ := r.call(http.MethodPatch, path, r.as(t, role), `{"status":"UNDER_INVESTIGATION","reason_code":"X"}`, "If-Match", `"3"`); code != http.StatusForbidden {
			t.Errorf("%s patched a case: %d", role, code)
		}
	}
}

func TestNotesAndEvidenceBecomeHistoryAndManualEvidenceIsNotFact(t *testing.T) {
	r := newCaseRig(t)
	_, c := r.failed(t, "INS-E1")
	path := "/v1/cases/" + c.ID.String()
	if code, _ := r.call(http.MethodPost, path+"/notes", r.as(t, "AGENT"), `{"note":"customer called twice"}`); code != http.StatusCreated {
		t.Errorf("agent note: %d", code)
	}
	if code, _ := r.call(http.MethodPost, path+"/notes", r.as(t, "AGENT"), `{"note":"  "}`); code != http.StatusUnprocessableEntity {
		t.Errorf("blank note: %d", code)
	}
	if code, _ := r.call(http.MethodPost, path+"/evidence", r.as(t, "AGENT"), `{"kind":"screenshot","ref":"IMG-1"}`); code != http.StatusForbidden {
		t.Errorf("agent evidence: %d", code)
	}
	if code, _ := r.call(http.MethodPost, path+"/evidence", r.as(t, "TXN_ANALYST"), `{"kind":"screenshot","ref":"IMG-1","detail":{"note":"from customer"}}`); code != http.StatusCreated {
		t.Errorf("analyst evidence: %d", code)
	}
	_, body := r.call(http.MethodGet, path, r.as(t, "SUPERVISOR"), "")
	var d caseDetail
	_ = json.Unmarshal(body, &d)
	manual := 0
	for _, e := range d.Evidence {
		if e.Ref == "IMG-1" {
			manual++
			if e.IsFact {
				t.Error("evidence a person added is presented as verified fact")
			}
		}
	}
	kinds := []string{}
	for _, h := range d.History {
		kinds = append(kinds, h.Kind)
	}
	if manual != 1 || strings.Join(kinds, ",") != "opened,note,evidence_added" {
		t.Fatalf("history %v, manual evidence %d", kinds, manual)
	}
}

func TestComplaintsJoinTheDetectedCaseOrOpenACustomerCase(t *testing.T) {
	r := newCaseRig(t)
	_, c := r.failed(t, "INS-K1")
	agent := r.as(t, "AGENT")

	// 1. The transaction already has a detected case: the complaint joins it.
	code, body := r.call(http.MethodPost, "/v1/complaints", agent, `{"instruction_ref":"INS-K1","text":"I was debited and my friend got nothing"}`)
	var res complaintResponse
	_ = json.Unmarshal(body, &res)
	if code != http.StatusOK || !res.Matched || res.Case.ID != c.ID.String() || !res.Linked {
		t.Fatalf("joined: %d %s", code, body)
	}

	// 2. A transaction with no case yet: a customer-reported CUS001 case on the same queue.
	r.event(t, "INS-K2", "DEBIT_POSTED", time.Now().Add(-time.Hour))
	code, body = r.call(http.MethodPost, "/v1/complaints", agent, `{"instruction_ref":"INS-K2","text":"Debited but the merchant says unpaid","contact_ref":"CALL-77"}`)
	_ = json.Unmarshal(body, &res)
	if code != http.StatusCreated || res.Matched || !res.Linked || res.Case.Classification != "CUS001" || res.Case.Source != "customer" || res.Case.TxnID == "" {
		t.Fatalf("linked: %d %s", code, body)
	}

	// 3. No such transaction: an unmatched claim.
	res = complaintResponse{}
	code, body = r.call(http.MethodPost, "/v1/complaints", agent, `{"instruction_ref":"NO-SUCH-REF","text":"I think I was debited twice"}`)
	_ = json.Unmarshal(body, &res)
	if code != http.StatusCreated || res.Linked || res.Case.TxnID != "" || res.Case.Classification != "CUS001" {
		t.Fatalf("unmatched: %d %s", code, body)
	}

	// Customer-reported cases sit in the same queue as detected ones.
	_, body = r.call(http.MethodGet, "/v1/exceptions?source=customer", r.as(t, "SUPERVISOR"), "")
	var l struct{ Items []caseJSON }
	_ = json.Unmarshal(body, &l)
	if len(l.Items) != 2 {
		t.Fatalf("customer cases in the queue: %s", body)
	}

	// The joined complaint is in the detected case's history.
	_, body = r.call(http.MethodGet, "/v1/cases/"+c.ID.String(), r.as(t, "SUPERVISOR"), "")
	var d caseDetail
	_ = json.Unmarshal(body, &d)
	found := false
	for _, e := range d.Evidence {
		found = found || e.Kind == "complaint"
	}
	if !found {
		t.Fatalf("complaint evidence missing: %+v", d.Evidence)
	}

	for name, b := range map[string]string{"short text": `{"instruction_ref":"X","text":"no"}`, "not json": `x`, "extra field": `{"text":"a long enough text","admin":1}`} {
		if code, _ := r.call(http.MethodPost, "/v1/complaints", agent, b); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := r.call(http.MethodPost, "/v1/complaints", r.as(t, "EXECUTIVE"), `{"text":"a long enough text"}`); code != http.StatusForbidden {
		t.Errorf("executive: %d", code)
	}
}

func TestCustomerStatusNeverPromisesMoneyBeforeVerification(t *testing.T) {
	r := newCaseRig(t)
	_, c := r.failed(t, "INS-CS1")
	code, body := r.call(http.MethodGet, "/v1/cases/"+c.ID.String()+"/customer-status", r.as(t, "AGENT"), "")
	var st customerStatus
	_ = json.Unmarshal(body, &st)
	if code != http.StatusOK || st.Stage != "investigating" || st.OutcomeVerified || !st.Simulated || len(st.Messages) != 1 || st.Messages[0].Stage != "detected" {
		t.Fatalf("%d %s", code, body)
	}
	for _, bad := range []string{"reversed", "refund", "returned"} {
		if strings.Contains(strings.ToLower(st.Headline+st.Messages[0].Body), bad) {
			t.Errorf("a customer is told %q before anything is verified: %s", bad, body)
		}
	}
	if len(st.Timeline) != 1 || st.Timeline[0].Label != "Problem detected" {
		t.Fatalf("timeline: %+v", st.Timeline)
	}
}
