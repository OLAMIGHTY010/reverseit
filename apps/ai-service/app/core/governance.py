"""AI/ML Governance, Capabilities Inventory, and Boundary Enforcement.

Implements Prompt A0 requirements:
- Three-tier AI capability classification (T1, T2, T3)
- Strict non-negotiable boundaries (no money movement, no overriding deterministic controls)
- Read-only tool execution allow-list
- Rejection of financial mutation or authorization actions
"""

from enum import Enum
from typing import Dict, List, Set, Any
from pydantic import BaseModel, Field


class AICapabilityTier(str, Enum):
    """Three capability tiers defined in Prompt A0."""
    TIER_1_LANGUAGE_ASSISTANCE = "T1_LANGUAGE_ASSISTANCE"
    TIER_2_ANALYTICS_AND_DETECTION = "T2_ANALYTICS_AND_DETECTION"
    TIER_3_RECOMMENDATIONS = "T3_RECOMMENDATIONS"


class AICapability(str, Enum):
    # Tier 1: Language Assistance (Read-Only)
    INVESTIGATION_SUMMARY = "investigation_summary"
    CASE_EXPLANATION = "case_explanation"
    DRAFT_NOTES = "draft_notes"
    MISSING_INFO_DETECTION = "missing_info_detection"
    INCIDENT_NARRATIVE = "incident_narrative"
    ALERT_PARSING = "alert_parsing"
    COMPLAINT_DRAFTING = "complaint_drafting"
    CUSTOMER_SAFE_WORDING = "customer_safe_wording"

    # Tier 2: Analytics and Detection (Advisory, Non-Decisive)
    ANOMALY_SPIKE_DETECTION = "anomaly_spike_detection"
    PRIORITY_RANKING = "priority_ranking"
    DUPLICATE_LIKELIHOOD_SCORING = "duplicate_likelihood_scoring"
    PENDING_RESOLUTION_PROBABILITY = "pending_resolution_probability"
    COMPLAINT_MATCHING = "complaint_matching"
    ALERT_AUTHENTICITY_SIGNAL = "alert_authenticity_signal"

    # Tier 3: Recommendations (Human-in-the-Loop)
    NEXT_BEST_CHECK = "next_best_check"
    RULE_CHANGE_SUGGESTION = "rule_change_suggestion"
    INCIDENT_GROUPING_SUGGESTION = "incident_grouping_suggestion"


# Explicit, immutable registry mapping each capability to its tier
CAPABILITY_TIER_MAPPING: Dict[AICapability, AICapabilityTier] = {
    AICapability.INVESTIGATION_SUMMARY: AICapabilityTier.TIER_1_LANGUAGE_ASSISTANCE,
    AICapability.CASE_EXPLANATION: AICapabilityTier.TIER_1_LANGUAGE_ASSISTANCE,
    AICapability.DRAFT_NOTES: AICapabilityTier.TIER_1_LANGUAGE_ASSISTANCE,
    AICapability.MISSING_INFO_DETECTION: AICapabilityTier.TIER_1_LANGUAGE_ASSISTANCE,
    AICapability.INCIDENT_NARRATIVE: AICapabilityTier.TIER_1_LANGUAGE_ASSISTANCE,
    AICapability.ALERT_PARSING: AICapabilityTier.TIER_1_LANGUAGE_ASSISTANCE,
    AICapability.COMPLAINT_DRAFTING: AICapabilityTier.TIER_1_LANGUAGE_ASSISTANCE,
    AICapability.CUSTOMER_SAFE_WORDING: AICapabilityTier.TIER_1_LANGUAGE_ASSISTANCE,

    AICapability.ANOMALY_SPIKE_DETECTION: AICapabilityTier.TIER_2_ANALYTICS_AND_DETECTION,
    AICapability.PRIORITY_RANKING: AICapabilityTier.TIER_2_ANALYTICS_AND_DETECTION,
    AICapability.DUPLICATE_LIKELIHOOD_SCORING: AICapabilityTier.TIER_2_ANALYTICS_AND_DETECTION,
    AICapability.PENDING_RESOLUTION_PROBABILITY: AICapabilityTier.TIER_2_ANALYTICS_AND_DETECTION,
    AICapability.COMPLAINT_MATCHING: AICapabilityTier.TIER_2_ANALYTICS_AND_DETECTION,
    AICapability.ALERT_AUTHENTICITY_SIGNAL: AICapabilityTier.TIER_2_ANALYTICS_AND_DETECTION,

    AICapability.NEXT_BEST_CHECK: AICapabilityTier.TIER_3_RECOMMENDATIONS,
    AICapability.RULE_CHANGE_SUGGESTION: AICapabilityTier.TIER_3_RECOMMENDATIONS,
    AICapability.INCIDENT_GROUPING_SUGGESTION: AICapabilityTier.TIER_3_RECOMMENDATIONS,
}


# Strict Allow-List of READ-ONLY tools permissible for AI / Copilot orchestration (A7)
READ_ONLY_TOOL_ALLOW_LIST: Set[str] = {
    "get_case",
    "get_evidence",
    "search_similar_cases",
    "get_incident",
    "get_policy_summary",
    "get_source_freshness",
}


# Explicit Disallow-List: ANY action attempting to touch money or modify state is strictly blocked
FORBIDDEN_MUTATING_ACTIONS: Set[str] = {
    "execute_reversal",
    "initiate_reversal",
    "approve_reversal",
    "reject_reversal",
    "change_case_status",
    "modify_policy",
    "override_gate",
    "post_ledger_entry",
    "dispatch_notification",
    "unmask_account_data",
}


class GovernanceViolationError(Exception):
    """Raised when an AI capability or tool execution attempts an unauthorized or mutating action."""
    pass


class AIGovernancePolicy:
    """Enforces non-negotiable governance boundaries across all AI/ML invocations."""

    @staticmethod
    def validate_tool_call(tool_name: str) -> None:
        """Verifies that a requested tool belongs strictly to the read-only allow-list.
        
        Raises GovernanceViolationError if tool is mutating or unapproved.
        """
        normalized_name = tool_name.strip().lower()
        if normalized_name in FORBIDDEN_MUTATING_ACTIONS:
            raise GovernanceViolationError(
                f"Governance Breach: Tool '{tool_name}' attempts financial or state mutation. "
                "AI components are strictly prohibited from moving money or changing system state."
            )
        if normalized_name not in READ_ONLY_TOOL_ALLOW_LIST:
            raise GovernanceViolationError(
                f"Governance Breach: Tool '{tool_name}' is not in the approved read-only allow-list. "
                f"Permitted tools: {sorted(list(READ_ONLY_TOOL_ALLOW_LIST))}"
            )

    @staticmethod
    def assert_non_decisive(capability: AICapability, confidence_score: float) -> None:
        """Asserts that confidence score cannot override deterministic controls."""
        # Non-negotiable rule 1 & 5: AI confidence never overrides deterministic controls or gates.
        if confidence_score < 0.0 or confidence_score > 1.0:
            raise ValueError(f"Confidence score must be in range [0.0, 1.0], received {confidence_score}")

    @staticmethod
    def get_tier(capability: AICapability) -> AICapabilityTier:
        """Returns the governance tier for a given capability."""
        return CAPABILITY_TIER_MAPPING[capability]
