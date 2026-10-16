package reversal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/smartreversal/smartreversal/apps/api/internal/authz"
	"github.com/smartreversal/smartreversal/apps/api/internal/cases"
	"github.com/smartreversal/smartreversal/apps/api/internal/configsvc"
	"github.com/smartreversal/smartreversal/apps/api/internal/events"
	"github.com/smartreversal/smartreversal/apps/api/internal/identity"
	"github.com/smartreversal/smartreversal/apps/api/internal/notify"
	"github.com/smartreversal/smartreversal/apps/api/internal/txn"
)

// approvalLimit is the largest amount any of the principal's roles may approve,
// from approved configuration. Nil means no limit is configured, which denies.
func (s *Service) approvalLimit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, p *identity.Principal) *int64 {
	var best *int64
	for _, role := range p.Roles {
		v, err := s.Config.Effective(ctx, tx, tenant, "approval.limit_minor."+role, s.now())
		if err != nil {
			continue
		}
		x := v.Value
		if best == nil || x > *best {
			best = &x
		}
	}
	return best
}

// Decide records a checker's decision on a reversal that needs approval.
//
// Approving needs the approve_reversal grant within the checker's limit, an
// MFA-backed token, and a checker who is not the maker and has no conflict with
// the accounts. It does not move money: the caller runs Execute afterwards.
func (s *Service) Decide(ctx context.Context, tenant uuid.UUID, checker *identity.Principal, approvalID uuid.UUID, approve bool, reason string) (Reversal, error) {
	reason = strings.TrimSpace(reason)
	if !approve && reason == "" {
		return Reversal{}, fmt.Errorf("%w: a reason is required to reject", cases.ErrInvalid)
	}
	var out Reversal
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		var revID uuid.UUID
		var maker *uuid.UUID
		var status string
		err := tx.QueryRow(ctx, `SELECT reversal_id, maker, status FROM approvals WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenant, approvalID).Scan(&revID, &maker, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status != "PENDING" {
			return ErrNotPending
		}
		rev, err := s.Get(ctx, tx, tenant, revID.String())
		if err != nil {
			return err
		}

		var limit *int64
		if approve {
			limit = s.approvalLimit(ctx, tx, tenant, checker)
		}
		amount := rev.AmountMinor
		attrs := authz.Attrs{AmountMinor: &amount, LimitMinor: limit}
		if !approve {
			// Rejecting is always safe, so it only needs the grant.
			if _, err := s.Policy.GrantFor(checker, "approve_reversal", authz.Attrs{AmountMinor: &amount, LimitMinor: ptr(amount)}); err != nil {
				return err
			}
		} else if _, err := s.Policy.Check(checker, "approve_reversal", attrs); err != nil {
			return err
		}

		// Maker is not checker, and the checker has no tie to the accounts.
		makerID := uuid.New() // automation as maker: any human may check, but the conflict check still runs
		if maker != nil {
			makerID = *maker
		}
		t, err := loadTxn(ctx, tx, tenant, rev.TxnID)
		if err != nil {
			return err
		}
		c0, err := s.Cases.Get(ctx, tx, tenant, rev.CaseID)
		if err != nil {
			return err
		}
		tokens := []string{t.DebitToken}
		if t.BeneficiaryTok != "" {
			tokens = append(tokens, t.BeneficiaryTok)
		}
		if err := s.SoD.CanApprove(ctx, checker, makerID, authz.Subject{AccountTokens: tokens}); err != nil {
			return err
		}

		newStatus, newApproval := "REJECTED", "REJECTED"
		if approve {
			newStatus, newApproval = "APPROVED", "APPROVED"
		}
		// An approval is valid for a configured time. Without an approved validity
		// period it could never expire, so it is not given.
		var expires *time.Time
		if approve {
			v, err := s.Config.Effective(ctx, tx, tenant, "approval.validity_seconds", s.now())
			if errors.Is(err, configsvc.ErrNotConfigured) {
				return ErrNoApprovalPolicy
			}
			if err != nil {
				return err
			}
			at := s.now().Add(time.Duration(v.Value) * time.Second)
			expires = &at
		}
		// The approval records the policy it was given under, so a later change that
		// matters can be recognised.
		basis, policyNow := "", ""
		if approve {
			var err error
			if basis, err = s.authorityBasis(ctx, tx, tenant, c0.Classification); err != nil {
				return err
			}
			pol, err := s.policy(ctx, tx, tenant, t.Rail, c0.Classification, s.now())
			if err != nil {
				return err
			}
			policyNow = strings.Join(pol.versions, ";")
		}
		if _, err := tx.Exec(ctx, `UPDATE approvals SET status = $3, checker = $4, decision_reason = $5, limit_applied = $6, decided_at = $7, expires_at = $8, policy_id = $9,
				policy_version = $10, authority_basis = $11
			WHERE tenant_id = $1 AND id = $2`,
			tenant, approvalID, newStatus, checker.UserID, reason, limit, s.now(), expires, rev.PolicyID, policyNow, basis); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE reversals SET approval_status = $3, version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, revID, newApproval); err != nil {
			return err
		}

		actor := cases.Actor{Type: "user", ID: checker.UserID}
		c, err := s.Cases.Get(ctx, tx, tenant, rev.CaseID)
		if err != nil {
			return err
		}
		if !approve {
			if err := s.moveCase(ctx, tx, tenant, c, cases.StRejectedAction, actor, "APPROVAL_REJECTED"); err != nil {
				return err
			}
			if _, err := s.Cases.Transition(ctx, tx, tenant, c.ID, actor, cases.StUnderInvestigation, 0, "APPROVAL_REJECTED"); err != nil {
				return err
			}
		}
		if err := s.event(ctx, tx, tenant, rev.CaseID, actor, "approval_"+strings.ToLower(newStatus), map[string]any{"reversal": rev.Ref, "reason": reason}); err != nil {
			return err
		}
		if err := s.auditEvent(ctx, tx, tenant, actor, "reversal.approval_"+strings.ToLower(newStatus), rev.Ref, rev.Gate.PolicyVersion,
			map[string]any{"case": c.Ref, "approval_id": approvalID.String(), "reason": reason}); err != nil {
			return err
		}
		if approve {
			if err := s.emitKeyed(ctx, tx, tenant, events.ReversalApproved, revID, approvalID.String(), nil); err != nil {
				return err
			}
		}
		out, err = s.Get(ctx, tx, tenant, revID.String())
		return err
	})
	return out, err
}

func ptr[T any](v T) *T { return &v }

// Execute submits an approved or pre-authorised reversal to the provider. Before
// it does, it re-checks the evidence, because time has passed since the gate ran,
// and it consults the kill switch. A re-check that fails ends the reversal safely.
func (s *Service) Execute(ctx context.Context, tenant, reversalID uuid.UUID) (Reversal, error) {
	var out Reversal
	expired, invalidated := false, false
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		rev, err := s.lockReversal(ctx, tx, tenant, reversalID)
		if err != nil {
			return err
		}
		out = rev
		if rev.ApprovalStatus == "EXPIRED" {
			if rev.AcceptedStatus == "PENDING" || rev.AcceptedStatus == "UNKNOWN" {
				return ErrApprovalExpired
			}
			return nil
		}
		if rev.ApprovalStatus != "NOT_REQUIRED" && rev.ApprovalStatus != "APPROVED" {
			return fmt.Errorf("%w: approval is %s", ErrBadState, rev.ApprovalStatus)
		}
		if rev.AcceptedStatus != "PENDING" && rev.AcceptedStatus != "UNKNOWN" {
			return nil // already submitted: nothing to do, and nothing is submitted twice
		}
		sub, err := s.loadSubject(ctx, tx, tenant, rev.CaseID)
		if err != nil {
			return err
		}
		actor := cases.System // the system carries out what people approved

		// An approval is only authority while it is valid. This is checked before the
		// freeze, so a lapse is recorded even while submissions are frozen, and again
		// after an unfreeze, because the check is made here and not when it was given.
		if rev.ApprovalStatus == "APPROVED" {
			if ok, why, err := s.approvalStillValid(ctx, tx, tenant, rev); err != nil {
				return err
			} else if !ok {
				if err := s.expireApproval(ctx, tx, tenant, rev, sub, actor, why); err != nil {
					return err
				}
				expired = true
				out, err = s.Get(ctx, tx, tenant, rev.ID.String())
				return err
			}
		}

		// Last look before money would move. A freeze parks the reversal: it keeps
		// its approval, nothing reaches the provider, and the sweep submits it after
		// an authorised unfreeze, once the gate has been evaluated again. A switch
		// that cannot be read ends the attempt with an error, so it is retried.
		if fe, err := s.frozenFor(ctx, tx, tenant, sub); err != nil {
			return err
		} else if fe != nil {
			return s.park(ctx, tx, tenant, rev, sub, fe, actor, &out)
		}
		gate, err := s.evaluate(ctx, tx, tenant, sub, Requester{}, rev.IdempotencyKey, true)
		if err != nil {
			return err
		}
		if gate.Verdict == Blocked {
			// A freeze that arrived while the gate ran is a pause, not a failure.
			if fe, err := s.frozenFor(ctx, tx, tenant, sub); err != nil {
				return err
			} else if fe != nil {
				return s.park(ctx, tx, tenant, rev, sub, fe, actor, &out)
			}
			f := gate.Failed()
			why := "evidence changed before execution"
			if len(f) > 0 {
				why = fmt.Sprintf("check %d failed before execution: %s", f[0].ID, f[0].Detail)
			}
			return s.failBeforeSubmit(ctx, tx, tenant, rev, sub, actor, why, &out)
		}

		// Policy may have changed since the approval was given. If the reversal still
		// needs a person's approval and the settings that authorisation relied on are
		// no longer the same, the earlier approval does not carry over. A reversal the
		// policy now lets automation submit needs no approval and proceeds.
		if rev.ApprovalStatus == "APPROVED" && gate.Verdict == PassNeedsApproval {
			now, err := s.authorityBasis(ctx, tx, tenant, sub.Case.Classification)
			if err != nil {
				return err
			}
			then, err := s.approvalBasis(ctx, tx, tenant, rev.ID)
			if err != nil {
				return err
			}
			if then == "" || then != now {
				why := "the policy the approval relied on changed (" + orNone(then) + " became " + orNone(now) + ")"
				if then == "" {
					why = "the approval did not record the policy it relied on"
				}
				if err := s.expireApprovalAs(ctx, tx, tenant, rev, sub, actor, why, "approval_invalidated", "reversal.approval_invalidated"); err != nil {
					return err
				}
				invalidated = true
				out, err = s.Get(ctx, tx, tenant, rev.ID.String())
				return err
			}
		}

		// A reversal recorded as automatic carries no approval to fall back on. If
		// policy now needs a person for it, it waits for one.
		if rev.ApprovalStatus == "NOT_REQUIRED" && gate.Verdict == PassNeedsApproval {
			why := gate.ApprovalReason
			if why == "" {
				why = "approval required"
			}
			if err := s.escalateToApproval(ctx, tx, tenant, rev, sub, actor, why); err != nil {
				return err
			}
			invalidated = true
			out, err = s.Get(ctx, tx, tenant, rev.ID.String())
			return err
		}

		if err := s.unpark(ctx, tx, tenant, rev, actor); err != nil {
			return err
		}
		ack, perr := s.Provider.Submit(ctx, tx, tenant, Submission{IdempotencyKey: rev.IdempotencyKey, InstructionRef: sub.Txn.Ref,
			ReversalRef: rev.Ref, AmountMinor: rev.AmountMinor, Currency: rev.Currency, ReasonCode: rev.ReasonCode})
		now := s.now()
		switch {
		case errors.Is(perr, ErrProviderTimeout):
			// No answer is UNKNOWN. The same key is used to find out; nothing is resubmitted blindly.
			if _, err := tx.Exec(ctx, `UPDATE reversals SET accepted_status = 'UNKNOWN', processing_status = 'UNKNOWN', submitted_at = $3, version = version + 1, updated_at = now()
				WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, now); err != nil {
				return err
			}
			if err := s.moveCase(ctx, tx, tenant, sub.Case, cases.StRemediationSubmitted, actor, "SUBMITTED_OUTCOME_UNKNOWN"); err != nil {
				return err
			}
			if err := s.event(ctx, tx, tenant, rev.CaseID, actor, "reversal_outcome_unknown", map[string]any{"reversal": rev.Ref}); err != nil {
				return err
			}
		case perr != nil:
			return perr
		case ack.Accepted == "REJECTED":
			if _, err := tx.Exec(ctx, `UPDATE reversals SET accepted_status = 'REJECTED', processing_status = 'FAILED', financial_status = 'FAILED',
				provider_ref = $3, failure_reason = $4, submitted_at = $5, version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`,
				tenant, rev.ID, ack.ProviderRef, ack.Reason, now); err != nil {
				return err
			}
			if err := s.failedReversal(ctx, tx, tenant, rev, sub, actor, "PROVIDER_REJECTED", ack.Reason); err != nil {
				return err
			}
		default:
			if _, err := tx.Exec(ctx, `UPDATE reversals SET accepted_status = 'ACCEPTED', processing_status = 'IN_PROGRESS', provider_ref = $3, submitted_at = $4,
				version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, ack.ProviderRef, now); err != nil {
				return err
			}
			if err := s.moveCase(ctx, tx, tenant, sub.Case, cases.StRemediationSubmitted, actor, "REVERSAL_SUBMITTED"); err != nil {
				return err
			}
			if err := s.event(ctx, tx, tenant, rev.CaseID, actor, "reversal_submitted", map[string]any{"reversal": rev.Ref, "provider_ref": ack.ProviderRef, "simulated": true}); err != nil {
				return err
			}
			if err := s.auditEvent(ctx, tx, tenant, actor, "reversal.submitted", rev.Ref, rev.Gate.PolicyVersion, map[string]any{"case": sub.Case.Ref, "provider_ref": ack.ProviderRef, "simulated": true}); err != nil {
				return err
			}
			if err := s.emit(ctx, tx, tenant, events.ReversalSubmitted, rev.ID, nil); err != nil {
				return err
			}
		}
		out, err = s.Get(ctx, tx, tenant, rev.ID.String())
		return err
	})
	if err == nil && expired {
		return out, ErrApprovalExpired // the lapse itself was committed above
	}
	if err == nil && invalidated {
		return out, ErrApprovalInvalidated
	}
	return out, err
}

func (s *Service) lockReversal(ctx context.Context, tx pgx.Tx, tenant, id uuid.UUID) (Reversal, error) {
	if _, err := tx.Exec(ctx, `SELECT 1 FROM reversals WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenant, id); err != nil {
		return Reversal{}, err
	}
	return s.Get(ctx, tx, tenant, id.String())
}

// failBeforeSubmit ends a reversal that never reached the provider, frees the
// lock, and returns the case to investigation. No money moved.
func (s *Service) failBeforeSubmit(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, rev Reversal, sub subject, a cases.Actor, why string, out *Reversal) error {
	if _, err := tx.Exec(ctx, `UPDATE reversals SET accepted_status = 'REJECTED', processing_status = 'FAILED', financial_status = 'FAILED', failure_reason = $3,
		version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, why); err != nil {
		return err
	}
	c, err := s.Cases.Get(ctx, tx, tenant, rev.CaseID)
	if err != nil {
		return err
	}
	if c.Status == cases.StAwaitingApproval {
		if _, err := s.Cases.Transition(ctx, tx, tenant, c.ID, a, cases.StRejectedAction, 0, "BLOCKED_BEFORE_EXECUTION"); err != nil {
			return err
		}
		if _, err := s.Cases.Transition(ctx, tx, tenant, c.ID, a, cases.StUnderInvestigation, 0, "BLOCKED_BEFORE_EXECUTION"); err != nil {
			return err
		}
	}
	if err := s.event(ctx, tx, tenant, rev.CaseID, a, "reversal_stopped", map[string]any{"reversal": rev.Ref, "why": why}); err != nil {
		return err
	}
	if _, err := notify.Record(ctx, tx, tenant, rev.CaseID, notify.OpsHeld(sub.Case.Ref, why), sub.Case.Ref+":ops:stopped:"+rev.Ref); err != nil {
		return err
	}
	if err := s.auditEvent(ctx, tx, tenant, a, "reversal.stopped", rev.Ref, rev.Gate.PolicyVersion, map[string]any{"why": why}); err != nil {
		return err
	}
	if err := s.emit(ctx, tx, tenant, events.ReversalFailed, rev.ID, map[string]any{"failure_code": "STOPPED_BEFORE_SUBMISSION"}); err != nil {
		return err
	}
	r, err := s.Get(ctx, tx, tenant, rev.ID.String())
	*out = r
	return err
}

// failedReversal handles a reversal that was refused or could not be verified:
// the case reflects it and a REV001 case is opened for the failure itself.
func (s *Service) failedReversal(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, rev Reversal, sub subject, a cases.Actor, code, why string) error {
	c, err := s.Cases.Get(ctx, tx, tenant, rev.CaseID)
	if err != nil {
		return err
	}
	switch c.Status {
	case cases.StRemediationSubmitted, cases.StOutcomeVerification:
		if _, err := s.Cases.Transition(ctx, tx, tenant, c.ID, a, cases.StRemediationFailed, 0, "REVERSAL_FAILED"); err != nil {
			return err
		}
	case cases.StAwaitingApproval:
		if _, err := s.Cases.Transition(ctx, tx, tenant, c.ID, a, cases.StRejectedAction, 0, "REVERSAL_REFUSED"); err != nil {
			return err
		}
		if _, err := s.Cases.Transition(ctx, tx, tenant, c.ID, a, cases.StUnderInvestigation, 0, "REVERSAL_REFUSED"); err != nil {
			return err
		}
	}
	amount := rev.AmountMinor
	tid := rev.TxnID
	if _, _, err := s.Cases.Open(ctx, tx, tenant, cases.OpenInput{Classification: cases.CodeReversalErr, Source: cases.SourceManual, TxnID: &tid, ExposureMinor: &amount,
		Currency: rev.Currency, Rail: sub.Txn.Rail, RootCause: "other", RuleID: "REV-FAIL-1", RuleVersion: "1.0",
		Explanation: "Reversal " + rev.Ref + " failed: " + why, Actor: a,
		Evidence: []cases.Evidence{{Kind: "reversal", Ref: rev.Ref, IsFact: true, Detail: map[string]string{"why": why}}}}); err != nil {
		return err
	}
	if _, err := notify.Record(ctx, tx, tenant, rev.CaseID, notify.OpsReversalFailed(sub.Case.Ref, rev.Ref, why), sub.Case.Ref+":ops:failed:"+rev.Ref); err != nil {
		return err
	}
	if err := s.event(ctx, tx, tenant, rev.CaseID, a, "reversal_failed", map[string]any{"reversal": rev.Ref, "why": why}); err != nil {
		return err
	}
	if err := s.auditEvent(ctx, tx, tenant, a, "reversal.failed", rev.Ref, rev.Gate.PolicyVersion, map[string]any{"case": sub.Case.Ref, "why": why}); err != nil {
		return err
	}
	return s.emit(ctx, tx, tenant, events.ReversalFailed, rev.ID, map[string]any{"failure_code": code})
}

// Advance moves a submitted reversal forward: it asks the provider where the
// request stands, and when the provider says it is done, verifies the result
// against the ledger. Call it again until the reversal is verified or failed. It
// is safe to call at any time and any number of times.
func (s *Service) Advance(ctx context.Context, tenant, reversalID uuid.UUID) (Reversal, error) {
	var out Reversal
	err := s.Pool.WithTenant(ctx, tenant, func(tx pgx.Tx) error {
		rev, err := s.lockReversal(ctx, tx, tenant, reversalID)
		if err != nil {
			return err
		}
		out = rev
		if rev.FinancialStatus != "UNVERIFIED" || rev.AcceptedStatus == "REJECTED" {
			return nil
		}
		sub, err := s.loadSubject(ctx, tx, tenant, rev.CaseID)
		if err != nil {
			return err
		}
		actor := cases.System

		if rev.AcceptedStatus == "PENDING" {
			return nil // not submitted yet
		}
		if rev.ProcessingStatus != "COMPLETED" {
			ack, found, err := s.Provider.Status(ctx, tx, tenant, rev.IdempotencyKey)
			if err != nil {
				return err
			}
			switch {
			case !found && rev.AcceptedStatus == "UNKNOWN":
				// The provider never received it. Using the same key makes a resubmit safe.
				if _, err := tx.Exec(ctx, `UPDATE reversals SET accepted_status = 'PENDING', processing_status = 'NOT_STARTED', version = version + 1 WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID); err != nil {
					return err
				}
				out, err = s.Get(ctx, tx, tenant, rev.ID.String())
				return err
			case !found:
				return nil
			case ack.Accepted == "REJECTED":
				if _, err := tx.Exec(ctx, `UPDATE reversals SET accepted_status = 'REJECTED', processing_status = 'FAILED', financial_status = 'FAILED', failure_reason = $3, provider_ref = $4,
					version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, ack.Reason, ack.ProviderRef); err != nil {
					return err
				}
				if err := s.failedReversal(ctx, tx, tenant, rev, sub, actor, "PROVIDER_REJECTED", ack.Reason); err != nil {
					return err
				}
				out, err = s.Get(ctx, tx, tenant, rev.ID.String())
				return err
			case ack.Processing != "COMPLETED":
				if rev.AcceptedStatus != "ACCEPTED" || rev.ProcessingStatus != "IN_PROGRESS" {
					if _, err := tx.Exec(ctx, `UPDATE reversals SET accepted_status = 'ACCEPTED', processing_status = 'IN_PROGRESS', provider_ref = $3,
						version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, ack.ProviderRef); err != nil {
						return err
					}
					out, err = s.Get(ctx, tx, tenant, rev.ID.String())
				}
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE reversals SET accepted_status = 'ACCEPTED', processing_status = 'COMPLETED', provider_ref = $3, posted_at = $4,
				version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, ack.ProviderRef, s.now()); err != nil {
				return err
			}
			if rev.AcceptedStatus == "UNKNOWN" {
				if err := s.moveCase(ctx, tx, tenant, sub.Case, cases.StRemediationSubmitted, actor, "OUTCOME_CONFIRMED_BY_STATUS_QUERY"); err != nil {
					return err
				}
			}
			rev, err = s.Get(ctx, tx, tenant, rev.ID.String())
			if err != nil {
				return err
			}
		}

		// The provider says it is done. That is a claim, not a fact: the ledger decides.
		out, err = s.verify(ctx, tx, tenant, rev, sub, actor)
		return err
	})
	return out, err
}

// verify checks the provider's claim against the core ledger. Only a ledger
// posting that matches the reversal in amount and currency verifies it.
func (s *Service) verify(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, rev Reversal, sub subject, a cases.Actor) (Reversal, error) {
	entry, found, err := s.Ledger.ReversalPosting(ctx, tx, tenant, sub.Txn.Ref, rev.Ref)
	if err != nil {
		return rev, err
	}
	if !found || entry.AmountMinor != rev.AmountMinor || entry.Currency != rev.Currency {
		why := "the provider reported completion but the ledger shows no matching reversal posting"
		if found {
			why = fmt.Sprintf("the ledger posting (%d %s) does not match the reversal (%d %s)", entry.AmountMinor, entry.Currency, rev.AmountMinor, rev.Currency)
		}
		if _, err := tx.Exec(ctx, `UPDATE reversals SET financial_status = 'FAILED', failure_reason = $3, version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, why); err != nil {
			return rev, err
		}
		if err := s.failedReversal(ctx, tx, tenant, rev, sub, a, "LEDGER_MISMATCH", why); err != nil {
			return rev, err
		}
		return s.Get(ctx, tx, tenant, rev.ID.String())
	}

	now := s.now()
	if _, err := tx.Exec(ctx, `UPDATE reversals SET financial_status = 'VERIFIED', verified_at = $3, version = version + 1, updated_at = now() WHERE tenant_id = $1 AND id = $2`, tenant, rev.ID, now); err != nil {
		return rev, err
	}
	// The ledger posting becomes an event, so the transaction's own state shows REVERSED.
	var core txn.Core
	var amount int64
	if err := tx.QueryRow(ctx, `SELECT instruction_ref, channel, rail, amount_minor, currency, debit_account_token, beneficiary_bank, beneficiary_token, session_id
		FROM transactions WHERE tenant_id = $1 AND id = $2`, tenant, rev.TxnID).Scan(&core.InstructionRef, &core.Channel, &core.Rail, &amount, &core.Currency,
		&core.DebitAccountToken, &core.BeneficiaryBank, &core.BeneficiaryToken, &core.SessionID); err != nil {
		return rev, err
	}
	core.AmountMinor = txn.Amount(amount)
	if _, err := s.Txn.Ingest(ctx, tx, tenant, txn.Envelope{SourceSystem: "SIM_CORE_LEDGER", SourceEventRef: "LEDGER-" + rev.Ref,
		EventType: txn.EvReversalPosted, SourceTS: entry.PostedAt, Txn: core}); err != nil {
		return rev, err
	}

	c, err := s.Cases.Get(ctx, tx, tenant, rev.CaseID)
	if err != nil {
		return rev, err
	}
	for _, step := range []string{cases.StOutcomeVerification, cases.StResolved} {
		if c.Status == step {
			continue
		}
		if _, err := s.Cases.Transition(ctx, tx, tenant, c.ID, a, step, 0, "REVERSAL_VERIFIED_AGAINST_LEDGER"); err != nil {
			return rev, err
		}
	}
	if err := s.Cases.AddEvidence(ctx, tx, tenant, rev.CaseID, a, cases.Evidence{Kind: "ledger_entry", Ref: rev.Ref, SourceSystem: "SIM_CORE_LEDGER", IsFact: true,
		Detail: map[string]any{"amount_minor": entry.AmountMinor, "currency": entry.Currency, "posted_at": entry.PostedAt.Format(time.RFC3339)}}); err != nil {
		return rev, err
	}
	// Only now may the customer be told the money was returned.
	if _, err := notify.Record(ctx, tx, tenant, rev.CaseID, notify.CustomerVerified(rev.AmountMinor, rev.Currency, sub.Txn.Ref), sub.Case.Ref+":customer:verified"); err != nil {
		return rev, err
	}
	if _, err := notify.Record(ctx, tx, tenant, rev.CaseID, notify.OpsReversalVerified(sub.Case.Ref, rev.Ref), sub.Case.Ref+":ops:verified:"+rev.Ref); err != nil {
		return rev, err
	}
	if err := s.event(ctx, tx, tenant, rev.CaseID, a, "reversal_verified", map[string]any{"reversal": rev.Ref, "ledger_amount_minor": entry.AmountMinor}); err != nil {
		return rev, err
	}
	if err := s.auditEvent(ctx, tx, tenant, a, "reversal.verified", rev.Ref, rev.Gate.PolicyVersion, map[string]any{"case": sub.Case.Ref, "simulated": true}); err != nil {
		return rev, err
	}
	if err := s.emit(ctx, tx, tenant, events.ReversalVerified, rev.ID, nil); err != nil {
		return rev, err
	}
	return s.Get(ctx, tx, tenant, rev.ID.String())
}
