// Package audiocache keeps synthesized speech on disk, addressed by what
// decides its sound, and synthesizes clips ahead of playback. A remote TTS
// model takes seconds to tens of seconds per request and answers with the
// whole clip at once, so speak serve prepares an uploaded document's parts in
// the background and plays them from disk; a part asked for before it is
// ready jumps the queue.
package audiocache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
)

// keyHexLen is how many hex characters of the sha256 a key keeps: 128 bits,
// far beyond what one machine's clips could ever collide in.
const keyHexLen = 32

// Key addresses a clip: a sha256 over the clip ID (provider, model and
// voice), the speed and the text, cut to keyHexLen lowercase hex characters.
// The clip ID is length-prefixed and the formatted speed holds no newline,
// so no two inputs can run together into the same hashed bytes.
func Key(clipID string, speed float64, text string) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%d:%s\n%s\n", len(clipID), clipID,
		strconv.FormatFloat(speed, 'g', -1, 64))
	_, _ = io.WriteString(h, text)
	return hex.EncodeToString(h.Sum(nil))[:keyHexLen]
}

// ValidKey reports whether s has the shape Key produces. Keys arrive in URLs,
// so every path is built only from a key that passed this check: it cannot
// hold a separator or a dot and so cannot leave the cache directory.
func ValidKey(s string) bool {
	if len(s) != keyHexLen {
		return false
	}
	for _, c := range []byte(s) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
