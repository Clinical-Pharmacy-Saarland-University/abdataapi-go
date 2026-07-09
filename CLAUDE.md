# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ClinPharm ABDA API - A RESTful Go API providing drug-drug interaction evaluations, adverse drug reaction (ADR) checks, Priscus list and QT-prolongation evaluations, and pharmaceutical product information/search using the ABDA database.

## Development Commands

```bash
# Install development tools (air, swag, gotestsum, golangci-lint) and set up .env
just init

# Run API with hot reload (uses air + swagger generation)
just run

# Unit tests (mocked DB via go-sqlmock; no database needed)
just test            # or: cd api && go test ./...

# Integration tests (real ABDA DB; needs api/.env.integration or ABDA_TEST_* env vars)
just test-integration   # or: cd api && go test -tags=integration ./...

# Build/run the container image (Podman; requires .env in root)
just deploy-build
just deploy-run

# Lint
cd api && golangci-lint run
```

All Go code is in the `/api` directory. Run commands from there or use `just` from root. The container image is OCI/Podman-based (`Dockerfile` builds on `golang:1.26`); the module targets `go 1.25`.

## Architecture

### Request Flow
`main.go` → `server/server.go` (creates DB, mailer, ResourceHandle) → `server/routes.go` (registers routes with middleware) → `controller/*controller/` (handles requests)

### Key Components

- **ResourceHandle** (`internal/handle/resourcehandler.go`): Central struct passed to all controllers containing config, DB connections (GORM + sqlx), and mailer
- **Controllers** (`internal/controller/*/`): Each domain has its own controller package (compound, interaction, adr, priscus, qt, pzn/product, formulation, user, admin, sys). Controllers receive ResourceHandle and define route handlers
- **Middleware** (`internal/middleware/`): JWT authentication and role-based access control
- **Database**: Uses both GORM (for migrations/user tables) and sqlx (for ABDA queries with squirrel query builder). `internal/controller/common` holds shared resolvers (`FamToPZN`, `StoToCompoundsMap`)
- **ENDPOINT_LOGIC.md**: per-endpoint reference of how inputs resolve to internal keys and which ABDA tables are queried

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
- Swagger docs auto-generated from code annotations (`swag init`); served at `/swagger/index.html`
- **Compound-name list parameters** (`compounds`/`names`/`compound` on the compound, interaction, QT, Priscus and ADR routes) are read via `handle.QueryList`/`handle.NormalizeList`: repeated query params are kept verbatim (so a name may contain a comma, e.g. the ABDA canonical `Mirtazapin-0,5-Wasser`), while a single value is comma-split for legacy clients. PZN lists stay comma-separated (PZNs never contain a comma). See README "List query parameters on compound endpoints".

## Testing

- **Unit tests** use `github.com/DATA-DOG/go-sqlmock` + gin `httptest` and require no database; they assert exact SQL/args and response shapes.
- **Integration tests** are tagged `//go:build integration` and run against a real ABDA MySQL database. They skip cleanly unless `ABDA_TEST_HOST` (+ `ABDA_TEST_USER`/`ABDA_TEST_PASSWORD`/`ABDA_TEST_DB`) is set, sourced from `api/.env.integration` (see `api/.env.integration.example`). `.env.integration` is gitignored — never commit credentials.

## Configuration

- **YAML config**: `api/cfg/default_config.yml` - server settings, DB pool, token expiration, rate limits
- **Environment**: Required vars in `api/cfg/default_env` - MySQL credentials, JWT secret, SendGrid API key
- Config merges YAML settings with environment variables via `envconfig` tags

## Linting

Uses strict golangci-lint config (`api/.golangci.yaml`) with 50+ linters enabled. Notable requirements:
- Comments must end with periods (godot)
- No global variables (gochecknoglobals); no init functions (gochecknoinits)
- Errors from external packages must be wrapped (wrapcheck)
- Row errors from sqlx must be checked (rowserrcheck)

Intentional exceptions configured in `.golangci.yaml`: `misspell` ignores the domain terms `Derivate` (German; also the `json:"derivate"` API key) and `pointes` (medical term "Torsade de pointes"); `lll` ignores swagger `@`-annotation lines (swaggo needs them on one line); test files relax `gochecknoglobals`/`lll`/`unparam`/`shadow`.
