package repopath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupRepo builds a repo root beside an outside directory holding a secret,
// with links from inside the repo to both.
func setupRepo(t *testing.T) (root string) {
	t.Helper()
	tmp := t.TempDir()
	root = filepath.Join(tmp, "repo")
	outsideDir := filepath.Join(tmp, "outside")
	for _, d := range []string{filepath.Join(root, "sub"), outsideDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(root, "main.go"):         "package main\n",
		filepath.Join(root, "sub", "inner.go"): "package sub\n",
		filepath.Join(outsideDir, "secret"):    "secret\n",
	}
	for p, body := range files {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		"escape-file": filepath.Join(outsideDir, "secret"),
		"escape-dir":  outsideDir,
		"escape-rel":  filepath.Join("..", "outside", "secret"),
		"inner-link":  filepath.Join("sub", "inner.go"),
		"sub-link":    "sub",
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestResolve(t *testing.T) {
	root := setupRepo(t)

	tests := []struct {
		name    string
		rel     string
		wantErr bool
	}{
		{name: "normal file", rel: "main.go"},
		{name: "nested file", rel: "sub/inner.go"},
		{name: "missing file", rel: "nope.go"},
		{name: "missing file in missing dir", rel: "a/b/nope.go"},
		{name: "repo root", rel: "."},
		{name: "empty means root", rel: ""},
		{name: "dot-dot that stays inside", rel: "sub/../main.go"},
		{name: "symlink to a file inside", rel: "inner-link"},
		{name: "file under a symlinked dir inside", rel: "sub-link/inner.go"},
		{name: "parent escape", rel: "../outside/secret", wantErr: true},
		{name: "deep parent escape", rel: "sub/../../outside/secret", wantErr: true},
		{name: "bare dot-dot", rel: "..", wantErr: true},
		{name: "absolute path", rel: "/etc/hosts", wantErr: true},
		{name: "symlink to a file outside", rel: "escape-file", wantErr: true},
		{name: "relative symlink out", rel: "escape-rel", wantErr: true},
		{name: "file under a symlinked dir outside", rel: "escape-dir/secret", wantErr: true},
		{name: "missing file under a symlinked dir outside", rel: "escape-dir/nope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(root, "file", tt.rel)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Resolve(%q) = %q, want an error", tt.rel, got)
				}
				if !strings.Contains(err.Error(), "file must stay inside the repo") {
					t.Errorf("error %q does not name the parameter and the rule", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(%q): %v", tt.rel, err)
			}
			if want := filepath.Join(root, tt.rel); got != want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.rel, got, want)
			}
		})
	}
}

// TestResolve_MissingFileStillNotFound pins the contract callers rely on: the
// guard passes a missing file, and opening the returned path reports
// not-found as it did before the guard existed.
func TestResolve_MissingFileStillNotFound(t *testing.T) {
	root := setupRepo(t)
	got, err := Resolve(root, "file", "nope.go")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := os.Open(got); !os.IsNotExist(err) {
		t.Errorf("open %s: err = %v, want not-exist", got, err)
	}
}

// TestResolve_RootBehindSymlink covers a checkout reached through a link: the
// real root is compared, so files inside still resolve.
func TestResolve_RootBehindSymlink(t *testing.T) {
	root := setupRepo(t)
	linkedRoot := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(root, linkedRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(linkedRoot, "file", "main.go"); err != nil {
		t.Errorf("Resolve through a linked root: %v", err)
	}
	if _, err := Resolve(linkedRoot, "file", "escape-file"); err == nil {
		t.Error("Resolve through a linked root let an escaping link through")
	}
}
