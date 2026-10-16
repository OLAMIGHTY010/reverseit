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
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/reversal"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

type revRig struct {
	*caseRig
	cfg    *configsvc.Service
	revSvc *reversal.Service
}

func newRevRig(t *testing.T) *revRig {
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
	ing := &txn.Service{}
	rev := &reversal.Service{Pool: pool, Config: cfg, Kill: &killswitch.Service{Audit: app}, Cases: cs, Audit: app, Txn: ing, Policy: policy,
		SoD:    authz.SoD{Conflicts: &authz.PGConflictChecker{Pool: pool}},
		Status: reversal.SimStatus{}, Ledger: reversal.SimLedger{}, Provider: reversal.SimProvider{}}
	logger, _ := quietLogger()
	rg := &revRig{cfg: cfg, revSvc: rev, caseRig: &caseRig{ing: ing, det: &detect.Detector{Cases: cs, Config: cfg},
		auditRig: &auditRig{pool: pool, signer: signer, tenant: dbtest.Tenant(t, pool), appender: app,
			handler: NewRouter(Options{Logger: logger, Verifier: identity.NewVerifier(ks, nil),
				Services: &Services{Pool: pool, Policy: policy, Audit: app, Txn: ing, Cases: cs, Config: cfg, Reversal: rev, KillSwitch: &killswitch.Service{Audit: app}}})}}}
	for k, v := range map[string]int{
		"duplicate.window_seconds": 30, "status_query.freshness_seconds": 120, "waiting_threshold_seconds.NIP": 300,
		"approval.auto_limit_minor": 5_000_000, "reversal.simulated_execution_enabled": 1,
		"approval.limit_minor.SUPERVISOR": 100_000_000, "approval.validity_seconds": 3600, "approval.limit_minor.REVERSAL_OFFICER": 5_000_000,
	} {
		rg.set(t, k, v)
	}
	return rg
}

func (r *revRig) set(t *testing.T, key string, value int) {
	t.Helper()
	raw, _ := json.Marshal(value)
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		v, err := r.cfg.Propose(context.Background(), tx, r.tenant, uuid.New(), configsvc.ProposeInput{Key: key, Value: raw,
			SourceReference: "SYNTHETIC DEMO POLICY - not approved for production", EffectiveFrom: time.Now(), VerifyStatus: "unverified", Reason: "synthetic policy for tests"})
		if err != nil {
			return err
		}
		_, err = r.cfg.Decide(context.Background(), tx, r.tenant, uuid.New(), key, v.Version, true, "")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (r *revRig) confirm(t *testing.T, ref string) {
	t.Helper()
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		if err := reversal.SeedDebit(context.Background(), tx, r.tenant, ref, 1850000, "NGN"); err != nil {
			return err
		}
		return reversal.ScriptStatus(context.Background(), tx, r.tenant, ref, reversal.OutcomeFailure, false, false, 0)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (r *revRig) mfa(t *testing.T, user uuid.UUID, roles ...string) string {
	t.Helper()
	tok, _, err := r.signer.Issue(identity.Principal{UserID: user, TenantID: r.tenant, Roles: roles, MFA: true}, identity.AudienceOps, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	return m
}

func TestReversalRequestApprovalAndVerificationOverHTTP(t *testing.T) {
	r := newRevRig(t)
	_, c := r.failed(t, "REF-H1")
	officer := r.as(t, "REVERSAL_OFFICER")
	body := `{"case_id":"` + c.ID.String() + `","reason_code":"CONFIRMED_FAILED_CREDIT"}`

	// No authoritative status yet: the gate stops it and nothing is created.
	code, out := r.call("POST", "/v1/reversals", officer, body, "Idempotency-Key", "k-1")
	if code != http.StatusConflict {
		t.Fatalf("want 409, got %d %s", code, out)
	}
	m := decode(t, out)
	if m["error"] != "GATE_FAILED" || len(m["failed_checks"].([]any)) == 0 {
		t.Fatalf("expected failed checks: %s", out)
	}

	r.confirm(t, "REF-H1")
	code, out = r.call("GET", "/v1/cases/"+c.ID.String()+"/reversal-eligibility", officer, "")
	if code != 200 || decode(t, out)["verdict"] != "PASS_NEEDS_APPROVAL" {
		t.Fatalf("eligibility: %d %s", code, out)
	}

	code, out = r.call("POST", "/v1/reversals", officer, body, "Idempotency-Key", "k-1")
	if code != http.StatusAccepted {
		t.Fatalf("want 202, got %d %s", code, out)
	}
	rev := decode(t, out)
	if rev["approval_status"] != "PENDING" || rev["simulated"] != true || rev["amount_minor"] != "1850000" || rev["policy_id"] != "reversal.FAIL001.NIP" {
		t.Fatalf("unexpected: %s", out)
	}
	tracks := rev["tracks"].(map[string]any)
	if tracks["financial"] == "VERIFIED" {
		t.Fatalf("must not be verified before approval: %s", out)
	}
	id := rev["id"].(string)

	code, out = r.call("POST", "/v1/reversals", officer, body, "Idempotency-Key", "k-1")
	if code != 200 || decode(t, out)["id"] != id {
		t.Fatalf("repeat must return the same reversal: %d %s", code, out)
	}
	code, _ = r.call("POST", "/v1/reversals", officer, body)
	if code != http.StatusBadRequest {
		t.Fatalf("missing Idempotency-Key must be refused, got %d", code)
	}

	code, out = r.call("GET", "/v1/approvals?status=PENDING", officer, "")
	items := decode(t, out)["items"].([]any)
	if code != 200 || len(items) != 1 {
		t.Fatalf("approvals: %d %s", code, out)
	}
	appr := items[0].(map[string]any)["approval_id"].(string)
	dec := `{"decision":"approved","reason":"checked"}`

	// The requester cannot approve their own request, even with MFA.
	// (officer token has a random user; build one for the requester instead.)
	requesterID := uuid.New()
	req2 := r.mfa(t, requesterID, "SUPERVISOR")
	_, c2 := r.failed(t, "REF-H2")
	r.confirm(t, "REF-H2")
	code, out = r.call("POST", "/v1/reversals", req2, `{"case_id":"`+c2.ID.String()+`","reason_code":"CONFIRMED_FAILED_CREDIT"}`, "Idempotency-Key", "k-2")
	if code != 202 {
		t.Fatalf("%d %s", code, out)
	}
	appr2 := decode(t, out)["approval_id"].(string)
	code, out = r.call("POST", "/v1/approvals/"+appr2+"/decision", req2, dec)
	if code != 403 || decode(t, out)["error"] != "SELF_APPROVAL" {
		t.Fatalf("self approval: %d %s", code, out)
	}

	// Without MFA nobody can approve.
	code, out = r.call("POST", "/v1/approvals/"+appr+"/decision", r.as(t, "SUPERVISOR"), dec)
	if code != 403 || decode(t, out)["error"] != "MFA_REQUIRED" {
		t.Fatalf("no MFA: %d %s", code, out)
	}
	// A role without the grant cannot.
	code, _ = r.call("POST", "/v1/approvals/"+appr+"/decision", r.mfa(t, uuid.New(), "TXN_ANALYST"), dec)
	if code != 403 {
		t.Fatalf("analyst approve: %d", code)
	}

	// A different person with MFA approves: executed and verified from the ledger.
	code, out = r.call("POST", "/v1/approvals/"+appr+"/decision", r.mfa(t, uuid.New(), "SUPERVISOR"), dec)
	if code != 200 {
		t.Fatalf("approve: %d %s", code, out)
	}
	rev = decode(t, out)
	tracks = rev["tracks"].(map[string]any)
	if tracks["financial"] != "VERIFIED" || rev["verified_at"] == nil {
		t.Fatalf("expected verified: %s", out)
	}
	code, out = r.call("POST", "/v1/approvals/"+appr+"/decision", r.mfa(t, uuid.New(), "SUPERVISOR"), dec)
	if code != 409 {
		t.Fatalf("a decided approval cannot be decided again: %d %s", code, out)
	}

	code, out = r.call("GET", "/v1/reversals/"+id, officer, "")
	if code != 200 || decode(t, out)["tracks"].(map[string]any)["financial"] != "VERIFIED" {
		t.Fatalf("get: %d %s", code, out)
	}
	code, out = r.call("POST", "/v1/reversals/"+id+"/advance", officer, "")
	if code != 200 {
		t.Fatalf("advance is safe to repeat: %d %s", code, out)
	}
	code, out = r.call("GET", "/v1/cases/"+c.ID.String(), officer, "")
	d := decode(t, out)
	if code != 200 || len(d["reversals"].([]any)) != 1 || d["case"].(map[string]any)["status"] != "RESOLVED" {
		t.Fatalf("case: %d %s", code, out)
	}

	// Another tenant's operator cannot see it.
	other := dbtest.Tenant(t, r.pool)
	tok := r.token(t, other, uuid.New(), "REVERSAL_OFFICER")
	if code, _ = r.call("GET", "/v1/reversals/"+id, tok, ""); code != 404 {
		t.Fatalf("cross tenant: %d", code)
	}
}

func TestAnUnknownOutcomeCannotBeReversedOverHTTP(t *testing.T) {
	r := newRevRig(t)
	_, c := r.failed(t, "REF-H3")
	r.confirm(t, "REF-H3")
	err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		return reversal.ScriptStatus(context.Background(), tx, r.tenant, "REF-H3", reversal.OutcomeUnknown, false, false, 0)
	})
	if err != nil {
		t.Fatal(err)
	}
	code, out := r.call("POST", "/v1/reversals", r.as(t, "REVERSAL_OFFICER"), `{"case_id":"`+c.ID.String()+`","reason_code":"CONFIRMED_FAILED_CREDIT"}`, "Idempotency-Key", "k-u")
	if code != http.StatusConflict || decode(t, out)["error"] != "GATE_FAILED" {
		t.Fatalf("unknown outcome must be blocked: %d %s", code, out)
	}
}

func TestReconciliationAndSummaryReflectVerifiedAndOrphanPostings(t *testing.T) {
	r := newRevRig(t)
	_, c := r.failed(t, "REF-H4")
	r.confirm(t, "REF-H4")
	body := `{"case_id":"` + c.ID.String() + `","reason_code":"CONFIRMED_FAILED_CREDIT"}`
	code, out := r.call("POST", "/v1/reversals", r.as(t, "REVERSAL_OFFICER"), body, "Idempotency-Key", "k-r")
	if code != 202 {
		t.Fatalf("%d %s", code, out)
	}
	code, out = r.call("POST", "/v1/approvals/"+decode(t, out)["approval_id"].(string)+"/decision", r.mfa(t, uuid.New(), "SUPERVISOR"), `{"decision":"approved"}`)
	if code != 200 {
		t.Fatalf("%d %s", code, out)
	}
	// A credit the system never asked for.
	if _, err := r.pool.Exec(context.Background(), `INSERT INTO sim_ledger_entries (tenant_id, instruction_ref, kind, reversal_ref, amount_minor, currency)
		VALUES ($1, 'GHOST-1', 'REVERSAL_CREDIT', 'RV-GHOST', 100, 'NGN')`, r.tenant); err != nil {
		t.Fatal(err)
	}
	code, out = r.call("GET", "/v1/reconciliation/reversals", r.as(t, "REVERSAL_OFFICER"), "")
	counts := decode(t, out)["counts"].(map[string]any)
	if code != 200 || counts["MATCHED"] != float64(1) || counts["ORPHAN_POSTING"] != float64(1) {
		t.Fatalf("recon: %d %s", code, out)
	}
	code, out = r.call("GET", "/v1/reports/summary", r.as(t, "REVERSAL_OFFICER"), "")
	m := decode(t, out)
	if code != 200 || m["reversals_verified"] != float64(1) || m["reversals_verified_value_minor"] != "1850000" || m["simulated"] != true {
		t.Fatalf("summary: %d %s", code, out)
	}
	if m["resolved_cases"] != float64(1) {
		t.Fatalf("the verified reversal should have resolved the case: %s", out)
	}
	if code, _ = r.call("GET", "/v1/reports/summary", "", ""); code != 401 {
		t.Fatalf("unauthenticated: %d", code)
	}
}

func TestAMissingApprovalPolicyIsReportedAsItsOwnError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeReversalError(rec, httptest.NewRequest("POST", "/v1/approvals/x/decision", nil), reversal.ErrNoApprovalPolicy)
	if rec.Code != http.StatusConflict || decode(t, rec.Body.Bytes())["error"] != "APPROVAL_POLICY_MISSING" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func (r *revRig) lapse(t *testing.T, revID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE approvals SET expires_at = now() - interval '1 second' WHERE tenant_id = $1 AND reversal_id = $2::uuid`, r.tenant, revID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAnApprovalThatLapsesWhileFrozenNeedsAFreshOneThroughTheAPI(t *testing.T) {
	r := newRevRig(t)
	_, c := r.failed(t, "REF-H5")
	r.confirm(t, "REF-H5")
	officer := r.mfa(t, uuid.New(), "REVERSAL_OFFICER")
	supervisor := r.mfa(t, uuid.New(), "SUPERVISOR")
	code, out := r.call("POST", "/v1/reversals", officer, `{"case_id":"`+c.ID.String()+`","reason_code":"CONFIRMED_FAILED_CREDIT"}`, "Idempotency-Key", "k-ex")
	if code != 202 {
		t.Fatalf("%d %s", code, out)
	}
	rev := decode(t, out)
	id, firstApproval := rev["id"].(string), rev["approval_id"].(string)

	// While a fresh approval is already waiting, asking again returns the same one.
	code, out = r.call("POST", "/v1/reversals/"+id+"/reapproval", officer, `{"reason":"early"}`)
	if code != 202 || decode(t, out)["approval_id"] != firstApproval {
		t.Fatalf("a waiting approval is returned unchanged: %d %s", code, out)
	}

	// Approve while submissions are frozen: the approval is given and recorded, nothing runs.
	if code, out = r.call("POST", "/v1/killswitch", supervisor, `{"action":"freeze","scope":"global","reason":"incident drill now"}`); code != 200 {
		t.Fatalf("freeze: %d %s", code, out)
	}
	code, out = r.call("POST", "/v1/approvals/"+firstApproval+"/decision", supervisor, `{"decision":"approved","reason":"ok"}`)
	got := decode(t, out)
	if code != 200 || got["approval_expires_at"] == nil || got["parked_at"] == nil || got["tracks"].(map[string]any)["accepted"] != "PENDING" {
		t.Fatalf("expected an approved, parked reversal: %d %s", code, out)
	}
	r.lapse(t, id)

	// Unfreeze (one asks, another approves), then the sweep records the lapse and submits nothing.
	code, out = r.call("POST", "/v1/killswitch", officer, `{"action":"request_unfreeze","scope":"global","reason":"incident closed now"}`)
	if code != 200 {
		t.Fatalf("%d %s", code, out)
	}
	reqID := int64(decode(t, out)["pending_unfreeze"].([]any)[0].(map[string]any)["request_id"].(float64))
	if code, out = r.call("POST", "/v1/killswitch", supervisor, fmt.Sprintf(`{"action":"approve_unfreeze","request_id":%d,"reason":"confirmed closed"}`, reqID)); code != 200 {
		t.Fatalf("%d %s", code, out)
	}
	if _, err := r.revSvc.Sweep(context.Background(), r.tenant); err != nil {
		t.Fatal(err)
	}
	code, out = r.call("GET", "/v1/reversals/"+id, officer, "")
	got = decode(t, out)
	if code != 200 || got["approval_status"] != "EXPIRED" || got["tracks"].(map[string]any)["accepted"] != "PENDING" {
		t.Fatalf("expected an expired, unsubmitted reversal: %d %s", code, out)
	}
	// A decision cannot revive it.
	if code, out = r.call("POST", "/v1/approvals/"+firstApproval+"/decision", supervisor, `{"decision":"approved","reason":"again"}`); code != 409 {
		t.Fatalf("the old approval was decided once: %d %s", code, out)
	}

	// Renewal needs the request grant and a login.
	if code, _ = r.call("POST", "/v1/reversals/"+id+"/reapproval", r.mfa(t, uuid.New(), "TXN_ANALYST"), `{"reason":"x"}`); code != 403 {
		t.Fatalf("a role without the grant: %d", code)
	}
	if code, _ = r.call("POST", "/v1/reversals/"+id+"/reapproval", "", `{"reason":"x"}`); code != 401 {
		t.Fatalf("unauthenticated: %d", code)
	}
	if code, _ = r.call("POST", "/v1/reversals/"+id+"/reapproval", officer, `{"reason":""}`); code != 422 {
		t.Fatalf("a reason is required: %d", code)
	}
	code, out = r.call("POST", "/v1/reversals/"+id+"/reapproval", officer, `{"reason":"customer is still waiting"}`)
	fresh := decode(t, out)
	if code != 202 || fresh["approval_status"] != "PENDING" || fresh["approval_id"] == firstApproval {
		t.Fatalf("%d %s", code, out)
	}
	// The person who asked cannot approve it; a different supervisor can, and it completes.
	if code, out = r.call("POST", "/v1/approvals/"+fresh["approval_id"].(string)+"/decision", officer, `{"decision":"approved","reason":"mine"}`); code != 403 {
		t.Fatalf("the requester must not approve: %d %s", code, out)
	}
	code, out = r.call("POST", "/v1/approvals/"+fresh["approval_id"].(string)+"/decision", r.mfa(t, uuid.New(), "SUPERVISOR"), `{"decision":"approved","reason":"renewed"}`)
	done := decode(t, out)
	if code != 200 || done["tracks"].(map[string]any)["financial"] != "VERIFIED" || done["id"] != id {
		t.Fatalf("%d %s", code, out)
	}
}

func TestAnUnknownOutcomeAppearsInTheReviewQueueWithEvidenceAndCannotBeReleased(t *testing.T) {
	r := newRevRig(t)
	at := time.Now().Add(-40 * time.Minute) // well apart from the other transfer, so they are not a duplicate pair
	r.event(t, "REF-RV1", "DEBIT_POSTED", at)
	r.event(t, "REF-RV1", "TRANSFER_SENT", at.Add(time.Second))
	id := r.event(t, "REF-RV1", "TRANSFER_RESPONSE_TIMEOUT", at.Add(30*time.Second))
	var c cases.Case
	if err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
		fs, err := r.det.Evaluate(context.Background(), tx, r.tenant, id)
		if err == nil && len(fs) == 1 {
			c = fs[0].Case
		}
		return err
	}); err != nil || c.Classification != "UNK001" {
		t.Fatalf("%v %+v", err, c)
	}
	// A confirmed failure on another transfer is an approval, not a review item.
	_, failedCase := r.failed(t, "REF-RV2")

	supervisor := r.as(t, "SUPERVISOR")
	code, out := r.call("GET", "/v1/review-items", supervisor, "")
	if code != 200 {
		t.Fatalf("%d %s", code, out)
	}
	var list struct {
		Items []reviewItemJSON `json:"items"`
	}
	_ = json.Unmarshal(out, &list)
	var found *reviewItemJSON
	for i := range list.Items {
		if list.Items[i].CaseID == failedCase.ID.String() {
			t.Fatalf("a confirmed failure was listed as a review item: %+v", list.Items[i])
		}
		if list.Items[i].CaseID == c.ID.String() {
			found = &list.Items[i]
		}
	}
	if found == nil {
		t.Fatalf("the unknown case is not in the review queue: %s", out)
	}
	if found.Kind != "REVIEW" || found.ReleaseAllowed || found.Classification != "UNK001" || len(found.Evidence) == 0 || found.Transaction == nil ||
		found.Transaction.LifecycleState != "UNKNOWN" || !strings.Contains(found.BlockedBecause, "UNKNOWN") || found.Gate.Verdict != reversal.Blocked || !found.Simulated {
		t.Fatalf("%+v", found)
	}
	// It cannot be released: asking for a reversal is refused by the gate.
	code, out = r.call("POST", "/v1/reversals", r.as(t, "REVERSAL_OFFICER"), `{"case_id":"`+c.ID.String()+`","reason_code":"CONFIRMED_FAILED_CREDIT"}`, "Idempotency-Key", "k-review")
	if code != http.StatusConflict || decode(t, out)["error"] != "GATE_FAILED" {
		t.Fatalf("an unknown outcome was released: %d %s", code, out)
	}
	// Only roles with the full queue scope see review items.
	if code, _ := r.call("GET", "/v1/review-items", r.as(t, "AGENT"), ""); code != http.StatusForbidden {
		t.Fatalf("agent: %d", code)
	}
	if code, _ := r.call("GET", "/v1/review-items", r.as(t, "EXECUTIVE"), ""); code != http.StatusForbidden {
		t.Fatalf("executive: %d", code)
	}
	if code, _ := r.call("GET", "/v1/review-items", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", code)
	}
}
