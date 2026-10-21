package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/tools/agent-sync/internal/pypath"
)

// The wants below are what Python 3.13 returned for the same input.

func TestPySplitLinesAndFields(t *testing.T) {
	lines := pySplitLines("a\nb\vc\x1cd\U00002028e\n")
	want := []string{"a", "b", "c", "d", "e"}
	if len(lines) != len(want) {
		t.Fatalf("pySplitLines = %q, want %q", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("pySplitLines[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
	if got := pyStrip("\x1f x \U00003000"); got != "x" {
		t.Errorf("pyStrip = %q, want %q", got, "x")
	}
	if got := pyText([]byte("a\r\nb\rc")); got != "a\nb\nc" {
		t.Errorf("pyText = %q", got)
	}
}

func TestMkdirParentsAndOSError(t *testing.T) {
	dir := t.TempDir()
	if err := mkdirParents(filepath.Join(dir, "a/b/c")); err != nil {
		t.Fatal(err)
	}
	if err := mkdirParents(filepath.Join(dir, "a/b")); err != nil {
		t.Errorf("mkdirParents of an existing directory = %v, want nil", err)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := mkdirParents(filepath.Join(file, "sub"))
	want := "[Errno 20] Not a directory: '" + filepath.Join(file, "sub") + "'"
	if err == nil || pypath.ErrorText(err) != want {
		t.Errorf("mkdirParents under a file = %v (%s), want %s", err, pypath.ErrorText(err), want)
	}
	err = mkdirParents(file)
	want = "[Errno 17] File exists: '" + file + "'"
	if err == nil || pypath.ErrorText(err) != want {
		t.Errorf("mkdirParents of a file = %v, want %s", err, want)
	}
}
