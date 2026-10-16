# Demo video sequence

About four minutes. Everything on screen is synthetic and simulated. Say so near the start. Do not show the terminal where `DEMO_PASSWORD` was set, the authenticator link or secret printed by `seed`, any `.env` file or signing key. Crop or blur if in doubt.

## Before recording

1. Services running as in `docs/runbooks/demo-runbook.md` (API and worker).
2. Run `go run ./cmd/demo play` to get a fresh state.
3. Sign in as the officer and as the supervisor in two browser profiles (or sign in with the supervisor just before step 3 below). Have `go run ./cmd/demo code supervisor` ready in a terminal that shows nothing but the six digits.

## Sequence

**0:00 Opening (say this).** "All of this uses synthetic data. The bank systems, the ledger and the reversal provider are simulated, and live money movement is disabled."

**0:20 Exception queue.** Show three cases: one resolved failure, one duplicate awaiting approval, one unknown.

**0:40 Case 1, the failed transfer (FAIL001, resolved).**
- Open the case. Point to the history: detected, status confirmed as failed, eight checks passed, reversal submitted, ledger check, resolved.
- Show the reversal tracks. The money track says `VERIFIED` only because the backend read the ledger itself.
- Open the customer view. Two messages: first "we found a problem", then "reversed, confirmed" after verification.
- Say: "This one was automatic because a policy allows it and the amount is under the automatic limit. That policy is a synthetic demonstration value, not a bank rule."

**1:50 Case 2, the duplicate (DUP001, approval required).**
- Open the case and show the reversal eligibility: passes, but needs approval.
- Open the approval queue. Say: "Duplicates are never reversed automatically. Nothing has moved."
- Sign in as the supervisor with a one-time code. Approve with a reason.
- Show the reversal now `VERIFIED` and the case resolved. Optional: show the officer who requested cannot approve their own request.

**3:00 Case 3, the unknown (UNK001, held).**
- Open the case. Show eligibility: blocked, with the failed checks (no fresh authoritative status).
- Try to request a reversal. Show the refusal listing the checks. Say: "When we do not know whether the transfer happened, reversing could pay twice. So the system holds it and a person investigates."

**3:30 Dashboard.** Show the summary and the reconciliation: reversals matched against the ledger.

**3:50 Close (say this).** "What is built: ingestion, detection, case management, a gated simulated reversal with approvals and independent verification. What is not: customer self-service sign-in, real bank integrations, and production controls such as real MFA and verified bank policy."

## Reset between takes

`go run ./cmd/demo play` resets the scenario data and plays it again. Case numbers keep counting up, so refer to cases by transfer reference (DEMO-FAIL-001, DEMO-DUP-002, DEMO-UNK-001) rather than by number. If you approved the duplicate, run it again to get the pending state back.
