# Demonstration runbook

Everything below is synthetic. No real customer, account or bank policy is used, and all money movement is simulated. The command refuses to run when `ENVIRONMENT=production`.

## Start the services

You need Postgres 16 and Redis 7. Set the usual variables (`DB_*`, `REDIS_*`, `JWT_PRIVATE_KEY` from `go run ./cmd/keygen`). From `apps/api`:

```bash
go run ./cmd/migrate up
export DEMO_PASSWORD='choose a passphrase of 12 or more characters'
go run ./cmd/demo seed          # institution DEMO-BANK, two operators, synthetic policies
go run ./cmd/api                # in one terminal
go run ./cmd/worker             # in another (detector, sweeps, reversal automation)
```

`seed` prints a development authenticator for each operator (an `otpauth://` link). Add it to any authenticator app, or print the current code with `go run ./cmd/demo code officer` or `go run ./cmd/demo code supervisor`.

Operators: `officer@demo.example` (REVERSAL_OFFICER, TXN_ANALYST) and `supervisor@demo.example` (SUPERVISOR). Institution: `DEMO-BANK`.

## Run the scenario

```bash
go run ./cmd/demo play          # resets scenario data, then plays it once; prints a JSON report
go run ./cmd/demo reset         # clears scenario data only
```

Play is repeatable. Users, policies and the audit trail are kept.

## What you should see

| Transfer | What happens | Final state |
|---|---|---|
| DEMO-FAIL-001 | Debit posted, beneficiary credit failed, ingestion accepts it, the detector opens a FAIL001 case, the (simulated) status source confirms the failure, the eight checks pass, a synthetic policy allows automatic reversal, the mock provider confirms, the backend checks the ledger itself | Case `RESOLVED`, reversal `VERIFIED`, customer told in two stages (detected, then verified) |
| DEMO-DUP-002 | Second debit for one transfer, DUP001 case | `AWAITING_APPROVAL`. Nothing moves until a different person with MFA approves |
| DEMO-UNK-001 | The switch timed out | UNK001 case stays `DETECTED`. No reversal is created. The gate reports it as blocked |

Approve the duplicate over HTTP:

1. Log in as the supervisor with `institution`, `email`, `password` and `otp` (from `demo code supervisor`). The response has `mfa: true`.
2. `GET /v1/approvals?status=PENDING`, then `POST /v1/approvals/{approval_id}/decision` with `{"decision":"approved","reason":"checked"}`. The reversal comes back with `tracks.financial: "VERIFIED"`.
3. `GET /v1/reconciliation/reversals` shows the reversals as `MATCHED`. `GET /v1/reports/summary` shows the totals.

Note that a REVERSAL_OFFICER with MFA may also approve a request the system made, within the configured limit. A person cannot approve their own request.

## Things to say out loud

- The status source, ledger and provider are simulated tables. The "independent verification" is real logic against a synthetic ledger.
- The policy values (windows, thresholds, limits, which codes may be reversed automatically) are labelled `SYNTHETIC DEMO POLICY - not approved for production` and are unverified. They are not bank policy.
- MFA is development TOTP.
