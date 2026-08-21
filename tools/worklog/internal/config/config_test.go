package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileIsZero(t *testing.T) {
	t.Setenv("WORKLOG_CONFIG", filepath.Join(t.TempDir(), "nope.yaml"))
	c := Load()
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
	t.Setenv("WORKLOG_CONFIG", p)
	c := Load()
	if len(c.Scan.LinearPrefixes) != 1 || c.Scan.LinearPrefixes[0] != "XYZ" {
		t.Errorf("linear_prefixes = %v", c.Scan.LinearPrefixes)
	}
	if len(c.Scan.PersonalPathMarkers) != 1 || c.Scan.PersonalPathMarkers[0] != "github.com/someone/" {
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

func TestLoadMalformedFileIsZero(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("scan: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKLOG_CONFIG", p)
	c := Load()
	if len(c.Scan.LinearPrefixes) != 0 {
		t.Errorf("expected zero config for malformed yaml, got %+v", c)
	}
}

func TestRemoteResolveUpstream(t *testing.T) {
	r := Remote{Upstreams: map[string]string{
		"personal": "git@example.com:me/personal.git",
		"work":     "git@example.com:me/work.git",
	}}
	tests := []struct {
		name     string
		profiles []string
		want     string
	}{
		{"first profile wins", []string{"work", "personal"}, "git@example.com:me/work.git"},
		{"personal machine", []string{"personal"}, "git@example.com:me/personal.git"},
		{"unknown profile", []string{"lab"}, ""},
		{"no profiles", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.ResolveUpstream(tt.profiles); got != tt.want {
				t.Errorf("ResolveUpstream(%v) = %q, want %q", tt.profiles, got, tt.want)
			}
		})
	}
	if got := (Remote{}).ResolveUpstream([]string{"personal"}); got != "" {
		t.Errorf("no upstreams: got %q, want empty", got)
	}
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

func TestMachineProfiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.local.toml")
	t.Setenv("WORKLOG_RALPH_CONFIG", p)
	if got := MachineProfiles(); got != nil {
		t.Errorf("missing file: got %v, want nil", got)
	}
	if err := os.WriteFile(p, []byte("profiles = [\"work\", \"lab\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := MachineProfiles()
	if len(got) != 2 || got[0] != "work" || got[1] != "lab" {
		t.Errorf("MachineProfiles() = %v, want [work lab]", got)
	}
}
