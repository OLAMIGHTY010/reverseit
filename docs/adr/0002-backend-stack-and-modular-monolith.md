# ADR 0002: Backend Stack and Modular Monolith

Status: Accepted for D1 and D2 (approved by the project owner on 2026-10-15). Other items in `docs/backend/decisions-requiring-approval.md` stay open and are not decided here.

## Context

ADR 0001 is a placeholder. The repository already uses a Go API (`apps/api`, Chi, `Dockerfile.api`, CI job) and a Python FastAPI service for AI (`apps/ai-service`). The Track H default stack in the v2 prompt pack names Python and FastAPI for the backend, but the same document says any layer may be swapped to fit the team. The project owner has approved keeping Go.

The v2 prompt B0 asks for service decomposition, a modular monolith recommendation, technology selection with alternatives, data store definitions, a tenancy strategy, repository layout, CI stages, and rejected alternatives with reasons. This ADR covers the stack and process model. Tenancy and authentication detail belong to B1 ADRs.

## Decision

1. **Language.** Go with Chi for the API, worker, migrator and simulator. Toolchain target is Go 1.27.2, declared once in `go.mod` and matched in CI, Docker and documentation. Python and FastAPI remain for the AI service only and are owned by the AI/ML developer.
2. **Process model.** Modular monolith in one repository. Separately runnable API, worker, migrator and simulator binaries share `internal/` packages. Reversal execution sits behind a single package with its own credentials, importable only by the reversal service.
3. **System of record.** PostgreSQL 16 is the only authoritative store for transactions, events, exceptions, cases, reversals, approvals, configuration and audit. Redis is never the source of truth for a financial outcome.
4. **Messaging.** Redis 7 Streams carries asynchronous events between the API and workers. State changes that must publish an event are written with a transactional outbox row in PostgreSQL, and a relay publishes outbox rows to Redis Streams. Consumers are idempotent, use consumer groups, recover pending entries after a crash, retry with bounded backoff and move poison messages to a dead-letter stream with replay tooling. Losing Redis must not lose a committed state change: the outbox is replayed.
5. **Redis uses.** Stream transport, short-lived caches and rate limit counters. Idempotency keys and remediation locks are persisted in PostgreSQL (unique constraints). A Redis lock may be added later only as an optimisation in front of the database constraint, never in place of it.
6. **API.** Versioned under `/v1`, OpenAPI first, with contract tests that fail when routes and specification diverge. One OpenAPI file per external interface listed in scope section 13.3, so a real integration can replace a mock without engine changes.
7. **Tenancy.** Shared schema, `tenant_id` on every tenant table, PostgreSQL row-level security as a second line of defence. Detailed in a B1 ADR.
8. **Financial execution.** Simulated only. Disabled by default, with a kill switch at global, rail, exception-type and policy scope.
9. **Mock integrations.** Status, reversal and notification mocks run as separate compose services with real-shaped contracts and an explicit `simulated` marker in every response.

## Alternatives considered and rejected

| Option | Reason rejected |
|---|---|
| Python and FastAPI for the whole backend | Matches the playbook default stack, but the owner chose Go. It would also discard the existing Go scaffold, Dockerfile and CI. |
| Microservices from the start | No team-size or scaling reason. More network failure modes in a system where partial failure has financial consequences. Modular boundaries keep extraction possible later. |
| Kafka | Heavier to run for the hackathon. Streams semantics are sufficient. The consumer interface stays abstract so Kafka can replace it. |
| Redis as primary store or lock authority | Loses data on failure modes that PostgreSQL survives. Not acceptable for financial outcomes or idempotency. |
| Publish to Redis directly from the request path | A crash between commit and publish loses events. The outbox removes that gap. |
| Database-only queue | Simpler, but no consumer groups or cheap replay. The outbox keeps the useful part. |
| Temporal-class workflow engine | Operational overhead not justified for the MVP. The case lifecycle is an in-process state machine with persisted transitions. |
| Schema-per-tenant | Harder migrations. Row-level security with isolation tests is preferred. |

## Consequences

- Dependency compatibility with Go 1.27.2 has to be verified and recorded. Any blocker is reported, not worked around silently.
- The existing in-memory packages are prototypes. Auth and audit are replaced in B1, and the demo login is disabled in B0.
- The outbox adds one table, one relay process and a duplicate-delivery obligation on every consumer.
- Ordering is per aggregate only. Consumers must tolerate late and out-of-order events, as the v2 reliability patterns require.
- Items the playbooks leave to the mentors stay open: status-query-first policy details, approval thresholds, and the meaning of "within regulatory limits". They are configuration with a VERIFY flag, not code constants.

## Verification still required

- Compatibility of `chi`, `jwt`, `uuid` and the chosen PostgreSQL and Redis drivers with Go 1.27.2.
- Redis Streams consumer recovery behaviour, covered by integration tests in B2.
