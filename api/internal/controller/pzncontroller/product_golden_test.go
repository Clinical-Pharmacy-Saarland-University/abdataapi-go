package pzncontroller_test

// Golden tests that PIN the exact runtime behavior of the product endpoints
// slated for a behavior-preserving refactor: GetProductList, GetProductSearch
// (fetchProductsByName) and GetProductInfo. Each test asserts the exact SQL
// squirrel emits (matched with sqlmock.QueryMatcherEqual, which normalizes
// whitespace only), the ordered bind args, and the response status + JSON shape.
//
// These run against a mocked *sqlx.DB (go-sqlmock) with no network or real DB.

import (
	"encoding/json"
	"fmt"
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

// The exact SQL strings squirrel / the handlers emit. Captured from the dev
// source; any drift during refactor must be intentional and update these.
const (
	// fetchProductsByName step 1: the product lookup by fuzzy name.
	goldenProductSearchSQL = "SELECT DISTINCT FAM_DB.Key_FAM, FAM_DB.Produktname, PAE_DB.PZN " +
		"FROM FAM_DB JOIN PAE_DB ON PAE_DB.Key_FAM = FAM_DB.Key_FAM " +
		"WHERE PAE_DB.PZN IS NOT NULL " +
		"AND FAM_DB.Key_ATC IS NOT NULL " +
		"AND FAM_DB.Veterinaerpraeparat = 0 " +
		"AND LOWER(FAM_DB.Produktname) LIKE ? " +
		"ORDER BY FAM_DB.Produktname, PAE_DB.PZN LIMIT %d"

	// fetchProductsByName step 2: active compounds for the matched Key_FAMs.
	goldenCompoundSQL = "SELECT DISTINCT FAI_DB.Key_FAM, FAI_DB.Key_STO, SNA_DB.Name, VSS_DB.Key_STO_1 " +
		"FROM FAI_DB " +
		"LEFT JOIN VSS_DB ON VSS_DB.Key_STO_2 = FAI_DB.Key_STO " +
		"LEFT JOIN SNA_DB ON FAI_DB.Key_STO = SNA_DB.Key_STO " +
		"WHERE FAI_DB.Key_FAM IN (?,?) " +
		"AND FAI_DB.Stofftyp = 1 " +
		"AND SNA_DB.Vorzugsbezeichnung = 1 " +
		"AND SNA_DB.Key_STO NOT IN ( SELECT Key_STO_1 FROM VSS_DB WHERE Typ = 8 ) " +
		"ORDER BY FAI_DB.Key_FAM, FAI_DB.Key_STO"

	// GetProductList step 1: count of distinct PZNs.
	goldenListCountSQL = "SELECT COUNT(DISTINCT PZN) as count " +
		"FROM PAE_DB " +
		"LEFT JOIN FAM_DB ON PAE_DB.Key_FAM = FAM_DB.Key_FAM " +
		"WHERE PZN IS NOT NULL " +
		"AND KEY_ATC IS NOT NULL " +
		"AND FAM_DB.Veterinaerpraeparat = 0"

	// GetProductList step 2: paginated distinct PZNs (LIMIT/OFFSET inlined).
	goldenListPageSQL = "SELECT DISTINCT PZN " +
		"FROM PAE_DB " +
		"LEFT JOIN FAM_DB ON PAE_DB.Key_FAM = FAM_DB.Key_FAM " +
		"WHERE PZN IS NOT NULL " +
		"AND KEY_ATC IS NOT NULL " +
		"AND FAM_DB.Veterinaerpraeparat = 0 " +
		"ORDER BY PZN " +
		"LIMIT %d OFFSET %d"

	// GetProductList step 3: product + compound rows for the page's PZNs.
	goldenListProductsSQL = "SELECT DISTINCT FAM_DB.Produktname, FAM_DB.Key_ATC, PAE_DB.PZN, " +
		"SNA_DB.Key_STO, VSS_DB.Key_STO_1, VSS_DB.Key_STO_2, VSS_DB.Typ, SNA_DB.Name " +
		"FROM FAM_DB " +
		"LEFT JOIN PAE_DB ON FAM_DB.Key_FAM = PAE_DB.Key_FAM " +
		"LEFT JOIN FAI_DB ON FAI_DB.Key_FAM = FAM_DB.Key_FAM " +
		"LEFT JOIN VSS_DB ON VSS_DB.Key_STO_2 = FAI_DB.Key_STO " +
		"LEFT JOIN SNA_DB ON FAI_DB.Key_STO = SNA_DB.Key_STO " +
		"WHERE FAM_DB.Key_ATC IS NOT NULL " +
		"AND FAM_DB.Veterinaerpraeparat = 0 " +
		"AND FAI_DB.Stofftyp = 1 " +
		"AND PAE_DB.PZN IS NOT NULL " +
		"AND (VSS_DB.Typ IS NULL OR VSS_DB.Typ <> 100) " +
		"AND SNA_DB.Vorzugsbezeichnung = 1 " +
		"AND SNA_DB.Key_STO NOT IN ( SELECT Key_STO_1 FROM VSS_DB WHERE Typ = 8 ) " +
		"AND PAE_DB.PZN IN (?,?)"

	// GetProductInfo: product info by PZN.
	goldenProductInfoSQL = "SELECT DISTINCT PAE_DB.PZN, FAM_DB.Produktgruppe, FAM_DB.Monopraeparat, FAM_DB.Produktname " +
		"FROM PAE_DB " +
		"RIGHT JOIN FAM_DB ON PAE_DB.Key_FAM = FAM_DB.Key_FAM " +
		"WHERE PAE_DB.PZN IN (%s) " +
		"ORDER BY PAE_DB.PZN"
)

// newGoldenController builds a PZNController wired to a mocked *sqlx.DB using the
// exact-SQL matcher so tests can pin the full query text (whitespace-normalized).
func newGoldenController(t *testing.T) (*pzncontroller.PZNController, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	pc := &pzncontroller.PZNController{
		DB: sqlxDB,
		// BatchQueries caps the number of names accepted by GetProductSearch.
		Limits:             cfg.LimitsConfig{InteractionDrugs: 100, BatchQueries: 50},
		CategoryTranslator: format.NewProductTranslator(),
	}

	cleanup := func() { _ = db.Close() }

	return pc, mock, cleanup
}

func serve(handler gin.HandlerFunc, path, target string) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET(path, handler)

	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// --- GetProductSearch / fetchProductsByName ---------------------------------

// TestGolden_GetProductSearch_Normal pins the two-query search path for a single
// name: the fuzzy product lookup (LIKE %term%, LIMIT defaulting to 20) followed
// by the active-compound lookup keyed on the matched Key_FAMs. It also pins the
// grouped response shape (input + results with product_name/pzn/active_compounds)
// and the active-compound derivate filtering (Key_STO present in another row's
// Key_STO_1 is dropped).
func TestGolden_GetProductSearch_Normal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	// Two matched products -> Key_FAM 10 and 11.
	productRows := sqlmock.NewRows([]string{"Key_FAM", "Produktname", "PZN"}).
		AddRow(uint64(10), "Aspirin 100", "03041347").
		AddRow(uint64(11), "Aspirin plus C", "05538454")

	// Default limit is 20 (no ?limit given).
	mock.ExpectQuery(fmtSQL(goldenProductSearchSQL, 20)).
		WithArgs("%aspirin%").
		WillReturnRows(productRows)

	// Compound rows. For Key_FAM 10, Key_STO "S2" is derived from "S1"
	// (S2 appears as another row's Key_STO_1) so only "S1"/Acetylsalicylsaeure
	// survives. Key_FAM 11 has a single active compound.
	compoundRows := sqlmock.NewRows([]string{"Key_FAM", "Key_STO", "Name", "Key_STO_1"}).
		AddRow(uint64(10), "S1", "Acetylsalicylsaeure", nil).
		AddRow(uint64(10), "S2", "ASS-Derivat", "S2").
		AddRow(uint64(11), "S3", "Ascorbinsaeure", nil)

	// fams come from the product rows, in order: 10, 11. squirrel binds them
	// as ints (uint64 fitting int64 is converted by the driver value converter).
	mock.ExpectQuery(goldenCompoundSQL).
		WithArgs(int64(10), int64(11)).
		WillReturnRows(compoundRows)

	w := serve(pc.GetProductSearch, "/product/search", "/product/search?name=Aspirin")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	type searchEnvelope struct {
		Status string                              `json:"status"`
		Data   []pzncontroller.ProductSearchGroup `json:"data"`
	}
	var got searchEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode body %q: %v", w.Body.String(), err)
	}

	if got.Status != "success" {
		t.Errorf("status = %q, want \"success\"", got.Status)
	}
	if len(got.Data) != 1 {
		t.Fatalf("expected 1 group, got %d: %+v", len(got.Data), got.Data)
	}
	group := got.Data[0]
	if group.Input != "Aspirin" {
		t.Errorf("group.Input = %q, want %q", group.Input, "Aspirin")
	}
	if len(group.Results) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(group.Results), group.Results)
	}

	// First product keeps only the non-derivate compound.
	r0 := group.Results[0]
	if r0.ProductName != "Aspirin 100" || r0.PZN != "03041347" {
		t.Errorf("results[0] = {%q,%q}, want {\"Aspirin 100\",\"03041347\"}", r0.ProductName, r0.PZN)
	}
	if len(r0.ActiveCompounds) != 1 || r0.ActiveCompounds[0] != "Acetylsalicylsaeure" {
		t.Errorf("results[0].ActiveCompounds = %#v, want [\"Acetylsalicylsaeure\"]", r0.ActiveCompounds)
	}

	r1 := group.Results[1]
	if r1.ProductName != "Aspirin plus C" || r1.PZN != "05538454" {
		t.Errorf("results[1] = {%q,%q}, want {\"Aspirin plus C\",\"05538454\"}", r1.ProductName, r1.PZN)
	}
	if len(r1.ActiveCompounds) != 1 || r1.ActiveCompounds[0] != "Ascorbinsaeure" {
		t.Errorf("results[1].ActiveCompounds = %#v, want [\"Ascorbinsaeure\"]", r1.ActiveCompounds)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGolden_GetProductSearch_LimitClamp pins that an explicit limit is inlined
// verbatim into the product query's LIMIT clause (here 5), and that it is used
// as-is when within [1,100].
func TestGolden_GetProductSearch_LimitClamp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	// One matched product -> a single Key_FAM, so the compound IN clause has one arg.
	productRows := sqlmock.NewRows([]string{"Key_FAM", "Produktname", "PZN"}).
		AddRow(uint64(7), "Ramipril", "03041347")

	mock.ExpectQuery(fmtSQL(goldenProductSearchSQL, 5)).
		WithArgs("%ramipril%").
		WillReturnRows(productRows)

	compoundOneFam := "SELECT DISTINCT FAI_DB.Key_FAM, FAI_DB.Key_STO, SNA_DB.Name, VSS_DB.Key_STO_1 " +
		"FROM FAI_DB " +
		"LEFT JOIN VSS_DB ON VSS_DB.Key_STO_2 = FAI_DB.Key_STO " +
		"LEFT JOIN SNA_DB ON FAI_DB.Key_STO = SNA_DB.Key_STO " +
		"WHERE FAI_DB.Key_FAM IN (?) " +
		"AND FAI_DB.Stofftyp = 1 " +
		"AND SNA_DB.Vorzugsbezeichnung = 1 " +
		"AND SNA_DB.Key_STO NOT IN ( SELECT Key_STO_1 FROM VSS_DB WHERE Typ = 8 ) " +
		"ORDER BY FAI_DB.Key_FAM, FAI_DB.Key_STO"
	mock.ExpectQuery(compoundOneFam).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"Key_FAM", "Key_STO", "Name", "Key_STO_1"}).
			AddRow(uint64(7), "S9", "Ramipril", nil))

	w := serve(pc.GetProductSearch, "/product/search", "/product/search?name=Ramipril&limit=5")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGolden_GetProductSearch_EmptyResult pins that when the fuzzy lookup returns
// no products, no compound query is issued and the group carries an empty (but
// non-null) results array.
func TestGolden_GetProductSearch_EmptyResult(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	// No matched products; compound query must NOT be issued (len==0 short-circuit).
	mock.ExpectQuery(fmtSQL(goldenProductSearchSQL, 20)).
		WithArgs("%nonexistent%").
		WillReturnRows(sqlmock.NewRows([]string{"Key_FAM", "Produktname", "PZN"}))

	w := serve(pc.GetProductSearch, "/product/search", "/product/search?name=Nonexistent")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	// Empty results must serialize as [] (not null) per the make(...) init.
	body := w.Body.String()
	if wantSub := `"results":[]`; !containsJSON(body, wantSub) {
		t.Errorf("expected body to contain %q, got %s", wantSub, body)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGolden_GetProductSearch_MissingName pins that a missing name/q parameter is
// rejected with 400 before any DB query runs.
func TestGolden_GetProductSearch_MissingName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	w := serve(pc.GetProductSearch, "/product/search", "/product/search")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if !containsJSON(w.Body.String(), "Missing required parameter: name") {
		t.Errorf("expected 'Missing required parameter: name', got %s", w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction on missing name: %v", err)
	}
}

// TestGolden_GetProductSearch_QFallback pins the documented fallback: when `name`
// is absent the handler reads the `q` parameter instead, and still runs the same
// fuzzy product query.
func TestGolden_GetProductSearch_QFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	mock.ExpectQuery(fmtSQL(goldenProductSearchSQL, 20)).
		WithArgs("%plavix%").
		WillReturnRows(sqlmock.NewRows([]string{"Key_FAM", "Produktname", "PZN"}))

	w := serve(pc.GetProductSearch, "/product/search", "/product/search?q=Plavix")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// --- GetProductList ---------------------------------------------------------

// TestGolden_GetProductList_Normal pins the three-query pagination path for page
// 1: the COUNT query, the paginated PZN query (LIMIT 1000 OFFSET 0), and the
// product/compound query keyed on the page's PZNs. It also pins the response
// envelope (pages/page/pzns_per_page/products) and the KNOWN grouping behavior:
// the final PZN group is not flushed by the loop, so with two distinct PZNs only
// the first appears in products.
func TestGolden_GetProductList_Normal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	// 1500 distinct PZNs => ceil(1500/1000) = 2 pages.
	mock.ExpectQuery(goldenListCountSQL).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1500))

	// Page 1 => LIMIT 1000 OFFSET 0. Return two PZNs.
	mock.ExpectQuery(fmtSQL(goldenListPageSQL, 1000, 0)).
		WillReturnRows(sqlmock.NewRows([]string{"PZN"}).
			AddRow("03041347").
			AddRow("05538454"))

	// Product/compound rows for the two PZNs. squirrel binds the PZNs as strings.
	listCols := []string{
		"Produktname", "Key_ATC", "PZN", "Key_STO",
		"Key_STO_1", "Key_STO_2", "Typ", "Name",
	}
	mock.ExpectQuery(goldenListProductsSQL).
		WithArgs("03041347", "05538454").
		WillReturnRows(sqlmock.NewRows(listCols).
			AddRow("Aspirin", "B01AC06", "03041347", "S1", nil, nil, nil, "Acetylsalicylsaeure").
			AddRow("Plavix", "B01AC04", "05538454", "S2", nil, nil, nil, "Clopidogrel"))

	w := serve(pc.GetProductList, "/product/list", "/product/list?page=1")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	type listResult struct {
		ProductName     string   `json:"product"`
		ATC             string   `json:"atc"`
		PZN             *string  `json:"pzn"`
		ActiveCompounds []string `json:"active_compounds"`
	}
	type listEnvelope struct {
		Status string `json:"status"`
		Data   struct {
			Pages      int          `json:"pages"`
			Page       int          `json:"page"`
			PZNPerPage int          `json:"pzns_per_page"`
			Products   []listResult `json:"products"`
		} `json:"data"`
	}
	var got listEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode body %q: %v", w.Body.String(), err)
	}

	if got.Status != "success" {
		t.Errorf("status = %q, want \"success\"", got.Status)
	}
	if got.Data.Pages != 2 {
		t.Errorf("pages = %d, want 2", got.Data.Pages)
	}
	if got.Data.Page != 1 {
		t.Errorf("page = %d, want 1", got.Data.Page)
	}
	if got.Data.PZNPerPage != 1000 {
		t.Errorf("pzns_per_page = %d, want 1000", got.Data.PZNPerPage)
	}

	// KNOWN behavior: the grouping loop never flushes the final PZN group, so
	// with two distinct PZNs only the first ("03041347"/Aspirin) is emitted.
	if len(got.Data.Products) != 1 {
		t.Fatalf("expected 1 product (final group not flushed), got %d: %+v",
			len(got.Data.Products), got.Data.Products)
	}
	p := got.Data.Products[0]
	if p.PZN == nil || *p.PZN != "03041347" {
		t.Errorf("products[0].pzn = %v, want \"03041347\"", p.PZN)
	}
	if p.ProductName != "Aspirin" || p.ATC != "B01AC06" {
		t.Errorf("products[0] = {%q,%q}, want {\"Aspirin\",\"B01AC06\"}", p.ProductName, p.ATC)
	}
	if len(p.ActiveCompounds) != 1 || p.ActiveCompounds[0] != "Acetylsalicylsaeure" {
		t.Errorf("products[0].active_compounds = %#v, want [\"Acetylsalicylsaeure\"]", p.ActiveCompounds)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGolden_GetProductList_Pagination pins that page 2 shifts the OFFSET to
// (page-1)*pageSize = 1000 while keeping LIMIT 1000.
func TestGolden_GetProductList_Pagination(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	// 2500 PZNs => 3 pages.
	mock.ExpectQuery(goldenListCountSQL).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2500))

	// Page 2 => LIMIT 1000 OFFSET 1000.
	mock.ExpectQuery(fmtSQL(goldenListPageSQL, 1000, 1000)).
		WillReturnRows(sqlmock.NewRows([]string{"PZN"}).AddRow("03041347"))

	listCols := []string{
		"Produktname", "Key_ATC", "PZN", "Key_STO",
		"Key_STO_1", "Key_STO_2", "Typ", "Name",
	}
	// A single PZN on the page: with one distinct PZN the group is never
	// flushed, so products ends up empty.
	mock.ExpectQuery(goldenListProductsSQL[:len(goldenListProductsSQL)-len("IN (?,?)")]+"IN (?)").
		WithArgs("03041347").
		WillReturnRows(sqlmock.NewRows(listCols).
			AddRow("Aspirin", "B01AC06", "03041347", "S1", nil, nil, nil, "Acetylsalicylsaeure"))

	w := serve(pc.GetProductList, "/product/list", "/product/list?page=2")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var got struct {
		Data struct {
			Pages    int             `json:"pages"`
			Page     int             `json:"page"`
			Products []json.RawMessage `json:"products"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode body %q: %v", w.Body.String(), err)
	}
	if got.Data.Pages != 3 {
		t.Errorf("pages = %d, want 3", got.Data.Pages)
	}
	if got.Data.Page != 2 {
		t.Errorf("page = %d, want 2", got.Data.Page)
	}
	// One distinct PZN -> the loop never emits it.
	if len(got.Data.Products) != 0 {
		t.Errorf("expected 0 products (single group never flushed), got %d", len(got.Data.Products))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGolden_GetProductList_EmptyPage pins that a valid in-range page whose PZN
// query returns no rows yields 404 "No PZNs found for page N" (after the count
// and page queries, before the product query).
func TestGolden_GetProductList_EmptyPage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	// 1 page total; page 1 is in range but returns no PZNs.
	mock.ExpectQuery(goldenListCountSQL).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))
	mock.ExpectQuery(fmtSQL(goldenListPageSQL, 1000, 0)).
		WillReturnRows(sqlmock.NewRows([]string{"PZN"}))

	w := serve(pc.GetProductList, "/product/list", "/product/list?page=1")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
	}
	if !containsJSON(w.Body.String(), "No PZNs found for page 1") {
		t.Errorf("expected 'No PZNs found for page 1', got %s", w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGolden_GetProductList_PageOutOfRange pins that a page beyond the total page
// count yields 404 with "Page N not found, total pages: M" after only the count
// query has run (no page/product queries).
func TestGolden_GetProductList_PageOutOfRange(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	// 10 PZNs => 1 page. Requesting page 2 is out of range.
	mock.ExpectQuery(goldenListCountSQL).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

	w := serve(pc.GetProductList, "/product/list", "/product/list?page=2")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
	}
	if !containsJSON(w.Body.String(), "Page 2 not found, total pages: 1") {
		t.Errorf("expected 'Page 2 not found, total pages: 1', got %s", w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGolden_GetProductList_InvalidPage pins that a non-positive page (0 or
// negative or defaulted) is rejected with 400 "Page must be a positive integer",
// but only AFTER the count query runs (the count query precedes the page check).
func TestGolden_GetProductList_InvalidPage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		target string
	}{
		{name: "missing page defaults to 0", target: "/product/list"},
		{name: "explicit zero", target: "/product/list?page=0"},
		{name: "negative", target: "/product/list?page=-3"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			pc, mock, cleanup := newGoldenController(t)
			defer cleanup()

			// The count query always runs before the page validation.
			mock.ExpectQuery(goldenListCountSQL).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

			w := serve(pc.GetProductList, "/product/list", tt.target)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
			}
			if !containsJSON(w.Body.String(), "Page must be a positive integer") {
				t.Errorf("expected 'Page must be a positive integer', got %s", w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet sqlmock expectations: %v", err)
			}
		})
	}
}

// --- GetProductInfo / fetchProductInfo --------------------------------------

// TestGolden_GetProductInfo_Normal pins the single-query product-info path for
// one PZN, the IsComb derivation (Monopraeparat == 0), category translation
// (Produktgruppe 1 -> "Drug") and the response envelope shape.
func TestGolden_GetProductInfo_Normal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	cols := []string{"PZN", "Produktgruppe", "Monopraeparat", "Produktname"}
	// Monopraeparat 0 -> is_combination true; Produktgruppe 1 -> "Drug".
	mock.ExpectQuery(fmtSQL(goldenProductInfoSQL, "?")).
		WithArgs("03041347").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow("03041347", uint64(1), uint64(0), "Aspirin 100"))

	w := serve(pc.GetProductInfo, "/product/info/pzns", "/product/info/pzns?pzns=03041347")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	// NOTE: fetchProductInfo returns a flat []ProductInfo (the ProductInfos
	// wrapper struct is declared but NOT used here), so handle.Success serializes
	// data as a JSON array of product-info objects, not {"product_info": [...]}.
	type infoEnvelope struct {
		Status string                        `json:"status"`
		Data   []pzncontroller.ProductInfo `json:"data"`
	}
	var got infoEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode body %q: %v", w.Body.String(), err)
	}
	if got.Status != "success" {
		t.Errorf("status = %q, want \"success\"", got.Status)
	}
	if len(got.Data) != 1 {
		t.Fatalf("expected 1 product info, got %d: %+v", len(got.Data), got.Data)
	}
	pi := got.Data[0]
	if pi.PZN != "03041347" {
		t.Errorf("pzn = %q, want \"03041347\"", pi.PZN)
	}
	if !pi.IsComb {
		t.Errorf("is_combination = %v, want true (Monopraeparat 0)", pi.IsComb)
	}
	if pi.Category != "Drug" {
		t.Errorf("category = %q, want \"Drug\"", pi.Category)
	}
	if pi.ProductName != "Aspirin 100" {
		t.Errorf("product_name = %q, want \"Aspirin 100\"", pi.ProductName)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGolden_GetProductInfo_MultiplePZNs pins the multi-PZN IN clause (two
// placeholders, args in split order) and the Monopraeparat != 0 -> false mapping.
func TestGolden_GetProductInfo_MultiplePZNs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	cols := []string{"PZN", "Produktgruppe", "Monopraeparat", "Produktname"}
	mock.ExpectQuery(fmtSQL(goldenProductInfoSQL, "?,?")).
		WithArgs("03041347", "05538454").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow("03041347", uint64(1), uint64(0), "Aspirin").
			AddRow("05538454", uint64(2), uint64(1), "SomeDevice"))

	w := serve(pc.GetProductInfo, "/product/info/pzns", "/product/info/pzns?pzns=03041347,05538454")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var got struct {
		Data []pzncontroller.ProductInfo `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode body %q: %v", w.Body.String(), err)
	}
	if len(got.Data) != 2 {
		t.Fatalf("expected 2 product infos, got %d", len(got.Data))
	}
	// Second product: Monopraeparat 1 -> is_combination false; group 2 -> "Medical device".
	second := got.Data[1]
	if second.IsComb {
		t.Errorf("second.is_combination = true, want false (Monopraeparat 1)")
	}
	if second.Category != "Medical device" {
		t.Errorf("second.category = %q, want \"Medical device\"", second.Category)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGolden_GetProductInfo_NotFound pins the 404 "PZN not found" when the query
// returns no rows for a valid PZN.
func TestGolden_GetProductInfo_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	cols := []string{"PZN", "Produktgruppe", "Monopraeparat", "Produktname"}
	mock.ExpectQuery(fmtSQL(goldenProductInfoSQL, "?")).
		WithArgs("03041347").
		WillReturnRows(sqlmock.NewRows(cols))

	w := serve(pc.GetProductInfo, "/product/info/pzns", "/product/info/pzns?pzns=03041347")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusNotFound, w.Code, w.Body.String())
	}
	if !containsJSON(w.Body.String(), "PZN not found") {
		t.Errorf("expected 'PZN not found', got %s", w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// TestGolden_GetProductInfo_InvalidPZN pins that an invalid PZN is rejected with
// 400 "Invalid PZN: <pzn>" before any DB query. The whole batch is rejected if
// any token is invalid.
func TestGolden_GetProductInfo_InvalidPZN(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "too short",
			target: "/product/info/pzns?pzns=123",
			want:   "Invalid PZN: 123",
		},
		{
			name:   "valid then invalid still rejects whole batch",
			target: "/product/info/pzns?pzns=03041347,123",
			want:   "Invalid PZN: 123",
		},
		{
			name:   "non digit",
			target: "/product/info/pzns?pzns=1234567a",
			want:   "Invalid PZN: 1234567a",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			pc, mock, cleanup := newGoldenController(t)
			defer cleanup()

			// No ExpectQuery: validation must bail before the DB.
			w := serve(pc.GetProductInfo, "/product/info/pzns", tt.target)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d (body: %s)", http.StatusBadRequest, w.Code, w.Body.String())
			}
			if !containsJSON(w.Body.String(), tt.want) {
				t.Errorf("expected %q, got %s", tt.want, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unexpected DB interaction on invalid PZN: %v", err)
			}
		})
	}
}

// TestGolden_GetProductInfo_MissingParam pins that omitting the required pzns
// parameter is rejected during binding (422 validation failure) with no DB query.
func TestGolden_GetProductInfo_MissingParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock, cleanup := newGoldenController(t)
	defer cleanup()

	w := serve(pc.GetProductInfo, "/product/info/pzns", "/product/info/pzns")

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusUnprocessableEntity, w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected DB interaction on missing param: %v", err)
	}
}

// --- helpers ----------------------------------------------------------------

// fmtSQL fills the printf-style placeholders in a golden SQL constant so tests
// can reuse a single canonical query string across limit/offset/IN variants.
func fmtSQL(tmpl string, args ...any) string {
	return fmt.Sprintf(tmpl, args...)
}

// containsJSON reports whether the response body contains the given substring.
func containsJSON(body, sub string) bool {
	return strings.Contains(body, sub)
}
