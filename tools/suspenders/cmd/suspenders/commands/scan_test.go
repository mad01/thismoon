package commands

import (
	"testing"

	"github.com/mad01/thismoon/tools/suspenders/internal/scanner"
)

// TestApplyExcludeRules covers the rule-filter shared by `scan` and the
// pre-commit hook: excluded IDs are dropped, others kept in order, and an
// empty exclude list is a no-op. IDs are arbitrary labels, not secrets.
func TestApplyExcludeRules(t *testing.T) {
	rules := []scanner.Rule{
		{ID: "alpha"},
		{ID: "beta"},
		{ID: "gamma"},
	}

	tests := []struct {
		name    string
		exclude []string
		want    []string
	}{
		{name: "no excludes keeps all", exclude: nil, want: []string{"alpha", "beta", "gamma"}},
		{name: "drops one", exclude: []string{"beta"}, want: []string{"alpha", "gamma"}},
		{name: "drops several", exclude: []string{"alpha", "gamma"}, want: []string{"beta"}},
		{
			name:    "unknown id is ignored",
			exclude: []string{"missing"},
			want:    []string{"alpha", "beta", "gamma"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Copy so the in-place filter never mutates the shared fixture.
			in := append([]scanner.Rule{}, rules...)
			got := applyExcludeRules(in, tt.exclude)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d rules, want %d (%v)", len(got), len(tt.want), tt.want)
			}
			for i, id := range tt.want {
				if got[i].ID != id {
					t.Errorf("rule %d: got %q, want %q", i, got[i].ID, id)
				}
			}
		})
	}
}
