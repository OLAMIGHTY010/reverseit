# Declarative detection rules (B3)

The built-in detectors (failure, unknown, duplicate, stuck, ATM, fee, vendor, mismatch, incomplete) stay in code. An institution can add rules as data. A rule can only open a case. It cannot move money.

## A rule

```json
{"rule_id":"DET-ATM-2","version":1,"code":"ATM001","risk":"HIGH","root_cause":"atm_hardware_fault",
 "explanation":"Cash was not dispensed after the debit posted.",
 "all":[{"field":"lifecycle_state","op":"eq","value":"CONFIRMED_FAILURE"},
        {"field":"channel","op":"in","value":["ATM"]}],
 "requires_fresh":["SWITCH_A"]}
```

All conditions must hold. Rules can read `lifecycle_state`, `credit_status`, `debit_status`, `rail`, `channel`, `currency`, `amount_minor`, `event_count`, `seconds_since_last_event` and `seconds_since_first_debit`, with `eq ne in not_in gt gte lt lte`. Nothing else is readable.

## What validation refuses

- No conditions (a rule that matches everything), more than 12, repeated field and operator pairs, unknown fields or operators, a value outside a fixed set, an operator on the wrong kind of field.
- A code that detection rules may not open. Allowed: FAIL001, ATM001, FEE001, VND001, MIS001, INC001, UNK001. Duplicates compare transactions with each other and stay in code.
- **A failure-family code (all but UNK001) that is not guarded by `lifecycle_state eq CONFIRMED_FAILURE`.** An unknown or unconfirmed outcome is never called a failure.
- A UNK001 rule about a transaction whose outcome is already confirmed.
- Unknown fields in the document, and an explanation that is not one line of 10 to 300 characters.

## Lifecycle

`draft` then `shadow` then `pending_activation` then `active`, and later `retired`. A stored version is immutable (a database trigger refuses edits, even from the table owner); a change is a new version, numbered one more than the latest.

1. **Create** (grant `change_rules`, scope `draft` or `propose`): validated and saved as a draft. A draft is not evaluated.
2. **Shadow**: the version is evaluated against transactions as they arrive and what it concluded is recorded in `rule_evaluations`. It opens no case. `POST .../backtest` evaluates it against recent existing transactions the same way.
3. **Request activation** (scope `propose`): allowed only from shadow, and only once the version has been evaluated against at least one transaction, so an approver has results to look at (`GET /v1/rules/{id}` shows matched, no_match and held counts).
4. **Decision** (scope `approve`, which only RISK_COMPLIANCE holds, plus MFA): the decider may be neither the author nor the requester, enforced in code and by a database constraint. Approval makes the version live and retires the version it replaces in the same transaction, so there are never two live and never a gap. Rejection needs a reason.
5. **Rollback** (scope `propose`): an earlier version that was once live is put forward again. It is an activation request, so the current version stays live until a different person approves.
6. **Disable** (scope `propose`, with a reason): takes the live version out of use at once. It needs no second person because it only reduces what the rule does; it is audited.

Every step is audited in the same transaction with the rule id and version.

## Live evaluation

The detector runs live rules after the built-in ones for each event, and a sweep (`SweepRules`, run with the existing stuck sweep) evaluates rules about silence for transactions still waiting for an answer. A match opens a case under the rule's own id and version, so the case says which rule raised it. If a built-in rule already opened a case of the same code for the transaction, none is opened twice. The kill switch applies to rule ids as policy ids, so a freeze on `DET-ATM-2` holds that rule's work.

## Source degradation

`requires_fresh` names sources that must be fresh for the rule to run. While any is stale, down, unconfigured or has sent nothing, the rule is **held**: no case is opened and the reason is recorded (`rule_evaluations.held_reason`). A missing event from a silent source is not evidence. The effect is that a rule using `requires_fresh` does nothing until someone has set an expectation for that source (`PUT /v1/sources/{system}/expectation`).

## Limits

- The built-in detectors are not yet expressed as rules, so they do not yet use `requires_fresh`. The simulator shows the consequence during a source outage (docs/backend/simulator.md).
- Rules do not see other transactions, so they cannot express duplicates or repeats.
- Changing a rule does not retroactively close or reopen cases it raised.
