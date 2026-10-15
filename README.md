# ReverseIt

ReverseIt helps a Nigerian bank customer challenge a debit they did not
authorise. It has two components:

1. **Alert parsing** - a pasted debit alert (SMS or email) becomes a
   structured dispute record.
2. **Letter generation** - the dispute record becomes a formal complaint to
   the bank and, if the bank does not resolve it, an escalation to the Central
   Bank of Nigeria (CBN).

This branch covers the **letters, the test set and the docs**.

## Core function

```python
from app.letters import generate_letter

letter = generate_letter("bank", dispute)   # "bank" or "cbn"
```

`generate_letter(kind, dispute) -> str` fills one of two templates from the
dispute record, then optionally asks an LLM to polish the wording. It only
returns text that contains **no unfilled `{placeholder}`**.

`generate_letters(dispute)` returns both letters keyed by kind.

## The placeholder contract

Both letters are built from the same six placeholders. Nothing else is allowed
in a template.

| Placeholder | Meaning | Example |
| --- | --- | --- |
| `{amount}` | disputed amount | `NGN 20,000` |
| `{date}` | date of the debit | `2026-10-13` |
| `{channel}` | how it happened | `a point-of-sale (POS) transaction` |
| `{bank}` | the customer's bank | `ExampleBank` |
| `{reference}` | transaction reference | `12345` |
| `{deadline}` | date the bank must reply by | `2026-10-27` |

So a dispute record looks like this:

```python
dispute = {
    "amount": 20000,
    "date": "2026-10-13",
    "channel": "pos",
    "bank": "ExampleBank",
    "reference": "12345",
    "deadline": "2026-10-27",
}
```

`channel` accepts `atm`, `pos`, `transfer` or `web` (the values the alert
parser emits) and renders as readable prose. A missing or blank field raises
`MissingDisputeFieldError` rather than producing a letter with a placeholder in
it.

### Keeping the LLM honest

The polish step may only improve grammar, tone and flow. Before any polished
text is returned it is checked:

- no `{placeholder}` is left;
- every dispute fact still appears;
- no new amount or date was introduced;
- no legal or regulatory claim appears that was not already in the letter.

If any check fails, the deterministic template letter is returned instead. The
module therefore works with **no API key at all**.

## Install

```bash
python3 -m venv .venv
source .venv/bin/activate        # Windows: .venv\Scripts\activate
pip install -r requirements.txt
cp .env.example .env             # optional: add ANTHROPIC_API_KEY for polish
```

## Run

```bash
# End-to-end demo: dispute record -> both letters
python demo.py

# Tests
python -m pytest -q

# API server (skeleton)
uvicorn app.main:app --reload
```

## Folder layout

```
.
├── app/
│   ├── __init__.py
│   ├── letters.py          # generate_letter(): templates + guarded LLM polish
│   └── main.py             # FastAPI app skeleton
├── data/
│   └── sample_alerts/      # 13 invented debit alerts + index (parser input)
├── docs/
│   └── alert_parsing_plan.md
├── tests/
│   ├── test_letters.py     # pytest suite for the letters
│   ├── test_main.py        # pytest suite for the API skeleton
│   └── test_table.md       # manual test table: input, expected, actual, pass/fail
├── demo.py                 # demo script
├── requirements.txt
└── .env.example
```

## Environment

| Variable | Required | Purpose |
| --- | --- | --- |
| `ANTHROPIC_API_KEY` | no | enables the LLM polish step |
| `ANTHROPIC_MODEL` | no | overrides the model name |
| `HOST`, `PORT`, `ENVIRONMENT` | no | server settings |

**Never commit an API key.** `.env` is git-ignored; only `.env.example` is
tracked, and it holds no secrets. Without `ANTHROPIC_API_KEY` the letters are
generated from the templates unchanged.

## Related documents

- [Alert parsing plan](docs/alert_parsing_plan.md)
- [Test table](tests/test_table.md)
- [Sample alerts](data/sample_alerts/README.md)
