# Alerts and incidents (B7)

Internal only. Nothing here is sent to a customer, and nothing moves money.

## Pieces
- `internal/alerts`: one open alert per kind and subject, routed to the people on call, escalated when unacknowledged after the approved `alert.escalate_after_minutes`. `Sweep` raises alerts for what is true now and clears them when it stops.
- `internal/incidents`: groups open exceptions by rail, channel and error signature. `DetectSpikes` compares against the same hour of earlier weeks, business-calendar aware.
- Worker: `opsAll` runs both on every sweep tick for every institution.

## API
| Route | Access |
|---|---|
| `GET /v1/alerts` | `view_exception_queue`, full scope |
| `POST /v1/alerts/{id}/acknowledge` | full scope, and routed to, target role of, or escalated role of the alert |
| `GET /v1/incidents` | full or metadata scope |
| `GET /v1/incidents/{id}` (id or reference) | full scope; includes case ids |
| `POST /v1/incidents/{id}/causation` | `add_notes_evidence`, full scope; judgement only moves up, with a note |
| `POST /v1/incidents/{id}/resolve` | same; needs a review note |

## Configuration (registry keys, no defaults, none approved)
`alert.escalate_after_minutes`, `alert.high_value_minor`, `alert.high_value_age_minutes`, `incident.group_window_minutes`, `incident.min_group_size`, `incident.spike_sigma_tenths`, `incident.spike_min_weeks`, `incident.spike_min_count`. Without an approved value the matching behaviour stays off. Values used in tests are synthetic.

## Not built
Delivery tracking, retry and opt-out; incident bulk handling with per-item gate checks.
