"""REST API endpoints for AI Evaluation Harness and Model Governance (Prompt A8)."""

from typing import List
from fastapi import APIRouter
from app.models.evaluation import EvaluationSuiteReport, ModelCard
from app.services.evaluation_harness import evaluation_harness

router = APIRouter(prefix="/evaluation", tags=["AI Quality & Evaluation Harness"])


@router.post("/run-ci-harness", response_model=EvaluationSuiteReport)
def run_ci_evaluation_harness():
    """Executes the offline regression test suite against synthetic benchmarks."""
    return evaluation_harness.run_ci_evaluation()


@router.get("/model-cards", response_model=List[ModelCard])
def get_certified_model_cards():
    """Returns certified model cards and governance records for compliance audits."""
    return evaluation_harness.get_active_model_cards()
