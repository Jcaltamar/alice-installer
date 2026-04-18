// Package secrets provides cryptographically-secure secret generation utilities.
package secrets

import (
	"crypto/rand"
	"encoding/base64"
)

// PasswordGenerator generates random passwords/tokens.
type PasswordGenerator interface {
	Generate(byteLen int) (string, error)
}

// CryptoRandGenerator implements PasswordGenerator using crypto/rand.
// The returned string is the standard base64 encoding of byteLen random bytes.
type CryptoRandGenerator struct{}

// Generate returns the base64 encoding of byteLen cryptographically-random bytes.
// A byteLen of 0 returns an empty string with no error.
func (CryptoRandGenerator) Generate(byteLen int) (string, error) {
	if byteLen == 0 {
		return "", nil
	}

	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b), nil
}
