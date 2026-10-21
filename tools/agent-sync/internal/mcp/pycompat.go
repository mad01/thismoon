package mcp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The helpers here reproduce the Python behavior only sync_mcp.py relied
// on: the whitespace and line splitting it used to parse the claude CLI's
// output, and Path.mkdir for the stamp's directory. The value and path
// helpers both sections share live in pyjson and pypath.

// pyIsSpace reports whether r is whitespace to Python's str.split and
// str.strip: Go's unicode.IsSpace plus the four ASCII separators.
func pyIsSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

// pyStrip returns s.strip().
func pyStrip(s string) string { return strings.TrimFunc(s, pyIsSpace) }

// pyFields returns s.split().
func pyFields(s string) []string { return strings.FieldsFunc(s, pyIsSpace) }

// pyText decodes a subprocess's output the way text=True does: universal
// newlines turn \r\n and a lone \r into \n.
func pyText(b []byte) string {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// pySplitLines returns s.splitlines() for text pyText decoded, which holds
// no \r.
func pySplitLines(s string) []string {
	var lines []string
	start := 0
	for i, r := range s {
		switch r {
		case '\n', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
			lines = append(lines, s[start:i])
			start = i + utf8.RuneLen(r)
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// mkdirParents creates dir the way Path.mkdir(parents=True,
// exist_ok=True) does: parents first when dir's parent is missing, and an
// existing directory is not an error.
func mkdirParents(dir string) error {
	err := os.Mkdir(dir, 0o777)
	if !errors.Is(err, fs.ErrNotExist) {
		return existOK(dir, err)
	}
	parent := filepath.Dir(dir)
	if parent == dir {
		return err
	}
	if err := mkdirParents(parent); err != nil {
		return err
	}
	err = os.Mkdir(dir, 0o777)
	if errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return existOK(dir, err)
}

// existOK drops a mkdir error when dir is a directory after all.
func existOK(dir string, err error) error {
	if err == nil {
		return nil
	}
	if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
		return nil
	}
	return err
}
