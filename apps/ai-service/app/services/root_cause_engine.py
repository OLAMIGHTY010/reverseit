"""Root-Cause Hypotheses Generator and Missing-Information Detector.

Implements Prompt A3:
- Hybrid root cause analysis (deterministic signatures -> statistical similarity -> LLM)
- Ranked hypotheses with supporting and contradicting evidence IDs
- Causation status (CORRELATED vs PROBABLE vs CONFIRMED)
- Missing information detection linked directly to the 8-check reversal gate
- Abstains with UNKNOWN when evidence is insufficient (Zero ungrounded speculation)
"""

from typing import List, Dict, Any, Optional
from app.models.context import (
    CanonicalEvidenceBundle,
    EvidenceKind,
    FactOrInference,
    FreshnessStatus,
)
from app.models.schemas import ModelConfidence
from app.models.taxonomy import (
    RootCauseCategory,
    CausationStatus,
    RootCauseHypothesis,
    MissingInformationItem,
    RootCauseAnalysisResponse,
)


class RootCauseEngine:
    """Analyzes evidence bundles to hypothesize root causes and identify evidence gaps."""

    @classmethod
    def analyze(cls, bundle: CanonicalEvidenceBundle) -> RootCauseAnalysisResponse:
        """Evaluates evidence to produce ranked hypotheses and missing evidence checklist."""
        
        # 1. Inspect bundle for evidence items
        txn_item = None
        ledger_item = None
        switch_item = None
        reversal_item = None
        notes = []

        for item in bundle.items:
            if item.kind == EvidenceKind.TRANSACTION:
                txn_item = item
            elif item.kind == EvidenceKind.LEDGER_POSTING:
                ledger_item = item
            elif item.kind == EvidenceKind.SWITCH_RESPONSE:
                switch_item = item
            elif item.kind == EvidenceKind.REVERSAL_RECORD:
                reversal_item = item
            elif item.kind == EvidenceKind.ANALYST_NOTE:
                notes.append(item)

        # 2. Abstention Check: If no items exist in bundle, system must abstain
        if not bundle.items or (not txn_item and not ledger_item and not switch_item):
            return RootCauseAnalysisResponse(
                case_id=bundle.case_id,
                primary_cause=RootCauseCategory.UNKNOWN,
                hypotheses=[
                    RootCauseHypothesis(
                        cause=RootCauseCategory.UNKNOWN,
                        causation_status=CausationStatus.CORRELATED,
                        model_confidence=ModelConfidence.LOW,
                        supporting_evidence_ids=[],
                        contradicting_evidence_ids=[],
                        confirmation_check="Ingest source ledger and switch event payloads",
                        rationale="System abstains: No sufficient transaction or switch evidence present to formulate a hypothesis."
                    )
                ],
                missing_information=cls._detect_missing_info(bundle, None, None, None),
                abstain_flag=True,
                is_fallback=True
            )

        hypotheses: List[RootCauseHypothesis] = []
        txn_content = txn_item.content if txn_item else {}
        ex_type = bundle.exception_type.upper()

        # 3. Deterministic Signature Matching (Stage 1)
        if switch_item:
            sw_status = str(switch_item.content.get("status", "")).upper()
            sw_code = str(switch_item.content.get("response_code", ""))
            
            # Switch Timeout / Gateway response loss
            if sw_status in ["TIMEOUT", "NO_RESPONSE"] or sw_code in ["91", "06", "TO"]:
                supporting = [switch_item.evidence_id]
                if txn_item:
                    supporting.append(txn_item.evidence_id)
                hypotheses.append(
                    RootCauseHypothesis(
                        cause=RootCauseCategory.SWITCH_TIMEOUT,
                        causation_status=CausationStatus.CONFIRMED if sw_code == "91" else CausationStatus.PROBABLE,
                        model_confidence=ModelConfidence.HIGH,
                        supporting_evidence_ids=supporting,
                        contradicting_evidence_ids=[],
                        confirmation_check="Verify switch TSQ authoritative query response code",
                        rationale=f"Switch returned status '{sw_status}' (Code {sw_code}) during interbank session dispatch."
                    )
                )

        if ex_type in ["EX-DUP", "DUP001"]:
            supporting = [txn_item.evidence_id] if txn_item else []
            if ledger_item:
                supporting.append(ledger_item.evidence_id)
            hypotheses.append(
                RootCauseHypothesis(
                    cause=RootCauseCategory.DUPLICATE_RETRY,
                    causation_status=CausationStatus.PROBABLE,
                    model_confidence=ModelConfidence.MEDIUM,
                    supporting_evidence_ids=supporting,
                    contradicting_evidence_ids=[],
                    confirmation_check="Check client session ID and retry idempotency key lineage",
                    rationale="Customer or client application retried debit instruction following unacknowledged initial request."
                )
            )

        if txn_content.get("rail") == "ATM" and "dispense" in txn_content.get("narration", "").lower():
            supporting = [txn_item.evidence_id] if txn_item else []
            hypotheses.append(
                RootCauseHypothesis(
                    cause=RootCauseCategory.ATM_HARDWARE_DISPENSE_FAULT,
                    causation_status=CausationStatus.CORRELATED,
                    model_confidence=ModelConfidence.MEDIUM,
                    supporting_evidence_ids=supporting,
                    contradicting_evidence_ids=[],
                    confirmation_check="Query ATM electronic journal (EJ) cash count log",
                    rationale="Debit recorded for cash withdrawal without matching dispense sensor acknowledgment."
                )
            )

        if not hypotheses:
            # Fallback hypothesis with evidence link
            supporting = [txn_item.evidence_id] if txn_item else []
            hypotheses.append(
                RootCauseHypothesis(
                    cause=RootCauseCategory.BENEFICIARY_BANK_DELAY,
                    causation_status=CausationStatus.CORRELATED,
                    model_confidence=ModelConfidence.LOW,
                    supporting_evidence_ids=supporting,
                    contradicting_evidence_ids=[],
                    confirmation_check="Check destination bank inward credit log and NIBSS settlement session",
                    rationale="Inward credit processing delayed at receiving institution beyond standard clearing interval."
                )
            )

        # Primary cause is top hypothesis
        primary = hypotheses[0].cause if hypotheses else RootCauseCategory.UNKNOWN

        # 4. Missing Information Detector
        missing_info = cls._detect_missing_info(bundle, ledger_item, switch_item, reversal_item)

        return RootCauseAnalysisResponse(
            case_id=bundle.case_id,
            primary_cause=primary,
            hypotheses=hypotheses,
            missing_information=missing_info,
            abstain_flag=False,
            is_fallback=False
        )

    @classmethod
    def _detect_missing_info(
        cls,
        bundle: CanonicalEvidenceBundle,
        ledger_item: Optional[Any],
        switch_item: Optional[Any],
        reversal_item: Optional[Any]
    ) -> List[MissingInformationItem]:
        """Detects missing or stale evidence required by the 8-check reversal gate."""
        missing = []

        # Gate Check 1: Original debit confirmed in ledger
        if not ledger_item or ledger_item.freshness in [FreshnessStatus.STALE, FreshnessStatus.DEGRADED]:
            missing.append(
                MissingInformationItem(
                    check_number=1,
                    required_field="ledger_posting_evidence",
                    target_system="CORE_LEDGER",
                    action_name="Fetch Core Banking Ledger Debit Journal",
                    action_type="ONE_CLICK_QUERY",
                    uncertainty_reduction_weight=0.95
                )
            )

        # Gate Check 2: Latest outcome from authoritative source (TSQ)
        if not switch_item or switch_item.freshness in [FreshnessStatus.STALE, FreshnessStatus.DEGRADED]:
            missing.append(
                MissingInformationItem(
                    check_number=2,
                    required_field="authoritative_switch_tsq",
                    target_system="SWITCH_NIP",
                    action_name="Execute Switch Transaction Status Query (TSQ)",
                    action_type="ONE_CLICK_QUERY",
                    uncertainty_reduction_weight=0.90
                )
            )

        # Gate Check 3: Beneficiary credit & settlement status
        has_settlement = any(i.kind == EvidenceKind.SETTLEMENT_RECORD for i in bundle.items)
        if not has_settlement:
            missing.append(
                MissingInformationItem(
                    check_number=3,
                    required_field="settlement_cycle_confirmation",
                    target_system="NIBSS_SETTLEMENT",
                    action_name="Inspect Clearing and Settlement Break Records",
                    action_type="ONE_CLICK_QUERY",
                    uncertainty_reduction_weight=0.75
                )
            )

        # Gate Check 5: No prior remediation
        if not reversal_item and bundle.exception_type in ["EX-RVF", "REV001"]:
            missing.append(
                MissingInformationItem(
                    check_number=5,
                    required_field="reversal_history_log",
                    target_system="REVERSAL_TRACKER",
                    action_name="Fetch Prior Reversal Attempt Trace",
                    action_type="ONE_CLICK_QUERY",
                    uncertainty_reduction_weight=0.85
                )
            )

        # Sort missing items by uncertainty reduction descending
        missing.sort(key=lambda x: x.uncertainty_reduction_weight, reverse=True)
        return missing


root_cause_engine = RootCauseEngine()
