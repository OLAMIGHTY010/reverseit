from fastapi.testclient import TestClient
from app.main import app

client = TestClient(app)


def test_read_root():
    response = client.get("/")
    assert response.status_code == 200
    assert response.json() == "ok"


if __name__ == "__main__":
    test_read_root()
    print("Test passed: GET / returned 'ok'")

