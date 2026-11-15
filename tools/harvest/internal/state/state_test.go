package state

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var fixed = time.Date(2026, 11, 14, 9, 30, 5, 0, time.FixedZone("CET", 3600))

func newDir(t *testing.T) *Dir {
	t.Helper()
	return New(filepath.Join(t.TempDir(), "state"), func() time.Time { return fixed })
}

func readLog(t *testing.T, d *Dir) string {
	t.Helper()
	b, err := os.ReadFile(d.LogPath())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	return string(b)
}

func TestLog(t *testing.T) {
	d := newDir(t)
	if err := d.Log("skip s1: nothing new since line 4"); err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	if err := d.LogBlock("done s2: model=m", "captured: inbox/a.md\ndropped: a guess\n"); err != nil {
		t.Fatalf("LogBlock() error = %v", err)
	}
	want := "2026-11-14T09:30:05+0100 skip s1: nothing new since line 4\n" +
		"2026-11-14T09:30:05+0100 done s2: model=m\n" +
		"  captured: inbox/a.md\n" +
		"  dropped: a guess\n"
	if got := readLog(t, d); got != want {
		t.Errorf("log = %q, want %q", got, want)
	}
}

func TestLogTail(t *testing.T) {
	d := newDir(t)
	if lines, err := d.LogTail(3); err != nil || lines != nil {
		t.Fatalf("LogTail() on no log = %q, %v; want nil, nil", lines, err)
	}
	for i := range 5 {
		if err := d.Log("line " + strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	lines, err := d.LogTail(2)
	if err != nil {
		t.Fatalf("LogTail() error = %v", err)
	}
	want := []string{
		"2026-11-14T09:30:05+0100 line 3",
		"2026-11-14T09:30:05+0100 line 4",
	}
	if !slices.Equal(lines, want) {
		t.Errorf("LogTail(2) = %q, want %q", lines, want)
	}
}

func TestLogTailPastTheWindow(t *testing.T) {
	d := newDir(t)
	if err := d.Log(strings.Repeat("x", tailWindow)); err != nil {
		t.Fatal(err)
	}
	if err := d.Log("last"); err != nil {
		t.Fatal(err)
	}
	lines, err := d.LogTail(5)
	if err != nil {
		t.Fatalf("LogTail() error = %v", err)
	}
	// The long line starts before the window, so only the last one is whole.
	if want := []string{"2026-11-14T09:30:05+0100 last"}; !slices.Equal(lines, want) {
		t.Errorf("LogTail() = %q, want %q", lines, want)
	}
}

func TestMark(t *testing.T) {
	d := newDir(t)
	if n, err := d.Mark("s1"); err != nil || n != 0 {
		t.Fatalf("Mark() of a new session = %d, %v; want 0, nil", n, err)
	}
	if err := d.SetMark("s1", 6); err != nil {
		t.Fatalf("SetMark() error = %v", err)
	}
	if n, err := d.Mark("s1"); err != nil || n != 6 {
		t.Fatalf("Mark() = %d, %v; want 6, nil", n, err)
	}
	b, err := os.ReadFile(filepath.Join(d.Path(), "sessions", "s1.lines"))
	if err != nil || string(b) != "6\n" {
		t.Errorf("mark file = %q, %v; want \"6\\n\"", b, err)
	}
}

func TestMarkRejectsGarbage(t *testing.T) {
	d := newDir(t)
	if err := d.Ensure(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(d.Path(), "sessions", "s1.lines")
	if err := os.WriteFile(p, []byte("six\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Mark("s1"); err == nil || !strings.Contains(err.Error(), "not a line count") {
		t.Errorf("Mark() of a garbage mark: error = %v, want \"not a line count\"", err)
	}
}

func TestMarkRejectsBadSession(t *testing.T) {
	d := newDir(t)
	for _, id := range []string{
		"", ".", "..", "../escape", "a/b", "a\nb", "a\rb", "a\x00b", "a\x1bb", "a\x7fb", "a\u0085b",
	} {
		if _, err := d.Mark(id); !errors.Is(err, ErrBadSession) {
			t.Errorf("Mark(%q) error = %v, want ErrBadSession", id, err)
		}
		if err := d.SetMark(id, 1); !errors.Is(err, ErrBadSession) {
			t.Errorf("SetMark(%q) error = %v, want ErrBadSession", id, err)
		}
	}
}

func TestCheckSessionTakesAPlainID(t *testing.T) {
	for _, id := range []string{"0b6c1f0e-4d1a-4e0b-9c4f-1a2b3c4d5e6f", "-s1", "a b", "é"} {
		if err := CheckSession(id); err != nil {
			t.Errorf("CheckSession(%q) = %v, want nil", id, err)
		}
	}
}

// TestModes checks that the state dir, its sessions dir and the log are
// their owner's alone, and that a state dir an earlier harvest made with
// wider modes is narrowed.
func TestModes(t *testing.T) {
	d := newDir(t)
	if err := os.MkdirAll(filepath.Join(d.Path(), "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{d.Path(), filepath.Join(d.Path(), "sessions")} {
		if err := os.Chmod(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(d.LogPath(), []byte("old line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := d.Ensure(); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if err := d.Log("skip s1: nothing new since line 4"); err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	for path, want := range map[string]os.FileMode{
		d.Path():                            0o700,
		filepath.Join(d.Path(), "sessions"): 0o700,
		d.LogPath():                         0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("mode of %s = %o, want %o", path, got, want)
		}
	}
}

func TestNewLogIsTheOwnersAlone(t *testing.T) {
	d := newDir(t)
	if err := d.Log("skip s1: nothing new since line 4"); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{d.Path(): 0o700, d.LogPath(): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("mode of %s = %o, want %o", path, got, want)
		}
	}
}

func TestStamp(t *testing.T) {
	d := newDir(t)
	if day, err := d.Stamp(); err != nil || day != "" {
		t.Fatalf("Stamp() before any = %q, %v; want \"\", nil", day, err)
	}
	if err := d.SetStamp("2026-11-14"); err != nil {
		t.Fatalf("SetStamp() error = %v", err)
	}
	if day, err := d.Stamp(); err != nil || day != "2026-11-14" {
		t.Errorf("Stamp() = %q, %v; want 2026-11-14", day, err)
	}
}
