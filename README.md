# ReverseIt

A web app that helps users recover money from failed transactions by guiding them through the dispute process.

## Alert Parsing Backend

This repository contains the backend for parsing debit alerts from SMS or screenshots.

### Features

- Extracts amount, date, channel, bank, and reference from debit alert text
- Handles various formats of Nigerian bank SMS alerts
- Returns structured JSON data

### Installation

1. Clone the repository
2. Install dependencies:
   ```
   pip install -r requirements.txt
   ```
3. Start the server:
   ```
   python app.py
   ```

### API

#### Parse Alert

**URL:** `/parse-alert`
**Method:** `POST`
**Content-Type:** `application/json`

**Request Body:**
```json
{
  "alertText": "Your account ending in 1234 has been debited NGN2,500.00 on 12/10/2026 at 10:30 via POS at MERCHANT NAME. Ref: ABC123XYZ."
}
```

**Response:**
```json
{
  "amount": 2500.0,
  "date": "12/10/2026",
  "channel": "POS",
  "bank": null,
  "reference": "ABC123XYZ",
  "raw": "Your account ending in 1234 has been debited NGN2,500.00 on 12/10/2026 at 10:30 via POS at MERCHANT NAME. Ref: ABC123XYZ."
}
```

### Parser Logic

The parser uses regular expressions to extract:

- **Amount:** Looks for currency patterns (NGN, ₦) or numbers with commas and decimal points.
- **Date:** Matches patterns like DD/MM/YYYY, DD-MM-YYYY, YYYY-MM-DD, or DD Month YYYY.
- **Channel:** Identifies keywords such as ATM, POS, transfer, web, mobile,ussd.
- **Bank:** Attempts to match known bank names (optional).
- **Reference:** Looks for patterns like "Ref: " or "Reference: " followed by alphanumeric characters.

### Testing

Run the test suite:
```bash
python test_parser.py
```

### Future Work

- Integrate with React frontend
- Add OCR support for screenshot alerts
- Connect to a database to store dispute records
- Implement escalation tracking and deadlines
