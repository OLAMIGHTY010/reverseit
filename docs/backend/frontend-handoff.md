# Frontend handoff: exact calls

Base URL in development: `http://localhost:8080/v1` (the API default port). Everything here was exercised against the running API on 15 Oct 2026 and each response was validated against `packages/api-contracts/openapi.yaml`. Money movement is simulated, so responses carry `simulated: true`.

## 1. Running the backend locally

Follow `docs/runbooks/demo-runbook.md`. In short: Postgres 16, Redis 7, migrate, `demo seed`, start the API and the worker, then `demo play`.

**Browser access.** The API sends no CORS headers on purpose. Do not call `localhost:8080` directly from the Vite dev server origin. Use a dev proxy in `apps/web/vite.config.ts` and call relative paths such as `/v1/exceptions`:

```ts
server: { proxy: { '/v1': 'http://localhost:8080' } }
```

## 2. Demo credentials

Nothing secret is stored in the repository. The demo operator password is whatever the person who ran `demo seed` put in the `DEMO_PASSWORD` environment variable (12 or more characters). Ask them for it in person, and never paste it into a doc, ticket or recording.

| Field | Value |
|---|---|
| institution | `DEMO-BANK` |
| officer | `officer@demo.example` (REVERSAL_OFFICER, TXN_ANALYST) |
| supervisor | `supervisor@demo.example` (SUPERVISOR) |
| one-time code | six digits, from `go run ./cmd/demo code officer` or `... code supervisor` (run in `apps/api`), or from an authenticator app using the link `seed` printed |

The authenticator secrets are synthetic and are printed by `seed` on the machine that ran it. Do not commit or record them. Running `seed` again issues new secrets.

## 3. Sign in

```http
POST /v1/auth/login
Content-Type: application/json

{"institution":"DEMO-BANK","email":"supervisor@demo.example","password":"<DEMO_PASSWORD>","otp":"123456"}
```

Response `200`: `{ access_token, token_type: "Bearer", expires_at, user_id, roles, mfa }`. Send `Authorization: Bearer <access_token>` on every other call. `mfa` is `true` only when a valid, unused `otp` was sent. A code can be used once and is valid for about 30 seconds either side. Without `otp` the login works but `mfa` is `false` and approving returns `403 MFA_REQUIRED`. Tokens last 15 minutes by default (`JWT_TOKEN_TTL`). A `401` means sign in again.

`GET /v1/me` returns `{user_id, tenant_id, roles, mfa, expires_at}`.

## 4. Calls by screen

All bodies are JSON. IDs are UUIDs, and anywhere `{id}` appears a case reference like `CASE-2026-000002` also works for cases and a `reversal_ref` for reversals.

| Screen | Request | Notes |
|---|---|---|
| Queue | `GET /v1/exceptions?limit=50` | Query filters in OpenAPI. `next_cursor` for paging. Items have `id, case_ref, classification, status, priority, ...` |
| Case detail | `GET /v1/cases/CASE-2026-000002` | `{case, evidence, history, transaction, notifications, reversals}` |
| Customer view | `GET /v1/cases/{id}/customer-status` | `{stage, headline, outcome_verified, timeline, messages}` |
| Eligibility | `GET /v1/cases/{id}/reversal-eligibility` | `{verdict, checks[{id,name,passed,detail}], ...}`. Verdicts: `PASS_AUTO`, `PASS_NEEDS_APPROVAL`, `BLOCKED` |
| Request reversal | see below | |
| Reversal status | `GET /v1/reversals/{id}` | `tracks.accepted / processing / financial` |
| Renew an expired approval | `POST /v1/reversals/{id}/reapproval` `{"reason":"..."}` | `202` with the reversal back at `approval_status: PENDING`. A different checker with MFA then approves it. See `approval-expiry.md` |
| Refresh a reversal | `POST /v1/reversals/{id}/advance` with `{}` | Safe to repeat |
| Approval queue | `GET /v1/approvals?status=PENDING` | `{items:[{approval_id, case_ref, status, reversal:{...}}]}` |
| Decide | see below | needs `mfa: true` |
| Dashboard | `GET /v1/reports/summary` | Counts by state, status, class; verified value; automation rate |
| Reconciliation | `GET /v1/reconciliation/reversals` | `{items, counts, simulated}` |
| Transactions | `GET /v1/transactions`, `GET /v1/transactions/{id}` | |
| Note | `POST /v1/cases/{id}/notes` `{"note":"text"}` | `201` |
| Evidence | `POST /v1/cases/{id}/evidence` `{"kind":"...","ref":"...","source_system":"...","detail":{}}` | stored as not verified |
| Update case | `PATCH /v1/cases/{id}` with `If-Match: "<version>"` | stale version is `412 STALE_VERSION` |
| Customer complaint | `POST /v1/complaints` `{"instruction_ref":"DEMO-UNK-001","text":"...","contact_ref":"..."}` | `200` joined an existing case, `201` new CUS001 case |

### Request a reversal

```http
POST /v1/reversals
Authorization: Bearer <officer token>
Idempotency-Key: ui-<any unique string per user action>
Content-Type: application/json

{"case_id":"CASE-2026-000002","reason_code":"CONFIRMED_FAILED_CREDIT"}
```

- `202` recorded (waits for an approver), or `200` if this key already made one. Body is the unified `Reversal`: `{id, reversal_ref, case_id, txn_id, amount_minor:"920000", currency, approval_status, approval_id, tracks:{accepted,processing,financial}, gate, simulated:true, ...}`.
- `409 GATE_FAILED`: `{error, message, failed_checks:[{id,name,detail}], gate}`. Normal for unknown outcomes. Nothing was created. Show the failed checks.
- `409 IDEMPOTENCY_KEY_REUSED`, `400` missing key, `403`, `422`.
- Send a new key for a new user action, and reuse the same key when retrying the same click.

### Approve or reject

```http
POST /v1/approvals/<approval_id>/decision
Authorization: Bearer <supervisor token with mfa:true>

{"decision":"approved","reason":"Checked against the statement"}
```

`200` with the `Reversal` after execution and ledger verification (`tracks.financial: "VERIFIED"`). Rejection needs a `reason`. Errors: `403 MFA_REQUIRED`, `403 SELF_APPROVAL`, `403 CONFLICT_OF_INTEREST`, `403 FORBIDDEN` (role or limit), `409` already decided, `409 APPROVAL_POLICY_MISSING` (no approval validity period is configured), `409 APPROVAL_EXPIRED` or `409 APPROVAL_INVALIDATED` (the approval lapsed or the policy changed before submission; renew it). The reversal shows `approval_expires_at`, `policy_id` and, while a kill switch freeze holds it back, `parked_at`.

## 5. The three demo cases after `demo play`

| Case | Class | Status | What the screen should show |
|---|---|---|---|
| `CASE-2026-000001` DEMO-FAIL-001 | FAIL001 | RESOLVED | Reversal `VERIFIED`, customer message "reversed, confirmed", `outcome_verified: true` |
| `CASE-2026-000002` DEMO-DUP-002 | DUP001 | AWAITING_APPROVAL | Pending approval, no money moved, approve with supervisor |
| `CASE-2026-000003` DEMO-UNK-001 | UNK001 | DETECTED | Eligibility `BLOCKED`, reversal request `409 GATE_FAILED`. Held, never reversed |

Case numbers are `000001` to `000003` on a fresh database. After `demo play` is run again on the same database the counter continues, so look cases up by `instruction_ref` or from the queue rather than hard-coding numbers.

## 6. What is real and what is not

See `docs/backend/known-limitations.md`. In particular: no customer sign-in (operators and agents only), simulated status/ledger/provider/notifications, synthetic unverified policy, development-only TOTP, and nothing is production-hardened.

## Review items for unknown outcomes (new)

`GET /v1/review-items` lists open cases whose outcome is not a confirmed failure (unknown, pending, debited only, conflicting). Show them in the approval queue with the label REVIEW. Each item has the evidence, the transaction, the reversal gate's checks, `blocked_because`, `release_requires` and `release_allowed: false`. Disable the Release action for these items. Asking for a reversal anyway returns `409 GATE_FAILED`. The scope document also describes a dual-approver override with a reason and fresh evidence; the backend does not offer it for unknown outcomes. It is the safer reading and a product decision for the bank. Needs the full exception-queue scope, so agents and executives get 403. Additive: the existing `/v1/approvals` response is unchanged.
