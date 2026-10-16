package server

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
)

type reconRow struct {
	ReversalRef     string `json:"reversal_ref"`
	InstructionRef  string `json:"instruction_ref"`
	AmountMinor     string `json:"amount_minor"`
	LedgerMinor     string `json:"ledger_amount_minor,omitempty"`
	Currency        string `json:"currency"`
	FinancialStatus string `json:"financial_status"`
	Result          string `json:"result"`
}

// reconcileReversals serves GET /v1/reconciliation/reversals. It compares each
// reversal the system recorded with what the (simulated) ledger holds, and lists
// ledger credits that no reversal accounts for. The ledger is synthetic.
func (s *Services) reconcileReversals(w http.ResponseWriter, r *http.Request) {
	p, ok := s.canReadQueue(w, r)
	if !ok {
		return
	}
	rows := []reconRow{}
	counts := map[string]int{}
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		q, err := tx.Query(r.Context(), `
			SELECT r.reversal_ref, t.instruction_ref, r.amount_minor, l.amount_minor, r.currency, r.financial_status,
			  CASE
			    WHEN l.id IS NOT NULL AND l.amount_minor <> r.amount_minor THEN 'AMOUNT_MISMATCH'
			    WHEN r.financial_status = 'VERIFIED' AND l.id IS NULL THEN 'MISSING_POSTING'
			    WHEN r.financial_status = 'VERIFIED' THEN 'MATCHED'
			    WHEN l.id IS NOT NULL THEN 'POSTED_NOT_VERIFIED'
			    WHEN r.submitted_at IS NOT NULL THEN 'IN_FLIGHT'
			    ELSE 'NOT_SUBMITTED' END
			FROM reversals r JOIN transactions t ON t.tenant_id = r.tenant_id AND t.id = r.txn_id
			LEFT JOIN sim_ledger_entries l ON l.tenant_id = r.tenant_id AND l.kind = 'REVERSAL_CREDIT' AND l.reversal_ref = r.reversal_ref
			ORDER BY r.created_at`)
		if err != nil {
			return err
		}
		for q.Next() {
			var x reconRow
			var amt int64
			var led *int64
			if err := q.Scan(&x.ReversalRef, &x.InstructionRef, &amt, &led, &x.Currency, &x.FinancialStatus, &x.Result); err != nil {
				q.Close()
				return err
			}
			x.AmountMinor = strconv.FormatInt(amt, 10)
			if led != nil {
				x.LedgerMinor = strconv.FormatInt(*led, 10)
			}
			counts[x.Result]++
			rows = append(rows, x)
		}
		q.Close()
		if err := q.Err(); err != nil {
			return err
		}
		o, err := tx.Query(r.Context(), `SELECT l.reversal_ref, l.instruction_ref, l.amount_minor, l.currency FROM sim_ledger_entries l
			WHERE l.kind = 'REVERSAL_CREDIT' AND NOT EXISTS (SELECT 1 FROM reversals r WHERE r.tenant_id = l.tenant_id AND r.reversal_ref = l.reversal_ref)
			ORDER BY l.posted_at`)
		if err != nil {
			return err
		}
		defer o.Close()
		for o.Next() {
			x := reconRow{FinancialStatus: "NONE", Result: "ORPHAN_POSTING"}
			var amt int64
			if err := o.Scan(&x.ReversalRef, &x.InstructionRef, &amt, &x.Currency); err != nil {
				return err
			}
			x.AmountMinor = strconv.FormatInt(amt, 10)
			counts[x.Result]++
			rows = append(rows, x)
		}
		return o.Err()
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": rows, "counts": counts, "simulated": true})
}

// summaryReport serves GET /v1/reports/summary: the dashboard numbers.
func (s *Services) summaryReport(w http.ResponseWriter, r *http.Request) {
	p, ok := s.canReadQueue(w, r)
	if !ok {
		return
	}
	out := map[string]any{"simulated": true}
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		group := func(sql string) (map[string]int, error) {
			m := map[string]int{}
			q, err := tx.Query(r.Context(), sql)
			if err != nil {
				return nil, err
			}
			defer q.Close()
			for q.Next() {
				var k string
				var n int
				if err := q.Scan(&k, &n); err != nil {
					return nil, err
				}
				m[k] = n
			}
			return m, q.Err()
		}
		var err error
		if out["transactions_by_state"], err = group(`SELECT lifecycle_state, count(*)::int FROM transactions GROUP BY 1`); err != nil {
			return err
		}
		if out["cases_by_status"], err = group(`SELECT status, count(*)::int FROM cases GROUP BY 1`); err != nil {
			return err
		}
		if out["cases_by_classification"], err = group(`SELECT classification, count(*)::int FROM cases GROUP BY 1`); err != nil {
			return err
		}
		if out["reversals_by_financial_status"], err = group(`SELECT financial_status, count(*)::int FROM reversals GROUP BY 1`); err != nil {
			return err
		}
		var verified, reversals, auto, resolved int
		var value int64
		var avg *float64
		if err := tx.QueryRow(r.Context(), `SELECT
			count(*) FILTER (WHERE financial_status = 'VERIFIED')::int,
			count(*)::int,
			count(*) FILTER (WHERE requested_by_type = 'system' AND financial_status = 'VERIFIED')::int,
			coalesce(sum(amount_minor) FILTER (WHERE financial_status = 'VERIFIED'), 0)::bigint
			FROM reversals`).Scan(&verified, &reversals, &auto, &value); err != nil {
			return err
		}
		if err := tx.QueryRow(r.Context(), `SELECT count(*)::int, avg(extract(epoch FROM resolved_at - opened_at))::float8
			FROM cases WHERE resolved_at IS NOT NULL`).Scan(&resolved, &avg); err != nil {
			return err
		}
		out["reversals_total"] = reversals
		out["reversals_verified"] = verified
		out["reversals_verified_value_minor"] = strconv.FormatInt(value, 10)
		out["automatically_verified"] = auto
		if verified > 0 {
			out["automation_rate"] = float64(auto) / float64(verified)
		} else {
			out["automation_rate"] = nil
		}
		out["resolved_cases"] = resolved
		out["average_seconds_to_resolve"] = avg
		return nil
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
