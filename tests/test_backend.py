from datetime import datetime, timedelta
import pytest
from fastapi.testclient import TestClient
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import StaticPool

from app.database import Base, get_db
from app.models import Dispute
from app.deadlines import compute_deadline, CHANNEL_REVERSAL_WINDOWS_HOURS
from app.parser import parse_alert
from app.letters import generate_letter
from app.scheduler import check_dispute_deadlines
from app.main import app

# In-memory SQLite engine with StaticPool for consistent test database access
TEST_DATABASE_URL = "sqlite:///:memory:"
engine_test = create_engine(
    TEST_DATABASE_URL,
    connect_args={"check_same_thread": False},
    poolclass=StaticPool,
)
TestingSessionLocal = sessionmaker(autocommit=False, autoflush=False, bind=engine_test)


@pytest.fixture(scope="function")
def db_session():
    Base.metadata.create_all(bind=engine_test)
    db = TestingSessionLocal()
    try:
        yield db
    finally:
        db.close()
        Base.metadata.drop_all(bind=engine_test)


@pytest.fixture(scope="function")
def client(db_session):
    def override_get_db():
        try:
            yield db_session
        finally:
            pass

    app.dependency_overrides[get_db] = override_get_db
    with TestClient(app) as test_client:
        yield test_client
    app.dependency_overrides.clear()


# ================= Function Signatures Verification ================= #

def test_signature_parse_alert():
    sample = "GTBank: Your account has been debited NGN15,000.00 on 13/10/2026 at 09:15 via ATM. Ref: GTB12345."
    res = parse_alert(sample)
    assert isinstance(res, dict)
    assert "amount" in res
    assert res["amount"] == 15000.0
    assert res["channel"] == "atm"
    assert res["bank"] == "GTBank"
    assert res["reference"] == "GTB12345"


def test_signature_compute_deadline():
    now = datetime(2026, 10, 13, 10, 0, 0)
    deadline_atm = compute_deadline("atm", now)
    assert isinstance(deadline_atm, datetime)
    assert deadline_atm == now + timedelta(hours=24)

    deadline_pos = compute_deadline("pos", now)
    assert deadline_pos == now + timedelta(hours=48)

    deadline_transfer = compute_deadline("transfer", now)
    assert deadline_transfer == now + timedelta(hours=24)

    deadline_web = compute_deadline("web", now)
    assert deadline_web == now + timedelta(hours=72)


def test_signature_generate_letter():
    dispute_data = {
        "amount": 25000.0,
        "bank": "Zenith Bank",
        "channel": "pos",
        "reference": "TXN998877",
        "transaction_time": "2026-10-13 14:00:00",
        "deadline": "2026-10-15 14:00:00",
        "status": "deadline_passed",
    }
    bank_letter = generate_letter("bank", dispute_data)
    assert isinstance(bank_letter, str)
    assert "Zenith Bank" in bank_letter
    assert "NGN 25,000.00" in bank_letter
    assert "TXN998877" in bank_letter

    cbn_letter = generate_letter("cbn", dispute_data)
    assert isinstance(cbn_letter, str)
    assert "CENTRAL BANK OF NIGERIA" in cbn_letter
    assert "TXN998877" in cbn_letter


# ================= Bad Input & Resiliency ================= #

def test_parse_alert_bad_input():
    empty_res = parse_alert("")
    assert empty_res["error"] == "could_not_parse"
    assert isinstance(empty_res["missing"], list)

    gibberish_res = parse_alert("Hello world, this is just a regular chat message")
    assert gibberish_res["error"] == "could_not_parse"
    assert "amount" in gibberish_res["missing"]


# ================= API Routes & End-to-End Workflow ================= #

def test_root_route(client):
    response = client.get("/")
    assert response.status_code == 200
    assert response.json() == {"status": "ok", "app": "ReverseIt"}


def test_create_and_list_dispute(client):
    alert_text = "FirstBank Debit Alert: NGN8,500.50 was withdrawn from your account via POS on 2026-10-12 16:30. Reference: FBK123456789."
    
    # 1. POST /disputes
    post_res = client.post("/disputes", json={"text": alert_text})
    assert post_res.status_code == 201
    created = post_res.json()
    assert created["id"] is not None
    assert created["amount"] == 8500.50
    assert created["channel"] == "pos"
    assert created["bank"] == "FirstBank"
    assert created["reference"] == "FBK123456789"
    assert created["status"] == "awaiting_reversal"

    # 2. GET /disputes
    get_res = client.get("/disputes")
    assert get_res.status_code == 200
    disputes = get_res.json()
    assert len(disputes) == 1
    assert disputes[0]["id"] == created["id"]

    # 2b. GET /disputes/{id}
    single_res = client.get(f"/disputes/{created['id']}")
    assert single_res.status_code == 200
    assert single_res.json()["reference"] == "FBK123456789"

    # 3. PATCH /disputes/{id}
    patch_res = client.patch(f"/disputes/{created['id']}", json={"status": "resolved"})
    assert patch_res.status_code == 200
    assert patch_res.json()["status"] == "resolved"

    # 4. POST /disputes/{id}/letter?kind=bank
    letter_res = client.post(f"/disputes/{created['id']}/letter?kind=bank")
    assert letter_res.status_code == 200
    assert "FirstBank" in letter_res.json()["letter"]

    # 5. POST /disputes/{id}/letter?kind=cbn
    cbn_res = client.post(f"/disputes/{created['id']}/letter?kind=cbn")
    assert cbn_res.status_code == 200
    assert "CENTRAL BANK OF NIGERIA" in cbn_res.json()["letter"]


# ================= Scheduler Deadline Transition ================= #

def test_scheduler_flips_deadline_passed(db_session):
    # Create dispute with deadline in the past
    past_deadline = datetime.now() - timedelta(minutes=10)
    dispute = Dispute(
        amount=5000.0,
        channel="atm",
        bank="GTBank",
        reference="PAST1234",
        transaction_time=datetime.now() - timedelta(hours=25),
        deadline=past_deadline,
        status="awaiting_reversal",
    )
    db_session.add(dispute)
    db_session.commit()
    db_session.refresh(dispute)

    # Run deadline check
    result = check_dispute_deadlines(db=db_session)
    assert result["deadline_passed"] >= 1

    # Verify status changed to deadline_passed
    db_session.refresh(dispute)
    assert dispute.status == "deadline_passed"


def test_scheduler_flips_due_soon(db_session):
    # Create dispute with deadline in 30 minutes (within 120 min due_soon window)
    near_deadline = datetime.now() + timedelta(minutes=30)
    dispute = Dispute(
        amount=7000.0,
        channel="transfer",
        bank="AccessBank",
        reference="NEAR1234",
        transaction_time=datetime.now() - timedelta(hours=23, minutes=30),
        deadline=near_deadline,
        status="awaiting_reversal",
    )
    db_session.add(dispute)
    db_session.commit()
    db_session.refresh(dispute)

    # Run deadline check
    result = check_dispute_deadlines(db=db_session, due_soon_minutes=120)
    assert result["due_soon"] >= 1

    # Verify status changed to due_soon
    db_session.refresh(dispute)
    assert dispute.status == "due_soon"
