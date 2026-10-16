"""REST endpoints for AI Case Summaries, Explanations, and Citation Verification."""

from typing import List, Dict, Any, Optional
from fastapi import APIRouter, HTTPException, Depends
from pydantic import BaseModel, Field

from app.models.schemas import InvestigationSummaryResponse, CitationValidationResult
from app.models.context import CallerContext, CanonicalEvidenceBundle
from app.services.context_builder import SafeContextBuilder
from app.services.summary_generator import summary_generator
from app.services.citation_validator import citation_validator

router = APIRouter(prefix="/summary", tags=["Summaries & Explanations"])


class SummaryGenerationRequest(BaseModel):
    case_data: Dict[str, Any] = Field(..., description="Case metadata, amounts, channel, rail, status")
    raw_evidence_items: List[Dict[str, Any]] = Field(default_factory=list, description="Associated evidence records")
    caller: CallerContext
    financial_verified: bool = Field(default=False, description="Whether core ledger confirms reversal posting")
    force_fallback: bool = Field(default=False, description="Force deterministic template output")


class CitationValidationRequest(BaseModel):
    summary: InvestigationSummaryResponse
    bundle: CanonicalEvidenceBundle


@router.post("", response_model=InvestigationSummaryResponse)
def generate_investigation_summary(request: SummaryGenerationRequest):
    """Generates a complete investigation summary with verified citations and dual wording."""
    try:
        bundle = SafeContextBuilder.build_bundle(
            case_data=request.case_data,
            raw_evidence_items=request.raw_evidence_items,
            caller=request.caller
        )
    except PermissionError as e:
        raise HTTPException(status_code=403, detail=str(e))

    summary = summary_generator.generate_summary(
        bundle=bundle,
        financial_verified=request.financial_verified,
        force_fallback=request.force_fallback
    )
    return summary


@router.post("/validate-citations", response_model=CitationValidationResult)
def validate_citations(request: CitationValidationRequest):
    """Verifies that all statement citations exist within the provided evidence bundle."""
    return citation_validator.validate_summary(request.summary, request.bundle)
