# Approval expiry and policy identity

How a reversal approval stops being valid, and what a reversal records about the policy that governed it.

## Policy identity

Every reversal decision carries a stable `policy_id`, `reversal.<exception code>.<rail>` (for example `reversal.FAIL001.NIP`). It is set when the decision is made, named in every audit record about the reversal, carried by the reversal domain events and shown in the API.

`policy_version` is recorded separately: the versions of the configuration in force at the original decision (for example `status_query.freshness_seconds@3;approval.auto_limit_minor@1`). It is never rewritten, even when the configuration changes later.

A `policy` kill switch freeze targets the policy id. If a reversal action ever lacks a policy identity, every policy freeze covers it.

## Approval validity

- `approval.validity_seconds` (60 seconds to 30 days) is an ordinary configuration key, versioned and maker-checker approved. With no approved value nothing can be approved (`409 APPROVAL_POLICY_MISSING`); rejecting is still possible. There is no built-in default.
- When an approval is given it records `expires_at`, the policy id, the full policy version, and the **authority basis**: the versions of the settings that decide who may authorise (approval limits, validity period, automatic eligibility for the exception type, the execution flag).
- The demonstration installs a labelled synthetic value (3600 seconds) through the same process. It is not bank policy.

## What happens at submission

Submission checks, in order, before anything is sent:

1. The newest approval is still valid. No expiry recorded, or the time has passed, means not valid. This is checked before the kill switch, so a lapse is recorded even while submissions are frozen, and again after an unfreeze.
2. The kill switch (a covering freeze parks the reversal).
3. The eight checks again.
4. If the reversal still needs a person's approval and the authority basis differs from the one recorded, the approval is invalidated. Settings that only describe the evidence (freshness, waiting time) are not part of the basis; the gate checks them afresh. A reversal policy now lets automation submit needs no approval and proceeds.

A lapse or invalidation sets the reversal's approval status to `EXPIRED`, writes a case event, an audit record (`reversal.approval_expired` or `reversal.approval_invalidated`) and an operations alert, once however many times it is tried. Nothing is submitted. The reversal keeps its remediation lock. The API answers `409 APPROVAL_EXPIRED` or `409 APPROVAL_INVALIDATED`. The sweep records lapses on its own (`ExpireDue`), frozen or not.

## Getting a fresh approval

`POST /v1/reversals/{id}/reapproval` with a reason (grant `request_reversal`). It runs the checks again with the caller as maker, and creates a new pending approval for the same reversal. A different person with the authority and an MFA-backed token must approve it; the caller cannot. Repeating the request while one is waiting returns the same approval. If the checks no longer pass the answer is `409 GATE_FAILED` with the failed checks and nothing changes. A reversal may therefore have several approvals over time; the newest is the one that counts. `reversal.approved` is announced once per approval.

## Not done

- A lapse is not yet escalated or reported if nobody renews it (B7).
- The authority basis compares configuration versions, not meaning: a new version with an identical value still invalidates, which errs on the safe side.
- There is no limit on how many times an approval can be renewed.

## Policy tightened after an automatic reversal was recorded

A reversal that policy let automation submit has no approval to fall back on. If, before it is submitted, the gate says a person must now approve it (for example automatic eligibility for the exception type was switched off, or the amount limit was lowered), the submission does not go out. The reversal becomes `PENDING` with a new approval whose maker is the system, the case moves to awaiting approval, and a case event (`approval_now_required`), an operations alert and an audit record (`reversal.escalated_to_approval`) are written once. Any authorised checker who is not otherwise conflicted can then approve it in the usual way. The caller sees `409 APPROVAL_INVALIDATED`; no new error code was added.

The other direction does not remove a human step. A reversal that is `PENDING` stays pending when policy is loosened: the sweep and the automatic path ignore reversals awaiting approval, and `Execute` refuses them. Only a reversal that a person already approved, whose approval is still valid, continues when policy now allows automation, and the gate is evaluated again at that moment.
