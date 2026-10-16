package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
	"github.com/smartreversal/smartreversal/apps/api/internal/correlate"
	"github.com/smartreversal/smartreversal/apps/api/internal/feed"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/dbtest"
	"github.com/smartreversal/smartreversal/apps/api/internal/rules"
	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

const tokA = "acct_0123456789abcdef0123456789abcdef"
const tokB = "acct_fedcba9876543210fedcba9876543210"

type ingestRig struct {
	*auditRig
	svcID   uuid.UUID
	svcKey  ed25519.PrivateKey
	tenantB uuid.UUID
}

func newIngestRig(t *testing.T) *ingestRig {
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
	logger, _ := quietLogger()

	r := &ingestRig{}
	tenant := dbtest.Tenant(t, pool)
	pub, sk, _ := ed25519.GenerateKey(rand.Reader)
	reg := &servauth.Registry{Pool: pool, Audit: app}
	r.svcID, err = reg.Register(context.Background(), tenant, uuid.New(), "simulator", pub, []string{"events:write"})
	if err != nil {
		t.Fatal(err)
	}
	r.svcKey = sk
	r.tenantB = dbtest.Tenant(t, pool)
	r.auditRig = &auditRig{pool: pool, signer: signer, tenant: tenant, appender: app,
		handler: NewRouter(Options{Logger: logger, Verifier: identity.NewVerifier(ks, nil),
			ServiceVerifier: &servauth.Verifier{Pool: pool},
			Services:        &Services{Pool: pool, Policy: policy, Audit: app, Txn: &txn.Service{}, Feed: &feed.Importer{Pool: pool, Txn: &txn.Service{}}, Correlate: &correlate.Matcher{Txn: &txn.Service{}}, Rules: &rules.Engine{Store: &rules.Store{Audit: app}}}})}
	return r
}

func (r *ingestRig) post(t *testing.T, key string, body any, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	var raw string
	switch b := body.(type) {
	case string:
		raw = b
	default:
		j, _ := json.Marshal(b)
		raw = string(j)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(raw))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if bearer == "assertion" {
		a, err := servauth.NewAssertion(r.tenant, r.svcID, r.svcKey, time.Now(), time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		bearer = a
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func ev(ref, typ, evRef string, at time.Time) map[string]any {
	return map[string]any{
		"source_system": "SWITCH_A", "source_event_ref": evRef, "event_type": typ, "source_ts": at.UTC().Format(time.RFC3339Nano),
		"txn": map[string]any{"instruction_ref": ref, "channel": "MOBILE", "rail": "NIP", "amount_minor": "1850000", "currency": "NGN",
			"debit_account_token": tokA, "beneficiary_bank": "TEST BANK", "beneficiary_token": tokB, "session_id": "S-" + ref},
	}
}

func TestEventEndpointAcceptsRepeatsAndRefuses(t *testing.T) {
	r := newIngestRig(t)
	now := time.Now()
	e := ev("INS-100001", "DEBIT_POSTED", "EV-100001", now)

	rec := r.post(t, "EV-100001", e, "assertion")
	var out eventResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != http.StatusAccepted || out.LifecycleState != "DEBITED" || out.Duplicate {
		t.Fatalf("first: %d %s", rec.Code, rec.Body.String())
	}
	rec = r.post(t, "EV-100001", e, "assertion")
	var again eventResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &again)
	if rec.Code != http.StatusOK || !again.Duplicate || again.EventID != out.EventID {
		t.Fatalf("repeat: %d %s", rec.Code, rec.Body.String())
	}
	changed := ev("INS-100001", "CREDIT_FAILED", "EV-100001", now)
	if rec := r.post(t, "EV-100001", changed, "assertion"); rec.Code != http.StatusConflict {
		t.Fatalf("conflict: %d %s", rec.Code, rec.Body.String())
	}
	mismatch := ev("INS-100001", "TRANSFER_SENT", "EV-100002", now)
	mismatch["txn"].(map[string]any)["amount_minor"] = "5"
	if rec := r.post(t, "EV-100002", mismatch, "assertion"); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("core mismatch: %d %s", rec.Code, rec.Body.String())
	}
}

func TestEventEndpointValidation(t *testing.T) {
	r := newIngestRig(t)
	now := time.Now()
	raw := ev("INS-100002", "DEBIT_POSTED", "EV-200001", now)
	raw["txn"].(map[string]any)["debit_account_token"] = "0123456789"
	numeric := ev("INS-100002", "DEBIT_POSTED", "EV-200002", now)
	numeric["txn"].(map[string]any)["amount_minor"] = 18500.5 // a fraction is never an amount in minor units
	extra := ev("INS-100002", "DEBIT_POSTED", "EV-200003", now)
	extra["admin"] = true
	badType := ev("INS-100002", "MONEY_APPEARED", "EV-200004", now)

	for name, c := range map[string]struct {
		key  string
		body any
		want int
	}{
		"a raw account number":       {"EV-200001", raw, http.StatusUnprocessableEntity},
		"amount with a fraction":     {"EV-200002", numeric, http.StatusUnprocessableEntity},
		"an unknown field":           {"EV-200003", extra, http.StatusUnprocessableEntity},
		"an unknown event type":      {"EV-200004", badType, http.StatusUnprocessableEntity},
		"not JSON":                   {"EV-200005", "nope", http.StatusUnprocessableEntity},
		"no idempotency key":         {"", ev("INS-100002", "DEBIT_POSTED", "EV-200006", now), http.StatusBadRequest},
		"key different from the ref": {"OTHER-KEY-1", ev("INS-100002", "DEBIT_POSTED", "EV-200007", now), http.StatusBadRequest},
	} {
		if rec := r.post(t, c.key, c.body, "assertion"); rec.Code != c.want {
			t.Errorf("%s: %d, want %d (%s)", name, rec.Code, c.want, rec.Body.String())
		}
	}
	var n int
	if err := r.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM transactions WHERE tenant_id = $1), (SELECT count(*) FROM quarantined_events WHERE tenant_id = $1)`, r.tenant).Scan(&n, new(int)); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("invalid events created %d transactions", n)
	}
	var q int
	_ = r.pool.QueryRow(context.Background(), `SELECT count(*) FROM quarantined_events WHERE tenant_id = $1`, r.tenant).Scan(&q)
	if q < 4 {
		t.Fatalf("only %d events were quarantined", q)
	}
}

func TestEventEndpointNeedsAServiceAssertion(t *testing.T) {
	r := newIngestRig(t)
	e := ev("INS-100003", "DEBIT_POSTED", "EV-300001", time.Now())
	operator := r.token(t, r.tenant, uuid.New(), "SUPERVISOR")
	for name, bearer := range map[string]string{"no token": "", "garbage": "nope", "an operator token": operator} {
		if rec := r.post(t, "EV-300001", e, bearer); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if rec := r.post(t, "EV-300001", e, "assertion"); rec.Code != http.StatusAccepted {
		t.Fatalf("control: %d", rec.Code)
	}
}

func TestTransactionsAreListedAndFetchedByOperatorsOnly(t *testing.T) {
	r := newIngestRig(t)
	now := time.Now()
	for i := 0; i < 5; i++ {
		ref := fmt.Sprintf("INS-40000%d", i)
		if rec := r.post(t, fmt.Sprintf("EV-40000%d", i), ev(ref, "DEBIT_POSTED", fmt.Sprintf("EV-40000%d", i), now.Add(time.Duration(i)*time.Second)), "assertion"); rec.Code != http.StatusAccepted {
			t.Fatal(rec.Body.String())
		}
	}
	r.post(t, "EV-400009", ev("INS-400000", "CREDIT_FAILED", "EV-400009", now.Add(time.Minute)), "assertion")

	tok := r.token(t, r.tenant, uuid.New(), "TXN_ANALYST")
	get := func(path, tk string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if tk != "" {
			req.Header.Set("Authorization", "Bearer "+tk)
		}
		rec := httptest.NewRecorder()
		r.handler.ServeHTTP(rec, req)
		return rec
	}

	var page struct {
		Items      []transactionJSON `json:"items"`
		NextCursor string            `json:"next_cursor"`
	}
	rec := get("/v1/transactions?limit=2", tok)
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if rec.Code != http.StatusOK || len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("page 1: %d %s", rec.Code, rec.Body.String())
	}
	if page.Items[0].InstructionRef != "INS-400000" || page.Items[0].LifecycleState != "CONFIRMED_FAILURE" || page.Items[0].AmountMinor != "1850000" || !page.Items[0].Simulated {
		t.Fatalf("newest first: %+v", page.Items[0])
	}
	seen := map[string]bool{}
	cursor := ""
	for i := 0; i < 5; i++ {
		var pg struct {
			Items      []transactionJSON `json:"items"`
			NextCursor string            `json:"next_cursor"`
		}
		path := "/v1/transactions?limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		_ = json.Unmarshal(get(path, tok).Body.Bytes(), &pg)
		for _, it := range pg.Items {
			seen[it.ID] = true
		}
		if pg.NextCursor == "" {
			break
		}
		cursor = pg.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("paging saw %d of 5", len(seen))
	}

	var filtered struct{ Items []transactionJSON }
	_ = json.Unmarshal(get("/v1/transactions?state=CONFIRMED_FAILURE", tok).Body.Bytes(), &filtered)
	if len(filtered.Items) != 1 {
		t.Fatalf("filter: %d", len(filtered.Items))
	}

	var detail struct {
		Transaction transactionJSON        `json:"transaction"`
		Events      []transactionEventJSON `json:"events"`
	}
	rec = get("/v1/transactions/"+page.Items[0].ID, tok)
	_ = json.Unmarshal(rec.Body.Bytes(), &detail)
	if rec.Code != http.StatusOK || len(detail.Events) != 2 || detail.Events[0].EventType != "DEBIT_POSTED" {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get("/v1/transactions/"+uuid.NewString(), tok); rec.Code != http.StatusNotFound {
		t.Errorf("unknown id: %d", rec.Code)
	}
	if rec := get("/v1/transactions", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: %d", rec.Code)
	}
	for _, role := range []string{"EXECUTIVE", "AGENT"} {
		if rec := get("/v1/transactions", r.token(t, r.tenant, uuid.New(), role)); rec.Code != http.StatusForbidden {
			t.Errorf("%s: %d", role, rec.Code)
		}
	}
	// Another institution sees none of it.
	other := r.token(t, r.tenantB, uuid.New(), "TXN_ANALYST")
	var none struct{ Items []transactionJSON }
	_ = json.Unmarshal(get("/v1/transactions", other).Body.Bytes(), &none)
	if len(none.Items) != 0 {
		t.Fatalf("tenant B sees %d transactions", len(none.Items))
	}
	if rec := get("/v1/transactions/"+page.Items[0].ID, other); rec.Code != http.StatusNotFound {
		t.Errorf("tenant B fetched tenant A's transaction: %d", rec.Code)
	}
	if rec := get("/v1/transactions?cursor=zzz", tok); rec.Code != http.StatusBadRequest {
		t.Errorf("bad cursor: %d", rec.Code)
	}
}

// The event exactly as the contract's example shows it: a list of refs, a bare
// integer amount, institution_id and raw_payload_hash, and the schema header.
func TestTheContractExampleEventIsAcceptedAsWritten(t *testing.T) {
	r := newIngestRig(t)
	var code string
	if err := r.pool.QueryRow(t.Context(), `SELECT code FROM institutions WHERE id = $1`, r.tenant).Scan(&code); err != nil {
		t.Fatal(err)
	}
	body := `{"institution_id":"` + code + `","source_system":"SWITCH_A","source_event_ref":"SW-20261014-000184233","event_type":"TRANSFER_RESPONSE_TIMEOUT",
	 "source_ts":"` + time.Now().Add(-time.Minute).UTC().Format(time.RFC3339) + `","seq":3,
	 "txn":{"instruction_ref":"NIP-DEMO-7781002","channel":"MOBILE","rail":"NIP","amount_minor":1850000,"currency":"NGN","debit_account_token":"` + tokA + `"},
	 "refs":[{"type":"SESSION_ID","value":"DEMO0000000000000000SESSION01"}],"raw_payload_hash":"sha256:7c1e0000aaaa"}`
	post := func(body, version string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body))
		req.Header.Set("Idempotency-Key", "SW-20261014-000184233")
		if version != "" {
			req.Header.Set("X-Schema-Version", version)
		}
		a, _ := servauth.NewAssertion(r.tenant, r.svcID, r.svcKey, time.Now(), time.Minute)
		req.Header.Set("Authorization", "Bearer "+a)
		rec := httptest.NewRecorder()
		r.handler.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(body, "1.2"); rec.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := post(body, "1.2"); rec.Code != http.StatusOK {
		t.Fatalf("repeat: %d %s", rec.Code, rec.Body.String())
	}
	var sess string
	_ = r.pool.WithTenant(t.Context(), r.tenant, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT ref_value FROM transaction_refs WHERE ref_type = 'SESSION_ID' LIMIT 1`).Scan(&sess)
	})
	if sess != "DEMO0000000000000000SESSION01" {
		t.Fatalf("the reference list was not stored: %q", sess)
	}

	// A body for a different institution is refused and kept for review.
	wrong := strings.Replace(body, `"institution_id":"`+code+`"`, `"institution_id":"SOMEONE-ELSE"`, 1)
	wrong = strings.Replace(wrong, "SW-20261014-000184233", "SW-20261014-000184234", 1)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(wrong))
	req.Header.Set("Idempotency-Key", "SW-20261014-000184234")
	a, _ := servauth.NewAssertion(r.tenant, r.svcID, r.svcKey, time.Now(), time.Minute)
	req.Header.Set("Authorization", "Bearer "+a)
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("another institution's id: %d %s", rec.Code, rec.Body.String())
	}
	var q int
	_ = r.pool.QueryRow(t.Context(), `SELECT count(*) FROM quarantined_events WHERE tenant_id = $1 AND payload IS NOT NULL`, r.tenant).Scan(&q)
	if q != 1 {
		t.Fatalf("%d quarantined", q)
	}

	// An unsupported schema version is refused before anything is read.
	for _, v := range []string{"2.0", "1", "1.x", "abc"} {
		if rec := post(body, v); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "UNSUPPORTED_SCHEMA_VERSION") {
			t.Errorf("version %q: %d %s", v, rec.Code, rec.Body.String())
		}
	}

	// The same reference with different content is a conflict and raises a data-quality record.
	changed := strings.Replace(body, `"amount_minor":1850000`, `"amount_minor":1850001`, 1)
	if rec := post(changed, "1.2"); rec.Code != http.StatusConflict {
		t.Fatalf("conflict: %d %s", rec.Code, rec.Body.String())
	}
	var alerts int
	_ = r.pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events WHERE tenant_id = $1 AND action = 'ingest.data_quality_conflict'`, r.tenant).Scan(&alerts)
	if alerts != 1 {
		t.Fatalf("%d data-quality records", alerts)
	}
}
