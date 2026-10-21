package claudecode

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLookPath(t *testing.T) {
	dir := t.TempDir()
	claude := filepath.Join(dir, Bin)
	if err := os.WriteFile(claude, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)
	if got, err := LookPath(); err != nil || got != claude {
		t.Errorf("LookPath() = %q, %v; want %q", got, err, claude)
	}

	t.Setenv("PATH", filepath.Join(dir, "absent"))
	if got, err := LookPath(); !errors.Is(err, ErrNotFound) {
		t.Errorf("LookPath() with no claude = %q, %v; want ErrNotFound", got, err)
	}

	t.Chdir(dir)
	for _, path := range []string{".", ":/nonexistent"} {
		t.Setenv("PATH", path)
		if got, err := LookPath(); !errors.Is(err, ErrNotFound) {
			t.Errorf("PATH=%q LookPath() = %q, %v; want ErrNotFound for a relative entry",
				path, got, err)
		}
	}
}
