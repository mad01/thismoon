package cslignore

import "testing"

// TestSkipDir pins the directory rule csl's walk and belt's large-read guard
// share: the generated trees and every hidden directory outside the
// allowlist are left out, .git always, and a nil allowlist leaves out every
// hidden directory.
func TestSkipDir(t *testing.T) {
	allow := HiddenAllowlist([]string{".circleci", ".git"})
	tests := []struct {
		base  string
		allow map[string]bool
		want  bool
	}{
		{"node_modules", allow, true},
		{"vendor", allow, true},
		{"__pycache__", allow, true},
		{"build", allow, true},
		{"dist", allow, true},
		{"target", allow, true},
		{".cache", allow, true},
		{".git", allow, true},
		{".github", allow, false},
		{".claude", allow, false},
		{".circleci", allow, false},
		{"internal", allow, false},
		{"vendored", allow, false},
		{".github", nil, true},
		{"internal", nil, false},
	}
	for _, tt := range tests {
		if got := SkipDir(tt.base, tt.allow); got != tt.want {
			t.Errorf("SkipDir(%q, %v) = %v, want %v", tt.base, tt.allow, got, tt.want)
		}
	}
}
