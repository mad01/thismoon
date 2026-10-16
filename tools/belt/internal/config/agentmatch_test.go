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
			[]string{"factory-worker-high*"},
			"factory-worker-high-csl-prune", "factory-worker-high*",
		},
		{
			"a glob matches its bare prefix",
			[]string{"factory-worker-high*"},
			"factory-worker-high", "factory-worker-high*",
		},
		{
			"a plain key does not match a slugged name",
			[]string{"factory-worker-high"},
			"factory-worker-high-csl-prune", "",
		},
		{
			"exact beats a glob",
			[]string{"factory-worker-high*", "factory-worker-high-csl"},
			"factory-worker-high-csl", "factory-worker-high-csl",
		},
		{
			"the longest glob wins",
			[]string{"factory-worker*", "factory-worker-high*", "*"},
			"factory-worker-high-csl-prune", "factory-worker-high*",
		},
		{
			"a glob of another tier does not match",
			[]string{"factory-worker-low*"},
			"factory-worker-high-csl", "",
		},
		{
			"matching is case-sensitive",
			[]string{"factory-worker-high*"},
			"Factory-Worker-High-csl", "",
		},
		{"a * before the end is no glob", []string{"factory-*-high"}, "factory-worker-high", ""},
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
