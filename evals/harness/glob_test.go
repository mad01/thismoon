package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTree writes files, slash path to content, under dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		glob, name string
		want       bool
	}{
		{"docs/*.md", "docs/a.md", true},
		{"docs/*.md", "docs/sub/a.md", false},
		{"**", "docs/a.md", true},
		{"**/*.md", "a.md", true},
		{"**/*.md", "docs/x/y.md", true},
		{"docs/**", "cmd/a.md", false},
		{"cmd/a.md", "cmd/a.md", true},
	}
	for _, tc := range tests {
		if got := matchGlob(tc.glob, tc.name); got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.glob, tc.name, got, tc.want)
		}
	}
}
