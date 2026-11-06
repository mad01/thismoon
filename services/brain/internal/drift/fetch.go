package drift

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
)

// maxFetchBytes caps the body read from one external source.
const maxFetchBytes = 32 << 20

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
	pin, err := lifecycle.ParseContentHash(s.ContentHash)
	if err != nil {
		return finding(KindBadHash, "content_hash %v", err)
	}
	h := pin.New()
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
	if got := hex.EncodeToString(h.Sum(nil)); got != pin.Digest {
		return finding(KindHashChanged, "the content no longer matches content_hash %s (now %s:%s)",
			s.ContentHash, pin.Algorithm, got)
	}
	return nil
}
