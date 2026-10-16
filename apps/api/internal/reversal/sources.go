// Package reversal runs the controlled reversal workflow (B5).
//
// Nothing here moves real money. The three outside systems it depends on, the
// authoritative status source, the core ledger and the reversal provider, are
// interfaces. In Track H each has a simulated implementation backed by clearly
// labelled sim_* tables. A real integration replaces them without changing the
// workflow. There is no live provider in this build.
package reversal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Outcomes a status source can report. Only CONFIRMED_FAILURE can support a
// reversal. UNKNOWN is not a failure, and nothing here ever treats it as one.
const (
	OutcomeSuccess     = "CONFIRMED_SUCCESS"
	OutcomeFailure     = "CONFIRMED_FAILURE"
	OutcomePending     = "PENDING"
	OutcomeUnknown     = "UNKNOWN"
	OutcomeConflicting = "CONFLICTING"
)

// StatusAnswer is what the authoritative source says about a transaction.
type StatusAnswer struct {
	Outcome             string
	SourceSystem        string
	AsOf                time.Time
	BeneficiaryCredited bool
	SettlementPaid      bool
	EvidenceRef         string
	Simulated           bool
}

// StatusSource answers "what actually happened to this transaction?".
type StatusSource interface {
	Query(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, instructionRef string, now time.Time) (StatusAnswer, error)
}

// LedgerEntry is a posting in the core ledger.
type LedgerEntry struct {
	InstructionRef string
	Kind           string
	ReversalRef    string
	AmountMinor    int64
	Currency       string
	PostedAt       time.Time
}

// LedgerSource reads the core ledger. It is independent of the reversal
// provider: the provider's word is never accepted in place of a ledger posting.
type LedgerSource interface {
	Debit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, instructionRef string) (LedgerEntry, bool, error)
	ReversalPosting(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, instructionRef, reversalRef string) (LedgerEntry, bool, error)
}

// Submission is a reversal request sent to the provider.
type Submission struct {
	IdempotencyKey string
	InstructionRef string
	ReversalRef    string
	AmountMinor    int64
	Currency       string
	ReasonCode     string
}

// Ack is the provider's answer. Accepted is not completed: money has moved only
// when the ledger shows it.
type Ack struct {
	Accepted    string // ACCEPTED, REJECTED, UNKNOWN
	Processing  string // IN_PROGRESS, COMPLETED, FAILED, UNKNOWN
	ProviderRef string
	Reason      string
	Simulated   bool
}

// ErrProviderTimeout means the call got no answer. The outcome is UNKNOWN: the
// provider may or may not have acted, and the same idempotency key must be used
// to find out.
var ErrProviderTimeout = errors.New("reversal: the provider did not answer")

// Provider submits reversals and reports their progress.
type Provider interface {
	Submit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, s Submission) (Ack, error)
	// Status finds a request by its idempotency key. Found is false when the
	// provider never received it.
	Status(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, idempotencyKey string) (ack Ack, found bool, err error)
}

// ---- simulated implementations ---------------------------------------------

// SimStatus is the simulated authoritative status source (a stand-in for a
// payment switch status query).
type SimStatus struct{}

var _ StatusSource = SimStatus{}

// Query answers from the scripted outcome for the instruction. An instruction
// with no script is UNKNOWN: the source has no record.
func (SimStatus) Query(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ref string, now time.Time) (StatusAnswer, error) {
	a := StatusAnswer{Outcome: OutcomeUnknown, SourceSystem: "SIM_SWITCH_STATUS", AsOf: now, EvidenceRef: "SQ-" + ref, Simulated: true}
	var age int
	err := tx.QueryRow(ctx, `SELECT outcome, beneficiary_credited, settlement_paid, age_seconds FROM sim_status_script
		WHERE tenant_id = $1 AND instruction_ref = $2`, tenant, ref).Scan(&a.Outcome, &a.BeneficiaryCredited, &a.SettlementPaid, &age)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, nil
	}
	if err != nil {
		return StatusAnswer{}, fmt.Errorf("reversal: simulated status: %w", err)
	}
	a.AsOf = now.Add(-time.Duration(age) * time.Second)
	return a, nil
}

// SimLedger is the simulated core ledger.
type SimLedger struct{}

var _ LedgerSource = SimLedger{}

func (SimLedger) Debit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ref string) (LedgerEntry, bool, error) {
	return readEntry(ctx, tx, tenant, `instruction_ref = $2 AND kind = 'DEBIT'`, ref)
}

func (SimLedger) ReversalPosting(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ref, reversalRef string) (LedgerEntry, bool, error) {
	return readEntry(ctx, tx, tenant, `instruction_ref = $2 AND kind = 'REVERSAL_CREDIT' AND reversal_ref = $3`, ref, reversalRef)
}

func readEntry(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, where string, args ...any) (LedgerEntry, bool, error) {
	var e LedgerEntry
	err := tx.QueryRow(ctx, `SELECT instruction_ref, kind, reversal_ref, amount_minor, currency, posted_at FROM sim_ledger_entries
		WHERE tenant_id = $1 AND `+where, append([]any{tenant}, args...)...).Scan(&e.InstructionRef, &e.Kind, &e.ReversalRef, &e.AmountMinor, &e.Currency, &e.PostedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LedgerEntry{}, false, nil
	}
	if err != nil {
		return LedgerEntry{}, false, fmt.Errorf("reversal: simulated ledger: %w", err)
	}
	e.PostedAt = e.PostedAt.UTC()
	return e, true, nil
}

// SeedDebit records the original debit in the simulated ledger. Demo and test
// setup use it, standing in for core banking having posted the customer's debit.
func SeedDebit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ref string, amount int64, currency string) error {
	_, err := tx.Exec(ctx, `INSERT INTO sim_ledger_entries (tenant_id, instruction_ref, kind, amount_minor, currency)
		VALUES ($1,$2,'DEBIT',$3,$4) ON CONFLICT DO NOTHING`, tenant, ref, amount, currency)
	return err
}

// ScriptStatus sets what the simulated status source will say about a transaction.
func ScriptStatus(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ref, outcome string, credited, settled bool, ageSeconds int) error {
	_, err := tx.Exec(ctx, `INSERT INTO sim_status_script (tenant_id, instruction_ref, outcome, beneficiary_credited, settlement_paid, age_seconds)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (tenant_id, instruction_ref) DO UPDATE SET
		outcome = EXCLUDED.outcome, beneficiary_credited = EXCLUDED.beneficiary_credited, settlement_paid = EXCLUDED.settlement_paid,
		age_seconds = EXCLUDED.age_seconds, set_at = now()`, tenant, ref, outcome, credited, settled, ageSeconds)
	return err
}

// ScriptProvider sets how the simulated provider behaves for a transaction:
// SUCCESS, REJECT, TIMEOUT (acts but never answers), DELAYED (posts on the third
// poll) or GHOST (claims completion without posting to the ledger).
func ScriptProvider(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, ref, behaviour string) error {
	_, err := tx.Exec(ctx, `INSERT INTO sim_provider_script (tenant_id, instruction_ref, behaviour) VALUES ($1,$2,$3)
		ON CONFLICT (tenant_id, instruction_ref) DO UPDATE SET behaviour = EXCLUDED.behaviour`, tenant, ref, behaviour)
	return err
}

// SimProvider is the simulated reversal provider.
type SimProvider struct{}

var _ Provider = SimProvider{}

// Submit is idempotent on the key, as a real provider must be: a repeat returns
// the first answer and never posts twice.
func (SimProvider) Submit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, s Submission) (Ack, error) {
	var existing string
	err := tx.QueryRow(ctx, `SELECT state FROM sim_provider_requests WHERE tenant_id = $1 AND idempotency_key = $2`, tenant, s.IdempotencyKey).Scan(&existing)
	if err == nil {
		ack, _, serr := SimProvider{}.Status(ctx, tx, tenant, s.IdempotencyKey)
		return ack, serr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Ack{}, err
	}

	behaviour := "SUCCESS"
	_ = tx.QueryRow(ctx, `SELECT behaviour FROM sim_provider_script WHERE tenant_id = $1 AND instruction_ref = $2`, tenant, s.InstructionRef).Scan(&behaviour)
	providerRef := "SIMPROV-" + uuid.NewString()[:8]
	state := "ACCEPTED"
	if behaviour == "REJECT" {
		state = "REJECTED"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sim_provider_requests (tenant_id, idempotency_key, provider_ref, instruction_ref, amount_minor, currency, reversal_ref, behaviour, state)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, tenant, s.IdempotencyKey, providerRef, s.InstructionRef, s.AmountMinor, s.Currency, s.ReversalRef, behaviour, state); err != nil {
		return Ack{}, err
	}
	switch behaviour {
	case "REJECT":
		return Ack{Accepted: "REJECTED", Processing: "FAILED", ProviderRef: providerRef, Reason: "simulated provider rejected the request", Simulated: true}, nil
	case "TIMEOUT":
		return Ack{}, ErrProviderTimeout // it acted, but the caller never hears
	}
	return Ack{Accepted: "ACCEPTED", Processing: "IN_PROGRESS", ProviderRef: providerRef, Simulated: true}, nil
}

// Status advances the simulation by one poll and reports where the request is.
func (SimProvider) Status(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, key string) (Ack, bool, error) {
	var providerRef, ref, reversalRef, currency, behaviour, state string
	var amount int64
	var polls int
	err := tx.QueryRow(ctx, `SELECT provider_ref, instruction_ref, reversal_ref, currency, behaviour, state, amount_minor, polls
		FROM sim_provider_requests WHERE tenant_id = $1 AND idempotency_key = $2 FOR UPDATE`, tenant, key).
		Scan(&providerRef, &ref, &reversalRef, &currency, &behaviour, &state, &amount, &polls)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ack{}, false, nil
	}
	if err != nil {
		return Ack{}, false, err
	}
	if state == "REJECTED" {
		return Ack{Accepted: "REJECTED", Processing: "FAILED", ProviderRef: providerRef, Reason: "simulated provider rejected the request", Simulated: true}, true, nil
	}
	if state == "ACCEPTED" {
		polls++
		due := behaviour != "DELAYED" || polls >= 3
		if due {
			if behaviour != "GHOST" { // a ghost says it posted but never touches the ledger
				if _, err := tx.Exec(ctx, `INSERT INTO sim_ledger_entries (tenant_id, instruction_ref, kind, reversal_ref, amount_minor, currency)
					VALUES ($1,$2,'REVERSAL_CREDIT',$3,$4,$5) ON CONFLICT DO NOTHING`, tenant, ref, reversalRef, amount, currency); err != nil {
					return Ack{}, false, err
				}
			}
			state = "POSTED"
		}
		if _, err := tx.Exec(ctx, `UPDATE sim_provider_requests SET polls = $3, state = $4 WHERE tenant_id = $1 AND idempotency_key = $2`, tenant, key, polls, state); err != nil {
			return Ack{}, false, err
		}
	}
	processing := "IN_PROGRESS"
	if state == "POSTED" {
		processing = "COMPLETED"
	}
	return Ack{Accepted: "ACCEPTED", Processing: processing, ProviderRef: providerRef, Simulated: true}, true, nil
}
