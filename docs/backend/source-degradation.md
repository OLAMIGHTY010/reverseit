# Source degradation and held findings (B3, M26)

A finding that depends on a source being silent is held while that source is stale or down. The absence of an event from a source that is not reporting is not evidence about the transaction.

## What is held

- **UNK001 from the stuck sweep.** A transfer unanswered past its waiting threshold is not raised while a source it depends on is degraded. A transaction depends on the sources that have contributed an event to it and on the sources that have recently answered for its rail (a transfer sent, a credit confirmed or failed, a timeout, read from the last 24 hours of traffic). The second part matters during an outage, when the source that should answer has contributed nothing yet.
- **DUP001 pairs.** If the earlier attempt has no final answer and a source it depends on is degraded, the pair is not judged. The earlier attempt may already have failed, which would make the second a retry. The transaction is re-examined by the sweep (`SweepHolds`) and whenever either transaction receives an event.

A hold is a row in `detection_holds` (migration 0028) with the reason. It is released when the transaction resolves, when the source recovers and the finding is judged, or when the pair no longer applies. The sweep job result reports `held_for_degraded_source`.

## Only sources with an expectation can be degraded

A source with no expectation (`PUT /v1/sources/{system}/expectation`) is never treated as degraded, so behaviour does not change until someone who knows the source sets one. (Declarative rules are stricter: `requires_fresh` holds a rule while a source is unconfigured, see detection-rules.md.)

## Measured effect (simulator, seed 12, a 38-minute switch outage)

Unexpected UNK001 cases: 10 without expectations, 0 with. The final lifecycle of every transaction is correct in both runs.

## Residual limits found by the same run

- A retry judged before the silent source has crossed its stale threshold is still flagged as a duplicate. Silence cannot be noticed before it has lasted long enough.
- A confirmed failure from one source stands even if another source's delayed confirmation later makes the evidence conflicting; the failure case is not superseded.
- A failure delivered in the same batch as its reversal opens no failure case, so case counts depend on delivery timing.
