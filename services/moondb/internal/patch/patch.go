// Package patch parses and applies the per-file hunks the harness reports
// for an edit, the structuredPatch shape: a JSON array of hunks whose lines
// start with ' ', '-' or '+'. Apply and Reverse check every context and
// changed line at the position the hunk states, so a wrong guess at a file's
// content fails with a MismatchError instead of producing a plausible file.
//
// Matching ignores line endings. Output keeps the ending of every line it
// copies and gives the lines a hunk adds the file's dominant ending, so a
// CRLF file stays CRLF whether the hunks carry carriage returns or not.
package patch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// noNewlineMarker is the line jsdiff and diff(1) put after the last line of
// a side that has no trailing newline. Only its leading '\' is significant.
const noNewlineMarker = '\\'

// Hunk is one hunk of a structuredPatch. Lines start with ' ' (context),
// '-' (removed) or '+' (added); a line starting with '\' marks the line
// before it as the last line of its side, with no newline after it.
// OldStart and NewStart are 1-based; a side with no lines may state 0.
type Hunk struct {
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Lines    []string `json:"lines"`
}

// MismatchError reports a hunk line that did not match the content it was
// applied to. Expected and Got are the line texts with "\n" appended when
// the line ends in a newline of either kind, so a missing final newline
// shows. Got is "" when the content ended before the line, and Expected is
// "" when the hunk ends the file and the content goes on.
type MismatchError struct {
	// Hunk is the 0-based index of the hunk in the patch.
	Hunk int
	// Line is the 1-based line number in the content being patched.
	Line     int
	Expected string
	Got      string
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf("patch: hunk %d line %d: expected %s, got %s",
		e.Hunk, e.Line, quoteLine(e.Expected), quoteLine(e.Got))
}

func quoteLine(s string) string {
	if s == "" {
		return "end of file"
	}
	return fmt.Sprintf("%q", s)
}

// Parse decodes a structuredPatch JSON array and checks that every hunk's
// line counts agree with its header. A JSON null yields no hunks.
func Parse(raw []byte) ([]Hunk, error) {
	var hunks []Hunk
	if err := json.Unmarshal(raw, &hunks); err != nil {
		return nil, fmt.Errorf("patch: parse: %w", err)
	}
	if err := validate(hunks); err != nil {
		return nil, err
	}
	return hunks, nil
}

// Apply returns before with the hunks applied. Every context and removed
// line must match before at the position its hunk states.
func Apply(before []byte, hunks []Hunk) ([]byte, error) {
	if err := validate(hunks); err != nil {
		return nil, err
	}
	src := split(before)
	out, err := apply(src, hunks, dominantEOL(src, hunks))
	if err != nil {
		return nil, err
	}
	return join(out), nil
}

// Reverse returns the content the hunks were applied to, given the content
// they produced. Every context and added line must match after at the
// position its hunk states.
func Reverse(after []byte, hunks []Hunk) ([]byte, error) {
	if err := validate(hunks); err != nil {
		return nil, err
	}
	inverted := invert(hunks)
	src := split(after)
	out, err := apply(src, inverted, dominantEOL(src, inverted))
	if err != nil {
		return nil, err
	}
	return join(out), nil
}

// DetectEOL names the line ending convention of content: "lf", "crlf",
// "mixed" when both occur, or "" when it has no line endings.
func DetectEOL(content []byte) string {
	crlf := bytes.Count(content, []byte("\r\n"))
	lf := bytes.Count(content, []byte("\n")) - crlf
	switch {
	case crlf > 0 && lf > 0:
		return "mixed"
	case crlf > 0:
		return "crlf"
	case lf > 0:
		return "lf"
	default:
		return ""
	}
}

// AddedLines returns the text of every line the hunks add, without the '+'
// prefix or a trailing carriage return.
func AddedLines(hunks []Hunk) []string {
	var added []string
	for _, h := range hunks {
		for _, l := range h.Lines {
			if strings.HasPrefix(l, "+") {
				added = append(added, strings.TrimSuffix(l[1:], "\r"))
			}
		}
	}
	return added
}

// Touches reports whether the 1-based line of the new file falls inside a
// hunk's new range. A hunk that only removes lines touches no new line.
func Touches(hunks []Hunk, line int) bool {
	for _, h := range hunks {
		if h.NewLines > 0 && line >= h.NewStart && line < h.NewStart+h.NewLines {
			return true
		}
	}
	return false
}

// validate checks the hunks' shape: known line prefixes, a marker only after
// a line, and line counts that agree with each header.
func validate(hunks []Hunk) error {
	for i, h := range hunks {
		if err := h.validate(); err != nil {
			return fmt.Errorf("patch: hunk %d: %w", i, err)
		}
	}
	return nil
}

func (h Hunk) validate() error {
	if h.OldStart < 0 || h.NewStart < 0 || h.OldLines < 0 || h.NewLines < 0 {
		return errors.New("negative position or count")
	}
	if h.OldLines > 0 && h.OldStart == 0 {
		return errors.New("old lines start at line 0")
	}
	var oldN, newN int
	for j, l := range h.Lines {
		if l == "" {
			return fmt.Errorf("line %d is empty", j)
		}
		switch l[0] {
		case ' ':
			oldN++
			newN++
		case '-':
			oldN++
		case '+':
			newN++
		case noNewlineMarker:
			if j == 0 || h.Lines[j-1][0] == noNewlineMarker {
				return fmt.Errorf("line %d: no-newline marker follows no line", j)
			}
		default:
			return fmt.Errorf("line %d: unknown prefix %q", j, l[0])
		}
	}
	if oldN != h.OldLines || newN != h.NewLines {
		return fmt.Errorf("lines count %d old, %d new; header says %d, %d",
			oldN, newN, h.OldLines, h.NewLines)
	}
	return nil
}

// invert swaps the sides of every hunk, so applying the result undoes the
// original. A no-newline marker stays attached to the line it follows.
func invert(hunks []Hunk) []Hunk {
	out := make([]Hunk, len(hunks))
	for i, h := range hunks {
		lines := make([]string, len(h.Lines))
		for j, l := range h.Lines {
			switch l[0] {
			case '-':
				l = "+" + l[1:]
			case '+':
				l = "-" + l[1:]
			}
			lines[j] = l
		}
		out[i] = Hunk{
			OldStart: h.NewStart, OldLines: h.NewLines,
			NewStart: h.OldStart, NewLines: h.OldLines,
			Lines: lines,
		}
	}
	return out
}
