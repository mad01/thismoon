package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupLsTestRepo builds a repo with a small tree:
//
//	main.go
//	docs/readme.md
//	internal/a.go
//	internal/b.txt
//	internal/deep/c.go
func setupLsTestRepo(t *testing.T) func() {
	t.Helper()
	cleanup := setupReadTestRepo(t, "main.go", 3)
	repoDir := filepath.Join(os.Getenv("HOME"), "workspace", "org", "testrepo")
	for _, f := range []string{"docs/readme.md", "internal/a.go", "internal/b.txt", "internal/deep/c.go"} {
		p := filepath.Join(repoDir, f)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("x\n"), 0o644)
	}
	return cleanup
}

func TestHandleLs_SingleLevelRoot(t *testing.T) {
	cleanup := setupLsTestRepo(t)
	defer cleanup()

	_, out, err := handleLs(context.Background(), nil, lsInput{Repo: "testrepo"})
	if err != nil {
		t.Fatalf("handleLs: %v", err)
	}
	// dirs sort first: docs, internal, then main.go — .git excluded
	want := []string{"docs", "internal", "main.go"}
	if len(out.Entries) != len(want) {
		t.Fatalf("got %d entries %v, want %v", len(out.Entries), out.Entries, want)
	}
	for i, w := range want {
		if out.Entries[i].Path != w {
			t.Errorf("entry[%d] = %q, want %q", i, out.Entries[i].Path, w)
		}
	}
	if !out.Entries[0].Dir || out.Entries[2].Dir {
		t.Error("expected docs to be a dir and main.go a file")
	}
}

func TestHandleLs_SubdirAndGlob(t *testing.T) {
	cleanup := setupLsTestRepo(t)
	defer cleanup()

	_, out, err := handleLs(context.Background(), nil, lsInput{
		Repo: "testrepo",
		Path: "internal",
		Glob: "*.go",
	})
	if err != nil {
		t.Fatalf("handleLs: %v", err)
	}
	// one level only: a.go matches, b.txt filtered, deep/ is a dir (name doesn't match *.go)
	if len(out.Entries) != 1 || out.Entries[0].Path != filepath.Join("internal", "a.go") {
		t.Errorf("got %v, want just internal/a.go", out.Entries)
	}
}

func TestHandleLs_RecursiveFilesOnly(t *testing.T) {
	cleanup := setupLsTestRepo(t)
	defer cleanup()

	_, out, err := handleLs(context.Background(), nil, lsInput{
		Repo:      "testrepo",
		Recursive: true,
		Glob:      "*.go",
	})
	if err != nil {
		t.Fatalf("handleLs: %v", err)
	}
	want := map[string]bool{
		"main.go":                                 true,
		filepath.Join("internal", "a.go"):         true,
		filepath.Join("internal", "deep", "c.go"): true,
	}
	if len(out.Entries) != len(want) {
		t.Fatalf("got %d entries %v, want %d", len(out.Entries), out.Entries, len(want))
	}
	for _, e := range out.Entries {
		if !want[e.Path] {
			t.Errorf("unexpected entry %q", e.Path)
		}
		if e.Dir {
			t.Errorf("recursive listing returned a dir: %q", e.Path)
		}
	}
}

func TestHandleLs_RejectsEscapingPath(t *testing.T) {
	cleanup := setupLsTestRepo(t)
	defer cleanup()

	for _, p := range []string{"..", "../other", "/etc", "internal/../.."} {
		_, _, err := handleLs(context.Background(), nil, lsInput{Repo: "testrepo", Path: p})
		if err == nil {
			t.Errorf("path %q: expected error, got nil", p)
		}
	}
}

func TestHandleLs_TruncatesAtCap(t *testing.T) {
	cleanup := setupLsTestRepo(t)
	defer cleanup()

	repoDir := filepath.Join(os.Getenv("HOME"), "workspace", "org", "testrepo")
	bulk := filepath.Join(repoDir, "bulk")
	_ = os.MkdirAll(bulk, 0o755)
	for i := range maxLsEntries + 10 {
		_ = os.WriteFile(filepath.Join(bulk, fmt.Sprintf("f%04d.txt", i)), []byte("x"), 0o644)
	}

	_, out, err := handleLs(context.Background(), nil, lsInput{Repo: "testrepo", Path: "bulk"})
	if err != nil {
		t.Fatalf("handleLs: %v", err)
	}
	if !out.Truncated {
		t.Fatal("expected truncated=true")
	}
	if out.Total != maxLsEntries {
		t.Errorf("total = %d, want %d", out.Total, maxLsEntries)
	}
	if out.TotalAvailable != maxLsEntries+10 {
		t.Errorf("total_available = %d, want %d", out.TotalAvailable, maxLsEntries+10)
	}
}

func TestHandleIndexInfo_EmptyStateNoError(t *testing.T) {
	cleanup := setupLsTestRepo(t)
	defer cleanup()

	_, out, err := handleIndexInfo(context.Background(), nil, indexInfoInput{})
	if err != nil {
		t.Fatalf("handleIndexInfo: %v", err)
	}
	// Fresh HOME: nothing indexed, no daemon, no semantic index.
	if out.ReposIndexed != 0 || out.Shards != 0 || out.Semantic.Built {
		t.Errorf("expected empty index info, got %+v", out)
	}
	if strings.TrimSpace(out.NewestIndexedAt) != "" {
		t.Errorf("newest_indexed_at = %q, want empty", out.NewestIndexedAt)
	}
}
