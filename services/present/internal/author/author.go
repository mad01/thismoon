// Package author turns the author key a client presents into the identity a
// shared page carries. A key is self-issued: the client mints it once, sends
// it as a bearer token on every write, and the server stores only its hash.
// A page's author is therefore "whoever holds this key" and nothing more:
// there are no accounts to look up and no revocation list, so a leaked key
// is the author until its pages are re-shared under a new one.
package author

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
)

// keyBytes is the entropy of a minted key: 32 bytes, 64 hex characters.
const keyBytes = 32

// ErrMissing is returned when a write carries no author key. Handlers map it
// to 401.
var ErrMissing = errors.New("authorization required: send the author key as a bearer token")

// ErrMismatch is returned when the key presented does not hash to the page's
// author. Handlers map it to 403.
var ErrMismatch = errors.New("page belongs to another author")

// NewKey mints a fresh author key.
func NewKey() string {
	b := make([]byte, keyBytes)
	if _, err := rand.Read(b); err != nil {
		panic("present: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Hash returns the hex SHA-256 of key: what a page stores as its author.
func Hash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// FromHeader reads the bearer token from an Authorization header and returns
// its hash. ok is false when the header is absent, is not a bearer scheme,
// or carries an empty token.
func FromHeader(h http.Header) (hash string, ok bool) {
	if h == nil {
		return "", false
	}
	scheme, key, found := strings.Cut(strings.TrimSpace(h.Get("Authorization")), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	return Hash(key), true
}

// Check reports whether the key in h is pageAuthor's. It returns ErrMissing
// without a key and ErrMismatch when the hashes differ; a page with no
// author matches no key.
func Check(pageAuthor string, h http.Header) error {
	hash, ok := FromHeader(h)
	if !ok {
		return ErrMissing
	}
	if pageAuthor == "" || subtle.ConstantTimeCompare([]byte(hash), []byte(pageAuthor)) != 1 {
		return ErrMismatch
	}
	return nil
}
