package interactioncontroller_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/interactioncontroller"
	"observeddb-go-api/internal/utils/format"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// newBatchTestEngine registers the two POST batch handlers and the description handler without
// auth middleware, so requests hit the controller logic directly.
func newBatchTestEngine(ic *interactioncontroller.InteractionController) *gin.Engine {
	r := gin.New()
	r.POST("/interactions/compounds", ic.PostInterCompounds)
	r.POST("/interactions/pzns", ic.PostInterPZNs)
	r.GET("/interactions/description", ic.GetInterDescription)
	return r
}

// newSmallBatchController builds a controller with a low BatchQueries limit so the "too many IDs"
// path is easy to trigger. BatchJobs stays > 0 so the semaphore is usable.
func newSmallBatchController(db *sqlx.DB) *interactioncontroller.InteractionController {
	return &interactioncontroller.InteractionController{
		DB:     db,
		Limits: cfg.LimitsConfig{InteractionDrugs: 100, BatchQueries: 2, BatchJobs: 4},
	}
}

// TestPostInterCompoundsValidation covers the request-level validation paths (bad JSON, duplicate
// IDs, too many IDs). None of these should issue a DB query.
func TestPostInterCompoundsValidation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "malformed JSON -> 400",
			body:       `{not json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "duplicate ids -> 400",
			body:       `[{"id":"1","compounds":["A","B"]},{"id":"1","compounds":["C","D"]}]`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Duplicate IDs",
		},
		{
			name: "too many ids -> 400",
			body: `[{"id":"1","compounds":["A","B"]},` +
				`{"id":"2","compounds":["C","D"]},` +
				`{"id":"3","compounds":["E","F"]}]`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Too many IDs",
		},
		{
			// Well-formed array whose element is missing the required `id`: a field
			// validation error, which must be 422 (not 400), matching single-object
			// endpoints and the documented "Bad query format" response.
			name:       "missing required field -> 422",
			body:       `[{"compounds":["A","B"]}]`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "errors",
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

			r := newBatchTestEngine(newSmallBatchController(sqlxDB))
			req := httptest.NewRequest(http.MethodPost, "/interactions/compounds", strings.NewReader(tt.body))
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

// TestPostInterCompoundsPerEntryFailure covers the key batch case: distinct valid IDs where each
// entry's compound list has < 2 items. validate.Compounds fails per entry BEFORE any DB call, so
// no query is issued. The batch still returns 207 and each embedded result carries a non-OK status.
func TestPostInterCompoundsPerEntryFailure(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()
	sqlxDB := sqlx.NewDb(db, "sqlmock")
	// No expectations: each entry fails validation before touching the DB.

	body := `[{"id":"1","compounds":["Solo"]},{"id":"2","compounds":["Alone"]}]`

	r := newBatchTestEngine(newTestController(sqlxDB))
	req := httptest.NewRequest(http.MethodPost, "/interactions/compounds", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMultiStatus {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusMultiStatus, w.Code, w.Body.String())
	}
	respBody := w.Body.String()
	// The embedded ResStatus serializes inline: OK results would show "status":200.
	if strings.Contains(respBody, `"status":200`) {
		t.Errorf("expected no per-result OK status, but found one; body: %s", respBody)
	}
	if !strings.Contains(respBody, `"status":400`) {
		t.Errorf("expected a per-result 400 status from failed validation, body: %s", respBody)
	}
	if !strings.Contains(respBody, "at least two compounds") {
		t.Errorf("expected per-result message about at least two compounds, body: %s", respBody)
	}
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("no DB query should have been issued, but sqlmock reports: %v", merr)
	}
}

// TestPostInterPZNsValidation covers request-level validation for the PZN batch endpoint.
func TestPostInterPZNsValidation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "malformed JSON -> 400",
			body:       `[{"id":`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "duplicate ids -> 400",
			body:       `[{"id":"1","pzns":["03041347","05538454"]},{"id":"1","pzns":["13880764","00189747"]}]`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Duplicate IDs",
		},
		{
			name: "too many ids -> 400",
			body: `[{"id":"1","pzns":["03041347","05538454"]},` +
				`{"id":"2","pzns":["13880764","00189747"]},` +
				`{"id":"3","pzns":["03041347","05538454"]}]`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "Too many IDs",
		},
		{
			// Missing required `id` is a field validation error -> 422, not 400.
			name:       "missing required field -> 422",
			body:       `[{"pzns":["03041347","05538454"]}]`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "errors",
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

			r := newBatchTestEngine(newSmallBatchController(sqlxDB))
			req := httptest.NewRequest(http.MethodPost, "/interactions/pzns", strings.NewReader(tt.body))
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

// TestPostInterPZNsPerEntryFailure covers the batch case where each entry's PZN list is invalid
// (single PZN => validate.PZNs fails on the min-2 check) before any DB call. The batch returns 207
// and each embedded result is non-OK, with no query issued.
func TestPostInterPZNsPerEntryFailure(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()
	sqlxDB := sqlx.NewDb(db, "sqlmock")
	// No expectations: each entry fails validate.PZNs before touching the DB.

	body := `[{"id":"1","pzns":["03041347"]},{"id":"2","pzns":["05538454"]}]`

	r := newBatchTestEngine(newTestController(sqlxDB))
	req := httptest.NewRequest(http.MethodPost, "/interactions/pzns", strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMultiStatus {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusMultiStatus, w.Code, w.Body.String())
	}
	respBody := w.Body.String()
	if strings.Contains(respBody, `"status":200`) {
		t.Errorf("expected no per-result OK status, but found one; body: %s", respBody)
	}
	if !strings.Contains(respBody, `"status":400`) {
		t.Errorf("expected a per-result 400 status from failed validation, body: %s", respBody)
	}
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("no DB query should have been issued, but sqlmock reports: %v", merr)
	}
}

// TestGetInterDescription verifies the static description endpoint returns 200 with the
// description payload built by format.Description().
func TestGetInterDescription(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()
	sqlxDB := sqlx.NewDb(db, "sqlmock")

	ic := newTestController(sqlxDB)
	ic.DescriptionStruct = format.Description()

	r := newBatchTestEngine(ic)
	req := httptest.NewRequest(http.MethodGet, "/interactions/description", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "description") {
		t.Errorf("expected body to contain a description field, got: %s", w.Body.String())
	}
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("no DB query should have been issued, but sqlmock reports: %v", merr)
	}
}
