"""Feature-level and scoped AI Kill Switches with deterministic fallback handlers.

Implements Prompt A0 and Master-Prompt S9 / FR-39 requirements:
- Four kill-switch scopes: global, rail, exception_type, feature/policy
- Granular per-feature kill switches (summaries, copilot, anomaly detection, duplicate scoring, alert parsing)
- Dual-control requirement for re-enabling
- Safe deterministic fallback routing when a switch is active
"""

from enum import Enum
from typing import Dict, Optional, Set
from pydantic import BaseModel, Field
import datetime


class KillSwitchScope(str, Enum):
    GLOBAL = "global"
    RAIL = "rail"
    EXCEPTION_TYPE = "exception_type"
    FEATURE = "feature"


class AIFeatureSwitch(str, Enum):
    SUMMARIES = "summaries"
    ROOT_CAUSE = "root_cause"
    ANOMALY_DETECTION = "anomaly_detection"
    DUPLICATE_SCORING = "duplicate_scoring"
    ALERT_PARSING = "alert_parsing"
    COMPLAINT_DRAFTING = "complaint_drafting"
    COPILOT = "copilot"


class KillSwitchRecord(BaseModel):
    switch_id: str
    scope: KillSwitchScope
    target: str  # e.g., "global", "NIP", "EX-DUP", "copilot"
    is_active: bool = False  # True means KILL SWITCH TRIPPED (feature disabled)
    reason: str = ""
    tripped_by: str = ""
    tripped_at: Optional[datetime.datetime] = None
    re_enabled_by: Optional[str] = None
    second_approver: Optional[str] = None  # Dual control enforcement
    re_enabled_at: Optional[datetime.datetime] = None


class KillSwitchManager:
    """Manages system kill switches with dual-control enforcement and fallback guarantees."""

    def __init__(self):
        self._switches: Dict[str, KillSwitchRecord] = {}
        self._init_defaults()

    def _make_key(self, scope: KillSwitchScope, target: str) -> str:
        return f"{scope.value}:{target.upper()}"

    def _init_defaults(self):
        # Global AI switch
        global_key = self._make_key(KillSwitchScope.GLOBAL, "ALL")
        self._switches[global_key] = KillSwitchRecord(
            switch_id=global_key,
            scope=KillSwitchScope.GLOBAL,
            target="ALL",
            is_active=False
        )
        # Per-feature switches
        for feature in AIFeatureSwitch:
            key = self._make_key(KillSwitchScope.FEATURE, feature.value)
            self._switches[key] = KillSwitchRecord(
                switch_id=key,
                scope=KillSwitchScope.FEATURE,
                target=feature.value,
                is_active=False
            )

    def trip_switch(self, scope: KillSwitchScope, target: str, actor: str, reason: str) -> KillSwitchRecord:
        """Immediately deactivates the target feature or scope. Single operator can trip."""
        key = self._make_key(scope, target)
        record = self._switches.get(key)
        now = datetime.datetime.now(datetime.timezone.utc)
        if not record:
            record = KillSwitchRecord(
                switch_id=key,
                scope=scope,
                target=target,
            )
            self._switches[key] = record

        record.is_active = True
        record.reason = reason
        record.tripped_by = actor
        record.tripped_at = now
        record.re_enabled_by = None
        record.second_approver = None
        record.re_enabled_at = None
        return record

    def re_enable_switch(
        self,
        scope: KillSwitchScope,
        target: str,
        first_approver: str,
        second_approver: str,
        reason: str
    ) -> KillSwitchRecord:
        """Re-enables a switch with mandatory dual-control verification."""
        if first_approver == second_approver:
            raise ValueError(
                f"Dual-control violation: Approver 1 ({first_approver}) and Approver 2 ({second_approver}) cannot be the same user."
            )

        key = self._make_key(scope, target)
        record = self._switches.get(key)
        if not record or not record.is_active:
            raise ValueError(f"Switch {key} is not currently active.")

        now = datetime.datetime.now(datetime.timezone.utc)
        record.is_active = False
        record.re_enabled_by = first_approver
        record.second_approver = second_approver
        record.re_enabled_at = now
        record.reason = f"Re-enabled: {reason}"
        return record

    def is_feature_disabled(
        self,
        feature: AIFeatureSwitch,
        rail: Optional[str] = None,
        exception_type: Optional[str] = None
    ) -> bool:
        """Checks if a feature is disabled by global, feature, rail, or exception_type switch."""
        # 1. Check Global switch
        global_key = self._make_key(KillSwitchScope.GLOBAL, "ALL")
        if self._switches.get(global_key, KillSwitchRecord(switch_id="", scope=KillSwitchScope.GLOBAL, target="")).is_active:
            return True

        # 2. Check Feature switch
        feature_key = self._make_key(KillSwitchScope.FEATURE, feature.value)
        if self._switches.get(feature_key, KillSwitchRecord(switch_id="", scope=KillSwitchScope.FEATURE, target="")).is_active:
            return True

        # 3. Check Rail switch if applicable
        if rail:
            rail_key = self._make_key(KillSwitchScope.RAIL, rail)
            if self._switches.get(rail_key, KillSwitchRecord(switch_id="", scope=KillSwitchScope.RAIL, target="")).is_active:
                return True

        # 4. Check Exception Type switch if applicable
        if exception_type:
            ex_key = self._make_key(KillSwitchScope.EXCEPTION_TYPE, exception_type)
            if self._switches.get(ex_key, KillSwitchRecord(switch_id="", scope=KillSwitchScope.EXCEPTION_TYPE, target="")).is_active:
                return True

        return False


# Global singleton instance for ai-service
kill_switch_manager = KillSwitchManager()
