# ==============================================================================
# SMARTREVERSAL PLATFORM MAKEFILE
# ==============================================================================

.PHONY: help dev build test lint clean docker-up docker-down db-migrate db-migrate-status db-migrate-down dev-worker test-api-integration

help: ## Display available commands
	@echo "SMARTREVERSAL Enterprise Financial Operations Platform"
	@echo "======================================================="
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ------------------------------------------------------------------------------
# Local Development & Orchestration
# ------------------------------------------------------------------------------
docker-up: ## Start core infrastructure and services with Docker Compose
	docker compose up -d

docker-down: ## Stop all Docker Compose containers
	docker compose down

docker-logs: ## Follow Docker Compose logs
	docker compose logs -f

dev-api: ## Run the Go backend API locally
	cd apps/api && go run ./cmd/api

dev-worker: ## Run the Go background worker locally
	cd apps/api && go run ./cmd/worker

db-migrate: ## Apply database migrations (needs DB_* variables)
	cd apps/api && go run ./cmd/migrate up

db-migrate-status: ## Show migration status
	cd apps/api && go run ./cmd/migrate status

db-migrate-down: ## Revert the most recent migration
	cd apps/api && go run ./cmd/migrate down 1

dev-ai: ## Run the Python AI service locally
	cd apps/ai-service && uvicorn app.main:app --reload --port 8000

dev-web: ## Run the Vite web operations console locally
	npm run dev --workspace=apps/web

dev-all: ## Run all three tiers locally (Go API + Python AI + Vite Web)
	node scripts/dev-all.mjs

# ------------------------------------------------------------------------------
# Build & Compilation
# ------------------------------------------------------------------------------
build: build-api build-web ## Build all production assets

build-api: ## Compile all Go binaries into apps/api/bin
	cd apps/api && go build -v -o bin/ ./cmd/...

build-web: ## Build Vite frontend production bundle
	npm run build --workspace=apps/web

# ------------------------------------------------------------------------------
# Quality & Verification
# ------------------------------------------------------------------------------
test: test-api test-ai test-web ## Run unit tests across all applications

test-api: ## Run Go tests (integration tests skip unless TEST_DATABASE_URL and TEST_REDIS_ADDR are set)
	cd apps/api && go test -v -race ./...

test-api-integration: ## Run Go tests against local Postgres and Redis (see docs/backend/local-setup.md)
	cd apps/api && go test -p 1 -race -count=1 ./...

test-ai: ## Run Python AI service tests
	cd apps/ai-service && pytest

test-web: ## Run web tests
	npm run test --workspace=apps/web --if-present

lint: lint-api lint-web ## Run linters across projects

lint-api: ## Check formatting and run go vet
	cd apps/api && test -z "$$(gofmt -l .)" && go vet ./...

lint-web: ## Run ESLint for web and UI packages
	npm run lint --workspaces --if-present

clean: ## Clean build artifacts and temporary files
	rm -rf apps/api/bin apps/web/dist packages/ui/dist
