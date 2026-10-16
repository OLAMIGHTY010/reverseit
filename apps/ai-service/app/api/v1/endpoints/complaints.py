"""REST API endpoints for Complaint Matching and Safe Wording (Prompt A6)."""

from typing import List, Dict, Any, Optional
from fastapi import APIRouter
from pydantic import BaseModel, Field

from app.models.complaint_matching import (
    ComplaintMatchResult,
    CustomerSafeWordingRequest,
    CustomerSafeWordingResponse,
)
from app.services.complaint_matcher import complaint_matcher
from app.services.safe_wording import safe_wording_service

router = APIRouter(prefix="/complaints", tags=["Complaint Matching & Wording"])


class MatchComplaintRequest(BaseModel):
    complaint_ref: str
    claimed_ref: Optional[str] = None
    claimed_amount_minor: Optional[int] = None
    claimed_account_tail: Optional[str] = None
    claimed_channel: Optional[str] = None
    candidate_pool: List[Dict[str, Any]] = Field(default_factory=list)


@router.post("/match", response_model=ComplaintMatchResult)
def match_complaint_to_transactions(request: MatchComplaintRequest):
    """Matches free-text or structured customer complaints against candidate transactions."""
    return complaint_matcher.match(
        complaint_ref=request.complaint_ref,
        claimed_ref=request.claimed_ref,
        claimed_amount_minor=request.claimed_amount_minor,
        claimed_account_tail=request.claimed_account_tail,
        claimed_channel=request.claimed_channel,
        candidate_pool=request.candidate_pool
    )


@router.post("/safe-wording", response_model=CustomerSafeWordingResponse)
def generate_customer_safe_wording(request: CustomerSafeWordingRequest):
    """Generates approved status notifications and contact-center wording in English or Pidgin."""
    return safe_wording_service.generate_wording(request)
