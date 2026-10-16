# ADR 0003: Identity, Tenancy and Data Protection

Status: Proposed defaults, implemented in B1 on 2026-10-15 at the project owner's instruction to start B1. Decisions D5 to D10 were still open when work began, so each choice below is the recommendation from the decision register and can be changed. Each is isolated behind an interface or a single package so a change is cheap.

## Decisions applied

| ID | Decision | Applied as |
|---|---|---|
| D5 | Token signing | Ed25519 (EdDSA) JWTs. Verification uses a key set of public keys identified by `kid`. Only the issuer holds a private key. No shared HMAC secret. |
| D6 | Identity source for Track H | Local `users` table with argon2id password hashes, behind a `UserStore` interface. An external identity provider can replace it later without changing token verification. |
| D7 | Tenancy | One schema. `tenant_id` on every tenant table. PostgreSQL row-level security as a second line of defence, enforced by a non-superuser role (`sr_app`) that tenant work runs as. |
| D8 | Customer identity | Separate token audience: `smartreversal-ops` for operators and `smartreversal-customer` for customers. A token is accepted only by routes that name its audience. Customer tables arrive in B10. |
| D9 | Admin | `ADMIN` has no implicit access. Roles hold explicit permissions from the permission matrix. A maker can never be the checker of the same item, including admins. |
| D10 | Masking and tokenisation | Account numbers are stored as a keyed HMAC-SHA256 token. The original is kept encrypted with AES-256-GCM in a vault table, readable only through a reason-gated, rate-limited, audited unmask call. |

## How tenancy is enforced

Tenant work runs inside a transaction that does `SET LOCAL ROLE sr_app` and `SET LOCAL app.tenant_id = '<uuid>'`. Policies compare `tenant_id` to that setting. `sr_app` has no `BYPASSRLS`, owns nothing, and has only the grants it needs: the audit table gives it `INSERT` and `SELECT` but never `UPDATE` or `DELETE`.

Privileged work (migrations, the outbox relay, audit chain verification) uses the connection's own role and is limited to named code paths.

Superusers bypass row-level security in PostgreSQL. Production must connect as a non-superuser. Tests run as a superuser, so they always switch to `sr_app` before asserting isolation.

## Alternatives rejected

| Option | Reason |
|---|---|
| Keep HMAC (HS256) | Every verifier would hold the signing secret. |
| External identity provider now | No provider chosen. The interface keeps the door open. |
| Schema per tenant | Harder migrations and no benefit at this scale. |
| Tenant filter in application code only | One missed `WHERE` leaks data. Policies make that a failed query instead. |
| Encrypt accounts with a tenant key from the start | Needs a key management service that does not exist yet. A single environment key is used and the limitation is documented. |

## Limits

- One environment key encrypts the account vault. Rotation and per-tenant keys are future work (I-04).
- Login protection is a simple failed-attempt lockout. Rate limiting and MFA enrolment belong to B9 and the identity provider.
- MFA is enforced only as a claim on the token (`mfa: true`) for privileged actions. Nothing here issues MFA challenges.

## What B1 learned

- Superusers bypass row-level security, so every isolation test switches to `sr_app` first. Any test that skipped that step would pass for the wrong reason.
- Rate limits for account unmasking are counted from the audit trail itself. That keeps one source of truth shared by every API instance, at the price of a query per request.
- The MFA rule belongs to the capability, but reading and proposing a change are not the privileged part. The first version applied it too widely, and an end-to-end run caught it (`GrantFor` now separates the two).
- There is still no way to obtain an MFA-backed token. See decision D20.
