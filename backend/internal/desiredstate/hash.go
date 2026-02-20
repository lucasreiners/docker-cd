package desiredstate

import (
	"crypto/sha256"
	"encoding/hex"
)

// ComposeHash computes a deterministic SHA-256 hash of compose file contents.
func ComposeHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}
