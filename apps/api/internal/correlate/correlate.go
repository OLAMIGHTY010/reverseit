// Package correlate matches evidence to transactions when the source does not know
// our instruction reference.
//
// A switch, a gateway or a settlement feed knows its own references (RRN, STAN,
// session, ledger id, gateway reference). Each such reference is only evidence of a
// link, so a match is scored from which references agree, whether the amount and
// currency agree, and whether the times are close. A confident, unambiguous match is
// attached as an ordinary event on the transaction. Anything else goes to a queue of
// unmatched evidence for a person to link or discard, and is never guessed.
//
// The scores are engineering defaults for synthetic data, not a policy. They are
// parameters on Matcher so they can be tuned and tested.
package correlate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Observation is one piece of evidence from a source that identifies the
// transaction only by its own references.
type Observation struct {
	SourceSystem   string     `json:"source_system"`
	SourceEventRef string     `json:"source_event_ref"`
	EventType      string     `json:"event_type"`
	SourceTS       time.Time  `json:"source_ts"`
	Refs           txn.Refs   `json:"refs"`
	AmountMinor    txn.Amount `json:"amount_minor"`
	Currency       string     `json:"currency"`
}

// ErrInvalid wraps every problem with an observation.
var ErrInvalid = errors.New("correlate: invalid observation")

func (o *Observation) validate() error {
	switch {
	case o.SourceSystem == "" || o.SourceEventRef == "" || o.EventType == "" || o.SourceTS.IsZero():
		return fmt.Errorf("%w: source_system, source_event_ref, event_type and source_ts are required", ErrInvalid)
	case len(o.Refs) == 0:
		return fmt.Errorf("%w: at least one external reference is required", ErrInvalid)
	case o.AmountMinor < 1 || o.Currency == "":
		return fmt.Errorf("%w: amount_minor and currency are required", ErrInvalid)
	}
	for k := range o.Refs {
		if !txn.RefTypes[k] {
			return fmt.Errorf("%w: %s is not a known reference type", ErrInvalid, k)
		}
	}
	return nil
}

func (o *Observation) hash() string {
	c := *o
	c.SourceTS = c.SourceTS.UTC().Truncate(time.Microsecond)
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Candidate is a transaction an observation might belong to, with the reasons.
type Candidate struct {
	TxnID      uuid.UUID `json:"txn_id"`
	Instr      string    `json:"instruction_ref"`
	Confidence float64   `json:"confidence"`
	Basis      []string  `json:"basis"`
}

// Matcher holds the scoring parameters.
type Matcher struct {
	Txn *txn.Service
	// AutoLink is the lowest confidence that links without a person. Default 0.9.
	AutoLink float64
	// Window is how far apart an observation and a transaction may be in time and
	// still count as the same. Default 48 hours.
	Window time.Duration
	// Margin is how much better the best candidate must be than the next. Default 0.05.
	Margin float64
}

func (m *Matcher) autoLink() float64 {
	if m.AutoLink > 0 {
		return m.AutoLink
	}
	return 0.9
}
func (m *Matcher) window() time.Duration {
	if m.Window > 0 {
		return m.Window
	}
	return 48 * time.Hour
}
func (m *Matcher) margin() float64 {
	if m.Margin > 0 {
		return m.Margin
	}
	return 0.05
}

type txnRow struct {
	id       uuid.UUID
	instr    string
	amount   int64
	currency string
	first    time.Time
	core     txn.Core
}

// Candidates scores every transaction that shares a reference with the observation.
func (m *Matcher) Candidates(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, o Observation) ([]Candidate, error) {
	matched := map[uuid.UUID]map[string]bool{}
	for k, v := range o.Refs {
		rows, err := tx.Query(ctx, `SELECT txn_id FROM transaction_refs WHERE tenant_id = $1 AND ref_type = $2 AND ref_value = $3`, tenant, k, v)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			if matched[id] == nil {
				matched[id] = map[string]bool{}
			}
			matched[id][k] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	var out []Candidate
	for id, refs := range matched {
		var t txnRow
		err := tx.QueryRow(ctx, `SELECT id, instruction_ref, amount_minor, currency, first_event_at FROM transactions WHERE tenant_id = $1 AND id = $2`, tenant, id).
			Scan(&t.id, &t.instr, &t.amount, &t.currency, &t.first)
		if err != nil {
			return nil, err
		}
		out = append(out, m.score(t, refs, o))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].TxnID.String() < out[j].TxnID.String()
	})
	return out, nil
}

// score rates one candidate. The reference evidence sets a ceiling, and a
// disagreement on amount, currency or time brings the score under the automatic
// threshold whatever the references say.
func (m *Matcher) score(t txnRow, refs map[string]bool, o Observation) Candidate {
	c := Candidate{TxnID: t.id, Instr: t.instr}
	var base float64
	switch {
	case refs["LEDGER_ID"]:
		base, c.Basis = 1.0, append(c.Basis, "ledger id matches")
	case refs["GATEWAY_REF"]:
		base, c.Basis = 1.0, append(c.Basis, "gateway reference matches")
	case refs["RRN"] && refs["STAN"]:
		base, c.Basis = 0.95, append(c.Basis, "RRN and STAN match")
	case refs["SESSION_ID"]:
		base, c.Basis = 0.9, append(c.Basis, "session id matches")
	case refs["RRN"]:
		base, c.Basis = 0.7, append(c.Basis, "RRN matches alone")
	case refs["STAN"]:
		base, c.Basis = 0.5, append(c.Basis, "STAN matches alone")
	}
	if int64(o.AmountMinor) == t.amount && o.Currency == t.currency {
		c.Basis = append(c.Basis, "amount and currency agree")
	} else {
		base = min(base, 0.4)
		c.Basis = append(c.Basis, "amount or currency differs")
	}
	gap := o.SourceTS.Sub(t.first)
	if gap < 0 {
		gap = -gap
	}
	if gap > m.window() {
		base = min(base, 0.3)
		c.Basis = append(c.Basis, "times are far apart")
	}
	c.Confidence = base
	return c
}

// Decision is what Resolve concluded.
type Decision struct {
	Linked     bool        `json:"linked"`
	TxnID      uuid.UUID   `json:"txn_id,omitempty"`
	Confidence float64     `json:"confidence"`
	Basis      []string    `json:"basis,omitempty"`
	Reason     string      `json:"reason,omitempty"`
	Queued     uuid.UUID   `json:"unmatched_id,omitempty"`
	Duplicate  bool        `json:"duplicate"`
	Candidates []Candidate `json:"candidates,omitempty"`
}

// Resolve correlates one observation in the caller's tenant transaction. If one
// candidate is confident enough and clearly better than the rest, the observation is
// attached to that transaction as an event. Otherwise it is queued as unmatched with
// the reason and the candidates. Repeating an observation changes nothing.
func (m *Matcher) Resolve(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, o Observation) (Decision, error) {
	if err := o.validate(); err != nil {
		return Decision{}, err
	}
	o.SourceTS = o.SourceTS.UTC()
	h := o.hash()

	// Two deliveries of one observation at once are taken in turn.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenant.String()+"|"+o.SourceSystem+"|"+o.SourceEventRef); err != nil {
		return Decision{}, err
	}

	// An observation already seen keeps its first outcome.
	var prevID uuid.UUID
	var prevHash, prevStatus string
	var prevTxn *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id, observation_hash, status, linked_txn_id FROM evidence_correlations
		WHERE tenant_id = $1 AND source_system = $2 AND source_event_ref = $3`, tenant, o.SourceSystem, o.SourceEventRef).Scan(&prevID, &prevHash, &prevStatus, &prevTxn)
	switch {
	case err == nil && prevHash != h:
		return Decision{}, txn.ErrConflict
	case err == nil:
		d := Decision{Duplicate: true, Queued: prevID, Reason: "seen before; status " + prevStatus}
		if prevTxn != nil {
			d.Linked, d.TxnID = true, *prevTxn
		}
		return d, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return Decision{}, err
	}

	d, err := m.decide(ctx, tx, tenant, o)
	if err != nil {
		return Decision{}, err
	}
	obs, _ := json.Marshal(o)
	cands, _ := json.Marshal(d.Candidates)
	status, method := "open", ""
	var linked *uuid.UUID
	var conf *float64
	if d.Linked {
		status, method, linked, conf = "linked", "automatic", &d.TxnID, &d.Confidence
	}
	if err := tx.QueryRow(ctx, `INSERT INTO evidence_correlations (tenant_id, source_system, source_event_ref, observation, observation_hash, reason, candidates, status, linked_txn_id, link_method, confidence, resolved_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, CASE WHEN $8 = 'linked' THEN now() END) RETURNING id`,
		tenant, o.SourceSystem, o.SourceEventRef, obs, h, d.Reason, cands, status, linked, method, conf).Scan(&d.Queued); err != nil {
		return Decision{}, err
	}
	return d, nil
}

// decide scores the candidates and, if one is clear, attaches the observation.
func (m *Matcher) decide(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, o Observation) (Decision, error) {
	cands, err := m.Candidates(ctx, tx, tenant, o)
	if err != nil {
		return Decision{}, err
	}
	d := Decision{Candidates: cands}
	switch {
	case len(cands) == 0:
		d.Reason = "no transaction shares any of its references"
		return d, nil
	case cands[0].Confidence < m.autoLink():
		d.Reason = fmt.Sprintf("best match has confidence %.2f, below %.2f", cands[0].Confidence, m.autoLink())
		return d, nil
	case len(cands) > 1 && cands[0].Confidence-cands[1].Confidence < m.margin():
		d.Reason = "more than one transaction matches about equally well"
		return d, nil
	}
	if _, err := m.attach(ctx, tx, tenant, cands[0].TxnID, o); err != nil {
		if errors.Is(err, txn.ErrInvalid) || errors.Is(err, txn.ErrCoreMismatch) || errors.Is(err, txn.ErrConflict) {
			d.Reason = "the matching transaction refused the evidence: " + err.Error()
			return d, nil
		}
		return Decision{}, err
	}
	d.Linked, d.TxnID, d.Confidence, d.Basis = true, cands[0].TxnID, cands[0].Confidence, cands[0].Basis
	return d, nil
}

// attach ingests the observation as an event of the given transaction. The event
// takes the transaction's own core facts, so it can never change them.
func (m *Matcher) attach(ctx context.Context, tx pgx.Tx, tenant, txnID uuid.UUID, o Observation) (txn.Result, error) {
	var c txn.Core
	var amount int64
	err := tx.QueryRow(ctx, `SELECT instruction_ref, channel, rail, amount_minor, currency, debit_account_token, beneficiary_bank, beneficiary_token, session_id
		FROM transactions WHERE tenant_id = $1 AND id = $2`, tenant, txnID).
		Scan(&c.InstructionRef, &c.Channel, &c.Rail, &amount, &c.Currency, &c.DebitAccountToken, &c.BeneficiaryBank, &c.BeneficiaryToken, &c.SessionID)
	if err != nil {
		return txn.Result{}, err
	}
	c.AmountMinor = txn.Amount(amount)
	sp, err := tx.Begin(ctx)
	if err != nil {
		return txn.Result{}, err
	}
	res, err := m.Txn.Ingest(ctx, sp, tenant, txn.Envelope{SourceSystem: o.SourceSystem, SourceEventRef: o.SourceEventRef, EventType: o.EventType,
		SourceTS: o.SourceTS, Txn: c, Refs: o.Refs})
	if err != nil {
		_ = sp.Rollback(ctx)
		return txn.Result{}, err
	}
	return res, sp.Commit(ctx)
}
