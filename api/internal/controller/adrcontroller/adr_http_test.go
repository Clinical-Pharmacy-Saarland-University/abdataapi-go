package adrcontroller_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/adrcontroller"
	"observeddb-go-api/internal/utils/format"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// newADRController builds an ADRController wired to the given mocked sqlx DB.
func newADRController(sqlxDB *sqlx.DB) *adrcontroller.ADRController {
	return &adrcontroller.ADRController{
		DB:                  sqlxDB,
		Limits:              cfg.LimitsConfig{InteractionDrugs: 100},
		FrequencyTranslator: format.NewAdrFrequencyTranslator(),
	}
}

// serveGetAdrs registers the bare handler (no auth middleware) and performs the request.
func serveGetAdrs(ac *adrcontroller.ADRController, rawQuery string) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/adrs/pzns", ac.GetAdrsForPZNs)

	req := httptest.NewRequest(http.MethodGet, "/adrs/pzns?"+rawQuery, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestGetAdrsForPZNs_InvalidLang asserts that an unsupported `lang` value fails the
// binding `oneof` constraint and yields HTTP 422 before any DB query runs.
func TestGetAdrsForPZNs_InvalidLang(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	ac := newADRController(sqlxDB)

	// A valid PZN is supplied so only the lang value is invalid.
	w := serveGetAdrs(ac, "pzns=03041347&lang=klingon")

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusUnprocessableEntity, w.Code, w.Body.String())
	}

	// No query should have been executed on the invalid-binding path.
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unexpected DB queries executed: %v", merr)
	}
}

// TestGetAdrsForPZNs_InvalidPZN asserts that a malformed PZN is rejected with HTTP 400.
func TestGetAdrsForPZNs_InvalidPZN(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	ac := newADRController(sqlxDB)

	// "03041348" is 8 digits but fails the PZN checksum (03041347 is the valid one).
	w := serveGetAdrs(ac, "pzns=03041348")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusBadRequest, w.Code, w.Body.String())
	}

	// Validation happens before any DB query.
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unexpected DB queries executed: %v", merr)
	}
}

// TestGetAdrsForPZNs_Success drives the full happy path: the handler first queries
// PAE_DB (PZN -> Key_FAM) and then NEB_C (the ADRs for those FAMs), and returns HTTP 200
// with a translated ADR description.
func TestGetAdrsForPZNs_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	ac := newADRController(sqlxDB)

	const (
		validPZN = "03041347"
		famKey   = uint64(100)
	)

	// First query: FamToPZN over PAE_DB.
	mock.ExpectQuery("FROM PAE_DB").
		WithArgs(validPZN).
		WillReturnRows(
			sqlmock.NewRows([]string{"PZN", "Key_FAM"}).
				AddRow(validPZN, famKey),
		)

	// Second query: fetchAdrs over NEB_C. Args are the FAM key(s) followed by the
	// language key (2 = english). Frequency code 2 maps to "Common (>= 1% to < 10%)".
	mock.ExpectQuery("FROM NEB_C").
		WithArgs(famKey, 2).
		WillReturnRows(
			sqlmock.NewRows([]string{"Key_FAM", "Haeufigkeit", "Name"}).
				AddRow(famKey, 2, "Headache"),
		)

	w := serveGetAdrs(ac, "pzns="+validPZN)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "Headache") {
		t.Errorf("expected body to contain ADR description %q, got: %s", "Headache", body)
	}
	if !strings.Contains(body, validPZN) {
		t.Errorf("expected body to contain PZN %q, got: %s", validPZN, body)
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unmet sqlmock expectations: %v", merr)
	}
}
