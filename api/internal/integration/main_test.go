//go:build integration

package integration_test

import (
	"os"
	"testing"
	"time"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
)

func TestMain(m *testing.M) {
	// Best-effort: source api/.env.integration if present. Real shell environment
	// variables win, because godotenv.Load does not overwrite already-set variables.
	_ = godotenv.Load("../../.env.integration")

	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// testLimits mirrors the limits from cfg/default_config.yml.
func testLimits() cfg.LimitsConfig {
	return cfg.LimitsConfig{InteractionDrugs: 50, BatchQueries: 100, BatchJobs: 25}
}

// dialOrSkip connects to the ABDA test database through the app's own connector, so the
// DSN (charset/parseTime/loc and the server-default collation) is identical to production.
// The test is skipped when ABDA_TEST_HOST is unset, keeping the suite green without a tunnel.
func dialOrSkip(t *testing.T) *sqlx.DB {
	t.Helper()

	host := os.Getenv("ABDA_TEST_HOST")
	if host == "" {
		t.Skip("integration: set ABDA_TEST_HOST (plus ABDA_TEST_USER/ABDA_TEST_PASSWORD) to run")
	}

	dbName := os.Getenv("ABDA_TEST_DB")
	if dbName == "" {
		dbName = "abda"
	}

	_, db, err := database.New(cfg.DatabaseConfig{
		Host:            host,
		DBName:          dbName,
		Username:        os.Getenv("ABDA_TEST_USER"),
		Password:        os.Getenv("ABDA_TEST_PASSWORD"),
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		MaxConnLifetime: time.Minute,
	})
	if err != nil {
		// Note: the error does not include the DSN, so no credentials are logged.
		t.Fatalf("cannot connect to ABDA test database at host %q: %v", host, err)
	}

	t.Cleanup(func() { _ = db.Close() })
	return db
}
