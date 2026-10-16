# Security Policy & Financial Data Protection

## 1. Classification & Scope
SMARTREVERSAL is an internal, enterprise financial operations platform handling sensitive transaction logs, reconciliation records, exception workflows, and resolution requests. 

This repository is **PROPRIETARY and CONFIDENTIAL**. Unauthorized distribution, exposure of source code, or leakage of configuration templates is strictly prohibited.

---

## 2. Main Branch Protection Policy
To safeguard production financial integrity, direct writes to `main` are restricted:
- **No Direct Commits**: Developers and operators must not push directly to `main`.
- **Branch Hierarchy**: All development workflows flow into `develop` through reviewed Pull Requests.
- **Pull Request Mandate**: Merges into `main` must come from `develop` (or verified hotfix branches) and require:
  - At least 2 authorized peer reviews (Senior Staff / Security lead).
  - Passing automated CI checks (linters, type-checks, security SAST, test suites).
  - Clean commit history without merge artifacts or broken intermediate states.
- **Signed Commits**: All commits targeting release branches must be cryptographically signed.

---

## 3. Financial Data Integrity & Secrets Management
- **Zero Credentials in Git**: Never commit credentials, private keys, API tokens, webhook signing secrets, or production connection strings.
- **Environment Isolation**: Production environments use hardware security modules (HSM) or dedicated secret vaults (e.g., HashiCorp Vault, AWS Secrets Manager).
- **PCI-DSS & PII Redaction**: Primary Account Numbers (PAN), CVVs, and sensitive customer identifiers must be masked at ingestion. Only tokenized or masked identifiers are persisted in audit trails and application logs.

---

## 4. Reporting a Security Vulnerability
If you discover a potential vulnerability, security flaw, or financial integrity risk within SMARTREVERSAL:
1. **DO NOT** create a public or team-wide open issue.
2. Immediately notify the internal Security Engineering team via:
   - **Internal Security Desk**: `security@smartreversal.internal`
   - **Urgent SecOps Pager**: Security Incident Channel (`#secops-financial-integrity`)
3. Include:
   - Severity assessment (CVSS if applicable).
   - Detailed reproduction steps or proof-of-concept.
   - Affected services (`apps/api`, `apps/ai-service`, `apps/web`, or infrastructure).
   - Potential impact on transaction processing or reversal operations.
