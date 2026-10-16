package detect

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/sources"
)

// Holds. A finding that depends on a source being silent is held while that source is
// stale or down, because the absence of an event from a source that is not reporting
// is not evidence. Only sources with an expectation can be judged: a source nobody
// has configured is treated as before, so nothing changes until someone sets one.
//
// The finding is not lost. The hold is recorded, the sweep looks again, and the case
// is raised if the transaction is still unresolved once the source is reporting.

// degraded reads the stale or down sources once per run.
func (d *Detector) degraded(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ec *run) (map[string]string, error) {
	if ec.degraded != nil {
		return ec.degraded, nil
	}
	m, err := sources.Degraded(ctx, tx, tenant, d.now())
	if err != nil {
		return nil, err
	}
	ec.degraded = m
	return m, nil
}

// answerWindow is how far back traffic is read to learn which sources answer for a
// rail.
const answerWindow = 24 * time.Hour

// answering returns the sources that have recently delivered answers (a transfer sent,
// a credit confirmed or failed, a timeout) for transactions on a rail. A transfer
// that has not been answered depends on them even when none of them has yet
// contributed an event to it, which is exactly the case during an outage.
func (d *Detector) answering(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ec *run, rail string) ([]string, error) {
	if v, ok := ec.answering[rail]; ok {
		return v, nil
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT e.source_system FROM transaction_events e
		JOIN transactions t ON t.tenant_id = e.tenant_id AND t.id = e.txn_id
		WHERE e.tenant_id = $1 AND t.rail = $2 AND e.received_at > $3
		  AND e.event_type IN ('TRANSFER_SENT', 'CREDIT_CONFIRMED', 'CREDIT_FAILED', 'TRANSFER_RESPONSE_TIMEOUT')`,
		tenant, rail, d.now().Add(-answerWindow))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if ec.answering == nil {
		ec.answering = map[string][]string{}
	}
	ec.answering[rail] = out
	return out, nil
}

// whyHeld names the degraded sources a transaction depends on: those that have
// contributed events to it, and those that answer for its rail. It returns "" when
// none is degraded.
func (d *Detector) whyHeld(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ec *run, txnID uuid.UUID) (string, error) {
	deg, err := d.degraded(ctx, tx, tenant, ec)
	if err != nil || len(deg) == 0 {
		return "", err
	}
	var rail string
	if err := tx.QueryRow(ctx, `SELECT rail FROM transactions WHERE tenant_id = $1 AND id = $2`, tenant, txnID).Scan(&rail); err != nil {
		return "", err
	}
	depends := map[string]bool{}
	rows, err := tx.Query(ctx, `SELECT DISTINCT source_system FROM transaction_events WHERE tenant_id = $1 AND txn_id = $2`, tenant, txnID)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return "", err
		}
		depends[s] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}
	ans, err := d.answering(ctx, tx, tenant, ec, rail)
	if err != nil {
		return "", err
	}
	for _, s := range ans {
		depends[s] = true
	}
	var why []string
	for s := range depends {
		if st, bad := deg[s]; bad {
			why = append(why, fmt.Sprintf("%s is %s", s, st))
		}
	}
	sort.Strings(why)
	return strings.Join(why, "; "), nil
}

func (d *Detector) hold(ctx context.Context, tx pgx.Tx, tenant, txnID uuid.UUID, code, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO detection_holds (tenant_id, txn_id, code, reason, held_at, last_seen_at) VALUES ($1,$2,$3,$4,$5,$5)
		ON CONFLICT (tenant_id, txn_id, code) DO UPDATE SET reason = EXCLUDED.reason, last_seen_at = EXCLUDED.last_seen_at,
			released_at = NULL, held_at = CASE WHEN detection_holds.released_at IS NULL THEN detection_holds.held_at ELSE EXCLUDED.held_at END`,
		tenant, txnID, code, reason, d.now())
	return err
}

func (d *Detector) release(ctx context.Context, tx pgx.Tx, tenant, txnID uuid.UUID, code string) error {
	_, err := tx.Exec(ctx, `UPDATE detection_holds SET released_at = $4 WHERE tenant_id = $1 AND txn_id = $2 AND code = $3 AND released_at IS NULL`,
		tenant, txnID, code, d.now())
	return err
}

// SweepHolds looks again at transactions whose duplicate finding was held, because
// no new event may ever arrive to prompt it.
func (d *Detector) SweepHolds(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]Finding, error) {
	rows, err := tx.Query(ctx, `SELECT txn_id FROM detection_holds WHERE tenant_id = $1 AND code = 'DUP001' AND released_at IS NULL ORDER BY held_at LIMIT 500`, tenant)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []Finding
	for _, id := range ids {
		fs, err := d.Evaluate(ctx, tx, tenant, id)
		if err != nil {
			return nil, err
		}
		out = append(out, fs...)
	}
	return out, nil
}

// OpenHolds counts findings currently held, by code.
func OpenHolds(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (map[string]int, error) {
	rows, err := tx.Query(ctx, `SELECT code, count(*)::int FROM detection_holds WHERE tenant_id = $1 AND released_at IS NULL GROUP BY code`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var c string
		var n int
		if err := rows.Scan(&c, &n); err != nil {
			return nil, err
		}
		out[c] = n
	}
	return out, rows.Err()
}
