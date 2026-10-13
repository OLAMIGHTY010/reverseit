"""Alert parsing module extracting structured transaction fields.

Signature: parse_alert(text: str) -> dict
"""

import json
import os
import re
from datetime import datetime


def parse_alert(text: str) -> dict:
    """
    Parse a debit alert message and extract structured transaction details.

    Returns:
        Success: {
            "amount": float,
            "date": "YYYY-MM-DD",
            "time": "HH:MM",
            "channel": "atm" | "pos" | "transfer" | "web",
            "bank": str,
            "reference": str
        }
        Failure: {
            "error": "could_not_parse",
            "missing": list[str]
        }
    """
    if not isinstance(text, str) or not text.strip():
        return {
            "error": "could_not_parse",
            "missing": ["amount", "date", "time", "channel", "bank", "reference"],
        }

    # Step 1: Regex extraction
    extracted = _extract_with_regex(text)
    missing, invalid = _validate_extracted(extracted)

    # If all fields present and valid, return normalized output
    if not missing and not invalid:
        return _normalize_output(extracted)

    # Step 2: LLM Fallback (if API key is present)
    llm_extracted = _try_llm_fallback(text)
    if llm_extracted:
        missing_llm, invalid_llm = _validate_extracted(llm_extracted)
        if not missing_llm and not invalid_llm:
            return _normalize_output(llm_extracted)

    # Determine what's missing overall
    combined_missing = sorted(list(set(missing + invalid)))
    if not combined_missing:
        combined_missing = ["amount", "reference"]

    return {
        "error": "could_not_parse",
        "missing": combined_missing,
    }


def _extract_with_regex(text: str) -> dict:
    result = {}

    # Amount: match currency symbols (NGN, N, ₦) and numbers with optional commas/decimals
    amount_match = re.search(r'(?:NGN|N|₦)\s*([0-9]{1,3}(?:,[0-9]{3})*(?:\.[0-9]{2})?|[0-9]+(?:\.[0-9]{2})?)', text, re.IGNORECASE)
    if amount_match:
        amount_str = amount_match.group(1).replace(',', '')
        try:
            result['amount'] = float(amount_str)
        except ValueError:
            pass

    # Date formats
    date_patterns = [
        r'\b(\d{4}-\d{2}-\d{2})\b',
        r'\b(\d{2}/\d{2}/\d{4})\b',
        r'\b(\d{2}-\d{2}-\d{4})\b',
        r'\b(\d{1,2}(?:st|nd|rd|th)?[\s\-]+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*[\s\-]+\d{4})\b',
        r'\b(\d{1,2}[\s\-]+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*[\s\-]+\d{4})\b',
        r'\b(\d{2}-[A-Za-z]{3}-\d{4})\b'
    ]
    for pattern in date_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            result['date_raw'] = match.group(1)
            break

    # Time format: HH:MM (24-hour)
    time_match = re.search(r'\b([01]?\d|2[0-3]):([0-5]\d)\b', text)
    if time_match:
        hour, minute = time_match.group(1), time_match.group(2)
        result['time_raw'] = f"{int(hour):02d}:{minute}"

    # Channel
    channel_patterns = [
        r'\b(atm)\b',
        r'\b(pos|point\s*of\s*sale)\b',
        r'\b(transfer|wire|ussd)\b',
        r'\b(web|online|internet)\b'
    ]
    for pattern in channel_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            channel_str = match.group(0).lower()
            if channel_str == 'atm':
                result['channel'] = 'atm'
            elif 'pos' in channel_str or 'point' in channel_str:
                result['channel'] = 'pos'
            elif channel_str in ['transfer', 'wire', 'ussd']:
                result['channel'] = 'transfer'
            elif channel_str in ['web', 'online', 'internet']:
                result['channel'] = 'web'
            break

    # Bank names
    bank_patterns = [
        r'\b(GTBANK|Guaranty\s*Trust\s*Bank)\b',
        r'\b(FIRST\s*BANK|FirstBank)\b',
        r'\b(ACCESS\s*BANK|AccessBank)\b',
        r'\b(ZENITH\s*BANK|ZenithBank)\b',
        r'\b(UBA|United\s*Bank\s*for\s*Africa)\b',
        r'\b(FIDELITY\s*BANK|FidelityBank)\b',
        r'\b(STERLING\s*BANK|SterlingBank)\b',
        r'\b(ECOBANK|Ecobank)\b',
        r'\b(KEYSTONE\s*BANK|KeystoneBank)\b',
        r'\b(KUDA|Kuda\s*Bank)\b',
        r'\b(OPAY|Opay)\b',
        r'\b(PALMPAY|Palmpay)\b',
        r'\b(STANBIC|Stanbic\s*IBTC)\b',
        r'\b(HERITAGE\s*BANK|HeritageBank)\b'
    ]
    for pattern in bank_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            result['bank'] = match.group(0).strip()
            break

    # Reference
    ref_patterns = [
        r'\b(?:Ref|Reference|Ref\s*No)\b\s*:?\s*([A-Za-z0-9_\-]+)',
        r'\b(?:Transaction\s*ID|Trans\s*ID|ID|Txn\s*ID)\b\s*:?\s*([A-Za-z0-9_\-]+)',
        r'\b([A-Za-z0-9]{8,})\b(?=\s*\.?)'
    ]
    for pattern in ref_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            result['reference'] = match.group(1).rstrip('.')
            break

    return result


def _validate_extracted(extracted: dict) -> tuple[list[str], list[str]]:
    missing = []
    invalid = []

    # Amount
    if 'amount' not in extracted or extracted['amount'] is None:
        missing.append('amount')
    else:
        try:
            val = float(extracted['amount'])
            if val <= 0:
                invalid.append('amount')
        except (ValueError, TypeError):
            invalid.append('amount')

    # Date
    date_val = extracted.get('date') or extracted.get('date_raw')
    if not date_val:
        missing.append('date')
    else:
        parsed_date = _parse_date(str(date_val))
        if parsed_date is None:
            invalid.append('date')
        else:
            extracted['date'] = parsed_date

    # Time
    time_val = extracted.get('time') or extracted.get('time_raw')
    if not time_val:
        missing.append('time')
    else:
        if not _validate_time(str(time_val)):
            invalid.append('time')
        else:
            extracted['time'] = str(time_val)

    # Channel
    if 'channel' not in extracted or not extracted['channel']:
        missing.append('channel')
    else:
        channel = str(extracted['channel']).lower()
        if channel not in ['atm', 'pos', 'transfer', 'web']:
            invalid.append('channel')
        else:
            extracted['channel'] = channel

    # Bank
    if 'bank' not in extracted or not extracted['bank']:
        missing.append('bank')
    elif not str(extracted['bank']).strip():
        invalid.append('bank')

    # Reference
    if 'reference' not in extracted or not extracted['reference']:
        missing.append('reference')
    elif not str(extracted['reference']).strip():
        invalid.append('reference')

    return missing, invalid


def _parse_date(date_str: str) -> str | None:
    date_str = re.sub(r'(\d+)(st|nd|rd|th)', r'\1', date_str.strip())
    formats = [
        '%Y-%m-%d',
        '%d/%m/%Y',
        '%d-%m-%Y',
        '%Y/%m/%d',
        '%d %b %Y',
        '%d %B %Y',
        '%d-%b-%Y',
        '%d-%B-%Y',
        '%b %d, %Y',
        '%B %d, %Y'
    ]
    for fmt in formats:
        try:
            dt = datetime.strptime(date_str, fmt)
            return dt.strftime('%Y-%m-%d')
        except ValueError:
            continue
    return None


def _validate_time(time_str: str) -> bool:
    time_str = time_str.strip()
    match = re.match(r'^(\d{1,2}):(\d{2})$', time_str)
    if not match:
        return False
    h, m = int(match.group(1)), int(match.group(2))
    return 0 <= h <= 23 and 0 <= m <= 59


def _normalize_output(extracted: dict) -> dict:
    return {
        'amount': float(extracted['amount']),
        'date': extracted['date'],
        'time': extracted['time'],
        'channel': extracted['channel'],
        'bank': extracted['bank'],
        'reference': extracted['reference'],
    }


def _try_llm_fallback(text: str) -> dict | None:
    api_key = os.getenv('ANTHROPIC_API_KEY')
    if not api_key:
        return None

    try:
        import anthropic
        client = anthropic.Anthropic(api_key=api_key)
        prompt = (
            "Extract the following debit alert fields. Return STRICT JSON with keys:\n"
            "amount (number), date (YYYY-MM-DD), time (HH:MM), channel (atm|pos|transfer|web), bank (string), reference (string).\n\n"
            f"Alert:\n{text}"
        )
        msg = client.messages.create(
            model="claude-3-5-sonnet-20241022",
            max_tokens=250,
            temperature=0,
            system="Return ONLY valid JSON. No conversational text.",
            messages=[{"role": "user", "content": prompt}],
        )
        content = msg.content[0].text
        match = re.search(r'\{.*\}', content, re.DOTALL)
        if match:
            return json.loads(match.group(0))
    except Exception:
        pass
    return None
