# ClinPharm ABDA API

![GitHub go.mod Go version (branch)](https://img.shields.io/github/go-mod/go-version/Clinical-Pharmacy-Saarland-University/abdataapi-go/main?filename=api%2Fgo.mod) ![GitHub License](https://img.shields.io/github/license/Clinical-Pharmacy-Saarland-University/abdataapi-go) ![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Clinical-Pharmacy-Saarland-University/abdataapi-go/publish-image.yaml?branch=main) ![Static Badge](https://img.shields.io/badge/status-under_active_development-red)

\*\*The API is currently under active development and not yet ready for production use.

This is an implementation of the ClinPharm ABDA API. The API is a RESTful API that provides access to evaluations using the [ABDA database](https://abdata.de/). The API is implemented in [Go](https://go.dev/) and uses the [Gin](https://github.com/gin-gonic/gin) framework. 2. Adverse Drug Reaction (ADR) evaluations 3. Priscus List evaluations 4. Various drug-related information

## Docker Image

There are two options to build/obtain a Docker image of the API:

```bash
# build
docker build -t clinical-pharmacy-saarland-university/abdataapi-go:latest .

# pull latest image
docker pull ghcr.io/clinical-pharmacy-saarland-university/abdataapi-go:latest
```

## Running the Docker Container

The docker container must be run with port mapping to port `3333` and needs the following environment variables to be set:

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

A request uses one form or the other: two or more `compounds`/`names` parameters are
always taken verbatim, while a single one is comma-split.

> **Caveat — a single value is always split.** The two forms are indistinguishable on the
> wire when only one value is present, so a comma inside a name survives only when at least
> two values are sent. A lone comma-bearing value (e.g. `?names=Mirtazapin-0,5-Wasser` on
> its own) is still split. On `GET /interactions/compounds` this never matters (it requires
> ≥ 2 compounds). On `GET /compounds/names` and `GET /compounds/guidelines` a single-name
> lookup is valid, so a lone comma-bearing name sent as one value is **silently split** and
> returns HTTP 200 with empty/incorrect results — send it together with at least one other
> value (repeated form) so the comma is preserved. Percent-encoding the comma as `%2C` does
> **not** help: it decodes to the same character before splitting.

The `POST /interactions/compounds` batch endpoint already takes a JSON array
(`"compounds": [...]`) and is unaffected.

> PZN list endpoints (`/interactions/pzns`, `/adrs/pzns`, `/priscus/pzns`, `/qt/pzns`)
> remain comma-separated: PZNs are 8-digit numbers and never contain a comma.

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

1. You need Go Version 1.23 or higher.
2. You need to install [air](https://github.com/air-verse/air) and [swag](https://github.com/swaggo/swag) for development.
3. Please install and use the [golangci-lint](https://golangci-lint.run/) linter.
4. You might want to install [just](https://github.com/casey/just) as a task runner.

Type `just` to see the available tasks.

1. `just init` will install air, swag, gotestsum and golangci-lint (windows only) and copy the default `.env` file to `/api`
2. `just run` will start the API with air and swag init/fmt in debug mode.

## Testing

The Go test suite runs entirely **without a database or network**. Database access is faked
with [`go-sqlmock`](https://github.com/DATA-DOG/go-sqlmock), so the HTTP handlers, query
building and result mapping are exercised deterministically.

```bash
# from the repository root
just test            # runs the suite inside /api

# or directly
cd api && go test ./...
```

`just test` uses [`gotestsum`](https://github.com/gotestyourself/gotestsum) (installed by
`just init`) for a readable per-package summary, and falls back to plain `go test` if it
isn't installed.

Tests live next to the code as `*_test.go` files (external `_test` packages). Coverage
includes the list-parameter parsing described above (the comma-in-name handling), the PZN
checksum validation, the response translators, compound name normalization, the JSend
response helpers and the panic-recovery middleware, and — against a mocked database — the
HTTP handlers for compounds and guidelines, drug-drug interactions (compounds and PZNs,
single and batch), ADRs, QT, Priscus, product info, formulations, and the system endpoints.

Authentication and user-management flows (login, JWT middleware, tokens, password hashing)
are not yet covered.

> These Go tests replace the previous ad-hoc R smoke scripts that used to live under `tests/`.

### Integration tests (real database)

A small, **opt-in** set of integration tests exercises the endpoints against a real,
populated ABDA database — this is the only way to verify behavior that depends on real data
(e.g. that a canonical compound name legitimately contains a comma) and that the generated
SQL is valid against the real schema. They live in `api/internal/integration/` behind the
`integration` build tag and **skip themselves** unless `ABDA_TEST_HOST` is set, so the normal
suite and CI never touch a database.

Configuration comes from environment variables, most conveniently a git-ignored
`api/.env.integration` file (copy `api/.env.integration.example`):

```dotenv
ABDA_TEST_HOST=127.0.0.1:<port>   # host:port; over an SSH tunnel this is the local end
ABDA_TEST_USER=readonly_user      # use a READ-ONLY account (queries are all SELECTs)
ABDA_TEST_PASSWORD="..."          # quote it if it contains #, spaces, etc.
ABDA_TEST_DB=abda                 # optional; defaults to "abda"
```

```bash
just test-integration
```
