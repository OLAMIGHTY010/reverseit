# ReverseIt customer backend (B10)

Customer-reported and bot-detected cases share one queue, one set of clocks and one audit trail. This document grows with each part of B10.

## Customer identity (approved design)

- A **separate token audience** (`smartreversal-customer`). A customer token works on no operator route and an operator token on no customer route; tests prove both directions.
- Customers are rows in `customers`, not users. The accounts a customer owns are recorded by the bank in `customer_accounts` (account tokens only). **Nothing a customer pastes, types or uploads, such as an alert or an account number, is ever proof of ownership.** There is no customer-facing way to link an account.
- Sign-in is behind the `customer.Provider` interface. `DevProvider` checks a development passphrase for **synthetic** customers: argon2id hash, a lock after five failures for 15 minutes, identical 401 for an unknown institution, unknown customer, wrong passphrase, disabled or locked customer, and a locked customer's attempts are not counted. It is enabled with `CUSTOMER_DEV_LOGIN_ENABLED=true` and the configuration loader **refuses it in production**. A bank identity provider is an external dependency and replaces it without other changes.
- Every successful sign-in is audited; a sign-in that cannot be audited does not happen.
- `POST /v1/customer/auth/login` and `GET /v1/customer/me` exist only when the provider is configured.

## Deadline engine (`internal/deadline`)

The dispute type to clock mapping is fixed in code (scope Section 10.4) and never chosen by a model. Every duration comes from approved configuration (the regulatory clock registry and the keys `internal_target_hours.<CLOCK>`, `customer.ack_target_hours`, `deadline.approaching_percent`, `deadline.urgent_percent`). A clock nobody approved yields no deadline and the result says which key is missing. The regulatory deadline and the bank's internal target are kept apart; the earlier one binds and is what a customer is shown as "your bank target". The clock's registry status (confirmed, reported, draft, conflicting) travels with the deadline. Working-day clocks use the bank calendar and holidays. The approaching and urgent states exist only when percentages are approved.

All clock values in this build are unverified synthetic policy.
