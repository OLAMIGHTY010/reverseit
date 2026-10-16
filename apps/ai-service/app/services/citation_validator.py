"""Citation and Grounding Validator for SMARTREVERSAL AI Summaries.

Implements Prompt A2 (Item 1 & Acceptance Criteria):
- Verifies that every cited evidence_id exists within the CanonicalEvidenceBundle
- Detects and flags fabricated or hallucinated evidence IDs
- Flags uncited assertions
- Sanitizes or rejects ungrounded claims to ensure 100% evidence faithfulness
"""

from typing import Set, List
from app.models.context import CanonicalEvidenceBundle
from app.models.schemas import (
    InvestigationSummaryResponse,
    CitationValidationResult,
    CitedFact,
    CitedInference,
)


class CitationValidationError(Exception):
    """Raised when generated AI text references invalid or hallucinated evidence IDs."""
    pass


class CitationValidator:
    """Validates citations against the provided canonical evidence bundle."""

    @classmethod
    def validate_summary(
        cls,
        summary: InvestigationSummaryResponse,
        bundle: CanonicalEvidenceBundle
    ) -> CitationValidationResult:
        """Validates all citations across confirmed facts and inferred explanations."""
        valid_bundle_ids: Set[str] = set(bundle.provided_evidence_ids)

        verified_ids: Set[str] = set()
        fabricated_ids: Set[str] = set()
        uncited_statements: List[str] = []

        # 1. Validate Confirmed Facts
        for fact in summary.confirmed_facts:
            if not fact.evidence_ids:
                uncited_statements.append(f"Fact: {fact.text}")
                continue
            for eid in fact.evidence_ids:
                if eid in valid_bundle_ids:
                    verified_ids.add(eid)
                else:
                    fabricated_ids.add(eid)

        # 2. Validate Inferred Explanations
        for inference in summary.inferred_explanations:
            if not inference.evidence_ids:
                uncited_statements.append(f"Inference: {inference.text}")
                continue
            for eid in inference.evidence_ids:
                if eid in valid_bundle_ids:
                    verified_ids.add(eid)
                else:
                    fabricated_ids.add(eid)

        is_valid = (len(fabricated_ids) == 0) and (len(uncited_statements) == 0)
        error_msg = None
        if not is_valid:
            parts = []
            if fabricated_ids:
                parts.append(f"Fabricated evidence IDs detected: {sorted(list(fabricated_ids))}")
            if uncited_statements:
                parts.append(f"Uncited statements: {len(uncited_statements)}")
            error_msg = "; ".join(parts)

        return CitationValidationResult(
            is_valid=is_valid,
            verified_evidence_ids=sorted(list(verified_ids)),
            fabricated_evidence_ids=sorted(list(fabricated_ids)),
            uncited_statements=uncited_statements,
            error_message=error_msg
        )

    @classmethod
    def filter_ungrounded_statements(
        cls,
        summary: InvestigationSummaryResponse,
        bundle: CanonicalEvidenceBundle
    ) -> InvestigationSummaryResponse:
        """Filters out statements with invalid or missing citations.
        
        If all confirmed facts become invalid, caller should trigger deterministic fallback.
        """
        valid_bundle_ids: Set[str] = set(bundle.provided_evidence_ids)

        clean_facts: List[CitedFact] = []
        for fact in summary.confirmed_facts:
            # Check if all cited IDs exist
            if fact.evidence_ids and all(eid in valid_bundle_ids for eid in fact.evidence_ids):
                clean_facts.append(fact)

        clean_inferences: List[CitedInference] = []
        for inf in summary.inferred_explanations:
            if inf.evidence_ids and all(eid in valid_bundle_ids for eid in inf.evidence_ids):
                clean_inferences.append(inf)

        summary.confirmed_facts = clean_facts
        summary.inferred_explanations = clean_inferences
        summary.cited_evidence_ids = sorted(list({
            eid for f in clean_facts for eid in f.evidence_ids
        } | {
            eid for inf in clean_inferences for eid in inf.evidence_ids
        }))
        return summary


citation_validator = CitationValidator()
