// Package automation keeps track of work the kill switch held back, and of what
// each scheduled job is doing, so that nothing a freeze blocks is lost and a
// pause can be seen. See docs/backend/kill-switch-enforcement.md.
package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
)

// Kinds of deferred work.
const KindDetect = "detect"

// Job names.
const (
	JobDetectorSweep = "detector_sweep"
	JobReversalSweep = "reversal_sweep"
	JobResumeHeld    = "resume_held"
)

// Deferred is one piece of held work.
type Deferred struct {
	ID            uuid.UUID `json:"id"`
	Kind          string    `json:"kind"`
	SubjectID     uuid.UUID `json:"subject_id"`
	BlockedScope  string    `json:"blocked_scope"`
	BlockedTarget string    `json:"blocked_target,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	Attempts      int       `json:"attempts"`
}

// Defer records that work on subject was blocked by a freeze. Blocking the same
// subject again keeps the single open row and updates what blocked it.
func Defer(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, kind string, subject uuid.UUID, f *killswitch.FrozenError) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO deferred_work (tenant_id, kind, subject_id, blocked_scope, blocked_target)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (tenant_id, kind, subject_id) WHERE resumed_at IS NULL
		DO UPDATE SET last_blocked_at = now(), blocked_scope = EXCLUDED.blocked_scope, blocked_target = EXCLUDED.blocked_target`,
		tenant, kind, subject, string(f.Scope), f.Target)
	if err != nil {
		return fmt.Errorf("automation: defer: %w", err)
	}
	return nil
}

// Claim locks up to limit open items of a kind, oldest first. Rows another worker
// has locked are skipped, so two workers never take the same item.
func Claim(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, kind string, limit int) ([]Deferred, error) {
	rows, err := tx.Query(ctx, `SELECT id, kind, subject_id, blocked_scope, blocked_target, created_at, attempts FROM deferred_work
		WHERE tenant_id = $1 AND kind = $2 AND resumed_at IS NULL ORDER BY created_at, id LIMIT $3 FOR UPDATE SKIP LOCKED`, tenant, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deferred
	for rows.Next() {
		var d Deferred
		if err := rows.Scan(&d.ID, &d.Kind, &d.SubjectID, &d.BlockedScope, &d.BlockedTarget, &d.CreatedAt, &d.Attempts); err != nil {
			return nil, err
		}
		d.CreatedAt = d.CreatedAt.UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}

// Resumed marks an item done.
func Resumed(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE deferred_work SET resumed_at = now(), attempts = attempts + 1 WHERE tenant_id = $1 AND id = $2`, tenant, id)
	return err
}

// StillHeld records a resume attempt that was blocked again.
func StillHeld(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE deferred_work SET attempts = attempts + 1, last_blocked_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, id)
	return err
}

// Counts returns the number of open items by kind.
func Counts(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (map[string]int, error) {
	rows, err := tx.Query(ctx, `SELECT kind, count(*)::int FROM deferred_work WHERE tenant_id = $1 AND resumed_at IS NULL GROUP BY kind`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// Job is the reported state of a scheduled job.
type Job struct {
	Job       string          `json:"job"`
	State     string          `json:"state"`
	Frozen    string          `json:"frozen,omitempty"`
	Since     time.Time       `json:"since"`
	LastRunAt *time.Time      `json:"last_run_at,omitempty"`
	LastRun   json.RawMessage `json:"last_result"`
}

// FrozenSummary describes the frozen scopes in a short fixed form such as
// "global" or "exception_type:FAIL001,rail:NIP".
func FrozenSummary(st killswitch.State) string {
	var parts []string
	for _, f := range st.Frozen {
		if f.Target.Value == "" {
			parts = append(parts, string(f.Target.Scope))
		} else {
			parts = append(parts, string(f.Target.Scope)+":"+f.Target.Value)
		}
	}
	return strings.Join(parts, ",")
}

// RecordJob stores the state of a job after a run. A job is paused when a global
// freeze covers everything it does; under a narrower freeze it keeps running and
// reports which scopes are frozen. The time in since only moves when the state
// changes.
func RecordJob(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, job string, paused bool, frozen string, result map[string]any) error {
	state := "running"
	if paused {
		state = "paused"
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO automation_jobs (tenant_id, job, state, frozen, last_run_at, last_result)
		VALUES ($1,$2,$3,$4,now(),$5)
		ON CONFLICT (tenant_id, job) DO UPDATE SET
			since = CASE WHEN automation_jobs.state <> EXCLUDED.state THEN now() ELSE automation_jobs.since END,
			state = EXCLUDED.state, frozen = EXCLUDED.frozen, last_run_at = now(), last_result = EXCLUDED.last_result`,
		tenant, job, state, frozen, raw)
	return err
}

// Jobs lists the recorded jobs.
func Jobs(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]Job, error) {
	rows, err := tx.Query(ctx, `SELECT job, state, frozen, since, last_run_at, last_result FROM automation_jobs WHERE tenant_id = $1 ORDER BY job`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.Job, &j.State, &j.Frozen, &j.Since, &j.LastRunAt, &j.LastRun); err != nil {
			return nil, err
		}
		j.Since = j.Since.UTC()
		out = append(out, j)
	}
	return out, rows.Err()
}
