package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sampleDir is the goscan package's own fixture tree, reused here so the
// MCP wrapper is exercised against the same extraction sites the goscan
// tests pin.
func sampleDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "goscan", "testdata", "sample")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("goscan testdata fixture missing: %v", err)
	}
	return dir
}

func TestScanGoPathError(t *testing.T) {
	t.Run("permission denial gets CLI guidance", func(t *testing.T) {
		stat := fmt.Errorf("stat /Users/x/other-repo: %w", fs.ErrPermission)
		got := scanGoPathError(stat, "/Users/x/other-repo")
		if !strings.Contains(got.Error(), "run the CLI instead") {
			t.Errorf("want guidance pointing at the CLI, got: %v", got)
		}
		if !strings.Contains(got.Error(), `"error":"/Users/x/other-repo is not readable`) {
			t.Errorf("want the path named in the machine-readable error field, got: %v", got)
		}
		if !errors.Is(got, fs.ErrPermission) {
			t.Errorf("original error should stay wrapped, got: %v", got)
		}
	})

	t.Run("other errors pass through unchanged", func(t *testing.T) {
		notExist := fmt.Errorf("stat /tmp/gone.go: %w", fs.ErrNotExist)
		if got := scanGoPathError(notExist, "/tmp/gone.go"); got != notExist {
			t.Errorf("want error unchanged, got: %v", got)
		}
	})
}

func TestHandleScanGoRequiresPath(t *testing.T) {
	_, _, err := handleScanGo(context.Background(), nil, scanGoInput{})
	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Errorf("want a path-required error, got: %v", err)
	}
}

func TestHandleScanGoMissingPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "absent.go")
	_, _, err := handleScanGo(context.Background(), nil, scanGoInput{Path: path})
	if err == nil {
		t.Fatal("want an error scanning a missing path")
	}
	if !strings.Contains(err.Error(), "absent.go") {
		t.Errorf("error %q does not name the path", err)
	}
}

func TestHandleScanGoRecursive(t *testing.T) {
	dir := sampleDir(t)
	no := false

	t.Run("default walks subdirectories", func(t *testing.T) {
		_, out, err := handleScanGo(context.Background(), nil, scanGoInput{Path: dir})
		if err != nil {
			t.Fatalf("handleScanGo() error = %v", err)
		}
		if !containsPath(out, "nested/deep.go") {
			t.Errorf("recursive scan missed nested/deep.go: %+v", out.Files)
		}
	})

	t.Run("recursive false stays at the top level", func(t *testing.T) {
		_, out, err := handleScanGo(
			context.Background(),
			nil,
			scanGoInput{Path: dir, Recursive: &no},
		)
		if err != nil {
			t.Fatalf("handleScanGo() error = %v", err)
		}
		if containsPath(out, "nested/deep.go") {
			t.Errorf("recursive=false scan descended into nested/: %+v", out.Files)
		}
		if !containsPath(out, "cmd.go") || !containsPath(out, "tools.go") {
			t.Errorf("recursive=false scan dropped a top-level file: %+v", out.Files)
		}
	})
}

func TestHandleScanGoBlocksMatchGoscan(t *testing.T) {
	dir := sampleDir(t)
	no := false
	_, out, err := handleScanGo(
		context.Background(),
		nil,
		scanGoInput{Path: filepath.Join(dir, "cmd.go"), Detect: &no},
	)
	if err != nil {
		t.Fatalf("handleScanGo() error = %v", err)
	}
	if out.TotalFiles != 1 {
		t.Fatalf("want 1 file, got %d", out.TotalFiles)
	}
	f := out.Files[0]
	if f.Detection != nil {
		t.Errorf("detect=false but Detection was populated: %+v", f.Detection)
	}
	var sawError bool
	for _, b := range f.Blocks {
		if b.Kind == "error" {
			sawError = true
			if b.File == "" || b.Line == 0 || b.Text == "" {
				t.Errorf("error block missing a source location: %+v", b)
			}
		}
	}
	if !sawError {
		t.Errorf("cmd.go should yield at least one error-kind block: %+v", f.Blocks)
	}
}

func TestHandleScanGoDetect(t *testing.T) {
	if _, err := exec.LookPath("vale"); err != nil {
		t.Skip("vale not installed")
	}
	dir := sampleDir(t)
	_, out, err := handleScanGo(
		context.Background(),
		nil,
		scanGoInput{Path: filepath.Join(dir, "cmd.go")},
	)
	if err != nil {
		t.Fatalf("handleScanGo() error = %v", err)
	}
	if out.TotalFiles != 1 {
		t.Fatalf("want 1 file, got %d", out.TotalFiles)
	}
	det := out.Files[0].Detection
	if det == nil {
		t.Fatal("detect defaults true; Detection should be populated")
	}
	if det.Engine != "vale" {
		t.Errorf("want engine %q, got %q", "vale", det.Engine)
	}
	// Every finding's line must map back into cmd.go's own line range, not
	// the joined-document line the vale run over the rendered doc used.
	for _, f := range det.Findings {
		if f.Line <= 0 {
			t.Errorf("finding %+v has no source line mapped back", f)
		}
	}
}

func containsPath(out scanGoOutput, suffix string) bool {
	for _, f := range out.Files {
		if strings.HasSuffix(filepath.ToSlash(f.Path), suffix) {
			return true
		}
	}
	return false
}
