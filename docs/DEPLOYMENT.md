# Production Cloud Deployment Guide

This guide details how to deploy the **Smart Reversal / ReverseIt** fullstack financial operations platform across industry-standard cloud providers: **Render**, **Vercel**, **Railway**, and **Docker / GitHub Container Registry (GHCR)**.

---

## Architecture Overview

```
                      +-----------------------------+
                      |   Vercel / Render Static    |
                      |   apps/web (React + Vite)   |
                      +--------------+--------------+
                                     |
                                     | HTTPS / API Requests
                                     v
       +-----------------------------+-----------------------------+
       |                                                           |
       v                                                           v
+-------------------------+                               +-------------------------+
|      smartreversal-api  |                               |    smartreversal-ai     |
|   (Go API Server :8080) |                               | (FastAPI Service :8000) |
+------------+------------+                               +------------+------------+
             |                                                         |
             +--------------------+              +---------------------+
                                  |              |
                                  v              v
                     +-----------------------------+
                     |    PostgreSQL 16 Database   |
                     +-----------------------------+
                                  ^
                                  |
                     +-----------------------------+
                     |       Redis 7 Cache         |
                     +-----------------------------+
                                  ^
                                  |
                     +------------+----------------+
                     |    smartreversal-worker     |
                     |   (Go Background Worker)    |
                     +-----------------------------+
```

---

## 1. Automated Render Deployment (Recommended Fullstack)

The repository includes a battle-tested **Render Blueprint** (`render.yaml`). It provisions the entire infrastructure (PostgreSQL, Redis, Go API, Worker, Database Migrations, FastAPI AI Service, and Web Frontend) with a single click.

### Steps:
1. Push your code to your GitHub repository (`main` branch).
2. Log in to [Render Dashboard](https://dashboard.render.com).
3. Navigate to **Blueprints** -> Click **New Blueprint Instance**.
4. Connect the `OLAMIGHTY010/reverseit` (or `smart_reversal`) repository.
5. Render reads `render.yaml` and displays all 7 resources:
   - `smartreversal-db` (Postgres)
   - `smartreversal-redis` (Redis)
   - `smartreversal-migrate` (DB migration runner)
   - `smartreversal-api` (Go API)
   - `smartreversal-worker` (Background Worker)
   - `smartreversal-ai` (Python AI Service)
   - `smartreversal-web` (Web Console)
6. Click **Apply**.
   - Render automatically provisions the database, runs schema migrations, generates cryptographically secure secrets, and links the services together over internal networking.

---

## 2. Vercel Deployment (Frontend Only)

The web console (`apps/web`) is fully configured for Vercel with zero-config root and directory settings (`vercel.json`).

### Steps:
1. Log in to [Vercel](https://vercel.com).
2. Click **Add New Project** -> Import your GitHub repository.
3. Configure Project Settings:
   - **Framework Preset**: Vite
   - **Root Directory**: `./` (or `apps/web`)
   - **Build Command**: `npm run build:web`
   - **Output Directory**: `apps/web/dist`
4. Environment Variables:
   - `VITE_API_BASE_URL`: `https://your-api-url.onrender.com`
   - `VITE_AI_SERVICE_URL`: `https://your-ai-url.onrender.com`
5. Click **Deploy**. Vercel will build the frontend and serve it with global edge CDN caching and SPA client-side routing.

---

## 3. GitHub Container Registry (GHCR) & Docker CI/CD

A production CD workflow (`.github/workflows/deploy.yml`) is included. Whenever changes are merged to `main`, GitHub Actions automatically builds, tags, and pushes multi-stage optimized Docker images to GitHub Container Registry:

- `ghcr.io/olamighty010/smartreversal-backend:latest`
- `ghcr.io/olamighty010/smartreversal-ai:latest`
- `ghcr.io/olamighty010/smartreversal-web:latest`

### Running the Published Containers with Docker Compose:
```bash
# Pull and start the stack
docker compose -f compose.yaml up -d
```

---

## 4. Railway Deployment

A `railway.json` configuration is included at the root of the project.

### Steps:
1. Go to [Railway](https://railway.app).
2. Click **New Project** -> **Deploy from GitHub repo**.
3. Select this repository.
4. Add a **PostgreSQL** plugin and a **Redis** plugin from the Railway template gallery.
5. Set `DATABASE_URL` and `REDIS_URL` in the service variables.

---

## 5. Environment Variables & Security Checklist

| Variable | Required For | Description | Default / Example |
| :--- | :--- | :--- | :--- |
| `DB_HOST` | API, Worker | Postgres database host | `localhost` or Render DB host |
| `DB_PORT` | API, Worker | Postgres database port | `5432` |
| `DB_USER` | API, Worker | Postgres username | `smartreversal_app` |
| `DB_PASSWORD` | API, Worker | Postgres password | Secure random string |
| `DB_NAME` | API, Worker | Postgres database name | `smartreversal_db` |
| `DB_SSLMODE` | API, Worker | SSL mode for Postgres | `require` (production) |
| `REDIS_HOST` | API, Worker | Redis cache host | `redis` or Render Redis host |
| `REDIS_PORT` | API, Worker | Redis cache port | `6379` |
| `JWT_KEY_ID` | API | Active JWT key identifier | `k1` |
| `JWT_PRIVATE_KEY` | API | RSA/ECDSA private key for signing tokens | Generated by `go run ./cmd/keygen` |
| `JWT_PUBLIC_KEYS` | API | JSON map of public keys for verifying tokens | Generated by `go run ./cmd/keygen` |
| `ACCOUNT_TOKEN_KEY` | API | 32-byte secret for blind token indexing | 32-byte hex string |
| `ACCOUNT_ENC_KEY` | API | AES-GCM 32-byte secret for vault encryption | 32-byte hex string |
| `DATABASE_URL` | AI Service | Postgres connection string | `postgresql://user:pass@host:5432/db` |
| `VITE_API_BASE_URL` | Web Frontend | Public URL of deployed Go API | `https://api.yourdomain.com` |
| `VITE_AI_SERVICE_URL`| Web Frontend | Public URL of deployed AI service | `https://ai.yourdomain.com` |

> **Keygen Helper**: To generate new cryptographic JWT and account encryption keys locally:
> ```bash
> cd apps/api && go run ./cmd/keygen
> ```

---

## 6. Health & Readiness Verification

After deployment, verify each service using its readiness endpoints:

- **Go Backend API**:
  `GET https://<api-domain>/v1/ready` -> Expected: `200 OK`
- **FastAPI AI Service**:
  `GET https://<ai-domain>/health` -> Expected: `{"status": "healthy"}`
- **Web Frontend**:
  `GET https://<web-domain>/healthz` (Docker) or `https://<web-domain>/` -> Expected: `200 OK`
