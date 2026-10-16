package bizcal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
)

// Load builds a tenant's calendar from approved configuration in force at time at:
// the institution's time zone, its holidays, and its rail cut-offs. A holiday key
// with value 0 cancels a holiday, so an unexpected working day can be recorded.
//
// A rail needs both calendar.cutoff_minute.<RAIL> and calendar.settlement_lag_days.<RAIL>
// to have a cut-off. A rail with only one of them has none, which is reported by
// NextCutoff and SettlementDate as ErrNoCutoff.
func Load(ctx context.Context, tx pgx.Tx, cfg *configsvc.Service, tenant uuid.UUID, at time.Time) (*Calendar, error) {
	var tz string
	if err := tx.QueryRow(ctx, `SELECT timezone FROM institutions WHERE id = $1`, tenant).Scan(&tz); err != nil {
		return nil, fmt.Errorf("bizcal: read institution: %w", err)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("bizcal: unknown time zone %q: %w", tz, err)
	}

	holidayKeys, err := cfg.EffectiveByPrefix(ctx, tx, tenant, "calendar.holiday.", at)
	if err != nil {
		return nil, err
	}
	holidays := map[Date]string{}
	for key, v := range holidayKeys {
		d, err := ParseDate(strings.TrimPrefix(key, "calendar.holiday."))
		if err != nil {
			return nil, err
		}
		if v.Value == 1 {
			holidays[d] = v.Reason
		}
	}

	cut, err := cfg.EffectiveByPrefix(ctx, tx, tenant, "calendar.cutoff_minute.", at)
	if err != nil {
		return nil, err
	}
	lag, err := cfg.EffectiveByPrefix(ctx, tx, tenant, "calendar.settlement_lag_days.", at)
	if err != nil {
		return nil, err
	}
	cutoffs := map[string]Cutoff{}
	for key, v := range cut {
		rail := strings.TrimPrefix(key, "calendar.cutoff_minute.")
		l, ok := lag["calendar.settlement_lag_days."+rail]
		if !ok {
			continue // half a rule is no rule
		}
		cutoffs[rail] = Cutoff{MinuteOfDay: int(v.Value), LagDays: int(l.Value)}
	}
	return New(Options{Location: loc, Holidays: holidays, Cutoffs: cutoffs})
}
