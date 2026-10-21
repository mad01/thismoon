package settings

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
)

func doctorReport(t *testing.T, yaml string) doctor.Report {
	t.Helper()
	f, err := config.Parse("c.yaml", []byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	return doctor.Collect(context.Background(), DoctorChecks(f.Settings))
}

func statusOf(r doctor.Report) map[string]string {
	m := map[string]string{}
	for _, c := range r.Checks {
		m[c.Name] = c.Status + " " + c.Detail
	}
	return m
}

func TestDoctorChecks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv(ClaudeConfigDirEnv, "")
	template := filepath.Join(dir, "template.json")
	if err := os.WriteFile(template, []byte(`{"hooks": {"Stop": []}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("healthy with no live file yet", func(t *testing.T) {
		r := doctorReport(t, "settings:\n  template: "+template+"\n")
		got := statusOf(r)
		if !r.OK || !strings.HasPrefix(got["settings-overlay"], "skipped") ||
			got["settings-live-writable"] != "ok " {
			t.Errorf("report = %v", got)
		}
	})
	t.Run("prune conflict fails the template check", func(t *testing.T) {
		r := doctorReport(t, "settings:\n  template: "+template+"\n  prune: [[hooks, Stop]]\n")
		if got := statusOf(r)["settings-template"]; !strings.Contains(got, "fail") ||
			!strings.Contains(got, "hooks.Stop") {
			t.Errorf("settings-template = %q", got)
		}
	})
	t.Run("a broken overlay fails, though sync only warns", func(t *testing.T) {
		claude := filepath.Join(dir, ".claude")
		if err := os.MkdirAll(claude, 0o755); err != nil {
			t.Fatal(err)
		}
		overlay := filepath.Join(claude, "settings.local.json")
		if err := os.WriteFile(overlay, []byte(`{`), 0o600); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Remove(overlay) }()
		r := doctorReport(t, "settings:\n  template: "+template+"\n")
		if got := statusOf(r)["settings-overlay"]; !strings.HasPrefix(got, "fail") {
			t.Errorf("settings-overlay = %q", got)
		}
	})
	t.Run("missing template fails", func(t *testing.T) {
		r := doctorReport(t, "settings:\n  template: /nonexistent/template.json\n")
		if got := statusOf(r)["settings-template"]; !strings.Contains(got, "template not found") {
			t.Errorf("settings-template = %q", got)
		}
	})
	t.Run("no settings section is skipped", func(t *testing.T) {
		r := doctorReport(t, "mcp: {}\n")
		if got := statusOf(r)["settings"]; !strings.HasPrefix(got, "skipped") || !r.OK {
			t.Errorf("settings = %q", got)
		}
	})
	t.Run("invalid section fails the config check", func(t *testing.T) {
		r := doctorReport(t, "settings:\n  live: /x.json\n")
		if got := statusOf(r)["settings-config"]; !strings.Contains(got, "required") {
			t.Errorf("settings-config = %q", got)
		}
	})
}

// TestDoctorLiveWritable checks the directory sync renames the live file
// in, whatever sits at the live path: sync never writes through a link, so
// a link's target directory does not matter.
func TestDoctorLiveWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a read-only directory")
	}
	tests := []struct {
		name     string
		setup    func(t *testing.T, live string)
		readOnly bool
		wantOK   bool
	}{
		{"a read-only file in a writable directory", func(t *testing.T, live string) {
			writeMode(t, live, 0o400)
		}, false, true},
		{"a writable file in a read-only directory", func(t *testing.T, live string) {
			writeMode(t, live, 0o600)
		}, true, false},
		{"a dangling symlink in a writable directory", func(t *testing.T, live string) {
			symlink(t, "/nonexistent/settings.json", live)
		}, false, true},
		{"a dangling symlink in a read-only directory", func(t *testing.T, live string) {
			symlink(t, "/nonexistent/settings.json", live)
		}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			live := filepath.Join(dir, "settings.json")
			tt.setup(t, live)
			if tt.readOnly {
				readOnly(t, dir)
			}
			if err := checkLiveWritable(live); (err == nil) != tt.wantOK {
				t.Errorf("checkLiveWritable = %v, want ok %v", err, tt.wantOK)
			}
		})
	}
}
