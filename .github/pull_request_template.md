## 📝 Description

Please provide a concise summary of the changes introduced in this pull request and the financial operations/reconciliation problem it solves.

---

## 🏷️ Change Category

- [ ] Core Banking / Switch Adapter Integration
- [ ] Reconciliation Engine / Discrepancy Matching
- [ ] Exception Investigation & Case Management
- [ ] Controlled Reversal & Maker-Checker Workflows
- [ ] AI Service / Anomaly Detection Models
- [ ] Web Operations Console UI / UX
- [ ] Database Migration & Schemas
- [ ] Security, Audit & Observability
- [ ] Infrastructure & CI/CD

---

## 🛡️ Financial Integrity & Maker-Checker Checklist

- [ ] Does this change preserve transactional idempotency?
- [ ] Have financial calculations avoided floating-point arithmetic (using `NUMERIC(18,4)` / decimal)?
- [ ] Are sensitive customer details (PAN, account numbers, tokens) redacted from logs?
- [ ] Does this change require an updated audit trail schema?
- [ ] Is dual-authorization enforced for any new destructive or financial action?

---

## 🧪 Testing Verification

- [ ] Unit tests added / updated and passing
- [ ] Integration tests verified with local PostgreSQL and Redis
- [ ] Docker Compose build succeeds cleanly
- [ ] Code formatted and linted (`make lint`)

---

## 🔗 Related Internal Ticket

- Closes: #
