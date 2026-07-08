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

// newCompoundController builds an ADRController wired to the given mocked sqlx DB.
// BatchQueries is set high enough that the compound-count guard never trips in
// these tests.
func newCompoundController(sqlxDB *sqlx.DB) *adrcontroller.ADRController {
	return &adrcontroller.ADRController{
		DB:                  sqlxDB,
		Limits:              cfg.LimitsConfig{InteractionDrugs: 100, BatchQueries: 100},
		FrequencyTranslator: format.NewAdrFrequencyTranslator(),
	}
}

// serveGetCompoundAdrs registers the bare handler (no auth middleware) and performs
// the request against the raw query string supplied by the caller.
func serveGetCompoundAdrs(ac *adrcontroller.ADRController, rawQuery string) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/adrs/compounds", ac.GetAdrsForCompound)

	req := httptest.NewRequest(http.MethodGet, "/adrs/compounds?"+rawQuery, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestGetAdrsForCompound_MissingParam asserts that omitting the required `compound`
// parameter yields HTTP 400 with the documented message, before any DB query runs.
func TestGetAdrsForCompound_MissingParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	ac := newCompoundController(sqlxDB)

	// A benign, unrelated parameter keeps the query string non-empty while `compound`
	// is entirely absent.
	w := serveGetCompoundAdrs(ac, "lang=english")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusBadRequest, w.Code, w.Body.String())
	}

	if body := w.Body.String(); !strings.Contains(body, "Missing required parameter: compound") {
		t.Errorf("expected body to contain %q, got: %s", "Missing required parameter: compound", body)
	}

	// No query is registered on the mock, so any query execution is an error.
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unexpected DB queries executed: %v", merr)
	}
}

// TestGetAdrsForCompound_RepeatedParamPreservesComma exercises the dev-only comma fix:
// two repeated `compound` parameters, one of which carries an intentional comma
// ("Mirtazapin-0,5-Wasser"). The comma must NOT be treated as a list separator, so the
// compound search runs with the intact name as a single LIKE prefix.
func TestGetAdrsForCompound_RepeatedParamPreservesComma(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	ac := newCompoundController(sqlxDB)

	// The compound search (fetchCompoundAdrs) issues a single SELECT against SNA_DB with
	// args [Stofftyp, prefix1, prefix2, ...]. Prefixes are lower-cased and suffixed with
	// '%'. The intact "Mirtazapin-0,5-Wasser" must appear as ONE argument, proving it was
	// not split on the comma into "mirtazapin-0%" and "5-wasser%".
	mock.ExpectQuery("FROM SNA_DB").
		WithArgs(1, "apixaban%", "mirtazapin-0,5-wasser%").
		WillReturnRows(sqlmock.NewRows([]string{
			"compound_name", "key_sto", "key_fam", "key_dar",
			"formulation_name", "applikationsweg_code",
			"applikationsweg_label", "application_group",
		}))

	// With no candidate FAMs, fetchRepresentativePZNsByFAM short-circuits without a query,
	// and the final ADR fetch degenerates to a (1=0) predicate over NEB_C carrying only
	// the language key.
	mock.ExpectQuery("FROM NEB_C").
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"Key_FAM", "Key_MIV", "Haeufigkeit", "Name"}))

	w := serveGetCompoundAdrs(ac, "compound=Apixaban&compound=Mirtazapin-0,5-Wasser")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unmet or unexpected DB expectations: %v", merr)
	}
}

// TestGetAdrsForCompound_LegacyCommaJoinedSplits asserts the backward-compatible path:
// a single comma-joined value ("A,B") is split into two separate compound names, so the
// SNA_DB search runs with two distinct LIKE prefixes.
func TestGetAdrsForCompound_LegacyCommaJoinedSplits(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	ac := newCompoundController(sqlxDB)

	// "A,B" as a single parameter is normalized into two names -> two prefixes.
	mock.ExpectQuery("FROM SNA_DB").
		WithArgs(1, "a%", "b%").
		WillReturnRows(sqlmock.NewRows([]string{
			"compound_name", "key_sto", "key_fam", "key_dar",
			"formulation_name", "applikationsweg_code",
			"applikationsweg_label", "application_group",
		}))

	mock.ExpectQuery("FROM NEB_C").
		WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"Key_FAM", "Key_MIV", "Haeufigkeit", "Name"}))

	w := serveGetCompoundAdrs(ac, "compound=A,B")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unmet or unexpected DB expectations: %v", merr)
	}
}
