# ClinPharm ABDA API

![GitHub go.mod Go version (branch)](https://img.shields.io/github/go-mod/go-version/Clinical-Pharmacy-Saarland-University/abdataapi-go/main?filename=api%2Fgo.mod) ![GitHub License](https://img.shields.io/github/license/Clinical-Pharmacy-Saarland-University/abdataapi-go) ![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Clinical-Pharmacy-Saarland-University/abdataapi-go/publish-image.yaml?branch=main) ![Static Badge](https://img.shields.io/badge/status-under_active_development-red)

\*\*The API is currently under active development and not yet ready for production use.

This is an implementation of the ClinPharm ABDA API. The API is a RESTful API that provides access to evaluations using the [ABDA database](https://abdata.de/). It is implemented in [Go](https://go.dev/) and uses the [Gin](https://github.com/gin-gonic/gin) framework. It provides:

1. Drug-drug interaction (DDI) evaluations
2. Adverse Drug Reaction (ADR) evaluations
3. Priscus list evaluations
4. QT-prolongation evaluations
5. Product / PZN information and product search
6. Compound name resolution and guideline (PharmGKB) lookups

Interactive API documentation is served via Swagger at `/swagger/index.html`.

## Container Image

The image is OCI-compliant and built/run with [Podman](https://podman.io/) (Docker works too — swap `podman` for `docker`). There are two options to build/obtain the image:

```bash
# build
podman build -t clinical-pharmacy-saarland-university/abdataapi-go:latest .

# pull latest image
podman pull ghcr.io/clinical-pharmacy-saarland-university/abdataapi-go:latest
```

## Running the Container

The container must be run with port mapping to port `3333` and needs the following environment variables to be set:

```bash
MYSQL_HOST=127.0.0.1
MYSQL_PORT=3306
MYSQL_USER=mysqluser
MYSQL_PASSWORD=yourpassword
# Secret for JWT token generation
JWT_SECRET=yourjwtsecret
# https://pkg.go.dev/github.com/gin-gonic/gin#Engine.SetTrustedProxies
TRUSTED_PROXIES=proxy1,proxy2
# Initial admin user for the user database (migration)
ADMIN_EMAIL=admin@me.com
ADMIN_PASSWORD=password
# mail and sendgrid api key for sending emails
SEND_EMAIL=admin@yourdomain.com
SEND_EMAIL_API_KEY=sendgridapikey
```

Log files will be written to `/logs` in the container.

**You will have access to the ABDA database as defined in your yaml configuration file with write access for the user defined in the environment variables**

**On first run, the user tables will be migrated and seeded with the initial admin user as defined in the environment variables.**

## Details

### List query parameters on compound endpoints

Endpoints that take a **list of compound names** accept that list in two formats:

| Endpoint | Parameter |
|---|---|
| `GET /interactions/compounds` | `compounds` |
| `GET /compounds/names` | `names` |
| `GET /compounds/guidelines` | `names` |
| `GET /qt/compounds` | `compounds` |
| `GET /priscus/compounds` | `compounds` |
| `GET /adrs/compounds` | `compound` |

1. **Repeated query parameter (preferred):**

   ```
   GET /interactions/compounds?compounds=Apixaban&compounds=Mirtazapin-0,5-Wasser&compounds=Bisoprolol
   ```

   Each occurrence of the parameter is treated as **one name, verbatim**. Sending a name
   that contains a comma requires this form — ABDA canonical compound names can carry a
   decimal-comma qualifier (e.g. `Mirtazapin-0,5-Wasser`), which is exactly what
   `GET /compounds/names` returns.

2. **Single comma-joined value (legacy, still supported):**

   ```
   GET /interactions/compounds?compounds=Aspirin,Paracetamol
   ```

   When the parameter appears **once**, its value is split on commas. This keeps older
   clients working, but — because the delimiter and a data comma are indistinguishable
   once URL-decoded — it **cannot represent a name that contains a comma**. Use the
   repeated form for those names.

> **Caveat — a single value is always split.** The two forms are indistinguishable on the
> wire when only one value is present, so a comma inside a name survives only when at least
> two values are sent. A lone comma-bearing value (e.g. `?names=Mirtazapin-0,5-Wasser` on
> its own) is still split. On `GET /interactions/compounds` this never matters (it requires
> ≥ 2 compounds); on the single-name-capable endpoints, send a comma-bearing name together
> with at least one other value (repeated form) so the comma is preserved. Percent-encoding
> the comma as `%2C` does **not** help: it decodes to the same character before splitting.

The `POST /interactions/compounds` batch endpoint already takes a JSON array
(`"compounds": [...]`) and is unaffected.

> PZN list endpoints (`/interactions/pzns`, `/adrs/pzns`, `/priscus/pzns`, `/qt/pzns`,
> `/product/info/pzns`, `/product/activecompounds/pzns`) remain comma-separated: PZNs are
> 8-digit numbers and never contain a comma.

### Testing

```bash
# unit tests (mocked DB via go-sqlmock; no database required)
just test          # or: cd api && go test ./...

# integration tests against a real ABDA database
just test-integration   # or: cd api && go test -tags=integration ./...
```

Unit tests are hermetic and run anywhere. The `integration`-tagged tests hit a **real ABDA
MySQL database** and are skipped unless connection details are provided. Supply them either
as environment variables or via `api/.env.integration` (see [`api/.env.integration.example`](api/.env.integration.example)):

```bash
ABDA_TEST_HOST=127.0.0.1:3306   # e.g. an SSH-tunneled ABDA host
ABDA_TEST_USER=...
ABDA_TEST_PASSWORD=...
ABDA_TEST_DB=abda
```

When `ABDA_TEST_HOST` is unset the integration suite skips cleanly, so the default
`go test ./...` stays green without a database.

### Database

You need a MySQL database with data from [ABDA](https://abdata.de/). The database is proprietary and not included in this or other repositories. If you have access to the ABDA database, you can use [https://github.com/Clinical-Pharmacy-Saarland-University/abdata.sql.db](https://github.com/Clinical-Pharmacy-Saarland-University/abdata.sql.db) to export the data to a MySQL database.

### Running the API outside of Docker

The API has the following command line options:

```bash
Usage of api:
  -config string
        Config file path (default "config.yml")
  -debug
        Enable debug mode
  -env string
        .env file path (if not set, will use .env if exists)
```

#### Environment Variables

Enviroment variables will be considered in the following order:

1. Already set environment variables
2. Variables from the .env file

#### Configuration File

The configuration file is a YAML file with the following structure: [config.yml](https://github.com/Clinical-Pharmacy-Saarland-University/abdataapi-go/blob/main/api/cfg/default_config.yml)

## Local Development

1. You need Go Version 1.25 or higher (the module targets `go 1.25`; the container builds on `golang:1.26`).
2. You need to install [air](https://github.com/air-verse/air) and [swag](https://github.com/swaggo/swag) for development.
3. Please install and use the [golangci-lint](https://golangci-lint.run/) linter.
4. You might want to install [just](https://github.com/casey/just) as a task runner.

Type `just` to see the available tasks.

1. `just init` will install air, swag, [gotestsum](https://github.com/gotestyourself/gotestsum) and golangci-lint (windows only) and copy the default `.env` file to `/api`
2. `just run` will start the API with air and swag init/fmt in debug mode.
3. `just test` / `just test-integration` run the unit / integration test suites.
