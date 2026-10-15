import re
from datetime import datetime

def parse_alert(text: str) -> dict:
    """
    Parse a debit alert SMS to extract amount, date, time, channel, bank, and reference.
    Returns a dict either with the six fields (on success) or with error and missing fields (on failure).
    """
    if not isinstance(text, str):
        return {"error": "could_not_parse", "missing": ["amount", "date", "time", "channel", "bank", "reference"]}

    # Step 1: Try regex extraction
    extracted = _extract_with_regex(text)
    missing, invalid = _validate_extracted(extracted)

    # If any missing or invalid, treat as not fully parsed
    if not missing and not invalid:
        # All fields present and valid, normalize and return
        return _normalize_output(extracted)

    # If we have missing or invalid fields, return error
    # Combine missing and invalid into a single list of fields that are not good
    not_good = list(set(missing + invalid))
    return {"error": "could_not_parse", "missing": not_good}


def _extract_with_regex(text: str) -> dict:
    """Extract fields using regex patterns."""
    result = {}

    # Amount: look for currency patterns
    amount_patterns = [
        r'(?:NGN|₦)\s*([\d,]+\.?\d*)',  # NGN1,234.56 or ₦1,234.56
        r'([\d,]+\.?\d*)\s*(?:NGN|₦)',  # 1,234.56 NGN
        r'([\d,]+\.?\d*)'               # fallback: just numbers with commas/decimal
    ]

    for pattern in amount_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            amount_str = match.group(1).replace(',', '')
            try:
                result['amount'] = float(amount_str)
                break
            except ValueError:
                pass

    # Date: try multiple formats
    date_patterns = [
        r'(\d{2}[/-]\d{2}[/-]\d{4})',         # DD/MM/YYYY or DD-MM-YYYY
        r'(\d{4}[/-]\d{2}[/-]\d{2})',         # YYYY-MM-DD
        r'(\d{2}-(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*-\d{4})',   # DD-Mon-YYYY
        r'(\d{2})\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+\d{4})',  # DD Month YYYY
        r'(\d{2})\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+(\d{4})',  # DD Month YYYY (groups)
    ]

    for pattern in date_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            if match.lastindex == 2:  # DD Month YYYY with two groups
                day, month_str, year = match.group(1), match.group(2), match.group(3)
                # Convert month string to number
                month_map = {
                    'jan': '01', 'feb': '02', 'mar': '03', 'apr': '04', 'may': '05', 'jun': '06',
                    'jul': '07', 'aug': '08', 'sep': '09', 'oct': '10', 'nov': '11', 'dec': '12'
                }
                month = month_map.get(month_str.lower()[:3], '01')
                result['date_raw'] = f"{year}-{month}-{day.zfill(2)}"
            else:
                date_str = match.group(0)
                # Try to normalize the date string to YYYY-MM-DD
                # We'll try to parse it with known formats
                for fmt in ('%d/%m/%Y', '%d-%m-%Y', '%Y/%m/%d', '%Y-%m-%d'):
                    try:
                        parsed = datetime.strptime(date_str, fmt)
                        result['date_raw'] = parsed.strftime('%Y-%m-%d')
                        break
                    except ValueError:
                        continue
                # If we didn't break, we might not have parsed correctly
                if 'date_raw' not in result:
                    # As a fallback, we'll use the string as is and hope validation catches it
                    result['date_raw'] = date_str
            break

    # Time: look for HH:MM or HH:MM:SS
    time_patterns = [
        r'(\d{2}:\d{2})',      # HH:MM
        r'(\d{2}:\d{2}:\d{2})' # HH:MM:SS
    ]

    for pattern in time_patterns:
        match = re.search(pattern, text)
        if match:
            time_str = match.group(1)
            # If we have HH:MM:SS, we'll keep only HH:MM for simplicity
            if len(time_str) == 8:
                result['time_raw'] = time_str[:5]
            else:
                result['time_raw'] = time_str
            break

    # Channel: look for atm, pos, transfer, web (case insensitive)
    channel_patterns = [
        r'\b(atm|pos|transfer|web)\b'
    ]

    for pattern in channel_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            result['channel'] = match.group(1).lower()
            break

    # Bank: look for the bank name after certain prepositions, excluding channel words.
    # Bank name must start with a letter.
    bank_patterns = [
        r'(?:from|at|via|through|with)\s+((?!(?:atm|pos|transfer|web)\b)[A-Za-z][A-Za-z0-9]*)',
    ]

    for pattern in bank_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            result['bank'] = match.group(1).strip()
            break

    # Reference: look for Ref:, Reference:, Transaction ID:, ID, etc.
    ref_patterns = [
        r'\b(?:Ref|Reference)\b\s*:?\s*([A-Za-z0-9]+)',
        r'\b(?:Transaction\s*ID|Trans\s*ID|ID)\b\s*:?\s*([A-Za-z0-9]+)',
        r'\b([A-Za-z0-9]{6,})\b(?=\s*$)'  # alphanumeric at least 6 chars at end (fallback)
    ]

    for pattern in ref_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            result['reference'] = match.group(1)
            break

    return result


def _validate_extracted(extracted: dict) -> tuple[list[str], list[str]]:
    """Validate extracted fields. Return (missing_fields, invalid_fields)."""
    missing = []
    invalid = []

    # Amount
    if 'amount' not in extracted:
        missing.append('amount')
    else:
        # Already float from extraction, but ensure it's a number
        if not isinstance(extracted['amount'], (int, float)):
            try:
                extracted['amount'] = float(extracted['amount'])
            except (ValueError, TypeError):
                invalid.append('amount')

    # Date
    if 'date_raw' not in extracted:
        missing.append('date')
    else:
        date_str = extracted['date_raw']
        parsed_date = _parse_date(date_str)
        if parsed_date is None:
            invalid.append('date')
        else:
            extracted['date'] = parsed_date  # Store normalized date

    # Time
    if 'time_raw' not in extracted:
        missing.append('time')
    else:
        time_str = extracted['time_raw']
        if not _validate_time(time_str):
            invalid.append('time')
        else:
            extracted['time'] = time_str  # Already in HH:MM format (we assume)

    # Channel
    if 'channel' not in extracted:
        missing.append('channel')
    else:
        channel = extracted['channel']
        if channel not in ['atm', 'pos', 'transfer', 'web']:
            invalid.append('channel')

    # Bank
    if 'bank' not in extracted:
        missing.append('bank')
    else:
        if not isinstance(extracted['bank'], str) or not extracted['bank'].strip():
            invalid.append('bank')

    # Reference
    if 'reference' not in extracted:
        missing.append('reference')
    else:
        if not isinstance(extracted['reference'], str) or not extracted['reference'].strip():
            invalid.append('reference')

    return missing, invalid


def _parse_date(date_str: str) -> str | None:
    """Try to parse date string and return YYYY-MM-DD or None."""
    # List of formats to try
    formats = [
        '%d/%m/%Y',
        '%d-%m-%Y',
        '%Y/%m/%d',
        '%Y-%m-%d',
        '%d %b %Y',
        '%d %B %Y',
        '%d-%b-%Y',
        '%d-%B-%Y',
    ]

    for fmt in formats:
        try:
            parsed = datetime.strptime(date_str, fmt)
            return parsed.strftime('%Y-%m-%d')
        except ValueError:
            continue
    return None


def _validate_time(time_str: str) -> bool:
    """Validate time string in HH:MM format."""
    if not re.match(r'^([01]\d|2[0-3]):([0-5]\d)$', time_str):
        return False
    return True


def _normalize_output(extracted: dict) -> dict:
    """Normalize the extracted fields to the output format."""
    # We have already normalized date and time in validation, but we'll do it again for safety.
    # The extracted dict should have: amount, date, time, channel, bank, reference
    # We'll return a new dict with these fields.
    return {
        'amount': extracted['amount'],
        'date': extracted['date'],
        'time': extracted['time'],
        'channel': extracted['channel'],
        'bank': extracted['bank'],
        'reference': extracted['reference']
    }