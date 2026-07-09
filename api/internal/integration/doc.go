// Package integration contains opt-in tests that run against a real, populated ABDA
// database. The test files are compiled only with the `integration` build tag and skip
// themselves unless the ABDA_TEST_* environment variables are set, so the normal
// `go test ./...` run (and CI) never touches a database. See api/.env.integration.example
// for configuration.
package integration
