<p align="center">
  <img src="docs/assets/logo.png" alt="SMARTREVERSAL Logo" width="130" height="130" />
</p>

<h1 align="center">SMARTREVERSAL</h1>

<p align="center">
  <strong>AI-Powered Transaction Investigation, Reconciliation, Exception Management, and Controlled Financial Transaction Resolution Platform</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Status-Active_Development-brightgreen?style=flat-square" alt="Status" />
  <img src="https://img.shields.io/badge/AI_Tests-58%2F58_Passing-blue?style=flat-square" alt="AI Tests" />
  <img src="https://img.shields.io/badge/Maker--Checker-Enforced-indigo?style=flat-square" alt="Maker Checker" />
  <img src="https://img.shields.io/badge/License-Proprietary-red?style=flat-square" alt="License" />
</p>

SMARTREVERSAL is an enterprise financial operations platform built for banks, fintech institutions, payment service providers (PSPs), and payment switches to automate the detection, investigation, reconciliation, and controlled remediation of transactional anomalies, settlement discrepancies, and payment exceptions.

---

## 📑 Table of Contents

- [About the Project](#about-the-project)
  - [Product Vision & System Boundaries](#product-vision--system-boundaries)
  - [Key Operational Scenarios](#key-operational-scenarios)
  - [Core Features](#core-features)
  - [Monorepo Architecture](#monorepo-architecture)
  - [Built With](#built-with)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Setup & Configuration](#setup--configuration)
  - [Running with Docker Compose](#running-with-docker-compose)
  - [Local Service Development](#local-service-development)
  - [Running Tests](#running-tests)
- [Financial Integrity & Security Safeguards](#financial-integrity--security-safeguards)
  - [Dual-Control / Maker-Checker Workflows](#dual-control--maker-checker-workflows)
  - [Idempotent Resolution Engine](#idempotent-resolution-engine)
  - [Tamper-Evident Audit Logging](#tamper-evident-audit-logging)
- [Roadmap & Planned Capabilities](#roadmap--planned-capabilities)
- [Branching & Release Governance](#branching--release-governance)
- [License](#license)
- [Contact & Support](#contact--support)

---

## 📖 About the Project

### Product Vision & System Boundaries

SMARTREVERSAL is engineered specifically to eliminate blind spots in payment processing and financial operations. 

- **What it IS**: An intelligent orchestration and investigation layer that integrates with core banking systems, settlement networks, switch logs, and payment gateways to identify discrepancies, track aged suspense balances, cluster failure patterns, and coordinate authorized financial resolutions.
- **What it is NOT**: SMARTREVERSAL is **not** a retail consumer banking application, a digital wallet, or an independent settlement rail. It operates as an authorized operational overlay communicating via secure APIs and adapters.

### Key Operational Scenarios

SMARTREVERSAL addresses the mission-critical edge cases of modern electronic payment operations:

1. **Debit Without Credit (DWC)**: Customer account debited on sending institution, but beneficiary never credited due to downstream switch or gateway failure.
2. **Duplicate Debits / Credits**: Repeated processing triggered by retries, network glitches, or misconfigured retry schedules.
3. **Pending / Delayed / Uncertain States**: Indeterminate statuses spanning multiple ledger settlement windows.
4. **Duplicate Webhook & Payment Events**: Repeated webhook callbacks causing erroneous secondary ledger entries.
5. **Reconciliation Discrepancies**: Mismatches between internal transaction ledgers, third-party processor logs (NIP, POS, Card rails), and clearing reports.
6. **Suspense Balance Tracking**: Unreconciled differences and aged suspense account items requiring audit-backed resolution.
7. **Failed or Delayed Reversals**: Reversal requests that were initiated but timed out, rejected, or partially posted.

### Core Features

- **Automated Discrepancy Matching**: 2-way and 3-way multi-source reconciliation matrix.
- **AI-Driven Root Cause Diagnosis**: Machine learning analysis to cluster recurring switch error patterns and recommend remediation steps with clear explanations.
- **Controlled Reversal Execution**: Automated or human-in-the-loop dispatch of reversal instructions through authenticated adapter rails.
- **Maker-Checker Enforcement**: Dual-authorization requirement preventing unilateral adjustments or high-value reversals.
- **Immutable Audit Trail**: Cryptographically traceable event log recording all actions, timestamps, and user identities.

---

### Monorepo Architecture

```text
smartreversal/
├── apps/
│   ├── api/             # High-performance Go + Chi backend financial transaction engine
│   ├── ai-service/      # Python + FastAPI machine learning & anomaly clustering service
│   └── web/             # Enterprise operations console in React 19, TypeScript & Vite
├── packages/
│   ├── api-contracts/   # OpenAPI 3.1 single source of truth schemas
│   └── ui/              # Shared design tokens & accessible financial UI components
├── infra/
│   ├── docker/          # Multi-stage production and development Dockerfiles
│   └── scripts/         # Environment bootstrapping and database seeding scripts
├── tests/
│   ├── integration/     # Cross-service integration test suites
│   ├── e2e/             # End-to-end browser workflows
│   └── fixtures/        # Mock settlement files (NIP, POS, Card reports)
├── docs/                # Architecture design records (ADR), security models, runbooks
├── compose.yaml         # Complete local multi-container composition
└── Makefile             # Developer ergonomics
```

---

### Built With

- **Backend**: [Go (1.27.2)](https://go.dev/) with [Chi](https://github.com/go-chi/chi) HTTP routing.
- **AI & Analytics**: [Python (3.13+)](https://www.python.org/) with [FastAPI](https://fastapi.tiangolo.com/), Pydantic, and Scikit-Learn.
- **Frontend Console**: [React 19](https://react.dev/), [TypeScript](https://www.typescriptlang.org/), [Vite](https://vitejs.dev/).
- **Database & Cache**: [PostgreSQL 16](https://www.postgresql.org/) and [Redis 7](https://redis.io/).
- **Containerization**: [Docker](https://www.docker.com/) & Docker Compose.

---

## 🚀 Getting Started

### Prerequisites

Ensure you have the following installed on your host system:
- **Git** (2.30+)
- **Go** (1.27.2)
- **Node.js** (v20+) & **npm** (v10+)
- **Python** (3.11+)
- **Docker Engine** & **Docker Compose**

### Setup & Configuration

1. Clone the private repository:
   ```bash
   git clone git@github.com:smartreversal/smartreversal.git
   cd smartreversal
   ```

2. Copy the environment variables:
   ```bash
   cp .env.example .env
   ```

3. Review `.env` and configure credentials suitable for your local development environment.

### Running with Docker Compose

To launch the complete infrastructure (Postgres, Redis, Go API, Python AI Service, and Web Console):

```bash
docker compose up --build -d
```

- **Web Operations Console**: `http://localhost:3000`
- **Core API Gateway**: `http://localhost:8080` (Healthcheck: `http://localhost:8080/health`)
- **AI Analytics Service**: `http://localhost:8000` (API Docs: `http://localhost:8000/docs`)
- **PostgreSQL**: `localhost:5432`
- **Redis**: `localhost:6379`

### Local Service Development

Alternatively, services can be executed natively for faster feedback:

```bash
# 1. Install frontend and shared dependencies
npm install

# 2. Run Go API
cd apps/api
go run cmd/api/main.go

# 3. Run AI Service (in a virtualenv)
cd apps/ai-service
python -m venv .venv
# activate virtualenv: .venv\Scripts\activate (Windows) or source .venv/bin/activate (Linux/Mac)
pip install -r requirements.txt
uvicorn app.main:app --reload --port 8000

# 4. Run Web Console
npm run dev:web
```

### Running Tests

```bash
# Backend unit tests
cd apps/api && go test -v -race ./...

# AI service unit tests
cd apps/ai-service && pytest

# Frontend type checking
npm run typecheck
```

---

## 🛡️ Financial Integrity & Security Safeguards

### Dual-Control / Maker-Checker Workflows
Financial remediation actions (such as initiating automated reversals or posting manual adjustment vouchers) cannot be dispatched unilaterally. An **Analyst (Maker)** proposes the action with evidence, and an **Approver (Checker)** verifies and authorizes the execution.

### Idempotent Resolution Engine
Every outgoing resolution instruction generates a unique cryptographic idempotency key:
$$\text{IdempotencyKey} = \text{HMAC-SHA256}(\text{TransactionRef} + \text{DiscrepancyType} + \text{ApprovedTimestamp})$$
This guarantees that repeated network dispatches cannot trigger multiple credits or duplicate reversals.

### Tamper-Evident Audit Logging
Every user intervention, automated scan result, status transition, and system dispatch is written to an append-only audit trail with correlation IDs, timestamps, and operator identity.

---

## 🧭 Branching & Release Governance

This repository adheres to strict release protection:
- `main`: **Protected branch**. Production releases only. Direct commits and force pushes are blocked.
- `develop`: Primary integration branch for active development.
- `feature/*` & `fix/*`: Feature branches merged into `develop` via peer-reviewed Pull Requests.

---

## 🔒 License

This repository is **PROPRIETARY and CONFIDENTIAL**.
Copyright &copy; 2026 SMARTREVERSAL. All rights reserved.

---

## 💬 Contact & Support

For operational access, integration inquiries, or internal support:
- **Engineering Desk**: `engineering@smartreversal.internal`
- **Security & Financial Integrity**: `security@smartreversal.internal`
