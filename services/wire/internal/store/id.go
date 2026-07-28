package store

import (
	"encoding/hex"
	"fmt"
	"io"
)

// idPrefix marks a channel id. It contains an underscore, which the channel
// name grammar excludes, so an id can never be mistaken for a name and either
// resolves a channel unambiguously.
const idPrefix = "ch_"

// newID mints a channel id: "ch_" plus 12 hex chars. Unlike the ids in the
// sibling services it carries no timestamp — channels are listed by their last
// activity, and a short id is easier to paste into another agent's prompt.
// rnd is the randomness source (crypto/rand.Reader in production; pinned in
// tests).
func newID(rnd io.Reader) string {
	return idPrefix + randHex(rnd, 6)
}

// newName mints a fallback channel name for an opener that did not pick one.
// It is already a valid slug, so a generated channel is handed over the same
// way as a named one.
func newName(rnd io.Reader) string {
	return "ch-" + randHex(rnd, 3)
}

// randHex reads n random bytes and renders them as hex. The default source
// (crypto/rand) never fails on supported platforms; panic rather than silently
// mint a colliding id.
func randHex(rnd io.Reader, n int) string {
	b := make([]byte, n)
	if _, err := io.ReadFull(rnd, b); err != nil {
		panic(fmt.Sprintf("store: rand read failed: %v", err))
	}
	return hex.EncodeToString(b)
}
