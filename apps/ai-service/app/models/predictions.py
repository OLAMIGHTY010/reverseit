"""Pydantic schemas for Decision-Support Predictive Models (Prompt A5).

Implements Prompt A5 and Scope v2:
- Priority ranking predicting customer harm & urgency
- GBDT Duplicate likelihood scoring on ambiguous retry patterns
- Pending transaction resolution probability model
- Explanations with feature contributions and baseline comparisons
- Mandatory 'SYNTHETIC' dataset disclosure
"""

from typing import Dict, List, Optional, Any
from pydantic import BaseModel, Field


class PriorityScoringRequest(BaseModel):
    amount_minor: int
    sla_minutes_remaining: float
    age_since_debit_minutes: float
    repeat_complaint_count: int = 0
    is_customer_reported: bool = False
    incident_membership: bool = False


class PriorityScoreResult(BaseModel):
    model_config = {"protected_namespaces": ()}

    priority_score: float = Field(..., ge=0.0, le=100.0, description="Normalized score 0-100")
    urgency_tier: str = "MEDIUM"  # "CRITICAL", "HIGH", "MEDIUM", "LOW"
    rule_baseline_score: float
    component_breakdown: Dict[str, float] = Field(default_factory=dict)
    is_synthetic_model: bool = True


class DuplicateRiskRequest(BaseModel):
    time_diff_seconds: float
    is_exact_amount: bool = True
    is_same_account: bool = True
    is_same_beneficiary: bool = True
    reference_similarity: float = Field(default=0.0, ge=0.0, le=1.0)
    session_id_match: bool = False
    is_retry_pattern: bool = False


class DuplicateRiskResult(BaseModel):
    model_config = {"protected_namespaces": ()}

    duplicate_risk_score: float = Field(..., ge=0.0, le=1.0)
    risk_grade: str = "LOW"  # "HIGH", "MEDIUM", "LOW"
    rule_candidate_verdict: str  # "CANDIDATE", "NON_CANDIDATE"
    feature_contributions: Dict[str, float] = Field(default_factory=dict)
    advisory_recommendation: str
    is_synthetic_model: bool = True


class PendingResolutionRequest(BaseModel):
    elapsed_minutes: float
    rail: str = "NIP"
    channel: str = "MOBILE"
    amount_minor: int = 100000


class PendingResolutionResult(BaseModel):
    model_config = {"protected_namespaces": ()}

    resolution_probability: float = Field(..., ge=0.0, le=1.0, description="Probability transaction resolves to success without intervention")
    hazard_rate: float
    recommended_re_query_interval_seconds: int
    calibration_status: str = "CALIBRATED_SYNTHETIC"
    advisory_guidance: str
