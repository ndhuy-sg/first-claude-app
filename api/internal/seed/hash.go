package seed

import (
	"crypto/sha256"
	"encoding/hex"
)

// sha matches auth.HashToken (kept separate to avoid an import cycle).
func sha(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
