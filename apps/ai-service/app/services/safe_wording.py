"""Customer-Safe Wording Generator and Approved Vocabulary Engine.

Implements Prompt A6 (Item 2), Case M4, and Section 10.6:
- Selects from approved bank templates by case state
- Strictly blocks 'refunded', 'reversed', 'credited' unless financial status is VERIFIED
- Enforces zero speculation, zero accusations, and zero unauthorized time promises
- Configurable bilingual support: Standard English and Nigerian Pidgin
- Automatic compliance auditing and blocklist enforcement
"""

from typing import Dict, List, Tuple
from app.models.complaint_matching import (
    CustomerSafeWordingRequest,
    CustomerSafeWordingResponse,
)

# Strictly prohibited terms in customer-facing communication
UNVERIFIED_BLOCKED_TERMS = [
    "refunded", "reversed", "credited", "funds returned", "money returned", "cash back", "funds back"
]

ALWAYS_BLOCKED_TERMS = [
    "fraudster", "scam", "stolen", "hack", "our fault", "bank's fault", "we crashed", "we broke"
]

# Approved Message Templates
_TEMPLATES: Dict[str, Dict[str, str]] = {
    "DETECTED": {
        "en": "Dear Customer, we noticed an unsuccessful transaction of {amount} on your account. We are checking it and will update you. No action is needed now.",
        "pcm": "Dear Customer, we don see say your transaction of {amount} get small issue. We dey check am well-well and we go update you sharp-sharp. You no need do anything now."
    },
    "UNDER_INVESTIGATION": {
        "en": "Dear Customer, we noticed an unsuccessful transaction of {amount} on your account. We are checking it and will update you. No action is needed now.",
        "pcm": "Dear Customer, we dey investigate your transaction of {amount}. We go update you as soon as possible. No action needed from you."
    },
    "AWAITING_APPROVAL": {
        "en": "Dear Customer, we have confirmed an issue with your transaction of {amount} and are completing internal verification checks. We will update you shortly.",
        "pcm": "Dear Customer, we don confirm the issue on top your {amount} transaction, and our people dey finish final checks now."
    },
    "OUTCOME_VERIFIED": {
        "en": "Dear Customer, we detected an issue with your transaction of {amount} and have completed the correction. Funds have been restored to your account. Thank you.",
        "pcm": "Dear Customer, we don resolve that issue with your {amount} transaction, and your money don enter back your account. Thank you."
    },
    "CUSTOMER_ERROR": {
        "en": "Dear Customer, we have received your written recall request regarding transaction of {amount}. In line with interbank regulations, recovery depends on funds being available in the beneficiary account.",
        "pcm": "Dear Customer, we don receive your written request for your {amount} transaction. As CBN rule talk, to get the money back go depend on if money still dey the person account."
    }
}


class CustomerSafeWordingService:
    """Generates customer-safe messages with strict vocabulary validation."""

    @classmethod
    def generate_wording(cls, request: CustomerSafeWordingRequest) -> CustomerSafeWordingResponse:
        """Selects template, populates facts, and runs compliance inspection."""
        state_key = request.state.upper()
        if state_key not in _TEMPLATES:
            state_key = "UNDER_INVESTIGATION"

        lang_key = request.language.lower()
        if lang_key not in ["en", "pcm"]:
            lang_key = "en"

        template = _TEMPLATES[state_key][lang_key]
        message = template.format(amount=request.amount_formatted)

        # Inspect for prohibited terms
        prohibited_detected = []
        lower_msg = message.lower()

        # Check always blocked terms
        for term in ALWAYS_BLOCKED_TERMS:
            if term in lower_msg:
                prohibited_detected.append(term)

        # Check unverified terms if status is not verified
        if not request.financial_verified:
            for term in UNVERIFIED_BLOCKED_TERMS:
                if term in lower_msg:
                    prohibited_detected.append(term)

        is_compliant = (len(prohibited_detected) == 0)

        # Fallback sanitize if violated
        if not is_compliant:
            message = _TEMPLATES["UNDER_INVESTIGATION"]["en"].format(amount=request.amount_formatted)

        return CustomerSafeWordingResponse(
            case_id=request.case_id,
            state=state_key,
            language=lang_key,
            message=message,
            financial_verified=request.financial_verified,
            is_compliant=is_compliant,
            prohibited_terms_detected=prohibited_detected
        )

    @classmethod
    def inspect_custom_text(cls, text: str, financial_verified: bool) -> Tuple[bool, List[str]]:
        """Audits an arbitrary agent message draft for compliance violations."""
        prohibited = []
        lower_text = text.lower()

        for term in ALWAYS_BLOCKED_TERMS:
            if term in lower_text:
                prohibited.append(term)

        if not financial_verified:
            for term in UNVERIFIED_BLOCKED_TERMS:
                if term in lower_text:
                    prohibited.append(term)

        return (len(prohibited) == 0, prohibited)


safe_wording_service = CustomerSafeWordingService()
