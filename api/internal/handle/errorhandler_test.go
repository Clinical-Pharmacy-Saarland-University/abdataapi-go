package handle_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"observeddb-go-api/internal/handle"
	"observeddb-go-api/internal/utils/apierr"

	"github.com/gin-gonic/gin"
)

// jsendEnvelope captures the top-level JSend fields common to all responses.
type jsendEnvelope struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// newRespContext builds a gin context backed by a recorder and a minimal
// request so handlers that inspect c.Request do not panic.
func newRespContext() (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	return c, rec
}

// decodeEnvelope unmarshals the recorder body into a jsendEnvelope.
func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) jsendEnvelope {
	t.Helper()
	var env jsendEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to unmarshal response body %q: %v", rec.Body.String(), err)
	}
	return env
}

func TestSuccessResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, rec := newRespContext()
	handle.Success(c, map[string]string{"key": "value"})

	if rec.Code != http.StatusOK {
		t.Errorf("Success status = %d, want %d", rec.Code, http.StatusOK)
	}
	env := decodeEnvelope(t, rec)
	if env.Status != "success" {
		t.Errorf("Success top-level status = %q, want %q", env.Status, "success")
	}
	if len(env.Data) == 0 {
		t.Errorf("Success data missing, body = %q", rec.Body.String())
	}
}

func TestSuccessWithStatusResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, rec := newRespContext()
	handle.SuccessWithStatus(c, http.StatusMultiStatus, map[string]string{"key": "value"})

	if rec.Code != http.StatusMultiStatus {
		t.Errorf("SuccessWithStatus status = %d, want %d", rec.Code, http.StatusMultiStatus)
	}
	env := decodeEnvelope(t, rec)
	if env.Status != "success" {
		t.Errorf("SuccessWithStatus top-level status = %q, want %q", env.Status, "success")
	}
}

func TestBadRequestErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, rec := newRespContext()
	handle.BadRequestError(c, "bad input")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("BadRequestError status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	env := decodeEnvelope(t, rec)
	if env.Status != "fail" {
		t.Errorf("BadRequestError top-level status = %q, want %q", env.Status, "fail")
	}

	var data struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("failed to unmarshal data field %q: %v", string(env.Data), err)
	}
	if data.Error == "" {
		t.Errorf("BadRequestError data.error is empty, body = %q", rec.Body.String())
	}
}

func TestNotFoundErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, rec := newRespContext()
	handle.NotFoundError(c, "not here")

	if rec.Code != http.StatusNotFound {
		t.Errorf("NotFoundError status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	env := decodeEnvelope(t, rec)
	if env.Status != "fail" {
		t.Errorf("NotFoundError top-level status = %q, want %q", env.Status, "fail")
	}
}

func TestServerErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, rec := newRespContext()
	handle.ServerError(c, http.ErrHandlerTimeout)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("ServerError status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	env := decodeEnvelope(t, rec)
	if env.Status != "error" {
		t.Errorf("ServerError top-level status = %q, want %q", env.Status, "error")
	}
}

func TestValidationErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, rec := newRespContext()
	handle.ValidationError(c, []apierr.ValidationError{{Field: "field", Reason: "required"}})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("ValidationError status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	env := decodeEnvelope(t, rec)
	if env.Status != "fail" {
		t.Errorf("ValidationError top-level status = %q, want %q", env.Status, "fail")
	}

	var data struct {
		Errors []apierr.ValidationError `json:"errors"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("failed to unmarshal data field %q: %v", string(env.Data), err)
	}
	if len(data.Errors) == 0 {
		t.Errorf("ValidationError data.errors is empty, body = %q", rec.Body.String())
	}
}
