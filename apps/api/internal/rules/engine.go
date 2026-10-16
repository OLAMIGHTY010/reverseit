package rules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/sources"
)

// Outcomes of one rule on one transaction.
const (
	OutcomeMatched = "matched"
	OutcomeNoMatch = "no_match"
	OutcomeHeld    = "held"
)

// Modes a rule is evaluated in.
const (
	ModeActive   = "active"
	ModeShadow   = "shadow"
	ModeBacktest = "backtest"
)

// Engine evaluates stored rules against transactions.
type Engine struct {
	Store *Store
	Now   func() time.Time
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

// FactsFor reads the facts a rule can see for one transaction.
func (e *Engine) FactsFor(ctx context.Context, tx pgx.Tx, tenant, txnID uuid.UUID) (Facts, error) {
	var f Facts
	var last time.Time
	var firstDebit *time.Time
	err := tx.QueryRow(ctx, `SELECT lifecycle_state, credit_status, debit_status, rail, channel, currency, amount_minor, event_count, last_event_at, first_debit_at
		FROM transactions WHERE tenant_id = $1 AND id = $2`, tenant, txnID).
		Scan(&f.LifecycleState, &f.CreditStatus, &f.DebitStatus, &f.Rail, &f.Channel, &f.Currency, &f.AmountMinor, &f.EventCount, &last, &firstDebit)
	if err != nil {
		return f, err
	}
	now := e.now()
	f.SecondsSinceLastEvent = max(0, int64(now.Sub(last).Seconds()))
	if firstDebit != nil {
		f.SecondsSinceFirstDebit = max(0, int64(now.Sub(*firstDebit).Seconds()))
	}
	return f, nil
}

// Match is a live rule that matched a transaction and so should open a case.
type Match struct {
	Rule Rule
}

// Evaluate runs every live and shadow rule against one transaction and records what
// each concluded. It returns the live rules that matched. A shadow rule never appears
// in the result: it is recorded and nothing more. A rule that requires fresh sources
// is held, not run, while any of them is not fresh, and the hold is recorded with the
// reason.
func (e *Engine) Evaluate(ctx context.Context, tx pgx.Tx, tenant, txnID uuid.UUID) ([]Match, error) {
	rows, err := tx.Query(ctx, `SELECT `+versionColumns+` FROM detection_rules WHERE tenant_id = $1 AND status IN ('active', 'shadow') ORDER BY rule_id, version`, tenant)
	if err != nil {
		return nil, err
	}
	var live []Version
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		live = append(live, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(live) == 0 {
		return nil, nil
	}
	f, err := e.FactsFor(ctx, tx, tenant, txnID)
	if err != nil {
		return nil, err
	}
	fresh := freshness{}
	var out []Match
	for _, v := range live {
		mode := ModeShadow
		if v.Status == Active {
			mode = ModeActive
		}
		outcome, why, err := e.run(ctx, tx, tenant, &fresh, v.Rule, f)
		if err != nil {
			return nil, err
		}
		if err := e.record(ctx, tx, tenant, v.Rule, txnID, mode, outcome, why); err != nil {
			return nil, err
		}
		if mode == ModeActive && outcome == OutcomeMatched {
			out = append(out, Match{Rule: v.Rule})
		}
	}
	return out, nil
}

// freshness reads the sources once per evaluation.
type freshness struct {
	read   bool
	status map[string]string
}

func (fr *freshness) load(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, now time.Time) error {
	if fr.read {
		return nil
	}
	fr.status = map[string]string{}
	list, err := sources.Report(ctx, tx, tenant, now)
	if err != nil {
		return err
	}
	for _, s := range list {
		fr.status[s.SourceSystem] = s.Status
	}
	fr.read = true
	return nil
}

func (e *Engine) run(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, fr *freshness, r Rule, f Facts) (outcome, why string, err error) {
	if len(r.RequiresFresh) > 0 {
		if err := fr.load(ctx, tx, tenant, e.now()); err != nil {
			return "", "", err
		}
		var problems []string
		for _, src := range r.RequiresFresh {
			st, known := fr.status[src]
			switch {
			case !known:
				problems = append(problems, src+" has sent nothing")
			case st != sources.Fresh:
				problems = append(problems, src+" is "+st)
			}
		}
		if len(problems) > 0 {
			return OutcomeHeld, strings.Join(problems, "; "), nil
		}
	}
	if r.Matches(f) {
		return OutcomeMatched, "", nil
	}
	return OutcomeNoMatch, "", nil
}

func (e *Engine) record(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, r Rule, txnID uuid.UUID, mode, outcome, why string) error {
	_, err := tx.Exec(ctx, `INSERT INTO rule_evaluations (tenant_id, rule_id, version, txn_id, mode, outcome, held_reason, evaluated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (tenant_id, rule_id, version, txn_id, mode) DO UPDATE SET outcome = EXCLUDED.outcome, held_reason = EXCLUDED.held_reason, evaluated_at = EXCLUDED.evaluated_at`,
		tenant, r.RuleID, r.Version, txnID, mode, outcome, why, e.now())
	return err
}

// Stats summarises what a version concluded, to help an approver.
type Stats struct {
	Evaluated int `json:"evaluated"`
	Matched   int `json:"matched"`
	NoMatch   int `json:"no_match"`
	Held      int `json:"held"`
}

// StatsFor counts a version's recorded evaluations in every mode.
func (e *Engine) StatsFor(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ruleID string, version int) (Stats, error) {
	var s Stats
	err := tx.QueryRow(ctx, `SELECT count(*)::int, count(*) FILTER (WHERE outcome = 'matched')::int, count(*) FILTER (WHERE outcome = 'no_match')::int,
			count(*) FILTER (WHERE outcome = 'held')::int FROM rule_evaluations WHERE tenant_id = $1 AND rule_id = $2 AND version = $3`, tenant, ruleID, version).
		Scan(&s.Evaluated, &s.Matched, &s.NoMatch, &s.Held)
	return s, err
}

// Backtest evaluates one version against up to limit of the institution's most
// recently active transactions and records the results, without opening any case. It
// gives an approver something to look at before a rule has seen live traffic. It
// works on a version in draft or shadow.
func (e *Engine) Backtest(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ruleID string, version, limit int) (Stats, error) {
	v, err := e.Store.Get(ctx, tx, tenant, ruleID, version)
	if err != nil {
		return Stats{}, err
	}
	if v.Status != Draft && v.Status != Shadow {
		return Stats{}, fmt.Errorf("%w: it is %s", ErrState, v.Status)
	}
	if limit < 1 || limit > 5000 {
		limit = 1000
	}
	rows, err := tx.Query(ctx, `SELECT id FROM transactions WHERE tenant_id = $1 ORDER BY last_event_at DESC, id LIMIT $2`, tenant, limit)
	if err != nil {
		return Stats{}, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return Stats{}, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Stats{}, err
	}
	fr := freshness{}
	for _, id := range ids {
		f, err := e.FactsFor(ctx, tx, tenant, id)
		if err != nil {
			return Stats{}, err
		}
		outcome, why, err := e.run(ctx, tx, tenant, &fr, v.Rule, f)
		if err != nil {
			return Stats{}, err
		}
		if err := e.record(ctx, tx, tenant, v.Rule, id, ModeBacktest, outcome, why); err != nil {
			return Stats{}, err
		}
	}
	return e.StatsFor(ctx, tx, tenant, ruleID, version)
}
