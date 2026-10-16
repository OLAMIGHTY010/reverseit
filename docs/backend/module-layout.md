# Proposed Backend Module Layout

Status: proposal. The Go stack is approved (ADR 0002). The package list is a starting point and may change as phases land.

## Runtime processes

| Process | Entry point | Role |
|---|---|---|
| API | `cmd/api` | HTTP server under `/v1` |
| Worker | `cmd/worker` | Stream consumers, schedulers, notification dispatch, verification jobs |
| Migrator | `cmd/migrate` | Applies and reports schema migrations |
| Simulator | `cmd/simulator` | Deterministic synthetic traffic and scenario replay (non-production builds only) |

All four share `internal/` packages and one PostgreSQL database. PostgreSQL is the system of record. They exchange events through Redis Streams fed by the transactional outbox. The mock status, reversal and notification services run as separate compose services and are reached only through their interface contracts.

## Packages under `apps/api/internal`

| Package | Responsibility | Exists today |
|---|---|---|
| `platform/config` | Typed environment config, fail on missing secrets | yes (`config`) |
| `platform/db` | Pool, transactions, tenant-scoped query helpers | no |
| `platform/httpx` | Router, error envelope, pagination, request validation | no |
| `platform/observability` | Structured logs, metrics, trace IDs | yes |
| `identity` | JWT verification, roles, permissions, service credentials, customer identity | yes (`auth`, to be replaced) |
| `tenancy` | Tenant context and isolation enforcement | no |
| `audit` | Append-only hash-chained store | yes, in memory |
| `policy` | Versioned configuration, thresholds, limits, business calendar | no |
| `ingest` | Canonical event, validation, normalisation, dedup, replay | partial (`transactions`) |
| `outbox` | Transactional outbox table, relay to Redis Streams, consumer helpers with retry and dead-letter | no |
| `channels` | Per-rail adapters behind one interface | no (`integrations` empty) |
| `detect` | Deterministic detectors and rule versions | partial (`exceptions`) |
| `cases` | Queue, lifecycle, SLA, escalation | no (`investigations` empty) |
| `reversal` | Eligibility gate, locks, approvals, state machine | no (`reversals` empty) |
| `reversal/executor` | The only package allowed to call a reversal provider | no |
| `reconcile` | Settlement ingestion, matching, reports | no (`reconciliation` empty) |
| `incidents`, `notify` | Incidents, templates, providers, delivery state | no |
| `reporting` | Read-only aggregates | no |
| `reverseit` | Customer-facing intake, matching, disputes | no |
| `aiclient` | Typed AI adapter, validation, timeout, rule fallback | no |
| `scenario` | Seeding, reset, simulated clock, replay | no |

## Dependency rules

1. `reversal/executor` is imported only by `reversal`. A lint check in CI enforces this.
2. `aiclient` is imported by `detect`, `cases` and `reverseit`. It is never imported by `reversal`.
3. Every package that touches tenant data takes a tenant-scoped handle from `platform/db`. Raw pool access is limited to `platform/db`, `cmd/migrate` and `audit`.
4. HTTP handlers hold no business logic. They validate, authorise, call a service, and map errors.
5. Reporting reads only. It cannot import writers.

## Database ownership

One schema, with tables prefixed by owning module. Every tenant table carries `tenant_id`, a foreign key, and a row-level security policy so a missed filter in code does not leak data. Money columns are `NUMERIC(18,4)`. Evidence tables have no UPDATE or DELETE grant for the application role.
