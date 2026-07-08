package priscuscontroller_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/priscuscontroller"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// newTestController wires a PriscusController to a mocked database. The returned
// mock is used to set query expectations; the cleanup closes the underlying DB.
func newTestController(t *testing.T) (*priscuscontroller.PriscusController, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	pc := &priscuscontroller.PriscusController{
		DB:     sqlxDB,
		Limits: cfg.LimitsConfig{InteractionDrugs: 100, BatchQueries: 100},
	}

	return pc, mock
}

func doRequest(t *testing.T, pc *priscuscontroller.PriscusController, target string) *httptest.ResponseRecorder {
	t.Helper()

	r := gin.New()
	r.GET("/priscus/pzns", pc.GetPriscusStatus)

	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

func TestGetPriscusStatus_InvalidPZN(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name   string
		target string
	}{
		{
			name:   "too short",
			target: "/priscus/pzns?pzns=1234567",
		},
		{
			name:   "non-numeric",
			target: "/priscus/pzns?pzns=abcdefgh",
		},
		{
			name:   "bad checksum",
			target: "/priscus/pzns?pzns=11111111",
		},
		{
			name:   "one valid one invalid",
			target: "/priscus/pzns?pzns=03041347,11111111",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Invalid PZNs are rejected before any DB query runs, so no
			// ExpectQuery is set; an unexpected query would fail the mock.
			pc, mock := newTestController(t)

			w := doRequest(t, pc, tc.target)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d (body: %s)",
					http.StatusBadRequest, w.Code, w.Body.String())
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet or unexpected DB expectations: %v", err)
			}
		})
	}
}

func TestGetPriscusStatus_ValidPZNs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pc, mock := newTestController(t)

	// The handler validates then queries FAI_DB. It selects PAE_DB.PZN and
	// FAI_DB.Key_STO with the PZNs as args (in order) followed by the priscus
	// Key_SGR filter value. Only 03041347 is returned by the mock, so it must
	// be reported priscus=true while the absent 05538454 is priscus=false.
	rows := sqlmock.NewRows([]string{"PZN", "Key_STO"}).
		AddRow("03041347", uint64(42))

	mock.ExpectQuery("FROM FAI_DB").
		WithArgs("03041347", "05538454", "10084520").
		WillReturnRows(rows)

	w := doRequest(t, pc, "/priscus/pzns?pzns=03041347,05538454")

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   []struct {
			PZN       string `json:"PZN"`
			IsPriscus bool   `json:"priscus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v (body: %s)", err, w.Body.String())
	}

	if resp.Status != "success" {
		t.Fatalf("expected status 'success', got %q", resp.Status)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 results, got %d (body: %s)", len(resp.Data), w.Body.String())
	}

	got := make(map[string]bool, len(resp.Data))
	for _, item := range resp.Data {
		got[item.PZN] = item.IsPriscus
	}

	if priscus, ok := got["03041347"]; !ok || !priscus {
		t.Errorf("expected 03041347 to be present and priscus=true, got present=%v value=%v", ok, priscus)
	}

	if priscus, ok := got["05538454"]; !ok || priscus {
		t.Errorf("expected 05538454 to be present and priscus=false, got present=%v value=%v", ok, priscus)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet or unexpected DB expectations: %v", err)
	}
}
