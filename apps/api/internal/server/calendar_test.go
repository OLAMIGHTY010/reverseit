package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
)

// calendarRig has the NIP rail and one holiday configured through the real
// configuration service.
func newCalendarRig(t *testing.T) (*configRig, string) {
	t.Helper()
	r := newConfigRig(t)
	svc := &configsvc.Service{Audit: r.appender}
	maker, checker := uuid.New(), uuid.New()
	set := func(key string, value int, reason string) {
		raw, _ := json.Marshal(value)
		err := r.pool.WithTenant(context.Background(), r.tenant, func(tx pgx.Tx) error {
			v, err := svc.Propose(context.Background(), tx, r.tenant, maker, configsvc.ProposeInput{
				Key: key, Value: raw, SourceReference: "test fixture", EffectiveFrom: time.Now(),
				VerifyStatus: "unverified", Reason: reason})
			if err != nil {
				return err
			}
			_, err = svc.Decide(context.Background(), tx, r.tenant, checker, key, v.Version, true, "")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	set("calendar.holiday.2026-10-14", 1, "Test holiday on a Wednesday")
	set("calendar.cutoff_minute.NIP", 16*60, "NIP cut-off 16:00 for the test")
	set("calendar.settlement_lag_days.NIP", 0, "NIP same-day settlement in the test")
	return r, r.tokenMFA(t, uuid.New(), false, "AGENT") // any signed-in operator may read the calendar
}

func (r *configRig) get(tok, path string, q url.Values) (int, map[string]any) {
	rec := r.do(http.MethodGet, path+"?"+q.Encode(), tok, "")
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestAddWorkingDaysRoute(t *testing.T) {
	r, tok := newCalendarRig(t)
	// Tuesday 13 Oct 2026 09:00 WAT + 1 working day skips the Wednesday holiday.
	code, body := r.get(tok, "/v1/calendar/add-working-days", url.Values{"from": {"2026-10-13T09:00:00+01:00"}, "days": {"1"}})
	if code != http.StatusOK || body["result"] != "2026-10-15T09:00:00+01:00" || body["time_zone"] != "Africa/Lagos" {
		t.Fatalf("%d %v", code, body)
	}
	code, body = r.get(tok, "/v1/calendar/add-working-days", url.Values{"from": {"2026-10-09T15:00:00+01:00"}, "days": {"1"}})
	if code != http.StatusOK || body["result"] != "2026-10-12T15:00:00+01:00" {
		t.Fatalf("over a weekend: %d %v", code, body)
	}
}

func TestBusinessTimeRoute(t *testing.T) {
	r, tok := newCalendarRig(t)
	code, body := r.get(tok, "/v1/calendar/business-time", url.Values{
		"from": {"2026-10-13T12:00:00+01:00"}, "to": {"2026-10-15T12:00:00+01:00"}})
	// Tuesday noon to Thursday noon, with Wednesday a holiday: 12 hours + 12 hours.
	if code != http.StatusOK || body["elapsed_business_seconds"] != float64(24*3600) || body["working_days"] != float64(1) {
		t.Fatalf("%d %v", code, body)
	}
}

func TestNextCutoffRoute(t *testing.T) {
	r, tok := newCalendarRig(t)
	code, body := r.get(tok, "/v1/calendar/next-cutoff", url.Values{"rail": {"NIP"}, "from": {"2026-10-13T17:00:00+01:00"}})
	if code != http.StatusOK || body["next_cutoff"] != "2026-10-15T16:00:00+01:00" || body["settlement_date"] != "2026-10-15" {
		t.Fatalf("%d %v", code, body)
	}
	code, body = r.get(tok, "/v1/calendar/next-cutoff", url.Values{"rail": {"CARD"}, "from": {"2026-10-13T17:00:00+01:00"}})
	if code != http.StatusUnprocessableEntity || body["error"] != "NOT_CONFIGURED" {
		t.Fatalf("unconfigured rail must not be guessed: %d %v", code, body)
	}
}

func TestCalendarRoutesValidateInputAndRequireSignIn(t *testing.T) {
	r, tok := newCalendarRig(t)
	for name, c := range map[string]struct {
		path string
		q    url.Values
		want int
	}{
		"add: no from":     {"/v1/calendar/add-working-days", url.Values{"days": {"1"}}, http.StatusBadRequest},
		"add: bad from":    {"/v1/calendar/add-working-days", url.Values{"from": {"yesterday"}, "days": {"1"}}, http.StatusBadRequest},
		"add: no days":     {"/v1/calendar/add-working-days", url.Values{"from": {"2026-10-13T09:00:00Z"}}, http.StatusBadRequest},
		"add: negative":    {"/v1/calendar/add-working-days", url.Values{"from": {"2026-10-13T09:00:00Z"}, "days": {"-1"}}, http.StatusUnprocessableEntity},
		"add: too many":    {"/v1/calendar/add-working-days", url.Values{"from": {"2026-10-13T09:00:00Z"}, "days": {"99999"}}, http.StatusUnprocessableEntity},
		"time: no from":    {"/v1/calendar/business-time", url.Values{}, http.StatusBadRequest},
		"time: reversed":   {"/v1/calendar/business-time", url.Values{"from": {"2026-10-15T09:00:00Z"}, "to": {"2026-10-13T09:00:00Z"}}, http.StatusUnprocessableEntity},
		"cutoff: no rail":  {"/v1/calendar/next-cutoff", url.Values{}, http.StatusBadRequest},
		"cutoff: bad from": {"/v1/calendar/next-cutoff", url.Values{"rail": {"NIP"}, "from": {"x"}}, http.StatusBadRequest},
	} {
		if code, _ := r.get(tok, c.path, c.q); code != c.want {
			t.Errorf("%s: status %d, want %d", name, code, c.want)
		}
	}
	if code, _ := r.get("", "/v1/calendar/next-cutoff", url.Values{"rail": {"NIP"}}); code != http.StatusUnauthorized {
		t.Errorf("no token: %d", code)
	}
}

func TestAnotherTenantDoesNotInheritTheCalendar(t *testing.T) {
	r, _ := newCalendarRig(t)
	other := newConfigRig(t) // a different institution with nothing configured
	tok := other.tokenMFA(t, uuid.New(), false, "AGENT")
	code, body := other.get(tok, "/v1/calendar/next-cutoff", url.Values{"rail": {"NIP"}})
	if code != http.StatusUnprocessableEntity || body["error"] != "NOT_CONFIGURED" {
		t.Fatalf("%d %v", code, body)
	}
	_ = r
}
