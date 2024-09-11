set windows-shell := ["pwsh.exe", "-NoLogo", "-c"]

GH_USER := env("GH_USER", "NOUSER")
GH_TOKEN := env("GH_TOKEN", "NOPASS")
port := "3333"

_default:
    @ just -l

# Runs the API via AIR
[group('dev')]
run:
    @ cd api && go mod tidy
    @ cd api && swag init
    @ cd api && swag fmt
    @ cd api && air

# Creates a docker deployment image
[group('deploy')]
deploy-build:
    @ docker build --no-cache . --tag ghcr.io/clinical-pharmacy-saarland-university/abdataapi-go:latest

# Pulls the deployed image from the container registry
[group('deploy')]
deploy-pull user=GH_USER pass=GH_TOKEN:
    @ echo {{user}} {{pass}}
    @ echo {{pass}} | docker login --username {{user}} --password-stdin ghcr.io
    @ docker pull ghcr.io/eracosysmed-inspiration/dss:latest
    @ docker logout ghcr.io     

# Runs the deployed image
[group('deploy')]
deploy-run port=port:
    @ docker run -it --rm -p {{port}}:3333 --env-file .env --name abdata-api ghcr.io/clinical-pharmacy-saarland-university/abdataapi-go:latest

# Deletes feature branch after merging
[group('git')]
git-done branch=`git rev-parse --abbrev-ref HEAD`:
    @ git checkout main
    @ git diff --no-ext-diff --quiet --exit-code
    @ git pull --rebase github main
    @ git diff --no-ext-diff --quiet --exit-code {{branch}}
    @ git branch -D {{branch}}

# Installs Air. Swag and sets default .env
[group('init')]
init:
    @ go install github.com/air-verse/air@latest
    @ go install github.com/swaggo/swag/cmd/swag@latest
    @ cp api/config/default_env api/.env
