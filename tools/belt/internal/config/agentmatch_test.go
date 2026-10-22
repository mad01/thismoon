package config

import (
	"slices"
	"testing"
)

// TestMatchAgentType pins the matcher ledger.agent_types and a custom
// hint's agent_types gate share. A pattern ending in * matches by prefix
// and any other only itself, case included. The exact pattern beats a
// glob and the longest glob beats a shorter one, in either pattern order.
func TestMatchAgentType(t *testing.T) {
	tests := []struct {
		name      string
		patterns  []string
		agentType string
		want      string // "" means no match
	}{
		{
			"a glob matches a slugged name",
			[]string{"reviewer-deep*"},
			"reviewer-deep-csl-prune", "reviewer-deep*",
		},
		{
			"a glob matches its bare prefix",
			[]string{"reviewer-deep*"},
			"reviewer-deep", "reviewer-deep*",
		},
		{
			"a plain key does not match a slugged name",
			[]string{"reviewer-deep"},
			"reviewer-deep-csl-prune", "",
		},
		{
			"exact beats a glob",
			[]string{"reviewer-deep*", "reviewer-deep-csl"},
			"reviewer-deep-csl", "reviewer-deep-csl",
		},
		{
			"the longest glob wins",
			[]string{"reviewer*", "reviewer-deep*", "*"},
			"reviewer-deep-csl-prune", "reviewer-deep*",
		},
		{
			"a glob of another tier does not match",
			[]string{"reviewer-quick*"},
			"reviewer-deep-csl", "",
		},
		{
			"matching is case-sensitive",
			[]string{"reviewer-deep*"},
			"Reviewer-Deep-csl", "",
		},
		{"a * before the end is no glob", []string{"reviewer-*-deep"}, "reviewer-deep", ""},
		{"a bare * matches any type", []string{"*"}, "Explore", "*"},
		{"an empty agent type matches nothing", []string{"*", ""}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backwards := slices.Clone(tt.patterns)
			slices.Reverse(backwards)
			for _, patterns := range [][]string{tt.patterns, backwards} {
				got, ok := MatchAgentType(slices.Values(patterns), tt.agentType)
				if got != tt.want || ok != (tt.want != "") {
					t.Errorf("MatchAgentType(%q, %q) = %q, %v; want %q",
						patterns, tt.agentType, got, ok, tt.want)
				}
			}
		})
	}
}
