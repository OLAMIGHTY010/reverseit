# Risk and Assumption Register (B0)

Status: draft. Review at the end of each phase.

## Risks

| ID | Risk | Likelihood | Impact | Mitigation | Owner |
|---|---|---|---|---|---|
| K1 | Stack disagreement between brief and repo causes rework | Closed | Closed | D1 decided: Go. See ADR 0002 | Backend lead |
| K2 | Requirements misread from the playbooks | Medium | Medium | Matrix reconciled against the PDFs on 2026-10-15. The Complete Playbook was checked for backend content only. | Backend |
| K3 | Existing dev login ships by accident | Closed in B0 | High | Handler disabled and tests assert 501 with no token. The route is not mounted. Real login arrives in B1 | Backend |
| K4 | Tenant filter missed in one query | Medium | High | Row-level security plus isolation tests | Backend |
| K5 | Reversal executed twice under retry or concurrency | Medium | Critical | Idempotency key, unique remediation lock, concurrency test | Backend |
| K6 | Timeout treated as failure and money reversed wrongly | Medium | Critical | Five outcome classes, status-query-first, tests for S1 | Backend |
| K7 | AI output influences a financial decision | Low | Critical | `aiclient` not importable from `reversal`, CI check | Backend |
| K8 | Contract drift with frontend | High | Medium | OpenAPI first, contract tests | Backend and frontend |
| K9 | Simulated data mistaken for verified data in reports | Medium | High | `simulated` flag, separate report sections | Backend |
| K10 | Real customer data enters fixtures | Low | High | Synthetic generator only, review fixtures in PRs | All |
| K11 | CI red from day one (missing go.sum, version mismatch) | Closed locally | Low | go.sum committed, versions aligned. The updated workflow has not yet run on GitHub | Backend |
| K12 | Hackathon time pressure tempts skipping safety tests | Medium | High | Safety tests are exit criteria, not optional | Backend lead |
| K13 | Outbox relay publishes twice or stalls, consumers double-apply | Medium | High | Idempotent consumers, unique constraints, relay lag metric, replay test | Backend |
| K14 | Redis loss or restart drops stream state | Medium | Medium | PostgreSQL stays authoritative, outbox replay rebuilds streams | Backend |
| K15 | Go 1.27.2 incompatibility with a dependency | Closed | Medium | Verified: no incompatibility found | Backend |
| K16 | Frontend built against types that differ from the shared contract | High | Medium | Resolve D14 and D15 before contract freeze, contract tests | Backend and frontend |
| K17 | Press-reported regulatory clocks used as fact | Medium | High | Configuration with VERIFY status, shown to users as such | Backend |
| K18 | MFA-gated actions are unreachable because nothing issues MFA tokens | Certain | High | Decide D20 and build a simulated second factor before B5 | Backend and mentors |
| K19 | Production connects as a superuser, which bypasses row-level security | Medium | Critical | Document, require a non-superuser member of `sr_app`, add a startup check in B9 | Backend and ops |
| K20 | Someone with superuser rights rewrites the audit chain and its head together | Low | High | Anchor the head outside the database on a schedule | Backend and ops |
| K21 | Role and capability names do not match the bank's delegated-authority matrix | Medium | Medium | Verify with Wema; roles carry `unverified` | Backend |
| K22 | Approval limits are not configured, so approvals deny | Certain | Medium | Obtain limits from Wema (D13). Fail closed is intended | Mentors |

## Assumptions

| ID | Assumption | Status |
|---|---|---|
| A1 | All money movement is simulated for the hackathon | Stated in brief |
| A2 | A single bank tenant is enough for the demo, with a second tenant used for isolation tests | Unverified |
| A3 | The frontend will consume REST JSON only | Unverified |
| A4 | The AI service can be absent and the system still passes all safety tests | Required by brief |
| A5 | Postgres 16 and Redis 7 are acceptable | Matches compose file |
| A6 | Business calendar covers Nigerian public holidays and weekends | Unverified, needs source |
| A7 | Waiting thresholds and approval limits come from Wema or Compliance. The playbooks do not supply values. | Unverified, see D13 |
| A8 | No production integration with NIBSS or any bank is attempted in Track H | Stated in brief |
