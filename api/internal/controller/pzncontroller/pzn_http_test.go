package pzncontroller_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/pzncontroller"
	"observeddb-go-api/internal/utils/format"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

func newPZNTestController(t *testing.T) (*pzncontroller.PZNController, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	controller := &pzncontroller.PZNController{
		DB:                 sqlxDB,
		Limits:             cfg.LimitsConfig{InteractionDrugs: 100},
		CategoryTranslator: format.NewProductTranslator(),
	}

	cleanup := func() {
		_ = db.Close()
	}

	return controller, mock, cleanup
}

func TestGetActiveCompounds_InvalidPZN(t *testing.T) {
	gin.SetMode(gin.TestMode)

	controller, mock, cleanup := newPZNTestController(t)
	defer cleanup()

	r := gin.New()
	r.GET("/product/activecompounds/pzns", controller.GetActiveCompounds)

	req := httptest.NewRequest(http.MethodGet, "/product/activecompounds/pzns?pzns=123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Invalid PZN") {
		t.Errorf("expected body to contain 'Invalid PZN', got %s", w.Body.String())
	}

	// No query should have been issued on the invalid-PZN path.
	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}

func TestGetActiveCompounds_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	controller, mock, cleanup := newPZNTestController(t)
	defer cleanup()

	cols := []string{"PZN", "Name", "Typ", "Herkunft", "Vorzugsbezeichnung", "Key_STO"}
	// The query carries two args: the PZN (IN clause) and the Stofftyp = 1 filter.
	mock.ExpectQuery("FROM PAE_DB").
		WithArgs("03041347", 1).
		WillReturnRows(sqlmock.NewRows(cols))

	r := gin.New()
	r.GET("/product/activecompounds/pzns", controller.GetActiveCompounds)

	req := httptest.NewRequest(http.MethodGet, "/product/activecompounds/pzns?pzns=03041347", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "PZN not found") {
		t.Errorf("expected body to contain 'PZN not found', got %s", w.Body.String())
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}

func TestGetActiveCompounds_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	controller, mock, cleanup := newPZNTestController(t)
	defer cleanup()

	cols := []string{"PZN", "Name", "Typ", "Herkunft", "Vorzugsbezeichnung", "Key_STO"}
	rows := sqlmock.NewRows(cols).
		AddRow("03041347", "Ibuprofen", "100", "ABDA;WHO", true, uint64(42))
	// The query carries two args: the PZN (IN clause) and the Stofftyp = 1 filter.
	mock.ExpectQuery("FROM PAE_DB").
		WithArgs("03041347", 1).
		WillReturnRows(rows)

	r := gin.New()
	r.GET("/product/activecompounds/pzns", controller.GetActiveCompounds)

	req := httptest.NewRequest(http.MethodGet, "/product/activecompounds/pzns?pzns=03041347", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Ibuprofen") {
		t.Errorf("expected body to contain 'Ibuprofen', got %s", body)
	}
	if !strings.Contains(body, "03041347") {
		t.Errorf("expected body to contain the PZN, got %s", body)
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}

func TestGetProductInfo_InvalidPZN(t *testing.T) {
	gin.SetMode(gin.TestMode)

	controller, mock, cleanup := newPZNTestController(t)
	defer cleanup()

	r := gin.New()
	r.GET("/product/info/pzns", controller.GetProductInfo)

	req := httptest.NewRequest(http.MethodGet, "/product/info/pzns?pzns=1234567a", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Invalid PZN") {
		t.Errorf("expected body to contain 'Invalid PZN', got %s", w.Body.String())
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}

func TestGetProductInfo_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	controller, mock, cleanup := newPZNTestController(t)
	defer cleanup()

	cols := []string{"PZN", "Produktgruppe", "Monopraeparat", "Produktname"}
	mock.ExpectQuery("FROM PAE_DB").
		WithArgs("05538454").
		WillReturnRows(sqlmock.NewRows(cols))

	r := gin.New()
	r.GET("/product/info/pzns", controller.GetProductInfo)

	req := httptest.NewRequest(http.MethodGet, "/product/info/pzns?pzns=05538454", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "PZN not found") {
		t.Errorf("expected body to contain 'PZN not found', got %s", w.Body.String())
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}

func TestGetProductInfo_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	controller, mock, cleanup := newPZNTestController(t)
	defer cleanup()

	cols := []string{"PZN", "Produktgruppe", "Monopraeparat", "Produktname"}
	// Produktgruppe must be a code known to format.NewProductTranslator() (1-8),
	// because the handler dereferences the translated result. 1 -> "Drug".
	rows := sqlmock.NewRows(cols).
		AddRow("05538454", uint64(1), uint64(1), "SomeProduct")
	mock.ExpectQuery("FROM PAE_DB").
		WithArgs("05538454").
		WillReturnRows(rows)

	r := gin.New()
	r.GET("/product/info/pzns", controller.GetProductInfo)

	req := httptest.NewRequest(http.MethodGet, "/product/info/pzns?pzns=05538454", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "SomeProduct") {
		t.Errorf("expected body to contain 'SomeProduct', got %s", body)
	}
	if !strings.Contains(body, "Drug") {
		t.Errorf("expected body to contain translated category 'Drug', got %s", body)
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}
