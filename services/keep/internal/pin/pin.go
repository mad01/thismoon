// Package pin turns a line range in a repo into a content-addressed evidence
// pin and later checks whether that evidence still holds. A pin captures the
// sha256 of the pinned lines (including their trailing newlines, so any edit
// to the range flips the hash) plus the HEAD commit at resolve time. Checking
// re-reads the working tree and re-hashes the same way; a mismatch, a vanished
// file, or a shrunk file is reported as a reason string, never an error.
package pin

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Pin is resolved evidence: a line range plus the content hash and commit that
// pinned it, so a later Check can tell whether the referenced code still holds.
type Pin struct {
	RepoPath      string    `json:"repo_path"`      // absolute path to repo working tree
	File          string    `json:"file"`           // repo-relative
	StartLine     int       `json:"start_line"`     // 1-based inclusive
	EndLine       int       `json:"end_line"`       // inclusive, >= StartLine
	ContentSHA256 string    `json:"content_sha256"` // sha256 hex of the line-range bytes
	HeadCommit    string    `json:"head_commit"`    // git rev-parse HEAD at resolve time
	ResolvedAt    time.Time `json:"resolved_at"`
}

// Ref is an unresolved request for a line range: the input to Resolve.
type Ref struct {
	RepoPath  string `json:"repo_path"`
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// Resolve reads the working-tree file for ref, hashes the requested line range,
// and stamps the current HEAD commit, returning a Pin. It reports a distinct
// wrapped error for each way the range can be invalid.
func Resolve(ref Ref, now time.Time) (Pin, error) {
	info, err := os.Stat(ref.RepoPath)
	if err != nil || !info.IsDir() {
		return Pin{}, fmt.Errorf("resolve pin: repo path %q is not a directory", ref.RepoPath)
	}
	content, err := os.ReadFile(filepath.Join(ref.RepoPath, ref.File))
	if err != nil {
		return Pin{}, fmt.Errorf("resolve pin: read file %q: %w", ref.File, err)
	}
	if ref.StartLine < 1 {
		return Pin{}, fmt.Errorf("resolve pin: start_line %d must be >= 1", ref.StartLine)
	}
	if ref.EndLine < ref.StartLine {
		return Pin{}, fmt.Errorf(
			"resolve pin: end_line %d is before start_line %d",
			ref.EndLine,
			ref.StartLine,
		)
	}
	total := countLines(content)
	if ref.EndLine > total {
		return Pin{}, fmt.Errorf(
			"resolve pin: end_line %d is beyond end of file (%d lines)",
			ref.EndLine,
			total,
		)
	}
	head, err := headCommit(ref.RepoPath)
	if err != nil {
		return Pin{}, fmt.Errorf("resolve pin: %w", err)
	}
	return Pin{
		RepoPath:      ref.RepoPath,
		File:          ref.File,
		StartLine:     ref.StartLine,
		EndLine:       ref.EndLine,
		ContentSHA256: hashLines(content, ref.StartLine, ref.EndLine),
		HeadCommit:    head,
		ResolvedAt:    now.UTC(),
	}, nil
}

// Check re-reads the working tree and reports whether the pin still holds. It
// never returns an error: a failure is the reason string ("repo missing",
// "file missing", "range out of bounds", "content changed"). A holding pin
// returns (true, "").
func Check(p Pin) (ok bool, reason string) {
	info, err := os.Stat(p.RepoPath)
	if err != nil || !info.IsDir() {
		return false, "repo missing"
	}
	content, err := os.ReadFile(filepath.Join(p.RepoPath, p.File))
	if err != nil {
		return false, "file missing"
	}
	if p.StartLine < 1 || p.EndLine < p.StartLine || p.EndLine > countLines(content) {
		return false, "range out of bounds"
	}
	if hashLines(content, p.StartLine, p.EndLine) != p.ContentSHA256 {
		return false, "content changed"
	}
	return true, ""
}

// headCommit returns the trimmed HEAD commit of the git repo at repoPath.
func headCommit(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD in %q: %w", repoPath, err)
	}
	head := strings.TrimSpace(string(out))
	if head == "" {
		return "", errors.New("git rev-parse HEAD returned empty output")
	}
	return head, nil
}

// hashLines returns the hex sha256 of lines start..end (1-based inclusive),
// including each line's trailing '\n', so a whitespace-only edit flips the
// hash. Callers guarantee the range is in bounds.
func hashLines(content []byte, start, end int) string {
	offsets := lineOffsets(content)
	sum := sha256.Sum256(content[offsets[start-1]:offsets[end]])
	return fmt.Sprintf("%x", sum)
}

// countLines returns the number of lines in content. A trailing final newline
// does not create a phantom extra line, so "a\nb\n" and "a\nb" both count 2.
func countLines(content []byte) int {
	return len(lineOffsets(content)) - 1
}

// lineOffsets returns the byte offset at which each line starts, plus a final
// entry at the end of content, so lines i (1-based) span offsets[i-1]:offsets[i]
// including the line's trailing '\n'. Returns [0] for empty content (0 lines).
func lineOffsets(content []byte) []int {
	offsets := []int{0}
	for i, b := range content {
		if b == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	// A trailing '\n' already closed the last line; only add a boundary for a
	// final line that has no terminator.
	if len(content) > 0 && content[len(content)-1] != '\n' {
		offsets = append(offsets, len(content))
	}
	return offsets
}
