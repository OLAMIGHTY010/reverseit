# ReverseIt

Helps customers dispute failed bank transactions: paste a debit alert, get the reversal deadline, and a ready-to-send complaint.

## Run it
```bash
python -m venv venv
source venv/bin/activate        # Windows: venv\Scripts\activate
pip install -r requirements.txt
uvicorn app.main:app --reload
```

Open http://127.0.0.1:8000 (should show "ok") and http://127.0.0.1:8000/docs.

## Layout
```
app/        backend modules (parsing, deadlines, letters, scheduler)
static/     frontend pages
data/       invented sample alerts
tests/      pytest tests
```

## Rules
One branch per task, a pull request for every change, no pushing to main. Never commit .env or any API key.
