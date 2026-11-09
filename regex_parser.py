import math
import re
from datetime import datetime


KNOWN_BANKS = {
    "access": "Access Bank",
    "gtb": "GTBank",
    "gtbank": "GTBank",
    "guaranty trust": "GTBank",
    "zenith": "Zenith Bank",
    "first bank": "First Bank",
    "fbn": "First Bank",
    "uba": "UBA",
    "united bank for africa": "UBA",
    "opay": "OPay",
    "palmpay": "PalmPay",
    "kuda": "Kuda Bank",
    "moniepoint": "Moniepoint",
    "stanbic": "Stanbic IBTC",
    "fcmb": "FCMB",
    "sterling": "Sterling Bank",
    "wema": "Wema Bank",
    "alat": "Wema Bank",
    "fidelity": "Fidelity Bank",
    "union": "Union Bank",
}

_CREDIT_PATTERN = re.compile(
    r"\bcredit\s+alert\b|\bcr\s*:|\bcredited\s+with\b|"
    r"\breceived\s+from\b|\bcredit\s+of\b",
    re.IGNORECASE,
)
_OUTGOING_PATTERN = re.compile(
    r"\b(?:debit(?:ed)?|dr|withdraw(?:n|al)?|spent|paid|purchase|payment|"
    r"transfer\s+sent|sent\s+to)\b",
    re.IGNORECASE,
)
_CURRENCY = r"(?:ngn|₦|(?<![A-Za-z])n(?![A-Za-z]))"
_DEBIT_AMOUNT_PATTERN = re.compile(
    r"\b(?:amt|amount|debit|dr|debited|spent|paid)\s*:?\s*"
    rf"{_CURRENCY}?\s*([\d,]+(?:\.\d{{1,2}})?)",
    re.IGNORECASE,
)
_CURRENCY_AMOUNT_PATTERN = re.compile(
    rf"{_CURRENCY}\s*([\d,]+(?:\.\d{{1,2}})?)",
    re.IGNORECASE,
)
_BALANCE_PREFIX_PATTERN = re.compile(
    r"(?:available\s+bal|avail\s+bal|balance|bal)\s*:?\s*"
    rf"{_CURRENCY}?\s*$",
    re.IGNORECASE,
)


def _parse_amount(text: str) -> float | None:
    for match in _DEBIT_AMOUNT_PATTERN.finditer(text):
        amount = _to_amount(match.group(1))
        if amount is not None:
            return amount

    for match in _CURRENCY_AMOUNT_PATTERN.finditer(text):
        prefix = text[max(0, match.start() - 60):match.start()]
        if _BALANCE_PREFIX_PATTERN.search(prefix):
            continue
        amount = _to_amount(match.group(1))
        if amount is not None:
            return amount
    return None


def _to_amount(raw: str) -> float | None:
    try:
        amount = float(raw.replace(",", ""))
    except ValueError:
        return None
    return amount if math.isfinite(amount) and amount > 0 else None


def _parse_date(text: str) -> str | None:
    patterns = (
        (r"\b(\d{4}-\d{1,2}-\d{1,2})\b", ("%Y-%m-%d",)),
        (
            r"\b(\d{1,2}/\d{1,2}/\d{4})\b",
            ("%d/%m/%Y",),
        ),
        (
            r"\b(\d{1,2}-(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)"
            r"[a-z]*-\d{4})\b",
            ("%d-%b-%Y", "%d-%B-%Y"),
        ),
        (
            r"\b(\d{1,2}\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)"
            r"[a-z]*\s+\d{4})\b",
            ("%d %b %Y", "%d %B %Y"),
        ),
    )
    for pattern, formats in patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if not match:
            continue
        for date_format in formats:
            try:
                return datetime.strptime(match.group(1), date_format).strftime("%Y-%m-%d")
            except ValueError:
                continue
        return None
    return None


def _parse_time(text: str) -> str | None:
    match = re.search(
        r"(?<!\d)(\d{1,2}):(\d{2})(?::\d{2})?\s*(AM|PM)?(?![A-Za-z])",
        text,
        re.IGNORECASE,
    )
    if not match:
        return None
    hour, minute = int(match.group(1)), int(match.group(2))
    meridiem = (match.group(3) or "").upper()
    if minute > 59:
        return None
    if meridiem:
        if not 1 <= hour <= 12:
            return None
        hour = hour % 12 + (12 if meridiem == "PM" else 0)
    elif hour > 23:
        return None
    return f"{hour:02d}:{minute:02d}"


def _parse_channel(text: str) -> str | None:
    match = re.search(r"\b(atm|pos|transfer|web)\b", text, re.IGNORECASE)
    return match.group(1).lower() if match else None


def _parse_bank(text: str) -> str:
    for keyword in sorted(KNOWN_BANKS, key=len, reverse=True):
        if re.search(rf"(?<![A-Za-z0-9]){re.escape(keyword)}(?![A-Za-z0-9])", text, re.IGNORECASE):
            return KNOWN_BANKS[keyword]
    return "Unknown"


def _parse_reference(text: str) -> str | None:
    match = re.search(
        r"\b(?:transaction\s*id|trans\s*id|reference|ref|id)\b"
        r"\s*:?\s*([A-Za-z0-9][A-Za-z0-9-]*)",
        text,
        re.IGNORECASE,
    )
    if match:
        return match.group(1)
    match = re.search(r"\b([A-Za-z0-9]{6,})\b\s*[.!]?\s*$", text)
    return match.group(1) if match else None


def parse_with_regex(text: str) -> dict | None:
    """Extract a complete outgoing transaction alert, or return None."""
    if not isinstance(text, str) or not text.strip():
        return None
    if _CREDIT_PATTERN.search(text) or not _OUTGOING_PATTERN.search(text):
        return None

    amount = _parse_amount(text)
    date = _parse_date(text)
    time = _parse_time(text)
    channel = _parse_channel(text)
    reference = _parse_reference(text)
    if amount is None or date is None or time is None or channel is None or reference is None:
        return None

    return {
        "amount": amount,
        "date": date,
        "time": time,
        "channel": channel,
        "bank": _parse_bank(text),
        "reference": reference,
    }
