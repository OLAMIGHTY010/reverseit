"""Unit tests for PII Masking and Tokenization Service.

Verifies Prompt A1 (Item 3), FR-54, and Privacy by Design:
- Redaction of 10-digit NUBAN account numbers
- Redaction of 11-digit BVNs and NINs
- Masking of card PANs, emails, and phone numbers
- Balance scrubbing from alert text
- Recursive masking of nested payloads
"""

import pytest
from app.services.masking import PIIMaskingService


def test_account_masking_and_tokenization():
    acct = "0123456789"
    # Display masking
    masked = PIIMaskingService.mask_account_display(acct)
    assert masked == "****6789"

    # Deterministic tokenization
    token1 = PIIMaskingService.tokenize_account(acct)
    token2 = PIIMaskingService.tokenize_account(acct)
    assert token1.startswith("acct_tok_")
    assert token1 == token2


def test_text_masking_balances_and_ids():
    alert_text = (
        "Acct: 0123456789. Debit: NGN 25,000.00. Time: 14-Oct 10:15. "
        "Avl Bal: NGN 180,450.50. Ref: NIP-10294. Help: support@wemabank.com or 08031234567"
    )
    sanitized = PIIMaskingService.mask_text(alert_text)

    # 10-digit NUBAN masked
    assert "0123456789" not in sanitized
    assert "****6789" in sanitized

    # Transaction amount preserved
    assert "25,000.00" in sanitized

    # Balance redacted
    assert "180,450.50" not in sanitized
    assert "[BALANCE_REDACTED]" in sanitized

    # Email & phone redacted
    assert "support@wemabank.com" not in sanitized
    assert "[EMAIL_REDACTED]" in sanitized
    assert "08031234567" not in sanitized
    assert "[PHONE_REDACTED]" in sanitized


def test_bvn_and_card_pan_redaction():
    text = "Customer BVN is 22184950281 and card is 5399-4120-8472-1092 used at POS."
    sanitized = PIIMaskingService.mask_text(text)

    # 11-digit BVN redacted
    assert "22184950281" not in sanitized
    assert "[ID_11DIGIT_REDACTED]" in sanitized

    # Card PAN masked to last 4 digits
    assert "5399-4120-8472-1092" not in sanitized
    assert "**** **** **** 1092" in sanitized


def test_recursive_payload_masking():
    payload = {
        "account_number": "0123456789",
        "beneficiary_account": "0987654321",
        "customer_name": "Olatunbosun Babatunde",
        "available_balance": 500000.00,
        "metadata": {
            "email": "customer@example.ng",
            "phone": "08099887766",
            "notes": "Customer reported debit on account 0123456789 with balance 500,000"
        }
    }

    masked = PIIMaskingService.mask_payload(payload)

    assert masked["account_number"] == "****6789"
    assert masked["beneficiary_account"] == "****4321"
    assert masked["customer_name"] == "[NAME_REDACTED]"
    assert masked["available_balance"] == "[BALANCE_REDACTED]"
    assert masked["metadata"]["email"] == "[EMAIL_REDACTED]"
    assert masked["metadata"]["phone"] == "[PHONE_REDACTED]"
    assert "0123456789" not in masked["metadata"]["notes"]
