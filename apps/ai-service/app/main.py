"""FastAPI application entrypoint for SMARTREVERSAL AI Service."""

from fastapi import FastAPI
from app.api.v1.endpoints.summary import router as summary_router
from app.api.v1.endpoints.reverseit import router as reverseit_router
from app.api.v1.endpoints.complaints import router as complaints_router
from app.api.v1.endpoints.root_cause import router as root_cause_router
from app.api.v1.endpoints.anomaly import router as anomaly_router
from app.api.v1.endpoints.predictions import router as predictions_router
from app.api.v1.endpoints.copilot import router as copilot_router
from app.api.v1.endpoints.evaluation import router as evaluation_router

app = FastAPI(
    title="SMARTREVERSAL AI Intelligence Service",
    description="Enterprise operations intelligence, anomaly scoring, and explainable dispute resolution.",
    version="0.1.0",
)

# Register endpoint routers
app.include_router(summary_router, prefix="/v1/ai")
app.include_router(reverseit_router, prefix="/v1/ai")
app.include_router(complaints_router, prefix="/v1/ai")
app.include_router(root_cause_router, prefix="/v1/ai")
app.include_router(anomaly_router, prefix="/v1/ai")
app.include_router(predictions_router, prefix="/v1/ai")
app.include_router(copilot_router, prefix="/v1/ai")
app.include_router(evaluation_router, prefix="/v1/ai")






@app.get("/health")
@app.get("/v1/ai/health")
def health_check():
    return {"status": "operational", "service": "ai-service"}

