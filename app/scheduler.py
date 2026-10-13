"""Dispute deadline monitor scheduler.

Periodically inspects active disputes and updates their status:
- 'due_soon' when nearing the regulatory deadline
- 'deadline_passed' once deadline has elapsed without resolution
"""

import logging
from datetime import datetime
from apscheduler.schedulers.background import BackgroundScheduler
from sqlalchemy.orm import Session
from app.database import SessionLocal
from app.models import Dispute

logger = logging.getLogger(__name__)

# Shared scheduler instance
scheduler: BackgroundScheduler | None = None


def check_dispute_deadlines(db: Session | None = None, due_soon_minutes: int = 120) -> dict:
    """
    Evaluate pending disputes against current time and update statuses.

    Args:
        db: Optional SQLAlchemy Session. If None, a temporary session is created.
        due_soon_minutes: Threshold in minutes before deadline to mark as 'due_soon' (default 2 hrs).

    Returns:
        Summary dict of status transitions.
    """
    should_close = False
    if db is None:
        db = SessionLocal()
        should_close = True

    try:
        now = datetime.now()
        active_disputes = (
            db.query(Dispute)
            .filter(Dispute.status.in_(["awaiting_reversal", "due_soon"]))
            .all()
        )

        passed_count = 0
        due_soon_count = 0

        for dispute in active_disputes:
            if not dispute.deadline:
                continue

            # Compare naive datetimes
            deadline = dispute.deadline
            if deadline.tzinfo is not None:
                deadline = deadline.replace(tzinfo=None)

            if now >= deadline:
                if dispute.status != "deadline_passed":
                    dispute.status = "deadline_passed"
                    passed_count += 1
            else:
                remaining_seconds = (deadline - now).total_seconds()
                if remaining_seconds <= (due_soon_minutes * 60) and dispute.status == "awaiting_reversal":
                    dispute.status = "due_soon"
                    due_soon_count += 1

        if passed_count > 0 or due_soon_count > 0:
            db.commit()

        return {
            "evaluated": len(active_disputes),
            "deadline_passed": passed_count,
            "due_soon": due_soon_count,
        }
    except Exception as exc:
        db.rollback()
        logger.error(f"Error checking dispute deadlines: {exc}")
        return {"error": str(exc)}
    finally:
        if should_close:
            db.close()


def _scheduled_job():
    """Worker task run periodically by APScheduler."""
    result = check_dispute_deadlines()
    if result.get("deadline_passed", 0) > 0 or result.get("due_soon", 0) > 0:
        logger.info(f"Dispute status update cycle: {result}")


def start_scheduler(interval_minutes: int = 2) -> BackgroundScheduler:
    """Initialize and start background dispute deadline monitoring."""
    global scheduler
    if scheduler is None or not scheduler.running:
        scheduler = BackgroundScheduler(daemon=True)
        scheduler.add_job(
            _scheduled_job,
            "interval",
            minutes=interval_minutes,
            id="check_dispute_deadlines",
            replace_existing=True,
        )
        scheduler.start()
        logger.info(f"Dispute deadline scheduler started (interval: {interval_minutes}m)")
    return scheduler


def shutdown_scheduler():
    """Gracefully stop background scheduler."""
    global scheduler
    if scheduler and scheduler.running:
        scheduler.shutdown(wait=False)
        logger.info("Dispute deadline scheduler stopped")
