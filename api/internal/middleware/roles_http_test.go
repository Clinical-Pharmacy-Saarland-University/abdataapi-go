package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"observeddb-go-api/internal/middleware"

	"github.com/gin-gonic/gin"
)

// newRoleRouter builds a gin engine where an upstream handler seeds the
// "user_role" context value exactly as the Authentication middleware would,
// followed by the role guard under test and a terminal handler that records
// whether it ran.
func newRoleRouter(guard gin.HandlerFunc, userRole string, spy *contextSpy) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	seed := func(c *gin.Context) {
		// Only set the role when provided; an empty role simulates an
		// unauthenticated context where user_role was never stored.
		if userRole != "" {
			c.Set("user_role", userRole)
		}
		c.Next()
	}

	r.GET("/admin", seed, guard, func(c *gin.Context) {
		if spy != nil {
			spy.called = true
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// serveRole issues a GET /admin request through the router and returns the
// recorder for assertions.
func serveRole(r *gin.Engine) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestAdminAccess_AllowsAdmin verifies that an admin identity clears the
// AdminAccess guard and the protected handler runs.
func TestAdminAccess_AllowsAdmin(t *testing.T) {
	t.Parallel()

	spy := &contextSpy{}
	r := newRoleRouter(middleware.AdminAccess(), "admin", spy)

	w := serveRole(r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}
	if !spy.called {
		t.Fatal("protected handler did not run for an admin identity")
	}
}

// TestAdminAccess_RejectsNonAdmin verifies that identities below admin are
// rejected with 403 and never reach the protected handler. A missing role is
// also covered: an unauthenticated context has no user_role and must be denied.
func TestAdminAccess_RejectsNonAdmin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		role string
	}{
		{name: "approver below admin", role: "approver"},
		{name: "user below admin", role: "user"},
		{name: "unknown role", role: "guest"},
		{name: "missing role", role: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spy := &contextSpy{}
			r := newRoleRouter(middleware.AdminAccess(), tt.role, spy)

			w := serveRole(r)

			if w.Code != http.StatusForbidden {
				t.Fatalf("expected status %d, got %d (body: %s)",
					http.StatusForbidden, w.Code, w.Body.String())
			}
			if spy.called {
				t.Errorf("protected handler ran for insufficient role %q", tt.role)
			}
			if body := w.Body.String(); !strings.Contains(body, `"status":"fail"`) {
				t.Errorf("expected JSend fail status in body, got %q", body)
			}
		})
	}
}

// TestApproverAccess_RoleHierarchy verifies the ApproverAccess guard honours the
// role hierarchy: admin and approver clear it, while a plain user is rejected
// with 403.
func TestApproverAccess_RoleHierarchy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		role       string
		wantCode   int
		wantCalled bool
	}{
		{name: "admin outranks approver", role: "admin", wantCode: http.StatusOK, wantCalled: true},
		{name: "approver allowed", role: "approver", wantCode: http.StatusOK, wantCalled: true},
		{name: "user rejected", role: "user", wantCode: http.StatusForbidden, wantCalled: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spy := &contextSpy{}
			r := newRoleRouter(middleware.ApproverAccess(), tt.role, spy)

			w := serveRole(r)

			if w.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d (body: %s)",
					tt.wantCode, w.Code, w.Body.String())
			}
			if spy.called != tt.wantCalled {
				t.Errorf("handler called = %v, want %v (role %q)", spy.called, tt.wantCalled, tt.role)
			}
		})
	}
}
