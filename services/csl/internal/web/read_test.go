package web

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
)

// newReadTestService builds a Service over a temp workspace holding one fake
// repo (a .git dir with an origin remote) containing a file with lineCount
// numbered lines.
func newReadTestService(t *testing.T, fileName string, lineCount int) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	repoDir := filepath.Join(root, "org", "testrepo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitCfg := "[remote \"origin\"]\n\turl = git@github.com:org/testrepo.git\n"
	if err := os.WriteFile(filepath.Join(repoDir, ".git", "config"), []byte(gitCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := range lineCount {
		fmt.Fprintf(&b, "line %d\n", i+1)
	}
	if err := os.WriteFile(filepath.Join(repoDir, fileName), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(&config.Config{Dirs: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	return svc, repoDir
}

func TestReadFileMetadata(t *testing.T) {
	svc, repoDir := newReadTestService(t, "main.go", 10)

	res, err := svc.ReadFile("testrepo", "main.go", 3, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Lines) != 3 || res.Lines[0].Number != 3 {
		t.Errorf("lines = %+v, want lines 3-5", res.Lines)
	}
	if res.TotalLines != 10 {
		t.Errorf("TotalLines = %d, want 10 (counted past the range)", res.TotalLines)
	}
	if res.Truncated {
		t.Errorf("Truncated = true, want false for a small range")
	}
	wantPath := filepath.Join(repoDir, "main.go")
	if home, err := os.UserHomeDir(); err == nil {
		wantPath = collapseHome(wantPath, home)
	}
	if res.LocalPath != wantPath {
		t.Errorf("LocalPath = %q, want %q", res.LocalPath, wantPath)
	}
	wantURL := "https://github.com/org/testrepo/blob/HEAD/main.go"
	if res.FileURL != wantURL {
		t.Errorf("FileURL = %q, want %q", res.FileURL, wantURL)
	}
}

func TestReadFileExactNameBeatsSubstring(t *testing.T) {
	svc, _ := newReadTestService(t, "main.go", 3)
	// A sibling repo whose name contains the exact name as a substring, with a
	// conflicting file, must not shadow the exact match.
	root := svc.cfg.Dirs[0]
	other := filepath.Join(root, "org", "testrepo-arcade")
	if err := os.MkdirAll(filepath.Join(other, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitCfg := "[remote \"origin\"]\n\turl = git@github.com:org/testrepo-arcade.git\n"
	if err := os.WriteFile(filepath.Join(other, ".git", "config"), []byte(gitCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := svc.ReadFile("org/testrepo", "main.go", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Repo != "org/testrepo" {
		t.Errorf("resolved repo = %q, want the exact match org/testrepo", res.Repo)
	}
}

func TestReadFileWholeFileCapped(t *testing.T) {
	svc, _ := newReadTestService(t, "big.txt", maxReadLines+50)

	res, err := svc.ReadFile("testrepo", "big.txt", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Lines) != maxReadLines {
		t.Errorf("len(lines) = %d, want capped at %d", len(res.Lines), maxReadLines)
	}
	if !res.Truncated {
		t.Errorf("Truncated = false, want true when the cap was hit")
	}
	if res.TotalLines != maxReadLines+50 {
		t.Errorf("TotalLines = %d, want %d", res.TotalLines, maxReadLines+50)
	}
}
