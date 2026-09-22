package store

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
)

// idBytes is the number of random bytes per local page id. 5 bytes -> 10 hex
// chars, which is short enough to paste in a URL yet collision-safe for a
// store one person writes.
const idBytes = 5

// sharedIDBytes is the number of random bytes per shared page id. A shared
// page is reachable only by its id, so the id is the capability: 16 bytes
// (128 bits) makes it unguessable, and 32 hex chars is still a valid
// Kubernetes object name.
const sharedIDBytes = 16

// idPattern accepts exactly the two shapes present mints. Anything else is
// not a page id, and rejecting it before a path join keeps a crafted id from
// escaping the pages directory.
var idPattern = regexp.MustCompile(`^(?:[0-9a-f]{10}|[0-9a-f]{32})$`)

// NewID returns a random lowercase-hex local page id (10 characters).
func NewID() string { return randomHex(idBytes) }

// NewSharedID returns a random lowercase-hex shared page id (32 characters).
func NewSharedID() string { return randomHex(sharedIDBytes) }

// ValidID reports whether id has the shape of a page id present minted.
func ValidID(id string) bool { return idPattern.MatchString(id) }

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read never returns an error on supported platforms;
		// panic rather than silently mint a weak id.
		panic("present: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
