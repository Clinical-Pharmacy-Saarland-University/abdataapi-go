package tokens

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"observeddb-go-api/internal/utils/hash"
)

type ResetTokenPair struct {
	Token     string `json:"token"`
	TokenHash string `json:"token_hash"`
}

func CreateResetTokens() (*ResetTokenPair, error) {
	const nBytes = 32
	token := make([]byte, nBytes)

	if _, err := rand.Read(token); err != nil {
		return nil, fmt.Errorf("cannot generate reset token: %w", err)
	}

	tokenString := base64.URLEncoding.EncodeToString(token)
	tokenHash, err := hash.Create(tokenString)
	if err != nil {
		return nil, fmt.Errorf("cannot hash reset token: %w", err)
	}

	return &ResetTokenPair{
		Token:     tokenString,
		TokenHash: tokenHash,
	}, nil
}
