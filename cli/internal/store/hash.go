package store

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// computeDataHash computes a SHA-256 hash over the given fields joined by "|".
// Used by InsertHookLog and InsertAudit to build per-table hash chains in data.db.
func computeDataHash(fields ...string) string {
	data := strings.Join(fields, "|")
	h := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", h[:])
}
