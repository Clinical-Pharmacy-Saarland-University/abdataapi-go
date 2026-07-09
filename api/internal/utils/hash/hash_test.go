package hash_test

import (
	"strings"
	"testing"

	"observeddb-go-api/internal/utils/hash"
)

// TestCreateProducesVerifiableHash verifies that Create returns a well-formed
// argon2id encoded hash that Check accepts for the original input.
func TestCreateProducesVerifiableHash(t *testing.T) {
	const password = "correct horse battery staple"

	encoded, err := hash.Create(password)
	if err != nil {
		t.Fatalf("Create returned unexpected error: %v", err)
	}

	// Create builds the standard PHC string layout produced by argon2id.
	if !strings.HasPrefix(encoded, "$argon2id$v=19$") {
		t.Errorf("hash %q does not have the expected argon2id prefix", encoded)
	}

	// The encoded hash is $argon2id$v=..$m=..,t=..,p=..$salt$key -> 6 parts.
	if parts := strings.Split(encoded, "$"); len(parts) != 6 {
		t.Errorf("expected 6 $-delimited segments, got %d in %q", len(parts), encoded)
	}

	match, err := hash.Check(encoded, password)
	if err != nil {
		t.Fatalf("Check returned unexpected error for valid hash: %v", err)
	}
	if !match {
		t.Errorf("Check reported no match for the original password")
	}
}

// TestCheckCorrectPassword verifies Check returns true only for the exact input
// used to create the hash and false for any other password.
func TestCheckCorrectPassword(t *testing.T) {
	const password = "s3cr3t-P@ssw0rd"

	encoded, err := hash.Create(password)
	if err != nil {
		t.Fatalf("Create returned unexpected error: %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "exact match", input: password, want: true},
		{name: "wrong password", input: "s3cr3t-P@ssw0rd!", want: false},
		{name: "empty input", input: "", want: false},
		{name: "case mismatch", input: "S3CR3T-p@SSW0RD", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			match, err := hash.Check(encoded, tt.input)
			if err != nil {
				t.Fatalf("Check returned unexpected error: %v", err)
			}
			if match != tt.want {
				t.Errorf("Check(%q) = %v, want %v", tt.input, match, tt.want)
			}
		})
	}
}

// TestCheckMalformedHash verifies Check surfaces an error and reports no match
// when the stored hash is not a valid argon2id encoded string. argon2id's
// DecodeHash returns ErrInvalidHash / ErrIncompatibleVariant for these, which
// Create's Check wraps, so the result is (false, non-nil error).
func TestCheckMalformedHash(t *testing.T) {
	valid, err := hash.Create("anything")
	if err != nil {
		t.Fatalf("Create returned unexpected error: %v", err)
	}

	tests := []struct {
		name       string
		storedHash string
	}{
		{name: "empty hash", storedHash: ""},
		{name: "not a hash", storedHash: "definitely-not-a-hash"},
		{name: "too few segments", storedHash: "$argon2id$v=19$badsalt"},
		{name: "wrong variant", storedHash: "$argon2i$v=19$m=65536,t=1,p=4$c29tZXNhbHQ$c29tZWtleQ"},
		{name: "truncated valid hash", storedHash: valid[:len(valid)-5]},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			match, err := hash.Check(tt.storedHash, "anything")
			if err == nil {
				t.Errorf("expected an error for malformed hash %q, got nil", tt.storedHash)
			}
			if match {
				t.Errorf("expected no match for malformed hash %q, got true", tt.storedHash)
			}
		})
	}
}

// TestCreateDistinctSalts verifies that hashing the same input twice yields two
// different encoded strings (because a fresh random salt is used each time) yet
// both still verify against the original password.
func TestCreateDistinctSalts(t *testing.T) {
	const password = "repeatable-input"

	first, err := hash.Create(password)
	if err != nil {
		t.Fatalf("first Create returned unexpected error: %v", err)
	}
	second, err := hash.Create(password)
	if err != nil {
		t.Fatalf("second Create returned unexpected error: %v", err)
	}

	if first == second {
		t.Errorf("expected distinct hashes for identical input, both were %q", first)
	}

	for i, encoded := range []string{first, second} {
		match, err := hash.Check(encoded, password)
		if err != nil {
			t.Fatalf("Check on hash #%d returned unexpected error: %v", i, err)
		}
		if !match {
			t.Errorf("hash #%d did not verify against the original password", i)
		}
	}
}

// TestCreateEmptyInput documents that Create hashes an empty password without
// error and Check confirms the empty string against that hash.
func TestCreateEmptyInput(t *testing.T) {
	encoded, err := hash.Create("")
	if err != nil {
		t.Fatalf("Create(\"\") returned unexpected error: %v", err)
	}

	match, err := hash.Check(encoded, "")
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}
	if !match {
		t.Errorf("empty password did not verify against its own hash")
	}

	match, err = hash.Check(encoded, "not empty")
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}
	if match {
		t.Errorf("non-empty input unexpectedly matched the empty-password hash")
	}
}
