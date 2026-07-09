package interactioncontroller_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"observeddb-go-api/internal/controller/interactioncontroller"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// newPZNTestEngine registers the bare GET /interactions/pzns handler without auth middleware.
func newPZNTestEngine(ic *interactioncontroller.InteractionController) *gin.Engine {
	r := gin.New()
	r.GET("/interactions/pzns", ic.GetInterPZNs)
	return r
}

// TestGetInterPZNsValidation covers the validation/early-return paths that never reach the
// database. No mock expectations are set: these paths must not issue any query.
func TestGetInterPZNsValidation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		rawQuery   string
		wantStatus int
		wantBody   string // substring expected in the response body ("" to skip)
	}{
		{
			name:       "missing pzns param -> 422",
			rawQuery:   "",
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "pzns",
		},
		{
			name:       "invalid PZN among two -> 400",
			rawQuery:   "pzns=123,05538454",
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid PZNs",
		},
		{
			name:       "single valid PZN -> 400 at least two PZNs",
			rawQuery:   "pzns=03041347",
			wantStatus: http.StatusBadRequest,
			wantBody:   "at least 2 PZNs",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()
			sqlxDB := sqlx.NewDb(db, "sqlmock")
			// Intentionally set NO expectations: these paths must not touch the DB.

			r := newPZNTestEngine(newTestController(sqlxDB))
			req := httptest.NewRequest(http.MethodGet, "/interactions/pzns?"+tt.rawQuery, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d (body: %s)", tt.wantStatus, w.Code, w.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(w.Body.String(), tt.wantBody) {
				t.Errorf("expected body to contain %q, got: %s", tt.wantBody, w.Body.String())
			}
			if merr := mock.ExpectationsWereMet(); merr != nil {
				t.Errorf("no DB query should have been issued, but sqlmock reports: %v", merr)
			}
		})
	}
}

// TestGetInterPZNsNotFound covers the happy validation path where two checksum-valid PZNs pass
// validate.PZNs, so fetchPznInteractions queries PAE_DB via common.FamToPZN. An empty result set
// means neither PZN maps to a FAM, so both are "not found" -> 404.
func TestGetInterPZNsNotFound(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()
	sqlxDB := sqlx.NewDb(db, "sqlmock")

	// FamToPZN issues the first query against PAE_DB with the exact PZNs as args.
	mock.ExpectQuery("FROM PAE_DB").
		WithArgs("03041347", "05538454").
		WillReturnRows(sqlmock.NewRows([]string{"PZN", "Key_FAM"}))

	r := newPZNTestEngine(newTestController(sqlxDB))
	req := httptest.NewRequest(http.MethodGet, "/interactions/pzns?pzns=03041347,05538454", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "PZNs not found") {
		t.Errorf("expected body to mention 'PZNs not found', got: %s", w.Body.String())
	}
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("sqlmock expectations not met: %v", merr)
	}
}
