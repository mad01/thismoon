package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/mad01/thismoon/services/csl"
)

// TestDefaultPaths covers both sides of the state-directory migration: a
// fresh machine puts the socket, PID file, and log under the XDG state
// root, while a machine that already has ~/.config/csl keeps them there so
// a running daemon and an upgraded binary still agree on the socket.
func TestDefaultPaths(t *testing.T) {
	paths := map[string]func() string{
		"search-daemon.sock": DefaultSocketPath,
		"search-daemon.pid":  DefaultPIDPath,
		"search-daemon.log":  DefaultLogPath,
	}

	t.Run("fresh install", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_STATE_HOME", "")
		for name, fn := range paths {
			want := filepath.Join(home, ".local", "state", "csl", name)
			if got := fn(); got != want {
				t.Errorf("%s = %q, want %q", name, got, want)
			}
		}
	})

	t.Run("legacy directory wins", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_STATE_HOME", "")
		legacy := filepath.Join(home, ".config", "csl")
		// The index is what marks the legacy directory as holding state; the
		// directory alone is created by provisioning on every machine.
		if err := os.MkdirAll(filepath.Join(legacy, csl.LegacyStateProbe), 0o755); err != nil {
			t.Fatal(err)
		}
		for name, fn := range paths {
			want := filepath.Join(legacy, name)
			if got := fn(); got != want {
				t.Errorf("%s = %q, want the pre-split %q", name, got, want)
			}
		}
	})
}

func TestWriteReadPID(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "test.pid")

	if err := WritePID(pidFile); err != nil {
		t.Fatalf("WritePID: %v", err)
	}

	got, err := ReadPID(pidFile)
	if err != nil {
		t.Fatalf("ReadPID: %v", err)
	}

	want := os.Getpid()
	if got != want {
		t.Fatalf("PID mismatch: got %d, want %d", got, want)
	}
}

func TestReadPID_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "missing.pid")

	_, err := ReadPID(pidFile)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadPID_InvalidContent(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "bad.pid")
	_ = os.WriteFile(pidFile, []byte("not-a-number"), 0o644)

	_, err := ReadPID(pidFile)
	if err == nil {
		t.Fatal("expected error for non-numeric content")
	}
}

func TestIsRunning_CurrentProcess(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "test.pid")

	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)

	if !IsRunning(pidFile) {
		t.Fatal("expected current process to be running")
	}
}

func TestIsRunning_DeadProcess(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "test.pid")

	_ = os.WriteFile(pidFile, []byte("99999999"), 0o644)

	if IsRunning(pidFile) {
		t.Fatal("expected PID 99999999 to not be running")
	}
}

func TestIsRunning_MissingFile(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "missing.pid")

	if IsRunning(pidFile) {
		t.Fatal("expected false for missing PID file")
	}
}

func TestRemoveStale_DeadProcess(t *testing.T) {
	dir := t.TempDir()
	sockFile := filepath.Join(dir, "test.sock")
	pidFile := filepath.Join(dir, "test.pid")

	// Create stale files with a dead PID.
	_ = os.WriteFile(sockFile, []byte("socket"), 0o644)
	_ = os.WriteFile(pidFile, []byte("99999999"), 0o644)

	if err := RemoveStale(sockFile, pidFile); err != nil {
		t.Fatalf("RemoveStale: %v", err)
	}

	if _, err := os.Stat(sockFile); !os.IsNotExist(err) {
		t.Fatal("expected socket file to be removed")
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatal("expected pid file to be removed")
	}
}

func TestRemoveStale_AliveProcess(t *testing.T) {
	dir := t.TempDir()
	sockFile := filepath.Join(dir, "test.sock")
	pidFile := filepath.Join(dir, "test.pid")

	// Create files with the current (alive) PID.
	_ = os.WriteFile(sockFile, []byte("socket"), 0o644)
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644)

	if err := RemoveStale(sockFile, pidFile); err != nil {
		t.Fatalf("RemoveStale: %v", err)
	}

	// Files should NOT be removed.
	if _, err := os.Stat(sockFile); err != nil {
		t.Fatal("expected socket file to still exist")
	}
	if _, err := os.Stat(pidFile); err != nil {
		t.Fatal("expected pid file to still exist")
	}
}

func TestRemoveStale_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	sockFile := filepath.Join(dir, "gone.sock")
	pidFile := filepath.Join(dir, "gone.pid")

	// No files exist, no PID file means IsRunning returns false,
	// and Remove on non-existent files should not error.
	if err := RemoveStale(sockFile, pidFile); err != nil {
		t.Fatalf("RemoveStale on missing files: %v", err)
	}
}
