package cache

import (
	"path/filepath"
	"testing"
)

func TestSessionFile(t *testing.T) {
	tests := []struct {
		name      string
		dir       string
		sessionID string
		want      string
	}{
		{"uuid session id", "/c", "00893aaf-19fa-41d2", "/c/seen-00893aaf-19fa-41d2"},
		{"path traversal is stripped", "/c", "../../etc/passwd", "/c/seen-etcpasswd"},
		{"nothing safe survives", "/c", "../..", ""},
		{"empty session id", "/c", "", ""},
		{"no cache dir", "", "s1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SessionFile(tt.dir, "seen-", tt.sessionID); got != tt.want {
				t.Errorf(
					"SessionFile(%q, seen-, %q) = %q, want %q",
					tt.dir,
					tt.sessionID,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestDirSitsUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got, want := Dir(), filepath.Join(home, ".cache", "belt"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}
