import os
import re
import json
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
    
    # Step 2: If regex fails, try LLM fallback
    llm_result = _try_llm_fallback(text)
    if llm_result is not None:
        missing_llm, invalid_llm = _validate_extracted(llm_result)
        if not missing_llm and not invalid_llm:
            return _normalize_output(llm_result)
        # If LLM result invalid, we could optionally try to merge with regex? 
        # For simplicity, we'll fall through to failure.
    
    # Step 3: If both attempts fail, return could_not_parse with missing fields
    # We'll combine missing and invalid from the best attempt (regex if LLM not tried or failed)
    # For failure, we consider a field missing if it's either missing or invalid.
    if llm_result is not None:
        missing_final, invalid_final = _validate_extracted(llm_result)
    else:
        missing_final, invalid_final = _validate_extracted(extracted)
    
    # Combine missing and invalid into a single list of fields that are not good
    not_good = list(set(missing_final + invalid_final))
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
        r'(\d{2}[/-]\d{2}[/-]\d{4})',      # DD/MM/YYYY or DD-MM-YYYY
        r'(\d{4}[/-]\d{2}[/-]\d{2})',      # YYYY-MM-DD
        r'(\d{2}\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+\d{4})',  # DD Month YYYY
        r'(\d{2}-(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*-\d{4})'   # DD-Mon-YYYY
    ]
    for pattern in date_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            result['date_raw'] = match.group(1)
            break
    
    # Time: look for HH:MM or HH:MM:SS
    time_patterns = [
        r'(\d{2}:\d{2})',          # HH:MM
        r'(\d{2}:\d{2}:\d{2})'     # HH:MM:SS
    ]
    for pattern in time_patterns:
        match = re.search(pattern, text)
        if match:
            result['time_raw'] = match.group(1)
            break
    
    # Channel: look for keywords (only atm, pos, transfer, web allowed)
    channel_patterns = [
        r'\b(ATM|atm)\b',
        r'\b(POS|pos|point\s*of\s*sale)\b',
        r'\b(transfer|wire)\b',
        r'\b(web|online|internet)\b'
    ]
    for pattern in channel_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            channel = match.group(0).lower()
            if channel == 'atm':
                result['channel'] = 'atm'
            elif channel == 'pos' or 'point of sale' in channel:
                result['channel'] = 'pos'
            elif channel in ['transfer', 'wire']:
                result['channel'] = 'transfer'
            elif channel in ['web', 'online', 'internet']:
                result['channel'] = 'web'
            else:
                result['channel'] = channel  # fallback
            break
    
    # Bank: look for known bank names (optional)
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
        r'\b(HERITAGE\s*BANK|HeritageBank)\b'
    ]
    for pattern in bank_patterns:
        match = re.search(pattern, text, re.IGNORECASE)
        if match:
            result['bank'] = match.group(0)
            break
    
    # Reference: look for Ref:, Reference:, Transaction ID:, etc.
    # Use word boundary to avoid matching partial words
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
        '%d-%B-%Y'
    ]
    for fmt in formats:
        try:
            dt = datetime.strptime(date_str, fmt)
            return dt.strftime('%Y-%m-%d')
        except ValueError:
            continue
    return None

def _validate_time(time_str: str) -> bool:
    """Validate time string is in HH:MM format (24-hour)."""
    if not re.match(r'^\d{2}:\d{2}$', time_str):
        return False
    try:
        hour, minute = map(int, time_str.split(':'))
        return 0 <= hour <= 23 and 0 <= minute <= 59
    except ValueError:
        return False

def _normalize_output(extracted: dict) -> dict:
    """Return normalized output with the six fields."""
    return {
        'amount': extracted['amount'],
        'date': extracted.get('date'),  # Should be present if validation passed
        'time': extracted.get('time'),
        'channel': extracted['channel'],
        'bank': extracted['bank'],
        'reference': extracted['reference']
    }

def _try_llm_fallback(text: str) -> dict | None:
    """
    Attempt to use an LLM to extract the fields.
    Returns a dict with the six fields if successful, else None.
    """
    # Get API key from environment
    api_key = os.environ.get('ANTHROPIC_API_KEY')
    if not api_key:
        # No API key available, skip LLM fallback
        return None
    
    try:
        import anthropic
        client = anthropic.Anthropic(api_key=api_key)
        
        prompt = f"""Extract the following fields from the debit alert text below.
Return ONLY a JSON object with exactly these keys: amount, date, time, channel, bank, reference.
- amount: a number (float or integer)
- date: a string in YYYY-MM-DD format
- time: a string in HH:MM format (24-hour)
- channel: one of "atm", "pos", "transfer", "web"
- bank: a string (name of the bank)
- reference: a string (transaction reference)

If a field cannot be determined, set its value to null.

Debit alert:
{text}

JSON:"""
        
        message = client.messages.create(
            model="claude-3-5-sonnet-20241022",
            max_tokens=200,
            temperature=0,
            system="You are a precise information extractor. Return only valid JSON.",
            messages=[
                {"role": "user", "content": prompt}
            ]
        )
        
        # Extract JSON from the response
        response_text = message.content[0].text
        # Find JSON object in the response
        json_match = re.search(r'\{.*\}', response_text, re.DOTALL)
        if json_match:
            json_str = json_match.group(0)
            data = json.loads(json_str)
            # Ensure we have all six keys (even if null)
            result = {}
            for key in ['amount', 'date', 'time', 'channel', 'bank', 'reference']:
                result[key] = data.get(key)
            return result
    except Exception as e:
        # Log the error? For now, just return None
        # In production, you might want to log this
        pass
    
    return None

# For testing purposes
if __name__ == "__main__":
    # Quick self-test
    test_alerts = [
        ("GTBank", "GTBank: Your account ending in 4521 has been debited NGN15,000.00 on 13/10/2026 at 09:15 via ATM. Ref: GTB20261013001."),
        ("First Bank", "FirstBank Debit Alert: NGN8,500.50 was withdrawn from your account ******6789 via POS on 2026-10-12 16:30 at SUPER MART. Reference: FBK123456789."),
        ("Access Bank", "AccessBank Notification: Transfer of NGN3,200.00 to account 0123456789 completed on 12-Oct-2026 at 10:30. Reference: ACC20261012TRF."),
        ("Zenith Bank", "Zenith Bank Alert: Your card ending in 1234 was used for a web payment of NGN12,400.00 on 10/10/2026 at 22:45. Transaction ID: ZBWEB20261010001."),
        ("UBA", "UBA Mobile Money: NGN500.00 sent to ******1122 via Transfer on 13-10-2026 14:05. Ref: UBAUSSD131026.")
    ]
    for bank, alert in test_alerts:
        print("Alert:", alert)
        print("Result:", parse_alert(alert))
        print("---")
