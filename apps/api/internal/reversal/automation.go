package reversal

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/automation"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
)

// AutoResult reports what automation did with one case.
type AutoResult struct {
	CaseRef  string
	Outcome  string // "reversed", "awaiting_approval", "held", "in_progress", "existing", "failed"
	Reversal *Reversal
	Gate     Gate
}

// AutoReverse is the automatic path for one case. It asks for a reversal as the
// system, which passes through exactly the same eight checks as a person's
// request. If policy lets automation proceed it executes and verifies; if not,
// it leaves the reversal waiting for a person's approval; if the gate fails it
// does nothing at all.
func (s *Service) AutoReverse(ctx context.Context, tenant, caseID uuid.UUID) (AutoResult, error) {
	var ref string
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		c, err := s.Cases.Get(ctx, tx, tenant, caseID)
		ref = c.Ref
		return err
	})
	if err != nil {
		return AutoResult{}, err
	}
	res := AutoResult{CaseRef: ref}

	out, err := s.Request(ctx, tenant, RequestInput{CaseID: caseID, IdempotencyKey: "auto-" + ref + "-v1", ReasonCode: "AUTOMATIC_ASSESSMENT", Who: Requester{}})
	if err != nil {
		return res, err
	}
	res.Gate = out.Gate
	if out.Blocked {
		res.Outcome = "held"
		return res, nil
	}
	rev := out.Reversal
	res.Reversal = &rev
	if out.Existing {
		res.Outcome = "existing"
	}
	if rev.ApprovalStatus == "PENDING" {
		res.Outcome = "awaiting_approval"
		return res, nil
	}
	if rev.ApprovalStatus == "REJECTED" {
		res.Outcome = "held"
		return res, nil
	}
	if rev.ApprovalStatus == "EXPIRED" {
		res.Outcome = "approval_expired"
		return res, nil
	}

	if rev, err = s.Execute(ctx, tenant, rev.ID); errors.Is(err, ErrApprovalExpired) || errors.Is(err, ErrApprovalInvalidated) {
		res.Reversal, res.Outcome = &rev, "approval_expired"
		return res, nil
	} else if err != nil {
		return res, err
	}
	if rev, err = s.Advance(ctx, tenant, rev.ID); err != nil {
		return res, err
	}
	res.Reversal = &rev
	switch {
	case rev.FinancialStatus == "VERIFIED":
		res.Outcome = "reversed"
	case rev.FinancialStatus == "FAILED":
		res.Outcome = "failed"
	default:
		res.Outcome = "in_progress"
	}
	return res, nil
}

// Sweep is the scheduled job behind the automation (FR-38). It does three
// things, each through the same gate as any other request:
//
//  1. asks for a reversal for every open detected case of a reversible type;
//  2. submits reversals that were approved or authorised but are still waiting,
//     including ones a freeze parked;
//  3. checks submitted reversals against the provider and the ledger.
//
// Steps 1 and 2 start new submissions and are held back by the kill switch: a
// global freeze pauses them for the institution, and a narrower freeze skips only
// the cases it covers, before the gate runs. Step 3 is never held back, because
// confirming what has already been submitted is always safe. The job's state is
// recorded so a pause can be seen.
func (s *Service) Sweep(ctx context.Context, tenant uuid.UUID) ([]AutoResult, error) {
	type item struct {
		id    uuid.UUID
		rail  string
		class string
	}
	// A lapsed approval is recorded first, whether or not anything is frozen.
	if _, err := s.ExpireDue(ctx, tenant); err != nil {
		return nil, fmt.Errorf("reversal: expire approvals: %w", err)
	}
	var candidates, waiting []item
	var inflight []uuid.UUID
	var st killswitch.State
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var err error
		if st, err = s.Kill.State(ctx, tx, tenant); err != nil {
			return fmt.Errorf("reversal: cannot read the kill switch: %w", err)
		}
		read := func(sql string, into *[]item) error {
			rows, err := tx.Query(ctx, sql, tenant)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var x item
				if err := rows.Scan(&x.id, &x.rail, &x.class); err != nil {
					return err
				}
				*into = append(*into, x)
			}
			return rows.Err()
		}
		if err := read(`SELECT c.id, t.rail, c.classification FROM cases c JOIN transactions t ON t.tenant_id = c.tenant_id AND t.id = c.txn_id
			WHERE c.tenant_id = $1 AND c.source = 'bot' AND c.classification IN ('FAIL001', 'DUP001') AND c.status IN ('DETECTED', 'UNDER_INVESTIGATION')
			  AND NOT EXISTS (SELECT 1 FROM reversals r WHERE r.tenant_id = c.tenant_id AND r.case_id = c.id)
			ORDER BY c.opened_at`, &candidates); err != nil {
			return err
		}
		if err := read(`SELECT r.id, t.rail, c.classification FROM reversals r
			JOIN cases c ON c.tenant_id = r.tenant_id AND c.id = r.case_id JOIN transactions t ON t.tenant_id = r.tenant_id AND t.id = r.txn_id
			WHERE r.tenant_id = $1 AND r.accepted_status = 'PENDING' AND r.approval_status IN ('NOT_REQUIRED', 'APPROVED') AND r.financial_status = 'UNVERIFIED'
			ORDER BY r.created_at`, &waiting); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id FROM reversals WHERE tenant_id = $1 AND financial_status = 'UNVERIFIED' AND accepted_status IN ('ACCEPTED', 'UNKNOWN')
			AND approval_status IN ('NOT_REQUIRED', 'APPROVED') ORDER BY created_at`, tenant)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			inflight = append(inflight, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	var out []AutoResult
	heldNew, heldWaiting, started, advanced := 0, 0, 0, 0
	for _, c := range candidates {
		if st.Covers(actionFor(c.class, c.rail)) != nil {
			heldNew++
			continue
		}
		r, err := s.AutoReverse(ctx, tenant, c.id)
		if err != nil {
			return out, fmt.Errorf("reversal: automatic path for case %s: %w", c.id, err)
		}
		started++
		out = append(out, r)
	}
	for _, w := range waiting {
		if st.Covers(actionFor(w.class, w.rail)) != nil {
			heldWaiting++
			continue
		}
		if _, err := s.Execute(ctx, tenant, w.id); errors.Is(err, ErrApprovalExpired) || errors.Is(err, ErrApprovalInvalidated) {
			continue // recorded; it waits for a fresh approval
		} else if err != nil {
			return out, fmt.Errorf("reversal: submit %s: %w", w.id, err)
		}
		inflight = append(inflight, w.id) // verify it straight away
	}
	for _, id := range inflight {
		if _, err := s.Advance(ctx, tenant, id); err != nil {
			return out, fmt.Errorf("reversal: advance %s: %w", id, err)
		}
		advanced++
	}

	paused := false
	for _, f := range st.Frozen {
		if f.Target.Scope == killswitch.ScopeGlobal {
			paused = true
		}
	}
	err = s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		return automation.RecordJob(ctx, tx, tenant, automation.JobReversalSweep, paused, automation.FrozenSummary(st),
			map[string]any{"new_requests": started, "held_new_requests": heldNew, "held_submissions": heldWaiting, "verified_or_advanced": advanced})
	})
	return out, err
}

var _ = cases.CodeNoCredit
