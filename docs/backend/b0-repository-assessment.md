# B0 Repository Assessment

Status: draft for review. Date of inspection: 2026-10-15. Baseline: `develop` at `c317be4`.

This is the first B0 deliverable. It records what exists in the repository today so that later backend work builds on the real state and does not overwrite it.

## 1. Scope and source documents

The backend brief names four source documents (Complete Playbook Vol 1 + Vol 2, Vol 2 Phase-by-Phase Production Playbook, v2 Integrated Scope Prompt Pack, Full Project Scope). They are not stored in this repository. The first version of this assessment was written from the backend brief alone. The project owner then supplied the PDFs, and `requirement-traceability.md` has since been reconciled against them. The findings below describe the repository and are unchanged by that reconciliation, except where a later section says otherwise.

## 2. What exists

| Area | Path | State |
|---|---|---|
| Go API skeleton | `apps/api` | Module and package layout present. `cmd/api/main.go` is an empty `main`. No HTTP server starts. |
| Go packages with code | `internal/auth`, `internal/audit`, `internal/transactions`, `internal/exceptions`, `internal/config`, `internal/observability` | 52 to 198 lines each. In-memory only. |
| Go packages that are empty | `internal/integrations`, `internal/investigations`, `internal/reconciliation`, `internal/reversals` | One line each (package clause only). |
| Migrations | `apps/api/migrations` | Empty (`.gitkeep`). No schema exists. |
| AI service | `apps/ai-service` | FastAPI app with `/anomaly` and `/recommendations` routers. Schemas file is a stub. |
| Web console | `apps/web` | React 19, TypeScript, Vite. Feature views for transactions, exceptions, investigations, reconciliation, reversals, audit, dashboard. |
| API contract | `packages/api-contracts/openapi.yaml` | 9 paths, unprefixed (`/transactions`, not `/v1/transactions`). |
| Infra | `compose.yaml`, `infra/docker/*`, `Makefile`, `.env.example` | Postgres 16, Redis 7, api, ai-service, web. |
| CI | `.github/workflows/ci.yml` | Go, Python and Node jobs. |
| Docs | `docs/adr/0001`, `docs/architecture/system-overview.md` | Both are one-line stubs. Maker-checker, threat model and DR runbook are real documents. |

## 3. Findings that affect the plan

### 3.1 The repository already chose Go, the brief proposes Python

`implement.md` and `apps/api/README.md` define `apps/api` as Go with Chi, and `apps/ai-service` as Python with FastAPI. The brief proposes Python and FastAPI for the backend and also says to follow existing repository conventions. These conflict. This was decision D1. The project owner approved keeping Go on 2026-10-15 (ADR 0002).

### 3.2 The Go module does not build

`go build ./...` and `go vet ./...` fail with `missing go.sum entry` for `github.com/google/uuid` and `github.com/golang-jwt/jwt/v5`. `go.sum` is not committed. The CI job `go mod verify` and `go vet` would fail on the same cause. CI also pins Go 1.22 while `go.mod` declares 1.23.0 and `CONTRIBUTING.md` asks for 1.24.

### 3.3 Existing auth is a development stub and is unsafe to keep

`internal/auth/auth.go`:

- `HandleLogin` ignores the password and issues a 24 hour token for any email. Unknown emails get the analyst role.
- Signing is HS256 with a shared secret taken from `JWT_SECRET`. The `.env.example` default is a readable placeholder string.
- Roles are `ANALYST_MAKER`, `SUPERVISOR_CHECKER`, `AUDITOR`, `ADMIN`. `RequireRoles` lets `ADMIN` pass every check, which is incompatible with maker-checker separation.
- No tenant claim, no audience, no token revocation, no service-to-service identity, no customer identity for ReverseIt.
- Error bodies are hand-written JSON strings with no common error shape.

This code must be replaced in B1, not extended.

### 3.4 Audit trail is in memory

`internal/audit` hash-chains events correctly in principle, but state is a Go slice behind a mutex and the chain restarts at a fixed genesis string on every boot. It is not append-only at the storage layer and does not survive restart. It is also not tenant-scoped.

### 3.5 Transaction model is narrower than the brief

`internal/transactions` supports channels `NIP`, `POS`, `CARD`, `WEBHOOK`, `DIRECT_DEBIT`. The brief requires NIP, POS, ATM, card, mobile, internet banking and USSD. Statuses are `SUCCESS`, `PENDING`, `FAILED`, `UNCERTAIN`, `REVERSED`. The brief requires five outcome classes (confirmed success, confirmed failure, pending, unknown, conflicting evidence), so `CONFLICTING` is missing and `REVERSED` mixes outcome with remediation state. Amounts are strings, which is acceptable if parsed as decimal, and the schema should use `NUMERIC(18,4)` as the PR template already requires.

### 3.6 Contract gaps against the brief

- No `/v1` prefix.
- No pagination, filtering or error envelope definitions.
- No endpoints for cases, approvals, incidents, notifications, ReverseIt, reporting, scenario control or AI integration.
- Frontend `apps/web/src/types/domain.ts` is the de facto consumer contract today and has not been compared with the OpenAPI file yet.

### 3.7 Governance already in place

`CONTRIBUTING.md` requires conventional commits, feature branches from `develop`, PRs into `develop`, and one logical unit per commit. This matches the brief. The PR template has a financial integrity checklist that the backend PRs should keep filling in.

## 4. What B0 must leave behind

1. A stack decision recorded as an ADR (done, ADR 0002).
2. A building, starting, health-checked API process with a database connection and a migration runner.
3. A worker entry point separate from the API process.
4. `/v1` OpenAPI baseline with the shared error, pagination and auth conventions.
5. CI that passes on the skeleton.
6. A risk and assumption register.

Nothing in B1 to B11 starts until these pass.
