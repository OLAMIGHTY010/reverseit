from datetime import datetime, timezone
from sqlalchemy import Column, Integer, Float, String, DateTime
from app.database import Base

VALID_STATUSES = (
    "awaiting_reversal",
    "due_soon",
    "deadline_passed",
    "resolved",
)


class Dispute(Base):
    __tablename__ = "disputes"

    id = Column(Integer, primary_key=True, index=True, autoincrement=True)
    amount = Column(Float, nullable=False)
    channel = Column(String(50), nullable=False)
    bank = Column(String(100), nullable=False)
    reference = Column(String(100), nullable=False, index=True)
    transaction_time = Column(DateTime, nullable=False)
    deadline = Column(DateTime, nullable=False, index=True)
    status = Column(String(50), nullable=False, default="awaiting_reversal", index=True)
    raw_alert = Column(String, nullable=True)
    created_at = Column(DateTime, default=lambda: datetime.now(timezone.utc), nullable=False)

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "amount": self.amount,
            "channel": self.channel,
            "bank": self.bank,
            "reference": self.reference,
            "transaction_time": self.transaction_time.isoformat() if self.transaction_time else None,
            "deadline": self.deadline.isoformat() if self.deadline else None,
            "status": self.status,
            "created_at": self.created_at.isoformat() if self.created_at else None,
        }
