import re

def parse_alert(alert_text):
    """
    Parse a debit alert SMS to extract amount, date, channel, bank, and reference.
    Returns a dictionary with the extracted fields.
    """
    if not alert_text or not isinstance(alert_text, str):
        raise ValueError("Invalid alert text")

    result = {
        'amount': None,
        'date': None,
        'channel': None,
        'bank': None,
        'reference': None,
        'raw': alert_text
    }

    # Amount: look for currency patterns
    amount_patterns = [
        # NGN1,234.56 or ₦1,234.56
        r'(?:NGN|₦)\s*([\d,]+\.?\d*)',
        # 1,234.56 NGN
        r'([\d,]+\.?\d*)\s*(?:NGN|₦)',
        # just numbers with commas and decimal (fallback)
        r'([\d,]+\.?\d*)'
    ]

    for pattern in amount_patterns:
        match = re.search(pattern, alert_text, re.IGNORECASE)
        if match:
            amount_str = match.group(1)
            # Remove commas and convert to float
            amount_str = amount_str.replace(',', '')
            try:
                amount = float(amount_str)
                result['amount'] = amount
                break
            except ValueError:
                continue

    # Date: look for date patterns
    date_patterns = [
        # DD/MM/YYYY or DD-MM-YYYY
        r'(\d{2}[/-]\d{2}[/-]\d{4})',
        # YYYY-MM-DD
        r'(\d{4}[/-]\d{2}[/-]\d{2})',
        # DD Month YYYY
        r'(\d{2}\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+\d{4})'
    ]

    for pattern in date_patterns:
        match = re.search(pattern, alert_text, re.IGNORECASE)
        if match:
            result['date'] = match.group(1)
            break

    # Channel: look for keywords
    channel_patterns = [
        r'\b(ATM|atm)\b',
        r'\b(POS|pos|point\s*of\s*sale)\b',
        r'\b(transfer|wire)\b',
        r'\b(web|online|internet)\b',
        r'\b(mobile|ussd)\b'
    ]

    for pattern in channel_patterns:
        match = re.search(pattern, alert_text, re.IGNORECASE)
        if match:
            channel = match.group(0).upper()
            if channel == 'ATM':
                result['channel'] = 'ATM'
            elif channel == 'POS' or 'POINT OF SALE' in channel:
                result['channel'] = 'POS'
            elif channel in ['TRANSFER', 'WIRE']:
                result['channel'] = 'transfer'
            elif channel in ['WEB', 'ONLINE', 'INTERNET']:
                result['channel'] = 'web'
            elif channel in ['MOBILE', 'USSD']:
                result['channel'] = 'ussd'
            else:
                result['channel'] = match.group(0)  # fallback
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
        match = re.search(pattern, alert_text, re.IGNORECASE)
        if match:
            result['bank'] = match.group(0)  # keep original case
            break

    # Reference: look for Ref: or Reference:
    ref_patterns = [
        r'(?:Ref|Reference)\s*:?\s*([A-Za-z0-9]+)',
        # sometimes reference is at the end after a space
        r'\b([A-Za-z0-9]{6,})\b(?=\s*$)'
    ]

    for pattern in ref_patterns:
        match = re.search(pattern, alert_text, re.IGNORECASE)
        if match:
            result['reference'] = match.group(1)
            break

    return result

if __name__ == "__main__":
    # Simple test
    test_alerts = [
        "Your account ending in 1234 has been debited NGN2,500.00 on 12/10/2026 at 10:30 via POS at MERCHANT NAME. Ref: ABC123XYZ.",
        "Debit alert: NGN1,500.00 withdrawn from your account ending 5678 via ATM on 12-10-2026. Ref: XYZ789.",
        "Transfer of NGN3,000.00 to account 0123456789 completed on 12/10/2026. Ref: TRF123."
    ]
    for alert in test_alerts:
        print(parse_alert(alert))
