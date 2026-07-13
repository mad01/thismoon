package store

import (
	"fmt"
	"io"
	"time"
)

// NewID returns a time-sortable id for an assertion created at t: the UTC
// unix-nano timestamp zero-padded to 20 digits, a dash, then two random bytes
// as 4 hex chars. Fixed widths make lexical order match time order, with the
// random suffix as a stable tiebreak for assertions in the same nanosecond.
// rnd is the randomness source (crypto/rand.Reader in production; pinned in
// tests).
func NewID(t time.Time, rnd io.Reader) string {
	var b [2]byte
	if _, err := io.ReadFull(rnd, b[:]); err != nil {
		// The default source (crypto/rand) never fails on supported platforms;
		// panic rather than silently mint a non-unique id.
		panic("store: rand read failed: " + err.Error())
	}
	return fmt.Sprintf("%020d-%04x", t.UTC().UnixNano(), b[:])
}
