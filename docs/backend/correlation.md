# Correlating evidence from other sources (B2)

A switch, gateway or settlement feed usually knows its own references and not our instruction reference. `POST /v1/observations` takes that evidence and `internal/correlate` decides which transaction it belongs to.

## How a match is scored

External references are stored per transaction (`transaction_refs`) from every event that carries them, plus the session id. A reference is evidence of a link and never a key: schemes can reuse an RRN, so a candidate is scored from the references that agree, then capped when the facts disagree.

| References that match | Confidence |
|---|---|
| Ledger id, or gateway reference | 1.00 |
| RRN and STAN together | 0.95 |
| Session id | 0.90 |
| RRN alone | 0.70 |
| STAN alone | 0.50 |

The score is capped at 0.4 if the amount or currency differs, and at 0.3 if the times are more than 48 hours apart. Evidence links automatically only if the best candidate is at least **0.90** and at least 0.05 better than the next. These numbers are engineering defaults for synthetic data, held on `correlate.Matcher` so they can be tuned. They are not an approved policy. A bank should confirm them against its real reference behaviour before any production use.

## What happens next

- **Linked:** the evidence becomes an ordinary event on that transaction, carrying the transaction's own core facts so it can never change them. The projection updates as for any event.
- **Queued:** anything else is kept in `evidence_correlations` as `open` with the reason and the scored candidates. `GET /v1/evidence` lists it. Nothing is guessed.
- **Late evidence:** `Matcher.Retry` looks at open items again, because their transaction may have arrived since. It is a function, not yet a scheduled job.
- **A person:** `POST /v1/evidence/{id}/link` (full scope, MFA, a reason) links only if amount and currency agree, so a real disagreement between sources is not hidden. `.../discard` closes it. Both are audited in the same transaction, and the row keeps who did it and why.
- Repeating the same observation returns its first outcome. Reusing its reference with different content is a conflict.

## Limits

- The same RRN on two transactions with equal amounts and times is queued as ambiguous, by design.
- Retry is not scheduled; the worker job belongs to B11.
- Duplicate-debit detection does not use these references yet.
