"""Pydantic schemas for ReverseIt Customer AI components.

Implements Prompt A9, Section 10, and Scope v2:
- Alert parse schemas with per-field confidence and source attribution
- Dispute classification proposing type with deterministic regulatory clock mapping
- Factual complaint drafting schemas with fact verification
- Advisory authenticity and tamper signals
"""

from enum import Enum
from typing import Dict, List, Optional, Any
from pydantic import BaseModel, Field
import datetime


class DisputeType(str, Enum):
    """Dispute categories per Section 10.2 & 10.4."""
    FAILED_TRANSFER = "failed_transfer"                     # NIP / instant transfer failed
    DUPLICATE_DEBIT = "duplicate_debit"                     # Double posting or repeat debit
    POS_WEB_FAILURE = "pos_web_failure"                     # POS declined or web timeout with debit
    ATM_FAILURE_ON_US = "atm_failure_on_us"                 # Wema ATM no/partial cash dispense
    ATM_FAILURE_NOT_ON_US = "atm_failure_not_on_us"         # Other bank ATM cash not dispensed
    WRONG_ACCOUNT_OR_AMOUNT = "wrong_account_or_amount"     # Customer error path (written request, recall)
    OTHER = "other"


class ParsedAlertField(BaseModel):
    value: Optional[str] = None
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    source: str = Field(default="rule", description="'rule' or 'model'")


class AlertParseResult(BaseModel):
    """Result of parsing Nigerian bank SMS / email debit alerts."""
    model_config = {"protected_namespaces": ()}
    
    raw_text_sanitized: str
    issuing_bank: str = Field(default="UNKNOWN", description="e.g. WEMA, GTBANK, ZENITH, ACCESS, FIRSTBANK, UBA")
    is_non_wema: bool = False
    amount_minor: Optional[int] = None
    amount_formatted: Optional[str] = None
    currency: str = "NGN"
    account_tail: Optional[str] = None
    date_time_str: Optional[str] = None
    parsed_datetime: Optional[datetime.datetime] = None
    narration: Optional[str] = None
    reference: Optional[str] = None
    channel: Optional[str] = None
    balance_minor: Optional[int] = None
    balance_redacted: bool = True
    overall_confidence: float = 0.0
    field_confidences: Dict[str, float] = Field(default_factory=dict)
    tamper_flags: List[str] = Field(default_factory=list)
    parse_source: str = "rule"  # "rule" or "llm_fallback"


class DisputeClassificationResult(BaseModel):
    """Dispute classification outcome.
    
    IMPORTANT: The LLM NEVER picks the clock. The clock is bound deterministically
    via the clock registry mapping table (Section 10.4).
    """
    model_config = {"protected_namespaces": ()}

    proposed_type: DisputeType
    confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    evidence_phrases: List[str] = Field(default_factory=list)
    requires_human_review: bool = False
    
    # Deterministic Clock Lookup (Hard-coded to Section 10.4 table)
    clock_id: str
    clock_name: str
    clock_duration_hours: int
    clock_unit: str  # "hours" or "working_days"
    clock_source_ref: str
    clock_status: str  # "confirmed", "reported", "draft", "conflicting"


class ComplaintDraftRequest(BaseModel):
    """Structured facts used to generate a customer dispute complaint."""
    case_ref: str = "PENDING_CUS"
    customer_token: str = "CUS-TOK-01"
    masked_account: str
    amount_formatted: str
    amount_minor: int
    transaction_date: str
    narration: Optional[str] = "Transfer"
    reference: Optional[str] = "N/A"
    dispute_type: DisputeType
    clock_name: str
    clock_duration_hours: int
    clock_unit: str = "hours"
    requested_action: str = "Investigation and reversal of uncredited funds"


class ComplaintDraftResponse(BaseModel):
    """Verified complaint draft with strict fact-checking metadata."""
    model_config = {"protected_namespaces": ()}

    complaint_text: str
    structured_facts_echo: Dict[str, Any]
    fact_verification_passed: bool
    unverified_tokens_detected: List[str] = Field(default_factory=list)
    is_fallback: bool = False
    generated_at: datetime.datetime = Field(default_factory=lambda: datetime.datetime.now(datetime.timezone.utc))


class AuthenticitySignalResult(BaseModel):
    """Advisory authenticity check results (N1 / Prompt A9 Item 6)."""
    model_config = {"protected_namespaces": ()}

    is_suspicious: bool = False
    tamper_flags: List[str] = Field(default_factory=list)
    advisory_notes: str = ""
    severity: str = "INFO"  # "INFO", "WARNING", "HIGH"
