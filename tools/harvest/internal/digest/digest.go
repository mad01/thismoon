// Package digest cuts a Claude Code transcript down to the text a person and
// the assistant wrote, in order. Tool calls, tool output, thinking, subagent
// turns and harness notes stay out: they are most of a transcript's bytes and
// little of its knowledge.
//
// A transcript is JSONL, and one line (a tool result, a pasted file) can run
// to many megabytes, so lines are read with no length limit.
package digest

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Digest is the rendered text of the transcript lines after a mark.
type Digest struct {
	// Text is the rendered entries, cut to the last maxBytes bytes.
	Text string
	// Lines is the number of complete lines in the whole transcript: the
	// mark the next harvest of the same session starts after.
	Lines int
}

// Build reads a transcript from r, skips its first skip lines, and renders
// the rest with Entry. Only newline-terminated lines count: a last line still
// being written is left for the next harvest, whole. A line that does not
// decode is counted and skipped. The result keeps the last maxBytes bytes of
// text, cut on a rune boundary.
func Build(r io.Reader, skip, maxBytes int) (Digest, error) {
	br := bufio.NewReader(r)
	var b strings.Builder
	lines := 0
	for {
		line, err := br.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Digest{}, fmt.Errorf("digest: read transcript: %w", err)
		}
		lines++
		if lines <= skip {
			continue
		}
		if entry, ok := Entry(line); ok {
			b.WriteString(entry)
		}
	}
	return Digest{Text: Tail(b.String(), maxBytes), Lines: lines}, nil
}

// line is the part of a transcript line Entry reads.
type line struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// block is one element of a message's content array.
type block struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Entry renders one transcript line as "## <type>\n<text>\n\n". ok is false
// for a line that is not a user or assistant turn, is a subagent
// (isSidechain) or harness (isMeta) line, or carries no text. The content is
// a string or an array of blocks; only "text" blocks count, joined by
// newlines.
func Entry(raw []byte) (string, bool) {
	var l line
	if err := json.Unmarshal(raw, &l); err != nil {
		return "", false
	}
	if (l.Type != "user" && l.Type != "assistant") || l.IsSidechain || l.IsMeta {
		return "", false
	}
	text := content(l.Message.Content)
	if text == "" {
		return "", false
	}
	return "## " + l.Type + "\n" + text + "\n\n", true
}

// content returns the text in a message's content: the string itself, or
// the text blocks of an array joined by newlines. Anything else has none.
func content(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []block
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// Tail returns the last n bytes of s, moved forward to the next rune start so
// the cut never splits a character. The result may be a few bytes short of n.
func Tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	i := len(s) - max(n, 0)
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return s[i:]
}
