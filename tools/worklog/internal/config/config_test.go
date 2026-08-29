package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileIsZero(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("Load(missing) = %v, want nil error", err)
	}
	if len(c.Scan.LinearPrefixes) != 0 {
		t.Errorf("expected zero config, got %+v", c)
	}
}

func TestLoadReadsScanSection(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	content := "scan:\n  linear_prefixes: [XYZ]\n  personal_path_markers: [\"github.com/someone/\"]\n  internal_path_markers: [\"/dayjob/\"]\n  checkout_roots: [\"/checkouts/\"]\n  repo_path_markers: [\"/repos/\"]\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Scan.LinearPrefixes) != 1 || c.Scan.LinearPrefixes[0] != "XYZ" {
		t.Errorf("linear_prefixes = %v", c.Scan.LinearPrefixes)
	}
	if len(c.Scan.PersonalPathMarkers) != 1 ||
		c.Scan.PersonalPathMarkers[0] != "github.com/someone/" {
		t.Errorf("personal_path_markers = %v", c.Scan.PersonalPathMarkers)
	}
	if len(c.Scan.InternalPathMarkers) != 1 || c.Scan.InternalPathMarkers[0] != "/dayjob/" {
		t.Errorf("internal_path_markers = %v", c.Scan.InternalPathMarkers)
	}
	if len(c.Scan.CheckoutRoots) != 1 || c.Scan.CheckoutRoots[0] != "/checkouts/" {
		t.Errorf("checkout_roots = %v", c.Scan.CheckoutRoots)
	}
	if len(c.Scan.RepoPathMarkers) != 1 || c.Scan.RepoPathMarkers[0] != "/repos/" {
		t.Errorf("repo_path_markers = %v", c.Scan.RepoPathMarkers)
	}
}

func TestLoadMalformedFileErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("scan: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Error("Load(malformed) = nil error, want the parse failure reported")
	}
}

// TestLoadUnreadableFileErrors covers the case the old Load swallowed: a file
// that is there and cannot be read is not the same as no file, and treating
// it as one drops the machine's firewall strings and push remote in silence.
func TestLoadUnreadableFileErrors(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte("scan:\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads unreadable files")
	}
	if _, err := Load(p); err == nil {
		t.Error("Load(unreadable) = nil error, want the read failure reported")
	}
}

func TestPath(t *testing.T) {
	t.Run("override wins", func(t *testing.T) {
		got, err := Path("/tmp/elsewhere.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if got != "/tmp/elsewhere.yaml" {
			t.Errorf("Path(override) = %q", got)
		}
	})

	t.Run("override expands ~", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		got, err := Path("~/wl.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(home, "wl.yaml"); got != want {
			t.Errorf("Path(~) = %q, want %q", got, want)
		}
	})

	t.Run("default honors XDG_CONFIG_HOME", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		got, err := Path("")
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(dir, Component, FileName); got != want {
			t.Errorf("Path(\"\") = %q, want %q", got, want)
		}
	})

	t.Run("default falls back to ~/.config", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", home)
		got, err := Path("")
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(home, ".config", Component, FileName); got != want {
			t.Errorf("Path(\"\") = %q, want %q", got, want)
		}
	})

	// The bug this replaced: an unresolvable home used to join into a
	// cwd-relative ".config/worklog/config.yaml", so a process started
	// anywhere read a config nobody wrote.
	t.Run("unresolvable home errors", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		got, err := Path("")
		if err == nil {
			t.Fatalf("Path(\"\") = %q, want an error with no home directory", got)
		}
		if strings.HasPrefix(got, ".config") {
			t.Errorf("Path(\"\") = %q, want no relative fallback", got)
		}
	})
}

func TestRemotePushEnabled(t *testing.T) {
	off, on := false, true
	if !(Remote{}).PushEnabled() {
		t.Error("unset push should be enabled")
	}
	if (Remote{Push: &off}).PushEnabled() {
		t.Error("push: false should disable")
	}
	if !(Remote{Push: &on}).PushEnabled() {
		t.Error("push: true should enable")
	}
}

func TestRemoteRetiredUpstreams(t *testing.T) {
	profileKeyed := Remote{Upstreams: map[string]string{"personal": "git@example.com:me/p.git"}}
	if !profileKeyed.RetiredUpstreams() {
		t.Error("a config with only upstreams should report the retired shape")
	}
	migrated := Remote{URL: "git@example.com:me/p.git", Upstreams: profileKeyed.Upstreams}
	if migrated.RetiredUpstreams() {
		t.Error("a config with a url should not warn about a leftover upstreams key")
	}
	if (Remote{}).RetiredUpstreams() {
		t.Error("a local-only config has nothing to warn about")
	}
}
