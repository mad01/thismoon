package lifecycle

import (
	"crypto/sha1" //nolint:gosec // matches a content_hash a source was pinned with, not a security check
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
)

// DefaultHashAlgorithm is the algorithm of a content_hash without a prefix.
const DefaultHashAlgorithm = "sha256"

// hashAlgorithms are the content_hash algorithms brain computes, by the
// prefix a content_hash names them with (a hyphen, as in sha-256, is
// ignored).
var hashAlgorithms = map[string]func() hash.Hash{
	"sha256": sha256.New,
	"sha512": sha512.New,
	"sha1":   sha1.New,
}

// ContentHash is a well-formed sources[].content_hash: an algorithm brain
// computes and the full lowercase hex digest of it.
type ContentHash struct {
	Algorithm string
	Digest    string
}

// New returns a fresh hash of the content hash's algorithm.
func (h ContentHash) New() hash.Hash { return hashAlgorithms[h.Algorithm]() }

// ParseContentHash splits a content_hash into its algorithm (sha256 when it
// names none) and its hex digest, ignoring case, and refuses one that is
// not the full digest of sha256, sha512 or sha1. brain check and brain
// drift both judge a content_hash by it. The error names the value, not
// the key.
func ParseContentHash(raw string) (ContentHash, error) {
	algorithm, digest, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok {
		algorithm, digest = DefaultHashAlgorithm, algorithm
	}
	h := ContentHash{
		Algorithm: strings.ReplaceAll(strings.ToLower(algorithm), "-", ""),
		Digest:    strings.ToLower(digest),
	}
	newHash, known := hashAlgorithms[h.Algorithm]
	if !known {
		return h, fmt.Errorf("%q names %q, not one of sha256, sha512 or sha1", raw, h.Algorithm)
	}
	if want := 2 * newHash().Size(); len(h.Digest) != want {
		return h, fmt.Errorf("%q has %d hex digits, a full %s digest has %d",
			raw, len(h.Digest), h.Algorithm, want)
	}
	if _, err := hex.DecodeString(h.Digest); err != nil {
		return h, fmt.Errorf("%q is not a hex digest", raw)
	}
	return h, nil
}
