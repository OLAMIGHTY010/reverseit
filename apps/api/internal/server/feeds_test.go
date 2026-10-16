package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/feed"
	"github.com/smartreversal/smartreversal/apps/api/internal/servauth"
)

const feedCSV = `event_id,code,ts,instr,channel,rail,amt,ccy,debit_acct,rrn
EV-5001,DR,2026-10-15 09:00:00,INS-5001,MOBILE,NIP,18500.00,NGN,acct_0123456789abcdef0123456789abcdef,RRN5001
EV-5002,ZZ,2026-10-15 09:00:00,INS-5001,MOBILE,NIP,18500.00,NGN,acct_0123456789abcdef0123456789abcdef,RRN5001
`

func (r *ingestRig) postFeed(t *testing.T, mapping, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/feeds/"+mapping+"/files", strings.NewReader(body))
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

func TestFeedUploadImportsOnceAndRefusesWhatItShould(t *testing.T) {
	r := newIngestRig(t)
	ctx := context.Background()
	m := &feed.Mapping{
		Name: "gw-csv", Version: 1, Format: feed.CSV, SourceSystem: "SWITCH_F",
		Fields: map[string]feed.Locator{feed.FSourceEventRef: {Column: "event_id"}, feed.FEventType: {Column: "code"}, feed.FSourceTS: {Column: "ts"},
			feed.FInstructionRef: {Column: "instr"}, feed.FChannel: {Column: "channel"}, feed.FRail: {Column: "rail"}, feed.FAmount: {Column: "amt"},
			feed.FCurrency: {Column: "ccy"}, feed.FDebitToken: {Column: "debit_acct"}, feed.FRRN: {Column: "rrn"}},
		EventCodes: map[string]string{"DR": "DEBIT_POSTED"}, TimeLayout: "2006-01-02 15:04:05", TimeZone: "Africa/Lagos", AmountUnit: feed.UnitMajor,
	}
	if err := r.pool.WithTenant(ctx, r.tenant, func(tx pgx.Tx) error {
		if err := feed.Save(ctx, tx, r.tenant, m, "test"); err != nil {
			return err
		}
		return feed.Activate(ctx, tx, r.tenant, m.Name, 1)
	}); err != nil {
		t.Fatal(err)
	}

	if rec := r.postFeed(t, "gw-csv", feedCSV, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no assertion: %d", rec.Code)
	}
	rec := r.postFeed(t, "gw-csv", feedCSV, "assertion")
	var out feed.Result
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != http.StatusCreated || out.Accepted != 1 || out.Rejected != 1 || out.Status != "imported_with_errors" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	rec = r.postFeed(t, "gw-csv", feedCSV, "assertion")
	var again feed.Result
	_ = json.Unmarshal(rec.Body.Bytes(), &again)
	if rec.Code != http.StatusOK || !again.AlreadySeen || again.FileID != out.FileID {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec := r.postFeed(t, "nope", feedCSV, "assertion"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown mapping: %d", rec.Code)
	}
	if rec := r.postFeed(t, "gw-csv", "a,b\n1,2\n", "assertion"); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unreadable file: %d %s", rec.Code, rec.Body.String())
	}
}
