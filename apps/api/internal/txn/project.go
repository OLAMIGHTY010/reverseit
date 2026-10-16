package txn

import "time"

// Lifecycle states of a transaction (v2 B2).
const (
	StateInitiated        = "INITIATED"
	StateDebited          = "DEBITED"
	StatePending          = "PENDING"
	StateConfirmedSuccess = "CONFIRMED_SUCCESS"
	StateConfirmedFailure = "CONFIRMED_FAILURE"
	StateUnknown          = "UNKNOWN"
	StateConflicting      = "CONFLICTING"
	StateReversed         = "REVERSED"
)

// Credit, debit and settlement statuses.
const (
	CreditNone        = "NONE"
	CreditPending     = "PENDING"
	CreditConfirmed   = "CONFIRMED"
	CreditFailed      = "FAILED"
	CreditUnknown     = "UNKNOWN"
	CreditConflicting = "CONFLICTING"

	DebitNone   = "NONE"
	DebitPosted = "POSTED"

	SettlementNone      = "NONE"
	SettlementCompleted = "COMPLETED"

	ReversalNone   = "NONE"
	ReversalPosted = "POSTED"
)

// Seen is one stored event, reduced to what the projection needs.
type Seen struct {
	Type string
	At   time.Time
}

// Projection is the state computed from a transaction's events.
type Projection struct {
	Lifecycle       string
	DebitStatus     string
	CreditStatus    string
	SettlementState string
	ReversalStatus  string
	FirstDebitAt    *time.Time
	FirstEventAt    time.Time
	LastEventAt     time.Time
	EventCount      int
}

// Project computes the state from events. It depends only on which events exist
// and their timestamps, never on the order they are listed in, so replaying the
// same events shuffled gives the same result.
//
// Rules, in plain words:
//   - A debit with a confirmed credit is a success. A debit with a failed credit
//     is a confirmed failure. Both together is a conflict, to be resolved by
//     evidence, never by guessing.
//   - A response timeout with no final answer is UNKNOWN, never failure.
//   - A late confirmation after a timeout turns UNKNOWN into success.
//   - Sent but unanswered is PENDING.
//   - A posted reversal of a failed transaction makes it REVERSED.
func Project(events []Seen) Projection {
	p := Projection{Lifecycle: StateInitiated, DebitStatus: DebitNone, CreditStatus: CreditNone,
		SettlementState: SettlementNone, ReversalStatus: ReversalNone, EventCount: len(events)}
	if len(events) == 0 {
		return p
	}

	var debit, sent, confirmed, failed, timeout, settled, reversed bool
	for i, e := range events {
		if i == 0 || e.At.Before(p.FirstEventAt) {
			p.FirstEventAt = e.At
		}
		if i == 0 || e.At.After(p.LastEventAt) {
			p.LastEventAt = e.At
		}
		switch e.Type {
		case EvDebitPosted:
			debit = true
			if p.FirstDebitAt == nil || e.At.Before(*p.FirstDebitAt) {
				t := e.At
				p.FirstDebitAt = &t
			}
		case EvTransferSent:
			sent = true
		case EvCreditConfirmed:
			confirmed = true
		case EvCreditFailed:
			failed = true
		case EvTransferTimeout:
			timeout = true
		case EvSettlementCompleted:
			settled = true
		case EvReversalPosted:
			reversed = true
		}
	}

	if debit {
		p.DebitStatus = DebitPosted
	}
	if settled {
		p.SettlementState = SettlementCompleted
	}
	if reversed {
		p.ReversalStatus = ReversalPosted
	}

	switch {
	case confirmed && failed:
		p.CreditStatus = CreditConflicting
	case confirmed:
		p.CreditStatus = CreditConfirmed
	case failed:
		p.CreditStatus = CreditFailed
	case timeout:
		p.CreditStatus = CreditUnknown
	case sent:
		p.CreditStatus = CreditPending
	}

	switch {
	case !debit:
		p.Lifecycle = StateInitiated
	case p.CreditStatus == CreditConflicting:
		p.Lifecycle = StateConflicting
	case p.CreditStatus == CreditConfirmed:
		p.Lifecycle = StateConfirmedSuccess
	case p.CreditStatus == CreditFailed && reversed:
		p.Lifecycle = StateReversed
	case p.CreditStatus == CreditFailed:
		p.Lifecycle = StateConfirmedFailure
	case p.CreditStatus == CreditUnknown:
		p.Lifecycle = StateUnknown
	case p.CreditStatus == CreditPending:
		p.Lifecycle = StatePending
	default:
		p.Lifecycle = StateDebited
	}
	return p
}
