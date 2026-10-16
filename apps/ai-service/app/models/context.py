"""Pydantic schemas for the Canonical Evidence Bundle, Evidence Items, and Context Retrieval.

Implements Prompt A1 requirements:
- Stable evidence IDs (e.g., EV-TXN-xxx, EV-SW-xxx)
- Tagging of each item as FACT, DERIVED, or NOTE
- Freshness tracking (FRESH, STALE, DEGRADED, UNKNOWN)
- Permission-aware caller context (tenant, role, masking level)
- Token budgeting and audit recording of provided evidence IDs
"""

from enum import Enum
from typing import Dict, List, Optional, Any
from pydantic import BaseModel, Field
import datetime


class FactOrInference(str, Enum):
    """Categorization of evidence confidence per Prompt A1 item 4."""
    FACT = "FACT"         # Sourced directly from immutable system of record (ledger, switch, journal)
    DERIVED = "DERIVED"   # Computed by platform rule, detector, or reconciliation engine
    NOTE = "NOTE"         # Human analyst annotation or customer input


class FreshnessStatus(str, Enum):
    """Freshness status of the evidence source per case M26-M28."""
    FRESH = "FRESH"
    STALE = "STALE"
    DEGRADED = "DEGRADED"
    UNKNOWN = "UNKNOWN"


class EvidenceKind(str, Enum):
    TRANSACTION = "transaction"
    SWITCH_RESPONSE = "switch_response"
    LEDGER_POSTING = "ledger_posting"
    SETTLEMENT_RECORD = "settlement_record"
    REVERSAL_RECORD = "reversal_record"
    ANOMALY_DETECTION = "anomaly_detection"
    CUSTOMER_ALERT = "customer_alert"
    ANALYST_NOTE = "analyst_note"
    POLICY_DECISION = "policy_decision"
    SOP_SNIPPET = "sop_snippet"
    SIMILAR_CASE = "similar_case"


class EvidenceItem(BaseModel):
    """An individual, stable evidence item within the canonical evidence bundle."""
    evidence_id: str = Field(..., description="Stable deterministic ID, e.g. 'EV-TXN-0001842'")
    kind: EvidenceKind
    source_system: str = Field(..., description="Authoritative source: CORE_LEDGER, SWITCH_NIP, etc.")
    fact_or_inference: FactOrInference
    freshness: FreshnessStatus = FreshnessStatus.FRESH
    timestamp: datetime.datetime
    content: Dict[str, Any] = Field(..., description="Structured payload of the evidence item")
    raw_payload_hash: Optional[str] = None
    is_untrusted_input: bool = False
    is_redacted: bool = False
    priority_weight: int = Field(default=50, description="Priority for token budgeting (1-100, 100 highest)")


class CallerContext(BaseModel):
    """Security and tenancy context of the user or system requesting AI context."""
    tenant_id: str = Field(..., description="Institution ID, e.g. 'INST-DEMO-01'")
    user_id: str
    roles: List[str] = Field(default_factory=list)
    can_view_unmasked: bool = False
    unmask_reason: Optional[str] = None


class CanonicalEvidenceBundle(BaseModel):
    """The canonical bundle passed to AI features with stable evidence IDs."""
    case_id: str
    tenant_id: str
    exception_type: str
    classification: str
    transaction_ref: str
    created_at: datetime.datetime = Field(default_factory=lambda: datetime.datetime.now(datetime.timezone.utc))
    items: List[EvidenceItem] = Field(default_factory=list)
    total_token_estimate: int = 0
    provided_evidence_ids: List[str] = Field(default_factory=list)
    stale_sources: List[str] = Field(default_factory=list)
    has_untrusted_inputs: bool = False

    def get_evidence_by_id(self, evidence_id: str) -> Optional[EvidenceItem]:
        for item in self.items:
            if item.evidence_id == evidence_id:
                return item
        return None
