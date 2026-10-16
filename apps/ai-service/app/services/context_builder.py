"""Evidence Retrieval and Safe Context Builder Service.

Implements Prompt A1:
- Builds canonical evidence bundle with stable deterministic evidence IDs
- Tags every item as FACT, DERIVED, or NOTE
- Multi-tenant boundary enforcement (no cross-tenant leakage)
- Permission-aware PII masking and redaction
- Freshness tracking and stale source detection
- Priority-based token budgeting
- Untrusted content sanitization and neutralization
- Approved SOP and similar case snippet retrieval
"""

import hashlib
import json
import datetime
from typing import List, Dict, Optional, Any
from dateutil import parser as date_parser

from app.models.context import (
    CanonicalEvidenceBundle,
    EvidenceItem,
    EvidenceKind,
    FactOrInference,
    FreshnessStatus,
    CallerContext,
)
from app.services.masking import masking_service
from app.services.sanitizer import sanitizer
from app.core.config import settings


def _parse_or_now(val: Any) -> datetime.datetime:
    """Robustly parses a datetime object, ISO/date string, or defaults to current UTC time."""
    if isinstance(val, datetime.datetime):
        if val.tzinfo is None:
            return val.replace(tzinfo=datetime.timezone.utc)
        return val
    if isinstance(val, str) and val.strip():
        try:
            parsed = date_parser.parse(val)
            if parsed.tzinfo is None:
                return parsed.replace(tzinfo=datetime.timezone.utc)
            return parsed
        except Exception:
            pass
    return datetime.datetime.now(datetime.timezone.utc)



class ApprovedSOPRegistry:
    """Registry of officially approved Standard Operating Procedures (SOPs)."""
    
    _APPROVED_SOPS = [
        {
            "sop_id": "SOP-NIP-REV-01",
            "title": "NIP Inward/Outward Failed Transaction Reversal SOP v2.2",
            "rail": "NIP",
            "effective_date": "2026-01-01",
            "summary": "Where NIP switch returns TIMEOUT or 91 (System error), status query (TSQ) must be performed. If TSQ confirms debit without beneficiary credit and 24-hour regulatory window applies, reversal is eligible upon ledger verification.",
        },
        {
            "sop_id": "SOP-POS-DISP-01",
            "title": "POS and Web Transaction Exception Handling SOP v1.4",
            "rail": "POS",
            "effective_date": "2026-02-15",
            "summary": "POS transactions with 72-hour dispute window require switch clearing log validation and merchant settlement confirmation before reversal.",
        },
        {
            "sop_id": "SOP-ATM-ONUS-01",
            "title": "ATM Cash Dispense Mismatch and Journal Verification SOP v3.0",
            "rail": "ATM",
            "effective_date": "2026-03-01",
            "summary": "ATM no-dispense exceptions require electronic journal (EJ) audit. On-us ATMs require sub-24h remediation upon cash count reconciliation.",
        }
    ]

    @classmethod
    def get_sop_for_rail(cls, rail: str) -> List[Dict[str, Any]]:
        return [s for s in cls._APPROVED_SOPS if s.get("rail", "").upper() == rail.upper()]


class SafeContextBuilder:
    """Builds secure, canonical, deterministic evidence bundles for AI features."""

    @classmethod
    def generate_evidence_id(cls, kind: EvidenceKind, source_system: str, ref_key: str) -> str:
        """Generates a stable, reproducible deterministic evidence ID."""
        clean_key = f"{kind.value}:{source_system}:{ref_key}".encode('utf-8')
        short_hash = hashlib.sha256(clean_key).hexdigest()[:8].upper()
        prefix = kind.value[:3].upper()
        return f"EV-{prefix}-{short_hash}"

    @classmethod
    def build_bundle(
        cls,
        case_data: Dict[str, Any],
        raw_evidence_items: List[Dict[str, Any]],
        caller: CallerContext,
        max_token_budget: Optional[int] = None
    ) -> CanonicalEvidenceBundle:
        """Assembles a canonical evidence bundle respecting tenancy, masking, and security rules."""
        case_tenant = case_data.get("tenant_id", "")
        
        # 1. Tenancy Enforcement: Cross-tenant access is strictly forbidden
        if caller.tenant_id != case_tenant:
            raise PermissionError(
                f"Tenancy violation: Caller tenant '{caller.tenant_id}' cannot access case tenant '{case_tenant}'."
            )

        token_budget = max_token_budget or settings.AI_MAX_INPUT_TOKENS
        bundle_items: List[EvidenceItem] = []
        stale_sources: List[str] = []
        has_untrusted = False

        # 2. Process Core Case / Transaction Record
        txn_ref = case_data.get("transaction_ref", "UNKNOWN_REF")
        case_id = case_data.get("case_id", "UNKNOWN_CASE")
        rail = case_data.get("rail", "NIP")

        # Stable ID for transaction record
        txn_ev_id = cls.generate_evidence_id(EvidenceKind.TRANSACTION, "CORE_BANKING", txn_ref)
        
        # Determine whether caller requires masking
        should_mask = (not caller.can_view_unmasked) or settings.MASK_PII_ENABLED

        txn_content = {
            "transaction_id": case_data.get("transaction_id"),
            "reference_number": txn_ref,
            "session_id": case_data.get("session_id"),
            "amount_minor": case_data.get("amount_minor"),
            "currency": case_data.get("currency", "NGN"),
            "channel": case_data.get("channel"),
            "rail": rail,
            "initiated_at": case_data.get("initiated_at"),
            "debit_status": case_data.get("debit_status"),
            "credit_status": case_data.get("credit_status"),
            "switch_status": case_data.get("switch_status"),
            "settlement_status": case_data.get("settlement_status"),
            "account_number": case_data.get("account_number"),
            "beneficiary_account": case_data.get("beneficiary_account"),
            "narration": case_data.get("narration", ""),
        }

        if should_mask:
            txn_content = masking_service.mask_payload(txn_content)

        # Sanitize narration if present
        if txn_content.get("narration"):
            clean_narration, was_injected, _ = sanitizer.sanitize_untrusted_text(txn_content["narration"])
            txn_content["narration"] = clean_narration
            if was_injected:
                has_untrusted = True

        bundle_items.append(
            EvidenceItem(
                evidence_id=txn_ev_id,
                kind=EvidenceKind.TRANSACTION,
                source_system="CORE_BANKING",
                fact_or_inference=FactOrInference.FACT,
                freshness=FreshnessStatus.FRESH,
                timestamp=_parse_or_now(case_data.get("created_at") or case_data.get("initiated_at")),
                content=txn_content,
                is_redacted=should_mask,
                priority_weight=100
            )
        )

        # 3. Process Ingested Evidence Items
        for raw_item in raw_evidence_items:
            kind_str = raw_item.get("kind", "event")
            try:
                kind = EvidenceKind(kind_str)
            except ValueError:
                kind = EvidenceKind.TRANSACTION

            source_system = raw_item.get("source_system", "UNKNOWN_SOURCE")
            item_ref = raw_item.get("ref", str(raw_item.get("timestamp", "")))
            ev_id = raw_item.get("evidence_id") or cls.generate_evidence_id(kind, source_system, item_ref)
            
            # Fact vs Inference tagging
            foi_str = raw_item.get("fact_or_inference", "FACT").upper()
            fact_or_inference = FactOrInference.FACT
            if foi_str in ["DERIVED", "INFERENCE"]:
                fact_or_inference = FactOrInference.DERIVED
            elif foi_str in ["NOTE", "HUMAN"]:
                fact_or_inference = FactOrInference.NOTE

            # Freshness tracking
            freshness_str = raw_item.get("freshness", "FRESH").upper()
            try:
                freshness = FreshnessStatus(freshness_str)
            except ValueError:
                freshness = FreshnessStatus.UNKNOWN

            if freshness in [FreshnessStatus.STALE, FreshnessStatus.DEGRADED]:
                if source_system not in stale_sources:
                    stale_sources.append(source_system)

            content = raw_item.get("content", {})
            if should_mask:
                content = masking_service.mask_payload(content)

            # Untrusted check for customer alerts or human notes
            is_untrusted = raw_item.get("is_untrusted_input", False) or (kind in [EvidenceKind.CUSTOMER_ALERT, EvidenceKind.ANALYST_NOTE])
            if is_untrusted and isinstance(content, dict):
                for text_key in ["raw_text", "note", "alert_body", "complaint_text"]:
                    if text_key in content and isinstance(content[text_key], str):
                        clean_text, was_injected, _ = sanitizer.sanitize_untrusted_text(content[text_key])
                        content[text_key] = clean_text
                        if was_injected:
                            has_untrusted = True

            priority_weight = raw_item.get("priority_weight", 50)
            if kind in [EvidenceKind.LEDGER_POSTING, EvidenceKind.SWITCH_RESPONSE]:
                priority_weight = 90
            elif kind == EvidenceKind.REVERSAL_RECORD:
                priority_weight = 85

            bundle_items.append(
                EvidenceItem(
                    evidence_id=ev_id,
                    kind=kind,
                    source_system=source_system,
                    fact_or_inference=fact_or_inference,
                    freshness=freshness,
                    timestamp=_parse_or_now(raw_item.get("timestamp")),
                    content=content,
                    raw_payload_hash=raw_item.get("raw_payload_hash"),
                    is_untrusted_input=is_untrusted,
                    is_redacted=should_mask,
                    priority_weight=priority_weight
                )
            )

        # 4. Append Relevant Approved SOP Snippet
        sop_list = ApprovedSOPRegistry.get_sop_for_rail(rail)
        for sop in sop_list:
            sop_id = sop["sop_id"]
            bundle_items.append(
                EvidenceItem(
                    evidence_id=f"EV-SOP-{sop_id}",
                    kind=EvidenceKind.SOP_SNIPPET,
                    source_system="COMPLIANCE_KNOWLEDGE_BASE",
                    fact_or_inference=FactOrInference.FACT,
                    freshness=FreshnessStatus.FRESH,
                    timestamp=_parse_or_now(case_data.get("created_at")),
                    content=sop,
                    is_redacted=False,
                    priority_weight=40
                )
            )

        # 5. Token Budgeting & Priority Sort
        # Sort by priority_weight descending, then timestamp ascending for stable, deterministic ordering
        sorted_items = sorted(
            bundle_items,
            key=lambda x: (-x.priority_weight, x.timestamp.isoformat() if hasattr(x.timestamp, 'isoformat') else str(x.timestamp), x.evidence_id)
        )

        selected_items: List[EvidenceItem] = []
        accumulated_token_estimate = 0
        provided_ids: List[str] = []

        for item in sorted_items:
            item_json = json.dumps(item.content, default=str)
            token_estimate = len(item_json) // 4 + 20
            if accumulated_token_estimate + token_estimate <= token_budget:
                selected_items.append(item)
                provided_ids.append(item.evidence_id)
                accumulated_token_estimate += token_estimate

        # Final sort by chronological order for LLM consumption
        final_items = sorted(
            selected_items,
            key=lambda x: (x.timestamp.isoformat() if hasattr(x.timestamp, 'isoformat') else str(x.timestamp), x.evidence_id)
        )

        return CanonicalEvidenceBundle(
            case_id=case_id,
            tenant_id=case_tenant,
            exception_type=case_data.get("exception_type", "UNKNOWN_TYPE"),
            classification=case_data.get("classification", "UNK001"),
            transaction_ref=txn_ref,
            created_at=_parse_or_now(case_data.get("created_at")),
            items=final_items,
            total_token_estimate=accumulated_token_estimate,
            provided_evidence_ids=provided_ids,
            stale_sources=stale_sources,
            has_untrusted_inputs=has_untrusted
        )


context_builder = SafeContextBuilder()
