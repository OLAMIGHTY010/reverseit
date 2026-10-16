# B1 Report

Date: 2026-10-15. Code: branch `feat/backend-b1-governance`, 58 commits on top of `develop` at `52391ac`. B2 has not been started.

B1 was started on the project owner's instruction while decisions D5 to D10 were still open. Each was implemented as the recommendation in the register and recorded in ADR 0003. Please confirm or change them (section 8).

## 1. What was built

| Area | Delivered | Where |
|---|---|---|
| Tenancy | `institutions`, `users`, `user_roles`. Row-level security on every tenant table. Tenant work runs as `sr_app` (no `BYPASSRLS`) with `app.tenant_id` set per transaction | migrations 0003, 0004; `internal/platform/db` |
| Outbox and streams | Events and consumers are tenant-scoped. Handlers run inside the message's tenant | `internal/outbox`, `internal/stream` |
| Tokens | Ed25519 JWTs, key sets with rotation, audience separation (operator, customer, service), 15 minute default life | `internal/identity` |
| Login | argon2id, uniform failures, lockout, every attempt audited | `internal/identity` |
| Authorisation | v2 matrix as data (99 cells), limits, ownership, reasons, dual control, MFA rule, no admin bypass | migration 0005; `internal/authz` |
| Segregation of duties | Maker is never checker. Pluggable conflict-of-interest check | `internal/authz`, migration 0006 |
| Audit | Per-tenant hash chain. Append-only and chain continuity enforced by the database. `GET /v1/audit` with scopes and `verify=true` | migration 0007; `internal/audit` |
| Data protection | Account tokens, AES-GCM vault, unmask endpoint (reason, MFA, rate limit, audited first), log masking | migration 0008; `internal/datashield` |
| Configuration | Immutable versions with a decision per version, maker-checker enforced by the database, rollback by re-proposal, key registry with ranges, regulatory clock values | migrations 0009, 0010; `internal/configsvc` |
| Regulatory clocks | Seven candidate clocks proposed as pending, with the v2 statuses. None is approved | `internal/configsvc/clockseed.go` |
| Business calendar | Working-day arithmetic, elapsed business time, cut-offs, settlement date, WAT. Built from approved configuration | `internal/bizcal` |
| Kill switch | Global, rail, exception type and policy scopes. Freeze by one person, lift by two. Fail-closed `Check` | migration 0011; `internal/killswitch` |
| Routing | Delegation, on-call shifts, escalation steps | migration 0012; `internal/routing` |
| Service auth | Per-service keys, scopes, 5 minute assertions, replay protection, `requireService` middleware | migration 0013; `internal/servauth` |
| Commands | `bootstrap` (first institution, operator, clock proposals), `keygen` (signing and vault keys) | `cmd/bootstrap`, `cmd/keygen` |
| Contract | 13 new operations marked `implemented` (15 in total) in `openapi.yaml`, all checked by the contract tests | `packages/api-contracts/openapi.yaml` |
| Docs | Auth and permission model, setup guide, ADR 0003, decision and risk registers, traceability | `docs/` |

Removed: the in-memory HMAC `internal/auth` prototype and the in-memory `internal/audit` prototype.

## 2. Build and test results

Linux, Go 1.27.2, PostgreSQL 16 and Redis 7 in containers.

| Check | Result |
|---|---|
| `gofmt -l .` | clean |
| `go build ./...`, `go vet ./...` | pass |
| `go test -p 1 -race -count=1 ./...` with services | **22 packages pass. 316 passing tests and subtests. 0 failed, 0 skipped** |
| Same without services | 147 pass, integration tests skip |
| `govulncheck` | One advisory, in `golang.org/x/crypto` v0.58.0 (GO-2026-5932). No fixed version exists and `govulncheck` reports that the code does not call it |
| `gitleaks` over 308 commits | no leaks |
| End-to-end with the real binaries | migrate, bootstrap two operators, start the API, log in, `/v1/me`, freeze a rail, refused unfreeze without MFA, read audit with `verify=true` (chain verified over 10 records), supervisor refused the `team` audit scope |

**GitHub Actions** on PR #12: Go (with Postgres and Redis services, a skipped test fails the job, `govulncheck`), secret scan, AI and web jobs all pass.

**Not run:** `docker compose up` (plugin not installed), the web and AI jobs, load or chaos tests, and any SAST beyond `go vet` and `govulncheck`.

The end-to-end run found a real defect, fixed in `9e2c00e`: reading configuration needed the same MFA as approving it, so a risk officer without MFA could not read a clock. Reading and proposing no longer need MFA. Approving still does.

History note: `919ba32` was pushed with a YAML error in `openapi.yaml` that the contract test caught. `b292901` fixes it.

## 3. What the tests prove

- **Tenant isolation.** A tenant cannot read, insert, update, move or delete another tenant's rows, for users, outbox, vault, configuration, kill-switch log, delegations and service accounts. The setting does not leak across pooled connections.
- **Authentication.** `HS256`, `none`, algorithm confusion, wrong or unknown key id, wrong audience, expired, not-yet-valid, over-long and malformed claims are all refused with the same 401. Every non-public route refuses an unauthenticated request, checked by walking the router.
- **Permissions.** All 99 role and capability cells match an independent copy of the v2 table. Admin gets nothing it was not given.
- **Maker-checker.** Refused in code and again by database triggers, for configuration and the kill switch.
- **Audit.** Changed content, hash links, time, actor and flags, a deleted first, middle or last record, all records deleted, two records swapped, and a forged record appended directly are all detected, each at the right record. Concurrent writers (25 at once) never fork the chain. A rolled-back change leaves no audit record.
- **Unmask.** Refused without the grant, MFA or a reason, before any lookup. Rate limited per caller. Nothing is released when the audit write fails. The audit trail never holds the number.
- **Kill switch.** Each scope blocks only what it covers. A requester cannot approve. A check that cannot read the switch blocks.

## 4. Security findings

| # | Finding | Status |
|---|---|---|
| B1-1 | **No MFA mechanism exists.** The code enforces an `mfa` claim but nothing issues one, so approving configuration, unmasking and lifting a freeze are unreachable for real users | Open. Decision D20. Deliberately fails closed |
| B1-2 | **Superusers bypass row-level security.** If production connects as a superuser, tenant isolation is silent | Open. Documented. A startup check is planned for B9 |
| B1-3 | **Audit head is not anchored outside the database.** Someone who rewrites every row and the head together is not detected | Open. `audit.Anchor` exists, the export job does not |
| B1-4 | Reading configuration demanded MFA | Fixed (`9e2c00e`), found by the end-to-end run |
| B1-5 | The HMAC token prototype and its shared secret | Removed |
| B1-6 | `x/crypto` advisory GO-2026-5932, no fix available | Open, not reachable per `govulncheck` |
| B1-7 | No token revocation. A token lives out its 15 minutes after the user is disabled | Accepted. Short lifetime is the control |
| B1-8 | Login protection is lockout only. No per-address rate limiting | Open, B9 |
| B1-9 | Client address is taken from the connection, not forwarding headers, so it is the proxy's address behind one | Open until a trusted proxy list exists |
| B1-10 | One environment key seals all accounts. Rotation tooling is not built (`key_id` is stored) | Open |
| B1-11 | The permission matrix is read at startup. A change needs a restart | Accepted |
| B1-12 | Role names are the project's working names and unverified against the bank's delegated-authority matrix | Open, with Wema |
| B1-13 | `sr_app` creation needs `CREATEROLE`, and the role is left in place when migrations are reverted | Documented |

## 5. Mocked, unbuilt or partial

- **Not built:** MFA issuance; customer login (B10); HTTP routes for delegation, on-call and escalation; scheduled export of the audit head; response-level masking (no data routes exist yet); OIDC or SAML clients.
- **Built but unused until later phases:** `killswitch.Check` (B5 must call it), `requireService` (B2 mounts event ingestion), `servauth`, `routing`, the vault `Store` (B2 tokenises accounts during ingestion), `datashield.Mask`.
- **No data was invented.** No holiday, cut-off, approval limit, waiting threshold or regulatory clock is approved or built in. The calendar reports `NOT_CONFIGURED`. Approvals deny because no limit is configured. The seven clocks are pending proposals with their press-report statuses.
- **Prototypes still present and unwired:** `internal/transactions` and `internal/exceptions` (in-memory, replaced in B2 and B3).

## 6. Traceability

See `requirement-traceability.md`, section 9. In short: every v2 B1 item is done except the three gaps listed above (MFA issuance, anchoring, response masking) and delegation having no HTTP routes.

## 7. Exit-gate assessment

v2 B1 acceptance: *no endpoint is reachable without auth and authorisation tests; tamper of any audit row is detected by verify-chain.*

| Criterion | Met | Evidence |
|---|---|---|
| Unauthorised users cannot reach protected resources | Yes | Router-walk test, per-route role tests |
| Cross-tenant access is demonstrably blocked | Yes | Isolation tests for every tenant table |
| Audit tampering detected | Yes | Eleven kinds, each at the right record |
| CI green | Yes | All four jobs pass on PR #12 |
| MFA-gated actions usable | **No** | D20 |

Assessment: the exit requirement is met for what can be exercised. I recommend not declaring B1 closed until D20 (MFA for Track H) is decided, because B5 approvals cannot be demonstrated without it.

## 8. Decisions needed

- **Confirm** D5 to D10 as applied (ADR 0003), or say what to change.
- **Decide D20:** how a user gets an MFA-backed token in Track H. Recommendation: a simulated second factor behind an interface (for example TOTP with a seeded secret), labelled as simulated everywhere.
- **Decide D21:** the team model behind the `team` audit scope (needed in B4).
- **Still open from B0:** D4 (contract owners), D14 (money representation), D15 (JSON naming), D12, D13, D17 to D19.
- **From Wema:** approval limits, waiting thresholds, rail cut-offs, public holidays, and confirmation of the regulatory clocks.

## 9. Proposed B2 breakdown

Needs D14 and D15 settled first, because the event schema fixes the money type and field names.

1. Canonical `TransactionEvent` schema and validation (versioned), with `amount_minor`.
2. Migrations: `transactions`, `transaction_events` (append-only), `external_refs`, lifecycle projection.
3. `POST /v1/events` behind `requireService("events:write")`: idempotency (200 duplicate, 409 conflict, 422 to dead letter).
4. Account tokenisation on ingest through the vault.
5. Source adapters, one per shape: NIP, POS, ATM, card, mobile, internet, USSD, gateway, merchant.
6. Correlation by external references, with confidence. Never account, amount and time alone.
7. Lifecycle projection with explicit `UNKNOWN`, late and out-of-order events, deterministic replay.
8. Unmatched-evidence queue with an audited manual link.
9. Source freshness and clock-skew reporting.
10. Settlement and statement file ingestion (CSV, XML, fixed-width) with versioned mappings.
11. Dead-letter replay tool and malformed-event quarantine (FR-03).
12. Simulator: seeded scenarios, ground-truth label file, the 50-transaction demo, burst mode.
13. Replay test: shuffled order gives identical projection and no duplicates.
14. Detection latency measurement (p50, p95).
15. OpenAPI promotion and the transaction feed contract test.
