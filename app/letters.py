"""Complaint and escalation letter generation module.

Signature: generate_letter(kind: str, dispute: dict | object) -> str
Where kind is "bank" or "cbn".
"""

from datetime import datetime


def generate_letter(kind: str, dispute: dict | object) -> str:
    """
    Generate formal complaint letter.

    Args:
        kind: "bank" or "cbn"
        dispute: dispute record (dict or model instance) with fields:
            amount, bank, channel, reference, transaction_time, deadline, etc.

    Returns:
        Formatted complaint letter text.
    """
    # Normalize dispute data to a dictionary
    if hasattr(dispute, "to_dict"):
        data = dispute.to_dict()
    elif isinstance(dispute, dict):
        data = dispute
    else:
        data = {
            "amount": getattr(dispute, "amount", "N/A"),
            "bank": getattr(dispute, "bank", "Your Bank"),
            "channel": getattr(dispute, "channel", "N/A"),
            "reference": getattr(dispute, "reference", "N/A"),
            "transaction_time": getattr(dispute, "transaction_time", "N/A"),
            "deadline": getattr(dispute, "deadline", "N/A"),
            "status": getattr(dispute, "status", "N/A"),
        }

    amount = f"NGN {float(data.get('amount', 0)):,.2f}" if data.get("amount") is not None else "NGN 0.00"
    bank = data.get("bank", "Your Bank")
    channel = str(data.get("channel", "electronic channel")).upper()
    reference = data.get("reference", "N/A")
    txn_time = data.get("transaction_time")
    deadline = data.get("deadline")
    status = data.get("status", "awaiting_reversal")
    current_date = datetime.now().strftime("%B %d, %Y")

    kind_normalized = (kind or "").strip().lower()

    if kind_normalized == "bank":
        return f"""DISPUTE NOTICE & DEMAND FOR IMMEDIATE REVERSAL

Date: {current_date}

To:
The Head of Customer Care / Electronic Banking Complaints
{bank}

SUBJECT: FORMAL DISPUTE REGARDING FAILED/UNREVERSED {channel} DEBIT OF {amount}

Dear Sir/Madam,

I am writing to formally lodge a dispute regarding an unreversed failed transaction debited from my account. Despite the failed transaction status, my account was debited without automatic reversal within the stipulated regulatory turnaround period.

TRANSACTION DETAILS:
- Bank Name: {bank}
- Amount Debited: {amount}
- Payment Channel: {channel}
- Transaction Reference: {reference}
- Date & Time of Transaction: {txn_time}
- Reversal Deadline: {deadline}
- Current Dispute Status: {status}

Under the Central Bank of Nigeria (CBN) Consumer Protection Framework and guidelines on Electronic Payment Channels, failed electronic transactions must be resolved within regulatory timelines. As the stipulated resolution timeframe has elapsed or is due, I hereby demand the immediate reversal and refund of the debited sum ({amount}) to my account.

Kindly treat this notice with urgency and confirm the refund with the reversal session ID.

Sincerely,
Aggrieved Customer
Reference: {reference}
"""

    elif kind_normalized == "cbn":
        return f"""PETITION TO THE CENTRAL BANK OF NIGERIA (CONSUMER PROTECTION DEPARTMENT)

Date: {current_date}

To:
The Director, Consumer Protection Department (CPD)
Central Bank of Nigeria (CBN)
Central Business District, Abuja, Nigeria
Email: cpd@cbn.gov.ng

SUBJECT: PETITION AGAINST {bank.upper()} FOR FAILURE TO RESOLVE UNREVERSED TRANSACTION - REF: {reference}

Dear Sir/Madam,

I wish to bring to the attention of the Central Bank of Nigeria a breach of electronic transaction guidelines and consumer protection directives by {bank}.

DETAILS OF THE COMPLAINT:
- Financial Institution: {bank}
- Disputed Sum: {amount}
- Channel of Transaction: {channel}
- Transaction Reference / Session ID: {reference}
- Transaction Timestamp: {txn_time}
- Regulatory Resolution Deadline: {deadline}
- Dispute Status: {status}

SUMMARY OF GRIEVANCE:
On {txn_time}, the sum of {amount} was debited from my account via {channel} without delivery of value. The matter was brought to the bank's attention, and the CBN stipulated resolution turnaround time ({deadline}) has lapsed without refund or resolution by {bank}.

PRAYERS:
In line with the CBN Circular on Sanctions for Failure to Refund Failed Transactions and the CBN Consumer Protection Framework, I respectfully request:
1. That the CBN mandates {bank} to immediately credit my account with the full debited sum of {amount}.
2. That statutory penalties and default interest be levied against {bank} for non-compliance with dispute resolution timelines.

Attached are copies of the transaction debit alert and formal correspondence.

Respectfully submitted,
Complainant
Ref: {reference}
"""
    else:
        raise ValueError(f"Invalid letter kind '{kind}'. Expected 'bank' or 'cbn'.")
