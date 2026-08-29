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

	seedCols := []string{"Key_FAM", "Key_IND_Haupt", "Key_IND_Neben", "Key_ATC", "Key_ATCA"}
	mock.ExpectQuery("FROM FAM_DB").
		WillReturnRows(sqlmock.NewRows(seedCols).
			AddRow(uint64(10), "02A01", "03B", "N02AA01", "N02AA59"))

	relationCols := []string{"Key_IND_Quelle", "Key_IND_Ziel"}
	mock.ExpectQuery("FROM INV_DB").
		WillReturnRows(sqlmock.NewRows(relationCols).
			AddRow("02A", "02A01"))

	nameCols := []string{"Key_IND", "indication_name", "indication_language"}
	mock.ExpectQuery("FROM IND_DB").
		WillReturnRows(sqlmock.NewRows(nameCols).
			AddRow("02A", "Opioids", "en").
			AddRow("02A01", "Morphine-type opioids", "en").
			AddRow("03B", "Weitere Indikation", "de"))

	atcCols := []string{"atc_code", "level", "label_en", "source_year", "source_url"}
	mock.ExpectQuery("FROM who_atc_mapping").
		WillReturnRows(sqlmock.NewRows(atcCols).
			AddRow("N", 1, "nervous system", 2026, "https://atcddd.fhi.no/atc_ddd_index/").
			AddRow("N02", 2, "analgesics", 2026, "https://atcddd.fhi.no/atc_ddd_index/").
			AddRow("N02A", 3, "opioids", 2026, "https://atcddd.fhi.no/atc_ddd_index/").
			AddRow("N02AA", 4, "natural opium alkaloids", 2026, "https://atcddd.fhi.no/atc_ddd_index/").
			AddRow("N02AA01", 5, "morphine", 2026, "https://atcddd.fhi.no/atc_ddd_index/").
			AddRow("N02AA59", 5, "codeine, combinations excl. psycholeptics", 2026, "https://atcddd.fhi.no/atc_ddd_index/"))

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
	if got.Data[0].Indications[0].Language != "en" {
		t.Errorf("indication language = %q, want en", got.Data[0].Indications[0].Language)
	}
	if got.Data[0].Indications[0].Name != "Opioids" {
		t.Errorf("indication name = %q, want Opioids", got.Data[0].Indications[0].Name)
	}
	if got.Data[0].Indications[2].Language != "de" {
		t.Errorf("fallback indication language = %q, want de", got.Data[0].Indications[2].Language)
	}
	if len(got.Data[0].Indications[0].ATCCodes) != 6 {
		t.Fatalf("expected 6 ATC hierarchy codes, got %#v", got.Data[0].Indications[0].ATCCodes)
	}
	expectedCodes := []string{"N", "N02", "N02A", "N02AA", "N02AA01", "N02AA59"}
	for index, expectedCode := range expectedCodes {
		if got.Data[0].Indications[0].ATCCodes[index].Code != expectedCode {
			t.Errorf("ATC code at index %d = %q, want %q", index, got.Data[0].Indications[0].ATCCodes[index].Code, expectedCode)
		}
	}
	if got.Data[0].Indications[0].ATCCodes[4].LabelEN != "morphine" {
		t.Errorf("level-5 label_en = %q, want morphine", got.Data[0].Indications[0].ATCCodes[4].LabelEN)
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}

func TestGetProductInfo_WithIndicationsMissingATCMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	controller, mock, cleanup := newPZNTestController(t)
	defer cleanup()

	productCols := []string{"PZN", "Key_FAM", "Produktgruppe", "Monopraeparat", "Produktname"}
	mock.ExpectQuery("FROM PAE_DB").
		WithArgs("05538454").
		WillReturnRows(sqlmock.NewRows(productCols).
			AddRow("05538454", uint64(10), uint64(1), uint64(1), "SomeProduct"))

	seedCols := []string{"Key_FAM", "Key_IND_Haupt", "Key_IND_Neben", "Key_ATC", "Key_ATCA"}
	mock.ExpectQuery("FROM FAM_DB").
		WillReturnRows(sqlmock.NewRows(seedCols).
			AddRow(uint64(10), "02A01", nil, "N02AA01", nil))
	mock.ExpectQuery("FROM INV_DB").
		WillReturnRows(sqlmock.NewRows([]string{"Key_IND_Quelle", "Key_IND_Ziel"}))
	mock.ExpectQuery("FROM IND_DB").
		WillReturnRows(sqlmock.NewRows([]string{"Key_IND", "indication_name", "indication_language"}).
			AddRow("02A01", "Morphine-type opioids", "en"))
	mock.ExpectQuery("FROM who_atc_mapping").
		WillReturnRows(sqlmock.NewRows([]string{"atc_code", "level", "label_en", "source_year", "source_url"}))

	r := gin.New()
	r.GET("/product/info/pzns", controller.GetProductInfo)

	req := httptest.NewRequest(http.MethodGet, "/product/info/pzns?pzns=05538454&indications=true&lang=english", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, `"code":"N02AA01"`) {
		t.Errorf("expected ATC code without label, got %s", body)
	}
	if strings.Contains(body, "label_en") {
		t.Errorf("expected missing label to be omitted, got %s", body)
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

	seedCols := []string{"Key_FAM", "Key_IND_Haupt", "Key_IND_Neben", "Key_ATC", "Key_ATCA"}
	mock.ExpectQuery("FROM FAM_DB").
		WillReturnRows(sqlmock.NewRows(seedCols).
			AddRow(uint64(10), "02A01", nil, "N02AA01", nil))
	mock.ExpectQuery("FROM INV_DB").
		WillReturnRows(sqlmock.NewRows([]string{"Key_IND_Quelle", "Key_IND_Ziel"}))
	mock.ExpectQuery("FROM IND_DB").
		WillReturnRows(sqlmock.NewRows([]string{"Key_IND", "indication_name", "indication_language"}).
			AddRow("02A01", "Morphine-type opioids", "en"))
	mock.ExpectQuery("FROM who_atc_mapping").
		WillReturnRows(sqlmock.NewRows([]string{"atc_code", "level", "label_en", "source_year", "source_url"}).
			AddRow("N", 1, "nervous system", 2026, "https://atcddd.fhi.no/atc_ddd_index/").
			AddRow("N02", 2, "analgesics", 2026, "https://atcddd.fhi.no/atc_ddd_index/").
			AddRow("N02A", 3, "opioids", 2026, "https://atcddd.fhi.no/atc_ddd_index/").
			AddRow("N02AA", 4, "natural opium alkaloids", 2026, "https://atcddd.fhi.no/atc_ddd_index/").
			AddRow("N02AA01", 5, "morphine", 2026, "https://atcddd.fhi.no/atc_ddd_index/"))

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
	if !strings.Contains(w.Body.String(), `"name":"Morphine-type opioids","language":"en"`) {
		t.Errorf("expected reviewed English indication, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"label_en":"morphine"`) {
		t.Errorf("expected WHO ATC label, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"code":"N02AA","label_en":"natural opium alkaloids","level":4`) {
		t.Errorf("expected intermediate WHO ATC level, got %s", w.Body.String())
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unfulfilled sqlmock expectations: %v", merr)
	}
}
