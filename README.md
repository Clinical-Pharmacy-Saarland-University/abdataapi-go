# ClinPharm ABDATA API

![GitHub go.mod Go version (branch)](https://img.shields.io/github/go-mod/go-version/Clinical-Pharmacy-Saarland-University/abdataapi-go/main?filename=api%2Fgo.mod) ![GitHub License](https://img.shields.io/github/license/Clinical-Pharmacy-Saarland-University/abdataapi-go) ![GitHub Actions Workflow Status](https://img.shields.io/github/actions/workflow/status/Clinical-Pharmacy-Saarland-University/abdataapi-go/publish-image.yaml?branch=main) ![Static Badge](https://img.shields.io/badge/status-under_active_development-red)

**The API is currently under active development and not yet ready for production use.**

This is an implementation of the ClinPharm ABDATA API. The API is a RESTful API that provides access to evaluations using the [ABDATA database](https://abdata.de/). The API is implemented in [Go](https://go.dev/) and uses the [Gin](https://github.com/gin-gonic/gin) framework.

**The API is designed to provide the following information:**

1. Drug-Drug Interaction (DDI) evaluations
2. Adverse Drug Reaction (ADR) evaluations
3. Priscus List evaluations
4. Various drug-related information

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

**You will have access to the ABDATA database as defined in your yaml configuration file with write access for the user defined in the environment variables**

**On first run, the user tables will be migrated and seeded with the initial admin user as defined in the environment variables.**

## Details

### Database

You need a MySQL database with data from [ABDATA](https://abdata.de/). The database is proprietary and not included in this or other repositories. If you have access to the ABDATA database, you can use [https://github.com/Clinical-Pharmacy-Saarland-University/abdata.sql.db](https://github.com/Clinical-Pharmacy-Saarland-University/abdata.sql.db) to export the data to a MySQL database.

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

1. `just init` will install air, swag and golangci-lint (windows only) and copy the default `.env` file to `/api`
2. `just run` will start the API with air and swag init/fmt in debug mode.
