"""Dispute Intent Classifier and Deterministic Regulatory Clock Mapper.

Implements Prompt A9 (Item 3), Section 10.4, and Rule R6:
- Proposes dispute type with confidence and evidence phrases
- Strictly maps dispute type to regulatory clock via DETERMINISTIC TABLE
- The LLM NEVER selects a clock; clock assignment is pure deterministic configuration
- Routes low-confidence cases (< 0.70) to human review
"""

import re
from typing import Dict, List, Tuple, Any, Optional
from app.models.reverseit import (


    DisputeType,
    DisputeClassificationResult,
)

# Section 10.4 Deterministic Regulatory Clock Registry Mapping
_DETERMINISTIC_CLOCK_REGISTRY: Dict[DisputeType, Dict[str, Any]] = {
    DisputeType.FAILED_TRANSFER: {
        "clock_id": "NIP_FAILED_REVERSAL",
        "clock_name": "CBN 2018 Failed Instant Payment Circular",
        "duration_hours": 24,
        "unit": "hours",
        "source_ref": "CBN/BPS/DIR/GEN/CIR/01/011 (2 Oct 2018)",
        "status": "confirmed",
    },
    DisputeType.DUPLICATE_DEBIT: {
        "clock_id": "NIP_FAILED_REVERSAL",
        "clock_name": "Duplicate Debit Correction Window",
        "duration_hours": 24,
        "unit": "hours",
        "source_ref": "Internal Target / CBN 2018 Circular",
        "status": "confirmed",
    },
    DisputeType.POS_WEB_FAILURE: {
        "clock_id": "POS_WEB_DISPUTE",
        "clock_name": "CBN 2020 POS/Web Refund Timeline",
        "duration_hours": 72,
        "unit": "hours",
        "source_ref": "CBN Statement 8 June 2020 (conflicting 48h report flagged)",
        "status": "conflicting",
    },
    DisputeType.ATM_FAILURE_ON_US: {
        "clock_id": "ATM_ONUS",
        "clock_name": "On-Us ATM Instant/Manual Reversal Target",
        "duration_hours": 24,
        "unit": "hours",
        "source_ref": "CBN Guidelines on ATM Operations",
        "status": "reported",
    },
    DisputeType.ATM_FAILURE_NOT_ON_US: {
        "clock_id": "ATM_NOTONUS",
        "clock_name": "Not-On-Us ATM 48-Hour Reversal Timeline",
        "duration_hours": 48,
        "unit": "hours",
        "source_ref": "CBN 2020 Timelines Statement",
        "status": "confirmed",
    },
    DisputeType.WRONG_ACCOUNT_OR_AMOUNT: {
        "clock_id": "WRITTEN_REVERSAL_REQUEST",
        "clock_name": "Customer Written Reversal Request Window",
        "duration_hours": 336,  # 14 working days (14 * 24)
        "unit": "working_days",
        "source_ref": "CBN 2018 Instant Payment Circular Section 4",
        "status": "confirmed",
    },
    DisputeType.OTHER: {
        "clock_id": "ACK_TARGET",
        "clock_name": "Bank Dispute Acknowledgement Target",
        "duration_hours": 24,
        "unit": "hours",
        "source_ref": "Bank Customer SLA / Complaint Manual",
        "status": "confirmed",
    },
}

# Heuristic patterns for dispute classification
_PATTERNS = {
    DisputeType.DUPLICATE_DEBIT: [
        re.compile(r'(?i)\b(?:debited\s+twice|double\s+debit|duplicate\s+debit|two\s+times|charged\s+twice)\b'),
    ],
    DisputeType.ATM_FAILURE_ON_US: [
        re.compile(r'(?i)\b(?:atm\s+did\s+not\s+dispense|cash\s+not\s+dispensed|no\s+cash\s+out|wema\s+atm)\b'),
    ],
    DisputeType.ATM_FAILURE_NOT_ON_US: [
        re.compile(r'(?i)\b(?:other\s+bank\s+atm|another\s+bank\s+atm|gtb\s+atm|zenith\s+atm|access\s+atm)\b'),
    ],
    DisputeType.POS_WEB_FAILURE: [
        re.compile(r'(?i)\b(?:pos\s+declined|pos\s+error|merchant\s+said\s+declined|web\s+payment\s+failed|paystack\s+timeout)\b'),
    ],
    DisputeType.WRONG_ACCOUNT_OR_AMOUNT: [
        re.compile(r'(?i)\b(?:wrong\s+account|wrong\s+person|sent\s+to\s+wrong|mistake\s+in\s+amount|excess\s+amount)\b'),
    ],
    DisputeType.FAILED_TRANSFER: [
        re.compile(r'(?i)\b(?:recipient\s+not\s+credited|beneficiary\s+not\s+received|transfer\s+failed|money\s+not\s+delivered|interbank\s+delay)\b'),
    ],
}


class DisputeClassifier:
    """Classifies customer dispute claims and strictly applies deterministic regulatory clocks."""

    @classmethod
    def classify(
        cls,
        customer_description: str,
        channel: Optional[str] = None,
        narration: Optional[str] = None
    ) -> DisputeClassificationResult:
        """Classifies dispute text and looks up the deterministic clock."""
        combined_text = f"{customer_description} {channel or ''} {narration or ''}"
        
        matched_type = DisputeType.OTHER
        max_matches = 0
        evidence_phrases: List[str] = []

        for dtype, regex_list in _PATTERNS.items():
            matches_found = []
            for pattern in regex_list:
                for match in pattern.finditer(combined_text):
                    matches_found.append(match.group(0))

            if len(matches_found) > max_matches:
                max_matches = len(matches_found)
                matched_type = dtype
                evidence_phrases = matches_found

        # Channel-based fallbacks if text is ambiguous
        confidence = 0.50
        if max_matches > 0:
            confidence = min(0.75 + (max_matches * 0.1), 0.98)
        else:
            if channel == "POS" or channel == "WEB":
                matched_type = DisputeType.POS_WEB_FAILURE
                confidence = 0.65
                evidence_phrases.append(f"Channel is {channel}")
            elif channel == "ATM":
                matched_type = DisputeType.ATM_FAILURE_ON_US
                confidence = 0.60
                evidence_phrases.append("Channel is ATM")
            elif channel in ["NIP", "MOBILE", "USSD"]:
                matched_type = DisputeType.FAILED_TRANSFER
                confidence = 0.65
                evidence_phrases.append(f"Channel is {channel}")

        # Deterministic clock lookup
        clock_data = _DETERMINISTIC_CLOCK_REGISTRY[matched_type]
        requires_review = (confidence < 0.70)

        return DisputeClassificationResult(
            proposed_type=matched_type,
            confidence=round(confidence, 2),
            evidence_phrases=evidence_phrases,
            requires_human_review=requires_review,
            clock_id=clock_data["clock_id"],
            clock_name=clock_data["clock_name"],
            clock_duration_hours=clock_data["duration_hours"],
            clock_unit=clock_data["unit"],
            clock_source_ref=clock_data["source_ref"],
            clock_status=clock_data["status"],
        )


intent_classifier = DisputeClassifier()
