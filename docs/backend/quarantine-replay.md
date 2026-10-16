# Quarantine and replay (B2)

An event that ingestion refuses is not lost. It goes to `quarantined_events` with the reason. Events refused after they decoded (an invalid or contradictory envelope, or a reused reference with different content) keep the envelope, which holds account tokens and never account numbers. Events that could not be decoded at all keep only a hash and a reason and cannot be replayed.

## Operations

| Route | Needs | Does |
|---|---|---|
| `GET /v1/quarantine?status=open` | `manage_ingestion` (any scope, no MFA) | Lists refused events, oldest first |
| `POST /v1/quarantine/{id}/replay` | `manage_ingestion` with `full` scope, MFA, a reason | Offers the event to ingestion again |
| `POST /v1/quarantine/{id}/discard` | same | Marks it as not to be replayed |

`manage_ingestion` is new in migration 0023: `ADMIN_INTEGRATION` has `full`, `HEAD_OPS` has `metadata` (view only). This grant is a proposal for review, not an approved role design.

## Rules

- A replay goes through exactly the same validation, idempotency and conflict checks as a new event. It runs in a savepoint, so a refusal stores nothing.
- Accepted or already stored: the row becomes `replayed`. Refused again: it stays `open`, the attempt count and latest reason are updated.
- The row is locked while it is replayed, so concurrent replays run once and the rest get `409`.
- Every replay attempt and every discard is audited with the operator and the reason. Discard keeps the row.
- Typical causes that replay fixes: a source clock that ran ahead, or an event that arrived before the data it depended on was loaded. A reused reference with different content stays refused, because replaying it cannot make it consistent.

## Limits

- No automatic replay job yet; replay is an operator action.
- Events quarantined before migration 0023 have no payload and can only be discarded.
