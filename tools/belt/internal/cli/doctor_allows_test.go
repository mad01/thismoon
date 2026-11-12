package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// TestDoctorListsAllows pins the allows section: an allow is listed as
// disabled by default, an enabled one names its roots and the worktree
// rule, and an ignored root, a root inside a git work tree, or an unknown
// allows key is a warning.
func TestDoctorListsAllows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := filepath.Join(home, "code", "src")
	outer := filepath.Join(home, "outer")
	for _, dir := range []string{src, filepath.Join(outer, ".git"), filepath.Join(outer, "plain")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	realSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		cfg       config.Config
		wantLines []string
		wantWarns []string
	}{
		{
			name: "off by default",
			cfg:  config.Config{},
			wantLines: []string{
				"git-c-read             bash    DISABLED (the default; set allows.git-c-read.enabled",
			},
		},
		{
			name: "enabled with roots",
			cfg: config.Config{Allows: map[string]config.Allow{
				"git-c-read": {
					Enabled: true,
					Roots:   []string{"~/code/src", "rel", "~", "~/outer/plain"},
				},
				"git-c-raed": {Enabled: true},
			}},
			wantLines: []string{
				"enabled  roots: the session's own work tree (unless it is home), " + realSrc + ", ",
				"a linked worktree needs its folder and its main checkout under roots",
			},
			wantWarns: []string{
				`allows.git-c-read.roots entry "rel" is not an absolute or ~/ path`,
				`allows.git-c-read.roots entry "~" covers the home directory`,
				"/outer/plain sits inside the git work tree at ",
				`unknown allow "git-c-raed"`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			warns := printAllows(&out, tt.cfg)
			for _, want := range tt.wantLines {
				if !strings.Contains(out.String(), want) {
					t.Errorf("section = %q, want %q", out.String(), want)
				}
			}
			if len(warns) != len(tt.wantWarns) {
				t.Fatalf("warns = %q, want %d", warns, len(tt.wantWarns))
			}
			for i, want := range tt.wantWarns {
				if !strings.Contains(warns[i], want) {
					t.Errorf("warns[%d] = %q, want %q", i, warns[i], want)
				}
			}
		})
	}
}
