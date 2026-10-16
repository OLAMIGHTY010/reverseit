# Backend coverage matrix against the v2 Integrated Scope Prompt Pack

Source of requirements: the pack's Section 7 (contracts), Section 12.2 (shared preamble), Section 12.3 (prompts B0 to B11 with their SCOPE v2 ADDITIONS) and Section 15 (X1 to X3). Architecture is unchanged: Go with Chi, PostgreSQL 16, Redis Streams with a transactional outbox, Ed25519 tokens, row-level tenant isolation. The pack's default Python stack is not adopted; the contracts are met in Go.

Classes: **COMPLETE** (implemented and covered by a test named here), **PARTIAL**, **MISSING**, **BLOCKED** (needs an external decision or party). "PR #19" is a teammate's open branch (feat/backend-gaps-implementation): it is not merged, its CI fails on formatting, and I have not verified its contents beyond reading its file list, so everything it claims is listed as **OPEN IN PR #19**, not counted as done.

Everything is synthetic and every money movement is simulated. Targets in the pack (60 s detection, 95 percent automation and so on) are unvalidated goals; this matrix makes no claim that any is met.

Merged since the demo baseline (`demo-baseline-2026-10-15`): #15 matrix, #16 domain events, #17 kill-switch coverage, #18 approval expiry and policy identity, #20 automatic-reversal policy fix and cleanup, #21 file ingestion and quarantine replay, #22 correlation, source health and the simulator, #23 message consumer, pipeline report and multi-channel simulator, #24 declarative rules and source-degradation holds, #25 review items and the shared event contract, #26 and #27 the ReverseIt backend (identity, parser, reports, deadline engine, submission, complaint generator). #28 (customer cases, escalation, feedback, labelled examples) is open.

## Section 7 contracts

| Item | Class | Evidence |
|---|---|---|
| 7.1 entities and keys | PARTIAL | Institution, user and role, transaction, event, external ref (`transaction_refs`), account token, exception and case, evidence, complaint (reports), approval, reversal (three tracks), notification, audit event: present. Missing as entities: Incident, ReconRecord (OPEN IN PR #19), AutomationPolicy as a table (policies are versioned configuration plus rules) |
| 7.2 event envelope | COMPLETE | `internal/txn/event.go`; `TestTheContractExampleEventIsAcceptedAsWritten` posts the pack's example form: refs as a list, bare integer amount, institution_id checked against the credentials, seq, raw_payload_hash, X-Schema-Version 1.x. Responses 202, 200, 409 (with a data-quality audit record), 422 (quarantined), 401 |
| 7.3 core APIs | PARTIAL | Present: events, transactions, exceptions, cases, PATCH with If-Match, complaints, reversals, approvals, audit, killswitch. Missing: `POST /cases` (manual create), transactions search by free-text `q` and date/amount filters beyond reference, state and rail, audit `?subject=` |
| 7.4 reversal request | COMPLETE | `server/reversals_test.go`; 409 GATE_FAILED with failed checks |
| 7.5 source authority matrix | PARTIAL | Source expectations and freshness exist (`internal/sources`); the authority matrix per fact (max staleness per fact) is configuration the bank must supply. BLOCKED on discovery |
| 7.6 reliability patterns | PARTIAL | Idempotent at-least-once ingestion, outbox, DLQ with replay (`stream`, `txn.Replay`), per-transaction lock, single-writer reversal, idempotency key before submission, per-source freshness. Missing: bounded queue depth limits and load shedding (reported only), circuit breaker and retry with jitter (OPEN IN PR #19), reconciliation-after-outage job |

## B0 and B1 foundations

| Requirement | Class | Evidence |
|---|---|---|
| B0 ADR, skeleton, six interface OpenAPI files | COMPLETE | `docs/adr`, `packages/api-contracts/interfaces`, `internal/contract` tests |
| B1 tenancy, RLS, token resolution | COMPLETE | `platform/db`, RLS on every table, isolation tests in each package |
| B1 auth, MFA claim, service auth, customer audience | COMPLETE for the development provider | `identity`, `servauth`, `customer`; production IdP BLOCKED |
| B1 RBAC as data, SoD, conflict check | COMPLETE | `authz`, migration 0005 |
| B1 tokenisation, masking, reason-gated unmask | COMPLETE | `datashield`, `server/unmask.go` |
| B1 audit chain, verify, tamper test | COMPLETE | `audit`; external anchoring is a B9 design item |
| B1 config service with maker-checker and VERIFY tags | COMPLETE | `configsvc` |
| B1 regulatory clock registry seeded with the Section 5.3 values and statuses | COMPLETE | `configsvc/clockseed.go`; proposals only, none approved |
| B1 business calendar in WAT, working-day arithmetic | COMPLETE | `bizcal`; `deadline` tests across a weekend and a holiday |
| B1 kill switch, four scopes, dual control to re-enable, every consumer checks | COMPLETE | `killswitch`, `docs/backend/kill-switch-enforcement.md` |
| B1 delegation and on-call model | PARTIAL | `routing` package exists; wired to cases only in PR #19 |

## B2 ingestion, correlation, simulator

| # | Requirement | Class | Evidence |
|---|---|---|---|
| 1 | REST, file-drop (CSV, XML, fixed width) and message-consumer adapters, versioned mappings | COMPLETE | `internal/feed` (`POST /v1/feeds/{mapping}/files`), `internal/inbound` (stream `events.inbound`, wired into the worker); settlement and NIBSS response files are B6 |
| 2 | Idempotency 200 / 409 with data-quality alert / 422 to DLQ with replay tool | COMPLETE | `TestTheSameEventDeliveredConcurrentlyIsStoredOnceAndReportedAsADuplicate` (a real race fixed here), `txn/quarantine.go`, `/v1/quarantine` replay and discard |
| 3 | Ordering, late events, deterministic projection, clock skew per source | COMPLETE | `txn/project.go`; `sources` p50 and p95 skew; `simrun` reordering and redelivery tests |
| 4 | Correlation by external reference with confidence, unmatched queue, audited manual link, never account+amount+time alone | COMPLETE | `internal/correlate`, `/v1/observations`, `/v1/evidence` |
| 5 | Lifecycle projection | PARTIAL | States exist (INITIATED, DEBITED, PENDING, CONFIRMED_*, UNKNOWN, CONFLICTING, REVERSED). SENT and the REVERSAL_* sub-states are not separate |
| 6 | Source freshness fresh/stale/down, attached to evidence | PARTIAL | `GET /v1/sources`; degraded sources shown on review items and used to hold findings. Freshness is not yet attached to each evidence item in a case |
| 7 | Backpressure, lag, load shedding that never drops | PARTIAL | `GET /v1/pipeline` reports outbox backlog and stream lag, pending and dead letters. No bound on queue depth, no spill to disk |
| 8 | Simulator: 18 named scenarios, seeds, ground-truth file, outage, skew, burst | PARTIAL | `internal/sim` (18 scenarios, 50-transaction mix with one unknown, jitter, redelivery, outage, skew), `cmd/sim`, `internal/simrun`. Not yet: reversal timeout, reversal rejected, late success after reversal, channel incident spike, malformed events, burst and time-acceleration mode over HTTP |
| 9 | Property tests; shuffled replay gives identical state | PARTIAL | Reordering with 30 percent redelivery leaves every lifecycle correct (`TestRedeliveryAndReordering...`). No property-based generator |
| v2 | Detection latency from source event time to case creation, p50 and p95 | PARTIAL | `simrun` reports p50, p95 and max in simulated time, measured from first debit; not from the deciding event, and not a performance figure |

## B3 rules engine and detectors

| # | Requirement | Class | Evidence |
|---|---|---|---|
| 1 | Declarative versioned rules, shadow mode, maker-checker, rollback | COMPLETE for conditions on one transaction | `internal/rules`, `/v1/rules`, `docs/backend/detection-rules.md`. Missing from the pack's model: scope (tenant, rail, channel) as a field, thresholds referencing config keys, severity mapping, recommended lane, explanation template with fields |
| 2 | Detectors EX-DUP, DNC, INC, STK, RVF, RVP, FEE, VND, ATM, LTE, MIS, RTY | PARTIAL | Built in: DUP001, FAIL001 (DNC), UNK001, stuck sweep, ATM001, FEE001, VND001, MIS001, INC001 (the last five are rail-keyed conventions from a teammate's PR). Missing: REV001 detectors RVF and RVP beyond reversal failure, EX-LTE (late success after reversal), EX-RTY (retry), EX-DLY; legitimate-repeat allow-list beyond reference/session/window |
| 3 | Exception record contents | PARTIAL | Missing confidence or rule outcome field, recommended next step, lane |
| 4 | Degraded-source marker and suppress policy | COMPLETE for UNK001 and DUP001 | `detect/degraded.go`, `docs/backend/source-degradation.md`; simulator outage: unexpected UNK001 10 to 0 |
| 5 | Re-query ladder with self-healing closure | PARTIAL | Late success closes an unknown case. No scheduler that asks the status source at configured intervals |
| 6 | Exposure: gross flagged, submitted, reversed, net resolved | MISSING | `exposure_minor` per case only |
| 7 | Configurable explainable priority score | MISSING | Fixed `priorityFor` |
| 8 | Golden-case suite, precision and recall, adversarial cases | COMPLETE for the labelled synthetic set | `internal/simrun`; `docs/backend/simulator.md`. Precision and recall 1.00 on 23 labelled transactions; the set is small and written by the same team, so this is not an accuracy claim |
| v2 | Risk grade, unknown never failed, codes | COMPLETE | detect tests; rule validation forbids it |
| v2 | ML score hook (A5) | BLOCKED | AI developer's interface |

## B4 cases, state machine, SLA, complaints

| # | Requirement | Class | Evidence |
|---|---|---|---|
| 1 | State machine, validation, reason codes, side states | PARTIAL | One machine for all types (COMPLETE); per-type workflow definitions MISSING |
| 2 | Ownership, assignment, queues, bulk | MISSING / OPEN IN PR #19 | |
| 3 | SLA and customer resolution clock, pause, breach events, escalation | PARTIAL | Deadline from approved clock; internal target kept apart (`cases.internal_target_at`, `deadline` engine). Pause, breach and approaching events: MISSING (SLA breach escalation OPEN IN PR #19) |
| 4 | Notes, attachments (scanned), evidence links, linked cases | PARTIAL | Notes and evidence yes. Attachments with scanning, linked cases, additional-check requests: MISSING |
| 5 | Complaint intake and matcher | COMPLETE for the customer path | `creport` (exact reference, ranked candidates, repeat contact counter, verified-successful and already-reversed packs, unmatched claim). The agent path `POST /v1/complaints` still matches by exact reference only |
| 6 | Search and saved views | PARTIAL | |
| 7 | Domain events through the outbox | COMPLETE | `docs/backend/domain-events.md` |
| v2 | Case fields incl. internal target and regulatory deadline apart | COMPLETE | |

## B5 controlled reversal

| # | Requirement | Class | Evidence |
|---|---|---|---|
| 1 | Eight-check gate, unknown is HOLD, stale evidence cannot pass | COMPLETE | `reversal/gate.go` tests |
| 2 | Remediation lock | COMPLETE for in-platform | Remediation started outside the platform BLOCKED on data |
| 3 | Policy decision AUTO / NEEDS_APPROVAL / MANUAL_ONLY / BLOCKED | PARTIAL | No MANUAL_ONLY, time-of-day or rollout percentage |
| 4 | Approvals with expiry and revalidation; policy change invalidates | COMPLETE | `docs/backend/approval-expiry.md`; an automatic reversal also stops when policy tightens (PR #20). Higher-limit chains MISSING |
| 4 | Unknown-status REVIEW items; dual-approver override with fresh evidence | PARTIAL | Review items listed, never releasable (`GET /v1/review-items`). The dual-approver override is OPEN IN PR #19 and needs a product decision |
| 5 | Execution: key before call, timeout is UNKNOWN, circuit breaker and bounded retries | PARTIAL | Key and unknown handling COMPLETE; breaker and retries OPEN IN PR #19 |
| 6 | Verification from the ledger, partial, fee, restricted account, recovery sub-case, watch window | PARTIAL | Ledger verification COMPLETE; others MISSING / OPEN IN PR #19 (recovery) |
| 7 | Manual reversal recording | MISSING | |
| 8 | Customer notices only after verification, deduplicated | COMPLETE | notify tests |
| 9 | Kill switch freeze, park, reconcile | COMPLETE | |
| 10 | Chaos tests | PARTIAL | |

## B6 reconciliation

All items MISSING on develop. PR #19 contains settlement file ingestion and break detection (not merged, not verified by me). The reversal reconciliation view exists (`GET /v1/reconciliation/reversals`).

## B7 incidents, alerts, notifications

| Requirement | Class | Evidence |
|---|---|---|
| Incident grouping and spike detection | COMPLETE (PR #30) | `internal/incidents`; `docs/backend/alerts-and-incidents.md`; needs approved `incident.*` keys |
| Alert service: routing, dedupe, suppression, acknowledge, escalate | COMPLETE (PR #30) | `internal/alerts`, `/v1/alerts`, worker `opsAll`; needs approved `alert.*` keys |
| Customer notifications: templates, dedupe, two-stage, never "returned" before VERIFIED | COMPLETE | |
| Delivery tracking, retry, fallback channel, opt-out, wrong-number guard, contact-change guard | MISSING | Provider adapters OPEN IN PR #19 (telecom) |
| Customer report received acknowledgement | COMPLETE | `notify.CustomerReportReceived` |
| Test sink adapter | PARTIAL | Notifications are stored as recorded and simulated |

## B8 reporting, integration health, observability

| Requirement | Class | Evidence |
|---|---|---|
| Metrics catalogue and the listed metrics | COMPLETE (PR #31) | `internal/metrics`, `GET /v1/metrics`, `/v1/metrics/catalogue`, plus summary, pipeline and sources |
| Controlled exports | MISSING | |
| Integration health | PARTIAL | Source freshness and pipeline backlog; per-adapter success rate and circuit state MISSING |
| OpenTelemetry, metrics endpoint | OPEN IN PR #19 | |
| Data lineage | PARTIAL | Rule id and version on every case; evaluations recorded per rule |
| Dashboards: operations, executive, customer timeline | PARTIAL | Customer timeline COMPLETE; others via summary only |

## B9 security and resilience

| Requirement | Class | Evidence |
|---|---|---|
| STRIDE threat model | MISSING | `docs/security/threat-model.md` is a stub |
| Secure headers, rate limits beyond unmask and customer reports, upload security | PARTIAL | Customer report rate limit COMPLETE; file imports bounded and DTD-free; headers not reviewed |
| Dependency scanning | PARTIAL | `govulncheck` run by hand: no vulnerabilities in code paths (run 16 Oct 2026, v1.9.0). Not a CI step. Container scanning MISSING |
| Privacy inventory, retention | MISSING | Counsel review needed |
| Load, DR restore, game day, runbooks | MISSING | `docs/runbooks/disaster-recovery.md` is a plan |
| Release checklist, pen-test pack | MISSING | Live execution sign-off is outside engineering |
| Production MFA and identity | BLOCKED | |

## B10 ReverseIt customer backend

| # | Requirement | Class | Evidence |
|---|---|---|---|
| 1 | Endpoints, customer-only data, idempotency, rate limits | COMPLETE | `internal/server/customer*.go`; cross-customer access returns 404 in every route test |
| 1 | Customer authentication | COMPLETE for the development provider; production BLOCKED | `internal/customer`; refused in production by configuration |
| 2 | Intake: consent, redacted copy and hash, never store or log the raw alert | COMPLETE | `TestAReportKeepsOnlyTheRedactedAlertItsHashAndTheConsent`. Image intake stores an opaque reference only |
| 3 | Parser interface | COMPLETE for the interface and a rule-based stand-in | `internal/alertparse`; the AI developer's parser (A9) is BLOCKED on them |
| 4 | Matching: exact, then constrained fuzzy with ranked reasons; never auto-link below threshold; unmatched claim; already reversed, already successful, duplicate report, other bank | COMPLETE | `creport` tests. Fuzzy matching is off until `customer_match.window_seconds` is approved |
| 5 | Dispute classification with a deterministic clock table; CUS002 | COMPLETE | `deadline`, `creport.classify`; ATM reports must state whose machine it was |
| 6 | Deadline engine: regulatory and internal apart, states, source status | COMPLETE | `deadline` tests; states beyond on_track and overdue need approved percentages |
| 7 | Complaint generator with validator, template fallback, edits stored | COMPLETE | `internal/complaint`; no model adapter is wired, so the template is always used |
| 8 | Case submission, ops alert, acknowledgement | COMPLETE | Scope acceptance 5 as `TestAPastedAlertBecomesACustomerCaseWithTheRightCountdownOnTheSharedQueue` |
| 9 | Status tracker including cases the customer never reported | COMPLETE (PR #28) | |
| 10 | Escalation ladder from configuration | COMPLETE (PR #28); contents BLOCKED on Compliance | Nothing is seeded; steps need a second person's approval |
| 11 | Feedback, labelled examples, reopen on dispute | COMPLETE (PR #28) | |
| 12 | Fraud and abuse | PARTIAL | Parser flags are advisory and stored with the case; rate limit; an unmatched claim cannot move money. No repeated-claim detector |

## B11 automation, mocks, demo controls

| Requirement | Class | Evidence |
|---|---|---|
| Schedulers: idempotent, observable, pausable | PARTIAL | Worker runs the detector consumer, inbound consumer, sweeps, rule sweep, hold sweep and reversal sweep; sweeps record job state under a freeze. No job registry, no status-requery, SLA, stale-source, notification-retry or file jobs |
| HTTP mock services with scripted outcomes | MISSING | `cmd/mock` answers 501; simulated status, ledger and provider live in database tables |
| OpenAPI for the interfaces and contract tests | PARTIAL | Six interface files and spec tests exist |
| Config service, hot reload, effective version on each decision | COMPLETE | |
| Demo controls: seed, scenario files, reset, time acceleration, burst, simulated flag, run-log export | PARTIAL | `cmd/demo` seed, play, reset and the simulated flag; `cmd/sim` seeds and the 50-transaction run. No time acceleration or burst over HTTP, no run-log export |
| Metrics endpoint | OPEN IN PR #19 | |
| Buffering and degraded-source state | PARTIAL | Held findings and source health; no buffering of outbound mock calls (there are none) |

## Section 15 acceptance

| Scope acceptance | Class | Evidence |
|---|---|---|
| 1 seeded duplicate detected and graded | COMPLETE | demo and detect tests |
| 2 confirmed failure reversed automatically, customer notified, audit recorded | COMPLETE | demo tests |
| 3 unknown status not reversed, in the approval queue with evidence | COMPLETE as review items | the pack's "dual-approver override" is not offered for unknown outcomes |
| 4 second reversal on the same case blocked | COMPLETE | |
| 5 customer alert parsed, classified, correct countdown, queued as CUS001 | COMPLETE | see B10 |
| 6 both dashboards reflect the above within seconds | PARTIAL | `reports/summary` reflects it; the executive and operations dashboard data are not separate endpoints |
| 7 changing a rule in configuration changes behaviour with no code change | COMPLETE | configuration and declarative rules |
| X1 20 end-to-end scenarios | PARTIAL | Covered by tests in the packages named above: 1, 2, 3, 4, 10, 11, 17. Covered in part: 12 (stale status evidence holds the gate and stale sources hold findings; no feed-outage end-to-end test through the gate), 14 (a customer report joins the exception; no agent view), 15 (classification only; lanes do not exist). Not covered: 5 to 9, 13, 16, 18 to 20 |
| X2 historical replay and pilot tooling | MISSING | Synthetic replay only |
| X3 demo hardening | COMPLETE for the baseline demo (earlier) | `docs/runbooks/demo-runbook.md` |

## Known conflicts and findings

- Migration numbers: develop holds 0001 to 0035. PR #19 adds 0022 and 0023 and must be renumbered after merging develop.
- Findings from the simulator under a source outage that are not fixed: a retry judged before the silent source crosses its stale threshold can still be flagged as a duplicate; a confirmed failure from one source is not superseded when another source's delayed confirmation makes the evidence conflict; a failure delivered together with its reversal opens no failure case (`docs/backend/simulator.md`, `docs/backend/source-degradation.md`).
- The failure-family detectors for ATM, fee, vendor, mismatch and incomplete routing key off rail or channel names, so a USSD or gateway failure is classified as a vendor drop. That is a convention to confirm with the bank.

## Test results at this gate (develop at 2883eff, 16 Oct 2026)

`go test -p 1 -race -count=1 ./...` against local Postgres 16 and Redis: 726 tests passed, 2 failed, 12 packages skipped, 46 passed. The 2 failures were in `internal/authz`: tests written before the `manage_ingestion` capability (migration 0023) did not count its two grants or its MFA flag. Fixed in the test only; `internal/authz` then passed. The full suite was not re-run after that fix. Not run in this pass: demo replay against `sr_demo`.
