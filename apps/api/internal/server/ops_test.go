package server

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/alerts"
	"github.com/smartreversal/smartreversal/apps/api/internal/audit"
	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/incidents"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/routing"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

func TestAlertsAndIncidentsAreReadAndActedOnOnlyByThosePermittedTo(t *testing.T) {
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
	al := &alerts.Service{Routing: &routing.Service{Audit: app}, Config: cfg, Audit: app}
	inc := &incidents.Service{Config: cfg, Alerts: al, Audit: app}
	logger, _ := quietLogger()
	rig := &auditRig{pool: pool, signer: signer, tenant: dbtest.Tenant(t, pool), appender: app,
		handler: NewRouter(Options{Logger: logger, Verifier: identity.NewVerifier(ks, nil),
			Services: &Services{Pool: pool, Policy: policy, Audit: app, Config: cfg, Txn: &txn.Service{}, Cases: &cases.Service{Audit: app, Config: cfg}, Alerts: al, Incidents: inc}})}

	approve := func(key string, v int) {
		raw, _ := json.Marshal(v)
		err := pool.WithTenant(context.Background(), rig.tenant, func(tx pgx.Tx) error {
			ver, err := cfg.Propose(context.Background(), tx, rig.tenant, uuid.New(), configsvc.ProposeInput{Key: key, Value: raw, SourceReference: "SYNTHETIC DEMO POLICY - not approved for production",
				EffectiveFrom: time.Now().Add(500 * time.Millisecond), VerifyStatus: "unverified", Reason: "synthetic value for a test"})
			if err != nil {
				return err
			}
			_, err = cfg.Decide(context.Background(), tx, rig.tenant, uuid.New(), key, ver.Version, true, "")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	approve("incident.group_window_minutes", 30)
	approve("incident.min_group_size", 3)
	ing := &txn.Service{}
	for i := 0; i < 4; i++ {
		err := pool.WithTenant(context.Background(), rig.tenant, func(tx pgx.Tx) error {
			res, err := ing.Ingest(context.Background(), tx, rig.tenant, txn.Envelope{SourceSystem: "SWITCH_O", SourceEventRef: fmt.Sprintf("EV-OPSAPI-%d", i), EventType: txn.EvDebitPosted, SourceTS: time.Now().Add(-time.Minute),
				Txn: txn.Core{InstructionRef: fmt.Sprintf("INS-OPSAPI%d", i), Channel: "MOBILE", Rail: "NIP", AmountMinor: 1000, Currency: "NGN", DebitAccountToken: tokA}})
			if err != nil {
				return err
			}
			_, err = tx.Exec(context.Background(), `INSERT INTO cases (tenant_id, case_ref, classification, status, priority, source, txn_id, opened_at, root_cause)
				VALUES ($1,$2,'UNK001','DETECTED','HIGH','bot',$3,now() - interval '1 minute','switch_response_failure')`, rig.tenant, fmt.Sprintf("CASE-OA-%d", i), res.TxnID)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(700 * time.Millisecond)
	_ = pool.WithTenant(context.Background(), rig.tenant, func(tx pgx.Tx) error {
		if _, err := inc.Group(context.Background(), tx, rig.tenant); err != nil {
			t.Fatal(err)
		}
		_, err := al.Sweep(context.Background(), tx, rig.tenant, incidents.Covering())
		return err
	})

	call := func(method, path, body, tok string) *recorder {
		return serve(rig.handler, newReq(method, path, body, tok))
	}
	sup := rig.token(t, rig.tenant, uuid.New(), "SUPERVISOR")
	head := rig.token(t, rig.tenant, uuid.New(), "HEAD_OPS")
	analyst := rig.token(t, rig.tenant, uuid.New(), "TXN_ANALYST")
	exec := rig.token(t, rig.tenant, uuid.New(), "EXECUTIVE")
	agent := rig.token(t, rig.tenant, uuid.New(), "AGENT")

	if rec := call("GET", "/v1/alerts", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	for _, tk := range []string{exec, agent} {
		if rec := call("GET", "/v1/alerts", "", tk); rec.Code != http.StatusForbidden {
			t.Fatalf("an aggregate-only or agent role read alerts: %d", rec.Code)
		}
	}
	rec := call("GET", "/v1/alerts", "", sup)
	var list struct{ Items []alerts.Alert }
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list.Items) != 1 || list.Items[0].Kind != alerts.WidespreadIncident || list.Items[0].TargetRole != "HEAD_OPS" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	alertID := list.Items[0].ID.String()
	// A supervisor is not the alert's role; the head of operations is.
	if rec := call("POST", "/v1/alerts/"+alertID+"/acknowledge", "", sup); rec.Code != http.StatusForbidden {
		t.Fatalf("a supervisor acknowledged a head-of-operations alert: %d", rec.Code)
	}
	if rec := call("POST", "/v1/alerts/"+alertID+"/acknowledge", "", head); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := call("POST", "/v1/alerts/"+alertID+"/acknowledge", "", head); rec.Code != http.StatusConflict {
		t.Fatalf("%d", rec.Code)
	}
	if rec := call("POST", "/v1/alerts/"+uuid.NewString()+"/acknowledge", "", head); rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}

	rec = call("GET", "/v1/incidents", "", exec)
	var il struct{ Items []incidents.Incident }
	_ = json.Unmarshal(rec.Body.Bytes(), &il)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("an aggregate-only role saw incidents: %d", rec.Code)
	}
	rec = call("GET", "/v1/incidents", "", analyst)
	_ = json.Unmarshal(rec.Body.Bytes(), &il)
	if rec.Code != http.StatusOK || len(il.Items) != 1 || il.Items[0].AffectedCount != 4 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	ref := il.Items[0].Ref
	rec = call("GET", "/v1/incidents/"+ref, "", analyst)
	var det incidentDetail
	_ = json.Unmarshal(rec.Body.Bytes(), &det)
	if rec.Code != http.StatusOK || len(det.CaseIDs) != 4 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := call("GET", "/v1/incidents/INC-0000-00000", "", analyst); rec.Code != http.StatusNotFound {
		t.Fatalf("%d", rec.Code)
	}
	if rec := call("POST", "/v1/incidents/"+ref+"/causation", `{"causation_status":"probable","suspected_cause":"SWITCH_O timing out","note":"timeouts began with its deploy"}`, agent); rec.Code != http.StatusForbidden {
		t.Fatalf("an agent set a cause: %d", rec.Code)
	}
	if rec := call("POST", "/v1/incidents/"+ref+"/causation", `{"causation_status":"probable","suspected_cause":"SWITCH_O timing out","note":"timeouts began with its deploy"}`, sup); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := call("POST", "/v1/incidents/"+ref+"/causation", `{"causation_status":"correlated","note":"going back"}`, sup); rec.Code != http.StatusConflict {
		t.Fatalf("a cause was downgraded: %d", rec.Code)
	}
	if rec := call("POST", "/v1/incidents/"+ref+"/resolve", `{"review_note":"short"}`, sup); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("%d", rec.Code)
	}
	if rec := call("POST", "/v1/incidents/"+ref+"/resolve", `{"review_note":"The switch recovered; nothing was lost; a freshness check was added."}`, sup); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}
