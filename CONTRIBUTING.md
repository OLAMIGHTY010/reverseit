# Contributing to SMARTREVERSAL

First of all, thank you for taking the time to contribute! SMARTREVERSAL is an enterprise financial operations platform dedicated to transaction investigation, reconciliation, and controlled reversal resolutions. Ensuring system reliability, audit integrity, and regulatory compliance is paramount.

---

## 1. Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md) and security handling protocols outlined in [SECURITY.md](SECURITY.md).

---

## 2. Branching Strategy & Governance

SMARTREVERSAL strictly adheres to an enterprise trunk-based branching model with protected baseline branches:

1. **`main` (Production Baseline)**:
   - Locked and protected. Direct commits and force pushes are strictly disabled.
   - Code enters `main` only via release pull requests from `develop` with mandatory architectural and dual-control sign-offs.
2. **`develop` (Integration Baseline)**:
   - Primary active integration branch.
   - All feature branches and bugfix branches must branch from `develop` and target `develop` for PR merges.
3. **Feature Branches (`feat/<feature-name>`) / Fix Branches (`fix/<bug-name>`)**:
   - Short-lived working branches.
   - Must be rebased on latest `develop` before submitting a Pull Request.

---

## 3. Commit Discipline & Granularity

We enforce a **single-responsibility, semantic commit workflow**:
- Every distinct file or atomic unit of functionality should be committed with a descriptive message.
- Follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:
  - `feat(...)`: A new user-facing or platform capability.
  - `fix(...)`: A bug fix.
  - `docs(...)`: Documentation only changes.
  - `chore(...)`: Tooling, build config, dependency bumps.
  - `refactor(...)`: Code change that neither fixes a bug nor adds a feature.
  - `test(...)`: Adding missing tests or correcting existing tests.
  - `ci(...)`: GitHub Actions or CI pipeline modifications.

**Example Commit Message**:
```text
feat(reversals): add dual-control maker-checker approval gate
```

---

## 4. Development Workflow

### Prerequisites
- Go `1.27.2` (the version declared in `apps/api/go.mod`)
- Node.js `20+` & npm `10+`
- Python `3.12+`
- Docker & Docker Compose `v2+`

### Setting Up Your Environment
```bash
# Clone the repository
git clone https://github.com/smartreversal/smartreversal.git
cd SMARTREVERSAL

# Copy the environment file
cp .env.example .env

# Install Node dependencies
npm install

# Start local dependencies (Postgres, Redis)
docker compose up -d postgres redis
```

---

## 5. Financial Integrity Rules

When writing code affecting transaction states, reconciliation, or reversals:
1. **Never Bypass Idempotency**: All resolution requests must include and enforce an `Idempotency-Key`.
2. **Dual-Control (Maker-Checker)**: An analyst who initiates a reversal or journal entry cannot be the user who approves it.
3. **Immutable Audit Logging**: Every operational change must emit a tamper-evident audit log entry recording actor ID, timestamp, prior state, new state, and client metadata.
4. **Deterministic Math**: Use fixed-precision `NUMERIC(18, 4)` for financial amounts; avoid floating-point math.

---

## 6. Pull Request Checklist

Before submitting a Pull Request:
- [ ] Code compiles and passes all unit and integration tests (`make test`).
- [ ] Linters and formatting checks pass (`make lint`).
- [ ] Contract changes are reflected in `packages/api-contracts/openapi.yaml`.
- [ ] Migration files include matching rollback/down migrations where applicable.
- [ ] No secrets or test credentials are committed.
