package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeFile is a test helper that creates a file (and parent dirs) with content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanDir_Monorepo(t *testing.T) {
	root := t.TempDir()
	// System at the repo root.
	writeFile(t, filepath.Join(root, "service-info.yaml"), `
kind: System
metadata:
  name: dotfiles
spec:
  owner: mad01
`)
	// Components in subdirs.
	writeFile(t, filepath.Join(root, "present", "service-info.yaml"), `
kind: Component
metadata:
  name: present
spec:
  type: cli
  owner: mad01
  system: dotfiles
`)
	writeFile(t, filepath.Join(root, "abacus", "service-info.yaml"), `
kind: Component
metadata:
  name: abacus
spec:
  type: cli
  owner: mad01
  system: dotfiles
`)
	// A service-info.yaml inside .git must be ignored.
	writeFile(t, filepath.Join(root, ".git", "service-info.yaml"), `bogus: true`)

	got, err := ScanDir(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entities, want 3: %+v", len(got), got)
	}
	// SourcePath must be populated.
	for _, e := range got {
		if e.SourcePath == "" {
			t.Errorf("entity %s has empty SourcePath", e.Metadata.Name)
		}
	}
}

func TestScanDir_SingleRepo(t *testing.T) {
	root := t.TempDir()
	// One file holding both a System and its Component.
	writeFile(t, filepath.Join(root, "service-info.yaml"), `
kind: System
metadata:
  name: code-search-local
spec:
  owner: mad01
---
kind: Component
metadata:
  name: csl
spec:
  type: cli
  owner: mad01
  system: code-search-local
`)
	got, err := ScanDir(context.Background(), root)
	if err != nil {
		t.Fatalf("ScanDir: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entities, want 2", len(got))
	}
}

func TestScanPaths_SkipsMissing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "service-info.yaml"), `
kind: System
metadata:
  name: a
spec:
  owner: mad01
`)
	got, err := ScanPaths(context.Background(), []string{root, filepath.Join(root, "does-not-exist")})
	if err != nil {
		t.Fatalf("ScanPaths: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entities, want 1", len(got))
	}
}

func TestScanDir_PropagatesParseError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "service-info.yaml"), `
kind: Component
metadata:
  name: orphan
spec:
  owner: mad01
`) // missing system -> validation error
	_, err := ScanDir(context.Background(), root)
	if err == nil {
		t.Fatal("expected error from invalid component, got nil")
	}
}

func TestParseRegistry(t *testing.T) {
	in := `
sources:
  - path: ~/code/src/github.com/mad01/dotfiles
  - path: /abs/path/repo
`
	r, err := ParseRegistry([]byte(in))
	if err != nil {
		t.Fatalf("ParseRegistry: %v", err)
	}
	if len(r.Sources) != 2 {
		t.Fatalf("got %d sources, want 2", len(r.Sources))
	}
	paths := r.Paths()
	if filepath.IsAbs(paths[0]) == false {
		t.Errorf("path[0] %q should be expanded to absolute", paths[0])
	}
	if paths[1] != "/abs/path/repo" {
		t.Errorf("path[1] = %q, want /abs/path/repo", paths[1])
	}
}
