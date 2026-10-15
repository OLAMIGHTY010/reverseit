"""Demo: turn one dispute record into the bank complaint and the CBN letter.

Run with:  python demo.py

The demo works with or without an LLM configured. When ANTHROPIC_API_KEY is
set the wording is polished by the model; when it is not, the letters come
straight from the templates. Either way the output is checked for leftover
placeholders before it is printed.
"""

from app.letters import (
    PLACEHOLDERS,
    MissingDisputeFieldError,
    find_unfilled_placeholders,
    generate_letters,
)

# One invented dispute record. Every field a letter needs is here.
DISPUTE = {
    "amount": 20000,
    "date": "2026-10-13",
    "channel": "pos",
    "bank": "ExampleBank",
    "reference": "12345",
    "deadline": "2026-10-27",
}


def main() -> int:
    print("Placeholder contract")
    print("  " + "  ".join("{" + name + "}" for name in PLACEHOLDERS))
    print()
    print("Dispute record")
    for name in PLACEHOLDERS:
        print(f"  {name}: {DISPUTE[name]}")
    print()

    letters = generate_letters(DISPUTE)

    for kind, heading in (("bank", "BANK COMPLAINT"), ("cbn", "CBN ESCALATION")):
        print("=" * 72)
        print(heading)
        print("=" * 72)
        print(letters[kind])
        print()

    leftover = {
        kind: find_unfilled_placeholders(text) for kind, text in letters.items()
    }
    if any(leftover.values()):
        print("FAIL: unfilled placeholders left:", leftover)
        return 1

    print("OK: both letters generated with no unfilled placeholders.")

    # A dispute record that is missing a field must fail loudly, not print a
    # letter containing "{deadline}".
    try:
        generate_letters({k: v for k, v in DISPUTE.items() if k != "deadline"})
    except MissingDisputeFieldError as exc:
        print(f"OK: incomplete dispute rejected -> {exc}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
