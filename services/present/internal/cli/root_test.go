package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	cases := []struct {
		in   string
		want string
	}{
		{"~", home},
		{"~/.config/present", filepath.Join(home, ".config", "present")},
		{"/abs/path", "/abs/path"},
		{"relative/path", "relative/path"},
		{"", ""},
		{"~notme/x", "~notme/x"}, // only ~ and ~/ expand, not ~user
	}
	for _, c := range cases {
		if got := expandTilde(c.in); got != c.want {
			t.Errorf("expandTilde(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExpandTildeNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	cases := []struct {
		in   string
		want string
	}{
		{"~", "."},
		{"~/.config/present", ".config/present"},
		{"/abs/path", "/abs/path"},
	}
	for _, c := range cases {
		if got := expandTilde(c.in); got != c.want {
			t.Errorf("expandTilde(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
