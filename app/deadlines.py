"""Deadline calculation module based strictly on regulatory rules."""

from datetime import datetime, timedelta

# Reversal windows per channel defined in ONE centralized config dictionary.
# Values represent maximum resolution window in hours according to CBN guidelines.
# Deadlines are computed by RULES ONLY, never by the LLM.
CHANNEL_REVERSAL_WINDOWS_HOURS: dict[str, int] = {
    "atm": 24,       # CBN Regulation: Failed ATM transactions resolution within 24 hours (instant for on-us, 24h max for not-on-us)
    "pos": 48,       # CBN Regulation: Point-of-Sale (POS) terminal disputes turnaround within 48 hours
    "transfer": 24,  # CBN Regulation: Instant bank transfers (NIP/inter-bank) within 24 hours
    "web": 72,       # TODO: verify - Web/online merchant card disputes window typically 48-72 hours
}


def compute_deadline(channel: str, transaction_time: datetime) -> datetime:
    """
    Computes resolution deadline based on channel rules.

    Signature: compute_deadline(channel, transaction_time) -> datetime
    """
    if not isinstance(transaction_time, datetime):
        raise TypeError("transaction_time must be an instance of datetime")

    normalized_channel = (channel or "").strip().lower()
    # If channel is not explicitly recognized, fallback to 24h default with verification flag
    hours = CHANNEL_REVERSAL_WINDOWS_HOURS.get(normalized_channel, 24)  # TODO: verify fallback duration
    return transaction_time + timedelta(hours=hours)
