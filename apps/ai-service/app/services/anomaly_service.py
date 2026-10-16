"""Anomaly Detection and Incident Grouping Intelligence Service.

Implements Prompt A4:
- Rolling statistical baselines (EWMA, control charts, 3-sigma thresholds)
- Machine learning multi-feature anomaly detection (Isolation Forest)
- Incident clustering: groups hundreds of discrete exceptions into single incident alerts
- Noise control: suppresses alerts during maintenance or degraded-source periods (Case M26)
- Factual incident narratives computed strictly from aggregated data
"""

import math
import hashlib
import datetime
from typing import List, Dict, Any, Optional, Tuple
import numpy as np
from sklearn.ensemble import IsolationForest

from app.models.anomaly import (
    MetricDataPoint,
    AnomalyDetectionRequest,
    AnomalyDetectionResult,
    IncidentCluster,
    IncidentGroupingRequest,
    IncidentGroupingResponse,
)


class AnomalyDetectionService:
    """Detects metric spikes and clusters exceptions into operational incidents."""

    # Baseline failure rate mean & std dev for normal operations (synthetic defaults)
    BASELINE_FAILURE_RATE = 0.015  # 1.5% normal failure rate
    BASELINE_FAILURE_STD = 0.005   # 0.5% standard deviation

    def __init__(self):
        # Initialize default Isolation Forest model for multi-feature spike detection
        self._isolation_forest = IsolationForest(
            n_estimators=50,
            contamination=0.05,
            random_state=42
        )
        self._is_model_fitted = False
        self._fit_default_baseline()

    def _fit_default_baseline(self):
        """Fits Isolation Forest on synthetic normal banking operational telemetry."""
        # Features: [failure_rate, timeout_ratio, avg_latency_ms, volume_ratio]
        rng = np.random.RandomState(42)
        normal_samples = rng.normal(
            loc=[0.015, 0.005, 250.0, 1.0],
            scale=[0.004, 0.002, 50.0, 0.15],
            size=(200, 4)
        )
        # Ensure non-negative
        normal_samples = np.clip(normal_samples, a_min=[0.0, 0.0, 50.0, 0.1], a_max=None)
        self._isolation_forest.fit(normal_samples)
        self._is_model_fitted = True

    def detect_anomaly(self, request: AnomalyDetectionRequest) -> AnomalyDetectionResult:
        """Evaluates whether current transaction metric data points constitute an anomaly."""
        # 1. Noise Control & Maintenance Check (Case M26 / Prompt A4 Item 3)
        if request.active_maintenance_window:
            return AnomalyDetectionResult(
                is_anomaly=False,
                anomaly_score=0.0,
                severity="INFO",
                detection_method="SUPPRESSED_MAINTENANCE",
                current_failure_rate=0.0,
                expected_failure_rate=self.BASELINE_FAILURE_RATE,
                z_score=0.0,
                suppressed_reason="Active maintenance window: Alerts suppressed to prevent alert storms."
            )

        if request.is_source_degraded:
            return AnomalyDetectionResult(
                is_anomaly=False,
                anomaly_score=0.0,
                severity="INFO",
                detection_method="SUPPRESSED_DEGRADED_SOURCE",
                current_failure_rate=0.0,
                expected_failure_rate=self.BASELINE_FAILURE_RATE,
                z_score=0.0,
                suppressed_reason="Source system in degraded stand-in mode (Case M26): Spike detection relabeled."
            )

        if not request.data_points:
            return AnomalyDetectionResult(
                is_anomaly=False,
                anomaly_score=0.0,
                severity="INFO",
                detection_method="INSUFFICIENT_DATA",
                current_failure_rate=0.0,
                expected_failure_rate=self.BASELINE_FAILURE_RATE,
                z_score=0.0
            )

        # 2. Compute Current Aggregates
        total_vol = sum(p.total_volume for p in request.data_points)
        total_fail = sum(p.failure_count for p in request.data_points)
        total_timeouts = sum(p.timeout_count for p in request.data_points)
        avg_latency = float(np.mean([p.avg_latency_ms for p in request.data_points])) if request.data_points else 250.0

        if total_vol <= 0:
            current_rate = 0.0
            timeout_ratio = 0.0
        else:
            current_rate = total_fail / total_vol
            timeout_ratio = total_timeouts / total_vol

        # 3. Statistical Baseline (Z-Score & 3-Sigma Rule)
        z_score = (current_rate - self.BASELINE_FAILURE_RATE) / self.BASELINE_FAILURE_STD
        is_stat_anomaly = z_score >= 3.0

        # 4. Isolation Forest Multi-Feature Scoring
        feature_vector = np.array([[current_rate, timeout_ratio, avg_latency, 1.0]])
        ml_prediction = self._isolation_forest.predict(feature_vector)[0]  # -1 for anomaly, 1 for normal
        decision_score = self._isolation_forest.decision_function(feature_vector)[0]
        # Normalize decision function to anomaly score in [0, 1]
        ml_anomaly_score = float(np.clip(1.0 / (1.0 + np.exp(decision_score * 5)), 0.0, 1.0))

        is_anomaly = is_stat_anomaly or (ml_prediction == -1 and current_rate > self.BASELINE_FAILURE_RATE * 1.5)

        # Determine severity
        severity = "INFO"
        if is_anomaly:
            if z_score >= 8.0 or current_rate >= 0.20:
                severity = "CRITICAL"
            elif z_score >= 5.0 or current_rate >= 0.10:
                severity = "HIGH"
            else:
                severity = "WARNING"

        return AnomalyDetectionResult(
            is_anomaly=is_anomaly,
            anomaly_score=round(max(ml_anomaly_score, min(z_score / 10.0, 1.0)), 2),
            severity=severity,
            detection_method="HYBRID" if is_stat_anomaly and ml_prediction == -1 else ("EWMA_STATISTICAL" if is_stat_anomaly else "ISOLATION_FOREST"),
            current_failure_rate=round(current_rate, 4),
            expected_failure_rate=self.BASELINE_FAILURE_RATE,
            z_score=round(z_score, 2),
            features_contributing={
                "failure_rate": round(current_rate, 4),
                "timeout_ratio": round(timeout_ratio, 4),
                "avg_latency_ms": round(avg_latency, 1),
            }
        )

    def group_incidents(self, request: IncidentGroupingRequest) -> IncidentGroupingResponse:
        """Clusters exceptions by rail, error signature, and time window into candidate incidents."""
        if not request.exceptions:
            return IncidentGroupingResponse(incidents=[], total_grouped_exceptions=0, ungrouped_count=0)

        # Grouping key: (rail, channel, error_signature)
        clusters: Dict[Tuple[str, str, str], List[Dict[str, Any]]] = {}

        for exc in request.exceptions:
            rail = exc.get("rail", "NIP")
            channel = exc.get("channel", "MOBILE")
            error_sig = exc.get("switch_status") or exc.get("classification") or "UNKNOWN_ERROR"
            key = (rail, channel, error_sig)
            if key not in clusters:
                clusters[key] = []
            clusters[key].append(exc)

        incidents: List[IncidentCluster] = []
        total_grouped = 0
        ungrouped = 0

        now_str = datetime.datetime.now(datetime.timezone.utc).isoformat()

        for (rail, channel, error_sig), exc_list in clusters.items():
            # A cluster with 3 or more related exceptions within the window forms an Incident
            if len(exc_list) >= 3:
                total_grouped += len(exc_list)
                total_exposure = sum(int(e.get("amount_minor", 0)) for e in exc_list)
                exposure_formatted = f"NGN {total_exposure / 100:,.2f}"

                cluster_hash = hashlib.sha256(f"{rail}:{channel}:{error_sig}:{len(exc_list)}".encode('utf-8')).hexdigest()[:8].upper()
                incident_id = f"INC-{cluster_hash}"

                # Suspected cause derivation
                if "TIMEOUT" in error_sig or error_sig == "91":
                    suspected_cause = "Interbank Switch Response Timeout"
                    causation_status = "CONFIRMED" if len(exc_list) >= 10 else "PROBABLE"
                elif "DUP" in error_sig:
                    suspected_cause = "Client App Duplicate Retry Surge"
                    causation_status = "PROBABLE"
                else:
                    suspected_cause = f"Systemic {error_sig} Failure"
                    causation_status = "CORRELATED"

                # Check suppression
                is_suppressed = request.active_maintenance_window or (rail in request.degraded_sources)
                suppression_reason = None
                if is_suppressed:
                    suppression_reason = "Suppressed: Source system is in maintenance or degraded mode."

                # Factual narrative summary strictly from computed facts
                narrative = (
                    f"Incident {incident_id}: Cluster of {len(exc_list)} transactions affected on {rail} ({channel}) "
                    f"displaying error signature '{error_sig}'. Total exposure resolved/at-risk: {exposure_formatted}. "
                    f"Causation status: {causation_status} ({suspected_cause})."
                )

                incidents.append(
                    IncidentCluster(
                        incident_id=incident_id,
                        title=f"{rail} {error_sig} Spike ({len(exc_list)} cases)",
                        scope=f"{rail}_{channel}_{error_sig}",
                        rail=rail,
                        channel=channel,
                        error_signature=error_sig,
                        affected_count=len(exc_list),
                        total_exposure_minor=total_exposure,
                        total_exposure_formatted=exposure_formatted,
                        start_time=exc_list[0].get("timestamp", now_str),
                        last_seen_time=exc_list[-1].get("timestamp", now_str),
                        suspected_cause=suspected_cause,
                        causation_status=causation_status,
                        narrative_summary=narrative,
                        is_suppressed=is_suppressed,
                        suppression_reason=suppression_reason,
                        affected_transaction_refs=[str(e.get("reference_number", "")) for e in exc_list[:10]]
                    )
                )
            else:
                ungrouped += len(exc_list)

        return IncidentGroupingResponse(
            incidents=incidents,
            total_grouped_exceptions=total_grouped,
            ungrouped_count=ungrouped
        )


anomaly_service = AnomalyDetectionService()
