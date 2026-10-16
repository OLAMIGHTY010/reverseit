package server

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/httpx"
	"github.com/smartreversal/smartreversal/apps/api/internal/reversal"
	"github.com/smartreversal/smartreversal/apps/api/internal/sources"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

type reviewItemJSON struct {
	Kind           string            `json:"kind"`
	CaseID         string            `json:"case_id"`
	CaseRef        string            `json:"case_ref"`
	Classification string            `json:"classification"`
	CaseStatus     string            `json:"case_status"`
	OpenedAt       time.Time         `json:"opened_at"`
	Transaction    *transactionJSON  `json:"transaction,omitempty"`
	Evidence       []cases.Evidence  `json:"evidence"`
	Gate           reversal.Gate     `json:"gate"`
	BlockedBecause string            `json:"blocked_because"`
	ReleaseAllowed bool              `json:"release_allowed"`
	ReleaseNeeds   string            `json:"release_requires"`
	Sources        map[string]string `json:"degraded_sources,omitempty"`
	Simulated      bool              `json:"simulated"`
}

// listReviewItems serves GET /v1/review-items: the cases whose outcome is not a
// confirmed failure, shown beside the approval queue as items to review. An unknown
// outcome is never reversed, so these cannot be released. The list shows why, with
// the evidence and the gate's failed checks, and says what would allow a release.
// It is read-only; it needs the full exception-queue scope, because it carries
// internal evidence.
func (s *Services) listReviewItems(w http.ResponseWriter, r *http.Request) {
	p, grant, ok := s.authorizeNoMFA(w, r, "view_exception_queue", authz.Attrs{})
	if !ok {
		return
	}
	if grant.Scope != "full" {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.CodeForbidden, "your role may not see review items")
		return
	}
	items := []reviewItemJSON{}
	err := s.Pool.WithTenant(r.Context(), p.TenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT c.id FROM cases c JOIN transactions t ON t.tenant_id = c.tenant_id AND t.id = c.txn_id
			WHERE c.tenant_id = $1 AND c.source = 'bot'
			  AND c.status NOT IN ('RESOLVED', 'CLOSED', 'CLOSED_NO_EXCEPTION', 'CLOSED_CUSTOMER_ERROR')
			  AND (c.classification = 'UNK001' OR t.lifecycle_state IN ('UNKNOWN', 'PENDING', 'CONFLICTING', 'DEBITED'))
			  AND NOT EXISTS (SELECT 1 FROM reversals r WHERE r.tenant_id = c.tenant_id AND r.case_id = c.id AND r.financial_status <> 'FAILED' AND r.accepted_status <> 'REJECTED')
			ORDER BY c.opened_at, c.id LIMIT 200`, p.TenantID)
		if err != nil {
			return err
		}
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		degraded, err := sources.Degraded(r.Context(), tx, p.TenantID, time.Now())
		if err != nil {
			return err
		}
		for _, id := range ids {
			c, err := s.Cases.Get(r.Context(), tx, p.TenantID, id)
			if err != nil {
				return err
			}
			it := reviewItemJSON{Kind: "REVIEW", CaseID: c.ID.String(), CaseRef: c.Ref, Classification: c.Classification, CaseStatus: c.Status,
				OpenedAt: c.OpenedAt.UTC(), Simulated: true}
			if it.Evidence, err = s.Cases.Evidence(r.Context(), tx, p.TenantID, c.ID); err != nil {
				return err
			}
			if c.TxnID != nil {
				if t, err := scanTxn(tx.QueryRow(r.Context(), `SELECT `+txnColumns+` FROM transactions WHERE id = $1`, *c.TxnID)); err == nil {
					it.Transaction = &t
					it.BlockedBecause = blockedBecause(t.LifecycleState)
					it.Sources = contributing(r, tx, p.TenantID, *c.TxnID, degraded)
				}
			}
			prin := p
			if it.Gate, err = s.Reversal.Assess(r.Context(), tx, p.TenantID, c.ID, reversal.Requester{Principal: &prin}); err != nil {
				return err
			}
			it.ReleaseAllowed = false
			it.ReleaseNeeds = "the outcome must first be a confirmed failure from the authoritative source, with fresh evidence"
			items = append(items, it)
		}
		return nil
	})
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "simulated": true})
}

func blockedBecause(state string) string {
	switch state {
	case txn.StateUnknown:
		return "The outcome is UNKNOWN. An unknown outcome is not a failure and is never reversed."
	case txn.StatePending, txn.StateDebited:
		return "The transfer has no final answer yet."
	case txn.StateConflicting:
		return "Sources disagree about the outcome."
	}
	return "The outcome is not a confirmed failure."
}

// contributing returns the degraded sources that contributed events to a transaction,
// so a reviewer can see when missing evidence may be a silent source.
func contributing(r *http.Request, tx pgx.Tx, tenant, txnID uuid.UUID, degraded map[string]string) map[string]string {
	if len(degraded) == 0 {
		return nil
	}
	rows, err := tx.Query(r.Context(), `SELECT DISTINCT source_system FROM transaction_events WHERE tenant_id = $1 AND txn_id = $2`, tenant, txnID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil {
			if st, bad := degraded[s]; bad {
				out[s] = st
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
