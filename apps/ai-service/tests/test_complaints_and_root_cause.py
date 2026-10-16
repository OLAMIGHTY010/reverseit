"""Unit and integration tests for Complaint Matching, Customer-Safe Wording, and Root-Cause Analysis (Prompts A6 & A3).

Tests:
- Multi-stage complaint matching (exact ref, fuzzy match, disambiguation, unmatched claim)
- Customer-safe wording guardrails and blocklist (English & Nigerian Pidgin)
- Root-cause taxonomy signature matching and causation labeling
- Missing-information detection linked to the 8-check reversal gate
- Abstention on insufficient evidence
- REST API endpoints for complaints and root-cause analysis
"""

import pytest
import datetime
from fastapi.testclient import TestClient

from app.main import app
from app.services.complaint_matcher import complaint_matcher
from app.services.safe_wording import safe_wording_service
from app.services.root_cause_engine import root_cause_engine
from app.services.context_builder import SafeContextBuilder
from app.models.context import CallerContext
from app.models.complaint_matching import CustomerSafeWordingRequest
from app.models.taxonomy import RootCauseCategory, CausationStatus

client = TestClient(app)


# ---------------------------------------------------------------------------
# Prompt A6: Complaint Matching Tests
# ---------------------------------------------------------------------------

@pytest.fixture
def candidate_transactions():
    return [
        {
            "transaction_id": "TXN-001",
            "reference_number": "NIP-WEMA-7781002",
            "session_id": "999000184299102",
            "amount_minor": 1850000,
            "channel": "MOBILE",
            "rail": "NIP",
            "initiated_at": "2026-10-14 09:12:40",
            "account_number": "0123456789"
        },
        {
            "transaction_id": "TXN-002",
            "reference_number": "NIP-WEMA-7781099",
            "session_id": "999000184299999",
            "amount_minor": 1850000,
            "channel": "MOBILE",
            "rail": "NIP",
            "initiated_at": "2026-10-14 09:12:42",
            "account_number": "0123456789"
        },
        {
            "transaction_id": "TXN-003",
            "reference_number": "POS-WEMA-112233",
            "session_id": "POS_SESSION_01",
            "amount_minor": 500000,
            "channel": "POS",
            "rail": "CARD",
            "initiated_at": "2026-10-14 10:00:00",
            "account_number": "0123456789"
        }
    ]


def test_complaint_exact_reference_matching(candidate_transactions):
    """Exact reference match immediately auto-links."""
    result = complaint_matcher.match(
        complaint_ref="CUS-COMPLAINT-01",
        claimed_ref="NIP-WEMA-7781002",
        claimed_amount_minor=1850000,
        claimed_account_tail="6789",
        claimed_channel="MOBILE",
        candidate_pool=candidate_transactions
    )

    assert result.auto_linked is True
    assert result.matching_stage == "exact_reference"
    assert result.top_candidate.transaction_id == "TXN-001"
    assert result.top_candidate.is_exact_match is True


def test_complaint_constrained_fuzzy_matching_disambiguation(candidate_transactions):
    """When multiple candidates share amount and account without ref, route to human disambiguation."""
    result = complaint_matcher.match(
        complaint_ref="CUS-COMPLAINT-02",
        claimed_ref=None,
        claimed_amount_minor=1850000,
        claimed_account_tail="6789",
        claimed_channel="MOBILE",
        candidate_pool=candidate_transactions
    )

    # Both TXN-001 and TXN-002 have same amount and account tail -> ambiguity
    assert result.route_to_human_disambiguation is True
    assert result.auto_linked is False
    assert len(result.candidates) >= 2


def test_complaint_unmatched_claim(candidate_transactions):
    """Non-matching transaction creates an unmatched claim."""
    result = complaint_matcher.match(
        complaint_ref="CUS-COMPLAINT-03",
        claimed_ref="NON_EXISTENT_REF",
        claimed_amount_minor=99999999,
        claimed_account_tail="0000",
        claimed_channel="ATM",
        candidate_pool=candidate_transactions
    )

    assert result.is_unmatched_claim is True
    assert len(result.candidates) == 0


def test_customer_safe_wording_unverified_blocklist():
    """Customer-safe wording generator strictly blocks prohibited terms when unverified."""
    req = CustomerSafeWordingRequest(
        case_id="CASE-100",
        state="UNDER_INVESTIGATION",
        financial_verified=False,
        amount_formatted="NGN 20,000.00",
        language="en"
    )
    resp = safe_wording_service.generate_wording(req)

    assert resp.is_compliant is True
    lower = resp.message.lower()
    assert "refunded" not in lower
    assert "reversed" not in lower
    assert "credited" not in lower
    assert "funds returned" not in lower
    assert "No action is needed now" in resp.message


def test_customer_safe_wording_pidgin_support():
    """Nigerian Pidgin language generation."""
    req = CustomerSafeWordingRequest(
        case_id="CASE-101",
        state="DETECTED",
        financial_verified=False,
        amount_formatted="NGN 50,000.00",
        language="pcm"
    )
    resp = safe_wording_service.generate_wording(req)

    assert resp.language == "pcm"
    assert "we don see say" in resp.message.lower()
    assert "refunded" not in resp.message.lower()


# ---------------------------------------------------------------------------
# Prompt A3: Root Cause & Missing Information Tests
# ---------------------------------------------------------------------------

@pytest.fixture
def switch_timeout_bundle():
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
        "initiated_at": now,
        "created_at": now + datetime.timedelta(seconds=20),
        "exception_type": "EX-DNC",
        "classification": "FAIL001",
        "debit_status": "CONFIRMED_SUCCESS",
        "switch_status": "TIMEOUT",
    }
    raw_evidence = [
        {
            "kind": "ledger_posting",
            "source_system": "CORE_LEDGER",
            "ref": "LEDG-001",
            "fact_or_inference": "FACT",
            "freshness": "FRESH",
            "timestamp": now,
            "content": {"status": "POSTED", "amount_minor": 1850000}
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


def test_root_cause_switch_timeout_signature(switch_timeout_bundle):
    """Switch response code 91 correctly identifies SWITCH_TIMEOUT as confirmed/probable root cause."""
    res = root_cause_engine.analyze(switch_timeout_bundle)

    assert res.abstain_flag is False
    assert res.primary_cause == RootCauseCategory.SWITCH_TIMEOUT
    assert len(res.hypotheses) >= 1
    top_hyp = res.hypotheses[0]
    assert top_hyp.cause == RootCauseCategory.SWITCH_TIMEOUT
    assert top_hyp.causation_status in [CausationStatus.CONFIRMED, CausationStatus.PROBABLE]
    # Every hypothesis must link to supporting evidence
    assert len(top_hyp.supporting_evidence_ids) > 0


def test_missing_information_detector(switch_timeout_bundle):
    """Missing information detector flags missing settlement or TSQ confirmation."""
    res = root_cause_engine.analyze(switch_timeout_bundle)

    # Check that missing evidence items are identified and ranked
    assert len(res.missing_information) >= 1
    # Check 3 (settlement confirmation) should be missing in this bundle
    check_numbers = [m.check_number for m in res.missing_information]
    assert 3 in check_numbers


def test_root_cause_abstention_on_empty_evidence():
    """System abstains when evidence is insufficient (Zero ungrounded speculation)."""
    empty_bundle = SafeContextBuilder.build_bundle(
        case_data={"case_id": "CASE-EMPTY", "tenant_id": "INST-DEMO-01", "transaction_ref": "REF-001"},
        raw_evidence_items=[],
        caller=CallerContext(tenant_id="INST-DEMO-01", user_id="analyst_01")
    )
    # Clear items to simulate completely empty context
    empty_bundle.items = []

    res = root_cause_engine.analyze(empty_bundle)

    assert res.abstain_flag is True
    assert res.primary_cause == RootCauseCategory.UNKNOWN
    assert "abstains" in res.hypotheses[0].rationale.lower()


# ---------------------------------------------------------------------------
# REST API Endpoint Tests
# ---------------------------------------------------------------------------

def test_complaint_matching_api(candidate_transactions):
    """Test POST /v1/ai/complaints/match."""
    resp = client.post(
        "/v1/ai/complaints/match",
        json={
            "complaint_ref": "CUS-API-01",
            "claimed_ref": "NIP-WEMA-7781002",
            "candidate_pool": candidate_transactions
        }
    )
    assert resp.status_code == 200
    data = resp.json()
    assert data["auto_linked"] is True
    assert data["top_candidate"]["transaction_id"] == "TXN-001"


def test_root_cause_analysis_api(switch_timeout_bundle):
    """Test POST /v1/ai/root-cause/analyze."""
    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    resp = client.post(
        "/v1/ai/root-cause/analyze",
        json={
            "case_data": {
                "case_id": "CASE-API-RC",
                "tenant_id": "INST-DEMO-01",
                "transaction_ref": "NIP-API-RC",
                "rail": "NIP",
                "amount_minor": 100000,
                "exception_type": "EX-DNC",
                "classification": "FAIL001",
                "switch_status": "TIMEOUT"
            },
            "raw_evidence_items": [
                {
                    "kind": "switch_response",
                    "source_system": "SWITCH_NIP",
                    "ref": "SW-01",
                    "fact_or_inference": "FACT",
                    "freshness": "FRESH",
                    "timestamp": now,
                    "content": {"status": "TIMEOUT", "response_code": "91"}
                }
            ],
            "caller": {
                "tenant_id": "INST-DEMO-01",
                "user_id": "analyst_api",
                "roles": ["analyst"]
            }
        }
    )
    assert resp.status_code == 200
    data = resp.json()
    assert data["primary_cause"] == RootCauseCategory.SWITCH_TIMEOUT.value
    assert len(data["hypotheses"]) >= 1
    assert len(data["missing_information"]) >= 1
