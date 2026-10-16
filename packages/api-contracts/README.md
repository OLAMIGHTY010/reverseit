# @smartreversal/api-contracts

This package contains the canonical OpenAPI 3.1 contract specification for the SMARTREVERSAL platform.

## Purpose & Scope
- Serves as the single source of truth for all API interactions across:
  - `apps/api` (Go backend routing and handlers)
  - `apps/ai-service` (FastAPI models and analytical inputs)
  - `apps/web` (React TypeScript API client queries)
- Enforces consistent data types, financial representations (`NUMERIC` string precision), error models, and dual-control reversal requests.
