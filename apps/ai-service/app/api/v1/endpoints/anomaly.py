"""REST API endpoints for Anomaly Detection and Incident Grouping (Prompt A4)."""

from fastapi import APIRouter
from app.models.anomaly import (
    AnomalyDetectionRequest,
    AnomalyDetectionResult,
    IncidentGroupingRequest,
    IncidentGroupingResponse,
)
from app.services.anomaly_service import anomaly_service

router = APIRouter(prefix="/anomaly", tags=["Anomaly & Incident Intelligence"])


@router.post("/detect", response_model=AnomalyDetectionResult)
def detect_metric_anomalies(request: AnomalyDetectionRequest):
    """Evaluates telemetry for statistical spikes and Isolation Forest anomalies."""
    return anomaly_service.detect_anomaly(request)


@router.post("/group-incidents", response_model=IncidentGroupingResponse)
def group_exceptions_into_incidents(request: IncidentGroupingRequest):
    """Clusters individual exceptions into candidate incidents to prevent alert storms."""
    return anomaly_service.group_incidents(request)
