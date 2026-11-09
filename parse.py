"""
ReverseIt - Workstream 1: Alert Parsing

Public contract:
    parse_alert(text: str) -> dict        # pasted SMS / receipt text
    parse_screenshot(path: str) -> dict   # screenshot -> text -> parse_alert

Success: {"amount", "date", "time", "channel", "bank", "reference"}
Failure: {"error": "could_not_parse", "missing": [<field names>]}
"""
import os
import re
import json
import math
from datetime import datetime

from dotenv import load_dotenv
from groq import Groq

load_dotenv()

# ---------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------
CHANNELS = ("atm", "pos", "transfer", "web")
REQUIRED_FIELDS = ["amount", "date", "time", "channel", "bank", "reference"]
DEFAULT_BANK = "Unknown"
EMPTY_MARKERS = {"", "unknown", "n/a", "na", "none", "null"}

LLM_MODEL = os.getenv("GROQ_MODEL", "qwen/qwen3.8-27b")
# Qwen models may emit reasoning tokens that eat the budget and truncate the
# JSON. Keep this comfortably above 150. Optionally set GROQ_REASONING_EFFORT
# (e.g. "none") in .env if the model supports it.
LLM_MAX_TOKENS = int(os.getenv("GROQ_MAX_TOKENS", "400"))

# ---------------------------------------------------------------------
# Normalizers (shared by regex path, LLM path and validator)
# ---------------------------------------------------------------------
_DATE_FORMATS = (
    "%Y-%m-%d", "%Y/%m/%d", "%d-%b-%Y", "%d-%B-%Y", "%d/%m/%Y", "%d-%m-%Y",
)


def normalize_date(raw) -> str | None:
    """Return YYYY-MM-DD or None if the value is not a recognizable date."""
    raw = str(raw).strip()
    for fmt in _DATE_FORMATS:
        try:
            return datetime.strptime(raw, fmt).strftime("%Y-%m-%d")
        except ValueError:
            continue
    return None


_TIME_RE = re.compile(r"^([01]?\d|2[0-3]):([0-5]\d)(?::[0-5]\d)?$")


def normalize_time(raw) -> str | None:
    """Return HH:MM (24h) or None. Accepts H:MM and HH:MM:SS."""
    m = _TIME_RE.match(str(raw).strip())
    if not m:
        return None
    return f"{int(m.group(1)):02d}:{m.group(2)}"


def normalize_amount(raw) -> float | None:
    """Return a positive finite float or None."""
    if isinstance(raw, bool):
        return None
    try:
        if isinstance(raw, str):
            raw = re.sub(r"[,\s₦]|NGN|N(?=\d)", "", raw, flags=re.IGNORECASE)
        value = float(raw)
    except (ValueError, TypeError):
        return None
    if not math.isfinite(value) or value <= 0:
        return None
    return value


# ---------------------------------------------------------------------
# Tasks 1 & 2: Regex parser lives in regex_parser.py (teammate's module)
# ---------------------------------------------------------------------
try:
    from regex_parser import parse_with_regex
except ModuleNotFoundError:  # teammate's file not merged yet
    print("[Warning] regex_parser.py not found; using LLM only.")

    def parse_with_regex(text: str) -> dict | None:
        return None


# ---------------------------------------------------------------------
# Screenshot input lives in image_input.py (image -> text)
# ---------------------------------------------------------------------
try:
    from image_input import extract_text_from_image
except ModuleNotFoundError:  # image_input.py not in the folder
    print("[Warning] image_input.py not found; screenshot input disabled.")

    def extract_text_from_image(image_path: str) -> str | None:
        return None


# ---------------------------------------------------------------------
# Task 3: LLM fallback
# ---------------------------------------------------------------------
SYSTEM_PROMPT = """You are an expert Nigerian bank transaction parser.
Extract the transaction details from the alert text and return ONLY a valid JSON object.
Allowed channels: "atm", "pos", "transfer", "web".

JSON schema:
{
  "amount": number,
  "date": "YYYY-MM-DD",
  "time": "HH:MM" (24-hour),
  "channel": "pos" | "atm" | "transfer" | "web",
  "bank": "Bank Name",
  "reference": "reference or session number"
}

Rules:
- Use ONLY information present in the text. If a field is not present, set it to null.
- Never guess, infer, or invent amounts, dates, times, or reference numbers.
- If the text is not a bank debit alert or transaction description, set every field to null.
- Do not include markdown tags, commentary, or extra text."""


def _extract_json(content: str) -> dict | None:
    """Strip <think> blocks / code fences and pull out the first JSON object."""
    content = re.sub(r"<think>.*?</think>", "", content, flags=re.DOTALL)
    content = re.sub(r"```(?:json)?", "", content).strip()
    try:
        data = json.loads(content)
    except json.JSONDecodeError:
        m = re.search(r"\{.*\}", content, flags=re.DOTALL)
        if not m:
            return None
        try:
            data = json.loads(m.group(0))
        except json.JSONDecodeError:
            return None
    return data if isinstance(data, dict) else None


def parse_with_llm(text: str) -> dict | None:
    api_key = os.getenv("GROQ_API_KEY")
    if not api_key:
        print("[Warning] GROQ_API_KEY not found in environment.")
        return None

    kwargs = {}
    effort = os.getenv("GROQ_REASONING_EFFORT")
    if effort:
        kwargs["reasoning_effort"] = effort

    try:
        client = Groq(api_key=api_key, timeout=15.0, max_retries=1)
        completion = client.chat.completions.create(
            messages=[
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user", "content": f"Alert text:\n{text}"},
            ],
            model=LLM_MODEL,
            temperature=0.0,
            max_tokens=LLM_MAX_TOKENS,
            response_format={"type": "json_object"},
            **kwargs,
        )
        return _extract_json(completion.choices[0].message.content or "")
    except Exception as e:
        print(f"[LLM Error]: {e}")
        return None


# ---------------------------------------------------------------------
# Task 4: Strict validation
# ---------------------------------------------------------------------
def validate_extracted_data(data) -> tuple[bool, list[str]]:
    """
    Validate and normalize `data` IN PLACE.
    Returns (is_valid, missing) where `missing` contains only field names
    from REQUIRED_FIELDS (never free-text messages).
    A missing/empty bank is not an error; it defaults to "Unknown".
    """
    if not isinstance(data, dict):
        return False, [f for f in REQUIRED_FIELDS if f != "bank"]

    missing = []

    amount = normalize_amount(data.get("amount"))
    if amount is None:
        missing.append("amount")
    else:
        data["amount"] = amount

    date = normalize_date(data.get("date")) if data.get("date") else None
    if date is None:
        missing.append("date")
    else:
        data["date"] = date

    time = normalize_time(data.get("time")) if data.get("time") else None
    if time is None:
        missing.append("time")
    else:
        data["time"] = time

    channel = str(data.get("channel") or "").strip().lower()
    if channel not in CHANNELS:
        missing.append("channel")
    else:
        data["channel"] = channel

    reference = str(data.get("reference") or "").strip()
    if reference.lower() in EMPTY_MARKERS:
        missing.append("reference")
    else:
        data["reference"] = reference

    bank = str(data.get("bank") or "").strip()
    data["bank"] = DEFAULT_BANK if bank.lower() in EMPTY_MARKERS else bank

    return (len(missing) == 0), missing



MAX_INPUT_CHARS = 2000  # real alerts are short; cap cost and abuse


def ungrounded_fields(data: dict, text: str) -> list[str]:
    """
    Anti-hallucination check for LLM output: amount and reference must
    literally appear in the user's text. Returns the field names that don't.
    """
    bad = []
    flat = re.sub(r"\s+", "", text)
    ref = re.sub(r"\s+", "", str(data.get("reference", "")))
    if not ref or ref not in flat:
        bad.append("reference")

    plain = text.replace(",", "")
    amt = data.get("amount")
    cands = {f"{amt:.2f}", f"{amt:g}"}
    if float(amt).is_integer():
        cands.add(f"{amt:.0f}")
    if not any(re.search(rf"(?<![\d.]){re.escape(c)}(?![\d])", plain) for c in cands):
        bad.append("amount")
    return bad

# ---------------------------------------------------------------------
# Core pipeline (never raises)
# ---------------------------------------------------------------------
def _clean(result: dict) -> dict:
    return {
        "amount": result["amount"],
        "date": result["date"],
        "time": result["time"],
        "channel": result["channel"],
        "bank": result["bank"],
        "reference": result["reference"],
    }


def parse_alert(text: str) -> dict:
    """regex -> (validate) -> LLM fallback -> validate + grounding -> result."""
    try:
        if not text or not isinstance(text, str) or not text.strip():
            return {"error": "could_not_parse", "missing": ["text"]}

        text = text.strip()[:MAX_INPUT_CHARS]

        # Step 1: teammate's regex. Its output is NOT trusted: it is validated,
        # and if it crashes or returns invalid data we fall back to the LLM.
        try:
            regex_result = parse_with_regex(text)
        except Exception as e:
            print(f"[Regex Exception]: {e}")
            regex_result = None

        if isinstance(regex_result, dict) and "error" in regex_result:
            regex_result = None  # a regex module may report failure as an error dict

        if regex_result is not None:
            ok, _ = validate_extracted_data(regex_result)
            if ok and regex_result["bank"] != DEFAULT_BANK:
                return _clean(regex_result)

        # Step 2: LLM fallback, validated + checked against the source text.
        llm_result = parse_with_llm(text)
        ok, missing = validate_extracted_data(llm_result)
        if ok:
            missing = ungrounded_fields(llm_result, text)
            ok = not missing
        if ok:
            return _clean(llm_result)
        return {"error": "could_not_parse", "missing": missing}
    except Exception as e:  # last-resort guard: the contract says never crash
        print(f"[parse_alert Exception]: {e}")
        return {"error": "could_not_parse", "missing": list(REQUIRED_FIELDS)}


def parse_screenshot(image_path: str) -> dict:
    """Screenshot -> text (image_input.py) -> parse_alert. Never raises."""
    try:
        text = extract_text_from_image(image_path)
        if not text:
            return {"error": "could_not_parse", "missing": ["image"]}
        return parse_alert(text)
    except Exception as e:
        print(f"[parse_screenshot Exception]: {e}")
        return {"error": "could_not_parse", "missing": ["image"]}


# ---------------------------------------------------------------------
# Quick manual runner (real test suite lives in tests/)
# ---------------------------------------------------------------------
if __name__ == "__main__":
    import sys

    if len(sys.argv) == 2:  # python parse.py <screenshot>  -> parse that image
        print(json.dumps(parse_screenshot(sys.argv[1]), indent=2))
        sys.exit()

    samples = [
        "Acct: 012****789 Amt: NGN 25,000.00 Dr Desc: POS WDL / 0001293849182 / Opay Terminal Ikeja Date: 2026-10-14 14:32",
        "Debit Alert: Your Access Bank account was debited with 15000 NGN for ATM cash withdrawal at Victoria Island on 2026-10-13 18:20. Ref: 9948271038.",
        "Hello, I sent money to my sister yesterday morning and she has not received it yet.",
        "",
    ]
    for i, s in enumerate(samples, 1):
        print(f"--- Alert #{i} ---")
        print(json.dumps(parse_alert(s), indent=2))