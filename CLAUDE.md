# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ClinPharm ABDA API - A RESTful Go API providing drug interaction evaluations, adverse drug reaction (ADR) checks, Priscus list evaluations, and pharmaceutical product information using the ABDA database.

## Development Commands

```bash
# Install development tools (air, swag, golangci-lint) and set up .env
just init

# Run API with hot reload (uses air + swagger generation)
just run

# Build Docker image
just deploy-build

# Run Docker container (requires .env file in root)
just deploy-run

# Lint
cd api && golangci-lint run
```

All Go code is in the `/api` directory. Run commands from there or use `just` from root.

## Architecture

### Request Flow
`main.go` → `server/server.go` (creates DB, mailer, ResourceHandle) → `server/routes.go` (registers routes with middleware) → `controller/*controller/` (handles requests)

### Key Components

- **ResourceHandle** (`internal/handle/resourcehandler.go`): Central struct passed to all controllers containing config, DB connections (GORM + sqlx), and mailer
- **Controllers** (`internal/controller/*/`): Each domain has its own controller package. Controllers receive ResourceHandle and define route handlers
- **Middleware** (`internal/middleware/`): JWT authentication and role-based access control
- **Database**: Uses both GORM (for migrations/user tables) and sqlx (for ABDA queries with squirrel query builder)

### Controller Pattern
Controllers follow a consistent pattern:
1. Define query struct with gin binding tags and swag annotations
2. Bind/validate request using `handle.QueryBind()` or `handle.JSONBind()`
3. Execute database queries (usually raw SQL via sqlx + squirrel)
4. Return with `handle.Success()` or `handle.Error()`

### API Features
- JWT authentication with refresh tokens
- Batch endpoints (POST) use concurrent processing with semaphores
- Batch responses use HTTP 207 for partial success scenarios
- Swagger docs auto-generated from code annotations (`swag init`)

## Configuration

- **YAML config**: `api/cfg/default_config.yml` - server settings, DB pool, token expiration, rate limits
- **Environment**: Required vars in `api/cfg/default_env` - MySQL credentials, JWT secret, SendGrid API key
- Config merges YAML settings with environment variables via `envconfig` tags

## Linting

Uses strict golangci-lint config (`.golangci.yaml`) with 50+ linters enabled. Notable requirements:
- Comments must end with periods (godot)
- No global variables (gochecknoglobals)
- No init functions (gochecknoinits)
- Errors from external packages must be wrapped (wrapcheck)
- Row errors from sqlx must be checked (rowserrcheck)
