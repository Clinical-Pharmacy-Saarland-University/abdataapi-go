package interactioncontroller_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/interactioncontroller"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// newTestController builds an InteractionController wired to the provided sqlmock DB.
// Translator fields are left nil because none of the covered paths reach the mapping
// stage (the DB compound lookup returns no rows, so every request short-circuits before
// any translator is invoked).
func newTestController(db *sqlx.DB) *interactioncontroller.InteractionController {
	return &interactioncontroller.InteractionController{
		DB:     db,
		Limits: cfg.LimitsConfig{InteractionDrugs: 100, BatchQueries: 50, BatchJobs: 4},
	}
}

// newTestEngine registers the bare handler without auth middleware.
func newTestEngine(ic *interactioncontroller.InteractionController) *gin.Engine {
	r := gin.New()
	r.GET("/interactions/compounds", ic.GetInterCompounds)
	return r
}

// TestGetInterCompoundsCommaHandling covers the core bug-fix behavior for the
// GET /interactions/compounds endpoint: how the `compounds` list parameter is parsed
// from the query string and passed to the DB layer intact.
func TestGetInterCompoundsCommaHandling(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	t.Run("repeated params with comma-bearing name reach SQL intact -> 404", func(t *testing.T) {
		t.Parallel()

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to open sqlmock: %v", err)
		}
		defer db.Close()
		sqlxDB := sqlx.NewDb(db, "sqlmock")

		// Repeated params (>=2) are used verbatim, so the comma inside the name survives.
		// Empty result set => all compounds "not found" => 404 echoing the input tokens.
		mock.ExpectQuery("SELECT DISTINCT Name").
			WithArgs("Apixaban", "Mirtazapin-0,5-Wasser", "Bisoprolol").
			WillReturnRows(sqlmock.NewRows([]string{"Name", "DDI_Key_STO"}))

		r := newTestEngine(newTestController(sqlxDB))
		req := httptest.NewRequest(http.MethodGet,
			"/interactions/compounds?compounds=Apixaban&compounds=Mirtazapin-0,5-Wasser&compounds=Bisoprolol", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "Mirtazapin-0,5-Wasser") {
			t.Errorf("expected body to echo the intact compound name, got: %s", w.Body.String())
		}
		if merr := mock.ExpectationsWereMet(); merr != nil {
			t.Errorf("sqlmock expectations not met (args were split/wrong?): %v", merr)
		}
	})

	t.Run("legacy single comma-joined value is split -> 404", func(t *testing.T) {
		t.Parallel()

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to open sqlmock: %v", err)
		}
		defer db.Close()
		sqlxDB := sqlx.NewDb(db, "sqlmock")

		// A single value is comma-split for backward compatibility.
		mock.ExpectQuery("SELECT DISTINCT Name").
			WithArgs("Apixaban", "Bisoprolol").
			WillReturnRows(sqlmock.NewRows([]string{"Name", "DDI_Key_STO"}))

		r := newTestEngine(newTestController(sqlxDB))
		req := httptest.NewRequest(http.MethodGet,
			"/interactions/compounds?compounds=Apixaban,Bisoprolol", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
		}
		if merr := mock.ExpectationsWereMet(); merr != nil {
			t.Errorf("sqlmock expectations not met (comma-join not split as expected): %v", merr)
		}
	})

	t.Run("documented limitation: lone comma-bearing value IS split -> 404", func(t *testing.T) {
		t.Parallel()

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to open sqlmock: %v", err)
		}
		defer db.Close()
		sqlxDB := sqlx.NewDb(db, "sqlmock")

		// A single value cannot be distinguished from a legacy comma-joined list, so
		// "Mirtazapin-0,5-Wasser" sent as ONE value is split into two fragments.
		mock.ExpectQuery("SELECT DISTINCT Name").
			WithArgs("Mirtazapin-0", "5-Wasser").
			WillReturnRows(sqlmock.NewRows([]string{"Name", "DDI_Key_STO"}))

		r := newTestEngine(newTestController(sqlxDB))
		req := httptest.NewRequest(http.MethodGet,
			"/interactions/compounds?compounds=Mirtazapin-0,5-Wasser", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, "Mirtazapin-0") || !strings.Contains(body, "5-Wasser") {
			t.Errorf("expected body to show the split fragments, got: %s", body)
		}
		if merr := mock.ExpectationsWereMet(); merr != nil {
			t.Errorf("sqlmock expectations not met (split not as documented): %v", merr)
		}
	})
}

// TestGetInterCompoundsValidation covers the validation/early-return paths that never
// reach the database. No mock expectations are set for these cases.
func TestGetInterCompoundsValidation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		rawQuery   string
		wantStatus int
		wantBody   string // substring expected in the response body ("" to skip)
	}{
		{
			name:       "missing compounds param -> 422",
			rawQuery:   "",
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "compounds",
		},
		{
			name:       "empty compounds value -> 422",
			rawQuery:   "compounds=",
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "compounds",
		},
		{
			name:       "single valid compound -> 400 at least two compounds",
			rawQuery:   "compounds=Aspirin",
			wantStatus: http.StatusBadRequest,
			wantBody:   "at least two compounds",
		},
		{
			name:       "duplicate compounds -> 400",
			rawQuery:   "compounds=Aspirin&compounds=Aspirin",
			wantStatus: http.StatusBadRequest,
			wantBody:   "duplicate compounds",
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

			r := newTestEngine(newTestController(sqlxDB))
			req := httptest.NewRequest(http.MethodGet, "/interactions/compounds?"+tt.rawQuery, nil)
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
