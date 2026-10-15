"""Letter generation for ReverseIt.

``generate_letter(kind, dispute)`` renders one of two formal letters:

* ``"bank"`` - a complaint sent to the customer's bank
* ``"cbn"``  - an escalation sent to the Central Bank of Nigeria (CBN)

Every letter is built from the dispute record only. An optional LLM pass
polishes the wording of the finished letter, but it is never allowed to add
facts: any wording that drops or invents a fact is discarded and the
deterministic letter is returned instead. Because of that, ``generate_letter``
is fully usable with no API key configured.
"""

from __future__ import annotations

import os
import re
from typing import Any, Mapping

__all__ = [
    "PLACEHOLDERS",
    "SUPPORTED_KINDS",
    "MissingDisputeFieldError",
    "find_unfilled_placeholders",
    "generate_letter",
    "generate_letters",
]

# The placeholder names agreed by the team. Every template uses these names and
# nothing else. Keep this tuple and the templates below in step.
PLACEHOLDERS = ("amount", "date", "channel", "bank", "reference", "deadline")

SUPPORTED_KINDS = ("bank", "cbn")

# Any leftover "{name}" left in a finished letter is a bug, so the output is
# scanned for placeholders before it is returned.
_PLACEHOLDER_RE = re.compile(r"\{([a-zA-Z_][a-zA-Z0-9_]*)\}")

# Channel codes come from the alert parser; letters need readable prose.
_CHANNEL_LABELS = {
    "atm": "an ATM withdrawal",
    "pos": "a point-of-sale (POS) transaction",
    "transfer": "a bank transfer",
    "web": "an online (web) transaction",
}

BANK_COMPLAINT_TEMPLATE = """\
{date}

The Complaints Desk
{bank}

Dear Sir/Madam,

COMPLAINT ABOUT AN UNAUTHORISED DEBIT OF {amount} ON {date}

I am writing to report an unauthorised debit on my account with {bank}.

On {date}, {amount} was taken from my account through {channel}. I did not
authorise this transaction. The transaction reference is {reference}.

Please treat this letter as my formal complaint.

I am asking {bank} to:
  1. Reverse the {amount} debit and credit my account in full;
  2. Provide the transaction records and the beneficiary details for
     {reference}; and
  3. Confirm in writing what has been done.

I would be grateful for your written response on or before {deadline}. If the
money has not been returned by then, I will refer the matter to the Central
Bank of Nigeria.

Please contact me if you need any further information. Thank you for your
help.

Yours faithfully,
"""

CBN_ESCALATION_TEMPLATE = """\
{date}

The Director, Consumer Protection Department
Central Bank of Nigeria

Dear Sir/Madam,

ESCALATION OF AN UNRESOLVED COMPLAINT AGAINST {bank}

I am writing to escalate an unresolved complaint against {bank}.

On {date}, {amount} was debited from my account with {bank} through {channel}.
I did not authorise this transaction. The transaction reference is
{reference}. I reported the matter to {bank} and asked them to reverse the
debit, but the complaint has not been resolved.

I am asking the Central Bank of Nigeria to look into this complaint and to
direct {bank} to:
  1. Reverse the {amount} debit and credit my account in full;
  2. Explain how the transaction was authorised; and
  3. Respond to my complaint in writing.

I gave {bank} until {deadline} to resolve the complaint.

Thank you for your assistance. I am happy to provide any documents you need.

Yours faithfully,
"""

_TEMPLATES = {
    "bank": BANK_COMPLAINT_TEMPLATE,
    "cbn": CBN_ESCALATION_TEMPLATE,
}


class MissingDisputeFieldError(ValueError):
    """Raised when the dispute record lacks a field a letter needs."""

    def __init__(self, missing: "list[str] | tuple[str, ...]") -> None:
        self.missing = tuple(missing)
        super().__init__(
            "dispute record is missing required field(s): " + ", ".join(self.missing)
        )


def find_unfilled_placeholders(text: str) -> list[str]:
    """Return the placeholder names still present in ``text`` (empty if clean)."""
    if not isinstance(text, str):
        return []
    return _PLACEHOLDER_RE.findall(text)


def _is_blank(value: Any) -> bool:
    if value is None:
        return True
    if isinstance(value, str):
        return not value.strip()
    return False


def _format_amount(amount: Any) -> str:
    """Render the disputed amount as naira, keeping the value untouched."""
    if isinstance(amount, str):
        cleaned = amount.replace(",", "").replace("\u20a6", "").upper()
        cleaned = cleaned.replace("NGN", "").strip()
    else:
        cleaned = amount
    try:
        value = float(cleaned)  # type: ignore[arg-type]
    except (TypeError, ValueError):
        return str(amount).strip()
    if value.is_integer():
        return f"NGN {int(value):,}"
    return f"NGN {value:,.2f}"


def _build_values(dispute: Mapping[str, Any]) -> dict[str, str]:
    """Map the six dispute fields onto the six placeholder names."""
    if not isinstance(dispute, Mapping):
        raise TypeError("dispute must be a mapping of dispute fields")

    missing = [name for name in PLACEHOLDERS if _is_blank(dispute.get(name))]
    if missing:
        raise MissingDisputeFieldError(missing)

    channel = str(dispute["channel"]).strip().lower()
    return {
        "amount": _format_amount(dispute["amount"]),
        "date": str(dispute["date"]).strip(),
        "channel": _CHANNEL_LABELS.get(channel, f"{channel} transaction"),
        "bank": str(dispute["bank"]).strip(),
        "reference": str(dispute["reference"]).strip(),
        "deadline": str(dispute["deadline"]).strip(),
    }


def _normalise_kind(kind: str) -> str:
    normalised = str(kind or "").strip().lower()
    if normalised not in _TEMPLATES:
        raise ValueError(
            f"unsupported letter kind {kind!r}; expected one of "
            + ", ".join(repr(k) for k in SUPPORTED_KINDS)
        )
    return normalised


def _polish_with_llm(letter: str, values: Mapping[str, str]) -> str:
    """Ask an LLM to polish ``letter``, using only the dispute facts.

    Returns the original letter whenever the model is unavailable, fails, or
    returns wording that does not preserve every fact.
    """
    api_key = os.environ.get("ANTHROPIC_API_KEY")
    if not api_key:
        return letter

    try:
        import anthropic
    except ImportError:
        return letter

    facts = "\n".join(f"- {name}: {values[name]}" for name in PLACEHOLDERS)
    system = (
        "You are a careful editor of formal complaint letters. Improve grammar, "
        "tone and flow only. Never invent or change amounts, dates, deadlines, "
        "bank names, transaction references, rules or legal claims. Never add "
        "facts that are not in the supplied list. Return only the letter text."
    )
    prompt = (
        "Here is the finished letter and the only facts you may rely on.\n\n"
        f"FACTS\n{facts}\n\nLETTER\n{letter}\n\n"
        "Return the letter with the same facts and the same meaning, in plain, "
        "polite English. Do not add a subject line or placeholder text."
    )

    try:
        client = anthropic.Anthropic(api_key=api_key)
        message = client.messages.create(
            model=os.environ.get("ANTHROPIC_MODEL", "claude-3-5-sonnet-20241022"),
            max_tokens=1200,
            temperature=0,
            system=system,
            messages=[{"role": "user", "content": prompt}],
        )
        candidate = "".join(
            block.text
            for block in message.content
            if getattr(block, "type", None) == "text"
        ).strip()
    except Exception:  # network, auth, rate limit: keep the deterministic letter
        return letter

    return candidate


# The polish pass must not smuggle in numbers, dates or legal claims of its
# own, so the finished wording is re-checked against the dispute facts.
_MONEY_RE = re.compile(r"(?:NGN|\u20a6)\s*([\d][\d,]*(?:\.\d+)?)", re.IGNORECASE)
_DATE_RE = re.compile(
    r"\b\d{4}-\d{2}-\d{2}\b"
    r"|\b\d{1,2}[/-]\d{1,2}[/-]\d{2,4}\b"
    r"|\b\d{1,2}\s+[A-Za-z]{3,9}\s+\d{4}\b"
    r"|\b\d{1,2}-[A-Za-z]{3}-\d{4}\b"
)
_LEGAL_RE = re.compile(
    r"\b(section|act|regulation|clause|statute|decree|subsidiary\s+legislation)\b",
    re.IGNORECASE,
)


def _digits(token: str) -> str:
    return re.sub(r"\D", "", token)


def _introduces_new_amounts(text: str, values: Mapping[str, str]) -> bool:
    allowed = {_digits(values["amount"])}
    return any(_digits(m.group(1)) not in allowed for m in _MONEY_RE.finditer(text))


def _introduces_new_dates(text: str, values: Mapping[str, str]) -> bool:
    allowed = {values["date"].strip().lower(), values["deadline"].strip().lower()}
    return any(
        m.group(0).strip().lower() not in allowed for m in _DATE_RE.finditer(text)
    )


def _introduces_new_legal_claims(candidate: str, original: str) -> bool:
    known = {token.group(0).lower() for token in _LEGAL_RE.finditer(original)}
    return any(
        token.group(0).lower() not in known for token in _LEGAL_RE.finditer(candidate)
    )


def _is_usable_polish(
    candidate: str, values: Mapping[str, str], original: str
) -> bool:
    """True only if the polished text is clean, complete and invents nothing."""
    if not candidate or not candidate.strip():
        return False
    if find_unfilled_placeholders(candidate):
        return False
    # Every fact must survive the rewrite, otherwise the model dropped - or
    # paraphrased away - something it was not allowed to touch.
    if not all(value in candidate for value in values.values()):
        return False
    if _introduces_new_amounts(candidate, values):
        return False
    if _introduces_new_dates(candidate, values):
        return False
    if _introduces_new_legal_claims(candidate, original):
        return False
    return True


def generate_letter(kind: str, dispute: Mapping[str, Any]) -> str:
    """Return the finished text of a ``"bank"`` or ``"cbn"`` letter.

    The letter is filled from ``dispute`` first, then optionally polished by an
    LLM. A letter is only ever returned when it contains no unfilled
    ``{placeholder}``.
    """
    template = _TEMPLATES[_normalise_kind(kind)]
    values = _build_values(dispute)

    filled = template.format(**values)
    if find_unfilled_placeholders(filled):
        # A template with an unknown placeholder is a programming error, not a
        # bad dispute record, so fail loudly rather than ship a broken letter.
        raise RuntimeError(
            "letter template produced unfilled placeholders: "
            + ", ".join(find_unfilled_placeholders(filled))
        )

    polished = _polish_with_llm(filled, values)
    if _is_usable_polish(polished, values, filled):
        filled = polished

    return filled


def generate_letters(dispute: Mapping[str, Any]) -> dict[str, str]:
    """Return both letters for ``dispute`` keyed by kind."""
    return {kind: generate_letter(kind, dispute) for kind in SUPPORTED_KINDS}
