"""PII Masking and Tokenization Engine for SMARTREVERSAL AI Gateway.

Implements Prompt A1 (Item 3), FR-54, and Privacy by Design:
- Tokenizes or masks 10-digit Nigerian NUBAN account numbers (e.g. '****1234' or token)
- Redacts 11-digit BVN and NIN identifiers
- Redacts Nigerian phone numbers, emails, and card PANs
- Redacts monetary balances in raw text while preserving transaction amounts
- Recursively masks nested dictionaries and lists before LLM transmission
"""

import re
import hashlib
from typing import Any, Dict, List, Union

# Regex patterns for Nigerian financial identifiers and PII
_NUBAN_REGEX = re.compile(r'\b(?:\d{10})\b')
_BVN_NIN_REGEX = re.compile(r'\b(?:\d{11})\b')
_CARD_PAN_REGEX = re.compile(r'\b(?:\d{4}[-\s]?){3}\d{4}\b')
_PHONE_REGEX = re.compile(r'(?:\+?234|0)[789]\d{9}\b')
_EMAIL_REGEX = re.compile(r'\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b')

# Balance patterns in SMS/Email alerts (e.g., "Bal: NGN 45,000.00", "Available Bal: 120,400.50")
_BALANCE_REGEX = re.compile(
    r'(?i)(?:bal(?:ance)?|avl(?:\s+bal)?|acct\s+bal(?:ance)?)\s*(?::|is)?\s*(?:NGN|N|₦)?\s*([0-9,]+(?:\.\d{2})?)'
)


class PIIMaskingService:
    """Provides robust, deterministic PII redaction and tokenization."""

    @staticmethod
    def tokenize_account(account_number: str, salt: str = "SMARTREVERSAL_SALT") -> str:
        """Returns deterministic token for an account number e.g. acct_tok_a1b2c3."""
        clean_acct = re.sub(r'\D', '', account_number)
        if len(clean_acct) < 4:
            return "acct_tok_invalid"
        hash_digest = hashlib.sha256(f"{salt}:{clean_acct}".encode('utf-8')).hexdigest()[:8]
        return f"acct_tok_{hash_digest}"

    @staticmethod
    def mask_account_display(account_number: str) -> str:
        """Masks account number to last 4 digits: ****1234."""
        clean_acct = re.sub(r'\D', '', account_number)
        if len(clean_acct) <= 4:
            return "****"
        return f"****{clean_acct[-4:]}"

    @classmethod
    def mask_text(cls, text: str, preserve_ref: bool = True) -> str:
        """Sanitizes PII in unstructured text strings (alerts, narrations, notes)."""
        if not text:
            return ""

        result = text

        # 1. Redact Balances
        result = _BALANCE_REGEX.sub(r'Bal: [BALANCE_REDACTED]', result)

        # 2. Redact Card PANs
        def _mask_pan(match):
            pan = re.sub(r'\D', '', match.group(0))
            return f"**** **** **** {pan[-4:]}"
        result = _CARD_PAN_REGEX.sub(_mask_pan, result)

        # 3. Redact Emails
        result = _EMAIL_REGEX.sub('[EMAIL_REDACTED]', result)

        # 4. Redact Phone Numbers
        result = _PHONE_REGEX.sub('[PHONE_REDACTED]', result)

        # 5. Redact BVN / NIN (11 digits)
        result = _BVN_NIN_REGEX.sub('[ID_11DIGIT_REDACTED]', result)

        # 6. Redact NUBAN Account Numbers (10 digits)
        # We replace standalone 10-digit numbers with masked representation
        def _mask_nuban(match):
            nuban = match.group(0)
            return f"****{nuban[-4:]}"
        result = _NUBAN_REGEX.sub(_mask_nuban, result)

        return result

    @classmethod
    def mask_payload(cls, data: Any) -> Any:
        """Recursively sanitizes dictionaries, lists, and primitives."""
        if isinstance(data, dict):
            masked_dict = {}
            for k, v in data.items():
                lower_k = k.lower()
                # Specific key-based redactions
                if any(x in lower_k for x in ["account_number", "debit_account", "beneficiary_account"]):
                    if isinstance(v, str):
                        masked_dict[k] = cls.mask_account_display(v)
                    else:
                        masked_dict[k] = v
                elif any(x in lower_k for x in ["bvn", "nin"]):
                    masked_dict[k] = "[ID_REDACTED]"
                elif any(x in lower_k for x in ["balance", "ledger_balance", "available_balance"]):
                    masked_dict[k] = "[BALANCE_REDACTED]"
                elif any(x in lower_k for x in ["customer_name", "account_name"]):
                    masked_dict[k] = "[NAME_REDACTED]"
                elif any(x in lower_k for x in ["phone", "mobile"]):
                    masked_dict[k] = "[PHONE_REDACTED]"
                elif any(x in lower_k for x in ["email"]):
                    masked_dict[k] = "[EMAIL_REDACTED]"
                elif isinstance(v, str):
                    masked_dict[k] = cls.mask_text(v)
                else:
                    masked_dict[k] = cls.mask_payload(v)
            return masked_dict
        elif isinstance(data, list):
            return [cls.mask_payload(item) for item in data]
        elif isinstance(data, str):
            return cls.mask_text(data)
        else:
            return data


masking_service = PIIMaskingService()
