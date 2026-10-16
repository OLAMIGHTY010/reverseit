"""Root-cause analysis and remediation recommendations endpoints."""

from fastapi import APIRouter

router = APIRouter(prefix="/recommendations", tags=["recommendations"])
