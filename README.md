# ReverseIt

Helps customers dispute failed bank transactions: paste a debit alert, get the reversal deadline, and generate a ready-to-send complaint.

---

## Python Version
- **Requires**: Python `>= 3.10` (Tested on `Python 3.14`)

---

## Installation & Setup

1. **Clone the repository:**
   ```bash
   git clone https://github.com/OLAMIGHTY010/reverseit.git
   cd reverseit
   ```

2. **Create and activate a virtual environment:**
   ```bash
   # macOS / Linux
   python -m venv venv
   source venv/bin/activate

   # Windows
   python -m venv venv
   venv\Scripts\activate
   ```

3. **Install dependencies:**
   ```bash
   pip install -r requirements.txt
   ```

4. **Environment variables:**
   ```bash
   cp .env.example .env
   ```
   Add your optional `ANTHROPIC_API_KEY` in `.env` for LLM parsing fallback. (Never commit `.env`!).

5. **Run the server:**
   ```bash
   uvicorn app.main:app --reload
   ```
   - Health check: `http://127.0.0.1:8000/` (returns `{"status": "ok", "app": "ReverseIt"}`)
   - Interactive docs: `http://127.0.0.1:8000/docs`

6. **Run tests:**
   ```bash
   pytest
   ```

---

## Core Function Signatures (Agreed with Pairs)

### 1. Alert Parser
```python
def parse_alert(text: str) -> dict:
    ...
```
- **Success shape**:
  ```json
  {
    "amount": 20000.0,
    "date": "2026-10-13",
    "time": "14:05",
    "channel": "pos",
    "bank": "ExampleBank",
    "reference": "12345"
  }
  ```
- **Failure shape**:
  ```json
  {
    "error": "could_not_parse",
    "missing": ["amount", "reference"]
  }
  ```
- Note: `channel` is strictly one of: `"atm"`, `"pos"`, `"transfer"`, `"web"`.

### 2. Deadline Calculator
```python
def compute_deadline(channel: str, transaction_time: datetime) -> datetime:
    ...
```
- Computed by **deterministic regulatory rules only** (never by LLM).
- Reversal windows defined in a single centralized config dictionary in `app/deadlines.py`.

### 3. Complaint Letter Generator
```python
def generate_letter(kind: str, dispute: dict | object) -> str:
    ...
```
- `kind`: `"bank"` (demand for immediate refund) or `"cbn"` (Consumer Protection Department escalation petition).

---

## Database Model (`models.py`)

SQLite database (`reverseit.db`) managed via SQLAlchemy.

| Field | Type | Description |
|---|---|---|
| `id` | Integer (PK) | Auto-incrementing primary key |
| `amount` | Float | Transaction amount debited |
| `channel` | String | `"atm"`, `"pos"`, `"transfer"`, `"web"` |
| `bank` | String | Bank / financial institution name |
| `reference` | String | Unique transaction reference or session ID |
| `transaction_time` | DateTime | Timestamp when the transaction took place |
| `deadline` | DateTime | Regulatory deadline for reversal |
| `status` | String | Current dispute resolution state |

### Allowed Status Values:
- `awaiting_reversal`: Initial state within regulatory turnaround window.
- `due_soon`: Nearing regulatory deadline (e.g. within 2 hours).
- `deadline_passed`: Window expired without reversal; eligible for CBN escalation.
- `resolved`: Transaction successfully reversed or resolved.

---

## API Routes (`app/main.py`)

All routes are kept thin, delegating core logic to specialized modules:

- `GET /` — Health check status (`{"status": "ok", "app": "ReverseIt"}`).
- `POST /disputes` — Accepts debit alert text; parses fields, calculates deadline via rule engine, and saves to SQLite.
- `GET /disputes` — Lists all disputes (optional filter query parameter: `?status=...`).
- `PATCH /disputes/{id}` — Updates dispute status (`status: "awaiting_reversal" | "due_soon" | "deadline_passed" | "resolved"`).
- `POST /disputes/{id}/letter?kind=bank|cbn` — Generates formatted bank demand letter or CBN petition.

---

## Scheduler (`app/scheduler.py`)

Runs periodic checks in the background using `APScheduler`:
- Checks active disputes (`awaiting_reversal` and `due_soon`).
- Automatically transitions disputes to `due_soon` when within the approaching threshold.
- Automatically transitions disputes to `deadline_passed` once the deadline has elapsed.

---

## Project Layout

```
├── app/
│   ├── __init__.py
│   ├── database.py       # SQLite connection and session management
│   ├── deadlines.py      # Rule-based reversal turnaround windows & compute_deadline
│   ├── letters.py        # Formal dispute letter templates (Bank & CBN)
│   ├── main.py           # FastAPI application & thin routes
│   ├── models.py         # SQLAlchemy Dispute model & statuses
│   ├── parser.py         # Regex extraction & LLM fallback parser
│   └── scheduler.py      # Background deadline monitoring job
├── data/
│   └── sample_alerts/    # Sample debit alert datasets
│       └── samples.json
├── static/               # Frontend static assets
├── tests/
│   ├── test_backend.py   # Full backend test suite
│   └── test_main.py      # Root endpoint test
├── .env.example          # Environment template (names only, no secrets)
├── .gitignore            # Ignores .env, *.db, pycache, virtualenvs
├── pytest.ini            # Pytest configuration
├── requirements.txt      # Python dependencies with version note
└── README.md             # Project documentation & team signatures
```

---

## Rules
- One branch per task, a pull request for every change, no pushing directly to `main`.
- Never commit `.env`, `*.db`, or any API keys.
- Keep routes thin and modular.
