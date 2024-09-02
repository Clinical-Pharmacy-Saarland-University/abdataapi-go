set windows-shell := ["pwsh.exe", "-NoLogo", "-c"]

r := if os_family() == "windows" {"RScript.exe"} else {"RScript"}

_default:
    @ just -l

# Runs the API
[group('dev')]
run:
    @ cd api && go mod tidy
    @ cd api && air

[group ('prod')]
build:
    @ cd api && go mod tidy
    @ cd api && go build -ldflags="-s -w -X main.versionTag=$(git rev-parse --short HEAD)" -trimpath -o tmp/api.exe ./cmd/api

# Deletes feature branch after merging
[group('git')]
git-done branch=`git rev-parse --abbrev-ref HEAD`:
    @ git checkout main
    @ git diff --no-ext-diff --quiet --exit-code
    @ git pull --rebase github main
    @ git diff --no-ext-diff --quiet --exit-code {{branch}}
    @ git branch -D {{branch}}

# Installs Air and sets default .env
[group('init')]
init:
    @ go install github.com/air-verse/air@latest
    @ cp api/config/default_env api/.env
