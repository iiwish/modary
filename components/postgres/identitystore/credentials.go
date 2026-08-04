package identitystore

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const generatedBearerTokenBytes = 32

// GenerateBearerToken returns a URL-safe bearer credential with 256 bits of
// entropy from crypto/rand. The caller is responsible for displaying or
// persisting the plaintext exactly once; identitystore stores only its digest.
func GenerateBearerToken() (string, error) {
	secret := make([]byte, generatedBearerTokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate PostgreSQL identity store bearer token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(secret), nil
}
