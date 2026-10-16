# SMARTREVERSAL Quick Launcher
# Usage:
#   .\run.ps1 dev            # Starts all services (API, AI, Web)
#   .\run.ps1 migrate        # Runs database migrations
#   .\run.ps1 migrate:status # Shows migration status
#   .\run.ps1 seed           # Seeds demo data
#   .\run.ps1 all            # Runs migrations then starts all services

param (
    [string]$Action = "dev"
)

$ErrorActionPreference = "Stop"

switch ($Action) {
    "dev" {
        node dev-all.mjs
    }
    "all" {
        node dev-all.mjs --migrate
    }
    "migrate" {
        node migrate.mjs up
    }
    "migrate:down" {
        node migrate.mjs down
    }
    "migrate:status" {
        node migrate.mjs status
    }
    "seed" {
        node migrate.mjs seed
    }
    "reset" {
        node migrate.mjs reset
    }
    default {
        Write-Host "Unknown action '$Action'. Available: dev, all, migrate, migrate:status, seed, reset" -ForegroundColor Red
        exit 1
    }
}
