package store

import (
	"crypto/sha256"
	"encoding/hex"
)

// ChunkKey returns the key of data: the lowercase hex SHA-256 of its bytes.
func ChunkKey(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ValidChunkKey reports whether key has the shape of a chunk key: 64
// lowercase hex digits. A key from outside the process is checked before
// it names an object.
func ValidChunkKey(key string) bool {
	if len(key) != sha256.Size*2 {
		return false
	}
	for _, c := range []byte(key) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
