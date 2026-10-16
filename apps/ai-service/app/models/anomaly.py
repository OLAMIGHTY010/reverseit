"""Pydantic schemas for Anomaly Detection, Spike Analysis, and Incident Grouping.

Implements Prompt A4 requirements:
- Rolling statistical baselines and Isolation Forest anomaly outputs
- Incident clustering schemas grouping individual exceptions
- Causation status tracking and noise control (suppression)
- Grounded incident narrative schemas
"""

from typing import Dict, List, Optional, Any
from pydantic import BaseModel, Field
import datetime


class MetricDataPoint(BaseModel):
    timestamp: str
    rail: str = "NIP"
    channel: str = "MOBILE"
    total_volume: int
    failure_count: int
    timeout_count: int
    avg_latency_ms: float
    total_amount_minor: int


class AnomalyDetectionRequest(BaseModel):
    data_points: List[MetricDataPoint] = Field(default_factory=list)
    active_maintenance_window: bool = False
    is_source_degraded: bool = False


class AnomalyDetectionResult(BaseModel):
    model_config = {"protected_namespaces": ()}

    is_anomaly: bool
    anomaly_score: float = Field(..., ge=0.0, le=1.0)
    severity: str = "INFO"  # "INFO", "WARNING", "HIGH", "CRITICAL"
    detection_method: str   # "EWMA_STATISTICAL", "ISOLATION_FOREST", "HYBRID"
    current_failure_rate: float
    expected_failure_rate: float
    z_score: float
    features_contributing: Dict[str, float] = Field(default_factory=dict)
    suppressed_reason: Optional[str] = None


class IncidentCluster(BaseModel):
    model_config = {"protected_namespaces": ()}

    incident_id: str
    title: str
    scope: str
    rail: str
    channel: str
    error_signature: str
    affected_count: int
    total_exposure_minor: int
    total_exposure_formatted: str
    start_time: str
    last_seen_time: str
    suspected_cause: str
    causation_status: str  # "CORRELATED", "PROBABLE", "CONFIRMED"
    narrative_summary: str
    is_suppressed: bool = False
    suppression_reason: Optional[str] = None
    affected_transaction_refs: List[str] = Field(default_factory=list)


class IncidentGroupingRequest(BaseModel):
    exceptions: List[Dict[str, Any]] = Field(default_factory=list)
    time_window_minutes: int = 15
    active_maintenance_window: bool = False
    degraded_sources: List[str] = Field(default_factory=list)


class IncidentGroupingResponse(BaseModel):
    model_config = {"protected_namespaces": ()}

    incidents: List[IncidentCluster] = Field(default_factory=list)
    total_grouped_exceptions: int = 0
    ungrouped_count: int = 0
