"""Pydantic schemas for AI Model Registry, Prompt Versioning, and Governance Audits.

Implements requirements from Prompt A0 and Section 7.1 technical contract:
- Model metadata and version tracking
- Prompt template registry with versioning
- Citation schema and verification records
- Evaluation metric records
"""

from typing import Dict, List, Optional, Any
from pydantic import BaseModel, Field
import datetime


class ModelRegistryEntry(BaseModel):
    model_id: str = Field(..., description="Unique model identifier, e.g. 'gpt-4o-mini-2024-07-18' or 'rf-dup-v1.2'")
    name: str = Field(..., description="Human readable model name")
    version: str = Field(..., description="Semver version string")
    provider: str = Field(..., description="Provider: mock, openai, anthropic, local_ollama, etc.")
    capability: str = Field(..., description="Target capability or task")
    is_active: bool = True
    is_shadow_mode: bool = False
    parameters: Dict[str, Any] = Field(default_factory=dict)
    trained_on_dataset_version: Optional[str] = None
    baseline_comparator: Optional[str] = None
    deployed_at: datetime.datetime = Field(default_factory=lambda: datetime.datetime.now(datetime.timezone.utc))


class PromptRegistryEntry(BaseModel):
    prompt_id: str = Field(..., description="Prompt identifier, e.g. 'case_summary_analyst_v1'")
    capability: str = Field(..., description="Target capability")
    version: str = Field(..., description="Prompt revision e.g. '1.0.0'")
    template_text: str = Field(..., description="Jinja or f-string template text")
    input_variables: List[str] = Field(default_factory=list)
    output_schema_name: str = Field(..., description="Expected JSON Schema name for structured output")
    approved_by: str = Field(..., description="Risk / Compliance / Lead reviewer who approved prompt")
    approved_at: datetime.datetime = Field(default_factory=lambda: datetime.datetime.now(datetime.timezone.utc))
    is_active: bool = True


class CitationVerificationRecord(BaseModel):
    statement: str
    cited_evidence_ids: List[str]
    is_verified: bool
    missing_evidence_ids: List[str] = Field(default_factory=list)
    rejection_reason: Optional[str] = None


class AIEvaluationMetricRecord(BaseModel):
    evaluation_id: str
    capability: str
    model_version: str
    prompt_version: Optional[str] = None
    dataset_version: str
    is_synthetic_data: bool = True
    metric_name: str  # e.g., "faithfulness", "citation_precision", "pr_auc", "hallucination_rate"
    metric_value: float
    threshold_target: float
    passed_gate: bool
    evaluated_at: datetime.datetime = Field(default_factory=lambda: datetime.datetime.now(datetime.timezone.utc))
