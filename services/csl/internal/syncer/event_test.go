package syncer

import (
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// TestSyncChanged: a run that found everything current posts no event; one
// that pulled, reindexed, pruned, or saw a pull fail does.
func TestSyncChanged(t *testing.T) {
	tests := []struct {
		name   string
		counts map[string]int
		report Report
		want   bool
	}{
		{"all current", map[string]int{"ok": 19, "dirty": 2, "skip": 1}, Report{}, false},
		{"pulled", map[string]int{"ok": 18, "updated": 1}, Report{}, true},
		{"pull failed", map[string]int{"ok": 18, "fail": 1}, Report{}, true},
		{
			"reindexed",
			map[string]int{"ok": 19},
			Report{Indexed: []finder.Repo{{Name: "a/b"}}},
			true,
		},
		{"pruned shards", map[string]int{"ok": 19}, Report{Pruned: 2}, true},
		{"pruned worktree index", map[string]int{"ok": 19}, Report{WorktreesPruned: 1}, true},
	}
	for _, tt := range tests {
		if got := syncChanged(tt.counts, &tt.report); got != tt.want {
			t.Errorf("%s: syncChanged = %v, want %v", tt.name, got, tt.want)
		}
	}
}
