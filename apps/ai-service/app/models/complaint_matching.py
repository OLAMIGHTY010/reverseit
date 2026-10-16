"""Pydantic schemas for Complaint-to-Transaction Matching and Customer-Safe Wording.

Implements Prompt A6 requirements:
- Ranked candidate matches with reasons and confidence scores
- Threshold routing (auto-link vs human disambiguation vs unmatched)
- Customer-safe wording schemas across languages (English, Nigerian Pidgin)
- Strict vocabulary compliance
"""

from enum import Enum
from typing import Dict, List, Optional, Any
from pydantic import BaseModel, Field
import datetime


class CandidateMatch(BaseModel):
    """A potential transaction matching a customer complaint."""
    transaction_id: str
    reference_number: str
    amount_minor: int
    amount_formatted: str
    channel: str
    rail: str
    initiated_at: str
    account_tail: str
    score: float = Field(..., ge=0.0, le=1.0, description="Match confidence score")
    match_reasons: List[str] = Field(default_factory=list)
    is_exact_match: bool = False


class ComplaintMatchResult(BaseModel):
    """Outcome of running the multi-stage complaint matcher."""
    complaint_ref: str
    candidates: List[CandidateMatch] = Field(default_factory=list)
    top_candidate: Optional[CandidateMatch] = None
    auto_linked: bool = False
    route_to_human_disambiguation: bool = False
    is_unmatched_claim: bool = False
    matching_stage: str = "none"  # "exact_reference", "constrained_fuzzy", "none"


class CustomerSafeWordingRequest(BaseModel):
    """Request for generating approved customer-facing communication."""
    case_id: str
    state: str  # "DETECTED", "UNDER_INVESTIGATION", "AWAITING_APPROVAL", "REMEDIATION_SUBMITTED", "OUTCOME_VERIFIED", "CUSTOMER_ERROR"
    financial_verified: bool = False
    amount_formatted: str = "NGN 0.00"
    channel: str = "TRANSFER"
    language: str = Field(default="en", description="'en' for English, 'pcm' for Nigerian Pidgin")
    customer_name: Optional[str] = None


class CustomerSafeWordingResponse(BaseModel):
    """Verified customer-safe wording adhering to bank approved vocabularies."""
    case_id: str
    state: str
    language: str
    message: str
    financial_verified: bool
    is_compliant: bool = True
    prohibited_terms_detected: List[str] = Field(default_factory=list)
