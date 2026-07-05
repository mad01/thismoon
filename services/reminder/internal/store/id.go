package store

import (
	"crypto/rand"
	"encoding/hex"
)

// idBytes is the number of random bytes per reminder id. 5 bytes -> 10 hex
// chars, short enough to paste yet collision-safe for this single-user store.
const idBytes = 5

// NewID returns a random lowercase-hex reminder id (10 characters).
func NewID() string {
	b := make([]byte, idBytes)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read never returns an error on supported platforms;
		// panic rather than silently mint a weak id.
		panic("reminder: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
