# SMARTREVERSAL AI & ML Intelligence Service

FastAPI-based intelligence engine providing anomaly scoring, root-cause pattern clustering, and explainable remediation recommendations.

## Structure
- `app/main.py`: FastAPI server entrypoint
- `app/core/config.py`: Environment and service configuration
- `app/models/schemas.py`: Pydantic request and response schemas
- `app/services/ml_engine.py`: Machine learning and heuristic engine
- `app/api/v1/endpoints`: REST endpoint handlers
  - `anomaly.py`: Transaction anomaly detection
  - `recommendations.py`: Root-cause analysis and reversal recommendations
- `tests`: Test suites
