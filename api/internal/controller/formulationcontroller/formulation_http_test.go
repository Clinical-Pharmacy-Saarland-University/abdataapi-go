package formulationcontroller_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"observeddb-go-api/internal/controller/formulationcontroller"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// newFormTestController wires a FormulationController to a mocked database. The
// returned mock is used to set query expectations; the cleanup closes the DB.
func newFormTestController(t *testing.T) (*formulationcontroller.FormulationController, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	fc := &formulationcontroller.FormulationController{
		DB: sqlxDB,
	}

	return fc, mock
}

func doFormRequest(t *testing.T, fc *formulationcontroller.FormulationController) *httptest.ResponseRecorder {
	t.Helper()

	r := gin.New()
	r.GET("/formulations", fc.GetFormulations)

	req := httptest.NewRequest(http.MethodGet, "/formulations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

func TestGetFormulations_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fc, mock := newFormTestController(t)

	// The handler runs "SELECT Key_DAR, Name FROM DAR_DB ORDER BY Key_DAR" and
	// maps Key_DAR -> formulation and Name -> description.
	rows := sqlmock.NewRows([]string{"Key_DAR", "Name"}).
		AddRow("TAB", "Tablet").
		AddRow("KAP", "Kapsel")

	mock.ExpectQuery("FROM DAR_DB").WillReturnRows(rows)

	w := doFormRequest(t, fc)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Formulations []struct {
				Formulation string `json:"formulation"`
				Description string `json:"description"`
			} `json:"formulations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v (body: %s)", err, w.Body.String())
	}

	if resp.Status != "success" {
		t.Fatalf("expected status 'success', got %q", resp.Status)
	}

	if len(resp.Data.Formulations) != 2 {
		t.Fatalf("expected 2 formulations, got %d (body: %s)",
			len(resp.Data.Formulations), w.Body.String())
	}

	got := make(map[string]string, len(resp.Data.Formulations))
	for _, f := range resp.Data.Formulations {
		got[f.Formulation] = f.Description
	}

	if desc, ok := got["TAB"]; !ok || desc != "Tablet" {
		t.Errorf("expected TAB->Tablet, got present=%v value=%q", ok, desc)
	}

	if desc, ok := got["KAP"]; !ok || desc != "Kapsel" {
		t.Errorf("expected KAP->Kapsel, got present=%v value=%q", ok, desc)
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Fatalf("unmet or unexpected DB expectations: %v", merr)
	}
}

func TestGetFormulations_DBError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fc, mock := newFormTestController(t)

	// A failing query surfaces as a JSend "error" response with HTTP 500.
	mock.ExpectQuery("FROM DAR_DB").WillReturnError(errors.New("boom"))

	w := doFormRequest(t, fc)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusInternalServerError, w.Code, w.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v (body: %s)", err, w.Body.String())
	}

	if resp.Status != "error" {
		t.Fatalf("expected status 'error', got %q (body: %s)", resp.Status, w.Body.String())
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Fatalf("unmet or unexpected DB expectations: %v", merr)
	}
}
