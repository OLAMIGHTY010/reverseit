# Domain events

Events about cases, reversals and notifications, written to the transactional outbox and delivered to Redis Streams by the relay. Code: `apps/api/internal/events`. Tests: `internal/events`, `internal/cases/events_test.go`, `internal/reversal/events_test.go`, `internal/demo/events_test.go`.

## Guarantees and what is not promised

- An event is written in the same PostgreSQL transaction as the change it describes. A change that rolls back never publishes. A change that commits always has its event.
- Delivery to Redis is at least once. Consumers must dedupe on `event_id`. The stream consumer in `internal/stream` already does this per consumer group using `processed_events`. Exactly-once delivery is not promised.
- `event_id` is stable: it is derived from tenant, type and the occurrence key, so a repeated request cannot store a second copy.
- Events carry identifiers and non-sensitive metadata only. Each type has an allow-list of fields and the emitter refuses any other. No account tokens, names, contact details, message text, idempotency keys, provider references or gate details are included.
- Every payload has `schema_version` (currently 1), `tenant_id` and `occurred_at`. Adding an optional field does not change the version.
- Financial events reflect stored state. `reversal.verified` is emitted only on the path where the ledger posting has been checked, never because a provider acknowledged a request.

## Catalogue

| Type | Stream | Emitted when | Occurrence key | Fields (besides the three above) |
|---|---|---|---|---|
| `case.opened` | `cases` | A new case is created. Not for an idempotent reopen | case id | case_id, case_ref, classification, source, priority, status, txn_id |
| `case.transitioned` | `cases` | A status change is committed. Not for a refused or stale move | case id and new version | case_id, case_ref, classification, from_status, to_status, version |
| `reversal.requested` | `reversals` | The gate passed and a reversal was recorded. Not when the gate blocks | reversal id | reversal fields |
| `reversal.approved` | `reversals` | A checker approved. Not for rejection | reversal id | reversal fields |
| `reversal.submitted` | `reversals` | The provider accepted the submission. Not for a timeout | reversal id | reversal fields |
| `reversal.verified` | `reversals` | The ledger posting matched | reversal id | reversal fields |
| `reversal.failed` | `reversals` | Provider rejection, ledger mismatch, or stopped before submission | reversal id | reversal fields, failure_code |
| `notification.recorded` | `notifications` | A new notification was stored. Not for a deduplicated one | notification id | notification_id, case_id, audience, stage, template_id |

Reversal fields: reversal_id, reversal_ref, case_id, txn_id, amount_minor (decimal string), currency, approval_status, accepted_status, processing_status, financial_status, requested_by_type, policy_id.

`failure_code` values: `PROVIDER_REJECTED`, `LEDGER_MISMATCH`, `STOPPED_BEFORE_SUBMISSION`.

## Not covered yet

- Rejection of an approval has no `reversal.rejected` event. The case transitions are announced by `case.transitioned`.
- A provider timeout emits nothing until the outcome is known. It is then announced as submitted or failed, and verified if the ledger agrees.
- Transaction updates keep their existing `transactions` stream and `transaction.updated` type.
- No consumer reacts to these events yet. Alerts (B7), metrics (B8) and jobs (B11) will.
