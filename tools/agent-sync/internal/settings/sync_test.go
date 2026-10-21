package settings

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

const oldContent = "old"

func TestWriteLive(t *testing.T) {
	doc := pyjson.NewObject()
	doc.Set("model", "opus")
	const want = "{\n  \"model\": \"opus\"\n}\n"
	tests := []struct {
		name  string
		setup func(t *testing.T, dir, live string)
		mode  fs.FileMode
	}{
		{"a regular file keeps its mode", func(t *testing.T, _, live string) {
			writeMode(t, live, 0o640)
		}, 0o640},
		{"a symlink takes its target's mode", func(t *testing.T, dir, live string) {
			writeMode(t, filepath.Join(dir, "target.json"), 0o644)
			symlink(t, "target.json", live)
		}, 0o644},
		{"a hard link keeps the file's mode", func(t *testing.T, dir, live string) {
			writeMode(t, filepath.Join(dir, "target.json"), 0o604)
			if err := os.Link(filepath.Join(dir, "target.json"), live); err != nil {
				t.Fatal(err)
			}
		}, 0o604},
		{"a new file is owner-only", func(*testing.T, string, string) {}, freshMode},
		{"a dangling symlink becomes a new file", func(t *testing.T, _, live string) {
			symlink(t, "missing/target.json", live)
		}, freshMode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			live := filepath.Join(dir, "settings.json")
			tt.setup(t, dir, live)
			if err := writeLive(live, doc); err != nil {
				t.Fatal(err)
			}
			info, err := os.Lstat(live)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() || info.Mode().Perm() != tt.mode {
				t.Errorf("live mode = %v, want a regular file with %v", info.Mode(), tt.mode)
			}
			if got := readString(t, live); got != want {
				t.Errorf("live = %q, want %q", got, want)
			}
			target := filepath.Join(dir, "target.json")
			if _, err := os.Stat(target); err == nil && readString(t, target) != oldContent {
				t.Errorf("the link's target changed: %q", readString(t, target))
			}
			assertOnly(t, dir, live, target)
		})
	}
}

// TestWriteLiveFailureLeavesLiveAlone covers the write that cannot happen:
// the link and its target stay as they were and no temp file is left.
func TestWriteLiveFailureLeavesLiveAlone(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only directory")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	live := filepath.Join(dir, "settings.json")
	writeMode(t, target, 0o644)
	symlink(t, "target.json", live)
	readOnly(t, dir)

	if err := writeLive(live, pyjson.NewObject()); err == nil {
		t.Fatal("writeLive into a read-only directory succeeded")
	}
	if info, err := os.Lstat(live); err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("live after a failed write: %v, %v; want the symlink", info, err)
	}
	if got := readString(t, target); got != oldContent {
		t.Errorf("target after a failed write = %q, want %q", got, oldContent)
	}
	assertOnly(t, dir, live, target)
}

// writeMode writes oldContent to path with exactly mode, whatever the umask.
func writeMode(t *testing.T, path string, mode fs.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(oldContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// readOnly makes dir read-only until the test ends.
func readOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// assertOnly fails when dir holds anything but the allowed paths: a temp
// file left behind, or a dangling link's target directory created.
func assertOnly(t *testing.T, dir string, allowed ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !slices.Contains(allowed, filepath.Join(dir, e.Name())) {
			t.Errorf("unexpected %s in %s", e.Name(), dir)
		}
	}
}
