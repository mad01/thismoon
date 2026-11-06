package drift

import (
	"context"
	"crypto/sha1" //nolint:gosec // matches a content_hash a source was pinned with, not a security check
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
)

// maxFetchBytes caps the body read from one external source.
const maxFetchBytes = 32 << 20

// defaultHashAlgorithm is the algorithm of a content_hash without a prefix.
const defaultHashAlgorithm = "sha256"

// hashes are the content_hash algorithms drift can compute, by the prefix
// a content_hash names them with (a hyphen, as in sha-256, is ignored).
var hashes = map[string]func() hash.Hash{
	"sha256": sha256.New,
	"sha512": sha512.New,
	"sha1":   sha1.New,
}

// contentHash is a well-formed content_hash: an algorithm drift computes
// and the full lowercase hex digest of it.
type contentHash struct {
	algorithm string
	digest    string
}

// parseContentHash splits a content_hash into its algorithm (sha256 when it
// names none) and its hex digest, and refuses one that is not the full
// digest of an algorithm drift computes.
func parseContentHash(raw string) (contentHash, error) {
	algorithm, digest, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok {
		algorithm, digest = defaultHashAlgorithm, algorithm
	}
	h := contentHash{
		algorithm: strings.ReplaceAll(strings.ToLower(algorithm), "-", ""),
		digest:    strings.ToLower(digest),
	}
	newHash, known := hashes[h.algorithm]
	if !known {
		return h, fmt.Errorf("content_hash %q names %q, not one of sha256, sha512 or sha1",
			raw, h.algorithm)
	}
	if want := 2 * newHash().Size(); len(h.digest) != want {
		return h, fmt.Errorf("content_hash %q has %d hex digits, a %s digest has %d",
			raw, len(h.digest), h.algorithm, want)
	}
	if _, err := hex.DecodeString(h.digest); err != nil {
		return h, fmt.Errorf("content_hash %q is not a hex digest", raw)
	}
	return h, nil
}

func isWebURL(u string) bool {
	return strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://")
}

// fetch checks one external source: a 404 or 410 is gone, another failure
// unresolved, and a body that no longer matches content_hash hash-changed.
// A source without content_hash only has to answer.
func fetch(ctx context.Context, client *http.Client, s lifecycle.Source) []Finding {
	finding := func(kind, format string, args ...any) []Finding {
		return []Finding{{Kind: kind, Source: s.URL, Message: fmt.Sprintf(format, args...)}}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return finding(KindUnresolved, "%v", err)
	}
	req.Header.Set("User-Agent", "brain-drift")
	resp, err := client.Do(req)
	if err != nil {
		return finding(KindUnresolved, "fetch failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		out := finding(KindGone, "answers HTTP %d", resp.StatusCode)
		if s.Archive != "" {
			out[0].Archive = s.Archive
			out[0].Message += "; the archived copy is " + s.Archive
		} else {
			out[0].Message += "; no archive is recorded"
		}
		return out
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return finding(KindUnresolved, "answers HTTP %d", resp.StatusCode)
	}
	if s.ContentHash == "" {
		return nil
	}
	pin, err := parseContentHash(s.ContentHash)
	if err != nil {
		return finding(KindBadHash, "%v", err)
	}
	h := hashes[pin.algorithm]()
	n, err := io.Copy(h, io.LimitReader(resp.Body, maxFetchBytes+1))
	if err != nil {
		return finding(KindUnresolved, "read failed: %v", err)
	}
	if n > maxFetchBytes {
		return finding(
			KindUnresolved,
			"the body is over %d MiB, too large to hash",
			maxFetchBytes>>20,
		)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != pin.digest {
		return finding(KindHashChanged, "the content no longer matches content_hash %s (now %s:%s)",
			s.ContentHash, pin.algorithm, got)
	}
	return nil
}
