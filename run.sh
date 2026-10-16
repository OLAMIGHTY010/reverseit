#!/usr/bin/env bash
# SMARTREVERSAL Quick Launcher
# Usage:
#   ./run.sh dev            # Starts all services (API, AI, Web)
#   ./run.sh migrate        # Runs database migrations
#   ./run.sh migrate:status # Shows migration status
#   ./run.sh seed           # Seeds demo data
#   ./run.sh all            # Runs migrations then starts all services

set -e

ACTION="${1:-dev}"

case "$ACTION" in
    dev)
        node dev-all.mjs
        ;;
    all)
        node dev-all.mjs --migrate
        ;;
    migrate)
        node migrate.mjs up
        ;;
    "migrate:down")
        node migrate.mjs down
        ;;
    "migrate:status")
        node migrate.mjs status
        ;;
    seed)
        node migrate.mjs seed
        ;;
    reset)
        node migrate.mjs reset
        ;;
    *)
        echo "Unknown action '$ACTION'. Available: dev, all, migrate, migrate:status, seed, reset"
        exit 1
        ;;
esac
