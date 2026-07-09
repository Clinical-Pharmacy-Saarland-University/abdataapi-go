package syscontroller_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/controller/syscontroller"

	"github.com/gin-gonic/gin"
)

func newTestSysController() *syscontroller.SysController {
	return &syscontroller.SysController{
		Meta: cfg.MetaConfig{
			Name:    "Test API",
			Version: "1.2.3",
		},
		Limit: cfg.LimitsConfig{
			InteractionDrugs: 50,
			BatchQueries:     100,
			BatchJobs:        4,
		},
	}
}

func TestGetPing(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	controller := newTestSysController()
	r := gin.New()
	r.GET("/sys/ping", controller.GetPing)

	req := httptest.NewRequest(http.MethodGet, "/sys/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var envelope struct {
		Status string `json:"status"`
		Data   struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to unmarshal response body: %v (body: %s)", err, w.Body.String())
	}

	if envelope.Status != "success" {
		t.Errorf("expected status %q, got %q", "success", envelope.Status)
	}
	if envelope.Data.Message != "pong" {
		t.Errorf("expected data.message %q, got %q", "pong", envelope.Data.Message)
	}
}

func TestGetInfo(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	controller := newTestSysController()
	r := gin.New()
	r.GET("/sys/info", controller.GetInfo)

	req := httptest.NewRequest(http.MethodGet, "/sys/info", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}

	var envelope struct {
		Status string `json:"status"`
		Data   struct {
			MetaInfo struct {
				Name    string `json:"api"`
				Version string `json:"version"`
			} `json:"meta_info"`
			APILimits struct {
				MaxDrugs        int `json:"max_drugs"`
				MaxBatchQueries int `json:"max_batch_queries"`
			} `json:"api_limits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to unmarshal response body: %v (body: %s)", err, w.Body.String())
	}

	if envelope.Status != "success" {
		t.Errorf("expected status %q, got %q", "success", envelope.Status)
	}
	if envelope.Data.MetaInfo.Name != "Test API" {
		t.Errorf("expected meta_info.api %q, got %q", "Test API", envelope.Data.MetaInfo.Name)
	}
	if envelope.Data.MetaInfo.Version != "1.2.3" {
		t.Errorf("expected meta_info.version %q, got %q", "1.2.3", envelope.Data.MetaInfo.Version)
	}
	if envelope.Data.APILimits.MaxDrugs != 50 {
		t.Errorf("expected api_limits.max_drugs %d, got %d", 50, envelope.Data.APILimits.MaxDrugs)
	}
	if envelope.Data.APILimits.MaxBatchQueries != 100 {
		t.Errorf("expected api_limits.max_batch_queries %d, got %d", 100, envelope.Data.APILimits.MaxBatchQueries)
	}
}
