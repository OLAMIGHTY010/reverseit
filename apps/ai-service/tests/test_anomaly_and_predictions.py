"""Unit and integration tests for Anomaly Detection, Incident Grouping, and Decision-Support Predictions (Prompts A4 & A5).

Tests:
- Statistical spike detection and Isolation Forest multi-feature anomaly detection
- Maintenance window and degraded-source alert suppression (Case M26 / Prompt A4)
- Incident clustering: grouping individual exceptions into single macro-incidents
- Priority ranker evaluating customer harm & SLA countdown
- GBDT duplicate likelihood scoring vs rule baseline
- Pending transaction resolution probability model
- REST API endpoints
"""

import pytest
from fastapi.testclient import TestClient

from app.main import app
from app.services.anomaly_service import anomaly_service
from app.services.predictive_models import (
    priority_engine,
    duplicate_scorer,
    pending_hazard_estimator,
)
from app.models.anomaly import (
    MetricDataPoint,
    AnomalyDetectionRequest,
    IncidentGroupingRequest,
)
from app.models.predictions import (
    PriorityScoringRequest,
    DuplicateRiskRequest,
    PendingResolutionRequest,
)

client = TestClient(app)


# ---------------------------------------------------------------------------
# Prompt A4: Anomaly Detection and Incident Grouping Tests
# ---------------------------------------------------------------------------

def test_normal_telemetry_no_anomaly():
    """Normal failure rate (1.5%) does not trigger anomaly."""
    points = [
        MetricDataPoint(
            timestamp="2026-10-15T10:00:00Z",
            rail="NIP",
            channel="MOBILE",
            total_volume=1000,
            failure_count=15,
            timeout_count=5,
            avg_latency_ms=220.0,
            total_amount_minor=50000000
        )
    ]
    req = AnomalyDetectionRequest(data_points=points)
    res = anomaly_service.detect_anomaly(req)

    assert res.is_anomaly is False
    assert res.severity == "INFO"
    assert res.current_failure_rate == 0.015


def test_failure_spike_triggers_anomaly():
    """Sudden surge in switch timeouts and failures triggers anomaly."""
    spike_points = [
        MetricDataPoint(
            timestamp="2026-10-15T10:05:00Z",
            rail="NIP",
            channel="MOBILE",
            total_volume=500,
            failure_count=120,   # 24% failure rate
            timeout_count=100,
            avg_latency_ms=4500.0,
            total_amount_minor=25000000
        )
    ]
    req = AnomalyDetectionRequest(data_points=spike_points)
    res = anomaly_service.detect_anomaly(req)

    assert res.is_anomaly is True
    assert res.severity in ["HIGH", "CRITICAL"]
    assert res.z_score >= 3.0


def test_noise_control_maintenance_suppression():
    """Alerts are suppressed during scheduled maintenance to prevent alert storms."""
    spike_points = [
        MetricDataPoint(
            timestamp="2026-10-15T02:00:00Z",
            rail="NIP",
            channel="MOBILE",
            total_volume=100,
            failure_count=80,
            timeout_count=70,
            avg_latency_ms=5000.0,
            total_amount_minor=1000000
        )
    ]
    req = AnomalyDetectionRequest(
        data_points=spike_points,
        active_maintenance_window=True
    )
    res = anomaly_service.detect_anomaly(req)

    assert res.is_anomaly is False
    assert "suppressed" in res.suppressed_reason.lower()


def test_incident_grouping_clusters_exceptions():
    """Clusters 20 discrete switch timeout exceptions into 1 single incident."""
    exceptions = [
        {
            "reference_number": f"NIP-SW-SPIKE-{i}",
            "rail": "NIP",
            "channel": "MOBILE",
            "switch_status": "TIMEOUT",
            "amount_minor": 100000,
            "timestamp": "2026-10-15T10:10:00Z"
        }
        for i in range(20)
    ]
    req = IncidentGroupingRequest(exceptions=exceptions, time_window_minutes=15)
    res = anomaly_service.group_incidents(req)

    assert len(res.incidents) == 1
    inc = res.incidents[0]
    assert inc.affected_count == 20
    assert inc.error_signature == "TIMEOUT"
    assert inc.total_exposure_minor == 2000000
    assert "NGN 20,000.00" in inc.total_exposure_formatted
    assert "Incident INC-" in inc.narrative_summary
    assert inc.causation_status == "CONFIRMED"


# ---------------------------------------------------------------------------
# Prompt A5: Predictive Models Tests
# ---------------------------------------------------------------------------

def test_priority_ranking_scoring():
    """High amount with near-breach SLA and repeat complaints yields CRITICAL priority."""
    req = PriorityScoringRequest(
        amount_minor=50000000,          # NGN 500,000
        sla_minutes_remaining=15.0,     # Urgently close to regulatory deadline
        age_since_debit_minutes=120.0,
        repeat_complaint_count=3,
        is_customer_reported=True
    )
    res = priority_engine.score_priority(req)

    assert res.priority_score >= 70.0
    assert res.urgency_tier in ["HIGH", "CRITICAL"]
    assert res.is_synthetic_model is True


def test_gbdt_duplicate_likelihood_high_risk():
    """Identical retry within 5 seconds with session match yields HIGH duplicate risk."""
    req = DuplicateRiskRequest(
        time_diff_seconds=4.5,
        is_exact_amount=True,
        is_same_account=True,
        is_same_beneficiary=True,
        reference_similarity=0.95,
        session_id_match=True,
        is_retry_pattern=True
    )
    res = duplicate_scorer.predict_risk(req)

    assert res.risk_grade == "HIGH"
    assert res.duplicate_risk_score >= 0.70
    assert res.rule_candidate_verdict == "CANDIDATE"


def test_gbdt_duplicate_likelihood_low_risk_legitimate_repeat():
    """Repeat payment to different beneficiary after 2 hours yields LOW risk."""
    req = DuplicateRiskRequest(
        time_diff_seconds=7200.0,
        is_exact_amount=True,
        is_same_account=True,
        is_same_beneficiary=False,
        reference_similarity=0.1,
        session_id_match=False,
        is_retry_pattern=False
    )
    res = duplicate_scorer.predict_risk(req)

    assert res.risk_grade == "LOW"
    assert res.duplicate_risk_score <= 0.35
    assert res.rule_candidate_verdict == "NON_CANDIDATE"


def test_pending_resolution_hazard_and_re_query_ladder():
    """Decaying resolution probability guides the re-query ladder intervals."""
    # 1. Immediate transaction (30 seconds elapsed)
    req_fresh = PendingResolutionRequest(elapsed_minutes=0.5, rail="NIP")
    res_fresh = pending_hazard_estimator.estimate_resolution(req_fresh)
    assert res_fresh.resolution_probability >= 0.80
    assert res_fresh.recommended_re_query_interval_seconds == 60

    # 2. Aged transaction (25 minutes elapsed)
    req_aged = PendingResolutionRequest(elapsed_minutes=25.0, rail="NIP")
    res_aged = pending_hazard_estimator.estimate_resolution(req_aged)
    assert res_aged.resolution_probability <= 0.10
    assert "route to Lane 3" in res_aged.advisory_guidance


# ---------------------------------------------------------------------------
# API Endpoints Integration Tests
# ---------------------------------------------------------------------------

def test_api_anomaly_and_predictions_endpoints():
    """Test REST API endpoints for anomaly and predictions."""
    # Anomaly Detect
    resp_anomaly = client.post(
        "/v1/ai/anomaly/detect",
        json={"data_points": [], "active_maintenance_window": False}
    )
    assert resp_anomaly.status_code == 200

    # Priority Predict
    resp_prio = client.post(
        "/v1/ai/predictions/priority",
        json={
            "amount_minor": 1000000,
            "sla_minutes_remaining": 60.0,
            "age_since_debit_minutes": 30.0,
            "repeat_complaint_count": 0
        }
    )
    assert resp_prio.status_code == 200
    assert "priority_score" in resp_prio.json()

    # Duplicate Risk Predict
    resp_dup = client.post(
        "/v1/ai/predictions/duplicate-risk",
        json={
            "time_diff_seconds": 10.0,
            "is_exact_amount": True,
            "is_same_account": True,
            "is_same_beneficiary": True,
            "reference_similarity": 0.9
        }
    )
    assert resp_dup.status_code == 200
    assert "duplicate_risk_score" in resp_dup.json()

    # Pending Resolution Predict
    resp_pend = client.post(
        "/v1/ai/predictions/pending-resolution",
        json={"elapsed_minutes": 2.0, "rail": "NIP"}
    )
    assert resp_pend.status_code == 200
    assert "resolution_probability" in resp_pend.json()
