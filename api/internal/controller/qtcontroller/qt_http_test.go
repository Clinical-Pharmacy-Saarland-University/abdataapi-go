package qtcontroller_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/qtcontroller"
	"observeddb-go-api/internal/utils/format"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// newQTController builds a QTController wired to a mocked *sqlx.DB. The returned
// mock lets each test assert the exact SQL args and control the rows returned.
func newQTController(t *testing.T) (*qtcontroller.QTController, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	sqlxDB := sqlx.NewDb(db, "sqlmock")

	qc := &qtcontroller.QTController{
		DB:                 sqlxDB,
		Limits:             cfg.LimitsConfig{InteractionDrugs: 100},
		CategoryTranslator: format.NewQTCategoryTranslator(),
	}

	cleanup := func() { _ = db.Close() }

	return qc, mock, cleanup
}

// serveQT performs a GET /qt/pzns request with the given raw query string and
// returns the recorder for assertions.
func serveQT(qc *qtcontroller.QTController, rawQuery string) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/qt/pzns", qc.GetQTStatus)

	target := "/qt/pzns"
	if rawQuery != "" {
		target += "?" + rawQuery
	}

	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// jsendData is the {"status": ..., "data": ...} envelope produced by
// handle.Success. On the success path Data holds the list of QTResponse.
type jsendData struct {
	Status string                    `json:"status"`
	Data   []qtcontroller.QTResponse `json:"data"`
}

// TestGetQTStatus_InvalidPZN verifies that an invalid PZN is rejected with 400
// before any DB query runs. validate.PZN is called for every token prior to the
// query, so no ExpectQuery is registered and ExpectationsWereMet must still pass.
func TestGetQTStatus_InvalidPZN(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		rawQuery string
	}{
		{
			name:     "too short and bad checksum",
			rawQuery: "pzns=123",
		},
		{
			name:     "valid then invalid token still rejected",
			rawQuery: "pzns=03041347,123",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			qc, mock, cleanup := newQTController(t)
			defer cleanup()

			// No ExpectQuery: the handler must bail out during validation,
			// before touching the database.

			w := serveQT(qc, tt.rawQuery)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d (body: %s)",
					http.StatusBadRequest, w.Code, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unexpected DB interaction on invalid input: %v", err)
			}
		})
	}
}

// TestGetQTStatus_MissingParam verifies that omitting the required pzns
// parameter is rejected during binding (before any DB query). The `required`
// binding tag produces a validator error, so handle surfaces it as a 422
// validation failure rather than a plain 400.
func TestGetQTStatus_MissingParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	qc, mock, cleanup := newQTController(t)
	defer cleanup()

	w := serveQT(qc, "")

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusUnprocessableEntity, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction on missing param: %v", err)
	}
}

// TestGetQTStatus_Success verifies the happy path: two valid PZNs are queried,
// one has a matching Key_SGR row (translated to its category) and the other has
// no row (defaulting to "unknown"). The response preserves input order.
func TestGetQTStatus_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	qc, mock, cleanup := newQTController(t)
	defer cleanup()

	// The handler splits "03041347,05538454" into two PZNs, both passing
	// validation, then issues a single query against FAI_DB. The IN clause
	// binds the two PZNs followed by the three Key_SGR category codes.
	rows := sqlmock.NewRows([]string{"PZN", "Key_SGR"}).
		AddRow("03041347", 10079780) // known risk; 05538454 intentionally absent -> unknown

	mock.ExpectQuery("FROM FAI_DB").
		WithArgs("03041347", "05538454", 10079780, 10079781, 10079782).
		WillReturnRows(rows)

	w := serveQT(qc, "pzns=03041347,05538454")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}

	var got jsendData
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body %q: %v", w.Body.String(), err)
	}

	if got.Status != "success" {
		t.Errorf("expected status \"success\", got %q", got.Status)
	}

	want := []qtcontroller.QTResponse{
		{PZN: "03041347", QTCategory: "known risk"},
		{PZN: "05538454", QTCategory: "unknown"},
	}

	if len(got.Data) != len(want) {
		t.Fatalf("expected %d results, got %d: %+v", len(want), len(got.Data), got.Data)
	}
	for i := range want {
		if got.Data[i] != want[i] {
			t.Errorf("result[%d] = %+v, want %+v", i, got.Data[i], want[i])
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations (args split or query mismatch?): %v", err)
	}
}

// TestGetQTStatus_AllCategories verifies each Key_SGR code translates to its
// expected non-detailed category string.
func TestGetQTStatus_AllCategories(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// PZNs used purely as row keys; all pass the checksum validation.
	const (
		pznKnown       = "03041347"
		pznPossible    = "05538454"
		pznConditional = "00000000"
	)

	qc, mock, cleanup := newQTController(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{"PZN", "Key_SGR"}).
		AddRow(pznKnown, 10079780).
		AddRow(pznPossible, 10079781).
		AddRow(pznConditional, 10079782)

	mock.ExpectQuery("FROM FAI_DB").
		WithArgs(pznKnown, pznPossible, pznConditional, 10079780, 10079781, 10079782).
		WillReturnRows(rows)

	w := serveQT(qc, "pzns="+pznKnown+","+pznPossible+","+pznConditional)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}

	var got jsendData
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body %q: %v", w.Body.String(), err)
	}

	want := map[string]string{
		pznKnown:       "known risk",
		pznPossible:    "possible risk",
		pznConditional: "conditional risk",
	}

	if len(got.Data) != len(want) {
		t.Fatalf("expected %d results, got %d: %+v", len(want), len(got.Data), got.Data)
	}
	for _, r := range got.Data {
		if wantCat, ok := want[r.PZN]; !ok {
			t.Errorf("unexpected PZN in response: %q", r.PZN)
		} else if r.QTCategory != wantCat {
			t.Errorf("PZN %q: category = %q, want %q", r.PZN, r.QTCategory, wantCat)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}
