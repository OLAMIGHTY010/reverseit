from fastapi.testclient import TestClient
from app.main import app

client = TestClient(app)


def test_root():
    response = client.get("/")
    assert response.status_code == 200
    assert response.json() == {"status": "ok", "app": "ReverseIt"}


if __name__ == "__main__":
    test_root()
    print("Test passed: GET / returned {'status': 'ok', 'app': 'ReverseIt'}")
