# Backend Local Setup

Every command here was run on Linux with Go 1.27.2 and Docker 29. Commands under "Docker Compose" were not run, see the note there.

## Requirements

- Go 1.27.2 (the version in `apps/api/go.mod`)
- Docker, to run PostgreSQL 16 and Redis 7
- `make`, `curl`

## 1. Start PostgreSQL and Redis

Using Docker directly:

```bash
docker run -d --name sr-pg -p 5432:5432 \
  -e POSTGRES_USER=smartreversal_app \
  -e POSTGRES_PASSWORD=development_secret_only \
  -e POSTGRES_DB=smartreversal_db postgres:16-alpine
docker run -d --name sr-redis -p 6379:6379 redis:7-alpine
```

If a port is already taken on your machine, publish a different host port (for example `-p 55433:5432`) and set `DB_PORT` or `REDIS_PORT` in step 2.

### Docker Compose

`compose.yaml` defines postgres, redis, a one-shot migrate step, api, worker and the three mock services. The `docker compose` plugin was not available where this was written, so the file was checked as YAML and its topology was reproduced with plain `docker run` on a private network: migrations applied, api and worker became ready, mocks reported health, and the processes ran as a non-root user. `docker compose up` itself has not been run.

```bash
docker compose up --build
```

## 2. Configure

The Go processes read the process environment, not `.env` files. Load `.env` into your shell:

```bash
cp .env.example .env        # edit ports if needed
set -a; . ./.env; set +a
```

Required: `DB_USER`, `DB_PASSWORD`, `DB_NAME`. The API also needs `JWT_PRIVATE_KEY` once token issuing is wired (see "Signing keys" below). The processes refuse to start without them and list every problem at once. The placeholder values in `.env.example` are accepted only when `ENVIRONMENT=development` or `test`. `FINANCIAL_EXECUTION_ENABLED` defaults to `false` and is refused in production.

### Keys

The API needs three secrets. Generate them once and put them in your shell or `.env`:

```bash
cd apps/api && go run ./cmd/keygen
```

| Variable | Used for |
|---|---|
| `JWT_PRIVATE_KEY` | Signs operator tokens. Belongs to the API process only |
| `ACCOUNT_TOKEN_KEY` | Turns account numbers into tokens. Changing it changes every token |
| `ACCOUNT_ENC_KEY` | Seals account numbers in the vault. Losing it makes sealed accounts unreadable |

Other processes verify tokens with public keys and never need these. To rotate the signing key, give the new key a new `JWT_KEY_ID`, keep the old public key in `JWT_PUBLIC_KEYS` as `kid:base64` until its tokens have expired (`JWT_TOKEN_TTL`, default 15 minutes), then remove it. If the two account keys are missing the API still starts, logs a warning and leaves `POST /v1/accounts/unmask` out. Do not commit keys.

## 3. Migrate

```bash
make db-migrate-status
make db-migrate
make db-migrate-down        # reverts the latest migration
```

Migration files live in `apps/api/migrations` as `NNNN_name.up.sql` and `NNNN_name.down.sql`. Each runs in one transaction. Editing a file after it has been applied is an error, so add a new migration instead.

### First institution and operators

Nothing is seeded. Create an institution and its first operator with `bootstrap`. The password is read from the environment, never from an argument:

```bash
cd apps/api
BOOTSTRAP_PASSWORD='a passphrase of 12 or more characters' \
  go run ./cmd/bootstrap -institution DEMO-BANK -email risk@example.test \
  -roles RISK_COMPLIANCE -seed-clocks
BOOTSTRAP_PASSWORD='another passphrase here' \
  go run ./cmd/bootstrap -institution DEMO-BANK -email sup@example.test -roles SUPERVISOR
```

Roles are `TXN_ANALYST`, `RECON_ANALYST`, `REVERSAL_OFFICER`, `SUPERVISOR`, `HEAD_OPS`, `RISK_COMPLIANCE`, `ADMIN_INTEGRATION`, `EXECUTIVE` and `AGENT`. `-seed-clocks` proposes the seven candidate regulatory clocks from the v2 pack as the operator it creates. They stay **pending** until a different person with the `RISK_COMPLIANCE` role approves each one, and approving needs an MFA-backed token (see below).

## 4. Run

```bash
make dev-api       # http://localhost:8080
make dev-worker    # http://localhost:8081
curl localhost:8080/v1/health
curl localhost:8080/v1/ready
```

Log in and call something:

```bash
TOKEN=$(curl -s -X POST localhost:8080/v1/auth/login \
  -d '{"institution":"DEMO-BANK","email":"risk@example.test","password":"a passphrase of 12 or more characters"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_token"])')
curl -s -H "Authorization: Bearer $TOKEN" localhost:8080/v1/me
curl -s -H "Authorization: Bearer $TOKEN" 'localhost:8080/v1/audit?verify=true&limit=5'
```

Tokens last 15 minutes. **Tokens from login have no MFA**, so actions that need it (approving a configuration change or a reversal, unmasking an account, lifting a kill-switch freeze) answer `403 MFA_REQUIRED` until an MFA mechanism is chosen. See `auth-and-permissions.md`.

`/v1/ready` reports `postgres` for the API and `postgres` plus `redis` for the worker. A failing dependency returns 503 and only says `failed`, never the underlying error.

## 5. Test

Unit tests:

```bash
make test-api
```

Integration tests need a disposable PostgreSQL database and a Redis. They **skip** when these variables are unset, so a green run without them proves less. Use a database that is not your development one:

```bash
docker exec sr-pg createdb -U smartreversal_app smartreversal_test
export TEST_DATABASE_URL='postgres://smartreversal_app:development_secret_only@localhost:5432/smartreversal_test?sslmode=disable'
export TEST_REDIS_ADDR=localhost:6379
(set -a; . ./.env; set +a; DB_NAME=smartreversal_test make db-migrate)
make test-api-integration
```

`make test-api-integration` runs packages one at a time (`-p 1`) because they share the database. CI fails if any test is skipped.

Other checks that CI runs:

```bash
make lint-api                                            # gofmt and go vet
cd apps/api && go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
docker run --rm -v "$PWD:/repo" zricethezav/gitleaks:v8.30.1 \
  git /repo --config /repo/.gitleaks.toml --redact --no-banner
```

## What is not here yet

Ingestion and the simulator (B2), detection (B3), cases and the team model (B4), reversal and approvals (B5), reconciliation (B6), notifications (B7). The mock services answer health checks only and return `501` on their interface routes. No MFA mechanism exists. Delegation, on-call and escalation are a service layer with no HTTP routes yet.

Integration tests create their own institutions and clean up after themselves, but run them against a disposable database: they drop tables of their own and, as a superuser, bypass the audit trail's delete protection to tidy up.
