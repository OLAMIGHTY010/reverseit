// Package detect turns transaction state into exception cases (B3).
//
// Every detector is deterministic and explains itself: the case it opens carries
// the rule id and version, the evidence it used and a plain explanation. AI
// scores may later sit beside these verdicts, but never replace them.
package detect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/automation"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/killswitch"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
	"github.com/smartreversal/smartreversal/apps/api/internal/platform/db"
	"github.com/smartreversal/smartreversal/apps/api/internal/rules"
	"github.com/smartreversal/smartreversal/apps/api/internal/stream"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Rule ids and versions. A change to a rule's behaviour bumps its version.
const (
	RuleFailure    = "DET-FAIL-1"
	RuleUnknown    = "DET-UNK-1"
	RuleDuplicate  = "DET-DUP-1"
	RuleStuck      = "DET-STUCK-1"
	RuleATM        = "DET-ATM-1"
	RuleFee        = "DET-FEE-1"
	RuleVendor     = "DET-VND-1"
	RuleMismatch   = "DET-MIS-1"
	RuleIncomplete = "DET-INC-1"
	RuleVersion    = "1.0"
)

// Finding is one detection result.
type Finding struct {
	Case    cases.Case
	Created bool
}

// Detector evaluates transactions.
type Detector struct {
	Cases  *cases.Service
	Config *configsvc.Service
	// Kill, when set, is consulted immediately before the detector opens a case
	// or closes one automatically. A covering freeze defers that work: it is
	// recorded in deferred_work and carried out after an authorised unfreeze.
	// Reading and storing evidence is never held back. Leaving Kill nil turns
	// enforcement off, which only tests should do.
	Kill *killswitch.Service
	// Rules, when set, adds the institution's declarative detection rules to the
	// built-in ones. Live rules open cases; shadow rules are only recorded.
	Rules *rules.Engine
	Now   func() time.Time

	// beforeAction lets a test change the world between "decide to act" and "act".
	beforeAction func()
}

// run carries what one evaluation learns. held is set when a freeze stopped any
// action, so the caller can record the deferral. at is the time a case counts as
// detected: normally now, but the original time when held work is resumed.
type run struct {
	at      time.Time
	held    *killswitch.FrozenError
	resumed bool
	// degraded caches the stale or down sources for this run (see degraded.go).
	degraded  map[string]string
	answering map[string][]string
}

// allowed reports whether an action may proceed. It checks the kill switch at the
// moment of action. A freeze is not an error: the work is noted as held. A switch
// that cannot be read stops the action and fails the evaluation, which the
// consumer retries.
func (d *Detector) allowed(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ec *run, rail, code, rule string) (bool, error) {
	if d.Kill == nil {
		return true, nil
	}
	if d.beforeAction != nil {
		d.beforeAction()
	}
	err := d.Kill.Check(ctx, tx, tenant, killswitch.Action{Rail: rail, ExceptionType: code, PolicyID: rule})
	var fe *killswitch.FrozenError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &fe):
		if ec.held == nil {
			ec.held = fe
		}
		return false, nil
	default:
		return false, err
	}
}

func (d *Detector) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}

type txnRow struct {
	ID             uuid.UUID
	Ref            string
	Channel        string
	Rail           string
	Amount         int64
	Currency       string
	DebitToken     string
	BeneficiaryTok string
	SessionID      string
	Lifecycle      string
	CreditStatus   string
	DebitStatus    string
	FirstDebitAt   *time.Time
	LastEventAt    time.Time
}

const txnSelect = `SELECT id, instruction_ref, channel, rail, amount_minor, currency, debit_account_token, beneficiary_token, session_id,
	lifecycle_state, credit_status, debit_status, first_debit_at, last_event_at FROM transactions`

func scanTxn(row pgx.Row) (txnRow, error) {
	var t txnRow
	err := row.Scan(&t.ID, &t.Ref, &t.Channel, &t.Rail, &t.Amount, &t.Currency, &t.DebitToken, &t.BeneficiaryTok, &t.SessionID,
		&t.Lifecycle, &t.CreditStatus, &t.DebitStatus, &t.FirstDebitAt, &t.LastEventAt)
	return t, err
}

func (d *Detector) evidence(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) ([]cases.Evidence, error) {
	rows, err := tx.Query(ctx, `SELECT id, event_type, source_system, source_event_ref, payload_hash, received_at FROM transaction_events
		WHERE tenant_id = $1 AND txn_id = $2 ORDER BY source_ts, id`, tenant, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []cases.Evidence
	for rows.Next() {
		var eid uuid.UUID
		var typ, sys, ref, hash string
		var at time.Time
		if err := rows.Scan(&eid, &typ, &sys, &ref, &hash, &at); err != nil {
			return nil, err
		}
		out = append(out, cases.Evidence{Kind: "transaction_event", Ref: eid.String(), SourceSystem: sys, FetchedAt: at.UTC(), Hash: hash, IsFact: true,
			Detail: map[string]string{"event_type": typ, "source_event_ref": ref}})
	}
	return out, rows.Err()
}

// Evaluate runs every detector against one transaction and returns what it found.
// It is idempotent: a transaction seen again yields the same cases.
func (d *Detector) Evaluate(ctx context.Context, tx pgx.Tx, tenant, txnID uuid.UUID) ([]Finding, error) {
	ec := &run{at: d.now()}
	out, err := d.evaluate(ctx, tx, tenant, txnID, ec)
	if err == nil && ec.held != nil {
		err = automation.Defer(ctx, tx, tenant, automation.KindDetect, txnID, ec.held)
	}
	return out, err
}

func (d *Detector) evaluate(ctx context.Context, tx pgx.Tx, tenant, txnID uuid.UUID, ec *run) ([]Finding, error) {
	t, err := scanTxn(tx.QueryRow(ctx, txnSelect+` WHERE tenant_id = $1 AND id = $2`, tenant, txnID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // the transaction is gone or not ours: nothing to do
	}
	if err != nil {
		return nil, err
	}
	ev, err := d.evidence(ctx, tx, tenant, txnID)
	if err != nil {
		return nil, err
	}

	var out []Finding
	add := func(f Finding, err error) error {
		if err != nil {
			return err
		}
		if f.Case.ID != uuid.Nil {
			out = append(out, f)
		}
		return nil
	}

	// A transaction with a final answer has nothing left to wait for.
	switch t.Lifecycle {
	case txn.StateConfirmedSuccess, txn.StateConfirmedFailure, txn.StateReversed:
		if err := d.release(ctx, tx, tenant, t.ID, cases.CodeUnknown); err != nil {
			return nil, err
		}
	}

	switch t.Lifecycle {
	case txn.StateConfirmedFailure:
		switch {
		case t.Rail == "MISMATCH" || t.Rail == "DISCREPANCY" || t.Channel == "MISMATCH":
			if err := add(d.open(ctx, tx, tenant, ec, t, cases.CodeMismatch, RuleMismatch, "HIGH", "amount_currency_mismatch",
				"Discrepancy detected between ledger debit amount/currency and switch/settlement confirmation.", ev)); err != nil {
				return nil, err
			}
		case t.Rail == "ATM" || t.Channel == "ATM":
			if err := add(d.open(ctx, tx, tenant, ec, t, cases.CodeAtmFault, RuleATM, "HIGH", "atm_hardware_fault",
				"ATM cash dispenser hardware failure: debit was posted but physical cash was not dispensed.", ev)); err != nil {
				return nil, err
			}
		case t.Rail == "FEE" || t.Channel == "FEE":
			if err := add(d.open(ctx, tx, tenant, ec, t, cases.CodeDisputedFee, RuleFee, "LOW", "unauthorized_fee_surcharge",
				"Unauthorized or disputed electronic service fee or surcharge debit.", ev)); err != nil {
				return nil, err
			}
		case t.Rail == "MERCHANT" || t.Rail == "VENDOR" || t.Channel == "GATEWAY" || t.Channel == "MERCHANT" || t.Channel == "USSD":
			if err := add(d.open(ctx, tx, tenant, ec, t, cases.CodeVendorDrop, RuleVendor, "MEDIUM", "vendor_aggregator_drop",
				"Third-party biller/utility aggregator failed to fulfill transaction after debit posted.", ev)); err != nil {
				return nil, err
			}
		case t.Rail == "INTERCHANGE" || t.Rail == "NIP_SWITCH" || t.Rail == "ACH_SWITCH" || t.Rail == "INTERMEDIARY":
			if err := add(d.open(ctx, tx, tenant, ec, t, cases.CodeIncomplete, RuleIncomplete, "MEDIUM", "multi_leg_routing_drop",
				"Incomplete multi-leg routing: intermediary switch dropped transfer before reaching beneficiary institution.", ev)); err != nil {
				return nil, err
			}
		default:
			if err := add(d.open(ctx, tx, tenant, ec, t, cases.CodeNoCredit, RuleFailure, "", "beneficiary_side_failure",
				"The debit was posted and the beneficiary credit was reported as failed by the source.", ev)); err != nil {
				return nil, err
			}
		}
	case txn.StateUnknown:
		if err := add(d.open(ctx, tx, tenant, ec, t, cases.CodeUnknown, RuleUnknown, "", "switch_response_failure",
			"The debit was posted and the switch did not answer. The outcome is unknown, which is not the same as failed.", ev)); err != nil {
			return nil, err
		}
	case txn.StateConflicting:
		if t.Rail == "MISMATCH" || t.Rail == "DISCREPANCY" || t.Channel == "MISMATCH" {
			if err := add(d.open(ctx, tx, tenant, ec, t, cases.CodeMismatch, RuleMismatch, "HIGH", "amount_currency_mismatch",
				"Discrepancy detected between ledger debit amount/currency and switch/settlement confirmation.", ev)); err != nil {
				return nil, err
			}
		} else {
			if err := add(d.open(ctx, tx, tenant, ec, t, cases.CodeUnknown, RuleUnknown, "", "conflicting_evidence",
				"Sources disagree: one reports the credit confirmed and another reports it failed.", ev)); err != nil {
				return nil, err
			}
		}
	case txn.StateConfirmedSuccess:
		if err := d.closeLateSuccess(ctx, tx, tenant, ec, t); err != nil {
			return nil, err
		}
	}

	if t.DebitStatus == txn.DebitPosted && t.FirstDebitAt != nil {
		if err := add(d.duplicate(ctx, tx, tenant, ec, t, ev)); err != nil {
			return nil, err
		}
	}
	if err := d.applyRules(ctx, tx, tenant, ec, t, ev, add); err != nil {
		return nil, err
	}
	return out, nil
}

// applyRules runs the institution's declarative rules for one transaction. A rule
// that matches opens a case exactly as a built-in rule would, under the rule's own id
// and version, so the case says which rule raised it. A case of the same code that a
// built-in rule already opened for the transaction is not opened again.
func (d *Detector) applyRules(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ec *run, t txnRow, ev []cases.Evidence, add func(Finding, error) error) error {
	if d.Rules == nil {
		return nil
	}
	matches, err := d.Rules.Evaluate(ctx, tx, tenant, t.ID)
	if err != nil {
		return err
	}
	for _, m := range matches {
		r := m.Rule
		if err := add(d.openVersion(ctx, tx, tenant, ec, t, r.Code, r.RuleID, fmt.Sprint(r.Version), r.Risk, r.RootCause, r.Explanation, ev)); err != nil {
			return err
		}
	}
	return nil
}

// SweepRules evaluates the declarative rules for transactions that are still waiting
// for an answer, because a rule about silence needs the clock and not a new event.
// It looks at the most recently active unresolved transactions first.
func (d *Detector) SweepRules(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]Finding, error) {
	if d.Rules == nil {
		return nil, nil
	}
	rows, err := tx.Query(ctx, txnSelect+` WHERE tenant_id = $1 AND lifecycle_state IN ('DEBITED', 'PENDING', 'UNKNOWN', 'CONFLICTING') ORDER BY last_event_at DESC LIMIT 1000`, tenant)
	if err != nil {
		return nil, err
	}
	var open []txnRow
	for rows.Next() {
		t, err := scanTxn(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		open = append(open, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []Finding
	for _, t := range open {
		ev, err := d.evidence(ctx, tx, tenant, t.ID)
		if err != nil {
			return nil, err
		}
		ec := &run{at: d.now()}
		add := func(f Finding, err error) error {
			if err == nil && f.Case.ID != uuid.Nil {
				out = append(out, f)
			}
			return err
		}
		if err := d.applyRules(ctx, tx, tenant, ec, t, ev, add); err != nil {
			return nil, err
		}
		if ec.held != nil {
			if err := automation.Defer(ctx, tx, tenant, automation.KindDetect, t.ID, ec.held); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func (d *Detector) open(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ec *run, t txnRow, code, rule, risk, root, why string, ev []cases.Evidence) (Finding, error) {
	return d.openVersion(ctx, tx, tenant, ec, t, code, rule, RuleVersion, risk, root, why, ev)
}

// openVersion is open for a rule with its own version.
func (d *Detector) openVersion(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ec *run, t txnRow, code, rule, version, risk, root, why string, ev []cases.Evidence) (Finding, error) {
	if ok, err := d.allowed(ctx, tx, tenant, ec, t.Rail, code, rule); err != nil || !ok {
		return Finding{}, err
	}
	if ec.resumed {
		why += " Opened late: detection was held by a kill switch freeze and resumed after it was lifted."
	}
	amount := t.Amount
	id := t.ID
	c, created, err := d.Cases.Open(ctx, tx, tenant, cases.OpenInput{
		Classification: code, Source: cases.SourceBot, RiskGrade: risk, TxnID: &id, ExposureMinor: &amount, Currency: t.Currency,
		Rail: t.Rail, RootCause: root, RuleID: rule, RuleVersion: version, Explanation: why, Evidence: ev, Actor: cases.System,
		At: ec.at,
	})
	if err != nil {
		return Finding{}, err
	}
	if created {
		if _, err := notify.Record(ctx, tx, tenant, c.ID, notify.OpsExceptionOpened(c.Ref, code, t.Ref, t.Amount, t.Currency), c.Ref+":ops:opened"); err != nil {
			return Finding{}, err
		}
		if _, err := notify.Record(ctx, tx, tenant, c.ID, notify.CustomerDetected(t.Amount, t.Currency, t.Ref), c.Ref+":customer:detected"); err != nil {
			return Finding{}, err
		}
	}
	return Finding{Case: c, Created: created}, nil
}

// duplicate looks for another debit with the same account, amount, currency and
// beneficiary inside the configured window. It never matches on account, amount
// and time alone when the earlier transfer failed: a customer who retries after a
// confirmed failure is not a duplicate.
func (d *Detector) duplicate(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ec *run, t txnRow, ev []cases.Evidence) (Finding, error) {
	window, err := d.Config.Effective(ctx, tx, tenant, "duplicate.window_seconds", d.now())
	if errors.Is(err, configsvc.ErrNotConfigured) {
		return Finding{}, nil // no approved window, so no duplicate rule runs
	}
	if err != nil {
		return Finding{}, err
	}
	span := time.Duration(window.Value) * time.Second

	rows, err := tx.Query(ctx, txnSelect+` WHERE tenant_id = $1 AND id <> $2 AND debit_account_token = $3 AND amount_minor = $4
		AND currency = $5 AND beneficiary_token = $6 AND debit_status = 'POSTED' AND first_debit_at IS NOT NULL
		AND first_debit_at BETWEEN $7 AND $8 ORDER BY first_debit_at`,
		tenant, t.ID, t.DebitToken, t.Amount, t.Currency, t.BeneficiaryTok, t.FirstDebitAt.Add(-span), t.FirstDebitAt.Add(span))
	if err != nil {
		return Finding{}, err
	}
	var others []txnRow
	for rows.Next() {
		o, err := scanTxn(rows)
		if err != nil {
			rows.Close()
			return Finding{}, err
		}
		others = append(others, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Finding{}, err
	}

	// The case belongs to the later of each pair, so a duplicate is flagged once,
	// whichever of the two is seen first.
	var result Finding
	heldPair := ""
	for _, o := range others {
		later, earlier := t, o
		if o.FirstDebitAt.After(*t.FirstDebitAt) || (o.FirstDebitAt.Equal(*t.FirstDebitAt) && o.ID.String() > t.ID.String()) {
			later, earlier = o, t
		}
		if earlier.CreditStatus == txn.CreditFailed {
			continue // a retry after a confirmed failure
		}
		// The earlier attempt may have failed already without our knowing, if a source
		// it depends on is not reporting. Judging the pair now could call a retry a
		// duplicate. Hold it and look again when the source is back.
		if earlier.CreditStatus != txn.CreditConfirmed {
			if why, err := d.whyHeld(ctx, tx, tenant, ec, earlier.ID); err != nil {
				return Finding{}, err
			} else if why != "" {
				heldPair = why
				continue
			}
		}
		risk := "MEDIUM"
		if later.SessionID != "" && later.SessionID == earlier.SessionID {
			risk = "HIGH"
		}
		if earlier.CreditStatus == txn.CreditConfirmed && later.CreditStatus == txn.CreditConfirmed {
			risk = "HIGH"
		}
		laterEv := ev
		if later.ID != t.ID {
			if laterEv, err = d.evidence(ctx, tx, tenant, later.ID); err != nil {
				return Finding{}, err
			}
		}
		laterEv = append(laterEv, cases.Evidence{Kind: "related_transaction", Ref: earlier.Ref, IsFact: true,
			Detail: map[string]any{"role": "earlier_debit", "instruction_ref": earlier.Ref, "first_debit_at": earlier.FirstDebitAt.UTC().Format(time.RFC3339),
				"credit_status": earlier.CreditStatus, "window_seconds": window.Value}})
		f, err := d.open(ctx, tx, tenant, ec, later, cases.CodeDuplicate, RuleDuplicate, risk, "duplicate_retry",
			fmt.Sprintf("Another debit with the same account, amount, currency and beneficiary (%s) was posted within %d seconds. Risk %s.", earlier.Ref, window.Value, risk), laterEv)
		if err != nil {
			return Finding{}, err
		}
		if f.Created || result.Case.ID == uuid.Nil {
			result = f
		}
	}
	if heldPair != "" {
		if err := d.hold(ctx, tx, tenant, t.ID, cases.CodeDuplicate, heldPair); err != nil {
			return Finding{}, err
		}
	} else if err := d.release(ctx, tx, tenant, t.ID, cases.CodeDuplicate); err != nil {
		return Finding{}, err
	}
	return result, nil
}

// closeLateSuccess closes an open "unknown" case once the transaction is later
// confirmed successful, so nobody reverses a transaction that actually succeeded.
func (d *Detector) closeLateSuccess(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ec *run, t txnRow) error {
	id := t.ID
	list, err := d.Cases.List(ctx, tx, tenant, cases.Filter{Classification: cases.CodeUnknown, TxnID: &id}, nil, 50)
	if err != nil {
		return err
	}
	for _, c := range list {
		if cases.IsTerminal(c.Status) || !cases.CanTransition(c.Status, cases.StClosedNoException) {
			continue
		}
		if ok, err := d.allowed(ctx, tx, tenant, ec, t.Rail, cases.CodeUnknown, RuleUnknown); err != nil || !ok {
			return err
		}
		if _, err := d.Cases.Transition(ctx, tx, tenant, c.ID, cases.System, cases.StClosedNoException, 0, "LATE_SUCCESS_CONFIRMED"); err != nil {
			return err
		}
	}
	return nil
}

// Handle is the stream consumer for the "transactions" stream.
func (d *Detector) Handle(ctx context.Context, tx pgx.Tx, m stream.Message) error {
	var p struct {
		TxnID uuid.UUID `json:"txn_id"`
	}
	if err := json.Unmarshal(m.Payload, &p); err != nil || p.TxnID == uuid.Nil {
		return fmt.Errorf("detect: bad payload for event %s", m.EventID)
	}
	_, err := d.Evaluate(ctx, tx, m.TenantID, p.TxnID)
	return err
}

// SweepStuck opens an "unknown" case for transactions that are still pending
// after the rail's approved waiting threshold. A rail with no approved threshold
// is skipped: nothing is assumed.
func (d *Detector) SweepStuck(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) ([]Finding, error) {
	rows, err := tx.Query(ctx, txnSelect+` WHERE tenant_id = $1 AND lifecycle_state IN ('PENDING', 'DEBITED') AND debit_status = 'POSTED'`, tenant)
	if err != nil {
		return nil, err
	}
	var stuck []txnRow
	for rows.Next() {
		t, err := scanTxn(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		stuck = append(stuck, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []Finding
	shared := &run{at: d.now()}
	for _, t := range stuck {
		v, err := d.Config.Effective(ctx, tx, tenant, "waiting_threshold_seconds."+t.Rail, d.now())
		if errors.Is(err, configsvc.ErrNotConfigured) || errors.Is(err, configsvc.ErrUnknownKey) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if d.now().Sub(t.LastEventAt) < time.Duration(v.Value)*time.Second {
			continue
		}
		// If a source this transaction depends on is not reporting, its silence says
		// nothing about the transaction yet.
		if why, err := d.whyHeld(ctx, tx, tenant, shared, t.ID); err != nil {
			return nil, err
		} else if why != "" {
			if err := d.hold(ctx, tx, tenant, t.ID, cases.CodeUnknown, why); err != nil {
				return nil, err
			}
			continue
		}
		if err := d.release(ctx, tx, tenant, t.ID, cases.CodeUnknown); err != nil {
			return nil, err
		}
		ev, err := d.evidence(ctx, tx, tenant, t.ID)
		if err != nil {
			return nil, err
		}
		ec := &run{at: d.now()}
		f, err := d.open(ctx, tx, tenant, ec, t, cases.CodeUnknown, RuleStuck, "", "switch_response_failure",
			fmt.Sprintf("No final answer %d seconds after the last event, past the rail's waiting threshold of %d seconds.", int(d.now().Sub(t.LastEventAt).Seconds()), v.Value), ev)
		if err != nil {
			return nil, err
		}
		if ec.held != nil {
			if err := automation.Defer(ctx, tx, tenant, automation.KindDetect, t.ID, ec.held); err != nil {
				return nil, err
			}
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

// SweepAll runs SweepStuck for every institution. It reads the list of
// institutions with the pool's own role, then does the work tenant by tenant.
func (d *Detector) SweepAll(ctx context.Context, pool *db.Pool) (int, error) {
	rows, err := pool.Query(ctx, `SELECT id FROM institutions ORDER BY code`)
	if err != nil {
		return 0, err
	}
	var tenants []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		tenants = append(tenants, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	opened := 0
	for _, t := range tenants {
		err := pool.WithTenant(ctx, t, func(tx pgx.Tx) error {
			paused, frozen, err := d.pauseState(ctx, tx, t)
			if err != nil {
				return err
			}
			if paused {
				// A global freeze covers everything the sweep could do. It is recorded, not run.
				return automation.RecordJob(ctx, tx, t, automation.JobDetectorSweep, true, frozen, map[string]any{"opened": 0})
			}
			fs, err := d.SweepStuck(ctx, tx, t)
			if err != nil {
				return err
			}
			rf, err := d.SweepRules(ctx, tx, t)
			if err != nil {
				return err
			}
			fs = append(fs, rf...)
			hf, err := d.SweepHolds(ctx, tx, t)
			if err != nil {
				return err
			}
			fs = append(fs, hf...)
			held, err := OpenHolds(ctx, tx, t)
			if err != nil {
				return err
			}
			n := 0
			for _, f := range fs {
				if f.Created {
					opened++
					n++
				}
			}
			return automation.RecordJob(ctx, tx, t, automation.JobDetectorSweep, false, frozen, map[string]any{"opened": n, "held_for_degraded_source": held})
		})
		if err != nil {
			return opened, fmt.Errorf("detect: sweep tenant %s: %w", t, err)
		}
	}
	return opened, nil
}

// pauseState reports whether a global freeze pauses a job, and a summary of every
// frozen scope for display. With no Kill service nothing is paused.
func (d *Detector) pauseState(ctx context.Context, tx pgx.Tx, tenant uuid.UUID) (paused bool, frozen string, err error) {
	if d.Kill == nil {
		return false, "", nil
	}
	st, err := d.Kill.State(ctx, tx, tenant)
	if err != nil {
		return false, "", fmt.Errorf("detect: cannot read the kill switch: %w", err)
	}
	for _, f := range st.Frozen {
		if f.Target.Scope == killswitch.ScopeGlobal {
			paused = true
		}
	}
	return paused, automation.FrozenSummary(st), nil
}

// ResumeDeferred re-evaluates transactions whose detection was held by a freeze.
// Each is checked against the switch again as it is processed, so an item stays
// held while a covering freeze remains and is done exactly once after it lifts.
// Cases opened this way are dated from the original detection, so the SLA clock
// does not start late because of the freeze, and say so in their explanation.
func (d *Detector) ResumeDeferred(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, limit int) (resumed, stillHeld int, err error) {
	items, err := automation.Claim(ctx, tx, tenant, automation.KindDetect, limit)
	if err != nil {
		return 0, 0, err
	}
	for _, it := range items {
		ec := &run{at: it.CreatedAt, resumed: true}
		if _, err := d.evaluate(ctx, tx, tenant, it.SubjectID, ec); err != nil {
			return resumed, stillHeld, err
		}
		if ec.held != nil {
			if err := automation.StillHeld(ctx, tx, tenant, it.ID); err != nil {
				return resumed, stillHeld, err
			}
			stillHeld++
			continue
		}
		if err := automation.Resumed(ctx, tx, tenant, it.ID); err != nil {
			return resumed, stillHeld, err
		}
		resumed++
	}
	return resumed, stillHeld, nil
}

// ResumeAll runs ResumeDeferred for every institution and records the job state.
func (d *Detector) ResumeAll(ctx context.Context, pool *db.Pool) (resumed int, err error) {
	rows, err := pool.Query(ctx, `SELECT id FROM institutions ORDER BY code`)
	if err != nil {
		return 0, err
	}
	var tenants []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		tenants = append(tenants, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, t := range tenants {
		err := pool.WithTenant(ctx, t, func(tx pgx.Tx) error {
			_, frozen, err := d.pauseState(ctx, tx, t)
			if err != nil {
				return err
			}
			r, held, err := d.ResumeDeferred(ctx, tx, t, 100)
			if err != nil {
				return err
			}
			resumed += r
			open, err := automation.Counts(ctx, tx, t)
			if err != nil {
				return err
			}
			return automation.RecordJob(ctx, tx, t, automation.JobResumeHeld, false, frozen,
				map[string]any{"resumed": r, "still_held": held, "open": open[automation.KindDetect]})
		})
		if err != nil {
			return resumed, fmt.Errorf("detect: resume tenant %s: %w", t, err)
		}
	}
	return resumed, nil
}
