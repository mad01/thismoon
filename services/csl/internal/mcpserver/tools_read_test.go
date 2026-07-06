package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupReadTestRepo(t *testing.T, fileName string, lineCount int) func() {
	t.Helper()
	tmp := t.TempDir()
	reposRoot := filepath.Join(tmp, "workspace")
	repoDir := filepath.Join(reposRoot, "org", "testrepo")
	_ = os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755)
	cfg := "[remote \"origin\"]\n\turl = git@github.com:org/testrepo.git\n"
	_ = os.WriteFile(filepath.Join(repoDir, ".git", "config"), []byte(cfg), 0o644)

	var lines []string
	for i := range lineCount {
		lines = append(lines, fmt.Sprintf("line %d content", i+1))
	}
	_ = os.WriteFile(
		filepath.Join(repoDir, fileName),
		[]byte(strings.Join(lines, "\n")+"\n"),
		0o644,
	)

	cfgDir := filepath.Join(tmp, ".config", "csl")
	_ = os.MkdirAll(cfgDir, 0o755)
	cslCfg := "dirs:\n  - " + reposRoot + "\n"
	_ = os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cslCfg), 0o644)

	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmp)
	return func() { _ = os.Setenv("HOME", origHome) }
}

func TestHandleRead_RepoMatchIsCaseInsensitiveRegex(t *testing.T) {
	cleanup := setupReadTestRepo(t, "small.go", 5)
	defer cleanup()

	_, out, err := handleRead(context.Background(), nil, readInput{
		Repo: "TESTREPO$",
		File: "small.go",
	})
	if err != nil {
		t.Fatalf("handleRead with uppercase regex: %v", err)
	}
	if out.Repo != "org/testrepo" {
		t.Errorf("resolved repo = %q, want org/testrepo", out.Repo)
	}
}

func TestHandleRead_AmbiguousRepoErrors(t *testing.T) {
	cleanup := setupReadTestRepo(t, "small.go", 5)
	defer cleanup()

	// Add a second repo that also matches "testrepo".
	reposRoot := filepath.Join(os.Getenv("HOME"), "workspace")
	otserviceir := filepath.Join(reposRoot, "org", "testrepo-two")
	_ = os.MkdirAll(filepath.Join(otserviceir, ".git"), 0o755)
	cfg := "[remote \"origin\"]\n\turl = git@github.com:org/testrepo-two.git\n"
	_ = os.WriteFile(filepath.Join(otserviceir, ".git", "config"), []byte(cfg), 0o644)

	_, _, err := handleRead(context.Background(), nil, readInput{
		Repo: "testrepo",
		File: "small.go",
	})
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want it to mention ambiguity", err)
	}

	// The exact org/repo name still disambiguates.
	_, out, err := handleRead(context.Background(), nil, readInput{
		Repo: "org/testrepo",
		File: "small.go",
	})
	if err != nil {
		t.Fatalf("handleRead with exact name: %v", err)
	}
	if out.Repo != "org/testrepo" {
		t.Errorf("resolved repo = %q, want org/testrepo", out.Repo)
	}
}

func TestHandleRead_SmallFileNotTruncated(t *testing.T) {
	cleanup := setupReadTestRepo(t, "small.go", 50)
	defer cleanup()

	_, out, err := handleRead(context.Background(), nil, readInput{
		Repo: "testrepo",
		File: "small.go",
	})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if out.Truncated {
		t.Error("expected truncated=false for 50-line file")
	}
	if out.TotalLines != 50 {
		t.Errorf("expected total_lines=50, got %d", out.TotalLines)
	}
	if len(out.Lines) != 50 {
		t.Errorf("expected 50 lines returned, got %d", len(out.Lines))
	}
}

func TestHandleRead_LargeFileTruncatedByDefault(t *testing.T) {
	cleanup := setupReadTestRepo(t, "large.go", 1000)
	defer cleanup()

	_, out, err := handleRead(context.Background(), nil, readInput{
		Repo: "testrepo",
		File: "large.go",
	})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if !out.Truncated {
		t.Error("expected truncated=true for 1000-line file with no range")
	}
	if out.TotalLines != 1000 {
		t.Errorf("expected total_lines=1000, got %d", out.TotalLines)
	}
	if len(out.Lines) != 500 {
		t.Errorf("expected 500 lines returned (default cap), got %d", len(out.Lines))
	}
}

func TestHandleRead_ExplicitRangeBypassesCap(t *testing.T) {
	cleanup := setupReadTestRepo(t, "big.go", 1000)
	defer cleanup()

	_, out, err := handleRead(context.Background(), nil, readInput{
		Repo:      "testrepo",
		File:      "big.go",
		StartLine: 1,
		EndLine:   800,
	})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if out.Truncated {
		t.Error("expected truncated=false when explicit range is provided")
	}
	if len(out.Lines) != 800 {
		t.Errorf("expected 800 lines returned, got %d", len(out.Lines))
	}
	if out.TotalLines != 1000 {
		t.Errorf("expected total_lines=1000, got %d", out.TotalLines)
	}
}

func TestHandleRead_StartLineOnlyNoTruncation(t *testing.T) {
	cleanup := setupReadTestRepo(t, "medium.go", 200)
	defer cleanup()

	_, out, err := handleRead(context.Background(), nil, readInput{
		Repo:      "testrepo",
		File:      "medium.go",
		StartLine: 100,
	})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	// start_line=100 with no end_line is NOT unbounded — it has a start constraint
	// The file is only 200 lines so 100-200 = 101 lines, well under cap.
	if out.Truncated {
		t.Error("expected truncated=false for partial file within cap")
	}
	if len(out.Lines) != 101 {
		t.Errorf("expected 101 lines (100-200 inclusive), got %d", len(out.Lines))
	}
}

func TestHandleRead_ExactlyAtCapNotTruncated(t *testing.T) {
	cleanup := setupReadTestRepo(t, "exact.go", 500)
	defer cleanup()

	_, out, err := handleRead(context.Background(), nil, readInput{
		Repo: "testrepo",
		File: "exact.go",
	})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if out.Truncated {
		t.Error("expected truncated=false for file exactly at cap (500 lines)")
	}
	if len(out.Lines) != 500 {
		t.Errorf("expected 500 lines, got %d", len(out.Lines))
	}
}

func TestHandleRead_LineNumbersCorrect(t *testing.T) {
	cleanup := setupReadTestRepo(t, "numbered.go", 100)
	defer cleanup()

	_, out, err := handleRead(context.Background(), nil, readInput{
		Repo:      "testrepo",
		File:      "numbered.go",
		StartLine: 50,
		EndLine:   55,
	})
	if err != nil {
		t.Fatalf("handleRead: %v", err)
	}
	if len(out.Lines) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(out.Lines))
	}
	if out.Lines[0].Number != 50 {
		t.Errorf("first line number = %d, want 50", out.Lines[0].Number)
	}
	if out.Lines[5].Number != 55 {
		t.Errorf("last line number = %d, want 55", out.Lines[5].Number)
	}
}
