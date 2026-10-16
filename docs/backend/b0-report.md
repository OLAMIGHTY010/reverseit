# B0 Report

Date: 2026-10-15. Code: branch `feat/backend-b0-foundation` (54 commits on top of `develop` at `7b54463`). This report is the review input. B1 has not been started.

## 1. Architecture decisions completed

| Decision | Outcome |
|---|---|
| D1 language | Go with Chi. Python stays with the AI service. |
| D2 messaging | Redis Streams with a PostgreSQL transactional outbox. PostgreSQL is authoritative. |
| D11 Go version | 1.27.2 in `go.mod`, CI (read from `go.mod`), Docker and docs. No dependency blocker. |
| D3 OpenAPI first | Adopted as default. Contract tests enforce it. |
| D16 error shape | Adopted as default: `{ error, message, trace_id, details }`. |

Recorded in ADR 0002 and the decision register. Open: D4, D5 to D10, D12 to D15, D17 to D19, see section 8.

## 2. What was built

| Area | Delivered | Where |
|---|---|---|
| Toolchain | Go 1.27.2 everywhere, `go.sum` committed, Dockerfile builds all binaries, non-root image | `go.mod`, `infra/docker/Dockerfile.api` |
| Config | Typed, validated, all errors reported together, no default secrets, placeholder secrets refused outside development and test, production refuses `FINANCIAL_EXECUTION_ENABLED=true`, config logs without secrets | `internal/config` |
| HTTP | `/v1/health`, `/v1/ready`, error envelope, trace id, request log without query strings, panic recovery without leaking, graceful shutdown that drains requests | `internal/server`, `internal/platform/httpx`, `internal/observability` |
| Database | pgx pool, ping check, transaction helper | `internal/platform/db` |
| Migrations | Checksummed, advisory-locked, transactional runner. Edited applied migration is an error. `migrate up/down/status` command | `internal/platform/migrate`, `cmd/migrate`, `apps/api/migrations` |
| Outbox | Transactional enqueue, ordered relay to Redis Streams with retry bookkeeping, single active relay, requeue for stream loss | `internal/outbox` |
| Consumers | Consumer groups, idempotent handling via `processed_events`, redelivery after idle, claiming work from crashed consumers, dead-letter stream, replay | `internal/stream` |
| Worker | Process that runs the relay and a health endpoint, stops together on failure | `internal/worker`, `cmd/worker` |
| Helpers | Cursor pagination (max 200), idempotency key validation, request fingerprint | `internal/platform/httpx` |
| Contracts | `/v1` OpenAPI baseline (2 implemented operations, 13 planned), six external interface contracts, v0.1 moved to `legacy/` | `packages/api-contracts` |
| Contract tests | Spec validity, status on every operation, `/v1` prefix, served routes equal documented routes, six interfaces valid, mock routes equal interface contracts | `internal/contract` |
| Mocks | Status, reversal and notification stubs: health only, `501` on interface routes, all say simulated and not implemented | `internal/mocks`, `cmd/mock` |
| Compose | migrate, api, worker, three mocks, with healthchecks, one backend image | `compose.yaml` |
| CI | Format, build, vet, race tests with real Postgres and Redis, fails on any skipped test, govulncheck, gitleaks | `.github/workflows/ci.yml`, `.gitleaks.toml` |
| Docs | C4 diagrams, local setup, package overview, contract change log | `docs/architecture/c4.md`, `docs/backend/local-setup.md`, `docs/backend/contract-changes.md` |

Not built, by design: authentication, tenancy, any domain table, ingestion, detection, cases, reversal. The mocks have no behaviour.

## 3. Build and test results

Run on Linux, Go 1.27.2, Docker 29.1.3, PostgreSQL 16 and Redis 7 in containers.

| Check | Command | Result |
|---|---|---|
| Format | `gofmt -l .` | clean |
| Build | `go build ./...` | pass |
| Vet | `go vet ./...` | pass |
| Tests with real services | `go test -p 1 -race -count=1 ./...` with `TEST_DATABASE_URL` and `TEST_REDIS_ADDR` set | 14 packages pass, 134 passing tests and subtests, 0 skipped |
| Tests without services | same, variables unset | pass, 102 passing, integration tests skip |
| Vulnerabilities | `govulncheck@v1.9.0 ./...` | none, after the fixes below |
| Secrets in history | gitleaks v8.30.1, 193 commits | none, after reviewed allowlist |
| Image build | `docker build -f infra/docker/Dockerfile.api` | pass, `/app` holds api, worker, migrate, mock |
| Topology | migrate, api, worker, 3 mocks run with `docker run` on a private network and compose healthcheck commands run inside | all healthy, api and worker run as uid 100 |
| Process checks | API and worker binaries run with env and signalled | `/v1/health` and `/v1/ready` answer, SIGTERM exits 0, missing config exits 1 with every problem listed |
| Guide | commands in `local-setup.md` run end to end | pass |

**Not run:**

- `docker compose up`. The compose plugin is not installed. A standalone binary was downloaded and deleted unrun because it did not match its published checksum. The compose file was parsed as YAML and its topology reproduced by hand.
- The GitHub Actions workflow. It has not run on GitHub; the same steps were run locally.
- The web and AI-service CI jobs and their tests.
- SAST beyond `go vet` and govulncheck. No gosec or CodeQL.
- Load, chaos and failure-injection tests beyond Redis-down and crash-recovery cases in the outbox and stream tests.

Two mistakes were made and corrected during the work, both visible in history: a config commit that declared worker settings but did not read them (`2064908` fixes it), and an unintended skip of integration tests in one local run, caught before reporting. A test-isolation bug where migration tests dropped a shared table was found by the full run and fixed (`2a0ba69`).

## 4. Commits

54 commits, all authored as `Hollujay <locko.charles@gmail.com>`. No co-author trailers and no generated-by lines (checked with `git log --format=%B | grep -iE 'co-authored|claude|generated'`, zero matches). Pushed to `origin/feat/backend-b0-foundation` after each unit. The first ones, in order:

`3a13b3f` chore(api): target Go 1.27.2 and commit go.sum
`c1888ee` fix(auth): disable demo login that issued tokens without credentials
`53793d4` fix(auth): remove implicit admin bypass from role checks
`9d4a6cc` fix(auth): require strong secret and pin HS256, expiry and issuer on tokens

The complete list is `git log --reverse upstream/develop..feat/backend-b0-foundation`.

## 5. Security findings

| # | Finding | Status |
|---|---|---|
| S-1 | Login issued a 24 hour token for any email, with no credential check, defaulting unknown users to analyst | Fixed. Handler returns 501 with no token, route not mounted, tests assert it (`c1888ee`) |
| S-2 | `RequireRoles` let ADMIN pass every check, defeating maker-checker | Fixed. No implicit bypass (`53793d4`) |
| S-3 | Token validation accepted any HMAC method, did not require expiry or issuer, and accepted an empty secret | Fixed. HS256 only, expiry and issuer required, secret of at least 32 bytes (`9d4a6cc`) |
| S-4 | `golang-jwt/jwt` 5.2.1 (GO-2025-3553), `golang.org/x/text` 0.29.0 (GO-2026-6629, GO-2026-5970) | Fixed by upgrades to 5.3.1 and 0.43.0 (`dd450bb`, `9a3a1dd`) |
| S-5 | Default `JWT_SECRET` and DB password in `.env.example` and `compose.yaml` are readable placeholders | Mitigated. Config refuses them in staging and production. Compose still defaults to them, so it is a development tool only |
| S-6 | `internal/audit` is in memory, restarts at a fixed genesis, not tenant scoped, not append-only | Open, not wired to anything. Replaced in B1 |
| S-7 | Signing is a shared HMAC secret (D5) | Open, B1 |
| S-8 | No route requires authentication yet. Only health and ready exist and they return no sensitive data | By design for B0. B1 adds auth before any data route |
| S-9 | Gitleaks reported 5 findings. Two are fake constants in tests I wrote, three are mock idempotency keys in `apps/web/src/services/mockSimulatorData.ts` written by another contributor | Reviewed as false positives and allowlisted file by file with a reason |
| S-10 | Compose publishes Postgres and Redis ports to the host with no password on Redis | Development only. Not for shared hosts |
| S-11 | `cmd/migrate` and `cmd/worker` require `JWT_SECRET` because they share the config loader | Open, minor. Split the loader requirements in B1 |
| S-12 | CORS is not configured (the unused `go-chi/cors` dependency was dropped by `go mod tidy`) | Open. Decide with the frontend owner when the web app first calls the API |
| S-13 | API `/v1/ready` does not check Redis because the API does not use it yet | Open, add when the API publishes events |

## 6. Traceability status

`requirement-traceability.md` was reconciled against the four playbooks. B0 items from v2 prompt B0:

| Item | Status |
|---|---|
| ADRs with rejected alternatives | Done (ADR 0002) |
| Service decomposition and modular monolith | Done (`module-layout.md`, `c4.md`) |
| C4 context and container diagrams in Mermaid | Done. Not rendered in a viewer during this work, only written |
| Data stores and tenancy decision | Decided in ADR. Tenancy implementation is B1 |
| Repo layout, CI with lint, unit, integration, contract, dependency scan, secret scan | Done. SAST not done |
| Dev compose with api, simulator, detector worker, scheduler, mock-status, mock-reversal, mock-notify, postgres, redis, frontend | Partly. api, worker, three mocks, postgres, redis, migrate and the existing web and ai-service. Simulator, detector worker and scheduler do not exist yet (B2, B3, B11) |
| OpenAPI v0.1 and one file per scope 13.3 interface with contract tests | Done |
| ADR items for mentors (status-query-first, approval thresholds, "within regulatory limits") | Listed as D19. Cannot be decided without Wema |
| Risk list | Done |

Safety rules S1 to S10 have no code yet except S7 (execution disabled by default, refused in production), S9 (mocks labelled simulated) and S10 (nothing hard-coded).

## 7. Exit-gate assessment

The B0 exit requirement is: the skeleton builds, starts and passes its foundation checks.

| Criterion | Met | Evidence |
|---|---|---|
| Builds | Yes | build, vet, image build |
| Starts | Yes | api, worker, mocks, migrate run locally and in containers, shutdown clean |
| Foundation checks pass | Yes, locally | 134 tests with real services, govulncheck and gitleaks clean |
| Migration up and down | Yes | tested against Postgres |
| Outbox and streams survive failure | Yes | Redis-down, crash-before-ack, poison message, replay, concurrent relays |
| Routes match the contract | Yes | contract tests |
| Unsafe login neutralised | Yes | tests |
| CI green | **Not shown** | workflow has not run on GitHub |
| Compose up | **Not shown** | plugin unavailable |
| Contract freeze possible | **No** | D4, D14, D15 open |

Assessment: the gate is met for everything that could be run here. I recommend not declaring B0 closed until the GitHub workflow is green on the pull request, someone runs `docker compose up`, and D4, D14 and D15 are answered. The first two are checks I could not perform. The third needs your decision.

## 8. Outstanding decisions and blockers

Blocking before the first contract freeze: D4 (named frontend and AI contract owners), D14 (`amount_minor` integer or decimal string), D15 (snake_case or camelCase).

Blocking B1: D5 (token signing), D6 (identity source), D7 (tenancy model), D8 (customer identity), D9 (admin and maker-checker), D10 (masking).

Blocking later phases: D12 (who enables execution), D13 (clock, limit and threshold values from Wema and Compliance), D17 (24 hour clock start), D18 (POS clock 72 h or 48 h), D19 (status-query-first and approval thresholds).

Other: an unmerged documentation update. PR #3 was merged on 2026-10-15 before the later documentation commits, so a second documentation PR carries the ADR acceptance, decision register, reconciled traceability matrix and this report.

## 9. Proposed B1 breakdown

Requires D5 to D10 first. Each line is one commit with tests.

1. Replace `internal/auth`: verify tokens against a key set, tenant and role claims, audience for operator and customer, MFA claim for privileged actions. Remove the HMAC helper.
2. Auth middleware that fails closed, with a test that every non-health route in the router requires it.
3. Migration: `institutions`, `users`, `roles`, tenant-scoped base. `tenant_id` on `outbox_events` and `processed_events`.
4. Tenant context and row-level security policies with a tenant-scoped transaction helper.
5. Cross-tenant read and write tests, including the outbox.
6. Permission matrix (v2 6.4) as data, plus the enforcement function with attribute rules (amount, queue, exception type, own-case).
7. Segregation of duties check (maker is not checker, pluggable conflict check).
8. Audit store: append-only table, hash chain, grants without UPDATE or DELETE, verify-chain function and endpoint, tamper test.
9. Audit of sensitive reads.
10. Account tokenisation and masking in the serialisation layer, reason-gated unmask with rate limit and audit.
11. Versioned configuration service with maker-checker for changes and `source_reference`, `effective_from`, `verify_status`.
12. Regulatory clock registry seeded with the v2 5.3 values marked unverified.
13. Business calendar (WAT, holidays, working-day arithmetic) with edge-case tests.
14. Kill switch at four scopes with dual control to re-enable, checked by a consumer interface.
15. Delegation and on-call model.
16. Service-to-service authentication.
17. Authorisation matrix tests for every role and capability.
18. OpenAPI: auth, audit, killswitch operations promoted from planned.

B1 exit: unauthorised users cannot reach protected resources, cross-tenant access is demonstrably blocked, tampering with an audit row is detected.
