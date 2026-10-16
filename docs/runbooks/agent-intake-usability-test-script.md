# Agent Intake & First-Contact Resolution Usability Test Script

## Protocol Overview
- **Target Role**: Contact-Centre Tier-1 Representatives & Branch Customer Service Officers (5 test agents)
- **Primary Operational Metric**: First-Contact Resolution Time (FCR-T) Target: **< 45 seconds** (aligned with Ops SLA)
- **Compliance Policy**: Zero compliance violations. **Strict Prohibition**: The word *"refunded"* or promises of financial credit must NEVER be stated or implied unless the system displays financial status `VERIFIED`.
- **Audit Requirement**: Any unmasking of sensitive customer data (Account Number, PAN, Phone) requires an explicit reason code and creates an immutable audit trail entry.

---

## Agent Participant Profiles
1. **Agent 1 (Contact Centre - Inbound Transfers)**: Standard phone queue handling high-volume transfer complaints.
2. **Agent 2 (Contact Centre - Card & POS)**: Specialized in ATM dispense errors and merchant POS double-debits.
3. **Agent 3 (Branch Operations - CSO)**: In-person walk-in customer handling with account lookup.
4. **Agent 4 (Branch Operations - Relationship Associate)**: High-net-worth VIP query handling with tokenised customer lookup.
5. **Agent 5 (Escalations Desk Trainee)**: Junior representative testing usability, wording clarity, and escalation ergonomics.

---

## Test Environment Setup
- **URL**: `http://localhost:5173`
- **Navigation**: Click `Agent Intake` in the top application navigation bar.
- **Timer**: Stopwatch starts when the customer scenario statement is read; stopwatch stops when the agent quotes the approved wording to the customer and completes the intake action.

---

## Scenario Test Matrix

### Scenario 1: Active Recon Exception Match (`TXN-9901`)
*Customer Prompt*: "Hello, I sent ₦150,000 via mobile banking 4 hours ago. My account was debited, but the recipient hasn't received anything! Give me my money back right now!"
- **Agent Action**:
  1. Select `Reference / Session ID` tab or click preset `TXN-9901`.
  2. Click **Query Switch Journal**.
  3. Observe **Active Recon Exception** state:
     - Customer-safe status: *"Transaction received by destination switch; pending interbank clearing settlement."*
     - Expected next step: *"Automated settlement retry via NIBSS Settlement Bridge."*
     - Time estimate: CBN 72-hour regulatory resolution clock.
     - Major incident banner: Displays `INC-8821` (Counterparty Switch Lag).
  4. Click **Copy Approved Wording** button.
  5. Verify approved wording contains **NO** mention of *"refunded"*.
- **Target Time**: < 30 seconds
- **Pass Criteria**: Agent reassures customer using approved wording; explains automated retry without promising instant refund; logs call.

---

### Scenario 2: Transaction Verified Successful (`TXN-9902`)
*Customer Prompt*: "My web merchant claims my payment of ₦25,000 for groceries failed, but my bank app shows debited. They are refusing to release my goods!"
- **Agent Action**:
  1. Input `TXN-9902` in reference field or click preset `TXN-9902`.
  2. Click **Query Switch Journal**.
  3. Observe **Verified Successful** card:
     - Core ledger debit confirmation (Reference, Amount, Timestamp).
     - Counterparty credit confirmation (GTBank Session ID `10000420261015110293847581`, Response code `00 - Approved`).
  4. Click **Copy Quotable Settlement Evidence**.
  5. Quote the Session ID and settlement proof to the customer to present to their merchant.
- **Target Time**: < 25 seconds
- **Pass Criteria**: Agent supplies the exact GTBank Session ID proof to the customer; confirms value has definitively left the sending institution and credited the beneficiary.

---

### Scenario 3: Ambiguous Multi-Candidate Disambiguation (`₦45,000 POS`)
*Customer Prompt*: "I tried paying ₦45,000 at a supermarket POS terminal on Victoria Island earlier today. The terminal printed a decline slip, but my phone beeped with a debit alert!"
- **Agent Action**:
  1. Select `Date / Amount / Channel` search tab or click preset `₦45,000 POS`.
  2. Click **Query Switch Journal**.
  3. Observe **Multiple Candidate Transactions Found (2)** disambiguation list:
     - Candidate 1: ₦45,000 POS at 10:15:30 on Terminal `POS-VI-0881` (Status: `FAILED`).
     - Candidate 2: ₦45,000 POS at 14:22:10 on Terminal `POS-IK-0219` (Status: `SUCCESS`).
  4. Ask customer to verify approximate time and store location.
  5. Click **Select & Investigate Candidate** for Candidate 1 (`TXN-POS-4401`).
- **Target Time**: < 40 seconds
- **Pass Criteria**: Agent accurately resolves ambiguity within 2 clarifying questions; selects the disputed failed terminal session without confusion.

---

### Scenario 4: No Match Found - Structured Customer Dispute Intake (`TXN-UNKNOWN`)
*Customer Prompt*: "I made an ATM withdrawal of ₦20,000 on an off-us ATM in Abuja. Machine dispensed nothing, but debited me. I have the ATM slip right here."
- **Agent Action**:
  1. Enter reference `TXN-UNKNOWN` or click preset `TXN-UNKNOWN`.
  2. Observe **No Matching Transaction Located** state.
  3. Complete the structured **Customer Dispute Intake Form**:
     - Channel: `ATM`
     - Claimed Amount: `20000`
     - Customer Account: `0123456789`
     - Transaction Date: Current date
     - Dispute Category: `ATM Dispense Error / No Cash Dispensed`
     - Terminal / ATM ID: `ATM-ABJ-044`
     - Agent Notes: *"Customer physically present with ATM decline slip."*
  4. Click **Create Customer Dispute Docket (CUS001)**.
  5. Quote the assigned dispute docket number and 72-hour CBN turnaround commitment to the customer.
- **Target Time**: < 45 seconds
- **Pass Criteria**: Structured dispute docket successfully generated; regulatory SLA timer initiated; customer provided tracking docket number.

---

### Scenario 5: Repeat Contact & Supervisor Priority Escalation (`+234 809 111 2233`)
*Customer Prompt*: "This is my third time calling about my ₦85,000 transfer! Every agent tells me to wait 24 hours and nothing happens. I want to speak to a manager right now!"
- **Agent Action**:
  1. Select `Tokenised Customer` tab or click preset `+234 809 111 2233 (3x Repeat)`.
  2. Observe **Repeat Contact Alert**:
     - Repeat Contact Badge: `3 contacts in 24 hours` (Threshold Exceeded).
     - Supervisor Escalation Button: Prominently displayed with pulse highlight.
  3. Click **Escalate to Supervisor Queue**.
  4. In the modal, select escalation reason: `Repeated Customer Follow-up (>= 3 calls)`.
  5. Add contact note: *"Customer agitated, third call within 24h for ₦85,000 transfer. Escalated for expedited supervisor review."*
  6. Click **Confirm Escalation & Log Contact**.
- **Target Time**: < 35 seconds
- **Pass Criteria**: Immediate priority queue routing; agent remains calm and provides escalation ticket number; repeat counter audited.

---

## Scoring & Evaluation Rubric

| Metric | Target | Failure Threshold | Weight |
|---|---|---|---|
| **First-Contact Resolution Time (FCR-T)** | < 45 seconds | > 90 seconds | 30% |
| **Approved Wording Adherence** | 100% adherence | Any unapproved promise | 30% |
| **Non-Refund Compliance Rule** | Zero mentions of "refund" | Any mention of "refund" | Mandatory Gate |
| **Masked Data Integrity** | Audit reason entered | Unmasked without reason | 20% |
| **Form Accuracy (No-match intake)** | 100% required fields | Incomplete submission | 20% |

---

## Usability Test Sign-Off
- **Operations Lead**: ____________________ Date: ______________
- **Compliance Officer**: ____________________ Date: ______________
- **Head of Customer Experience**: ____________________ Date: ______________
