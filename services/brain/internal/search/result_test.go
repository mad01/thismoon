package search

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
		{Vault: "v", Path: "notes/a.md", Line: 1, Column: 1, Text: "x"},
		{Vault: "v", Path: "notes/a.md", Line: 2, Column: 1, Text: "y"},
		{Vault: "v", Path: "notes/missing.md", Line: 1, Column: 1, Text: "z"},
		{Vault: "w", Path: "notes/a.md", Line: 1, Column: 1, Text: "x"},
	}
	tests := []struct {
		mode      string
		wantTotal int
	}{
		{ModeFiles, 3},
		{ModeContent, 4},
	}
	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			res := NewResult(map[string]string{"v": root}, tc.mode, matches)
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

func TestMergeMatches(t *testing.T) {
	personal := []Match{
		{Vault: "personal", Path: "a.md", Score: 9, Line: 1},
		{Vault: "personal", Path: "a.md", Score: 9, Line: 2},
		{Vault: "personal", Path: "b.md", Score: 3, Line: 1},
	}
	team := []Match{
		{Vault: "team", Path: "a.md", Score: 5, Line: 1},
		{Vault: "team", Path: "c.md", Score: 3, Line: 1},
	}
	tests := []struct {
		name  string
		limit int
		want  []string
	}{
		{"all, by score, ties in list order", 0, []string{
			"personal:a.md:1", "personal:a.md:2", "team:a.md:1", "personal:b.md:1", "team:c.md:1",
		}},
		{"limit counts notes, not lines", 2, []string{
			"personal:a.md:1", "personal:a.md:2", "team:a.md:1",
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, m := range MergeMatches([][]Match{personal, team}, tc.limit) {
				got = append(got, fmt.Sprintf("%s:%s:%d", m.Vault, m.Path, m.Line))
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("merged = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMergeCounts(t *testing.T) {
	got := MergeCounts(
		[]CountResult{{Group: "notes", Count: 2}, {Group: "inbox", Count: 1}},
		[]CountResult{{Group: "inbox", Count: 4}},
	)
	want := []CountResult{{Group: "inbox", Count: 5}, {Group: "notes", Count: 2}}
	if !slices.Equal(got, want) {
		t.Errorf("MergeCounts = %+v, want %+v", got, want)
	}
}
