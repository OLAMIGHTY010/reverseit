"""Pydantic schemas for Investigation Summaries, Explanations, and Citations.

Implements Prompt A2 requirements:
- Validated JSON output schema
- Mandatory evidence_id citations for confirmed facts and inferred explanations
- Dual wording: technical explainability for analysts and agent-safe wording for contact center
- Scope v2 addition: Ops reviewer summary with citations
- Editable draft investigation notes and incident report templates
"""

from enum import Enum
from typing import List, Optional, Dict, Any
from pydantic import BaseModel, Field
import datetime


class ModelConfidence(str, Enum):
    LOW = "LOW"
    MEDIUM = "MEDIUM"
    HIGH = "HIGH"


class CitedFact(BaseModel):
    """A factual statement strictly grounded in verified evidence with stable IDs."""
    text: str = Field(..., description="Factual statement derived from system of record")
    evidence_ids: List[str] = Field(..., min_length=1, description="One or more valid evidence IDs confirming the fact")


class CitedInference(BaseModel):
    """An analytical inference or hypothesis explaining what likely happened."""
    model_config = {"protected_namespaces": ()}
    text: str = Field(..., description="Analytical inference statement")
    evidence_ids: List[str] = Field(..., min_length=1, description="Supporting evidence IDs")
    basis: str = Field(..., description="Logical rationale linking evidence to inference")
    model_confidence: ModelConfidence = Field(default=ModelConfidence.MEDIUM)


class NextCheckItem(BaseModel):
    """Recommended investigation action and the evidence it is expected to produce."""
    action: str = Field(..., description="Recommended check or verification step")
    target_system: str = Field(..., description="System to check (e.g., SWITCH_TSQ, CORE_LEDGER, ATM_EJ)")
    expected_evidence: str = Field(..., description="Evidence type expected from this check")


class InvestigationSummaryResponse(BaseModel):
    """Canonical investigation summary and explanation payload."""
    model_config = {"protected_namespaces": ()}
    case_id: str
    headline: str

    confirmed_facts: List[CitedFact] = Field(default_factory=list)
    inferred_explanations: List[CitedInference] = Field(default_factory=list)
    missing_evidence: List[str] = Field(default_factory=list)
    unresolved_questions: List[str] = Field(default_factory=list)
    recommended_next_checks: List[NextCheckItem] = Field(default_factory=list)
    customer_impact_note: str = ""
    uncertainty_statement: str = ""
    
    # Scope v2 mandatory addition: Concise summary for operations reviewer
    ops_reviewer_summary: str = Field(..., description="Executive summary for ops reviewer with citation tags")
    
    # Contact-centre agent-safe wording (Prompt A2 item 3 & Case M4)
    agent_safe_wording: str = Field(..., description="Approved wording for customer-facing agents; never states funds returned unless verified")
    
    # Editable investigation note draft owned by analyst
    draft_investigation_note: str = ""
    
    # Audit and telemetry metadata
    is_fallback: bool = False
    model_version: Optional[str] = None
    prompt_version: Optional[str] = None
    generated_at: datetime.datetime = Field(default_factory=lambda: datetime.datetime.now(datetime.timezone.utc))
    cited_evidence_ids: List[str] = Field(default_factory=list)


class CitationValidationResult(BaseModel):
    """Result of citation validation against the canonical evidence bundle."""
    is_valid: bool
    verified_evidence_ids: List[str]
    fabricated_evidence_ids: List[str]
    uncited_statements: List[str]
    error_message: Optional[str] = None
