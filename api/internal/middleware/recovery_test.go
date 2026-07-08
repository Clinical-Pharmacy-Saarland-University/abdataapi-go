package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"observeddb-go-api/internal/middleware"

	"github.com/gin-gonic/gin"
)

func newRecoveryRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.CustomRecovery(middleware.RecoveryHandler))
	r.GET("/panic", handler)
	return r
}

func TestRecoveryHandler_ErrorPanic(t *testing.T) {
	t.Parallel()

	r := newRecoveryRouter(func(_ *gin.Context) {
		panic(errors.New("kaboom"))
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	if body := w.Body.String(); !strings.Contains(body, `"status":"error"`) {
		t.Fatalf("expected JSend error status in body, got %q", body)
	}
}

func TestRecoveryHandler_NonErrorPanic(t *testing.T) {
	t.Parallel()

	r := newRecoveryRouter(func(_ *gin.Context) {
		panic("oops")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	if body := w.Body.String(); !strings.Contains(body, `"status":"error"`) {
		t.Fatalf("expected JSend error status in body, got %q", body)
	}
}
