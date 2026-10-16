package reversal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// Verdicts of the gate.
const (
	// PassAuto means every check passed and policy lets automation proceed.
	PassAuto = "PASS_AUTO"
	// PassNeedsApproval means every check passed but a second person must approve.
	PassNeedsApproval = "PASS_NEEDS_APPROVAL"
	// Blocked means at least one check failed. Nothing is done. Any ambiguity
	// ends here: the answer to doubt is HOLD, never a reversal.
	Blocked = "BLOCKED"
)

// Check is one of the eight reversal checks.
type Check struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// Gate is the result of evaluating the eight checks.
type Gate struct {
	Verdict        string  `json:"verdict"`
	Checks         []Check `json:"checks"`
	ApprovalReason string  `json:"approval_reason,omitempty"`
	PolicyVersion  string  `json:"policy_version"`
	EvaluatedAt    string  `json:"evaluated_at"`
	StatusOutcome  string  `json:"status_outcome,omitempty"`
	StatusEvidence string  `json:"status_evidence,omitempty"`
}

// Failed lists the checks that did not pass.
func (g Gate) Failed() []Check {
	var out []Check
	for _, c := range g.Checks {
		if !c.Passed {
			out = append(out, c)
		}
	}
	return out
}

// Names of the eight checks, in the order the playbook lists them.
var checkNames = [9]string{"", "original debit confirmed", "fresh authoritative status", "beneficiary credit and settlement checked",
	"waiting threshold elapsed", "no prior remediation", "rail and institution policy permit", "authority", "safe submission"}

// Who is asking.
type Requester struct {
	// Principal is the signed-in operator, nil for automation.
	Principal *identity.Principal
}

func (r Requester) system() bool { return r.Principal == nil }

// subject is what the gate looks at.
type subject struct {
	Case cases.Case
	Txn  txnFacts
}

type txnFacts struct {
	ID             uuid.UUID
	Ref            string
	Rail           string
	Amount         int64
	Currency       string
	DebitToken     string
	BeneficiaryTok string
	DebitStatus    string
	CreditStatus   string
	ReversalStatus string
	Lifecycle      string
	LastEventAt    time.Time
}

func loadTxn(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (txnFacts, error) {
	var t txnFacts
	err := tx.QueryRow(ctx, `SELECT id, instruction_ref, rail, amount_minor, currency, debit_account_token, beneficiary_token, debit_status,
		credit_status, reversal_status, lifecycle_state, last_event_at FROM transactions WHERE tenant_id = $1 AND id = $2`, tenant, id).
		Scan(&t.ID, &t.Ref, &t.Rail, &t.Amount, &t.Currency, &t.DebitToken, &t.BeneficiaryTok, &t.DebitStatus, &t.CreditStatus, &t.ReversalStatus, &t.Lifecycle, &t.LastEventAt)
	return t, err
}

// eligible exception types. Others are not reversible by this workflow.
func reversibleCode(code string) bool {
	return code == cases.CodeNoCredit || code == cases.CodeDuplicate || code == cases.CodeAtmFault || code == cases.CodeVendorDrop || code == cases.CodeDisputedFee
}

type policyValues struct {
	freshness    *int64
	waiting      *int64
	autoLimit    *int64
	autoEligible bool
	execEnabled  bool
	versions     []string
}

func (s *Service) policy(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, rail, code string, now time.Time) (policyValues, error) {
	var p policyValues
	get := func(key string) (*int64, error) {
		v, err := s.Config.Effective(ctx, tx, tenant, key, now)
		if errors.Is(err, configsvc.ErrNotConfigured) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		p.versions = append(p.versions, fmt.Sprintf("%s@%d", key, v.Version))
		x := v.Value
		return &x, nil
	}
	var err error
	if p.freshness, err = get("status_query.freshness_seconds"); err != nil {
		return p, err
	}
	if p.waiting, err = get("waiting_threshold_seconds." + rail); err != nil && !errors.Is(err, configsvc.ErrUnknownKey) {
		return p, err
	}
	if p.autoLimit, err = get("approval.auto_limit_minor"); err != nil {
		return p, err
	}
	if flag, err := get("reversal.auto_eligible." + code); err == nil && flag != nil && *flag == 1 {
		p.autoEligible = true
	} else if err != nil && !errors.Is(err, configsvc.ErrUnknownKey) {
		return p, err
	}
	if flag, err := get("reversal.simulated_execution_enabled"); err != nil {
		return p, err
	} else if flag != nil && *flag == 1 {
		p.execEnabled = true
	}
	return p, nil
}

// evaluate runs the eight checks and returns the gate with a verdict. It reads
// and writes nothing except the simulated provider is never called. requester
// says who is asking. key is the idempotency key the caller intends to use.
// execution is true when re-checking just before submission, when the question
// of who asked has already been settled.
func (s *Service) evaluate(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, sub subject, who Requester, key string, execution bool) (Gate, error) {
	return s.evaluateOwn(ctx, tx, tenant, sub, who, key, execution, uuid.Nil)
}

// evaluateOwn is evaluate for a reversal that already exists and is being renewed:
// that reversal's own remediation lock is not counted against it in check 5.
func (s *Service) evaluateOwn(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, sub subject, who Requester, key string, execution bool, own uuid.UUID) (Gate, error) {
	now := s.now()
	g := Gate{EvaluatedAt: now.Format(time.RFC3339)}
	pol, err := s.policy(ctx, tx, tenant, sub.Txn.Rail, sub.Case.Classification, now)
	if err != nil {
		return g, err
	}
	g.PolicyVersion = strings.Join(pol.versions, ";")
	if g.PolicyVersion == "" {
		g.PolicyVersion = "no approved policy"
	}
	add := func(id int, ok bool, detail string) {
		g.Checks = append(g.Checks, Check{ID: id, Name: checkNames[id], Passed: ok, Detail: detail})
	}
	t := sub.Txn

	// 1. Original debit confirmed, in the ledger and in the transaction record.
	if entry, found, err := s.Ledger.Debit(ctx, tx, tenant, t.Ref); err != nil {
		return g, err
	} else {
		switch {
		case !found:
			add(1, false, "the ledger has no debit for this transaction")
		case entry.AmountMinor != t.Amount || entry.Currency != t.Currency:
			add(1, false, fmt.Sprintf("the ledger debit (%d %s) does not match the transaction (%d %s)", entry.AmountMinor, entry.Currency, t.Amount, t.Currency))
		case t.DebitStatus != txn.DebitPosted:
			add(1, false, "the transaction record does not show a posted debit")
		default:
			add(1, true, "the ledger debit matches the transaction")
		}
	}

	// 2. Fresh evidence from the authoritative source. Only a confirmed failure passes.
	answer, err := s.Status.Query(ctx, tx, tenant, t.Ref, now)
	if err != nil {
		return g, err
	}
	g.StatusOutcome, g.StatusEvidence = answer.Outcome, answer.EvidenceRef
	switch {
	case pol.freshness == nil:
		add(2, false, "no approved freshness limit for status evidence: nothing can count as fresh")
	case now.Sub(answer.AsOf) > time.Duration(*pol.freshness)*time.Second:
		add(2, false, fmt.Sprintf("the status answer is %d seconds old, over the %d second limit", int(now.Sub(answer.AsOf).Seconds()), *pol.freshness))
	case answer.Outcome == OutcomeFailure:
		add(2, true, "the authoritative source confirms the transfer failed")
	case answer.Outcome == OutcomeUnknown:
		add(2, false, "the outcome is UNKNOWN. Unknown is not failed, so nothing is reversed")
	case answer.Outcome == OutcomePending:
		add(2, false, "the transfer is still pending at the source")
	case answer.Outcome == OutcomeConflicting:
		add(2, false, "sources conflict about this transfer")
	case answer.Outcome == OutcomeSuccess:
		add(2, false, "the authoritative source says the transfer succeeded")
	default:
		add(2, false, "unrecognised status outcome "+answer.Outcome)
	}

	// 3. The beneficiary was not credited and settlement has not paid out.
	switch {
	case answer.BeneficiaryCredited:
		add(3, false, "the beneficiary was credited")
	case answer.SettlementPaid:
		add(3, false, "settlement has already paid out")
	case t.CreditStatus == txn.CreditConfirmed:
		add(3, false, "the transaction record shows a confirmed credit")
	default:
		add(3, true, "no beneficiary credit and no settlement payout")
	}

	// 4. The configured waiting time has passed since the last event.
	switch {
	case pol.waiting == nil:
		add(4, false, "no approved waiting threshold for rail "+t.Rail)
	case now.Sub(t.LastEventAt) < time.Duration(*pol.waiting)*time.Second:
		add(4, false, fmt.Sprintf("only %d seconds have passed, the threshold is %d", int(now.Sub(t.LastEventAt).Seconds()), *pol.waiting))
	default:
		add(4, true, fmt.Sprintf("%d seconds have passed, the threshold is %d", int(now.Sub(t.LastEventAt).Seconds()), *pol.waiting))
	}

	// 5. No remediation already exists for this debit.
	var live string
	lerr := tx.QueryRow(ctx, `SELECT reversal_ref FROM reversals WHERE tenant_id = $1 AND txn_id = $2
		AND approval_status <> 'REJECTED' AND accepted_status <> 'REJECTED' AND financial_status <> 'FAILED' AND id <> $3`, tenant, t.ID, own).Scan(&live)
	switch {
	case lerr == nil && !execution:
		add(5, false, "reversal "+live+" already exists for this transaction")
	case lerr != nil && !errors.Is(lerr, pgx.ErrNoRows):
		return g, lerr
	case t.ReversalStatus == txn.ReversalPosted:
		add(5, false, "a reversal has already posted for this transaction")
	default:
		add(5, true, "no earlier reversal")
	}

	// 6. Policy permits this exception type, rail and amount, and nothing is frozen.
	auto := false
	switch {
	case !reversibleCode(sub.Case.Classification):
		add(6, false, "exception type "+sub.Case.Classification+" is not reversible by this workflow")
	case !pol.execEnabled:
		add(6, false, "simulated reversal execution is not enabled by approved policy")
	default:
		if err := s.Kill.Check(ctx, tx, tenant, actionFor(sub.Case.Classification, t.Rail)); err != nil {
			add(6, false, err.Error())
		} else {
			auto = pol.autoEligible && pol.autoLimit != nil && t.Amount <= *pol.autoLimit
			if auto {
				add(6, true, "policy lets automation reverse this exception type at this amount")
			} else {
				g.ApprovalReason = approvalReason(pol, sub.Case.Classification, t.Amount)
				add(6, true, "policy permits a reversal here but requires a second person to approve it")
			}
		}
	}

	// 7. The requester has authority.
	switch {
	case execution:
		add(7, true, "authority was established when the reversal was requested and approved")
	case who.system():
		if auto {
			add(7, true, "automation policy covers this reversal")
		} else {
			add(7, true, "automation may request this reversal; a person must approve it")
		}
	default:
		if _, err := s.Policy.GrantFor(who.Principal, "request_reversal", authz.Attrs{}); err != nil {
			add(7, false, "the requester does not hold the request_reversal grant")
		} else {
			g.ApprovalReason = "a reversal requested by a person always needs a different person to approve it"
			auto = false
			add(7, true, "the requester may request reversals; a second person must approve")
		}
	}

	// 8. Safe submission: a usable idempotency key and a free remediation lock.
	switch {
	case execution:
		add(8, true, "the idempotency key and lock were taken when the reversal was requested")
	case strings.TrimSpace(key) == "" || len(key) > 200:
		add(8, false, "no usable idempotency key")
	default:
		add(8, true, "idempotency key present; the remediation lock is taken when the reversal is recorded")
	}

	for _, c := range g.Checks {
		if !c.Passed {
			g.Verdict = Blocked
			return g, nil
		}
	}
	if auto && g.ApprovalReason == "" {
		g.Verdict = PassAuto
	} else {
		if g.ApprovalReason == "" {
			g.ApprovalReason = "approval required"
		}
		g.Verdict = PassNeedsApproval
	}
	return g, nil
}

func approvalReason(p policyValues, code string, amount int64) string {
	switch {
	case !p.autoEligible:
		return "policy does not allow automation to reverse " + code + " without a second person"
	case p.autoLimit == nil:
		return "no approved automatic limit: every amount needs a second person"
	default:
		return fmt.Sprintf("the amount %d is over the automatic limit of %d", amount, *p.autoLimit)
	}
}

func (g Gate) json() []byte {
	b, _ := json.Marshal(g)
	return b
}
