# Source freshness and clock skew (B2)

`GET /v1/sources` reports, for every source system that has sent events:

- **status**: `fresh`, `stale` or `down` against the expectation set for that source, or `unconfigured` when none is set. The platform assumes no thresholds, and an unconfigured source is never shown as fresh.
- **silence_secs**: time since anything was last received from it. This uses receipt time, so a source replaying old events is not mistaken for a silent one.
- **skew p50 and p95**: received time minus the time on the event, over the newest 200 events, nearest-rank. `skew_exceeded` is true if either exceeds the expected maximum in absolute value, which also catches a clock running ahead.

`PUT /v1/sources/{system}/expectation` sets `stale_after_secs`, `down_after_secs` and `max_skew_secs`. It needs `manage_ingestion` with full scope and MFA, is audited, and only works for a source that has already sent events.

## Limits

- Ingestion already refuses events dated more than 5 minutes in the future, so a clock running far ahead shows up as quarantined events, not as skew.
- Skew is not meaningful for file imports, because receipt time is the import time.
- Detection does not yet use freshness to hold back a "missing event" finding when its source is stale or down. That is a B3 task and is the reason this report exists.
