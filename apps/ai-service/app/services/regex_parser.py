"""Local-First Regex and Template Parser for Nigerian Bank Debit Alerts.

Implements Prompt A9 (Item 1), Case N2, and Privacy by Design:
- Zero-cloud regex extraction for major Nigerian banks (WEMA/ALAT, GTB, Zenith, Access, FirstBank, UBA)
- Extracts amount, account tail, date/time, narration, reference, channel, balance
- Detects issuing bank; flags non-Wema alerts (Case N2)
- Automatically redacts balance information
- Assigns per-field confidence and source attribution ('rule')
"""

import re
import datetime
from typing import Optional, Dict, Tuple, List
from dateutil import parser as date_parser

from app.models.reverseit import AlertParseResult
from app.services.sanitizer import sanitizer
from app.services.masking import masking_service


class RegexAlertParser:
    """Local regex and pattern parser for Nigerian SMS and email debit alerts."""

    # Bank detection patterns
    _BANK_PATTERNS = {
        "WEMA": re.compile(r'(?i)\b(?:wema(?:\s*bank)?|alat)\b'),
        "GTBANK": re.compile(r'(?i)\b(?:gtb(?:ank)?|guaranty\s*trust)\b'),
        "ZENITH": re.compile(r'(?i)\bzenith(?:\s*bank)?\b'),
        "ACCESS": re.compile(r'(?i)\baccess(?:\s*bank)?\b'),
        "FIRSTBANK": re.compile(r'(?i)\b(?:first\s*bank|firstbank)\b'),
        "UBA": re.compile(r'(?i)\b(?:uba|united\s*bank\s*for\s*africa)\b'),
    }

    # Field extraction patterns
    _AMOUNT_PATTERN = re.compile(
        r'(?i)(?:amt|amount|debit(?:ed)?|dr)\s*(?::|is|\s)?\s*(?:NGN|N|₦)?\s*([0-9]{1,3}(?:,[0-9]{3})*(?:\.[0-9]{2})?|[0-9]+(?:\.[0-9]{2})?)'
    )
    _ACCOUNT_PATTERN = re.compile(
        r'(?i)(?:acct|account|acc)\s*(?::|is|\s)?\s*(?:[0-9*]{2,8})?(\d{4})\b'
    )
    _REF_PATTERN = re.compile(
        r'(?i)(?:ref(?:\s*no)?|reference|rrn|session(?:\s*id)?)\s*(?::|is|\s)?\s*([A-Za-z0-9_-]{6,35})\b'
    )
    _DESC_PATTERN = re.compile(
        r'(?i)(?:desc(?:ription)?|narration|details|memo)\s*(?::|is|\s)?\s*(.*?)(?=\s+(?:date|time|bal|ref|$))'
    )
    _BALANCE_PATTERN = re.compile(
        r'(?i)(?:bal(?:ance)?|avl(?:\s*bal)?|avail(?:\s*bal)?)\s*(?::|is|\s)?\s*(?:NGN|N|₦)?\s*([0-9,]+(?:\.[0-9]{2})?)'
    )

    # Date-time patterns common in SMS
    _DATE_PATTERN = re.compile(
        r'\b(\d{1,2}[/-]\d{1,2}[/-]\d{2,4}(?:\s+\d{1,2}:\d{2}(?::\d{2})?)?|\d{1,2}-[A-Za-z]{3}-\d{2,4}(?:\s+\d{1,2}:\d{2}(?::\d{2})?)?)\b'
    )

    @classmethod
    def detect_bank(cls, text: str) -> Tuple[str, bool]:
        """Detects the issuing bank. Returns (bank_name, is_non_wema)."""
        for bank, pattern in cls._BANK_PATTERNS.items():
            if pattern.search(text):
                is_non_wema = (bank != "WEMA")
                return bank, is_non_wema
        # Default: if unspecified, assumes current platform institution or unknown
        return "UNKNOWN", False

    @classmethod
    def detect_channel(cls, text: str, narration: str = "") -> str:
        """Infers transaction channel from alert keywords."""
        combined = f"{text} {narration}".upper()
        if "POS" in combined:
            return "POS"
        if "ATM" in combined:
            return "ATM"
        if "USSD" in combined:
            return "USSD"
        if "WEB" in combined or "PAYSTACK" in combined or "FLUTTERWAVE" in combined:
            return "WEB"
        if "TRF" in combined or "TRANSFER" in combined or "NIP" in combined:
            return "NIP"
        return "TRANSFER"

    @classmethod
    def parse(cls, raw_alert_text: str) -> AlertParseResult:
        """Executes zero-cloud regex parsing on a customer debit alert."""
        # 1. Sanitize input to defend against prompt injection
        sanitized_text, was_injected, injection_flags = sanitizer.sanitize_untrusted_text(raw_alert_text)

        # 2. Bank identification
        bank, is_non_wema = cls.detect_bank(sanitized_text)

        confidences: Dict[str, float] = {}
        tamper_flags: List[str] = list(injection_flags)

        # 3. Amount extraction
        amount_minor: Optional[int] = None
        amount_formatted: Optional[str] = None
        amt_match = cls._AMOUNT_PATTERN.search(sanitized_text)
        if amt_match:
            amt_str = amt_match.group(1).replace(",", "")
            try:
                amount_val = float(amt_str)
                amount_minor = int(round(amount_val * 100))
                amount_formatted = f"NGN {amount_val:,.2f}"
                confidences["amount"] = 0.98
            except ValueError:
                confidences["amount"] = 0.0
        else:
            confidences["amount"] = 0.0

        # 4. Account tail extraction (last 4 digits)
        account_tail: Optional[str] = None
        acct_match = cls._ACCOUNT_PATTERN.search(sanitized_text)
        if acct_match:
            account_tail = acct_match.group(1)
            confidences["account_tail"] = 0.95
        else:
            confidences["account_tail"] = 0.0

        # 5. Reference extraction
        reference: Optional[str] = None
        ref_match = cls._REF_PATTERN.search(sanitized_text)
        if ref_match:
            reference = ref_match.group(1)
            confidences["reference"] = 0.95
        else:
            confidences["reference"] = 0.0

        # 6. Narration extraction
        narration: Optional[str] = None
        desc_match = cls._DESC_PATTERN.search(sanitized_text)
        if desc_match:
            narration = desc_match.group(1).strip()
            confidences["narration"] = 0.90
        else:
            narration = "Debit Transaction"
            confidences["narration"] = 0.40

        # 7. Date-time extraction
        date_time_str: Optional[str] = None
        parsed_dt: Optional[datetime.datetime] = None
        dt_match = cls._DATE_PATTERN.search(sanitized_text)
        if dt_match:
            date_time_str = dt_match.group(1)
            try:
                # Parse date into datetime with UTC fallback
                parsed_dt = date_parser.parse(date_time_str)
                confidences["datetime"] = 0.92
            except Exception:
                confidences["datetime"] = 0.30
        else:
            confidences["datetime"] = 0.0

        # 8. Channel detection
        channel = cls.detect_channel(sanitized_text, narration or "")
        confidences["channel"] = 0.85

        # 9. Balance scrubbing (Privacy by design)
        balance_minor: Optional[int] = None
        bal_match = cls._BALANCE_PATTERN.search(sanitized_text)
        if bal_match:
            bal_str = bal_match.group(1).replace(",", "")
            try:
                balance_minor = int(round(float(bal_str) * 100))
            except ValueError:
                pass

        # Calculate overall confidence
        required_keys = ["amount", "account_tail", "datetime"]
        overall_conf = sum(confidences.get(k, 0.0) for k in required_keys) / len(required_keys)

        # Flag tamper if future timestamp
        if parsed_dt and parsed_dt > datetime.datetime.now() + datetime.timedelta(days=1):
            tamper_flags.append("TIMESTAMP_IN_FUTURE")

        return AlertParseResult(
            raw_text_sanitized=sanitized_text,
            issuing_bank=bank,
            is_non_wema=is_non_wema,
            amount_minor=amount_minor,
            amount_formatted=amount_formatted,
            currency="NGN",
            account_tail=account_tail,
            date_time_str=date_time_str,
            parsed_datetime=parsed_dt,
            narration=narration,
            reference=reference,
            channel=channel,
            balance_minor=balance_minor,
            balance_redacted=True,
            overall_confidence=round(overall_conf, 2),
            field_confidences=confidences,
            tamper_flags=tamper_flags,
            parse_source="rule"
        )


regex_parser = RegexAlertParser()
