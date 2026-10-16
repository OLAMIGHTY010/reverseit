# Kill switch enforcement matrix

Source of the rules: `internal/killswitch` (package comment and `Check`), B1 report, v2 prompt pack B1 item 8 ("every consumer checks it before acting"), B5 item 9 and M32 ("freeze new submissions; drain or park in-flight; reconcile after unfreeze"), B11 item 1 ("each job ... can be paused by the kill switch"), and S9 (four scopes). No new scope is added and no policy is reinterpreted.

## What a freeze means

A freeze stops **automation that starts something new**. It never stops the system from **recording what is true**, and it never stops the check on something already submitted. The four scopes are matched against the action by `Check(Action{Rail, ExceptionType, PolicyID})`:

| Scope | Matches an action when |
|---|---|
| `global` | always |
| `rail` | the action's rail equals the target |
| `exception_type` | the action's exception code (FAIL001, DUP001, UNK001, REV001, ...) equals the target |
| `policy` | the action's policy or rule id equals the target |

A check that cannot read the switch counts as frozen.

## Matrix

"Frozen" means a covering freeze (any scope that matches the action) changes the behaviour shown. Scopes that do not match the action change nothing.

| Operation | Under a covering freeze | Why |
|---|---|---|
| Event ingestion, projection, quarantine (`POST /v1/events`) | **Never blocked.** Stored, projected, audited. The `transaction.updated` outbox event is still written | Evidence collection. Blocking it would lose facts |
| Audit appends, case history, notes, evidence added by people | **Never blocked** | Audit continuity |
| Detector: opening a case and its customer "detected" message | **Deferred.** Nothing is opened. A durable deferral row is written and the stream message is acknowledged. After unfreeze the transaction is re-evaluated and the case opens once, dated from the original detection | Automation that starts something new, scoped by the exception code and rail |
| Detector: automatic closure of an unknown case after late success | **Deferred** the same way | An automated decision about a case |
| Detector sweep for stuck transactions | **Job paused** under a global freeze (state recorded). Under a narrower freeze each finding is deferred like any other detection | Same as detection |
| Customer-reported cases (`POST /v1/complaints`) | **Not blocked** | A person is entering a report |
| Reversal request (human or automatic) | **Blocked by the gate** (check 7), unchanged. Returns 409 `GATE_FAILED` | Existing, tested behaviour |
| Reversal submission to the provider for an approved or authorised reversal | **Parked.** The reversal keeps its approval, stays unsubmitted, `parked_at` is set and a case event is written. Nothing reaches the provider. After unfreeze the sweep submits it, after the gate is evaluated again | M32: freeze new submissions, park in-flight |
| Reversal sweep (automatic reversals) | **Job paused** under a global freeze. Under a narrower freeze each case the freeze covers is skipped before the gate runs, so no blocked-gate noise is written | B11 item 1 |
| Approval decisions by a person | **Not blocked** (a decision is not a submission). The submission is then parked | Segregation of duties stays in force |
| Provider status poll, ledger verification, case resolution and customer "verified" message for an **already submitted** reversal | **Never blocked** | Reconciling previously submitted actions is safe and necessary |
| Operations alerts (awaiting approval, held, failed, verified) | **Never blocked** | Critical safety notifications |
| Customer "detected" message | Follows the detection it belongs to, so it is deferred with it and sent exactly once afterwards (dedupe key unchanged) | |
| Kill switch freeze, unfreeze request, decision | **Never blocked** | The control itself |

## Deferred and parked work

- **Detection deferrals** live in `deferred_work` (PostgreSQL). One open row per transaction. Rows are claimed with `FOR UPDATE SKIP LOCKED` so several workers never resume the same row twice, and re-evaluation is idempotent anyway.
- **Parked reversals** are ordinary reversal rows with `parked_at` set.
- **Job state** lives in `automation_jobs` (running or paused, which scopes are frozen, last run, counts). `GET /v1/killswitch` reports deferred counts, parked reversals and job states as additional fields.
- Nothing is discarded. Every held item can be listed, counted and resumed.

## Decision recorded

The task asks for enforcement "at transaction ingestion" and also says a freeze must not disable evidence collection. These conflict if read literally. I resolved it by the second rule: ingestion keeps storing and projecting events during any freeze, and the freeze applies to the automation that ingestion triggers (detection and what follows). If you want ingestion itself to be refusable, that is a different and riskier policy and needs a separate decision.

Parking an approved reversal (rather than ending it, as the earlier test `TestAFreezeAfterApprovalStopsTheReversalBeforeItReachesTheProvider` did) follows M32 and the B5 plan. The gate is evaluated again at submission, so stale evidence still stops it. Approvals do not yet expire; that is a separate B5 gap.

## Limits found while building this

- **Policy scope and reversals.** A reversal submission is governed by the policy `reversal.<exception code>.<rail>` (for example `reversal.FAIL001.NIP`), and a `policy` freeze naming it stops requests, automatic sweeps and parked submissions for that code on that rail. If the identity cannot be determined, every policy freeze covers the action. A detector rule id such as `DET-FAIL-1` is a different policy and does not stop reversals.
- **Approvals expire.** A parked reversal keeps its approval only for the configured validity period. A lapse (or a change to the policy the approval relied on) is recorded even while frozen, the reversal becomes EXPIRED, and it needs a fresh approval. See `approval-expiry.md`.
- **Resumed cases are dated from the original detection.** The SLA clock therefore counts the time spent frozen. That is deliberate (the customer's clock does not pause because of our incident) and the case says so, but Compliance should confirm it.
- **Held detection has no age limit or alert.** Open deferrals are counted in `GET /v1/killswitch`, but nothing raises an alert if they stay open for a long time. That belongs to B7.
- **Deferral covers detection only.** Other future automation (notification delivery with real providers, reconciliation imports) will need the same check at its own action when it exists.
