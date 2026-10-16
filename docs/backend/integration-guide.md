# Integration guide for the web and AI developers

Status: current as of the `feat/backend-demo-pipeline` branch. The machine-readable contract is `packages/api-contracts/openapi.yaml`. Every operation there is marked `x-status: implemented` or `planned`, and a test fails if the served routes and the document disagree. Where this guide and the OpenAPI document differ, the OpenAPI document wins, and please tell the backend owner.

All money movement is simulated. Every reversal, notification and report carries `simulated: true`. Nothing here talks to a bank.

## Conventions (decisions D14, D15)

- JSON is snake_case.
- Money is `amount_minor` as a decimal string (for example `"1850000"` is NGN 18,500.00) with an explicit `currency`. Never a float. Parse it with a big-integer or string type.
- Errors are `{ "error": CODE, "message": text }`. A failed reversal gate is `409` with `error: "GATE_FAILED"` and `failed_checks: [{id, name, detail}]`.
- Writes that must not repeat take an `Idempotency-Key` header. Reusing a key for a different request is `409 IDEMPOTENCY_KEY_REUSED`.
- Case updates use `If-Match: "<version>"`. A stale version is `412 STALE_VERSION`.
- Lists use `?limit=` and an opaque `next_cursor`.

## Signing in (operators)

`POST /v1/auth/login` with `institution`, `email`, `password` and, to approve things, `otp`.

- Without `otp` the token has `mfa: false`. It can read and request, but approving a reversal returns `403 MFA_REQUIRED`.
- With a valid, unused code the token has `mfa: true`. Codes are TOTP (RFC 6238). This is development-only MFA with a synthetic secret (decision D20). It is not the bank's MFA.
- A wrong code fails the login like a wrong password and counts toward lockout. There is no fallback to a non-MFA token.
- Customer self-service sign-in does not exist yet. The customer status endpoint is read by an operator or an agent on the customer's behalf.

## What the web app needs

| Screen | Calls |
|---|---|
| Exception queue | `GET /v1/exceptions` (filters, cursor) |
| Case detail | `GET /v1/cases/{id or case_ref}` returns the case, evidence, history, transaction, notifications and reversals together |
| Update a case, add a note or evidence | `PATCH /v1/cases/{id}`, `POST /v1/cases/{id}/notes`, `POST /v1/cases/{id}/evidence` |
| Customer-reported problem | `POST /v1/complaints` |
| What to tell the customer | `GET /v1/cases/{id}/customer-status` |
| Can this be reversed? | `GET /v1/cases/{id}/reversal-eligibility` returns the eight checks and a verdict (`PASS_AUTO`, `PASS_NEEDS_APPROVAL`, `BLOCKED`) |
| Request a reversal | `POST /v1/reversals` (`case_id` as id or case reference, `reason_code`) |
| Reversal progress | `GET /v1/reversals/{id}` shows `tracks.accepted`, `tracks.processing`, `tracks.financial` separately; `POST /v1/reversals/{id}/advance` asks the provider and verifies |
| Approval queue | `GET /v1/approvals?status=PENDING`, `POST /v1/approvals/{id}/decision` with `{decision: approved or rejected, reason}` |
| Dashboard | `GET /v1/reports/summary`, `GET /v1/reconciliation/reversals` |
| Transactions | `GET /v1/transactions`, `GET /v1/transactions/{id}` |

### Rules the interface must respect

1. Show `tracks.financial` and nothing else as the money outcome. `VERIFIED` is the only value that means funds were confirmed in the ledger. `accepted` and `processing` are not proof.
2. The customer message in `customer-status` says funds were returned only when `outcome_verified` is true. Do not rephrase it into a promise.
3. A `BLOCKED` verdict or a `409 GATE_FAILED` is a normal, expected answer. Show the failed checks. Do not offer a retry button that creates a new key for the same debit.
4. An approval is not available to the person who requested the reversal (`SELF_APPROVAL`). The interface may hide the button, but the server decides.
5. Label every reversal and ledger figure as simulated while `simulated` is true.

## What the AI service can use

- `POST /v1/events` takes transaction events from a service (service assertion with scope `events:write`, header `Idempotency-Key` equal to `source_event_ref`). Events are applied in any order and a duplicate event is a no-op.
- `GET /v1/transactions/{id}` and the case detail expose the facts the detector used. Evidence entered by a person is stored with `is_fact: false`.
- The backend decides eligibility. An AI score or explanation can be attached as evidence or a note, but it cannot change a gate result, an approval or a reversal. Unknown, pending and conflicting transaction outcomes are never reversed, whatever a model says.

## Contract changes you may notice

- Reversal request: `txn_id` and `evidence_ids` are no longer sent. The case already identifies the transaction.
- `ReversalAccepted` and `ReversalStatus` are replaced by one `Reversal` shape.
- `LoginRequest` gained `otp`, `LoginResponse` gained `mfa`.
- Customer and agent authentication for self-service is not built. See `docs/backend/known-limitations.md`.
