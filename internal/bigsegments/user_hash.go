package bigsegments

import (
	"crypto/sha256"
	"encoding/base64"
)

// HashForContextKey computes the hash that we use in the Big Segment store. This function is exported
// for use in LDClient tests.
func HashForContextKey(key string) string {
	hashBytes := sha256.Sum256([]byte(key))
	return base64.StdEncoding.EncodeToString(hashBytes[:])
}
