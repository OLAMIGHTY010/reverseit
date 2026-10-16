"""REST API endpoints for AI Copilot Panel and Tool Boundaries (Prompt A7)."""

from fastapi import APIRouter
from pydantic import BaseModel, Field

from app.models.copilot import (
    CopilotChatRequest,
    CopilotChatResponse,
    RuleImprovementProposal,
)
from app.services.copilot_orchestrator import copilot_orchestrator

router = APIRouter(prefix="/copilot", tags=["AI Copilot & Tool Orchestration"])


class RuleSuggestionRequest(BaseModel):
    rule_id: str = "RULE-DUP-30S"
    feedback_summary: str = "High volume of legitimate transfers marked duplicate"


@router.post("/chat", response_model=CopilotChatResponse)
def copilot_chat(request: CopilotChatRequest):
    """Processes interactive analyst inquiries under strict read-only tool boundaries."""
    return copilot_orchestrator.chat(request)


@router.post("/suggest-rule-change", response_model=RuleImprovementProposal)
def suggest_rule_improvement(request: RuleSuggestionRequest):
    """Analyzes false positive feedback and drafts a proposed rule adjustment in shadow mode."""
    return copilot_orchestrator.suggest_rule_improvement(
        rule_id=request.rule_id,
        feedback_summary=request.feedback_summary
    )
