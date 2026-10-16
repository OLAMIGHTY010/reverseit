# Backend Decision Register

Last updated: 2026-10-15. "Blocks" says which phase cannot finish, or start, without the answer. Items marked Default are proceeding on the stated recommendation until someone objects. Values that the playbooks leave to Wema or Compliance are configuration with a VERIFY flag. They are not decided here and must not be invented.

## Decided

| ID | Decision | Outcome |
|---|---|---|
| D1 | Backend language | Go with Chi. Python stays with the AI service only. Approved 2026-10-15. See ADR 0002. |
| D2 | Messaging | Redis Streams with a PostgreSQL transactional outbox. PostgreSQL is authoritative. Redis is never the source of truth for financial outcomes. Approved 2026-10-15. |
| D11 | Go version | Go 1.27.2 across `go.mod`, CI, Docker and docs, if dependencies are compatible. Done: all direct dependencies build, vet and test on 1.27.2, and the Docker image builds on `golang:1.27.2-alpine`. No blocker found. |

## Applied as defaults in B1 (not yet confirmed)

B1 started with D5 to D10 still open, on the project owner's instruction. Each was implemented as the recommendation in this register and recorded in ADR 0003. They remain open for confirmation, and each is isolated enough to change.

| ID | Applied as |
|---|---|
| D5 | Ed25519 keys with a key set by `kid` |
| D6 | Local users table, argon2id, behind `UserStore` |
| D7 | Shared schema with row-level security and the `sr_app` role |
| D8 | Separate audience for customers (verification only; issuance is B10) |
| D9 | No admin bypass, maker never equals checker |
| D10 | HMAC tokens and an AES-GCM vault for account numbers |

## Open, and what they block

| ID | Decision | Recommendation | Blocks | Status |
|---|---|---|---|---|
| D3 | OpenAPI first with route-to-spec contract tests | Adopt | B0 contract work | Default, proceeding. Low risk and easy to reverse. |
| D4 | Named owners for frontend and AI contract sign-off | One person each | Contract freeze (end of B0 contract work). Does not block B0 code. | Open. Need names. |
| D5 | Token signing: HS256 shared secret or asymmetric keys | Asymmetric, with a key set | B1 | Open. B0 does not issue tokens. |
| D6 | Identity source for Track H | Local user table, hashed passwords, behind an interface | B1 | Open. |
| D7 | Tenancy model | Shared schema with row-level security | B1 | Open. Baseline migration contains no tenant tables, so B0 is unaffected. |
| D8 | Customer identity for ReverseIt | Separate token audience | B1 and B10 | Open. |
| D9 | Admin and maker-checker | Admin cannot self-approve or bypass the gate | B1 | Open for the permission model. Current admin bypass is removed in B0 and fails closed. |
| D10 | Masking and tokenisation | Masked responses plus tokenised storage of account numbers | B1 | Open. |
| D12 | Who may enable financial execution | Off everywhere by default. Enabling needs dual control and an audit record. | B5 | Open for the approver list. Default-off is already mandated. |
| D13 | Values the playbooks mark VERIFY: regulatory clocks, approval limits, waiting thresholds, rail codes | Configuration with source, effective date and VERIFY status | B3, B5, B10 values, not code | Open with Wema and Compliance. See "Not to be invented". |
| D14 | Money representation. The v2 shared contract uses `amount_minor` (integer minor units). The PR template says `NUMERIC(18,4)`. The existing Go code uses decimal strings. | Integer minor units plus ISO currency in API and `BIGINT` in storage, to match the v2 contract. Fee and FX never use floats. | B0 OpenAPI and B2 schema | Approved 15 Oct 2026. Money is a decimal string `amount_minor` with explicit `currency` in JSON, integer minor units in storage. Implemented. |
| D15 | JSON field naming. The v2 contract uses snake_case. The web app types and the current Go code use camelCase. | snake_case per the shared contract | Contract freeze | Approved 15 Oct 2026. snake_case. Implemented. |
| D16 | Error shape. The v2 contract returns `{ "error": "GATE_FAILED", ... }`. | Flat error code string plus `message`, `trace_id`, optional `details`, extended with `failed_checks` for the gate | Contract freeze | Default, proceeding. |
| D17 | Internal clock start for the 24 hour failed-transfer clock. Press reports tie it to a complaint. v2 recommends starting at failure time. | Failure time (more protective), pending Compliance | B4 and B10 | Open with Compliance. |
| D18 | Disputed POS/web clock: 72 hours in the scope document, 48 hours in another summary | Keep 72 h as configured value flagged `conflicting` | B10 | Open with Wema. |
| D19 | Status-query-first policy, approval thresholds, and meaning of "within regulatory limits" | Documented as ADR items to confirm with mentors | B5 | Open with mentors. |
| D20 | How a user obtains an MFA-backed token in Track H. The code enforces an `mfa` claim but nothing issues it. | A simulated second factor behind an interface (for example TOTP with a seeded secret), clearly labelled as simulated | B5 (approvals), B1 follow-up. Approving config, unmasking and lifting a freeze are unreachable until it exists | Approved 15 Oct 2026. Implemented as development-only TOTP (RFC 6238) with a synthetic secret, one-time use, and no bypass for privileged actions. Not production MFA. |
| D21 | Team model for the `team` audit scope and queue visibility | Define with case management | B4 | Approved 15 Oct 2026: the `team` scope stays inaccessible. |

## Blocks B0

None of the open items blocks B0. D11 is decided, D3 and D16 proceed on defaults, and D4 only gates contract freeze. D14 and D15 must be settled before the first contract freeze, so they are scheduled before the OpenAPI baseline is declared stable and the B0 report will say whether they were answered.

## Can wait for their phase

- B1: D5, D6, D7, D8, D9, D10
- B3: D13 rule thresholds
- B4: D17, D21
- B5: D12, D19, D20
- B10: D18

## Not to be invented

The following come from Wema, Compliance or the rail operators and are loaded as versioned configuration, each with `source_reference`, `effective_from` and `verify_status`:

- Regulatory clock durations and start events. The v2 register lists candidate values, and marks them as press-reported and unverified.
- Approval limits by amount, risk grade and exception type.
- Waiting thresholds per rail.
- Rail status and response codes. The playbooks say to use illustrative codes only.
- Source freshness limits in the Source Authority Matrix.
- Any acceptance threshold for AI or detector accuracy.
