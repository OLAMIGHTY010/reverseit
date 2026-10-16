"""REST API endpoints for Decision-Support Predictive Models (Prompt A5)."""

from fastapi import APIRouter
from app.models.predictions import (
    PriorityScoringRequest,
    PriorityScoreResult,
    DuplicateRiskRequest,
    DuplicateRiskResult,
    PendingResolutionRequest,
    PendingResolutionResult,
)
from app.services.predictive_models import (
    priority_engine,
    duplicate_scorer,
    pending_hazard_estimator,
)

router = APIRouter(prefix="/predictions", tags=["Decision-Support Predictions"])


@router.post("/priority", response_model=PriorityScoreResult)
def predict_priority_score(request: PriorityScoringRequest):
    """Predicts customer harm and urgency score (0-100) to order operations queues."""
    return priority_engine.score_priority(request)


@router.post("/duplicate-risk", response_model=DuplicateRiskResult)
def predict_duplicate_risk(request: DuplicateRiskRequest):
    """Evaluates duplicate debit likelihood on borderline retry transactions using GBDT."""
    return duplicate_scorer.predict_risk(request)


@router.post("/pending-resolution", response_model=PendingResolutionResult)
def predict_pending_resolution(request: PendingResolutionRequest):
    """Estimates self-healing probability and guides the status query ladder."""
    return pending_hazard_estimator.estimate_resolution(request)
