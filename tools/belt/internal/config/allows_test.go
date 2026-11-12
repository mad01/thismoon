package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestAllowsLoadAndResolve pins the allows section: off unless enabled,
// roots expanded and resolved physically, and an unusable root dropped
// with an adjustment instead of failing the file.
func TestAllowsLoadAndResolve(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv := t.TempDir()
	for _, dir := range []string{filepath.Join(home, "code", "src"), filepath.Join(srv, "repos")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := writeFile(t, t.TempDir(), "config.yaml", `
allows:
  git-c-read:
    enabled: true
    roots:
      - ~/code/src/
      - `+srv+`/repos/../repos
      - relative/dir
`)
	cfg, err := LoadFrom(Paths{BeltYAML: path})
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if !cfg.AllowEnabled("git-c-read") || cfg.AllowEnabled("unknown") {
		t.Errorf("AllowEnabled = %v / %v, want true for the set entry and false otherwise",
			cfg.AllowEnabled("git-c-read"), cfg.AllowEnabled("unknown"))
	}
	allow := cfg.Allows["git-c-read"]
	want := []string{
		realPath(t, filepath.Join(home, "code", "src")),
		realPath(t, filepath.Join(srv, "repos")),
	}
	if got := allow.RootDirs(); !slices.Equal(got, want) {
		t.Errorf("RootDirs = %v, want %v", got, want)
	}
	adj := allow.Adjustments("git-c-read")
	if len(adj) != 1 || !strings.Contains(adj[0], `"relative/dir"`) {
		t.Errorf("Adjustments = %v, want one naming relative/dir", adj)
	}
	if (Config{}).AllowEnabled("git-c-read") {
		t.Error("an allow is on with no config, want off")
	}
}

// TestAllowRootsDropHomeAndMissing: a root that is /, home or an ancestor
// of home would approve calls in every repo the user has, so it is dropped
// with a doctor note, and so is one that does not resolve (review finding
// 6). A root is resolved physically, like a git -C directory, so .. after
// a symlink climbs from the link's target.
func TestAllowRootsDropHomeAndMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := filepath.Join(home, "code", "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(src, filepath.Join(home, "src-link")); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		entry string
		want  string // the root in effect, "" when dropped
		note  string // what the adjustment says, when dropped
	}{
		{"/", "", "covers the home directory"},
		{"~", "", "covers the home directory"},
		{"~/..", "", "covers the home directory"},
		{"~/code/../..", "", "covers the home directory"},
		{"~/missing", "", "does not resolve"},
		{"~/src-link/..", realPath(t, filepath.Join(home, "code")), ""},
		{"~/code/src", realPath(t, src), ""},
	}
	for _, tt := range tests {
		t.Run(tt.entry, func(t *testing.T) {
			a := Allow{Roots: []string{tt.entry}}
			got := a.RootDirs()
			adj := a.Adjustments("git-c-read")
			if tt.want == "" {
				if len(got) != 0 || len(adj) != 1 || !strings.Contains(adj[0], tt.note) {
					t.Errorf(
						"RootDirs = %v, Adjustments = %q; want it dropped with %q",
						got,
						adj,
						tt.note,
					)
				}
				return
			}
			if !slices.Equal(got, []string{tt.want}) || len(adj) != 0 {
				t.Errorf(
					"RootDirs = %v, Adjustments = %q; want [%s] and no note",
					got,
					adj,
					tt.want,
				)
			}
		})
	}
}

// realPath is path with every symlink resolved: temp dirs on macOS sit
// under /var, a symlink to /private/var.
func realPath(t *testing.T, path string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return real
}
