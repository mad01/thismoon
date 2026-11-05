package search

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInvalidQueryIsSentinel(t *testing.T) {
	_, indexDir := indexFixture(t)
	ctx := context.Background()
	_, err := Search(ctx, indexDir, SearchOptions{Pattern: "foo("})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Errorf("Search err = %v, want ErrInvalidQuery", err)
	}
	_, _, err = Count(ctx, indexDir, CountOptions{Pattern: "foo("})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Errorf("Count err = %v, want ErrInvalidQuery", err)
	}
}

func TestNewResult(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	note := "---\nevidence: B\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "a.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	matches := []Match{
		{Path: "notes/a.md", Line: 1, Column: 1, Text: "x"},
		{Path: "notes/a.md", Line: 2, Column: 1, Text: "y"},
		{Path: "notes/missing.md", Line: 1, Column: 1, Text: "z"},
	}
	tests := []struct {
		mode      string
		wantTotal int
	}{
		{ModeFiles, 2},
		{ModeContent, 3},
	}
	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			res := NewResult(root, tc.mode, matches)
			if res.OutputMode != tc.mode || res.Total != tc.wantTotal {
				t.Fatalf("result = %+v", res)
			}
			first := ""
			if tc.mode == ModeFiles {
				first = res.Files[0].Evidence
			} else {
				first = res.Lines[0].Evidence
			}
			if first != "B" {
				t.Errorf("evidence = %q, want B", first)
			}
		})
	}
}
