package store

import (
	"crypto/rand"
	"encoding/hex"
)

// idBytes is the number of random bytes per page id. 5 bytes -> 10 hex chars,
// which is short enough to paste in a URL yet collision-safe for this use.
const idBytes = 5

// NewID returns a random lowercase-hex page id (10 characters).
func NewID() string {
	b := make([]byte, idBytes)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read never returns an error on supported platforms;
		// panic rather than silently mint a weak id.
		panic("present: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
