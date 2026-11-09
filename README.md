# ReverseIt Alert Parser

ReverseIt parses Nigerian bank transaction alerts using a regex parser first
and a Groq-powered LLM fallback when regex cannot confidently parse an alert.

## Setup

You need Python 3.10 or newer and your own Groq API key to use the LLM fallback.
Each person running the project must create and use their own key; do not share
or commit API keys.

1. Clone the repository and move into its directory.
2. Create and activate a virtual environment:

   ```bash
   python3 -m venv .venv
   source .venv/bin/activate
   ```

3. Install the project dependencies:

   ```bash
   python -m pip install -r requirements.txt
   ```

4. Create a `.env` file in the project root and add your own key:

   ```dotenv
   GROQ_API_KEY=your_groq_api_key_here
   ```

   Replace `your_groq_api_key_here` with your key from Groq. The `.env` file is
   ignored by Git; keep it private and never commit it.

## Run

With the virtual environment activated and `.env` configured, run:

```bash
python parse.py
```

This runs the parser against the sample alerts in `parse.py`. The LLM fallback
requires a valid `GROQ_API_KEY`; without one, regex parsing may still succeed,
but alerts that need the fallback will not be parsed by the LLM.

## Test

Run the parser tests with:

```bash
python test_parser.py
```

The tests use a stubbed LLM response to verify that the fallback path is called
without sending requests to Groq or requiring an API key.
