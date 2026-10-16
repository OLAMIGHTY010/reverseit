package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/bizcal"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

// withCalendar loads the caller's tenant calendar as of now and runs fn. The
// calendar holds only public information (holidays, cut-offs), so any signed-in
// operator may use these routes.
func (s *Services) withCalendar(w http.ResponseWriter, r *http.Request, fn func(cal *bizcal.Calendar) (any, error)) {
	p := identity.FromContext(r.Context())
	if p == nil {
		unauthorized(w, r)
		return
	}
	var out any
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		cal, err := bizcal.Load(r.Context(), tx, s.Config, p.TenantID, time.Now())
		if err != nil {
			return err
		}
		out, err = fn(cal)
		return err
	})
	switch {
	case errors.Is(err, bizcal.ErrNoCutoff):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "NOT_CONFIGURED", "no cut-off is configured for this rail")
	case errors.Is(err, bizcal.ErrRange):
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, httpx.CodeValidation, err.Error())
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	default:
		httpx.WriteJSON(w, http.StatusOK, out)
	}
}

func timeParam(r *http.Request, name string, required bool) (time.Time, bool, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		if required {
			return time.Time{}, false, errors.New(name + " is required")
		}
		return time.Now(), true, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false, errors.New(name + " must be an RFC 3339 time")
	}
	return t, true, nil
}

func badParam(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, err.Error())
}

// addWorkingDays serves GET /v1/calendar/add-working-days?from=&days=.
func (s *Services) addWorkingDays(w http.ResponseWriter, r *http.Request) {
	from, _, err := timeParam(r, "from", true)
	if err != nil {
		badParam(w, r, err)
		return
	}
	days, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil {
		badParam(w, r, errors.New("days must be an integer"))
		return
	}
	s.withCalendar(w, r, func(cal *bizcal.Calendar) (any, error) {
		to, err := cal.AddWorkingDays(from, days)
		if err != nil {
			return nil, err
		}
		return map[string]any{"from": from, "days": days, "result": to, "time_zone": cal.Location().String()}, nil
	})
}

// businessTime serves GET /v1/calendar/business-time?from=&to=.
func (s *Services) businessTime(w http.ResponseWriter, r *http.Request) {
	from, _, err := timeParam(r, "from", true)
	if err != nil {
		badParam(w, r, err)
		return
	}
	to, _, err := timeParam(r, "to", false)
	if err != nil {
		badParam(w, r, err)
		return
	}
	s.withCalendar(w, r, func(cal *bizcal.Calendar) (any, error) {
		elapsed, err := cal.ElapsedBusinessTime(from, to)
		if err != nil {
			return nil, err
		}
		days, err := cal.WorkingDaysBetween(from, to)
		if err != nil {
			return nil, err
		}
		return map[string]any{"from": from, "to": to, "elapsed_business_seconds": int64(elapsed / time.Second),
			"working_days": days, "time_zone": cal.Location().String()}, nil
	})
}

// nextCutoff serves GET /v1/calendar/next-cutoff?rail=&from=.
func (s *Services) nextCutoff(w http.ResponseWriter, r *http.Request) {
	rail := r.URL.Query().Get("rail")
	if rail == "" || len(rail) > 30 {
		badParam(w, r, errors.New("rail is required"))
		return
	}
	from, _, err := timeParam(r, "from", false)
	if err != nil {
		badParam(w, r, err)
		return
	}
	s.withCalendar(w, r, func(cal *bizcal.Calendar) (any, error) {
		cutoff, err := cal.NextCutoff(rail, from)
		if err != nil {
			return nil, err
		}
		settle, err := cal.SettlementDate(rail, from)
		if err != nil {
			return nil, err
		}
		return map[string]any{"rail": rail, "from": from, "next_cutoff": cutoff, "settlement_date": settle.String(),
			"time_zone": cal.Location().String()}, nil
	})
}
