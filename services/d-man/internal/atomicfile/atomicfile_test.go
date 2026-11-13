package atomicfile

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// assertEntries fails unless dir holds exactly the named entries, so a stray
// temp file shows up.
func assertEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("%s holds %v, want %v", dir, got, want)
	}
}

func TestWriteSetsContentAndMode(t *testing.T) {
	for _, perm := range []fs.FileMode{0o600, 0o644} {
		dir := t.TempDir()
		path := filepath.Join(dir, "file.json")
		if err := os.WriteFile(path, []byte("old"), 0o666); err != nil {
			t.Fatal(err)
		}
		if err := Write(path, []byte("new"), perm); err != nil {
			t.Fatalf("Write: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != perm {
			t.Errorf("mode = %v, want %v", got, perm)
		}
		if data, _ := os.ReadFile(path); string(data) != "new" {
			t.Errorf("content = %q, want new", data)
		}
		assertEntries(t, dir, "file.json")
	}
}

func TestWriteReplacesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "file.json")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("%s is %v, want a regular file", path, info.Mode().Type())
	}
	if data, _ := os.ReadFile(target); string(data) != "keep" {
		t.Errorf("symlink target = %q, want it untouched", data)
	}
	assertEntries(t, dir, "file.json", "target")
}

// A directory at path makes the rename fail, even as root.
func TestWriteFailureLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new"), 0o644); err == nil {
		t.Fatal("Write over a directory succeeded, want an error")
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Errorf("path changed: %v, %v", info, err)
	}
	assertEntries(t, dir, "file.json")
}
