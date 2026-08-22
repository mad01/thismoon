package cli

import (
	"testing"

	status "github.com/mad01/thismoon/services/status"
)

func TestResolvedDefaultWorkdir(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "unset falls back to default", env: "", want: status.DefaultWorkdir},
		{name: "env overrides default", env: "/tmp/custom", want: "/tmp/custom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("STATUS_WORKDIR", tt.env)
			if got := resolvedDefaultWorkdir(); got != tt.want {
				t.Errorf("resolvedDefaultWorkdir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExpandTilde(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "bare tilde", in: "~", want: "/home/test"},
		{name: "tilde slash", in: "~/foo", want: "/home/test/foo"},
		{name: "no tilde unchanged", in: "/abs/path", want: "/abs/path"},
		{name: "embedded tilde unchanged", in: "/x/~/y", want: "/x/~/y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandTilde(tt.in); got != tt.want {
				t.Errorf("expandTilde(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExpandTildeNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "bare tilde degrades to cwd", in: "~", want: "."},
		{name: "tilde slash strips prefix", in: "~/foo", want: "foo"},
		{name: "no tilde unchanged", in: "/abs/path", want: "/abs/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandTilde(tt.in); got != tt.want {
				t.Errorf("expandTilde(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
