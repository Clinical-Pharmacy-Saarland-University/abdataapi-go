package tokens_test

import (
	"strings"
	"testing"
	"time"

	"observeddb-go-api/cfg"
	"observeddb-go-api/internal/utils/tokens"

	"github.com/golang-jwt/jwt/v4"
)

// testAuthConfig returns a valid AuthTokenConfig with generous expiry windows so
// freshly minted tokens verify cleanly. Individual tests clone and mutate it.
func testAuthConfig() *cfg.AuthTokenConfig {
	return &cfg.AuthTokenConfig{
		Secret:                cfg.Bytes("super-secret-signing-key"),
		AccessExpirationTime:  15 * time.Minute,
		RefreshExpirationTime: 24 * time.Hour,
		Issuer:                "observeddb-test",
	}
}

// testUser is the canonical claims payload round-tripped through the tokens.
func testUser() *tokens.CustomClaims {
	return &tokens.CustomClaims{
		Email: "user@example.com",
		Role:  "admin",
		ID:    42,
	}
}

// TestCreateAuthTokens_Shape verifies the envelope returned by CreateAuthTokens:
// both tokens are populated, the type is Bearer, and the expiry times reflect
// the configured access/refresh windows (refresh strictly later than access).
func TestCreateAuthTokens_Shape(t *testing.T) {
	cfgAuth := testAuthConfig()

	before := time.Now()
	auth, err := tokens.CreateAuthTokens(testUser(), cfgAuth)
	after := time.Now()
	if err != nil {
		t.Fatalf("CreateAuthTokens returned error: %v", err)
	}

	if auth.AccessToken == "" {
		t.Error("access token is empty")
	}
	if auth.RefreshToken == "" {
		t.Error("refresh token is empty")
	}
	if auth.AccessToken == auth.RefreshToken {
		t.Error("access and refresh tokens are identical, expected distinct tokens")
	}
	if auth.TokenType != "Bearer" {
		t.Errorf("token type = %q, want %q", auth.TokenType, "Bearer")
	}

	// The access expiry must sit within [before+window, after+window]; likewise
	// for refresh. This confirms the correct window is applied to each token.
	wantAccessLo := before.Add(cfgAuth.AccessExpirationTime)
	wantAccessHi := after.Add(cfgAuth.AccessExpirationTime)
	if auth.AccessExpiresIn.Before(wantAccessLo) || auth.AccessExpiresIn.After(wantAccessHi) {
		t.Errorf("access expiry %v not within [%v, %v]", auth.AccessExpiresIn, wantAccessLo, wantAccessHi)
	}

	wantRefreshLo := before.Add(cfgAuth.RefreshExpirationTime)
	wantRefreshHi := after.Add(cfgAuth.RefreshExpirationTime)
	if auth.RefreshExpiresIn.Before(wantRefreshLo) || auth.RefreshExpiresIn.After(wantRefreshHi) {
		t.Errorf("refresh expiry %v not within [%v, %v]", auth.RefreshExpiresIn, wantRefreshLo, wantRefreshHi)
	}

	if !auth.RefreshExpiresIn.After(auth.AccessExpiresIn) {
		t.Errorf("refresh expiry %v should be after access expiry %v", auth.RefreshExpiresIn, auth.AccessExpiresIn)
	}
}

// TestCheckAccessToken_RoundTrip verifies that an access token created by
// CreateAuthTokens verifies with CheckAccessToken and returns the exact claims.
func TestCheckAccessToken_RoundTrip(t *testing.T) {
	cfgAuth := testAuthConfig()
	user := testUser()

	auth, err := tokens.CreateAuthTokens(user, cfgAuth)
	if err != nil {
		t.Fatalf("CreateAuthTokens returned error: %v", err)
	}

	got, err := tokens.CheckAccessToken(auth.AccessToken, cfgAuth)
	if err != nil {
		t.Fatalf("CheckAccessToken rejected a valid token: %v", err)
	}

	if *got != *user {
		t.Errorf("claims round-trip mismatch: got %+v, want %+v", *got, *user)
	}
}

// TestCheckRefreshToken_RoundTrip verifies the refresh token verifies with
// CheckRefreshToken and returns the exact claims.
func TestCheckRefreshToken_RoundTrip(t *testing.T) {
	cfgAuth := testAuthConfig()
	user := testUser()

	auth, err := tokens.CreateAuthTokens(user, cfgAuth)
	if err != nil {
		t.Fatalf("CreateAuthTokens returned error: %v", err)
	}

	got, err := tokens.CheckRefreshToken(auth.RefreshToken, cfgAuth)
	if err != nil {
		t.Fatalf("CheckRefreshToken rejected a valid token: %v", err)
	}

	if *got != *user {
		t.Errorf("claims round-trip mismatch: got %+v, want %+v", *got, *user)
	}
}

// TestCheckToken_WrongType verifies the type separation enforced by checkToken:
// an access token is rejected by CheckRefreshToken and a refresh token is
// rejected by CheckAccessToken, even though both are signed with the same key.
func TestCheckToken_WrongType(t *testing.T) {
	cfgAuth := testAuthConfig()

	auth, err := tokens.CreateAuthTokens(testUser(), cfgAuth)
	if err != nil {
		t.Fatalf("CreateAuthTokens returned error: %v", err)
	}

	if _, err := tokens.CheckRefreshToken(auth.AccessToken, cfgAuth); err == nil {
		t.Error("CheckRefreshToken accepted an access token, expected rejection")
	}

	if _, err := tokens.CheckAccessToken(auth.RefreshToken, cfgAuth); err == nil {
		t.Error("CheckAccessToken accepted a refresh token, expected rejection")
	}
}

// TestCheckToken_WrongSecret verifies that a token signed with one secret is
// rejected when verified with a config carrying a different secret (bad
// signature).
func TestCheckToken_WrongSecret(t *testing.T) {
	cfgAuth := testAuthConfig()

	auth, err := tokens.CreateAuthTokens(testUser(), cfgAuth)
	if err != nil {
		t.Fatalf("CreateAuthTokens returned error: %v", err)
	}

	otherCfg := testAuthConfig()
	otherCfg.Secret = cfg.Bytes("a-completely-different-secret-key")

	if _, err := tokens.CheckAccessToken(auth.AccessToken, otherCfg); err == nil {
		t.Error("CheckAccessToken accepted a token signed with a different secret")
	}
	if _, err := tokens.CheckRefreshToken(auth.RefreshToken, otherCfg); err == nil {
		t.Error("CheckRefreshToken accepted a token signed with a different secret")
	}
}

// TestCheckToken_WrongIssuer verifies the issuer claim is enforced: a token
// minted with one issuer is rejected by a checker configured with another.
func TestCheckToken_WrongIssuer(t *testing.T) {
	cfgAuth := testAuthConfig()

	auth, err := tokens.CreateAuthTokens(testUser(), cfgAuth)
	if err != nil {
		t.Fatalf("CreateAuthTokens returned error: %v", err)
	}

	otherCfg := testAuthConfig()
	otherCfg.Issuer = "some-other-issuer"

	if _, err := tokens.CheckAccessToken(auth.AccessToken, otherCfg); err == nil {
		t.Error("CheckAccessToken accepted a token with a mismatched issuer")
	}
}

// TestCheckToken_Expired verifies an expired access token is rejected. The
// expiry window is set well beyond the 5-minute server time skew so the token
// is unambiguously in the past.
func TestCheckToken_Expired(t *testing.T) {
	cfgAuth := testAuthConfig()
	// Negative window places ExpiresAt ~10 minutes in the past, comfortably
	// past the 5-minute ServerTimeSkew grace period applied by checkToken.
	cfgAuth.AccessExpirationTime = -10 * time.Minute

	auth, err := tokens.CreateAuthTokens(testUser(), cfgAuth)
	if err != nil {
		t.Fatalf("CreateAuthTokens returned error: %v", err)
	}

	if _, err := tokens.CheckAccessToken(auth.AccessToken, cfgAuth); err == nil {
		t.Error("CheckAccessToken accepted an expired token, expected rejection")
	}
}

// TestCheckToken_Garbage verifies that malformed / tampered token strings are
// rejected rather than panicking or returning claims.
func TestCheckToken_Garbage(t *testing.T) {
	cfgAuth := testAuthConfig()

	garbage := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"not a jwt", "this-is-not-a-token"},
		{"two segments", "header.payload"},
		{"random dots", "aaa.bbb.ccc"},
	}

	for _, g := range garbage {
		g := g
		t.Run(g.name, func(t *testing.T) {
			if _, err := tokens.CheckAccessToken(g.token, cfgAuth); err == nil {
				t.Errorf("CheckAccessToken accepted garbage token %q", g.token)
			}
			if _, err := tokens.CheckRefreshToken(g.token, cfgAuth); err == nil {
				t.Errorf("CheckRefreshToken accepted garbage token %q", g.token)
			}
		})
	}
}

// TestCheckToken_Tampered verifies that flipping a character in the signature
// segment of a valid token invalidates it.
func TestCheckToken_Tampered(t *testing.T) {
	cfgAuth := testAuthConfig()

	auth, err := tokens.CreateAuthTokens(testUser(), cfgAuth)
	if err != nil {
		t.Fatalf("CreateAuthTokens returned error: %v", err)
	}

	parts := strings.Split(auth.AccessToken, ".")
	if len(parts) != 3 {
		t.Fatalf("expected a 3-segment JWT, got %d segments", len(parts))
	}

	// Mutate the last character of the signature to break verification.
	sig := parts[2]
	last := sig[len(sig)-1]
	replacement := byte('A')
	if last == 'A' {
		replacement = 'B'
	}
	parts[2] = sig[:len(sig)-1] + string(replacement)
	tampered := strings.Join(parts, ".")

	if _, err := tokens.CheckAccessToken(tampered, cfgAuth); err == nil {
		t.Error("CheckAccessToken accepted a token with a tampered signature")
	}
}

// TestCheckToken_WrongSigningMethod verifies that a token forged with an
// unexpected signing algorithm (here the "none" method) does not verify. The
// keyfunc always returns the HMAC key, so a non-HMAC token must fail signature
// validation.
func TestCheckToken_WrongSigningMethod(t *testing.T) {
	cfgAuth := testAuthConfig()

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Issuer:    cfgAuth.Issuer,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	// The unsafe none method requires this sentinel key.
	forged, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to build forged token: %v", err)
	}

	if _, err := tokens.CheckAccessToken(forged, cfgAuth); err == nil {
		t.Error("CheckAccessToken accepted a token signed with the none method")
	}
}
