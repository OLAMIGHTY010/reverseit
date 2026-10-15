"""Tests for app.letters: the bank complaint and the CBN escalation letter."""

import re

import pytest

from app.letters import (
    PLACEHOLDERS,
    SUPPORTED_KINDS,
    MissingDisputeFieldError,
    find_unfilled_placeholders,
    generate_letter,
    generate_letters,
)

DISPUTE = {
    "amount": 20000,
    "date": "2026-10-13",
    "channel": "pos",
    "bank": "ExampleBank",
    "reference": "12345",
    "deadline": "2026-10-27",
}


@pytest.fixture(autouse=True)
def no_api_key(monkeypatch):
    """Keep the suite deterministic: never call an LLM from the tests."""
    monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)


@pytest.mark.parametrize("kind", SUPPORTED_KINDS)
def test_generate_letter_returns_text(kind):
    letter = generate_letter(kind, DISPUTE)
    assert isinstance(letter, str)
    assert letter.strip()


@pytest.mark.parametrize("kind", SUPPORTED_KINDS)
def test_no_unfilled_placeholders(kind):
    """The acceptance criterion: no leftover {placeholder} in the output."""
    letter = generate_letter(kind, DISPUTE)
    assert find_unfilled_placeholders(letter) == []
    assert re.search(r"\{[a-zA-Z_][a-zA-Z0-9_]*\}", letter) is None


@pytest.mark.parametrize("kind", SUPPORTED_KINDS)
def test_letter_contains_every_dispute_fact(kind):
    letter = generate_letter(kind, DISPUTE)
    assert "NGN 20,000" in letter
    assert "2026-10-13" in letter
    assert "2026-10-27" in letter
    assert "ExampleBank" in letter
    assert "12345" in letter
    assert "point-of-sale (POS) transaction" in letter


def test_bank_letter_is_addressed_to_the_bank():
    letter = generate_letter("bank", DISPUTE)
    assert "The Complaints Desk" in letter
    assert "ExampleBank" in letter


def test_cbn_letter_is_addressed_to_the_cbn():
    letter = generate_letter("cbn", DISPUTE)
    assert "Central Bank of Nigeria" in letter
    assert "Consumer Protection" in letter


def test_letters_are_different():
    letters = generate_letters(DISPUTE)
    assert set(letters) == set(SUPPORTED_KINDS)
    assert letters["bank"] != letters["cbn"]


def test_kind_is_case_insensitive():
    assert generate_letter("BANK", DISPUTE) == generate_letter("bank", DISPUTE)


def test_unknown_kind_is_rejected():
    with pytest.raises(ValueError):
        generate_letter("police", DISPUTE)


@pytest.mark.parametrize("field", PLACEHOLDERS)
def test_missing_field_is_rejected(field):
    dispute = {k: v for k, v in DISPUTE.items() if k != field}
    with pytest.raises(MissingDisputeFieldError) as excinfo:
        generate_letter("bank", dispute)
    assert field in excinfo.value.missing


def test_blank_field_is_rejected():
    dispute = dict(DISPUTE, reference="   ")
    with pytest.raises(MissingDisputeFieldError):
        generate_letter("cbn", dispute)


@pytest.mark.parametrize(
    "amount,expected",
    [
        (20000, "NGN 20,000"),
        (20000.0, "NGN 20,000"),
        (20000.5, "NGN 20,000.50"),
        ("NGN20,000.00", "NGN 20,000"),
        ("\u20a620,000.00", "NGN 20,000"),
    ],
)
def test_amount_is_formatted(amount, expected):
    assert expected in generate_letter("bank", dict(DISPUTE, amount=amount))


def test_channels_render_as_prose():
    labels = {
        "atm": "an ATM withdrawal",
        "pos": "a point-of-sale (POS) transaction",
        "transfer": "a bank transfer",
        "web": "an online (web) transaction",
    }
    for channel, label in labels.items():
        letter = generate_letter("bank", dict(DISPUTE, channel=channel))
        assert label in letter


def test_unknown_channel_falls_back_to_its_name():
    letter = generate_letter("bank", dict(DISPUTE, channel="ussd"))
    assert "ussd transaction" in letter


def test_polished_letter_is_used_when_it_keeps_the_facts(monkeypatch):
    import app.letters as letters

    def fake_polish(letter, values):
        return letter.replace("Dear Sir/Madam,", "Dear Sir/Madam,") + "\n"

    monkeypatch.setattr(letters, "_polish_with_llm", fake_polish)
    polished = generate_letter("bank", DISPUTE)
    assert polished.endswith("\n")
    assert find_unfilled_placeholders(polished) == []


def test_polish_that_drops_a_fact_is_discarded(monkeypatch):
    import app.letters as letters

    monkeypatch.setattr(
        letters,
        "_polish_with_llm",
        lambda letter, values: "Dear Sir/Madam,\n\nPlease reverse my money.\n",
    )
    letter = generate_letter("bank", DISPUTE)
    assert "NGN 20,000" in letter
    assert "12345" in letter


def test_polish_that_invents_a_rule_is_discarded(monkeypatch):
    import app.letters as letters

    def inventing_polish(letter, values):
        return letter.replace(
            "Thank you", "Under section 12 of the Banks Act, you owe me NGN 50,000"
        )

    monkeypatch.setattr(letters, "_polish_with_llm", inventing_polish)
    letter = generate_letter("bank", DISPUTE)
    assert "NGN 50,000" not in letter
    assert "section 12" not in letter


def test_polish_with_placeholders_is_discarded(monkeypatch):
    import app.letters as letters

    monkeypatch.setattr(
        letters,
        "_polish_with_llm",
        lambda letter, values: "COMPLAINT ABOUT {amount} ON {date}",
    )
    letter = generate_letter("bank", DISPUTE)
    assert find_unfilled_placeholders(letter) == []
