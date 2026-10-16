# Authentication and Permission Model

Status: B1. Describes what the code does today. Decisions behind it are in ADR 0003.

## Who can call the API

| Caller | Credential | Audience | Where it is checked |
|---|---|---|---|
| Operator (analyst, officer, supervisor, admin, and so on) | Ed25519 JWT from `POST /v1/auth/login` | `smartreversal-ops` | `requireAuth` on every route except health, ready and login |
| Customer (ReverseIt, from B10) | Same signing key, different audience | `smartreversal-customer` | Not issued yet. Verification already refuses it on operator routes |
| Another service | Assertion the service signs with its own Ed25519 key | `smartreversal-service` | `requireService(scope)` on machine routes (mounted from B2) |

The three audiences are never interchangeable. An operator token fails on a machine route, a service assertion fails on an operator route, and a customer token fails on both.

### Operator tokens

- Signed with EdDSA only. `HS256`, `none` and any other algorithm are refused, as is a token signed with the public key as an HMAC secret.
- Required claims: issuer, audience, subject (user id), `tid` (tenant id), `jti`, issued-at, not-before and expiry. A token whose lifetime exceeds 24 hours is refused whatever the issuer was set to. The default lifetime is 15 minutes.
- Verifiers hold public keys only, looked up by the `kid` header. Several keys can be live during rotation.
- Every failure is the same 401 with no reason.
- The tenant comes from the token, never from the request.
- There is no revocation list. Short lifetimes are the control. A disabled user stops being able to log in, and a token already issued lives out its 15 minutes.

### Login

`POST /v1/auth/login` takes `institution`, `email`, `password`.

- Passwords are argon2id (19 MiB, 2 passes, 1 lane), 12 to 128 characters. Hashes carry their parameters and are upgraded on login when weaker than current.
- An unknown institution, an unknown user, a wrong password, a disabled account and a locked account all return the same 401, and spend about the same time.
- Five failed attempts lock an account for 15 minutes. Attempts made while locked do not extend the lock.
- Every attempt in a known institution is written to the audit chain. A successful login that cannot be audited fails. Failed attempts by an unknown email are recorded with a short digest, not the address.
- Tokens from login carry `mfa=false`. **Nothing in this build issues an MFA-backed token.** See "MFA" below.

### Service accounts

Each service has its own key pair and a per-tenant account with scopes such as `events:write`. It sends an assertion valid for at most 5 minutes. The API checks the signature against the key stored for exactly that tenant and service, the scope, the lifetime, and that the assertion id has not been used. Scopes come from the database, not the token.

## What an operator may do

Permissions are data. The `permission_grants` table holds the v2 matrix: 9 roles, 11 capabilities, 99 cells, loaded when the API starts.

`authz.Policy.Check` answers a question with four inputs: who, which capability, facts about the request, and whether the token has MFA. A missing role, capability, limit or fact is a denial.

| Scope | Extra rule the code enforces |
|---|---|
| `within_limit`, `higher_limit` | The amount must be at or under a limit supplied from configuration. No limit configured means no. |
| `own` | The record's owner must be the caller |
| `reason_gated` | A reason of at least 10 characters must be given |
| `override_dual`, `dual_control` | A reason, and a second approver who is a different person |
| anything else | The scope narrows what the handler returns, and the handler applies it |

When someone holds several roles the widest applicable scope wins. **ADMIN has no implicit access.** There is no bypass of any kind.

Roles and capabilities are the project's working names from the v2 matrix, which says to verify them against the institution's delegated-authority matrix. Every role is `unverified`.

### Segregation of duties

- The maker can never be the checker, whatever roles they hold. Enforced in code and, for configuration and the kill switch, by the database as well.
- An approver cannot be linked to the account involved (`staff_account_links`). The conflict check is pluggable, and a failure to run it denies.

### MFA

Capabilities marked `requires_mfa`: approve a reversal, override the gate, change rules (approval), manage users, view unmasked accounts. Lifting a kill-switch freeze also needs MFA. Freezing does not, so stopping automation is always quick.

Reading configuration and proposing a change do not need MFA. Approving does.

`mfa` is a claim in the token. The code that checks it works and is tested with tokens made for the purpose. **There is no mechanism that makes a user pass an MFA challenge and receive such a token.** Until one is chosen (decision D20), every MFA-gated action is unreachable for real users. This is deliberate: it is better to fail closed than to pretend.

## Tenant isolation

Tenant work runs in a transaction that switches to the `sr_app` database role and sets `app.tenant_id`. Row-level security policies on every tenant table compare against it. `sr_app` has no `BYPASSRLS`, and its grants are the minimum for each table: for the audit trail, the account vault, configuration, the kill-switch log, delegations and shifts it can add and read but never change or remove.

Superusers bypass row-level security in PostgreSQL. **Production must connect as a non-superuser that is a member of `sr_app`.** The migration that creates the role needs `CREATEROLE`.

## Data protection

- Account numbers are replaced by tokens (keyed HMAC, per tenant). The number lives only in `account_vault`, sealed with AES-256-GCM and bound to its tenant and token.
- `POST /v1/accounts/unmask` needs the `view_unmasked_account` grant, a reason, MFA, and is limited to 10 reveals per caller per 10 minutes. Each call, including a request for an unknown token, is audited before anything is released, in the same transaction.
- Log values that look like account numbers (10 digits) or card numbers (13 to 19 digits) are masked before they are written. References such as `NIP202610001234` are left alone.
- The audit trail rejects detail keys that look like secrets (`password`, `token`, `secret`, `authorization`, `private_key`, `api_key`, `pan`, `cvv`).

## Audit trail

One hash chain per tenant. The database refuses updates, deletes and truncation, refuses a row that does not continue the chain, and serialises writers per tenant. `GET /v1/audit?verify=true` recomputes every hash from the rows.

Detected in tests: changed content, changed hash links, a deleted first, middle or last record, a swapped pair, and a forged record appended behind the database's back. Not detected: someone with superuser rights who rewrites every row **and** the head. Close that gap by copying the head (`audit.Anchor`) to storage the database cannot reach, on a schedule. That job is not built.
