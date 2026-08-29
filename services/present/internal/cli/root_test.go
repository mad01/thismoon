package cli

import (
	"os"
	"path/filepath"
	"testing"

	present "github.com/mad01/thismoon/services/present"
)

// The page store moved to the XDG state directory, but pages are never
// migrated: a machine that has published pages must keep reading the
// directory they are actually in, or every existing page URL 404s.
func TestDefaultWorkdirKeepsAnExistingLegacyStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("PRESENT_WORKDIR", "")
	if err := os.MkdirAll(filepath.Join(home, ".config", "present", present.LegacyWorkdirProbe), 0o755); err != nil {
		t.Fatalf("seed legacy store: %v", err)
	}

	if got := defaultWorkdir(); got != present.LegacyWorkdir {
		t.Errorf("defaultWorkdir() = %q, want the legacy store %q", got, present.LegacyWorkdir)
	}
}

// A legacy directory holding only the provisioned template and assets is not
// a page store: without pages/ the machine has published nothing, so it
// should adopt the state directory like any fresh install.
func TestDefaultWorkdirIgnoresProvisionedLegacyDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("PRESENT_WORKDIR", "")
	legacy := filepath.Join(home, ".config", "present")
	if err := os.MkdirAll(filepath.Join(legacy, "assets"), 0o755); err != nil {
		t.Fatalf("seed legacy dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "template.html"), []byte("<html>"), 0o644); err != nil {
		t.Fatalf("seed template: %v", err)
	}

	want := filepath.Join(home, ".local", "state", "present")
	if got := defaultWorkdir(); got != want {
		t.Errorf("defaultWorkdir() = %q with no pages/ in the legacy dir, want %q", got, want)
	}
}

func TestDefaultWorkdirLandsInTheStateDirWhenFresh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("PRESENT_WORKDIR", "")

	want := filepath.Join(home, ".local", "state", "present")
	if got := defaultWorkdir(); got != want {
		t.Errorf("defaultWorkdir() = %q, want %q", got, want)
	}
}

// The fleet's recipe and the MCP's seatbelt wrapper both set PRESENT_WORKDIR
// explicitly, so a supervised process must never take either branch above.
func TestDefaultWorkdirHonorsTheEnvOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PRESENT_WORKDIR", "/tmp/present-pages")

	if got := defaultWorkdir(); got != "/tmp/present-pages" {
		t.Errorf("defaultWorkdir() = %q, want the PRESENT_WORKDIR override", got)
	}
}
