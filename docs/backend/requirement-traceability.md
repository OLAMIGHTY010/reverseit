# Backend Requirement Traceability Matrix

Last updated: 2026-10-15. Section 1 was refreshed against the implemented backend (baseline tag `demo-baseline-2026-10-15`). The detailed per-requirement status for B2 to B11 is in `remaining-work-matrix.md`; sections 2 to 9 below are unchanged from earlier phases. Reconciled against the four playbooks supplied by the project owner:

1. Full Project Scope ("Scope", sections numbered as in that document)
2. v2 Integrated Scope Prompt Pack ("v2")
3. Vol 2 Phase-by-Phase Production Playbook ("Vol2")
4. Complete Playbook Vol 1 + Vol 2 ("Complete"). Its text overlaps v2 and Vol2 for the backend, and no backend requirement was found that exists only in it. A line-by-line check of its non-backend content (frontend, AI, pilot, deployment) was not done and is not claimed.

State key: `none` nothing in the repo, `stub` placeholder or in-memory, `partial` usable base. "Track H" means required for the hackathon demo.

## 1. Functional requirements (Scope 13.1, tightened by v2 section 5.4)

| FR | Requirement | Phase | Repo state |
|---|---|---|---|
| FR-01 to FR-03 | Ingest all sources into one normalised stream, support NIP, POS, ATM, card, mobile, internet, USSD, gateway, merchant, NIBSS and settlement files, quarantine malformed events | B2 | partial. REST ingestion, validation and quarantine done (`txn`, `server/ingest.go`). File and consumer adapters, mapping configs, source freshness: none |
| FR-04 to FR-06 | Duplicate detection with references and session, risk grade Low/Medium/High, 30 s default window, configurable | B3 | partial. Duplicate detector with risk grade and configurable window (`detect`). No legitimate-repeat allow-list |
| FR-07 to FR-09 | Debit without credit, debit with settlement incomplete, NIP failed with funds withheld, POS timeout with debit (plus ATM, vendor, fee cases) | B3 | partial. Confirmed failure and stuck transactions detected. ATM, vendor, fee and settlement-incomplete detectors: none |
| FR-10 | Unknown status detected, status query triggered, never treated as failed | B3, B5 | complete. Unknown is never treated as failed; blocked at the gate (`detect`, `reversal`) |
| FR-11 | Settlement file versus core banking mismatch | B3, B6 | none. Needs B6 |
| FR-12 | ML score on flagged transactions | AI track (A5) | AI track. No backend adapter yet |
| FR-13 to FR-16 | Auto-create case, codes, root cause on every case, priority | B4, B3 | partial. Auto-created cases with codes, root cause and fixed priority (`cases`, `detect`). No configurable priority score |
| FR-17 | SLA deadline from regulatory clock, regulatory deadline and internal target shown separately | B4, B10 | partial. Deadline from approved clocks. No separate internal target |
| FR-18 to FR-23 | 8-check gate, status query first, auto-reverse only on confirmed failure, approval routing, idempotency, REV001 on failure | B5 | mostly complete against simulated sources (`reversal`). Approval expiry, circuit breaker, partial and fee handling: none |
| FR-24, FR-25 | Customer and ops notifications, deduplicated, two-stage | B7 | partial. Two-stage, deduplicated, recorded and simulated (`notify`). No delivery tracking, retry or provider adapters |
| FR-26 to FR-34 | ReverseIt intake, parse, match, classify, countdown, complaint, status timeline, escalation ladder, labelled examples | B10 (parse is AI A9) | partial. Agent-entered complaints and customer status only. No customer authentication, parser, fuzzy matching, deadline states, escalation ladder or label store |
| FR-35, FR-36 | Operations and executive dashboard data | B8 | partial. `GET /v1/reports/summary` and reversal reconciliation. No metrics catalogue |
| FR-37 | Decision log with actor, before, after, time, hash-chained | B1 | complete. Hash-chained audit with verification (`audit`) |
| FR-38 | Scheduled bots never bypass the gate | B11 | partial. Worker sweeps go through the gate. Not kill-switch aware at job level |
| FR-39 | Rules, thresholds, windows, limits and clocks editable without code, versioned, audited | B1, B3 | complete. Versioned maker-checker configuration (`configsvc`). Rules are code, not yet declarative |
| FR-40 | RBAC for operations, management, compliance, customers, plus supervisor, agent, admin | B1 | complete for operators (`authz`). Customer audience: none |
| FR-41 to FR-58 | Added by v2 section 5.5: customer auth and rate limits (41), alert authenticity (42), wrong-account path (43), duplicate posting vs transfer (44), notification tracking and opt-out (45), inward credit delay EX-DLY (46), business calendar WAT (47), internal target earlier than deadline (48), demo controls (49), label store with ops confirmation (50), link repeat reports (51), reopen on disputed outcome (52), OpenAPI per interface with contract tests (53), mask before external AI call (54), mobile-first flow (55, frontend), consent and retention (56), agent view (57, frontend), mocked status and reversal services with scripted outcomes (58) | B0 to B11 as in v2 | none |

## 2. B0 deliverables from v2 (prompt B0)

| Item | Source | Planned in B0 | Notes |
|---|---|---|---|
| ADRs listing rejected alternatives | B0 | Yes, ADR 0002 | Accepted for D1, D2 |
| Service decomposition and modular monolith recommendation | B0 | Yes, `module-layout.md` | |
| C4 context and container diagrams in Mermaid | B0 | Yes | Not in the earlier plan |
| Data store definitions: operational, append-only event, audit (hash-chained), analytical later | B0 | ADR and layout doc | Schemas arrive in B1, B2 |
| Multi-tenant strategy: `tenant_id` plus row-level security | B0 | Decision only | Implementation is B1 (D7) |
| Repo layout, coding standards, CI with lint, unit, integration, contract, SAST, dependency scan, secret scan | B0 | Yes, staged | Scans added where tooling is available |
| Makefile and dev compose with api, simulator, detector worker, scheduler, mock-status, mock-reversal, mock-notify, postgres, redis, frontend | B0 | Compose carries only services that exist. Mock services start as health-only stubs and are labelled so | Behaviour arrives in B2, B5, B7 |
| OpenAPI v0.1 and one OpenAPI file per scope 13.3 interface, with contract tests | B0, FR-53 | Yes | Six interfaces: transaction feed, status query, reversal API, settlement and NIBSS files, SMS and email gateway, customer app and ALAT |
| ADR items for mentors: status-query-first, approval thresholds, "within regulatory limits" | B0 | Listed as D19 | Cannot be decided without Wema |
| Risk list | B0 | `risk-assumption-register.md` | |

Vol2 phase 3 exit gate also names threat model, DPIA draft, IaC, observability, and tests for tenancy, auth, RBAC, audit and config. Of these, B0 contributes the skeleton and observability foundation. Tenancy, auth, RBAC, audit and config tests belong to B1. The threat model exists in `docs/security/threat-model.md`. The DPIA and IaC are programme deliverables outside the backend brief.

## 3. Exception taxonomy and codes

Scope codes (confirmed in the Scope document): DUP001, FAIL001, UNK001, REV001, STL001, CUS001.

v2 section 5.6 proposes more. They are "proposed" in v2 and are not confirmed by the bank:

| Proposed code | Meaning | Taxonomy type |
|---|---|---|
| DUP002 | Duplicate transfer, two credits, recall path | EX-DUP |
| FAIL002 | ATM no or partial dispense | EX-ATM |
| FAIL003 | Vendor value not delivered | EX-VND |
| FAIL004 | Fee on failed transaction | EX-FEE |
| UNK002 | Late success after reversal | EX-LTE |
| CUS002 | Wrong account or amount, customer error path | EX-CUS |
| DLY001 | Inward credit delay | EX-DLY |

Taxonomy types in v2 section 6.1: EX-DUP, EX-DNC, EX-INC, EX-STK, EX-STL, EX-MIS, EX-RVF, EX-RVP, EX-RTY, EX-INC2 (incident), EX-ATM, EX-VND, EX-FEE, EX-LTE, EX-CUS. EX-DLY appears in v2 section 5.3 and 5.6.

Implementation rule: store the code list as configuration so proposed codes can be confirmed or dropped without a code change.

## 4. State machines and contracts defined by the playbooks

| Item | Source | Phase |
|---|---|---|
| Case states: Detected, Awaiting Evidence, Under Investigation, Awaiting Approval, Remediation Submitted, Outcome Verification, Resolved, Closed. Side states: Rejected Action, Remediation Failed, External Dependency, Suppressed, Closed No Exception, Closed Customer Error, Reopened | v2 6.2 | B4 |
| Transaction lifecycle: INITIATED, DEBITED, SENT, PENDING, CONFIRMED_SUCCESS, CONFIRMED_FAILURE, UNKNOWN, REVERSAL_* | v2 B2 | B2 |
| Reversal tracks: accepted, processing, financial outcome verified, tracked as separate fields | v2 6.3, 7.1 | B5 |
| 8-check gate with failure outcomes per check, ambiguity returns HOLD | v2 6.3 | B5 |
| Permissions matrix 6.4 stored as data, roles: transaction analyst, recon analyst, reversal officer, supervisor, head of ops, risk/compliance, admin/integration, executive, agent | v2 6.4 | B1 |
| Event envelope, `POST /v1/events` with 202, 200, 409, 422, 429 | v2 7.2 | B2 |
| Core API list (events, transactions, exceptions, cases, complaints, reversals, approvals, audit, killswitch) | v2 7.3 | B1 to B7 |
| Source Authority Matrix, to be completed in discovery | v2 7.5 | B2, B5 |
| Reliability patterns: at-least-once with dedupe, outbox, DLQ with replay, circuit breaker, single-writer reversal with lock, reconciliation after outage, per-source freshness | v2 7.6 | B2, B5, B6, B11 |

## 5. Items in the playbooks that the brief and the first matrix did not list

These are real backend requirements found in the documents and are now tracked:

1. Kill switch at four scopes (global, rail, exception type, policy) with dual control to re-enable. Brief mentioned one switch. (B1, B5)
2. Regulatory clock registry table with source, status (confirmed, reported, draft, conflicting) and effective date. (B1, B10)
3. Delegation, out-of-office reassignment and on-call model. (B1)
4. Reason-gated unmask endpoint with rate limit, and audit of reads of sensitive data. (B1)
5. MFA claim enforcement for privileged actions. (B1)
6. Rules in shadow mode before activation, rollback, maker-checker on rule changes. (B3)
7. Unmatched evidence queue with audited manual link. (B2)
8. Per-source freshness status and clock-skew measurement. Stale evidence cannot satisfy gate checks. (B2, B5)
9. Detection latency measurement from source time to case creation, reporting p50 and p95. (B2, B8)
10. SLA pause rules: named external dependency only, who may pause, maximum pause. (B4)
11. Repeat-contact count and linking of duplicate customer reports. (B4, B10)
12. Labelled example store with ops confirmation before training use. (B10)
13. Simulator with ground-truth label file, shuffled replay equals identical projection. (B2)
14. Per-tenant AI data boundary and mask-before-external-call. (B10, AI gateway)

## 6. Conflicts and gaps between documents

| Item | Finding |
|---|---|
| Stack | v2 default is Python and FastAPI. Owner approved Go. Recorded in ADR 0002. |
| Idempotency and duplicate windows | Scope puts them in Redis. ADR 0002 keeps keys and locks in PostgreSQL, with Redis as optional cache only. |
| Money type | v2 contract uses `amount_minor`. PR template says `NUMERIC(18,4)`. See D14. |
| JSON naming | v2 uses snake_case. Web app and current Go use camelCase. See D15. |
| Reversal table | Scope says its field list was cut off in the original. Fields are proposed, not authoritative. |
| POS/web clock | 72 h versus 48 h. See D18. |
| ATM newer guidelines | Some outlets say draft, some say issued. Final status unconfirmed. |
| Existing OpenAPI and web types | Differ from the v2 contract (paths, casing, channel and exception type names, role names `ANALYST_MAKER` and `SUPERVISOR_CHECKER` versus the v2 matrix). To reconcile in the contract freeze. |

## 7. Safety rules carried into every phase

Source: v2 Shared Preamble rules 1 to 11 and brief section 5. Each needs a named test before its phase can close.

- S1 Unknown is not failed. Unknown never triggers automatic reversal.
- S2 Eight checks, any failure blocks automatic reversal, ambiguity returns HOLD.
- S3 Three separate reversal tracks. An acknowledgement is not proof of posting.
- S4 Maker and checker differ. Admin does not bypass.
- S5 Evidence and transitions are append-only.
- S6 Retries and duplicate submits have no second financial effect.
- S7 Execution off by default, kill switch honoured by every consumer.
- S8 AI cannot execute, fabricate evidence, or override deterministic controls.
- S9 All execution is simulated and labelled. No customer is told funds are returned before verification.
- S10 Regulatory, rail, SLA and threshold values are configuration with source and VERIFY flag.

## 8. Phase mapping summary

| Phase | Status |
|---|---|
| B0 | Code delivered on `feat/backend-b0-foundation`, awaiting review. Exit gate assessed in `b0-report.md`: met for everything that was run, with listed exceptions |
| B1 | Code delivered on `feat/backend-b1-governance`, awaiting review. See section 9 and `b1-report.md` |
| B2 to B11 | Not started |

## 9. B1 status against the v2 prompt B1

| v2 B1 item | Status | Notes |
|---|---|---|
| 1 Multi-tenancy, row-level security, cross-tenant tests | Done | `sr_app` role, policies on every tenant table, tests for read, write, move, leak, and for each later feature |
| 2 JWT validation, session timeout, MFA claim, service-to-service auth | Done, with a gap | EdDSA key sets with rotation; 15 minute tokens; MFA enforced as a claim; signed service assertions with replay protection. **No mechanism issues MFA (D20)**. "OIDC/SAML-ready" means external issuers can be added as key-set entries; no OIDC or SAML client is built |
| 3 RBAC plus attribute rules, matrix as data, maker-checker, conflict check | Done | 99 cells checked against an independent copy of the v2 table. Limits come from configuration, and none is configured |
| 4 Tokenisation, masking, reason-gated unmask with rate limit | Done, with a gap | Masking applies to logs. No response serialisers exist yet, so response masking arrives with the first data routes |
| 5 Audit: append-only, hash chain, verify, audit of sensitive reads, time discipline | Done, with a gap | Out-of-database anchoring of the chain head is not built. Time comes from one appender clock, truncated to microseconds |
| 6 Config service: versioned, maker-checker, source, effective date, verify status | Done | Integer keys and structured regulatory clocks. No values are built in |
| 7 Business calendar: holidays, weekends, cut-offs, elapsed business time, next cut-off | Done | Holidays and cut-offs are configuration. None is seeded because no source was supplied (assumption A6) |
| 8 Kill switch: four scopes, dual control to re-enable, consumers check first | Done | `Check` fails closed. No consumer exists yet, so B5 must call it |
| 9 Delegation and on-call | Service layer only | No HTTP routes. Routes work, never lends authority |
| Regulatory clock registry seeded with the 5.3 values and statuses | Done | Seven candidate clocks proposed as pending, with the pack's statuses. None is approved |
| FR-37 decision log | Done | Hash-chained, in the same transaction as the decision |
| FR-39 editable rules, versioned, audited, maker-checker | Done | Integers and clocks |
| FR-40 RBAC | Done | |
| FR-41 customer authentication | Partial | Separate audience and verification only. Customer login and tables are B10 |

B1 exit requirement: "no endpoint is reachable without auth and authorisation tests; tamper of any audit row is detected by verify-chain". Met for every route that exists, enforced by a test that walks the router and fails if a non-public route answers without a token. Eleven kinds of tampering are detected in tests.
