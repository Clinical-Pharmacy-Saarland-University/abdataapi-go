package tokens_test

import (
	"encoding/base64"
	"testing"

	"observeddb-go-api/internal/utils/hash"
	"observeddb-go-api/internal/utils/tokens"
)

// TestCreateResetTokens_HashVerifies verifies the core contract of the reset
// token pair: the returned plaintext token verifies against the returned hash,
// and neither field is empty.
func TestCreateResetTokens_HashVerifies(t *testing.T) {
	pair, err := tokens.CreateResetTokens()
	if err != nil {
		t.Fatalf("CreateResetTokens returned error: %v", err)
	}

	if pair.Token == "" {
		t.Error("reset token is empty")
	}
	if pair.TokenHash == "" {
		t.Error("reset token hash is empty")
	}
	if pair.Token == pair.TokenHash {
		t.Error("token and hash are identical; the hash must not equal the plaintext")
	}

	match, err := hash.Check(pair.TokenHash, pair.Token)
	if err != nil {
		t.Fatalf("hash.Check returned error: %v", err)
	}
	if !match {
		t.Error("hash.Check reported no match for the token against its own hash")
	}
}

// TestCreateResetTokens_TokenIsBase64URL verifies the plaintext token is a
// URL-safe base64 encoding of 32 random bytes, as documented by the source.
func TestCreateResetTokens_TokenIsBase64URL(t *testing.T) {
	pair, err := tokens.CreateResetTokens()
	if err != nil {
		t.Fatalf("CreateResetTokens returned error: %v", err)
	}

	raw, err := base64.URLEncoding.DecodeString(pair.Token)
	if err != nil {
		t.Fatalf("token %q is not valid URL-safe base64: %v", pair.Token, err)
	}
	if len(raw) != 32 {
		t.Errorf("decoded token length = %d, want 32", len(raw))
	}
}

// TestCreateResetTokens_Unique verifies that repeated calls yield distinct
// tokens and distinct hashes (random source + per-call argon2id salt).
func TestCreateResetTokens_Unique(t *testing.T) {
	pair1, err := tokens.CreateResetTokens()
	if err != nil {
		t.Fatalf("first CreateResetTokens returned error: %v", err)
	}
	pair2, err := tokens.CreateResetTokens()
	if err != nil {
		t.Fatalf("second CreateResetTokens returned error: %v", err)
	}

	if pair1.Token == pair2.Token {
		t.Error("two reset tokens are identical, expected unique random tokens")
	}
	if pair1.TokenHash == pair2.TokenHash {
		t.Error("two reset token hashes are identical, expected unique salts")
	}
}

// TestCreateResetTokens_WrongTokenFailsCheck verifies a hash does not verify
// against a different token (cross-pair mismatch).
func TestCreateResetTokens_WrongTokenFailsCheck(t *testing.T) {
	pair1, err := tokens.CreateResetTokens()
	if err != nil {
		t.Fatalf("first CreateResetTokens returned error: %v", err)
	}
	pair2, err := tokens.CreateResetTokens()
	if err != nil {
		t.Fatalf("second CreateResetTokens returned error: %v", err)
	}

	match, err := hash.Check(pair1.TokenHash, pair2.Token)
	if err != nil {
		t.Fatalf("hash.Check returned error: %v", err)
	}
	if match {
		t.Error("hash.Check matched a token against a foreign hash")
	}
}
