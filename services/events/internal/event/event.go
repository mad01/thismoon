// Package event defines the Event record and its pure helpers: validation,
// source-name sanitization, and the time-sortable id generator. No I/O lives
// here — the store owns persistence.
package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Levels an event may carry. An empty level normalizes to LevelInfo.
const (
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// maxSourceLen caps a sanitized source name so it stays a sane filename.
const maxSourceLen = 64

// Event is one record in the audit log. Time and ID are stamped server-side at
// ingest; producers supply the rest.
type Event struct {
	ID        string            `json:"id"`                  // time-sortable: "%020d-%04x" of unixNanos + 2 random bytes, so lexical order == time order
	Time      time.Time         `json:"time"`                // UTC, stamped at ingest
	Source    string            `json:"source"`              // required; sanitized into the per-source filename
	Component string            `json:"component,omitempty"` // optional sub-area within a source
	Level     string            `json:"level"`               // info|warn|error (default info)
	Title     string            `json:"title"`               // required
	Message   string            `json:"message,omitempty"`   // optional longer detail
	Tags      map[string]string `json:"tags,omitempty"`      // optional key/value labels
	Data      json.RawMessage   `json:"data,omitempty"`      // optional structured payload, passed through opaquely
}

// Validate fails fast on a malformed event. An empty level is accepted (it
// defaults to info at ingest); a non-empty unknown level is rejected.
func (e Event) Validate() error {
	if strings.TrimSpace(e.Source) == "" {
		return errors.New("event: source is required")
	}
	if strings.TrimSpace(e.Title) == "" {
		return errors.New("event: title is required")
	}
	switch e.Level {
	case "", LevelInfo, LevelWarn, LevelError:
		return nil
	default:
		return fmt.Errorf("event: invalid level %q (want info|warn|error)", e.Level)
	}
}

// NormalizeLevel maps an empty level to the default (info) and leaves any other
// value untouched (callers validate it first).
func NormalizeLevel(level string) string {
	if level == "" {
		return LevelInfo
	}
	return level
}

// SanitizeSource lowercases s, keeps [a-z0-9_-], replaces every other run with a
// single dash, trims leading/trailing dashes, and caps the length. It returns an
// error if nothing usable survives, since the result names a file.
func SanitizeSource(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		var c byte
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			c = byte(r)
		default:
			c = '-'
		}
		// Collapse any run of dashes (kept or substituted) into one.
		if c == '-' && b.Len() > 0 && b.String()[b.Len()-1] == '-' {
			continue
		}
		b.WriteByte(c)
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > maxSourceLen {
		out = strings.Trim(out[:maxSourceLen], "-")
	}
	if out == "" {
		return "", errors.New("event: source is empty after sanitize")
	}
	return out, nil
}

// NewID returns a time-sortable id for an event stamped at t: the UTC unix-nano
// timestamp zero-padded to 20 digits, a dash, then two random bytes as 4 hex
// chars. Fixed widths make lexical order match time order, with the random
// suffix as a stable tiebreak for events in the same nanosecond. rnd is the
// randomness source (crypto/rand.Reader in production; pinned in tests).
func NewID(t time.Time, rnd io.Reader) string {
	var b [2]byte
	if _, err := io.ReadFull(rnd, b[:]); err != nil {
		// The default source (crypto/rand) never fails on supported platforms;
		// panic rather than silently mint a non-unique id.
		panic("event: rand read failed: " + err.Error())
	}
	return fmt.Sprintf("%020d-%04x", t.UTC().UnixNano(), b[:])
}
