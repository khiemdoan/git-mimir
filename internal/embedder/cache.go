package embedder

import (
	"crypto/sha256"
	"fmt"
)

// TextHash returns the sha256 hex of a text string.
func TextHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h[:])
}
