package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteReplacesAndLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("content = %q, %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, %v; want 0644", info.Mode().Perm(), err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Errorf("dir holds %v, want the target alone", entries)
	}
}

// TestConcurrentWritersNeverShareATemporaryFile is the race a fixed
// path+".tmp" name loses: one writer renames the other's half-written file.
func TestConcurrentWritersNeverShareATemporaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Write(path, []byte(fmt.Sprintf("writer %02d\n", i)), 0o644)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Write: %v", err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) != len("writer 00\n") {
		t.Errorf("final content %q, %v; want one whole write", data, err)
	}
}

func TestWriteIntoMissingDirectoryFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "x")
	if err := Write(path, []byte("x"), 0o644); err == nil {
		t.Error("Write into a missing directory succeeded")
	}
}

func TestCreateIsExclusive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	const writers = 8
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Create(path, []byte(fmt.Sprintf("writer %02d\n", i)), 0o644)
		}()
	}
	wg.Wait()
	close(errs)
	won := 0
	for err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, fs.ErrExist):
			t.Errorf("Create: %v, want success or fs.ErrExist", err)
		}
	}
	if won != 1 {
		t.Errorf("%d writers created the file, want exactly 1", won)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Errorf("dir holds %v, want the note alone", entries)
	}
}
