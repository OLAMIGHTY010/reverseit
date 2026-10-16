"""Pydantic schemas for AI Evaluation Harness, Model Cards, and Monitoring (Prompt A8)."""

from typing import Dict, List, Optional, Any
from pydantic import BaseModel, Field
import datetime


class MetricThresholdGate(BaseModel):
    metric_name: str
    target_value: float
    actual_value: float
    comparator: str  # "GE" (greater equal), "LE" (less equal), "EQ" (exact)
    passed: bool


class ModelCard(BaseModel):
    """Formal Model Card for Risk and Compliance Audits (Prompt A8 Item 5)."""
    model_config = {"protected_namespaces": ()}

    model_id: str
    model_name: str
    version: str
    capability: str
    intended_use: str
    training_data_summary: str
    is_synthetic: bool = True
    evaluation_summary: Dict[str, float] = Field(default_factory=dict)
    known_limitations: List[str] = Field(default_factory=list)
    risk_approval_status: str = "APPROVED_SHADOW_MODE"
    approved_by: str = "CHIEF_RISK_OFFICER"
    effective_from: str = "2026-10-15"


class EvaluationSuiteReport(BaseModel):
    """Comprehensive report output by the CI/CD AI Evaluation Harness."""
    model_config = {"protected_namespaces": ()}

    suite_id: str
    evaluated_at: datetime.datetime = Field(default_factory=lambda: datetime.datetime.now(datetime.timezone.utc))
    environment: str = "CI_AUTOMATED"
    dataset_evaluated: str = "DS-SYN-01 & DS-SYN-02 (Synthetic Benchmark Suite)"
    total_test_samples: int
    faithfulness_rate: float
    citation_validity_rate: float
    hallucination_rate: float
    dispute_classification_accuracy: float
    all_regression_gates_passed: bool
    gates: List[MetricThresholdGate] = Field(default_factory=list)
    active_model_cards: List[ModelCard] = Field(default_factory=list)
