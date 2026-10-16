# Shared API Contracts (B0 inventory)

Status: draft, reconciled with the v2 shared contract on 2026-10-15. This lists the contracts that the frontend and AI developers will depend on. Nothing here is final until D4, D14 and D15 in `decisions-requiring-approval.md` are answered. Changes to a published contract must be written down and sent to the affected developer.

## 1. Conventions (aligned with the v2 shared contract, section 7)

Items marked "open" depend on decisions D14, D15 and D4 and are not frozen.

| Topic | Convention |
|---|---|
| Base path | `/v1` |
| Auth | `Authorization: Bearer <JWT>` for operators and customers. Separate service credential for service-to-service calls (B1). |
| Tenant | Taken from the verified token, never from a request body or query parameter. The `institution_id` in an event body must match the token. |
| Error shape | `{ "error": "GATE_FAILED", "message": "...", "trace_id": "...", "details": [...] }`. The reversal gate adds `failed_checks: [{ "id": 5, "detail": "..." }]`. |
| Pagination | Cursor based: `limit` (default 50, max 200) and `cursor`. Response has `items` and `next_cursor`. |
| Concurrency | `If-Match` version on case updates. Transition validated, reason code required. |
| Idempotency | `Idempotency-Key` required on `POST /events`, `POST /cases`, `POST /reversals`. Same key and same payload returns the original result. Same key and different payload returns 409. |
| Money | `amount_minor` integer plus ISO 4217 `currency` (open, D14). Never a float. |
| JSON naming | snake_case (open, D15). |
| Time | RFC 3339 with offset. Stored UTC. Business calendar in WAT. |
| Simulated data | Every simulated response carries `"simulated": true`. |
| Trace | `X-Trace-Id` accepted and echoed, also logged. |
| Status codes | 202 accepted for queued work, 200 for duplicate event, 409 conflict, 422 schema error (event goes to dead-letter with reason), 429 rate limited, 401 or 403 for auth. |

## 2. Frontend-facing resources

| Resource | Phase | Existing in OpenAPI |
|---|---|---|
| Session and identity (`/v1/auth/*`, `/v1/me`) | B1 | `/auth/login` only. Disabled in B0 |
| Event ingest `POST /v1/events` | B2 | none |
| Transactions (search, detail with timeline, evidence, source freshness) | B2 | `/transactions` list |
| Exceptions (queue, detail, classification) | B3 | `/exceptions` list |
| Cases and complaints (`/v1/cases`, `PATCH` with `If-Match`, `/v1/complaints`) | B4 | `/investigations` create |
| Reversals (`POST /v1/reversals`, `GET /v1/reversals/{id}` with three tracks) and approvals (`POST /v1/approvals/{id}/decision`) | B5 | `/reversals/propose`, `/reversals/{id}/authorize` (to be replaced) |
| Kill switch `POST /v1/killswitch` | B1, B5 | none |
| Reconciliation (runs, mismatches, reports) | B6 | `/reconciliation/runs` |
| Incidents and notifications | B7 | none |
| Reporting and dashboards | B8 | none |
| Audit log | B1 | `/audit-logs` |
| ReverseIt customer endpoints | B10 | none |
| Scenario and demo controls | B11 | none |
| Health (`/v1/health`, `/v1/ready`) | B0 | `/health` |

## 3. AI integration contracts

Direction is backend to AI service. The backend validates every response, records provenance, applies a timeout, and falls back to rules when the service is down or returns invalid output.

| Capability | Request | Response must include |
|---|---|---|
| Anomaly score | Canonical transaction plus context features | `score` 0 to 1, `modelVersion`, `generatedAt` |
| Classification suggestion | Exception candidate and evidence references | `suggestedCode`, `confidence`, `modelVersion` |
| Root-cause recommendation | Case summary and evidence references | Ranked causes with reasons, `modelVersion` |
| Debit-alert parsing | Customer-supplied text or image reference | Extracted fields each with `confidence`, `parserVersion` |
| Complaint wording | Case facts and deadline data | Draft text, `modelVersion` |
| Case summary | Case timeline | Summary text, `modelVersion` |
| Feedback label | Case outcome and reviewer label | Accepted acknowledgement |

Rules for all of them: output is advisory, low confidence is shown as such, and no AI field is an input to the eight-check reversal gate.

## 3a. External interface contracts (scope 13.3)

One OpenAPI file per interface, so a real system can replace the mock without engine changes. Each is covered by a contract test.

| Interface | Direction | Track H implementation |
|---|---|---|
| Transaction feed | In | Simulator publishing events |
| Transaction status query | Out | Mock status service with scripted outcomes |
| Reversal API | Out | Mock reversal service |
| Settlement and NIBSS response files | In | Generated files |
| SMS and email gateway | Out | Mock notification service |
| Customer app and ALAT integration | In and out | Documented ReverseIt API |

## 4. Known drift to resolve

- OpenAPI paths lack `/v1`, and `/reversals/propose` plus `/reversals/{id}/authorize` do not match the v2 `POST /v1/reversals` and approvals flow.
- Money, casing and error shape differ between the existing OpenAPI, the web types and the v2 contract (D14, D15, D16).
- Role names differ: `ANALYST_MAKER` and `SUPERVISOR_CHECKER` in code, nine roles in the v2 matrix.
- No shared error, pagination or auth scheme components.
- `apps/web/src/types/domain.ts` needs to be diffed against the OpenAPI schemas before the first contract freeze.
- The AI service has no request or response models yet, so the contract above is a proposal for the AI developer to accept or amend.
