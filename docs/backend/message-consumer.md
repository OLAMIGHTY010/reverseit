# Message-consumer ingestion (B2)

A source can write to the Redis stream `events.inbound` instead of calling the API. The worker consumes it with the same group machinery as other streams (`internal/stream`): idempotency through `processed_events`, redelivery of unacknowledged entries, and a dead-letter stream after repeated failures.

## Entry format

The same fields the outbox relay writes:

| Field | Value |
|---|---|
| `tenant_id` | UUID of the institution |
| `event_id` | UUID unique per entry; a redelivered entry is skipped |
| `type` | `transaction.event` (a canonical envelope) or `transaction.observation` (evidence by external reference, see correlation.md) |
| `payload` | the JSON, with unknown fields refused |

`inbound.Entry` builds an entry. Writing to the stream needs Redis credentials, so the trust boundary is the network and the credentials on the stream, not a service assertion. Production access control for that stream is an open deployment decision.

## Failure policy

- **Bad data that can never succeed** is quarantined and acknowledged, not retried: an invalid envelope, one that contradicts its transaction, a reused reference with different content, malformed JSON, or an unknown type. Envelopes are kept whole for review and replay (`quarantine-replay.md`). Retrying five times and then dead-lettering would only delay the answer.
- **Our own failures** (database errors, a missing correlator) return an error. The entry is not acknowledged, is redelivered after the idle time, and is dead-lettered after the maximum number of deliveries, where `stream.ReplayDeadLetters` can return it.
- A refused event runs in a savepoint, so it leaves no half-created transaction.
- Sending the same event twice, in one entry or two, stores it once.

## Limits

- Per-source authentication of stream writers is not implemented.
- No bound on stream length or consumer lag is enforced yet.

## Backlog and lag

`GET /v1/pipeline` (grant `manage_ingestion`, no MFA to view) reports, for the caller's institution, the unpublished outbox rows, how many have already failed, and the age of the oldest. For the shared platform it reports each stream's length, dead-letter count, and per group the consumers, pending entries and lag. It states numbers and no verdict, because what counts as too much depends on volumes nobody has measured yet. `redis_available` false means Redis was not reachable and says nothing about the streams. The API opens its Redis client lazily, so an unreachable Redis never stops it.
