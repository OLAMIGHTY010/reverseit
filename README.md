# Alert Parsing Implementation Plan

## Goal
Turn a pasted debit alert into structured fields.

## Core Function
```python
def parse_alert(text: str) -> dict:
    ...
```

### Success Response
```json
{
  "amount": 20000,
  "date": "2026-10-13",
  "time": "14:05",
  "channel": "pos",
  "bank": "ExampleBank",
  "reference": "12345"
}
```

### Failure Response
```json
{
  "error": "could_not_parse",
  "missing": ["amount", "reference"]
}
```

**Note:** `channel` must be one of: `atm`, `pos`, `transfer`, `web`.

## Tasks

### 1. Write 5 invented alert formats
- Create 5 sample debit alerts from different banks (no real customer data)
- For each, list the fields it contains (amount, date, time, channel, bank, reference)
- Share with Workstream 4

### 2. Step 1: Write regex rules (Working Version)
- Implement regex-based extraction that parses the sample alerts
- This is the initial working version

### 3. Step 2: Add LLM Fallback
- For alerts the regex misses, ask a language model for strict JSON output
- Validate the model's response in code:
  - `amount` must be a number
  - `date` must be a real date
  - `channel` must be one of `atm`, `pos`, `transfer`, `web`
- If validation fails, return the `could_not_parse` result

### 4. Requirements
- Never crash on bad input
- Always return a result in one of the two shapes above (success or failure)

### 5. Testing
- Test every sample alert and record pass/fail
- **Success criterion**: `parse_alert` correctly handles at least 80% of the sample alerts
- For the remaining alerts, return a clean `could_not_parse` result

## Due Dates
- **Regex version**: Wednesday noon
- **LLM fallback**: Wednesday night
- **Stretch goal** (if finished early): Screenshot upload support

---
*This document outlines the specification for the alert parsing component of the ReverseIt project.*
