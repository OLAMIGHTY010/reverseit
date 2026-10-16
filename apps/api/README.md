# SMARTREVERSAL Backend (Go)

Modular monolith. Four binaries share `internal/` and one PostgreSQL database.

| Binary | Purpose |
|---|---|
| `cmd/api` | HTTP API under `/v1` |
| `cmd/worker` | Outbox relay to Redis Streams, health endpoint |
| `cmd/migrate` | Apply, revert and report migrations |
| `cmd/mock` | Simulated external service (`MOCK_KIND=status\|reversal\|notify`) |

## Packages

| Package | Role |
|---|---|
| `internal/config` | Typed, validated environment configuration |
| `internal/server` | Router, health and readiness, graceful serve |
| `internal/platform/httpx` | Error envelope, JSON decoding, pagination, idempotency key |
| `internal/platform/db` | PostgreSQL pool and transaction helper |
| `internal/platform/redisx` | Redis client |
| `internal/platform/migrate` | Migration runner |
| `internal/outbox` | Transactional outbox and relay |
| `internal/stream` | Idempotent stream consumer, dead letters, replay |
| `internal/worker` | Worker process wiring |
| `internal/mocks` | Track H stubs |
| `internal/observability` | Structured logging, trace ids |
| `internal/contract` | Tests tying code to `packages/api-contracts` |
| `internal/auth` | **Not usable.** Login is disabled. Replaced in B1 |
| `internal/audit` | In-memory prototype, not wired |

See `docs/backend/local-setup.md` to run it and `docs/architecture/c4.md` for the diagrams.
