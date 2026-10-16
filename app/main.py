import os
from app.parser import parse_alert
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

app = FastAPI(title="ReverseIt - Sovereign Trust Engine")

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

SAMPLE_ALERTS = [
    {
        "bank": "ExampleBank",
        "format_name": "Standard POS SMS Alert",
        "text": (
            "Debit Alert: Amt NGN 20,000.00 Dr, Channel POS, Bank ExampleBank,"
            " Ref 12345, Date 2026-10-13 14:05:00"
        ),
        "expected": {
            "amount": 20000.0,
            "date": "2026-10-13",
            "time": "14:05",
            "channel": "pos",
            "bank": "ExampleBank",
            "reference": "12345",
        },
    },
    {
        "bank": "ZenithBank",
        "format_name": "Email Web Transfer Alert",
        "text": (
            "NOTIFICATION OF OUTBOUND TRANSFER\nAccount: **9012\nAmount: ₦45,500"
            " Dr\nChannel: Web\nInstitution: ZenithBank\nTransaction Ref:"
            " TRF987654\nTimestamp: 14-Oct-2026 at 09:12"
        ),
        "expected": {
            "amount": 45500.0,
            "date": "2026-10-14",
            "time": "09:12",
            "channel": "web",
            "bank": "ZenithBank",
            "reference": "TRF987654",
        },
    },
    {
        "bank": "FirstBank",
        "format_name": "ATM Dispense Error Alert",
        "text": (
            "ATM CASH WITHDRAWAL DEBIT\nAmt: NGN 12,500\nChannel: ATM\nBank:"
            " FirstBank\nRef: RRN778899\nDate: 2026/10/12 Time: 18:30"
        ),
        "expected": {
            "amount": 12500.0,
            "date": "2026-10-12",
            "time": "18:30",
            "channel": "atm",
            "bank": "FirstBank",
            "reference": "RRN778899",
        },
    },
    {
        "bank": "GTBank",
        "format_name": "Mobile App P2P Transfer Alert",
        "text": (
            "Transfer Alert\nAmount: 5,000 NGN\nChannel: Transfer\nBank:"
            " GTBank\nReference ID: GTB-TRF-00123\nWhen: 2026-10-11 08:00:15"
        ),
        "expected": {
            "amount": 5000.0,
            "date": "2026-10-11",
            "time": "08:00",
            "channel": "transfer",
            "bank": "GTBank",
            "reference": "GTB-TRF-00123",
        },
    },
    {
        "bank": "UBA",
        "format_name": "Malformed Alert (Edge Case)",
        "text": "Debit of NGN 10,000 on your account.",
        "expected": {
            "error": "could_not_parse",
            "missing": ["date", "time", "channel", "reference"],
        },
    },
]


class ParseRequest(BaseModel):
  alert_text: str


@app.get("/health")
def health_check():
  return {"status": "ok", "service": "ReverseIt Parser Engine"}


@app.post("/parse-alert")
def api_parse_alert(payload: ParseRequest):
  return parse_alert(payload.alert_text)


@app.get("/test-parser")
def run_parser_tests():
  results = []
  passed_count = 0
  total_tests = len(SAMPLE_ALERTS)

  for idx, sample in enumerate(SAMPLE_ALERTS, 1):
    output = parse_alert(sample["text"])
    expected = sample["expected"]

    is_success_expected = "error" not in expected
    is_success_output = "error" not in output

    passed = False
    if is_success_expected and is_success_output:
      passed = (
          output.get("amount") == expected.get("amount")
          and output.get("channel") == expected.get("channel")
          and output.get("reference") == expected.get("reference")
      )
    elif not is_success_expected and not is_success_output:
      passed = output.get("error") == expected.get("error")

    if passed:
      passed_count += 1

    results.append({
        "test_id": idx,
        "format_name": sample["format_name"],
        "input_text": sample["text"],
        "expected": expected,
        "output": output,
        "status": "PASS" if passed else "FAIL",
    })

  success_rate = (passed_count / total_tests) * 100
  return {
      "total_tests": total_tests,
      "passed_count": passed_count,
      "success_rate_percentage": success_rate,
      "criteria_met": success_rate >= 80.0,
      "test_details": results,
  }


base_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
static_dir = os.path.join(base_dir, "static")

app.mount("/", StaticFiles(directory=static_dir, html=True), name="static")

if __name__ == "__main__":
  import uvicorn

  uvicorn.run("app.main:app", host="0.0.0.0", port=8000, reload=True)