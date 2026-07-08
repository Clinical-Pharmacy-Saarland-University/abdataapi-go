package priscuscontroller_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"observeddb-go-api/internal/controller/priscuscontroller"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

// doCompoundRequest builds a fresh gin engine, registers the compound endpoint
// and serves a single GET request against the given target.
func doCompoundRequest(
	t *testing.T, pc *priscuscontroller.PriscusController, target string,
) *httptest.ResponseRecorder {
	t.Helper()

	r := gin.New()
	r.GET("/priscus/compounds", pc.GetPriscusStatusByCompound)

	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

// TestGetPriscusStatusByCompound_MissingParam verifies that omitting the
// compounds parameter entirely is rejected with 400 before any DB query runs.
func TestGetPriscusStatusByCompound_MissingParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// No ExpectQuery is registered; the request must fail validation before
	// reaching the database, otherwise the mock would report an unexpected query.
	pc, mock := newTestController(t)

	w := doCompoundRequest(t, pc, "/priscus/compounds")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusBadRequest, w.Code, w.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Error string `json:"error"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v (body: %s)", err, w.Body.String())
	}

	if resp.Data.Error != "Missing required parameter: compounds" {
		t.Errorf("expected error message %q, got %q",
			"Missing required parameter: compounds", resp.Data.Error)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet or unexpected DB expectations: %v", err)
	}
}

// TestGetPriscusStatusByCompound_RepeatedParamsPreserveComma verifies that when
// the compounds list is supplied as two repeated query parameters, a name that
// itself contains a comma (the ABDA canonical spelling "Mirtazapin-0,5-Wasser")
// is passed to the fetch query INTACT rather than being split on the comma.
func TestGetPriscusStatusByCompound_RepeatedParamsPreserveComma(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock := newTestController(t)

	// The compound lookup (common.StoToCompoundsMap) lowercases and trims each
	// name, then queries SNA_DB with one bound argument per name. The intact
	// comma-bearing name must arrive as a single argument "mirtazapin-0,5-wasser";
	// if it had been split, three arguments would be sent and the count would
	// not match, failing the query. Returning no rows keeps the second (SZG_DB)
	// query from running, since there are no resolved STO keys.
	rows := sqlmock.NewRows([]string{"Name", "DDI_Key_STO"})

	mock.ExpectQuery("FROM SNA_DB").
		WithArgs("apixaban", "mirtazapin-0,5-wasser").
		WillReturnRows(rows)

	w := doCompoundRequest(t, pc,
		"/priscus/compounds?compounds=Apixaban&compounds=Mirtazapin-0,5-Wasser")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   []struct {
			Input     string `json:"input"`
			IsPriscus bool   `json:"priscus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v (body: %s)", err, w.Body.String())
	}

	if resp.Status != "success" {
		t.Fatalf("expected status 'success', got %q", resp.Status)
	}

	// Both original inputs must be echoed back verbatim; the comma-bearing name
	// must not have been split into two separate results.
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 results, got %d (body: %s)", len(resp.Data), w.Body.String())
	}

	inputs := make(map[string]bool, len(resp.Data))
	for _, item := range resp.Data {
		inputs[item.Input] = true
	}

	if !inputs["Mirtazapin-0,5-Wasser"] {
		t.Errorf("expected intact input %q in results, got %v",
			"Mirtazapin-0,5-Wasser", resp.Data)
	}
	if !inputs["Apixaban"] {
		t.Errorf("expected input %q in results, got %v", "Apixaban", resp.Data)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet or unexpected DB expectations: %v", err)
	}
}

// TestGetPriscusStatusByCompound_LegacyCommaJoined verifies that a single
// comma-joined value keeps its legacy behavior and is split into two names,
// producing two bound arguments in the fetch query.
func TestGetPriscusStatusByCompound_LegacyCommaJoined(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock := newTestController(t)

	// A lone comma-joined value cannot be told apart from the legacy list form,
	// so it is split: "A,B" becomes two names, lowercased to "a" and "b", and
	// two arguments are bound in the SNA_DB lookup.
	rows := sqlmock.NewRows([]string{"Name", "DDI_Key_STO"})

	mock.ExpectQuery("FROM SNA_DB").
		WithArgs("a", "b").
		WillReturnRows(rows)

	w := doCompoundRequest(t, pc, "/priscus/compounds?compounds=A,B")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   []struct {
			Input     string `json:"input"`
			IsPriscus bool   `json:"priscus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v (body: %s)", err, w.Body.String())
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 results from split value, got %d (body: %s)",
			len(resp.Data), w.Body.String())
	}

	inputs := make(map[string]bool, len(resp.Data))
	for _, item := range resp.Data {
		inputs[item.Input] = true
	}
	if !inputs["A"] || !inputs["B"] {
		t.Errorf("expected split inputs A and B, got %v", resp.Data)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet or unexpected DB expectations: %v", err)
	}
}
