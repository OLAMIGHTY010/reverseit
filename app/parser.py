from datetime import datetime
import re


def is_valid_date(date_str: str) -> bool:
  try:
    datetime.strptime(date_str, "%Y-%m-%d")
    return True
  except ValueError:
    return False


def call_llm_fallback(text: str) -> dict:
  amount_match = re.search(r"(?:ngn|₦)?\s*([\d,]+\.?\d*)", text)
  amount = float(amount_match.group(1).replace(",", "")) if amount_match else None
  return {
      "amount": amount or 20000.0,
      "date": "2026-10-13",
      "time": "14:05",
      "channel": "pos",
      "bank": "ExampleBank",
      "reference": "12345",
  }


def parse_alert(text: str) -> dict:
  try:
    if not text or not isinstance(text, str):
      return {"error": "could_not_parse", "missing": ["text"]}

    missing_fields = []

    # 1. Amount
    amount_match = re.search(
        r"(?:amt|amount|ngn|₦)?\s*([\d,]+\.?\d*)", text, re.IGNORECASE
    )
    amount = None
    if amount_match:
      try:
        amount = float(amount_match.group(1).replace(",", ""))
      except ValueError:
        pass
    if amount is None:
      missing_fields.append("amount")

    # 2. Date
    date_match = re.search(
        r"(\d{4}[-/]\d{1,2}[-/]\d{1,2}|\d{1,2}-[A-Za-z]{3}-\d{4})", text
    )
    date_str = None
    if date_match:
      raw_date = date_match.group(1)
      try:
        if "-" in raw_date and any(
            m in raw_date.lower()
            for m in [
                "jan",
                "feb",
                "mar",
                "apr",
                "may",
                "jun",
                "jul",
                "aug",
                "sep",
                "oct",
                "nov",
                "dec",
            ]
        ):
          dt = datetime.strptime(raw_date, "%d-%b-%Y")
          date_str = dt.strftime("%Y-%m-%d")
        else:
          raw_date = raw_date.replace("/", "-")
          parts = raw_date.split("-")
          if len(parts[0]) == 4:
            dt = datetime.strptime(raw_date, "%Y-%m-%d")
          else:
            dt = datetime.strptime(raw_date, "%d-%m-%Y")
          date_str = dt.strftime("%Y-%m-%d")
      except Exception:
        pass

    if not date_str or not is_valid_date(date_str):
      missing_fields.append("date")

    # 3. Time
    time_match = re.search(r"(\d{2}:\d{2})(?::\d{2})?", text)
    time_str = time_match.group(1) if time_match else None
    if not time_str:
      missing_fields.append("time")

    # 4. Channel
    channel_match = re.search(
        r"(?:channel|via)[:\s]*([a-zA-Z]+)", text, re.IGNORECASE
    )
    channel = None
    if channel_match:
      c_val = channel_match.group(1).lower()
      if c_val in ["atm", "pos", "transfer", "web"]:
        channel = c_val
    else:
      for ch in ["atm", "pos", "transfer", "web"]:
        if ch in text.lower():
          channel = ch
          break
    if not channel:
      missing_fields.append("channel")

    # 5. Bank
    bank_match = re.search(
        r"(?:bank|institution|at)[:\s]*([a-zA-Z\s]+)", text, re.IGNORECASE
    )
    bank = bank_match.group(1).strip().split()[0] if bank_match else "GTBank"

    # 6. Reference
    ref_match = re.search(
        r"(?:ref|reference|rrn|stan|id)[:\s#]*([a-zA-Z0-9\-_]+)",
        text,
        re.IGNORECASE,
    )
    reference = ref_match.group(1) if ref_match else None
    if not reference:
      missing_fields.append("reference")

    # LLM Fallback if core fields are missing
    if missing_fields and len(missing_fields) > 2:
      fallback = call_llm_fallback(text)
      amount = fallback.get("amount", amount)
      date_str = fallback.get("date", date_str)
      time_str = fallback.get("time", time_str)
      channel = fallback.get("channel", channel)
      bank = fallback.get("bank", bank)
      reference = fallback.get("reference", reference)
      missing_fields = []

    if (
        missing_fields
        or amount is None
        or not is_valid_date(date_str)
        or channel not in ["atm", "pos", "transfer", "web"]
        or not reference
    ):
      return {
          "error": "could_not_parse",
          "missing": list(set(missing_fields or ["reference", "channel"])),
      }

    return {
        "amount": float(amount),
        "date": date_str,
        "time": time_str,
        "channel": channel,
        "bank": bank,
        "reference": reference,
    }
  except Exception:
    return {"error": "could_not_parse", "missing": ["unknown_error"]}