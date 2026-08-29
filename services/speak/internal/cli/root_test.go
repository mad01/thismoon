package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	speak "github.com/mad01/thismoon/services/speak"
)

// TestDefaultStateDirKeepsPlayedInstall is the migration contract: a machine
// that has generated audio under the legacy directory keeps using it, so an
// upgrade does not strand the audio cache or the playback lock.
func TestDefaultStateDirKeepsPlayedInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("SPEAK_STATE_DIR", "")

	legacy := filepath.Join(home, ".local", "share", "speak")
	if err := os.MkdirAll(filepath.Join(legacy, speak.LegacyStateProbe), 0o755); err != nil {
		t.Fatalf("seed legacy audio cache: %v", err)
	}

	if got := defaultStateDir(); got != speak.LegacyStateDir {
		t.Errorf("defaultStateDir() = %q, want the legacy dir %q", got, speak.LegacyStateDir)
	}
}

// TestDefaultStateDirIgnoresEngineInstall is the case the probe exists for,
// and the one this fleet actually has: the mlx-audio engine installs its
// virtualenv and logs under the legacy path, so the directory is there on
// every machine that has the TTS engine, whether or not speak ever played
// anything. Without a probe such a machine could never adopt the state
// directory.
func TestDefaultStateDirIgnoresEngineInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("SPEAK_STATE_DIR", "")

	legacy := filepath.Join(home, ".local", "share", "speak")
	for _, dir := range []string{"venv", "logs"} {
		if err := os.MkdirAll(filepath.Join(legacy, dir), 0o755); err != nil {
			t.Fatalf("seed engine install: %v", err)
		}
	}

	want := filepath.Join(home, ".local", "state", "speak")
	if got := defaultStateDir(); got != want {
		t.Errorf("defaultStateDir() = %q with only the engine install present, want %q", got, want)
	}
}

func TestDefaultStateDirFreshInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("SPEAK_STATE_DIR", "")

	want := filepath.Join(home, ".local", "state", "speak")
	if got := defaultStateDir(); got != want {
		t.Errorf("defaultStateDir() = %q, want %q", got, want)
	}
}

// TestDefaultStateDirEnvWins keeps the supervised path pinned: the fleet sets
// SPEAK_STATE_DIR explicitly, and it has to beat both branches above.
func TestDefaultStateDirEnvWins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("SPEAK_STATE_DIR", "/tmp/speak-state")

	legacy := filepath.Join(home, ".local", "share", "speak")
	if err := os.MkdirAll(filepath.Join(legacy, speak.LegacyStateProbe), 0o755); err != nil {
		t.Fatalf("seed legacy audio cache: %v", err)
	}

	if got := defaultStateDir(); got != "/tmp/speak-state" {
		t.Errorf("defaultStateDir() = %q, want the env override", got)
	}
}

// TestStateDirReadable covers the three states of the playback directory:
// absent before the first synthesis (a skip, not a fault), present and
// readable, and present but unopenable (a real failure).
func TestStateDirReadable(t *testing.T) {
	base := t.TempDir()

	t.Run("absent skips", func(t *testing.T) {
		report := doctor.Collect(context.Background(),
			[]doctor.Check{stateDirReadable(filepath.Join(base, "not-yet"))})
		if !report.OK {
			t.Fatalf("report.OK = false, want a skip to pass: %+v", report.Checks)
		}
		if got := report.Checks[0].Status; got != doctor.StatusSkipped {
			t.Errorf("status = %q, want %q", got, doctor.StatusSkipped)
		}
		if got := report.Checks[0].Detail; !strings.Contains(got, "no playback state yet") {
			t.Errorf("detail = %q, want it to explain the empty state", got)
		}
	})

	t.Run("present passes", func(t *testing.T) {
		dir := filepath.Join(base, "played")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := stateDirReadable(dir).Run(context.Background()); err != nil {
			t.Errorf("Run() = %v, want nil", err)
		}
	})

	t.Run("unreadable fails", func(t *testing.T) {
		dir := filepath.Join(base, "locked")
		if err := os.MkdirAll(dir, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory permissions")
		}
		if err := stateDirReadable(dir).Run(context.Background()); err == nil {
			t.Error("Run() = nil, want an error for an unreadable state dir")
		}
	})
}
