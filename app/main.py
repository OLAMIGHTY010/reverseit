from fastapi import FastAPI

app = FastAPI(title="ReverseIt")


@app.get("/")
def root():
    return {"status": "ok", "app": "ReverseIt"}
