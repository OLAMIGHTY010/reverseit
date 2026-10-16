"""REST API endpoints for Root-Cause Hypotheses and Missing Information (Prompt A3)."""

from typing import Dict, List, Any
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field

from app.models.taxonomy import RootCauseAnalysisResponse
from app.models.context import CallerContext
from app.services.context_builder import SafeContextBuilder
from app.services.root_cause_engine import root_cause_engine

router = APIRouter(prefix="/root-cause", tags=["Root Cause & Missing Information"])


class RootCauseAnalysisRequest(BaseModel):
    case_data: Dict[str, Any]
    raw_evidence_items: List[Dict[str, Any]] = Field(default_factory=list)
    caller: CallerContext


@router.post("/analyze", response_model=RootCauseAnalysisResponse)
def analyze_root_cause(request: RootCauseAnalysisRequest):
    """Generates ranked root-cause hypotheses and identifies missing 8-check gate evidence."""
    try:
        bundle = SafeContextBuilder.build_bundle(
            case_data=request.case_data,
            raw_evidence_items=request.raw_evidence_items,
            caller=request.caller
        )
    except PermissionError as e:
        raise HTTPException(status_code=403, detail=str(e))

    return root_cause_engine.analyze(bundle)
