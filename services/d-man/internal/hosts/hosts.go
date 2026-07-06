// Package hosts owns the managed block d-man maintains inside /etc/hosts.
//
// The mutation flow is fail-safe: the pure functions (Validate, Render, Splice)
// are tested in isolation; Sync is the only one that touches the filesystem and
// it refuses to write anything it would have made worse than what it found.
//
// Splitting it this way keeps the dangerous part small: Sync never edits
// foreign lines — it replaces only the text between our two marker lines and
// leaves every other byte exactly as it was.
package hosts

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	beginMarker = "# >>> d-man managed >>>"
	endMarker   = "# <<< d-man managed <<<"
	// banner sits inside the block; a reminder for anyone reading /etc/hosts.
	banner = "# Managed by d-man — do not edit by hand; edit routes.toml instead."
)

// hostLabel is RFC 1123: a label is alphanumeric with internal hyphens, 1-63
// chars. A hostname is dot-joined labels.
var hostLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// Result reports what Sync did.
type Result struct {
	Changed  bool     // true if /etc/hosts was rewritten
	Backup   string   // path of the backup written, "" if none
	Warnings []string // non-fatal issues (e.g. pre-existing malformed lines)
}

// Validate parses hosts-file content and returns an error on the first line
// that is neither blank, a comment, nor a well-formed "IP host [aliases…]"
// entry. It is the strict check used to guard our own output.
func Validate(data []byte) error {
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if net.ParseIP(fields[0]) == nil {
			return fmt.Errorf("line %d: %q is not a valid IP", i+1, fields[0])
		}
		if len(fields) < 2 {
			return fmt.Errorf("line %d: IP %q has no hostname", i+1, fields[0])
		}
		for _, h := range fields[1:] {
			if !ValidHostname(h) {
				return fmt.Errorf("line %d: invalid hostname %q", i+1, h)
			}
		}
	}
	return nil
}

// Render builds the managed block (markers + one 127.0.0.1 line per host). The
// returned text has no trailing newline; Splice adds line separation.
func Render(hosts []string) string {
	var b strings.Builder
	b.WriteString(beginMarker)
	b.WriteByte('\n')
	b.WriteString(banner)
	for _, h := range hosts {
		b.WriteString("\n127.0.0.1\t")
		b.WriteString(h)
	}
	b.WriteByte('\n')
	b.WriteString(endMarker)
	return b.String()
}

// Splice replaces the existing managed block (between the markers) with block,
// or appends block if no markers are present. Every byte outside the block is
// preserved verbatim.
func Splice(existing []byte, block string) []byte {
	text := string(existing)
	block = strings.TrimRight(block, "\n")

	bStart, _, bok := findMarkerLine(text, beginMarker)
	_, eEnd, eok := findMarkerLine(text, endMarker)

	if bok && eok && eEnd >= bStart {
		// before ends with the newline that preceded the begin marker (or is
		// empty); after starts right past the end marker's newline.
		return []byte(text[:bStart] + block + "\n" + text[eEnd:])
	}

	var sb strings.Builder
	sb.WriteString(text)
	if len(text) > 0 && !strings.HasSuffix(text, "\n") {
		sb.WriteByte('\n')
	}
	sb.WriteString(block)
	sb.WriteByte('\n')
	return []byte(sb.String())
}

// Sync writes the managed block for hosts into the file at path. It never
// produces a file worse than the one it read: our own block is validated
// strictly, and if the spliced result is invalid only because the existing file
// already was, the foreign breakage is preserved and reported as a warning
// rather than aborting. A backup is written before any change.
func Sync(path string, hosts []string) (Result, error) {
	var res Result

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return res, fmt.Errorf("read %s: %w", path, err)
	}
	existingErr := Validate(existing)
	if existingErr != nil {
		res.Warnings = append(
			res.Warnings,
			fmt.Sprintf(
				"existing %s has a malformed line (preserved, not edited): %v",
				path,
				existingErr,
			),
		)
	}

	block := Render(hosts)
	if err := Validate([]byte(block)); err != nil {
		// Defensive: config validation should make this impossible.
		return res, fmt.Errorf("refusing to write: generated block is invalid: %w", err)
	}

	next := Splice(existing, block)
	if err := Validate(next); err != nil {
		if existingErr == nil {
			return res, fmt.Errorf("refusing to write: result would be invalid: %w", err)
		}
		// The only invalidity is the pre-existing foreign one; already warned.
	}

	if bytes.Equal(next, existing) {
		return res, nil // idempotent: nothing to do
	}

	backup, err := writeAtomic(path, next, existing)
	if err != nil {
		return res, err
	}
	res.Changed = true
	res.Backup = backup
	return res, nil
}

// writeAtomic backs up the current file then atomically replaces it: write a
// temp file in the same directory, fsync, and rename over the target.
func writeAtomic(path string, data, existing []byte) (string, error) {
	dir := filepath.Dir(path)

	var backup string
	if len(existing) > 0 {
		backup = path + ".d-man.bak"
		if err := os.WriteFile(backup, existing, 0o644); err != nil {
			return "", fmt.Errorf("write backup %s: %w", backup, err)
		}
	}

	tmp, err := os.CreateTemp(dir, ".d-man-hosts-*")
	if err != nil {
		return backup, fmt.Errorf("create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // harmless no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return backup, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return backup, err
	}
	if err := tmp.Close(); err != nil {
		return backup, err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return backup, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return backup, fmt.Errorf("rename into place: %w", err)
	}
	return backup, nil
}

// ValidHostname reports whether s is a valid dotted hostname (each label
// RFC 1123, total length <= 253).
func ValidHostname(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if !hostLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// findMarkerLine returns the byte offsets [lineStart, lineEnd) of the first line
// that exactly equals marker (lineEnd includes the trailing newline if present).
func findMarkerLine(text, marker string) (start, end int, ok bool) {
	pos := 0
	for pos <= len(text) {
		nl := strings.IndexByte(text[pos:], '\n')
		var line string
		var lineEnd int
		if nl < 0 {
			line = text[pos:]
			lineEnd = len(text)
		} else {
			line = text[pos : pos+nl]
			lineEnd = pos + nl + 1
		}
		if strings.TrimRight(line, "\r") == marker {
			return pos, lineEnd, true
		}
		if nl < 0 {
			break
		}
		pos += nl + 1
	}
	return 0, 0, false
}
