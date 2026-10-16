"""REST API endpoints for ReverseIt Customer-Side AI Services (Prompt A9)."""

from typing import Optional
from fastapi import APIRouter
from pydantic import BaseModel, Field

from app.models.reverseit import (
    AlertParseResult,
    DisputeClassificationResult,
    ComplaintDraftRequest,
    ComplaintDraftResponse,
    AuthenticitySignalResult,
)
from app.services.regex_parser import regex_parser
from app.services.intent_classifier import intent_classifier
from app.services.complaint_generator import complaint_engine
from app.services.authenticity_detector import authenticity_detector

router = APIRouter(prefix="/reverseit", tags=["ReverseIt Customer AI"])


class ParseAlertRequest(BaseModel):
    alert_text: str = Field(..., description="Raw SMS or Email debit alert text pasted by customer")


class ClassifyDisputeRequest(BaseModel):
    customer_description: str = Field(..., description="Customer's explanation of what went wrong")
    channel: Optional[str] = Field(default=None, description="Inferred or selected channel (POS, ATM, NIP, etc.)")
    narration: Optional[str] = Field(default=None, description="Transaction narration from alert")


class AuthenticityCheckRequest(BaseModel):
    parsed_amount_minor: Optional[int] = None
    reference: Optional[str] = None
    prior_reports_count: int = 0


@router.post("/parse-alert", response_model=AlertParseResult)
def parse_customer_alert(request: ParseAlertRequest):
    """Parses customer debit alerts using local regexes with PII redaction and issuing bank detection."""
    return regex_parser.parse(request.alert_text)


@router.post("/classify-dispute", response_model=DisputeClassificationResult)
def classify_dispute(request: ClassifyDisputeRequest):
    """Classifies dispute type and binds to the deterministic regulatory clock registry."""
    return intent_classifier.classify(
        customer_description=request.customer_description,
        channel=request.channel,
        narration=request.narration
    )


@router.post("/draft-complaint", response_model=ComplaintDraftResponse)
def draft_dispute_complaint(request: ComplaintDraftRequest):
    """Generates a neutral, respectful dispute complaint with fact verification."""
    return complaint_engine.generate_draft(request)


@router.post("/authenticity-check", response_model=AuthenticitySignalResult)
def check_alert_authenticity(request: AuthenticityCheckRequest):
    """Evaluates advisory tamper indicators and impossible timestamps."""
    return authenticity_detector.check_authenticity(
        parsed_amount_minor=request.parsed_amount_minor,
        parsed_datetime=None,
        reference=request.reference,
        prior_reports_count=request.prior_reports_count
    )
