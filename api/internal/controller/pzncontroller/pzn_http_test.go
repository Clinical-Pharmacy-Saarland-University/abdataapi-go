package pzncontroller_test

import (
	"encoding/json"
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
		Limits:             cfg.LimitsConfig{InteractionDrugs: 100, BatchQueries: 50},
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

	cols := []string{"PZN", "Key_FAM", "Produktgruppe", "Monopraeparat", "Produktname"}
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

	cols := []string{"PZN", "Key_FAM", "Produktgruppe", "Monopraeparat", "Produktname"}
	// Produktgruppe must be a code known to format.NewProductTranslator() (1-8),
	// because the handler dereferences the translated result. 1 -> "Drug".
	rows := sqlmock.NewRows(cols).
		AddRow("05538454", uint64(10), uint64(1), uint64(1), "SomeProduct")
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
	if strings.Contains(body, "indications") {
		t.Errorf("expected indications to be omitted by default, got %s", body)
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}

func TestGetProductInfo_WithIndications(t *testing.T) {
	gin.SetMode(gin.TestMode)

	controller, mock, cleanup := newPZNTestController(t)
	defer cleanup()

	productCols := []string{"PZN", "Key_FAM", "Produktgruppe", "Monopraeparat", "Produktname"}
	mock.ExpectQuery("FROM PAE_DB").
		WithArgs("05538454").
		WillReturnRows(sqlmock.NewRows(productCols).
			AddRow("05538454", uint64(10), uint64(1), uint64(1), "SomeProduct"))

	seedCols := []string{"Key_FAM", "Key_MIV"}
	mock.ExpectQuery("FROM IND_C").
		WillReturnRows(sqlmock.NewRows(seedCols).
			AddRow(uint64(10), 100).
			AddRow(uint64(10), 100).
			AddRow(uint64(10), 200).
			AddRow(uint64(10), 300))

	nameCols := []string{"Key_MIV", "Name", "Sprache", "Vorzugsbezeichnung"}
	mock.ExpectQuery("FROM MIN_C").
		WillReturnRows(sqlmock.NewRows(nameCols).
			AddRow(100, "Opioide", 1, true).
			AddRow(100, "Opioids", 2, false).
			AddRow(200, "Morphinartige Opioide", 1, true).
			AddRow(200, "Morphine-type opioids", 2, false).
			AddRow(300, "Weitere Indikation", 1, true))

	r := gin.New()
	r.GET("/product/info/pzns", controller.GetProductInfo)

	req := httptest.NewRequest(http.MethodGet, "/product/info/pzns?pzns=05538454&indications=true&lang=english", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var got struct {
		Data []pzncontroller.ProductInfo `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode body %q: %v", w.Body.String(), err)
	}
	if len(got.Data) != 1 {
		t.Fatalf("expected 1 product, got %d", len(got.Data))
	}
	if len(got.Data[0].Indications) != 3 {
		t.Fatalf("expected 3 indications, got %#v", got.Data[0].Indications)
	}
	if got.Data[0].Indications[0] != "Opioids" {
		t.Errorf("indication name = %q, want Opioids", got.Data[0].Indications[0])
	}
	if got.Data[0].Indications[2] != "Weitere Indikation" {
		t.Errorf("fallback indication name = %q, want Weitere Indikation", got.Data[0].Indications[2])
	}
	if strings.Contains(w.Body.String(), "atc_codes") {
		t.Errorf("expected ATC data to be omitted, got %s", w.Body.String())
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}

func TestGetProductInfo_IndicationsContainNoATCData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	controller, mock, cleanup := newPZNTestController(t)
	defer cleanup()

	productCols := []string{"PZN", "Key_FAM", "Produktgruppe", "Monopraeparat", "Produktname"}
	mock.ExpectQuery("FROM PAE_DB").
		WithArgs("05538454").
		WillReturnRows(sqlmock.NewRows(productCols).
			AddRow("05538454", uint64(10), uint64(1), uint64(1), "SomeProduct"))

	seedCols := []string{"Key_FAM", "Key_MIV"}
	mock.ExpectQuery("FROM IND_C").
		WillReturnRows(sqlmock.NewRows(seedCols).
			AddRow(uint64(10), 100))
	mock.ExpectQuery("FROM MIN_C").
		WillReturnRows(sqlmock.NewRows([]string{"Key_MIV", "Name", "Sprache", "Vorzugsbezeichnung"}).
			AddRow(100, "Morphinartige Opioide", 1, true).
			AddRow(100, "Morphine-type opioids", 2, false))
	r := gin.New()
	r.GET("/product/info/pzns", controller.GetProductInfo)

	req := httptest.NewRequest(http.MethodGet, "/product/info/pzns?pzns=05538454&indications=true&lang=english", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	body := w.Body.String()
	if strings.Contains(body, "atc") || strings.Contains(body, "label_en") {
		t.Errorf("expected indication output without ATC data, got %s", body)
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}

func TestGetProductSearch_WithIndications(t *testing.T) {
	gin.SetMode(gin.TestMode)

	controller, mock, cleanup := newPZNTestController(t)
	defer cleanup()

	productRows := sqlmock.NewRows([]string{"Key_FAM", "Produktname", "PZN"}).
		AddRow(uint64(10), "Morphin", "05538454")
	mock.ExpectQuery("FROM FAM_DB").
		WithArgs("%morphin%").
		WillReturnRows(productRows)

	compoundRows := sqlmock.NewRows([]string{"Key_FAM", "Key_STO", "Name", "Key_STO_1"}).
		AddRow(uint64(10), "S1", "Morphin", nil)
	mock.ExpectQuery("FROM FAI_DB").
		WillReturnRows(compoundRows)

	seedCols := []string{"Key_FAM", "Key_MIV"}
	mock.ExpectQuery("FROM IND_C").
		WillReturnRows(sqlmock.NewRows(seedCols).
			AddRow(uint64(10), 100))
	mock.ExpectQuery("FROM MIN_C").
		WillReturnRows(sqlmock.NewRows([]string{"Key_MIV", "Name", "Sprache", "Vorzugsbezeichnung"}).
			AddRow(100, "Morphinartige Opioide", 1, true).
			AddRow(100, "Morphine-type opioids", 2, false))
	r := gin.New()
	r.GET("/product/search", controller.GetProductSearch)

	req := httptest.NewRequest(http.MethodGet, "/product/search?name=Morphin&indications=true&lang=english", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"indications"`) {
		t.Errorf("expected indications in product search response, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"indications":["Morphine-type opioids"]`) {
		t.Errorf("expected English MIN_C indication, got %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"language"`) {
		t.Errorf("expected indication output without a language field, got %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "atc_codes") {
		t.Errorf("expected ATC data to be omitted, got %s", w.Body.String())
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}
