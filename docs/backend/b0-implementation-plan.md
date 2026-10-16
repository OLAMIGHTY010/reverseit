# B0 Implementation Sequence

Status: executed. The table below is the plan as approved. What was actually delivered, including the differences, is in `b0-report.md`. Code landed on `feat/backend-b0-foundation`, separate from the documentation PR.

Items are ordered by dependency. Rows marked (needs Docker) or (needs services) need Postgres or Redis running and say so in the verification result if they were not run.

| # | Commit | Verification |
|---|---|---|
| 1 | `chore(api): standardise on Go 1.27.2 and commit go.sum` | `go mod verify`, `go build ./...`, `go vet ./...` |
| 2 | `fix(auth): disable demo login and fail closed on admin bypass` | Tests: login returns 501, no token issued, admin does not pass role checks it is not granted |
| 3 | `ci(api): align Go version and run build, vet and tests` | Workflow lint, local equivalent of each step |
| 4 | `feat(config): typed config with required-secret validation` | Unit tests for missing and weak secrets |
| 5 | `feat(api): http server with graceful shutdown and /v1/health` | Server test starts, calls health, stops |
| 6 | `feat(api): error envelope, trace id and request logging middleware` | Handler tests |
| 7 | `feat(db): postgres pool and readiness check` (needs services) | Integration test |
| 8 | `feat(db): migration runner and baseline migration` (needs services) | Up, down, up on an empty database |
| 9 | `feat(outbox): outbox table and transactional enqueue` (needs services) | Rollback leaves no outbox row, commit leaves one |
| 10 | `feat(outbox): redis streams relay with consumer groups and dead-letter` (needs services) | Duplicate publish, crash recovery, poison message tests |
| 11 | `feat(worker): worker entry point with readiness and signal handling` | Starts, reports ready, exits on signal |
| 12 | `feat(api): pagination and idempotency-key helpers` | Unit tests |
| 13 | `docs(api): move OpenAPI to /v1 with shared components` | Spec lint |
| 14 | `docs(api): OpenAPI files for the six external interfaces` | Spec lint |
| 15 | `test(api): contract test that routes match OpenAPI` | Fails when a route has no spec entry |
| 16 | `feat(mocks): health-only mock status, reversal and notification services` | Health tests. Labelled simulated. No behaviour claimed |
| 17 | `chore(compose): services, healthchecks and profiles` (needs Docker) | `docker compose config`, and up if Docker Compose is available |
| 18 | `docs(architecture): C4 context and container diagrams` | Mermaid renders |
| 19 | `docs(backend): local setup, migrations and testing guide` | Commands in the guide were run |
| 20 | `ci(api): add secret scan and dependency vulnerability scan` | Tool runs locally if available, else reported as not run |

Not in B0: real authentication (B1), any tenant table, any domain table beyond migration bookkeeping and the outbox, detectors, reversal logic, mock behaviour.

## Exit checks for B0

1. `go build ./...`, `go vet ./...`, `go test ./...` pass locally and in CI.
2. API and worker start, and `/v1/health` and `/v1/ready` respond.
3. Migration up and down work on an empty database.
4. Outbox enqueue and relay tests pass against real Postgres and Redis.
5. OpenAPI lints and the route-versus-spec test passes.
6. The demo login cannot issue a token.
7. Risk register and decision list have no unanswered B0 blocker.

An exit check that could not be run in the available environment is reported as not run, and B0 is not declared complete on it.
