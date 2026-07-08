package compoundcontroller_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"observeddb-go-api/internal/controller/compoundcontroller"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// serveGuidelines builds a bare gin engine (no auth middleware) and executes a GET
// request against the /compounds/guidelines route.
func serveGuidelines(cc *compoundcontroller.CompoundController, target string) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/compounds/guidelines", cc.GetCompoundGuidelines)

	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

func TestGetCompoundGuidelines_MissingNames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cc, mock, cleanup := newTestController(t, 50)
	defer cleanup()

	// No `names` param at all -> IsEmptyList true -> 400, and no DB query issued.
	w := serveGuidelines(cc, "/compounds/guidelines")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Missing required parameter") {
		t.Errorf("expected body to mention missing parameter, got: %s", w.Body.String())
	}
	// No query should have been expected/run; ExpectationsWereMet must pass.
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unexpected DB interaction: %v", merr)
	}
}

func TestGetCompoundGuidelines_TooManyNames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// BatchQueries limit of 2, but 3 repeated names -> 400 before any DB query.
	cc, mock, cleanup := newTestController(t, 2)
	defer cleanup()

	w := serveGuidelines(cc,
		"/compounds/guidelines?names=Metoprolol&names=Metoprolol&names=Metoprolol")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Too many names") {
		t.Errorf("expected body to mention too many names, got: %s", w.Body.String())
	}
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unexpected DB interaction: %v", merr)
	}
}

func TestGetCompoundGuidelines_RepeatedNamesReachSQLIntact(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cc, mock, cleanup := newTestController(t, 50)
	defer cleanup()

	cols := []string{"Name", "Herkunft", "Vorzugsbezeichnung", "Key_STO", "Input"}

	// Repeated params, including one bearing a comma. Because >=2 values are sent,
	// NormalizeList returns them verbatim, so the comma-bearing name survives intact
	// into the FetchCompounds SNA_DB query args. Empty rows mean no compounds resolve,
	// so only this single query is issued (no guideline_table query follows).
	mock.ExpectQuery("FROM SNA_DB").
		WithArgs("Metoprolol", "Mirtazapin-0,5-Wasser", "Aspirin").
		WillReturnRows(sqlmock.NewRows(cols))

	w := serveGuidelines(cc,
		"/compounds/guidelines?names=Metoprolol&names=Mirtazapin-0,5-Wasser&names=Aspirin")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	// The crucial assertion: WithArgs matched exactly, proving the comma-bearing
	// name was not split, and only the one SNA_DB query ran.
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("SQL args were not intact or extra query ran: %v", merr)
	}
}

func TestGetCompoundGuidelines_LegacySingleCommaValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cc, mock, cleanup := newTestController(t, 50)
	defer cleanup()

	cols := []string{"Name", "Herkunft", "Vorzugsbezeichnung", "Key_STO", "Input"}

	// Legacy form: single `names` param with a comma-joined value is split into two,
	// which reach the SNA_DB query as separate args. Empty rows -> only this query.
	mock.ExpectQuery("FROM SNA_DB").
		WithArgs("Metoprolol", "Aspirin").
		WillReturnRows(sqlmock.NewRows(cols))

	w := serveGuidelines(cc, "/compounds/guidelines?names=Metoprolol,Aspirin")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("SQL args were not intact: %v", merr)
	}
}
