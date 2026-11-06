package search

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// statusNotes all mention walrus. The superseded note ranks first, so with
// a limit of one it would take the only slot if the filter ran after the
// limit, and the search would come back empty.
var statusNotes = map[string]string{
	"notes/kept.md":       "---\ntitle: Kept\nstatus: validated\n---\nwalrus\n",
	"notes/old.md":        "---\ntitle: Old\nstatus: superseded\nsuperseded_by: id:01J\n---\nwalrus walrus walrus walrus\n",
	"notes/wrong.md":      "---\ntitle: Wrong\nstatus: retracted\n---\nwalrus walrus walrus walrus\n",
	"notes/gone.md":       "---\nstatus: withdrawn\n---\n",
	"notes/challenged.md": "---\ntitle: Challenged\nstatus: disputed\n---\nwalrus walrus\n",
}

func TestSearchHidesRevokedNotes(t *testing.T) {
	root := t.TempDir()
	for rel, content := range statusNotes {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	indexDir := t.TempDir()
	if err := IndexVault(Target{Name: "v", Root: root, IndexDir: indexDir}, nil); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		opts SearchOptions
		want []string
	}{
		{
			"default hides revoked",
			SearchOptions{Pattern: "walrus"},
			[]string{"notes/challenged.md", "notes/kept.md"},
		},
		{
			"hidden notes take no slot",
			SearchOptions{Pattern: "walrus", Limit: 1},
			[]string{"notes/kept.md"},
		},
		{
			"include revoked",
			SearchOptions{Pattern: "walrus", IncludeRevoked: true},
			[]string{"notes/challenged.md", "notes/kept.md", "notes/old.md", "notes/wrong.md"},
		},
		{
			"status filter names a revoked status",
			SearchOptions{Pattern: "walrus", Status: "retracted"},
			[]string{"notes/wrong.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := Search(context.Background(), indexDir, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range matches {
				if !slices.Contains(got, m.Path) {
					got = append(got, m.Path)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("paths = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestStatusFilterUsesTheTierDefault pins that a note with no status line
// matches the default status of its tier: validated in notes, raw in inbox.
func TestStatusFilterUsesTheTierDefault(t *testing.T) {
	root := t.TempDir()
	notes := map[string]string{
		"notes/bare.md":      "---\ntitle: Bare\n---\nnarwhal\n",
		"notes/explicit.md":  "---\ntitle: Explicit\nstatus: Validated\n---\nnarwhal\n",
		"notes/disputed.md":  "---\ntitle: Disputed\nstatus: disputed\n---\nnarwhal\n",
		"inbox/capture.md":   "---\ntitle: Capture\n---\nnarwhal\n",
		"inbox/promoted.md":  "---\ntitle: Odd\nstatus: validated\n---\nnarwhal\n",
		"outputs/report.md":  "---\ntitle: Report\n---\nnarwhal\n",
		"elsewhere/loose.md": "---\ntitle: Loose\n---\nnarwhal\n",
	}
	for rel, content := range notes {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	indexDir := t.TempDir()
	if err := IndexVault(Target{Name: "v", Root: root, IndexDir: indexDir}, nil); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		status string
		want   []string
	}{
		{"validated", []string{
			"inbox/promoted.md", "notes/bare.md", "notes/explicit.md", "outputs/report.md",
		}},
		{"raw", []string{"inbox/capture.md"}},
		{"RAW", []string{"inbox/capture.md"}},
		{"disputed", []string{"notes/disputed.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			matches, err := Search(context.Background(), indexDir,
				SearchOptions{Pattern: "narwhal", Status: tt.status})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range matches {
				if !slices.Contains(got, m.Path) {
					got = append(got, m.Path)
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Errorf("paths = %v, want %v", got, tt.want)
			}
		})
	}
}
