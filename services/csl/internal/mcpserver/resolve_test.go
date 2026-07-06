package mcpserver

import "testing"

func TestInsensitiveRepoFilter(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"mad01/csl", "(?i)mad01/csl"},
		{"(?i)already", "(?i)(?i)already"},
	}
	for _, tt := range tests {
		if got := insensitiveRepoFilter(tt.in); got != tt.want {
			t.Errorf("insensitiveRepoFilter(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
