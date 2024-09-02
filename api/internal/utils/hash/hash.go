package hash

import (
	"fmt"

	"github.com/alexedwards/argon2id"
)

func Create(input string) (string, error) {
	const (
		argonMemory      = 64 * 1024
		argonIterations  = 1
		argonParallelism = 4
		argonSaltLength  = 16
		argonKeyLength   = 16
	)

	params := &argon2id.Params{
		Memory:      argonMemory,
		Iterations:  argonIterations,
		Parallelism: argonParallelism,
		SaltLength:  argonSaltLength,
		KeyLength:   argonKeyLength,
	}

	hash, err := argon2id.CreateHash(input, params)
	if err != nil {
		return "", fmt.Errorf("cannot hash string: %w", err)
	}
	return hash, nil
}

func Check(storedHash, input string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(input, storedHash)
	if err != nil {
		return false, fmt.Errorf("cannot verify input: %w", err)
	}
	return match, nil
}
