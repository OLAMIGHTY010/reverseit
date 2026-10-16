package alerts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
	"github.com/smartreversal/smartreversal/apps/api/internal/sources"
)

// Covered tells whether a known incident covers a case, so its alerts are suppressed.
// It returns the incident or nil.
type Covered func(ctx context.Context, tx pgx.Tx, tenant, caseID uuid.UUID) (*uuid.UUID, error)

// SweepResult counts what a sweep did.
type SweepResult struct {
	Created    int `json:"created"`
	Repeated   int `json:"repeated"`
	Suppressed int `json:"suppressed"`
	Resolved   int `json:"resolved"`
}

// condition is one thing that is currently true.
type condition struct {
	kind, severity, key, subjectType, subjectRef, body, role string
	caseID                                                   *uuid.UUID
}

// Sweep looks at the platform for every alert-worthy condition that is true now,
// raises it (once), and resolves alerts whose condition has cleared. It is safe to run
// as often as wanted: raising is idempotent. Conditions that need an approved threshold
// (high value, approaching deadline) are checked only when one has been approved.
func (s *Service) Sweep(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, covered Covered) (SweepResult, error) {
	var res SweepResult
	now := s.now()
	var conds []condition
	add := func(c ...condition) { conds = append(conds, c...) }

	// Critical exceptions: open cases graded HIGH or CRITICAL.
	rows, err := tx.Query(ctx, `SELECT id, case_ref, classification, priority, coalesce(exposure_minor, 0), coalesce(currency, '') FROM cases
		WHERE tenant_id = $1 AND priority IN ('HIGH', 'CRITICAL') AND status NOT IN ('RESOLVED', 'CLOSED', 'CLOSED_NO_EXCEPTION', 'CLOSED_CUSTOMER_ERROR')`, tenant)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var id uuid.UUID
		var ref, class, prio, cur string
		var amt int64
		if err := rows.Scan(&id, &ref, &class, &prio, &amt, &cur); err != nil {
			rows.Close()
			return res, err
		}
		cid := id
		add(condition{CriticalException, map[string]string{"HIGH": "HIGH", "CRITICAL": "CRITICAL"}[prio], "case:" + id.String() + ":critical", "case", ref,
			fmt.Sprintf("Alert Type: %s. Case: %s. Amount: %s. Priority: %s.", class, ref, notify.Money(amt, orNGN(cur)), prio), "SUPERVISOR", &cid})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	// Approval requests: reversals waiting for a person.
	rows, err = tx.Query(ctx, `SELECT a.id, c.id, c.case_ref, r.amount_minor, r.currency FROM approvals a
		JOIN reversals r ON r.tenant_id = a.tenant_id AND r.id = a.reversal_id JOIN cases c ON c.tenant_id = r.tenant_id AND c.id = r.case_id
		WHERE a.tenant_id = $1 AND a.status = 'PENDING'`, tenant)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var aid, cid uuid.UUID
		var ref, cur string
		var amt int64
		if err := rows.Scan(&aid, &cid, &ref, &amt, &cur); err != nil {
			rows.Close()
			return res, err
		}
		c := cid
		add(condition{ApprovalRequest, "MEDIUM", "approval:" + aid.String(), "case", ref, fmt.Sprintf("Case %s needs an approval before any reversal. Amount: %s.", ref, notify.Money(amt, cur)), "SUPERVISOR", &c})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	// Reversal failures and lapsed approvals.
	rows, err = tx.Query(ctx, `SELECT r.id, c.id, c.case_ref, r.reversal_ref, r.financial_status, r.approval_status FROM reversals r
		JOIN cases c ON c.tenant_id = r.tenant_id AND c.id = r.case_id
		WHERE r.tenant_id = $1 AND (r.financial_status = 'FAILED' OR (r.approval_status = 'EXPIRED' AND r.accepted_status IN ('PENDING', 'UNKNOWN')))
		  AND c.status NOT IN ('RESOLVED', 'CLOSED', 'CLOSED_NO_EXCEPTION', 'CLOSED_CUSTOMER_ERROR')`, tenant)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var rid, cid uuid.UUID
		var caseRef, rvRef, fin, appr string
		if err := rows.Scan(&rid, &cid, &caseRef, &rvRef, &fin, &appr); err != nil {
			rows.Close()
			return res, err
		}
		c := cid
		if fin == "FAILED" {
			add(condition{ReversalFailure, "HIGH", "reversal:" + rid.String() + ":failed", "case", caseRef, fmt.Sprintf("Reversal %s for case %s did not complete. No money was moved.", rvRef, caseRef), "SUPERVISOR", &c})
		} else {
			add(condition{ApprovalExpired, "MEDIUM", "reversal:" + rid.String() + ":expired", "case", caseRef, fmt.Sprintf("The approval for reversal %s (case %s) is no longer valid. A fresh approval is needed.", rvRef, caseRef), "SUPERVISOR", &c})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	// Integration outages: configured sources that are down.
	deg, err := sources.Degraded(ctx, tx, tenant, now)
	if err != nil {
		return res, err
	}
	for src, st := range deg {
		if st == sources.Down {
			add(condition{IntegrationOutage, "HIGH", "source:" + src + ":down", "source", src, fmt.Sprintf("Source %s has sent nothing for longer than its expected limit and is treated as down.", src), "ADMIN_INTEGRATION", nil})
		}
	}

	// Deadlines: breached, and approaching when a percentage is approved.
	approach := int64(0)
	if v, err := s.Config.Effective(ctx, tx, tenant, "deadline.approaching_percent", now); err == nil {
		approach = v.Value
	} else if !errors.Is(err, configsvc.ErrNotConfigured) {
		return res, err
	}
	rows, err = tx.Query(ctx, `SELECT id, case_ref, opened_at, sla_due_at FROM cases WHERE tenant_id = $1 AND sla_due_at IS NOT NULL
		AND status NOT IN ('RESOLVED', 'CLOSED', 'CLOSED_NO_EXCEPTION', 'CLOSED_CUSTOMER_ERROR')`, tenant)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var id uuid.UUID
		var ref string
		var opened, due time.Time
		if err := rows.Scan(&id, &ref, &opened, &due); err != nil {
			rows.Close()
			return res, err
		}
		c := id
		switch {
		case !now.Before(due):
			add(condition{SLABreached, "HIGH", "case:" + id.String() + ":sla_breached", "case", ref, fmt.Sprintf("Case %s is past its regulatory deadline.", ref), "SUPERVISOR", &c})
		case approach > 0 && float64(now.Sub(opened))/float64(due.Sub(opened))*100 >= float64(approach):
			add(condition{SLAApproaching, "MEDIUM", "case:" + id.String() + ":sla_approaching", "case", ref, fmt.Sprintf("Case %s is nearing its regulatory deadline.", ref), "SUPERVISOR", &c})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	// High-value cases that stay unresolved, when both the amount and the age are approved.
	amount, err1 := s.Config.Effective(ctx, tx, tenant, "alert.high_value_minor", now)
	age, err2 := s.Config.Effective(ctx, tx, tenant, "alert.high_value_age_minutes", now)
	switch {
	case err1 == nil && err2 == nil:
		rows, err = tx.Query(ctx, `SELECT id, case_ref, coalesce(exposure_minor, 0), coalesce(currency, '') FROM cases WHERE tenant_id = $1 AND exposure_minor >= $2 AND opened_at <= $3
			AND status NOT IN ('RESOLVED', 'CLOSED', 'CLOSED_NO_EXCEPTION', 'CLOSED_CUSTOMER_ERROR')`, tenant, amount.Value, now.Add(-time.Duration(age.Value)*time.Minute))
		if err != nil {
			return res, err
		}
		for rows.Next() {
			var id uuid.UUID
			var ref, cur string
			var amt int64
			if err := rows.Scan(&id, &ref, &amt, &cur); err != nil {
				rows.Close()
				return res, err
			}
			c := id
			add(condition{HighValueUnresolved, "HIGH", "case:" + id.String() + ":high_value", "case", ref, fmt.Sprintf("Case %s of %s has stayed unresolved.", ref, notify.Money(amt, orNGN(cur))), "HEAD_OPS", &c})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return res, err
		}
	case err1 != nil && !errors.Is(err1, configsvc.ErrNotConfigured):
		return res, err1
	case err2 != nil && !errors.Is(err2, configsvc.ErrNotConfigured):
		return res, err2
	}

	// Raise everything that is true.
	live := map[string]map[string]bool{}
	for _, c := range conds {
		in := Input{Kind: c.kind, Severity: c.severity, DedupeKey: c.key, SubjectType: c.subjectType, SubjectRef: c.subjectRef, Body: c.body, TargetRole: c.role}
		if covered != nil && c.caseID != nil && c.kind != WidespreadIncident {
			inc, err := covered(ctx, tx, tenant, *c.caseID)
			if err != nil {
				return res, err
			}
			in.IncidentID = inc
		}
		_, outcome, err := s.Raise(ctx, tx, tenant, in)
		if err != nil {
			return res, err
		}
		switch outcome {
		case Created:
			res.Created++
		case Repeated:
			res.Repeated++
		case Suppression:
			res.Suppressed++
		}
		if live[c.kind] == nil {
			live[c.kind] = map[string]bool{}
		}
		live[c.kind][c.key] = true
	}

	// Resolve alerts of the kinds this sweep owns whose condition has cleared.
	for _, kind := range []string{CriticalException, ApprovalRequest, ReversalFailure, ApprovalExpired, IntegrationOutage, SLABreached, SLAApproaching, HighValueUnresolved} {
		rows, err := tx.Query(ctx, `SELECT dedupe_key FROM alerts WHERE tenant_id = $1 AND kind = $2 AND status <> 'resolved'`, tenant, kind)
		if err != nil {
			return res, err
		}
		var stale []string
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				rows.Close()
				return res, err
			}
			if !live[kind][k] {
				stale = append(stale, k)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return res, err
		}
		for _, k := range stale {
			if err := s.Resolve(ctx, tx, tenant, k); err != nil {
				return res, err
			}
			res.Resolved++
		}
	}
	return res, nil
}

func orNGN(c string) string {
	if c == "" {
		return "NGN"
	}
	return c
}
