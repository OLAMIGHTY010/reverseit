"""Root-Cause Taxonomy and Missing-Information Models.

Implements Prompt A3 and Section 5.7:
- 14-category root-cause taxonomy
- Causation status tracking (CORRELATED, PROBABLE, CONFIRMED)
- Evidence-grounded hypotheses with supporting & contradicting evidence IDs
- Missing-information detection mapped to the 8-check reversal gate
"""

from enum import Enum
from typing import Dict, List, Optional, Any
from pydantic import BaseModel, Field
from app.models.schemas import ModelConfidence


class RootCauseCategory(str, Enum):
    SWITCH_TIMEOUT = "SWITCH_TIMEOUT"
    BENEFICIARY_BANK_DELAY = "BENEFICIARY_BANK_DELAY"
    GATEWAY_ACK_LOSS = "GATEWAY_ACK_LOSS"
    DUPLICATE_RETRY = "DUPLICATE_RETRY"
    LEDGER_POSTING_DELAY = "LEDGER_POSTING_DELAY"
    SETTLEMENT_CUTOFF = "SETTLEMENT_CUTOFF"
    VENDOR_DELIVERY_FAILURE = "VENDOR_DELIVERY_FAILURE"
    ATM_HARDWARE_DISPENSE_FAULT = "ATM_HARDWARE_DISPENSE_FAULT"
    ACCOUNT_RESTRICTION = "ACCOUNT_RESTRICTION"
    DATA_MAPPING_ERROR = "DATA_MAPPING_ERROR"
    INFRASTRUCTURE_OUTAGE = "INFRASTRUCTURE_OUTAGE"
    CUSTOMER_ERROR = "CUSTOMER_ERROR"
    SUSPECTED_FRAUD = "SUSPECTED_FRAUD"
    UNKNOWN = "UNKNOWN"


class CausationStatus(str, Enum):
    CORRELATED = "CORRELATED"   # Statistical correlation with active incident or error code
    PROBABLE = "PROBABLE"       # High evidence alignment, awaiting final system confirmation
    CONFIRMED = "CONFIRMED"     # Verified by system of record (e.g., TSQ 91 confirmed or ATM journal verified)


class RootCauseHypothesis(BaseModel):
    model_config = {"protected_namespaces": ()}

    cause: RootCauseCategory
    causation_status: CausationStatus
    model_confidence: ModelConfidence
    supporting_evidence_ids: List[str] = Field(default_factory=list)
    contradicting_evidence_ids: List[str] = Field(default_factory=list)
    confirmation_check: str = Field(..., description="Action that will confirm or refute this hypothesis")
    rationale: str


class MissingInformationItem(BaseModel):
    model_config = {"protected_namespaces": ()}

    check_number: int = Field(..., description="Reversal gate check ID (1-8)")
    required_field: str
    target_system: str
    action_name: str
    action_type: str = "ONE_CLICK_QUERY"
    uncertainty_reduction_weight: float = Field(..., ge=0.0, le=1.0)


class RootCauseAnalysisResponse(BaseModel):
    model_config = {"protected_namespaces": ()}

    case_id: str
    primary_cause: RootCauseCategory
    hypotheses: List[RootCauseHypothesis] = Field(default_factory=list)
    missing_information: List[MissingInformationItem] = Field(default_factory=list)
    abstain_flag: bool = False
    is_fallback: bool = False
