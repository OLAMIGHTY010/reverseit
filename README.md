Turn a pasted debit alert into structured fields.

The function everyone depends on
parse_alert(text: str) -> dict

Success returns:
{"amount": 20000, "date": "2026-10-13", "time": "14:05", "channel": "pos", "bank": "ExampleBank", "reference": "12345"}

Failure returns:
{"error": "could_not_parse", "missing": ["amount", "reference"]}

channel must be one of: atm, pos, transfer, web.

Tasks

Write 5 invented alert formats from different banks (no real customer data) and list the fields each one contains. Share them with Workstream 4.

Step 1: write regex rules that parse the sample alerts. This is the working version.

Step 2: add an LLM fallback for alerts the regex misses. Ask the model for strict JSON, then VALIDATE it in code (amount is a number, date is real, channel is one of the four). If validation fails, return the could_not_parse result.

Never crash on bad input. Always return a result in one of the two shapes above.

Test every sample alert and record pass or fail.
Done when
parse_alert correctly handles at least 80% of the sample alerts and returns a clean could_not_parse for the rest.

Due
Regex version: Wednesday noon.
LLM fallback: Wednesday night.
Stretch (only if finished early): screenshot upload
