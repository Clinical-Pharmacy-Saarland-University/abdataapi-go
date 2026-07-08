//go:build integration

package integration_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"observeddb-go-api/internal/controller/adrcontroller"
	"observeddb-go-api/internal/controller/formulationcontroller"
	"observeddb-go-api/internal/controller/priscuscontroller"
	"observeddb-go-api/internal/controller/pzncontroller"
	"observeddb-go-api/internal/controller/qtcontroller"
	"observeddb-go-api/internal/utils/format"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// These tests exist to validate the generated SQL against the REAL ABDA schema — something
// the sqlmock unit tests cannot do. Inputs are self-seeded from the database (a real PZN, a
// real compound name), so the tests do not hardcode fixtures that drift as ABDA is updated.
// Assertions focus on "the query executed against the real schema" (no 5xx / DB error); where
// a real, existing ID is used, the endpoint is also expected to return 200.

// doGET registers a single GET handler (no auth middleware) and executes rawQuery against it.
func doGET(handler gin.HandlerFunc, rawQuery string) *httptest.ResponseRecorder {
	r := gin.New()
	r.GET("/t", handler)
	req := httptest.NewRequest(http.MethodGet, "/t?"+rawQuery, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// sampleStrings runs a single-column query and returns the first n values, skipping the test
// if the dataset does not hold enough rows to exercise the endpoint.
func sampleStrings(t *testing.T, db *sqlx.DB, query, what string, n int) []string {
	t.Helper()
	var vals []string
	if err := db.Select(&vals, query); err != nil {
		t.Fatalf("sampling %s failed (schema issue?): %v", what, err)
	}
	if len(vals) < n {
		t.Skipf("need >= %d %s in the database, found %d", n, what, len(vals))
	}
	return vals[:n]
}

func samplePZNs(t *testing.T, db *sqlx.DB, n int) []string {
	t.Helper()
	return sampleStrings(t, db, fmt.Sprintf("SELECT DISTINCT PZN FROM PAE_DB LIMIT %d", n), "PZNs", n)
}

func sampleCompoundNames(t *testing.T, db *sqlx.DB, n int) []string {
	t.Helper()
	return sampleStrings(t, db, fmt.Sprintf("SELECT DISTINCT Name FROM SNA_DB LIMIT %d", n), "compound names", n)
}

// requireOK fails with the response body when the status is not 200.
func requireOK(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

// requireNoServerError fails only on a 5xx — used where the endpoint may legitimately 404 for
// a given sample ID but the SQL must still be valid against the schema.
func requireNoServerError(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code >= http.StatusInternalServerError {
		t.Fatalf("query failed at the DB/server layer (schema mismatch?): status %d; body: %s",
			w.Code, w.Body.String())
	}
}

func repeatedParam(key string, values []string) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = key + "=" + url.QueryEscape(v)
	}
	return strings.Join(parts, "&")
}

func newADRController(db *sqlx.DB) *adrcontroller.ADRController {
	return &adrcontroller.ADRController{
		DB:                  db,
		Limits:              testLimits(),
		FrequencyTranslator: format.NewAdrFrequencyTranslator(),
	}
}

func newQTController(db *sqlx.DB) *qtcontroller.QTController {
	return &qtcontroller.QTController{
		DB:                 db,
		Limits:             testLimits(),
		CategoryTranslator: format.NewQTCategoryTranslator(),
	}
}

func newPriscusController(db *sqlx.DB) *priscuscontroller.PriscusController {
	return &priscuscontroller.PriscusController{DB: db, Limits: testLimits()}
}

func newPZNController(db *sqlx.DB) *pzncontroller.PZNController {
	return &pzncontroller.PZNController{
		DB:                 db,
		Limits:             testLimits(),
		CategoryTranslator: format.NewProductTranslator(),
	}
}

func newFormulationController(db *sqlx.DB) *formulationcontroller.FormulationController {
	return &formulationcontroller.FormulationController{DB: db}
}

// --- Drug-drug interactions -------------------------------------------------------------

func TestIntegrationInteractionsPZNs(t *testing.T) {
	db := dialOrSkip(t)
	pzns := samplePZNs(t, db, 2)

	// Exercises FamToPZN plus the FZI_C/SZI_C/INT_C PZN interaction join. Both PZNs exist,
	// so the request resolves (200); the interaction set itself may be empty.
	w := doGET(newInteractionController(db).GetInterPZNs, "pzns="+pzns[0]+","+pzns[1])
	requireOK(t, w)
}

func TestIntegrationInteractionsCompoundsWithDoses(t *testing.T) {
	db := dialOrSkip(t)
	names := sampleCompoundNames(t, db, 2)

	// doses=true exercises fetchCompoundDoses (FZI_C/FAI_DB/FAM_DB) in addition to the
	// compound resolution and interaction queries. Repeated params keep any intra-name comma
	// intact. Both names come from SNA_DB, so they resolve (200).
	w := doGET(newInteractionController(db).GetInterCompounds, repeatedParam("compounds", names)+"&doses=true")
	requireOK(t, w)
}

// --- ADR / QT / Priscus (PZN-based) -----------------------------------------------------

func TestIntegrationADRsPZNs(t *testing.T) {
	db := dialOrSkip(t)
	pzn := samplePZNs(t, db, 1)[0]

	w := doGET(newADRController(db).GetAdrsForPZNs, "pzns="+pzn) // NEB_C/MIN_C join
	requireOK(t, w)
}

func TestIntegrationQTPZNs(t *testing.T) {
	db := dialOrSkip(t)
	pzn := samplePZNs(t, db, 1)[0]

	w := doGET(newQTController(db).GetQTStatus, "pzns="+pzn) // FAI_DB/SZG_DB/PAE_DB join
	requireOK(t, w)
}

func TestIntegrationPriscusPZNs(t *testing.T) {
	db := dialOrSkip(t)
	pzn := samplePZNs(t, db, 1)[0]

	w := doGET(newPriscusController(db).GetPriscusStatus, "pzns="+pzn) // FAI_DB/SZG_DB/PAE_DB join
	requireOK(t, w)
}

// --- Product (PZN-based) ----------------------------------------------------------------

func TestIntegrationProductActiveCompounds(t *testing.T) {
	db := dialOrSkip(t)
	pzn := samplePZNs(t, db, 1)[0]

	// The most complex query (PAE_DB/FAI_DB/VSS_DB/SNA_DB + a NOT IN subquery). A sampled PZN
	// may legitimately have no active compounds under the filter (404), but the SQL must run.
	w := doGET(newPZNController(db).GetActiveCompounds, "pzns="+pzn)
	requireNoServerError(t, w)
}

func TestIntegrationProductInfo(t *testing.T) {
	db := dialOrSkip(t)
	pzn := samplePZNs(t, db, 1)[0]

	w := doGET(newPZNController(db).GetProductInfo, "pzns="+pzn) // PAE_DB/FAM_DB join
	requireNoServerError(t, w)
}

// --- Guidelines (pharmgkb) & formulations -----------------------------------------------

func TestIntegrationCompoundGuidelines(t *testing.T) {
	db := dialOrSkip(t)
	names := sampleCompoundNames(t, db, 2)

	// Resolves the names (SNA_DB) and queries the pharmgkb guideline_table. Repeated params
	// avoid the single-value comma split, so real names resolve and the guideline query runs.
	// A 5xx here would flag that guideline_table is missing from the deployed schema.
	w := doGET(newCompoundController(db).GetCompoundGuidelines, repeatedParam("names", names))
	requireOK(t, w)
}

func TestIntegrationFormulations(t *testing.T) {
	db := dialOrSkip(t)

	w := doGET(newFormulationController(db).GetFormulations, "") // SELECT ... FROM DAR_DB
	requireOK(t, w)
	if !strings.Contains(w.Body.String(), "formulations") {
		t.Errorf("expected a formulations field in the response, got: %s", w.Body.String())
	}
}
