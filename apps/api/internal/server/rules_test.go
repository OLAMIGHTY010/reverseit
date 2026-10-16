package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smartreversal/smartreversal/apps/api/internal/rules"
)

const apiRule = `{"rule_id":"DET-API-1","version":1,"code":"FAIL001","risk":"HIGH","root_cause":"beneficiary_side_failure",
 "explanation":"The beneficiary credit failed after the debit posted.",
 "all":[{"field":"lifecycle_state","op":"eq","value":"CONFIRMED_FAILURE"}]}`

func TestARuleIsWrittenTestedRequestedAndApprovedByDifferentPeopleThroughTheAPI(t *testing.T) {
	r := newIngestRig(t)
	now := time.Now()
	for i, typ := range []string{"DEBIT_POSTED", "TRANSFER_SENT", "CREDIT_FAILED"} {
		ref := "EV-A1000" + string(rune('0'+i))
		if rec := r.post(t, ref, ev("INS-A10000", typ, ref, now.Add(time.Duration(i)*time.Second)), "assertion"); rec.Code != http.StatusAccepted {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	}
	author := r.opToken(t, false, "ADMIN_INTEGRATION") // draft
	proposer := r.opToken(t, false, "SUPERVISOR")      // propose
	approver := r.opToken(t, true, "RISK_COMPLIANCE")  // approve, with MFA
	approverNoMFA := r.opToken(t, false, "RISK_COMPLIANCE")
	analyst := r.opToken(t, true, "TXN_ANALYST")

	if rec := r.do(http.MethodPost, "/v1/rules", apiRule, analyst); rec.Code != http.StatusForbidden {
		t.Fatalf("analyst: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/rules", apiRule, approver); rec.Code != http.StatusForbidden {
		t.Fatalf("the approver wrote a rule: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/rules", `{"rule_id":"DET-API-9","version":1,"code":"FAIL001","root_cause":"some_cause","explanation":"Waited too long, so failed.","all":[{"field":"rail","op":"eq","value":"NIP"}]}`, author); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an unsafe rule: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodPost, "/v1/rules", apiRule, author); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	base := "/v1/rules/DET-API-1/versions/1"
	if rec := r.do(http.MethodPost, base+"/activation", `{"reason":"ready to go"}`, proposer); rec.Code != http.StatusConflict {
		t.Fatalf("activation before shadow: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, base+"/shadow", ``, author); rec.Code != http.StatusOK {
		t.Fatalf("shadow: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodPost, base+"/activation", `{"reason":"ready to go"}`, proposer); rec.Code != http.StatusConflict {
		t.Fatalf("activation with no evaluation: %d", rec.Code)
	}
	rec := r.do(http.MethodPost, base+"/backtest", ``, author)
	var st rules.Stats
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if rec.Code != http.StatusOK || st.Evaluated != 1 || st.Matched != 1 {
		t.Fatalf("backtest: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodPost, base+"/activation", `{"reason":"ready to go"}`, author); rec.Code != http.StatusForbidden {
		t.Fatalf("the draft scope asked for activation: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, base+"/activation", `{"reason":"backtest matched the one failure"}`, proposer); rec.Code != http.StatusOK {
		t.Fatalf("request: %d %s", rec.Code, rec.Body.String())
	}
	decision := `{"approve":true,"reason":"reviewed the backtest"}`
	if rec := r.do(http.MethodPost, base+"/decision", decision, approverNoMFA); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "MFA_REQUIRED") {
		t.Fatalf("no MFA: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodPost, base+"/decision", decision, r.opToken(t, true, "SUPERVISOR")); rec.Code != http.StatusForbidden {
		t.Fatalf("a proposer approved: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, base+"/decision", `{"reason":"x"}`, approver); rec.Code != http.StatusBadRequest {
		t.Fatalf("no decision: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, base+"/decision", decision, approver); rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	rec = r.do(http.MethodGet, "/v1/rules/DET-API-1", "", approver)
	var got struct {
		Items []struct {
			Status string `json:"status"`
			Stats  struct {
				Matched int `json:"matched"`
			} `json:"shadow_and_backtest_stats"`
		}
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != http.StatusOK || len(got.Items) != 1 || got.Items[0].Status != "active" || got.Items[0].Stats.Matched != 1 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodGet, "/v1/rules/NOPE-1", "", approver); rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/rules/DET-API-1/disable", `{"reason":"x"}`, proposer); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a disable with no reason: %d", rec.Code)
	}
	if rec := r.do(http.MethodPost, "/v1/rules/DET-API-1/disable", `{"reason":"it matched a test channel"}`, proposer); rec.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodPost, "/v1/rules/DET-API-1/rollback", `{"to_version":1,"reason":"turn it back on"}`, proposer); rec.Code != http.StatusOK {
		t.Fatalf("rollback: %d %s", rec.Code, rec.Body.String())
	}
	if rec := r.do(http.MethodPost, base+"/decision", decision, r.opToken(t, true, "RISK_COMPLIANCE")); rec.Code != http.StatusOK {
		t.Fatalf("approve the rollback: %d %s", rec.Code, rec.Body.String())
	}
}
