package compoundcontroller_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/compoundcontroller"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// newTestController wires a CompoundController to a fresh sqlmock-backed DB.
func newTestController(
	t *testing.T, batchQueries int,
) (*compoundcontroller.CompoundController, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	sqlxDB := sqlx.NewDb(db, "sqlmock")

	cc := &compoundcontroller.CompoundController{
		DB:     sqlxDB,
		Limits: cfg.LimitsConfig{BatchQueries: batchQueries},
	}

	cleanup := func() {
		_ = db.Close()
	}

	return cc, mock, cleanup
}

// serve builds a bare gin engine (no auth middleware) and executes a GET request.
func serve(cc *compoundcontroller.CompoundController, target string) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/compounds/names", cc.GetSelectCompounds)

	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

func TestGetSelectCompounds_MissingNames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cc, mock, cleanup := newTestController(t, 50)
	defer cleanup()

	// No `names` param at all -> IsEmptyList true -> 400, and no DB query issued.
	w := serve(cc, "/compounds/names")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Missing required parameter") {
		t.Errorf("expected body to mention missing parameter, got: %s", w.Body.String())
	}
	// No query should have been expected/run; ExpectationsWereMet must pass.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction: %v", err)
	}
}

func TestGetSelectCompounds_EmptyNamesValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cc, mock, cleanup := newTestController(t, 50)
	defer cleanup()

	// `?names=` -> single empty value -> IsEmptyList true -> 400, no DB query.
	w := serve(cc, "/compounds/names?names=")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Missing required parameter") {
		t.Errorf("expected body to mention missing parameter, got: %s", w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction: %v", err)
	}
}

func TestGetSelectCompounds_TooManyNames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// BatchQueries limit of 2, but 3 repeated names -> 400 before any DB query.
	cc, mock, cleanup := newTestController(t, 2)
	defer cleanup()

	w := serve(cc, "/compounds/names?names=Metoprolol&names=Aspirin&names=Bisoprolol")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Too many names") {
		t.Errorf("expected body to mention too many names, got: %s", w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction: %v", err)
	}
}

func TestGetSelectCompounds_RepeatedNamesReachSQLIntact(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cc, mock, cleanup := newTestController(t, 50)
	defer cleanup()

	cols := []string{"Name", "Herkunft", "Vorzugsbezeichnung", "Key_STO", "Input"}

	// Repeated params, including one bearing a comma. Because >=2 values are sent,
	// NormalizeList returns them verbatim, so the comma-bearing name survives intact
	// all the way into the SQL args.
	mock.ExpectQuery("FROM SNA_DB").
		WithArgs("Metoprolol", "Mirtazapin-0,5-Wasser", "Aspirin").
		WillReturnRows(sqlmock.NewRows(cols))

	w := serve(cc,
		"/compounds/names?names=Metoprolol&names=Mirtazapin-0,5-Wasser&names=Aspirin")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	// Empty rows -> each input echoed with empty matches.
	body := w.Body.String()
	if !strings.Contains(body, "Mirtazapin-0,5-Wasser") {
		t.Errorf("expected body to echo the comma-bearing input verbatim, got: %s", body)
	}
	// The crucial assertion: WithArgs matched, proving args were not comma-split.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL args were not intact: %v", err)
	}
}

func TestGetSelectCompounds_LegacySingleCommaValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cc, mock, cleanup := newTestController(t, 50)
	defer cleanup()

	cols := []string{"Name", "Herkunft", "Vorzugsbezeichnung", "Key_STO", "Input"}

	// Legacy form: single `names` param with a comma-joined value is split into two.
	mock.ExpectQuery("FROM SNA_DB").
		WithArgs("Metoprolol", "Aspirin").
		WillReturnRows(sqlmock.NewRows(cols))

	w := serve(cc, "/compounds/names?names=Metoprolol,Aspirin")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("SQL args were not intact: %v", err)
	}
}
