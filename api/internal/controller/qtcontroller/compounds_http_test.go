package qtcontroller_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/qtcontroller"
	"observeddb-go-api/internal/utils/format"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// newQTCompoundController builds a QTController wired to a mocked *sqlx.DB for the
// compound endpoint. BatchQueries governs the compound-count limit, so it is set
// generously here to keep the tests focused on the list-splitting behavior.
func newQTCompoundController(t *testing.T) (*qtcontroller.QTController, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	sqlxDB := sqlx.NewDb(db, "sqlmock")

	qc := &qtcontroller.QTController{
		DB:                 sqlxDB,
		Limits:             cfg.LimitsConfig{BatchQueries: 100},
		CategoryTranslator: format.NewQTCategoryTranslator(),
	}

	cleanup := func() { _ = db.Close() }

	return qc, mock, cleanup
}

// serveQTCompounds performs a GET /qt/compounds request with the given raw query
// string and returns the recorder for assertions.
func serveQTCompounds(qc *qtcontroller.QTController, rawQuery string) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/qt/compounds", qc.GetQTStatusByCompound)

	target := "/qt/compounds"
	if rawQuery != "" {
		target += "?" + rawQuery
	}

	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// jsendFail mirrors the {"status": "fail", "data": {"error": ...}} envelope that
// handle.BadRequestError produces on the 400 failure path.
type jsendFail struct {
	Status string `json:"status"`
	Data   struct {
		Error string `json:"error"`
	} `json:"data"`
}

// jsendCompoundData is the success envelope for the compound endpoint; on success
// Data holds the list of QTCompoundResult.
type jsendCompoundData struct {
	Status string                          `json:"status"`
	Data   []qtcontroller.QTCompoundResult `json:"data"`
}

// TestGetQTStatusByCompound_MissingParam verifies that omitting the compounds
// parameter entirely is rejected with a 400 before any DB query runs. No
// ExpectQuery is registered, so ExpectationsWereMet must still pass.
func TestGetQTStatusByCompound_MissingParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	qc, mock, cleanup := newQTCompoundController(t)
	defer cleanup()

	// No ExpectQuery: the handler must bail out on the empty list before
	// touching the database.

	w := serveQTCompounds(qc, "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusBadRequest, w.Code, w.Body.String())
	}

	var got jsendFail
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body %q: %v", w.Body.String(), err)
	}
	if !strings.Contains(got.Data.Error, "Missing required parameter: compounds") {
		t.Errorf("expected error message to contain %q, got %q",
			"Missing required parameter: compounds", got.Data.Error)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction on missing param: %v", err)
	}
}

// TestGetQTStatusByCompound_RepeatedParamPreservesComma verifies the core
// comma-fix: when the list is supplied as two repeated params and one item
// contains a comma ("Mirtazapin-0,5-Wasser"), that item is kept intact rather
// than being split into "Mirtazapin-0" and "5-Wasser".
//
// StoToCompoundsMap lowercases and trims each name before building the SNA_DB
// query, so the expected bound arg is the lowercased-but-intact form
// "mirtazapin-0,5-wasser" (a single arg containing the comma), proving no split
// occurred. Returning zero SNA_DB rows short-circuits the follow-up SZG_DB query,
// so only this one query is expected.
func TestGetQTStatusByCompound_RepeatedParamPreservesComma(t *testing.T) {
	gin.SetMode(gin.TestMode)

	qc, mock, cleanup := newQTCompoundController(t)
	defer cleanup()

	// Two bound args means the comma-bearing name stayed whole; three would mean
	// it was wrongly split on the comma.
	mock.ExpectQuery("FROM SNA_DB").
		WithArgs("apixaban", "mirtazapin-0,5-wasser").
		WillReturnRows(sqlmock.NewRows([]string{"Name", "DDI_Key_STO"}))

	rawQuery := "compounds=Apixaban&compounds=" + url.QueryEscape("Mirtazapin-0,5-Wasser")

	w := serveQTCompounds(qc, rawQuery)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}

	var got jsendCompoundData
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body %q: %v", w.Body.String(), err)
	}
	if got.Status != "success" {
		t.Errorf("expected status \"success\", got %q", got.Status)
	}

	// The response echoes the two intact input names (order preserved); the
	// comma-bearing name must survive as a single item.
	want := []qtcontroller.QTCompoundResult{
		{Input: "Apixaban", QTCategory: "unknown"},
		{Input: "Mirtazapin-0,5-Wasser", QTCategory: "unknown"},
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
		t.Errorf("unmet sqlmock expectations (comma split or query mismatch?): %v", err)
	}
}

// TestGetQTStatusByCompound_LegacyCommaJoinedSplits verifies the legacy transport
// path: a single comma-joined value ?compounds=A,B is split into two names, so
// the SNA_DB query binds two args (the lowercased "a" and "b").
func TestGetQTStatusByCompound_LegacyCommaJoinedSplits(t *testing.T) {
	gin.SetMode(gin.TestMode)

	qc, mock, cleanup := newQTCompoundController(t)
	defer cleanup()

	// A single comma-joined value is split into two names -> two bound args.
	mock.ExpectQuery("FROM SNA_DB").
		WithArgs("apixaban", "mirtazapin").
		WillReturnRows(sqlmock.NewRows([]string{"Name", "DDI_Key_STO"}))

	w := serveQTCompounds(qc, "compounds="+url.QueryEscape("Apixaban,Mirtazapin"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}

	var got jsendCompoundData
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body %q: %v", w.Body.String(), err)
	}

	// Two intact input names come back, confirming the value was split into two.
	want := []qtcontroller.QTCompoundResult{
		{Input: "Apixaban", QTCategory: "unknown"},
		{Input: "Mirtazapin", QTCategory: "unknown"},
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
		t.Errorf("unmet sqlmock expectations (legacy split mismatch?): %v", err)
	}
}
