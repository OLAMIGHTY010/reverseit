# SMARTREVERSAL — AI Architecture, Governance and Data Protection Specification (A0)

## 1. Executive Summary & Purpose
This document establishes the architectural blueprints, governance boundaries, security postures, and deployment constraints for all Artificial Intelligence (AI) and Machine Learning (ML) subsystems within SMARTREVERSAL.

SMARTREVERSAL is an **enterprise operations intelligence and controlled-resolution platform** deployed in Nigerian banking environments (e.g., Wema Bank / ALAT ecosystem). The AI/ML subsystems serve strictly as **read-only assistance, decision support, and explainability layers**.

---

## 2. Non-Negotiable Invariants & Financial Safety Rules

1. **Zero Financial Mutation**: AI/ML models **never** initiate reversals, modify account balances, or approve money movement. All remediation decisions pass through the deterministic 8-check reversal gate (Section 6.3) and human maker-checker controls.
2. **Deterministic Precedence**: Statistical predictions or language model confidence scores **never** override deterministic rules, approval limits, or regulatory clocks.
3. **Regulatory Clocks are Deterministic (R6 / N10)**: The LLM **never** selects a regulatory timeline or deadline. The dispute intent classifier proposes a dispute category (e.g., `failed_transfer`, `duplicate_debit`), which is deterministically mapped to the clock registry (Section 10.4) using configured CBN/NIBSS values.
4. **Mandatory Evidence Citation**: Every claim, fact statement, or root-cause inference produced by an LLM must cite concrete, stable `evidence_id`s from the verified evidence bundle. Statements failing citation validation are stripped by schema enforcement.
5. **Two-Stage Customer Notice Language (R7 / M4)**: AI text generation must strictly enforce approved vocabulary: words like `"refunded"`, `"reversed"`, or `"credited"` are forbidden until the financial status is verified in core ledger records.
6. **Synthetic Data Policy**: Outside certified banking sandboxes, all development, testing, and pitch demonstrations operate on synthetic data. Demo metrics must be prominently labeled as synthetic.

---

## 3. Capability Inventory: Three-Tier Governance Model

All AI/ML capabilities are classified into one of three strict tiers:

```mermaid
graph TD
    subgraph Tier 1: Language Assistance [Read-Only Assistance]
        A2[A2: Investigation Summaries]
        A2B[A2: Case Explanation for Ops Reviewer]
        A6[A6: Customer-Safe Status Wording]
        A3_MISS[A3: Missing-Info Identification]
        A4_NAR[A4: Incident Narratives]
        A9_PARSE[A9: Alert Parsing]
        A9_DRAFT[A9: Complaint Drafting]
    end

    subgraph Tier 2: Analytics & Detection [Advisory Scoring]
        A4_SPIKE[A4: Statistical Spike & Anomaly Detection]
        A5_PRIO[A5: Priority Harm Ranking]
        A5_DUP[A5: Duplicate Likelihood Scorer]
        A5_HAZ[A5: Pending Resolution Probability]
        A6_MATCH[A6: Complaint-to-Transaction Matcher]
        A9_AUTH[A9: Advisory Alert Authenticity Signals]
    end

    subgraph Tier 3: Recommendations [Human-in-the-Loop]
        A3_REC[A3: Next-Best Check Recommendations]
        A7_RULE[A7: Rule-Improvement Suggestions]
        A4_BULK[A4: Incident Bulk-Action Proposal]
    end

    Tier 1 --> DeterministicGate[Deterministic 8-Check Gate / Human Review]
    Tier 2 --> DeterministicGate
    Tier 3 --> DeterministicGate
    DeterministicGate --> LedgerExecution[Core Banking / Switch Action]
```

### Tier 1: Language Assistance (Read-Only)
- **Role**: Synthesizes complex multi-system transaction timelines, drafts structured notes, and parses unstructured input into schema-validated payloads.
- **Scope**:
  - `investigation_summary`: Generates factual summaries of transactions across switch, ledger, and gateway logs with citations.
  - `case_explanation`: Formulates plain-language explanation of why an exception was flagged for ops reviewers.
  - `missing_info_detection`: Highlights absent or stale records required by the 8-check gate.
  - `alert_parsing`: Extracts amount, account tail, timestamp, and reference from customer alerts.
  - `complaint_drafting`: Produces neutral, factual complaint letters for customer submission (CUS001/CUS002).
  - `customer_safe_wording`: Generates status text enforcing two-stage customer messaging.

### Tier 2: Analytics & Detection (Advisory, Non-Decisive)
- **Role**: Flags statistical patterns, evaluates priority, and provides advisory scores alongside deterministic rule outcomes.
- **Scope**:
  - `anomaly_spike_detection`: Detects unexpected surges in failure rates or timeouts using robust rolling statistics (EWMA, control charts) and Isolation Forest.
  - `priority_ranking`: Computes priority queues based on customer harm, amount, SLA countdown, and repeat contacts.
  - `duplicate_likelihood_scoring`: Evaluates borderline duplicate debit candidates beyond the 30-second rule window using gradient boosting (advisory to human reviewer).
  - `pending_resolution_probability`: Estimates survival curves for pending transactions to optimize re-query ladder intervals.
  - `complaint_matching`: Constrained fuzzy matching linking customer dispute inquiries to recorded ledger/switch transactions.

### Tier 3: Recommendations (Human-in-the-Loop)
- **Role**: Suggests investigation steps or rule optimizations for authorized human approval.
- **Scope**:
  - `next_best_check`: Proposes which system (e.g., switch TSQ, ATM journal, or ledger query) will resolve missing evidence with least latency.
  - `rule_change_suggestion`: Analyzes false-positive/false-negative trends and proposes versioned declarative rule adjustments in shadow mode.

---

## 4. AI Gateway Architecture & Sandboxing

The **AI Gateway** in `apps/ai-service` acts as the mandatory single entry and exit point for all AI capabilities.

```
                      +------------------------------------------+
                      |               AI GATEWAY                 |
                      |                                          |
                      |  1. Tenant Isolation & Auth Token Check  |
                      |  2. PII Tokenization & Masking Engine    |
                      |  3. Rate Limiting & Token Budgeting      |
                      |  4. Kill Switch Verification             |
                      +---------------------+--------------------+
                                            |
                         +------------------+------------------+
                         |                                     |
                         v                                     v
             [External LLM / Cloud API]             [Local/On-Prem Model]
             (Masked Payloads Only)                 (Ollama / vLLM / Slurm)
                         |                                     |
                         +------------------+------------------+
                                            |
                      +---------------------v--------------------+
                      |  5. Structured JSON Schema Validator     |
                      |  6. Citation & Evidence ID Verifier      |
                      |  7. Customer-Safe Wording Filter         |
                      |  8. Redacted Audit & Telemetry Logging   |
                      +------------------------------------------+
```

### Architectural Controls
1. **Tenant-Scoped Context**: Context retrieval queries explicitly enforce `institution_id = current_tenant`. Cross-tenant prompt contamination is strictly blocked.
2. **PII Masking & Minimization (FR-54 / N12)**:
   - Account numbers are tokenized or masked as `****1234`.
   - Customer names and balances are redacted before payload leaves memory.
   - Raw alert text and screenshots are never sent unredacted to external models.
3. **Model & Prompt Registry**: Every AI invocation logs the active `model_id` (with version), `prompt_id` (with hash and version), timestamp, latency, and token counts.
4. **Structured JSON Output**: All model calls enforce strict Pydantic schemas. Unparsable or schema-invalid responses trigger immediate fallback to rule-based templates.
5. **Read-Only Tool Execution Allow-List**:
   - Authorized tools: `get_case`, `get_evidence`, `search_similar_cases`, `get_incident`, `get_policy_summary`, `get_source_freshness`.
   - Any mutating call (`execute_reversal`, `modify_policy`, `change_case_status`) raises `GovernanceViolationError`.
6. **Timeouts & Circuit Breaker**: External model calls have a strict 10.0-second timeout. If the circuit trips, the system instantly switches to deterministic templates with no customer interruption.

---

## 5. Deployment Options & Data Residency (NDPA / CBN)

| Deployment Option | Architecture | Data Residency | Legal & Risk Posture | Recommended Phase |
| :--- | :--- | :--- | :--- | :--- |
| **Option A: On-Premise / Private Cloud** | Local vLLM/Ollama running quantized open models (Llama-3.1 8B, Mistral Nemo) on bank infrastructure | 100% within Nigerian borders (bank data center) | Zero cross-border exposure; satisfies NDPA and strict CBN InfoSec rules | **Production Target (Track P)** |
| **Option B: Hybrid with Masked External API** | Local PII tokenization layer; only sanitized/tokenized text sent to Azure/OpenAI/Anthropic | Tokenized data egresses to cloud region | Requires explicit CMP/LEG review of Data Processing Agreements (DPA) | Optional pilot / evaluation |
| **Option C: Mock Provider & Rule Fallbacks** | Deterministic rule-based synthesizers, regex parsers, and statistical models in Python | In-memory / Local | 100% self-contained; zero cloud dependency; deterministic output | **Track H (Hackathon) & Resilient Fallback** |

### Data Egress Rules (Answer to Mentor Question 3)
Under no circumstances do real customer names, unmasked 10-digit NUBAN account numbers, BVNs, NINs, card PANs, or actual account balances leave the bank's boundary. When using cloud APIs, only tokenized synthetic identifiers (e.g., `acct_tok_93af21`, `TXN-0001842`) and sanitized dispute facts are transmitted.

---

## 6. Feature Kill Switches & Fallbacks (Master-Prompt S9)

Kill switches are maintained in `KillSwitchManager` across 4 levels of granularity:
1. **Global Switch**: Instantly freezes all AI/ML components across the entire platform.
2. **Rail Switch**: Disables AI assistance for a specific rail (e.g., `NIP`, `POS`, `ATM`).
3. **Exception Type Switch**: Disables AI assistance for an exception class (e.g., `EX-DUP`, `EX-DNC`).
4. **Feature Switch**: Toggles individual features (`summaries`, `copilot`, `duplicate_scoring`, `anomaly_detection`, `alert_parsing`).

### Fallback Mapping
| Feature | AI Mechanism | Fallback Mechanism When Tripped |
| :--- | :--- | :--- |
| **Case Summary** | LLM synthesis with citations | Deterministic string template populating verified facts from ledger/switch |
| **Duplicate Scoring** | Gradient Boosting likelihood | Hard 30-second window candidate rule + reference equality check |
| **Dispute Classification** | LLM intent extraction | Keyword regex + guided radio selection in ReverseIt UI |
| **Complaint Drafting** | LLM phrasing & polishing | Fixed formal complaint template populated with structured fields |
| **Anomaly Detection** | Isolation Forest multi-feature spike | Rolling 3-sigma statistical threshold on hourly failure counts |
| **Copilot Panel** | Interactive tool-calling agent | Static evidence panel with raw timeline and link tabs |
