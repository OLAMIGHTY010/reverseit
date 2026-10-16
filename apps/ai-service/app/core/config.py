"""Settings and environment configuration for SMARTREVERSAL AI Service.

Configures AI Gateway parameters, model registry, prompt templates,
security/PII masking parameters, and integration fallbacks.
"""

import os
from typing import List, Optional

try:
    from pydantic_settings import BaseSettings
except ImportError:
    from pydantic import BaseModel

    class BaseSettings(BaseModel):
        """Fallback BaseSettings using os.environ when pydantic_settings is not installed."""
        def __init__(self, **values):
            env_values = {}
            for field_name in self.model_fields.keys():
                val = os.getenv(field_name)
                if val is not None:
                    # Basic type conversion for bool/int/float
                    field_type = self.model_fields[field_name].annotation
                    if field_type is bool:
                        env_values[field_name] = val.lower() in ("true", "1", "yes")
                    elif field_type is int:
                        try:
                            env_values[field_name] = int(val)
                        except ValueError:
                            pass
                    elif field_type is float:
                        try:
                            env_values[field_name] = float(val)
                        except ValueError:
                            pass
                    else:
                        env_values[field_name] = val
            env_values.update(values)
            super().__init__(**env_values)



class Settings(BaseSettings):
    ENVIRONMENT: str = "development"
    PORT: int = 8000
    HOST: str = "0.0.0.0"
    DATABASE_URL: str = ""
    REDIS_URL: str = "redis://localhost:6379/0"

    # AI Gateway Provider Configuration
    AI_PROVIDER: str = "mock"  # "mock", "openai", "anthropic", "ollama", "azure"
    AI_MODEL_NAME: str = "gpt-4o-mini"
    AI_API_KEY: Optional[str] = None
    AI_API_BASE: Optional[str] = None
    AI_REQUEST_TIMEOUT_SECONDS: float = 10.0
    AI_MAX_RETRIES: int = 2

    # Token & Cost Governance
    AI_MAX_INPUT_TOKENS: int = 4096
    AI_MAX_OUTPUT_TOKENS: int = 1024
    AI_RATE_LIMIT_PER_MINUTE: int = 60

    # PII & Data Protection Controls
    MASK_PII_ENABLED: bool = True
    ALLOW_EXTERNAL_AI_CALLS: bool = False  # Set to True only with compliance/counsel sign-off
    REDACT_BALANCES: bool = True
    REDACT_ACCOUNT_NUMBERS: bool = True
    REDACT_CUSTOMER_NAMES: bool = True

    # Feature Kill Switches (Defaults: True = Enabled)
    ENABLE_AI_SUMMARIES: bool = True
    ENABLE_AI_ROOT_CAUSE: bool = True
    ENABLE_AI_ANOMALY_DETECTION: bool = True
    ENABLE_AI_DUPLICATE_SCORING: bool = True
    ENABLE_AI_COMPLAINT_DRAFTING: bool = True
    ENABLE_AI_ALERT_PARSING: bool = True
    ENABLE_AI_COPILOT: bool = True

    # Deterministic Baseline Enforcements
    STRICT_CITATION_ENFORCEMENT: bool = True
    REJECT_UNCITED_CLAIMS: bool = True

    class Config:
        env_file = ".env"
        extra = "ignore"


settings = Settings()

