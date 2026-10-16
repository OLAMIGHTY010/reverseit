# Simulator, ground truth and scoring (B2)

Everything here is synthetic. No money moves and no real customer data is used.

## Pieces

- `internal/sim` generates events and **labels**. A run is a pure function of its seed: the same seed gives the same bytes. It holds eighteen named scenarios (`sim list`), a fifty-transaction demonstration mix with exactly one unknown (`Demo50`), and delivery perturbations: jitter and reordering, redelivery, a source outage that catches up in order, and clock skew.
- `internal/simrun` plays a run through the real `txn.Service` and `detect.Detector` on a simulated clock, runs the stuck sweep on that clock, and scores the result.
- `cmd/sim` is the command line: `sim generate -seed 7 -demo50 -out DIR` writes `events.jsonl` and `truth.jsonl` without a database. `sim play -seed 7 -demo50` plays the run in a fresh synthetic institution `SIM-<seed>` and prints the score as JSON. It refuses to run in production and refuses a seed whose institution already holds data.

## The labels are written from the domain, not from the detector

For each transaction the label gives the final lifecycle state and the exception codes a correct detector raises. They come from rules such as "a timeout is unknown, never a failure", "a retry after a confirmed failure is not a duplicate", and "a repeat outside the duplicate window is legitimate". If the detector disagrees, that is a finding to investigate, not a label to edit.

## What is scored, and what it does not mean

- Lifecycle accuracy: final state equals the label.
- Precision and recall over findings, per code: a finding is raised if a case of that code was ever opened for the transaction.
- Detection delay: first debit to case opened, **in simulated time**. It reflects rule thresholds and scenario timing. It is not a performance measurement.
- These figures describe the labelled synthetic cases only. They are not a claim about accuracy on real traffic. The labelled set is small and written by the same team that wrote the detector, so a perfect score here shows the detector agrees with the authors' expectations, not that it is right in the field.

## Results at the time of writing

In-order delivery of one of each scenario: lifecycle 23 of 23, precision 1.00, recall 1.00. The fifty-transaction run: 50 of 50, with one UNK001, the timeout. Reordering and redelivery (jitter up to 90 seconds, 30 percent redelivered) leave every final lifecycle correct. A 3-minute clock skew on the switch leaves every final lifecycle correct.

## Findings the simulator exposed

A 38-minute switch outage, with the switch's events delivered together when it recovers, leaves every final lifecycle correct but changes which cases exist:

1. The stuck sweep raises UNK001 for transfers that are only waiting on the silent source. Those cases are not closed when the answer arrives unless it is a success.
2. A retry after a failed transfer whose failure was held back by the outage is raised as DUP001, because at that moment the earlier attempt had not failed.
3. A transfer with a failure from one source and a delayed confirmation from another keeps a FAIL001 case after the evidence turns conflicting.
4. A failure that arrives in the same delivery as its reversal opens no FAIL001 case, so case counts depend on delivery timing.

All four point at B3 work: detection should know when the source it depends on is stale or down (`GET /v1/sources`), hold back or mark such findings, and close or supersede cases whose evidence has changed. The tests log these cases and assert only the lifecycle, so the behaviour is recorded and not hidden.
