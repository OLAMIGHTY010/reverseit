"""Unit and integration tests for Investigation Summary Generator, Citation Validator, and Dual Wording (Prompt A2).

Verifies Prompt A2 requirements:
- Validated JSON schema output
- 0.0% tolerance for fabricated evidence IDs
- Seeded fabrication detection and rejection
- Deterministic template fallback
- Customer-safe / agent-safe wording (forbids 'refunded', 'reversed', 'credited' when unverified)
- Ops reviewer summary with citations (Scope v2)
- REST API endpoint integration
"""

import pytest
import datetime
from fastapi.testclient import TestClient

from app.main import app
from app.services.context_builder import SafeContextBuilder
from app.services.summary_generator import summary_generator
from app.services.citation_validator import citation_validator
from app.models.context import CallerContext
from app.models.schemas import (
    InvestigationSummaryResponse,
    CitedFact,
    CitedInference,
    ModelConfidence,
)
from app.core.kill_switch import kill_switch_manager, KillSwitchScope, AIFeatureSwitch

client = TestClient(app)


@pytest.fixture
def sample_bundle():
    now = datetime.datetime(2026, 10, 14, 9, 12, 40, tzinfo=datetime.timezone.utc)
    case_data = {
        "case_id": "CASE-2026-004417",
        "tenant_id": "INST-DEMO-01",
        "transaction_ref": "NIP-DEMO-7781002",
        "rail": "NIP",
        "channel": "MOBILE",
        "amount_minor": 1850000,
        "currency": "NGN",
        "account_number": "0123456789",
        "beneficiary_account": "0987654321",
        "initiated_at": now,
        "created_at": now + datetime.timedelta(seconds=20),
        "exception_type": "EX-DNC",
        "classification": "FAIL001",
        "debit_status": "CONFIRMED_SUCCESS",
        "credit_status": "UNKNOWN",
        "switch_status": "TRANSFER_RESPONSE_TIMEOUT",
    }
    raw_evidence = [
        {
            "kind": "ledger_posting",
            "source_system": "CORE_LEDGER",
            "ref": "LEDG-001",
            "fact_or_inference": "FACT",
            "freshness": "FRESH",
            "timestamp": now,
            "content": {"posting_id": "LEDG-001", "status": "POSTED", "amount_minor": 1850000}
        },
        {
            "kind": "switch_response",
            "source_system": "SWITCH_NIP",
            "ref": "SW-001",
            "fact_or_inference": "FACT",
            "freshness": "FRESH",
            "timestamp": now + datetime.timedelta(seconds=1),
            "content": {"response_code": "91", "status": "TIMEOUT"}
        }
    ]
    caller = CallerContext(
        tenant_id="INST-DEMO-01",
        user_id="analyst_01",
        roles=["analyst"]
    )
    return SafeContextBuilder.build_bundle(case_data, raw_evidence, caller)


def test_summary_generation_happy_path(sample_bundle):
    """Test generating a valid summary with all facts cited."""
    summary = summary_generator.generate_summary(sample_bundle, financial_verified=False)

    assert summary.case_id == "CASE-2026-004417"
    assert "NGN 18,500.00" in summary.headline
    assert len(summary.confirmed_facts) >= 2
    assert len(summary.inferred_explanations) >= 1

    # Verify all cited evidence IDs exist in bundle
    validation = citation_validator.validate_summary(summary, sample_bundle)
    assert validation.is_valid is True
    assert len(validation.fabricated_evidence_ids) == 0
    assert len(validation.uncited_statements) == 0

    # Ops reviewer summary must contain citations (Scope v2)
    assert "Case CASE-2026-004417" in summary.ops_reviewer_summary
    assert "[" in summary.ops_reviewer_summary and "]" in summary.ops_reviewer_summary


def test_citation_validator_catches_seeded_fabrications(sample_bundle):
    """Zero tolerance: validator must detect seeded hallucinated evidence IDs."""
    summary = summary_generator.generate_summary(sample_bundle, financial_verified=False)

    # Seed an invalid/hallucinated evidence ID
    summary.confirmed_facts.append(
        CitedFact(text="Beneficiary bank acknowledged payment.", evidence_ids=["EV-FABRICATED-FAKE999"])
    )

    validation = citation_validator.validate_summary(summary, sample_bundle)
    assert validation.is_valid is False
    assert "EV-FABRICATED-FAKE999" in validation.fabricated_evidence_ids
    assert "Fabricated evidence IDs detected" in validation.error_message


def test_agent_safe_wording_unverified_guardrail(sample_bundle):
    """When financial status is unverified, 'refunded', 'reversed', or 'credited' must never appear."""
    summary = summary_generator.generate_summary(sample_bundle, financial_verified=False)

    lower_wording = summary.agent_safe_wording.lower()
    assert "refunded" not in lower_wording
    assert "reversed" not in lower_wording
    assert "credited" not in lower_wording
    assert "funds back" not in lower_wording
    assert "money returned" not in lower_wording
    assert "No action is needed from you" in summary.agent_safe_wording or "internal" in summary.agent_safe_wording


def test_agent_safe_wording_verified_state(sample_bundle):
    """When financial status is VERIFIED, restored funds confirmation is permitted."""
    summary = summary_generator.generate_summary(sample_bundle, financial_verified=True)

    lower_wording = summary.agent_safe_wording.lower()
    assert "restored" in lower_wording or "resolved" in lower_wording


def test_kill_switch_triggers_deterministic_fallback(sample_bundle):
    """Tripping the summaries kill switch forces deterministic fallback."""
    kill_switch_manager.trip_switch(
        scope=KillSwitchScope.FEATURE,
        target="summaries",
        actor="risk_officer",
        reason="Model audit test"
    )

    try:
        summary = summary_generator.generate_summary(sample_bundle, financial_verified=False)
        assert summary.is_fallback is True
        assert summary.model_version == "rule-based-fallback-v1.0"
    finally:
        kill_switch_manager.re_enable_switch(
            scope=KillSwitchScope.FEATURE,
            target="summaries",
            first_approver="risk_officer",
            second_approver="ops_lead",
            reason="Test complete"
        )


def test_api_endpoint_summary_generation(sample_bundle):
    """Test REST API endpoint /v1/ai/summary."""
    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    payload = {
        "case_data": {
            "case_id": "CASE-API-01",
            "tenant_id": "INST-DEMO-01",
            "transaction_ref": "NIP-API-9901",
            "rail": "NIP",
            "channel": "MOBILE",
            "amount_minor": 500000,
            "currency": "NGN",
            "account_number": "0123456789",
            "created_at": now,
            "exception_type": "EX-DNC",
            "classification": "FAIL001",
            "debit_status": "CONFIRMED_SUCCESS",
            "switch_status": "TIMEOUT"
        },
        "raw_evidence_items": [
            {
                "kind": "switch_response",
                "source_system": "SWITCH_NIP",
                "ref": "SW-API-01",
                "fact_or_inference": "FACT",
                "freshness": "FRESH",
                "timestamp": now,
                "content": {"status": "TIMEOUT", "response_code": "91"}
            }
        ],
        "caller": {
            "tenant_id": "INST-DEMO-01",
            "user_id": "analyst_api",
            "roles": ["analyst"],
            "can_view_unmasked": False
        },
        "financial_verified": False,
        "force_fallback": False
    }

    response = client.post("/v1/ai/summary", json=payload)
    assert response.status_code == 200
    data = response.json()
    assert data["case_id"] == "CASE-API-01"
    assert "NGN 5,000.00" in data["headline"]
    assert len(data["confirmed_facts"]) >= 1
    assert "ops_reviewer_summary" in data
    assert "agent_safe_wording" in data
