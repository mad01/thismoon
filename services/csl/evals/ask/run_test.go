package main

import "testing"

func TestServedMatches(t *testing.T) {
	for _, tc := range []struct {
		asked, served string
		want          bool
	}{
		{"claude-fable-5-1", "claude-fable-5-1", true},
		{"claude-fable-5-1", "claude-fable-5-1[1m]", true},
		{"claude-haiku-5-5", "claude-haiku-5-5-20260401", true},
		{"claude-haiku-5-5", "anthropic/claude-haiku-5.5", true},
		{"claude-haiku-5-5", "anthropic/claude-haiku-5.5:floor", true},
		{"claude-fable-5-1", "claude-fable-5", false},
		{"claude-haiku-5-5", "claude-sonnet-5-5", false},
	} {
		if got := servedMatches(tc.asked, tc.served); got != tc.want {
			t.Errorf("servedMatches(%q, %q) = %v, want %v", tc.asked, tc.served, got, tc.want)
		}
	}
}
