# API Contract Change Log

Every change to a shared contract is recorded here and must be communicated to the frontend and AI owners before it is merged.

## 2026-10-15: `/v1` baseline replaces the v0.1 scaffold

Affects: frontend (`apps/web`). AI service is not affected.

| Topic | v0.1 scaffold | `/v1` baseline |
|---|---|---|
| Base URL | `http://localhost:8080/api/v1` | `http://localhost:8080`, paths start with `/v1` |
| Health | `GET /health` | `GET /v1/health` (liveness) and `GET /v1/ready` (dependencies) |
| Login | `POST /auth/login` | Not available. Authentication arrives in B1. The old handler returned a token for any email and has been disabled. |
| Reversal flow | `POST /reversals/propose`, `POST /reversals/{id}/authorize` | `POST /v1/reversals` runs the eight-check gate, `GET /v1/reversals/{id}` returns three status tracks, `POST /v1/approvals/{id}/decision` records approvals |
| Investigations | `POST /investigations` | `POST /v1/cases`, `PATCH /v1/cases/{id}`, `POST /v1/complaints` |
| Reconciliation | `POST /reconciliation/runs` | Planned for B6. Not in the baseline |
| Errors | Free-form | `{ "error", "message", "trace_id", "details" }` |
| Lists | Bare arrays | `{ "items": [], "next_cursor": "" }` with `limit` and `cursor` |
| Money | Decimal string | `amount_minor` integer plus `currency` (open decision D14) |
| JSON naming | camelCase | snake_case (open decision D15) |
| Trace | `X-Correlation-ID` | `X-Trace-Id` |

Status: implemented operations are `GET /v1/health` and `GET /v1/ready`. Every other operation is marked `x-status: planned` with the phase that implements it. Planned operations describe shape only where the v2 shared contract gives one.

`.env.example` still sets `VITE_API_BASE_URL=http://localhost:8080/api/v1`, which no longer matches. The web app does not read it yet, so it was left for the frontend owner to change together with the types.

Open before the first contract freeze: D4 (named owners), D14 (money), D15 (naming).

Frontend action: `apps/web/src/types/domain.ts` still mirrors v0.1. Nothing breaks today, because the web app does not call the API yet. Types should move to the `/v1` shapes before the first integration.
