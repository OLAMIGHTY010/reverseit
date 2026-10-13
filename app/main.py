"""ReverseIt - Dispute Resolution & Deadline Management API.

Routes are kept thin, delegating core logic to specialized modules.
"""

from collections.abc import AsyncGenerator
from contextlib import asynccontextmanager
from datetime import datetime
from fastapi import FastAPI, Depends, HTTPException, Query, status
from pydantic import BaseModel, Field
from sqlalchemy.orm import Session

from app.database import Base, engine, get_db
from app.models import Dispute, VALID_STATUSES
from app.parser import parse_alert
from app.deadlines import compute_deadline
from app.letters import generate_letter
from app.scheduler import start_scheduler, shutdown_scheduler


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncGenerator[None, None]:
    # Initialize SQLite database tables
    Base.metadata.create_all(bind=engine)
    # Start periodic deadline monitoring
    start_scheduler()
    yield
    # Cleanup scheduler
    shutdown_scheduler()


app = FastAPI(title="ReverseIt", lifespan=lifespan)


# ------------------ Request / Response Schemas ------------------ #

class AlertInput(BaseModel):
    text: str = Field(..., description="Pasted debit alert text")


class UpdateStatusInput(BaseModel):
    status: str = Field(..., description="New dispute status")


# ------------------ Routes ------------------ #

@app.get("/")
def root():
    return {"status": "ok", "app": "ReverseIt"}


@app.post("/disputes", status_code=status.HTTP_201_CREATED)
def create_dispute(payload: AlertInput, db: Session = Depends(get_db)):
    """POST /disputes: Paste alert text; parse, compute deadline, save dispute record."""
    parsed = parse_alert(payload.text)
    if "error" in parsed:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=parsed,
        )

    # Convert extracted date & time into transaction_time datetime
    try:
        txn_time = datetime.strptime(f"{parsed['date']} {parsed['time']}", "%Y-%m-%d %H:%M")
    except (ValueError, KeyError):
        txn_time = datetime.now()

    deadline = compute_deadline(parsed["channel"], txn_time)

    dispute = Dispute(
        amount=parsed["amount"],
        channel=parsed["channel"],
        bank=parsed["bank"],
        reference=parsed["reference"],
        transaction_time=txn_time,
        deadline=deadline,
        status="awaiting_reversal",
        raw_alert=payload.text,
    )
    db.add(dispute)
    db.commit()
    db.refresh(dispute)
    return dispute.to_dict()


@app.get("/disputes")
def list_disputes(status_filter: str | None = Query(None, alias="status"), db: Session = Depends(get_db)):
    """GET /disputes: Retrieve list of disputes, optionally filtered by status."""
    query = db.query(Dispute)
    if status_filter:
        query = query.filter(Dispute.status == status_filter)
    disputes = query.order_by(Dispute.id.desc()).all()
    return [d.to_dict() for d in disputes]


@app.patch("/disputes/{dispute_id}")
def update_dispute_status(dispute_id: int, payload: UpdateStatusInput, db: Session = Depends(get_db)):
    """PATCH /disputes/{id}: Update dispute status."""
    if payload.status not in VALID_STATUSES:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail=f"Invalid status '{payload.status}'. Allowed: {list(VALID_STATUSES)}",
        )

    dispute = db.query(Dispute).filter(Dispute.id == dispute_id).first()
    if not dispute:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Dispute not found")

    dispute.status = payload.status
    db.commit()
    db.refresh(dispute)
    return dispute.to_dict()


@app.post("/disputes/{dispute_id}/letter")
def get_complaint_letter(
    dispute_id: int,
    kind: str = Query("bank", pattern="^(bank|cbn)$", description="Type of letter: 'bank' or 'cbn'"),
    db: Session = Depends(get_db),
):
    """POST /disputes/{id}/letter?kind=bank|cbn: Generate bank or CBN complaint letter."""
    dispute = db.query(Dispute).filter(Dispute.id == dispute_id).first()
    if not dispute:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="Dispute not found")

    letter_text = generate_letter(kind=kind, dispute=dispute)
    return {
        "dispute_id": dispute.id,
        "kind": kind,
        "letter": letter_text,
    }


if __name__ == "__main__":
    import uvicorn
    uvicorn.run("app.main:app", host="0.0.0.0", port=8000, reload=True)
