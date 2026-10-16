# Known limitations and what is simulated

Read this before relying on anything in the demonstration.

## Simulated, by design

- **Authoritative transaction status, the core ledger and the reversal provider** are tables in our own database (`sim_status_script`, `sim_ledger_entries`, `sim_provider_script`). Behaviours available: SUCCESS, REJECT, TIMEOUT, DELAYED, GHOST. No adapter for a real switch, core banking system or provider exists. Live financial execution is disabled (decision D12).
- **Independent verification** is real logic (the backend reads the ledger itself and compares the posting, amount and reference) running against the synthetic ledger. It shows the control. It is not proof against a real ledger.
- **Customer and operations notifications** are recorded with `simulated: true`. Nothing is sent by SMS, email or push.
- **Reports and reconciliation** read the synthetic data above.

## Policy that is not verified

Time windows, waiting thresholds, approval limits, which exception codes may be reversed automatically, and regulatory clocks are not confirmed with any bank or regulator (decisions D13, D17 to D19). The demonstration installs explicitly synthetic values labelled `SYNTHETIC DEMO POLICY - not approved for production`, through the same maker-checker as any change. Without approved values the system blocks (for example, a duplicate is not detected without `duplicate.window_seconds`, and nothing is reversed without a status-freshness setting). Nothing is defaulted silently.

## Security and access

- **MFA is development-only TOTP** (D20). The secret is synthetic and stored as is in `users.totp_secret`. It is not sealed with the vault key, there is no recovery flow, and no enrolment screen (the bootstrap and demo commands enrol). A bank deployment must replace it with its identity provider.
- **No customer self-service sign-in.** There is no customer audience login. `customer-status` is read by an operator on the customer's behalf. A customer-facing channel needs its own authentication design.
- **No user administration API.** Users are created by the bootstrap or demo commands.
- **The `team` audit scope is inaccessible** (D21). Only `full` and `own` apply.
- Reports and reconciliation use the existing queue permission (`view_exception_queue`). A dedicated reporting permission has not been defined.
- Rate limiting exists for unmask only.

## Functional gaps

- Complaints are entered by an agent (`POST /v1/complaints`). The ReverseIt customer workflow beyond that (customer-facing form, attachments upload, reopening by the customer) is not built.
- Only reversal-oriented codes have automation: FAIL001 and DUP001 (the latter always needs an approver), and UNK001 is held. Other exception types are detected or entered manually but have no automated remediation.
- The detector is driven by the transaction stream and a sweep. It needs the worker running. Without the worker, events are accepted and stored but no cases open.
- The sweep processes tenants one after another and has no per-tenant concurrency control beyond the database locks. That is fine for a demo, not for volume.
- Case assignment, escalation and SLA breach alerting are not implemented. SLA due times are calculated from approved clocks and shown, nothing acts on them.
- No load or soak testing has been done.
- Events from the AI service use the same ingestion path. There is no separate AI interface yet.

## Not run

- Nothing has been run against a real Postgres cluster with replication, a managed Redis, or behind a real proxy. Tests use one Postgres 16 and one Redis 7 container.
- CI results are reported in the pull request, not here.
