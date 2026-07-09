package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/middleware"
	"observeddb-go-api/internal/utils/tokens"

	"github.com/gin-gonic/gin"
)

// testAuthCfg returns an AuthTokenConfig with a fixed test secret and issuer so
// that tokens minted here can be verified by the middleware without a database
// or network. Access tokens are long-lived to keep the happy path deterministic.
func testAuthCfg() *cfg.AuthTokenConfig {
	return &cfg.AuthTokenConfig{
		Secret:                cfg.Bytes("test-secret-do-not-use-in-prod"),
		AccessExpirationTime:  time.Hour,
		RefreshExpirationTime: 24 * time.Hour,
		Issuer:                "observeddb-test",
	}
}

// mintAccessToken creates a signed access token for the given identity using the
// real tokens package, so the Authentication middleware verifies a genuine JWT.
func mintAccessToken(t *testing.T, authCfg *cfg.AuthTokenConfig, claims *tokens.CustomClaims) string {
	t.Helper()

	authTokens, err := tokens.CreateAuthTokens(claims, authCfg)
	if err != nil {
		t.Fatalf("failed to mint auth tokens: %v", err)
	}
	return authTokens.AccessToken
}

// newAuthRouter builds a gin engine guarded by the Authentication middleware.
// The terminal handler echoes the identity that the middleware stored in the
// context so tests can assert the claims were propagated.
func newAuthRouter(authCfg *cfg.AuthTokenConfig, spy *contextSpy) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", middleware.Authentication(authCfg), func(c *gin.Context) {
		if spy != nil {
			spy.called = true
			spy.userID = c.GetUint("user_id")
			spy.userEmail = c.GetString("user_email")
			spy.userRole = c.GetString("user_role")
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// contextSpy captures the identity values the Authentication middleware writes
// into the gin context, plus whether the terminal handler ran at all.
type contextSpy struct {
	called    bool
	userID    uint
	userEmail string
	userRole  string
}

// TestAuthentication_MissingHeader verifies that a request without an
// Authorization header is rejected with 401 and the guarded handler never runs.
func TestAuthentication_MissingHeader(t *testing.T) {
	t.Parallel()

	authCfg := testAuthCfg()
	spy := &contextSpy{}
	r := newAuthRouter(authCfg, spy)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusUnauthorized, w.Code, w.Body.String())
	}
	if spy.called {
		t.Error("guarded handler ran despite missing Authorization header")
	}
	if body := w.Body.String(); !strings.Contains(body, `"status":"fail"`) {
		t.Errorf("expected JSend fail status in body, got %q", body)
	}
}

// TestAuthentication_MalformedHeader verifies that Authorization headers that do
// not match the "Bearer <token>" shape are rejected with 401 before any token
// verification is attempted.
func TestAuthentication_MalformedHeader(t *testing.T) {
	t.Parallel()

	authCfg := testAuthCfg()

	tests := []struct {
		name   string
		header string
	}{
		{name: "no bearer keyword", header: "sometoken"},
		{name: "basic auth scheme", header: "Basic dXNlcjpwYXNz"},
		{name: "double bearer keyword", header: "Bearer Bearer token"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spy := &contextSpy{}
			r := newAuthRouter(authCfg, spy)

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected status %d, got %d (body: %s)",
					http.StatusUnauthorized, w.Code, w.Body.String())
			}
			if spy.called {
				t.Error("guarded handler ran despite malformed Authorization header")
			}
		})
	}
}

// TestAuthentication_ValidToken verifies the happy path: a genuinely signed
// access token lets the guarded handler run and the middleware propagates the
// identity (id, email, role) into the gin context.
func TestAuthentication_ValidToken(t *testing.T) {
	t.Parallel()

	authCfg := testAuthCfg()
	want := &tokens.CustomClaims{ID: 42, Email: "user@example.com", Role: "user"}
	token := mintAccessToken(t, authCfg, want)

	spy := &contextSpy{}
	r := newAuthRouter(authCfg, spy)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusOK, w.Code, w.Body.String())
	}
	if !spy.called {
		t.Fatal("guarded handler did not run for a valid access token")
	}
	if spy.userID != want.ID {
		t.Errorf("user_id in context = %d, want %d", spy.userID, want.ID)
	}
	if spy.userEmail != want.Email {
		t.Errorf("user_email in context = %q, want %q", spy.userEmail, want.Email)
	}
	if spy.userRole != want.Role {
		t.Errorf("user_role in context = %q, want %q", spy.userRole, want.Role)
	}
}

// TestAuthentication_ExpiredToken verifies that an access token whose expiry has
// already passed (beyond the server time skew) is rejected with 401.
func TestAuthentication_ExpiredToken(t *testing.T) {
	t.Parallel()

	// A negative access expiry mints a token that expired an hour ago, which is
	// well outside the ServerTimeSkew tolerance, so verification must fail.
	authCfg := testAuthCfg()
	authCfg.AccessExpirationTime = -time.Hour

	token := mintAccessToken(t, authCfg,
		&tokens.CustomClaims{ID: 7, Email: "expired@example.com", Role: "user"})

	spy := &contextSpy{}
	r := newAuthRouter(testAuthCfg(), spy)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusUnauthorized, w.Code, w.Body.String())
	}
	if spy.called {
		t.Error("guarded handler ran despite an expired access token")
	}
}

// TestAuthentication_WrongSecret verifies that a token signed with a different
// secret than the middleware is configured with fails signature verification and
// yields 401.
func TestAuthentication_WrongSecret(t *testing.T) {
	t.Parallel()

	// Mint with one secret, verify with another.
	minting := testAuthCfg()
	minting.Secret = cfg.Bytes("a-completely-different-secret")
	token := mintAccessToken(t, minting,
		&tokens.CustomClaims{ID: 1, Email: "attacker@example.com", Role: "admin"})

	spy := &contextSpy{}
	r := newAuthRouter(testAuthCfg(), spy)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusUnauthorized, w.Code, w.Body.String())
	}
	if spy.called {
		t.Error("guarded handler ran despite a token signed with the wrong secret")
	}
}

// TestAuthentication_RefreshTokenRejected verifies that a valid refresh token
// (correct signature and issuer, but wrong token type) is not accepted by the
// access-token guard.
func TestAuthentication_RefreshTokenRejected(t *testing.T) {
	t.Parallel()

	authCfg := testAuthCfg()
	authTokens, err := tokens.CreateAuthTokens(
		&tokens.CustomClaims{ID: 9, Email: "refresh@example.com", Role: "user"}, authCfg)
	if err != nil {
		t.Fatalf("failed to mint auth tokens: %v", err)
	}

	spy := &contextSpy{}
	r := newAuthRouter(authCfg, spy)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+authTokens.RefreshToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusUnauthorized, w.Code, w.Body.String())
	}
	if spy.called {
		t.Error("guarded handler ran despite a refresh token on the access guard")
	}
}

// TestAuthentication_GarbageToken verifies that a well-formed Bearer header
// carrying a non-JWT string is rejected with 401.
func TestAuthentication_GarbageToken(t *testing.T) {
	t.Parallel()

	spy := &contextSpy{}
	r := newAuthRouter(testAuthCfg(), spy)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d (body: %s)",
			http.StatusUnauthorized, w.Code, w.Body.String())
	}
	if spy.called {
		t.Error("guarded handler ran despite a garbage token")
	}
}
