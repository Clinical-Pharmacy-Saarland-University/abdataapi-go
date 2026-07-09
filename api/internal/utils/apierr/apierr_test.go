package apierr_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"observeddb-go-api/internal/utils/apierr"

	"github.com/gin-gonic/gin"
)

func TestBatchStatusCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		nTotal   int
		nSuccess int
		want     int
	}{
		{"all succeed", 5, 5, http.StatusOK},
		{"all succeed zero total", 0, 0, http.StatusOK},
		{"partial success", 5, 3, http.StatusMultiStatus},
		{"none succeed", 5, 0, http.StatusMultiStatus},
		{"single success", 1, 1, http.StatusOK},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := apierr.BatchStatusCode(tt.nTotal, tt.nSuccess)
			if got != tt.want {
				t.Errorf("BatchStatusCode(%d, %d) = %d, want %d", tt.nTotal, tt.nSuccess, got, tt.want)
			}
		})
	}
}

func TestResStatusOk(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		want   bool
	}{
		{"200 is ok", http.StatusOK, true},
		{"404 is not ok", http.StatusNotFound, false},
		{"500 is not ok", http.StatusInternalServerError, false},
		{"201 is not ok", http.StatusCreated, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := apierr.ResStatus{Status: tt.status, Message: "msg"}
			if got := r.Ok(); got != tt.want {
				t.Errorf("ResStatus{Status: %d}.Ok() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		msg         string
		wantStatus  int
		wantMessage string
		wantError   string
	}{
		{
			name:        "404 keeps message in Error",
			status:      http.StatusNotFound,
			msg:         "not found here",
			wantStatus:  http.StatusNotFound,
			wantMessage: "not found here",
			wantError:   "not found here",
		},
		{
			name:        "500 masks message in Error",
			status:      http.StatusInternalServerError,
			msg:         "raw db failure detail",
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "raw db failure detail",
			wantError:   "Internal server error",
		},
		{
			name:        "400 keeps message in Error",
			status:      http.StatusBadRequest,
			msg:         "bad input",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "bad input",
			wantError:   "bad input",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := apierr.New(tt.status, tt.msg)
			if got := e.Status(); got != tt.wantStatus {
				t.Errorf("Status() = %d, want %d", got, tt.wantStatus)
			}
			if got := e.Message(); got != tt.wantMessage {
				t.Errorf("Message() = %q, want %q", got, tt.wantMessage)
			}
			if got := e.Error(); got != tt.wantError {
				t.Errorf("Error() = %q, want %q", got, tt.wantError)
			}
		})
	}
}

func TestToResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "nil error is success",
			err:         nil,
			wantStatus:  http.StatusOK,
			wantMessage: "Success",
		},
		{
			name:        "apierr 404 passes through",
			err:         apierr.New(http.StatusNotFound, "x"),
			wantStatus:  http.StatusNotFound,
			wantMessage: "x",
		},
		{
			name:        "generic error becomes 500 with masked message",
			err:         errors.New("some raw failure"),
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "Internal server error",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			got := apierr.ToResponse(c, tt.err)
			if got.Status != tt.wantStatus {
				t.Errorf("ToResponse status = %d, want %d", got.Status, tt.wantStatus)
			}
			if got.Message != tt.wantMessage {
				t.Errorf("ToResponse message = %q, want %q", got.Message, tt.wantMessage)
			}
		})
	}
}
