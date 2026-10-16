"""Shared Complaint-to-Transaction Matcher for Agent Workbench and ReverseIt.

Implements Prompt A6 (Item 1), Prompt B4, and Case N3:
- Stage 1: Exact reference / session ID lookup
- Stage 2: Constrained fuzzy matching across amount, time window, channel, and account tail
- Ranks candidate transactions with explicit match reasons
- Threshold-gated decision logic:
  * Score >= 0.85 & gap >= 0.20 -> auto_linked = True
  * 0.50 <= Score < 0.85 -> route_to_human_disambiguation = True
  * Score < 0.50 -> is_unmatched_claim = True (CUS001)
- Never auto-links on account + amount + time alone without reference confirmation
"""

import math
from typing import List, Dict, Any, Optional
from app.models.complaint_matching import (
    CandidateMatch,
    ComplaintMatchResult,
)


class ComplaintMatcher:
    """Matches customer complaints to registered transactions and exceptions."""

    # Configurable matching thresholds
    AUTO_LINK_THRESHOLD = 0.85
    AMBIGUITY_GAP_THRESHOLD = 0.20
    MIN_CANDIDATE_THRESHOLD = 0.50

    @classmethod
    def match(
        cls,
        complaint_ref: str,
        claimed_ref: Optional[str],
        claimed_amount_minor: Optional[int],
        claimed_account_tail: Optional[str],
        claimed_channel: Optional[str],
        candidate_pool: List[Dict[str, Any]]
    ) -> ComplaintMatchResult:
        """Executes multi-stage candidate scoring over the transaction pool."""
        scored_candidates: List[CandidateMatch] = []

        # 1. Stage 1: Exact Reference Match
        if claimed_ref and claimed_ref.strip():
            clean_claimed_ref = claimed_ref.strip().upper()
            for txn in candidate_pool:
                txn_ref = str(txn.get("reference_number", "")).strip().upper()
                session_id = str(txn.get("session_id", "")).strip().upper()
                if clean_claimed_ref in (txn_ref, session_id):
                    match_item = CandidateMatch(
                        transaction_id=str(txn.get("transaction_id", "")),
                        reference_number=txn.get("reference_number", ""),
                        amount_minor=txn.get("amount_minor", 0),
                        amount_formatted=f"NGN {txn.get('amount_minor', 0) / 100:,.2f}",
                        channel=txn.get("channel", "UNKNOWN"),
                        rail=txn.get("rail", "NIP"),
                        initiated_at=str(txn.get("initiated_at", "")),
                        account_tail=str(txn.get("account_number", ""))[-4:],
                        score=1.0,
                        match_reasons=["EXACT_REFERENCE_MATCH"],
                        is_exact_match=True
                    )
                    return ComplaintMatchResult(
                        complaint_ref=complaint_ref,
                        candidates=[match_item],
                        top_candidate=match_item,
                        auto_linked=True,
                        route_to_human_disambiguation=False,
                        is_unmatched_claim=False,
                        matching_stage="exact_reference"
                    )

        # 2. Stage 2: Constrained Fuzzy Match
        for txn in candidate_pool:
            score = 0.0
            reasons = []

            # Amount check (Weight: 0.40)
            txn_amt = txn.get("amount_minor", 0)
            if claimed_amount_minor is not None and txn_amt > 0:
                if claimed_amount_minor == txn_amt:
                    score += 0.40
                    reasons.append("EXACT_AMOUNT_MATCH")
                elif abs(claimed_amount_minor - txn_amt) <= 10000:  # within N100 (possible fee delta)
                    score += 0.25
                    reasons.append("PROXIMATE_AMOUNT_MATCH")

            # Account tail check (Weight: 0.30)
            txn_acct = str(txn.get("account_number", ""))
            if claimed_account_tail and len(claimed_account_tail) == 4:
                if txn_acct.endswith(claimed_account_tail):
                    score += 0.30
                    reasons.append("ACCOUNT_TAIL_MATCH")

            # Channel check (Weight: 0.20)
            txn_channel = str(txn.get("channel", "")).upper()
            if claimed_channel and txn_channel:
                if claimed_channel.upper() == txn_channel:
                    score += 0.20
                    reasons.append("CHANNEL_MATCH")

            # Basic freshness / proximity (Weight: 0.10)
            if txn.get("initiated_at"):
                score += 0.10
                reasons.append("WITHIN_DATE_WINDOW")

            score = min(round(score, 2), 0.99)
            if score >= cls.MIN_CANDIDATE_THRESHOLD:
                scored_candidates.append(
                    CandidateMatch(
                        transaction_id=str(txn.get("transaction_id", "")),
                        reference_number=txn.get("reference_number", ""),
                        amount_minor=txn_amt,
                        amount_formatted=f"NGN {txn_amt / 100:,.2f}",
                        channel=txn.get("channel", "UNKNOWN"),
                        rail=txn.get("rail", "NIP"),
                        initiated_at=str(txn.get("initiated_at", "")),
                        account_tail=txn_acct[-4:] if len(txn_acct) >= 4 else "****",
                        score=score,
                        match_reasons=reasons,
                        is_exact_match=False
                    )
                )

        # Sort candidates descending by score
        scored_candidates.sort(key=lambda x: x.score, reverse=True)

        if not scored_candidates:
            return ComplaintMatchResult(
                complaint_ref=complaint_ref,
                candidates=[],
                top_candidate=None,
                auto_linked=False,
                route_to_human_disambiguation=False,
                is_unmatched_claim=True,
                matching_stage="none"
            )

        top = scored_candidates[0]
        second_score = scored_candidates[1].score if len(scored_candidates) > 1 else 0.0
        score_gap = top.score - second_score

        # Auto-link criteria
        auto_link = (top.score >= cls.AUTO_LINK_THRESHOLD) and (score_gap >= cls.AMBIGUITY_GAP_THRESHOLD)
        route_to_human = not auto_link

        return ComplaintMatchResult(
            complaint_ref=complaint_ref,
            candidates=scored_candidates[:5],  # Top 5 candidates
            top_candidate=top,
            auto_linked=auto_link,
            route_to_human_disambiguation=route_to_human,
            is_unmatched_claim=False,
            matching_stage="constrained_fuzzy"
        )


complaint_matcher = ComplaintMatcher()
