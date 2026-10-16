"""Unit tests for AI Governance, Boundaries, and Kill Switches.

Tests strict Prompt A0 invariants:
- Capabilities mapped to correct Tiers (T1, T2, T3)
- Tool call allow-list enforcement
- Rejection of financial mutation actions
- Multi-scope kill switch tripping and dual-control re-enabling
"""

import pytest
from app.core.governance import (
    AICapability,
    AICapabilityTier,
    AIGovernancePolicy,
    GovernanceViolationError,
    READ_ONLY_TOOL_ALLOW_LIST,
    FORBIDDEN_MUTATING_ACTIONS,
)
from app.core.kill_switch import (
    KillSwitchManager,
    KillSwitchScope,
    AIFeatureSwitch,
)


def test_capability_tier_inventory():
    """Verify all capabilities are mapped to one of the three approved tiers."""
    for cap in AICapability:
        tier = AIGovernancePolicy.get_tier(cap)
        assert tier in [
            AICapabilityTier.TIER_1_LANGUAGE_ASSISTANCE,
            AICapabilityTier.TIER_2_ANALYTICS_AND_DETECTION,
            AICapabilityTier.TIER_3_RECOMMENDATIONS,
        ]


def test_read_only_tool_allow_list():
    """Verify that allowed read-only tools pass validation."""
    for tool in READ_ONLY_TOOL_ALLOW_LIST:
        AIGovernancePolicy.validate_tool_call(tool)


def test_mutating_actions_are_strictly_forbidden():
    """Verify that any action attempting to touch money or alter state is blocked with GovernanceViolationError."""
    for forbidden_tool in FORBIDDEN_MUTATING_ACTIONS:
        with pytest.raises(GovernanceViolationError) as exc_info:
            AIGovernancePolicy.validate_tool_call(forbidden_tool)
        assert "Governance Breach" in str(exc_info.value)
        assert "financial or state mutation" in str(exc_info.value)


def test_unregistered_tools_are_rejected():
    """Any unapproved tool not in the allow-list is rejected."""
    with pytest.raises(GovernanceViolationError) as exc_info:
        AIGovernancePolicy.validate_tool_call("arbitrary_unknown_tool")
    assert "not in the approved read-only allow-list" in str(exc_info.value)


def test_confidence_score_validation():
    """Verify confidence score validity bounds."""
    AIGovernancePolicy.assert_non_decisive(AICapability.DUPLICATE_LIKELIHOOD_SCORING, 0.85)
    with pytest.raises(ValueError):
        AIGovernancePolicy.assert_non_decisive(AICapability.DUPLICATE_LIKELIHOOD_SCORING, 1.2)
    with pytest.raises(ValueError):
        AIGovernancePolicy.assert_non_decisive(AICapability.DUPLICATE_LIKELIHOOD_SCORING, -0.1)


def test_kill_switch_lifecycle_and_dual_control():
    """Verify kill switch tripping and dual-control re-enabling."""
    manager = KillSwitchManager()

    # Feature is initially enabled
    assert not manager.is_feature_disabled(AIFeatureSwitch.SUMMARIES)

    # Trip the feature switch
    manager.trip_switch(
        scope=KillSwitchScope.FEATURE,
        target="summaries",
        actor="ops_analyst_01",
        reason="Suspected hallucination in summary outputs"
    )
    assert manager.is_feature_disabled(AIFeatureSwitch.SUMMARIES)

    # Attempting to re-enable with same approver fails dual control
    with pytest.raises(ValueError) as exc_info:
        manager.re_enable_switch(
            scope=KillSwitchScope.FEATURE,
            target="summaries",
            first_approver="risk_officer_01",
            second_approver="risk_officer_01",
            reason="Verified safe"
        )
    assert "Dual-control violation" in str(exc_info.value)

    # Re-enable with two distinct approvers succeeds
    record = manager.re_enable_switch(
        scope=KillSwitchScope.FEATURE,
        target="summaries",
        first_approver="risk_officer_01",
        second_approver="head_of_ops_02",
        reason="Model fine-tuned and verified"
    )
    assert not record.is_active
    assert not manager.is_feature_disabled(AIFeatureSwitch.SUMMARIES)


def test_kill_switch_scopes():
    """Verify global, rail, and exception_type kill switch cascades."""
    manager = KillSwitchManager()

    # Global kill switch disables everything
    manager.trip_switch(KillSwitchScope.GLOBAL, "ALL", "cro_01", "Global emergency freeze")
    assert manager.is_feature_disabled(AIFeatureSwitch.COPILOT)
    assert manager.is_feature_disabled(AIFeatureSwitch.ANOMALY_DETECTION)

    # Re-enable global switch
    manager.re_enable_switch(KillSwitchScope.GLOBAL, "ALL", "cro_01", "cto_02", "Emergency resolved")
    assert not manager.is_feature_disabled(AIFeatureSwitch.COPILOT)

    # Rail switch disables feature specifically for that rail
    manager.trip_switch(KillSwitchScope.RAIL, "NIP", "head_payments", "NIP switch maintenance")
    assert manager.is_feature_disabled(AIFeatureSwitch.SUMMARIES, rail="NIP")
    assert not manager.is_feature_disabled(AIFeatureSwitch.SUMMARIES, rail="CARD")
