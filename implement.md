# SMARTREVERSAL Monorepo Implementation & Scaffolding Plan

## 1. System & Environment Inspection Summary
- **Current Directory**: `c:\Users\user\Desktop\Clietn_Project\SMARTREVERSAL`
- **Git Status**: Empty repository initialized on branch `main` with 0 commits.
- **Operating System**: Windows (AMD64)
- **Shell**: PowerShell (`pwsh`)
- **Installed Toolchain**:
  - **Go**: `1.27.2` (target; originally scaffolded on 1.26.3)
  - **Node.js**: `v26.5.0`
  - **npm**: `12.0.2`
  - **Python**: `3.13.3`
  - **Docker**: `29.7.2`
  - **Docker Compose**: `v5.4.0`

---

## 2. Architecture & Monorepo Design Principles
SMARTREVERSAL is an **enterprise financial operations platform** specifically targeted at bank, fintech, and PSP operations for transaction investigation, reconciliation, exception management, and controlled financial transaction resolution.

### Core Boundaries & Tenets
1. **Not a Core Banking System or Consumer Wallet**: SMARTREVERSAL integrates via strict adapters into existing ledgers, payment switches (NIP, POS, Card, Webhook gateways), and settlement reports.
2. **Financial Integrity & Maker-Checker**: No financial resolution (e.g., reversal dispatch, manual journal entry, adjustment) occurs without immutable audit logging, state validation, and dual-control approval workflows.
3. **Idempotency & Eventual Consistency**: All resolution actions and webhook receptions guarantee idempotency using cryptographic deduplication and unique idempotency keys.
4. **Isolated Service runtimes**:
   - `apps/api`: Go + Chi HTTP router (high throughput, low latency, robust concurrency, database migrations, financial transactions, audit).
   - `apps/ai-service`: Python + FastAPI (anomaly detection, root-cause pattern analysis, automated exception clustering, explainable remediation recommendations).
   - `apps/web`: React 19 + TypeScript + Vite + custom design system (desk-based financial analyst and compliance operator workflow).
   - `packages/api-contracts`: Single source of truth OpenAPI 3.1 specification for contracts across all tiers.
   - `packages/ui`: Shared typed design tokens and accessible React components.

---

## 3. Phased Execution Roadmap

### Phase 1: Repository Governance, Security & Monorepo Root Setup
- Root configuration files:
  - `.editorconfig` (consistent indentation across Go, TS, Py)
  - `.gitignore` (comprehensive ignore rules for Go, Node, Python, Docker, OS)
  - `.dockerignore`
  - `LICENSE` (Apache 2.0 / Enterprise Financial Open Core)
  - `SECURITY.md` (Vulnerability disclosure, financial data handling compliance, branch protection rules for `main`)
  - `CODE_OF_CONDUCT.md`
  - `CONTRIBUTING.md` (Branching model: `main` locked, PRs target `develop`, conventional commits, single-commit workflow)
  - `.github/workflows/ci.yml` (Lint, test, type-check, security audit)
  - `.github/workflows/branch-protection-rules.yml`
  - `.github/pull_request_template.md`
  - `.github/ISSUE_TEMPLATE/bug_report.md` & `feature_request.md`
  - `package.json` (npm workspaces root managing `apps/web`, `packages/ui`, `packages/api-contracts`)
  - `.env.example` (PostgreSQL, Redis, Go API, AI Service, Auth secrets)
  - `compose.yaml` (Postgres 16, Redis 7, API, AI Service, Web)
  - `Makefile` (Developer ergonomics: build, test, lint, run, migrate)
  - Root `README.md` (Following Microverse README standard with financial stakeholders in mind)

### Phase 2: Packages (Shared Contracts & UI)
- `packages/api-contracts`:
  - `package.json`
  - `openapi.yaml` (Complete enterprise API definition covering auth, transactions, reconciliation, exceptions, investigations, reversals, and audit)
  - `README.md`
- `packages/ui`:
  - `package.json`
  - `tsconfig.json`
  - Design tokens, button, badge, status pill, table, metric card components
  - `README.md`

### Phase 3: Backend Core (`apps/api` - Go + Chi)
- Structure:
  - `go.mod` (module `github.com/smartreversal/smartreversal/apps/api`)
  - `cmd/api/main.go` (Graceful shutdown, environment configuration, Chi router, middleware chain)
  - `internal/config/config.go`
  - `internal/observability/` (Structured logging with zerolog/slog, correlation ID, metrics, healthchecks)
  - `internal/auth/` (JWT verification, role-based access control, maker-checker roles: `analyst`, `approver`, `admin`)
  - `internal/transactions/` (Model, repository, handler for transaction ingestion & querying)
  - `internal/reconciliation/` (Two-way and three-way reconciliation engine contracts)
  - `internal/exceptions/` (Exception classification: debit without credit, duplicate debit, status mismatch, suspense balances)
  - `internal/investigations/` (Investigation case management, evidence timeline, notes)
  - `internal/reversals/` (Idempotent reversal workflows, dual-control approval gate, dispatch adapters)
  - `internal/audit/` (Tamper-evident audit trail)
  - `internal/integrations/` (NIP, POS, Card, Webhook mock adapters)
  - `migrations/` (Initial PostgreSQL DDL with proper UUIDs, precision `NUMERIC(18,4)`, indexes, audit triggers)
  - `README.md`

### Phase 4: AI & ML Service (`apps/ai-service` - Python + FastAPI)
- Structure:
  - `pyproject.toml`
  - `requirements.txt`
  - `app/main.py` (FastAPI app, health endpoints, CORS, lifespan)
  - `app/core/config.py`
  - `app/api/v1/endpoints/anomaly.py` (Transaction anomaly detection)
  - `app/api/v1/endpoints/recommendations.py` (Root-cause analysis and automated resolution suggestions)
  - `app/models/` (Pydantic schemas aligning with API contracts)
  - `app/services/ml_engine.py` (Rule-heuristic + vector/statistical anomaly clustering)
  - `tests/test_api.py`
  - `README.md`

### Phase 5: Frontend Operations Console (`apps/web` - React + TypeScript + Vite)
- Structure:
  - `package.json`, `tsconfig.json`, `vite.config.ts`
  - `index.html`, `src/main.tsx`, `src/App.tsx`
  - `src/theme/` (Premium financial UI tokens, high contrast, dark/light modes)
  - `src/components/` (Navigation, layout, header, maker-checker modal, status badges)
  - `src/features/`:
    - `dashboard` (Executive exception metrics, pending approvals, reconciliation rate)
    - `transactions` (Search, filter, deep-dive timeline)
    - `reconciliation` (Discrepancy matrix, batch file upload)
    - `investigations` (Case tracker, evidence trail, AI diagnosis panel)
    - `reversals` (Maker-checker dual approval queue, idempotency key inspector)
    - `audit` (Immutable audit log viewer)
  - `src/services/apiClient.ts`
  - `README.md`

### Phase 6: Infrastructure, Documentation & Tests
- `infra/docker/`:
  - `Dockerfile.api` (multi-stage Go scratch/alpine build)
  - `Dockerfile.ai` (slim Python container)
  - `Dockerfile.web` (Node build + Nginx static server)
  - `init-db.sql`
- `infra/scripts/`:
  - `seed-data.sh` / `seed-data.ps1`
  - `healthcheck.sh`
- `tests/fixtures/`: Sample settlement files (NIP, Card POS, Webhook batches)
- `docs/`:
  - `docs/architecture/system-overview.md`
  - `docs/financial-integrity/maker-checker-protocol.md`
  - `docs/security/threat-model.md`
  - `docs/adr/0001-monorepo-structure-and-tech-stack.md`
  - `docs/runbooks/disaster-recovery.md`

### Phase 7: Branch Management & Commit Strategy
- **Requirement**: Commit each file singly and not by merge committing!
- **Branch Strategy**:
  1. Commit foundational governance and setup files directly to `main` one by one with clean semantic commits (`feat: ...`, `docs: ...`, `chore: ...`).
  2. Create and switch to `develop` branch.
  3. Ensure documentation and workflows enforce that `main` is protected (signed commits, required reviews, passing CI, no direct force push).
  4. Continue developing and committing all scaffold files individually on `develop`.
