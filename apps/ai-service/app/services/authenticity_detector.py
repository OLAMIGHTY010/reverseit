"""Advisory Alert Authenticity and Tamper Detection Service.

Implements Prompt A9 (Item 6) & Case N1:
- Advisory checks for impossible timestamps (future dates, ancient dates)
- Checks for inconsistent currency/amount syntax
- Flags repeated reporting attempts on identical reference
- Advisory signals only: raises neutral review flags, NEVER blocks customer and NEVER triggers money movement
"""

import datetime
from typing import Optional, List, Dict, Any
from app.models.reverseit import AuthenticitySignalResult


class AdvisoryAuthenticityDetector:
    """Evaluates advisory tamper indicators on customer alerts."""

    @classmethod
    def check_authenticity(
        cls,
        parsed_amount_minor: Optional[int],
        parsed_datetime: Optional[datetime.datetime],
        reference: Optional[str],
        prior_reports_count: int = 0
    ) -> AuthenticitySignalResult:
        """Runs advisory tamper checks. Outputs neutral warning flags."""
        flags: List[str] = []
        now = datetime.datetime.now(datetime.timezone.utc)

        # 1. Timestamp checks
        if parsed_datetime:
            # Check if timestamp is in the future (> 2 hours ahead to account for small clock skew)
            dt_utc = parsed_datetime.replace(tzinfo=datetime.timezone.utc) if parsed_datetime.tzinfo is None else parsed_datetime
            if dt_utc > now + datetime.timedelta(hours=2):
                flags.append("TIMESTAMP_IN_FUTURE")

            # Check if older than 180 days
            if dt_utc < now - datetime.timedelta(days=180):
                flags.append("TRANSACTION_OLDER_THAN_180_DAYS")

        # 2. Suspicious amount checks
        if parsed_amount_minor is not None:
            if parsed_amount_minor <= 0:
                flags.append("ZERO_OR_NEGATIVE_AMOUNT")
            elif parsed_amount_minor > 500_000_000:  # > 5 million NGN in minor units (500m kobo = N50m)
                flags.append("HIGH_VALUE_THRESHOLD_EXCEEDED")

        # 3. Repeated dispute attempt check (Case N14)
        if prior_reports_count > 0:
            flags.append(f"REPEAT_REPORT_COUNT_{prior_reports_count}")

        # Determine severity
        is_suspicious = len(flags) > 0
        severity = "INFO"
        if "TIMESTAMP_IN_FUTURE" in flags or prior_reports_count >= 3:
            severity = "WARNING"

        advisory_notes = ""
        if flags:
            advisory_notes = f"Advisory review flags noted: {', '.join(flags)}. Transaction verification against ledger required."

        return AuthenticitySignalResult(
            is_suspicious=is_suspicious,
            tamper_flags=flags,
            advisory_notes=advisory_notes,
            severity=severity
        )


authenticity_detector = AdvisoryAuthenticityDetector()
