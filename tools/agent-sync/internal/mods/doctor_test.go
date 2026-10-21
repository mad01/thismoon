package mods

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
)

// doctorHome lays out a home doctor passes on: a mods file, its marketplace
// on disk, empty plugin state, and an executable named claude on PATH.
func doctorHome(t *testing.T) (home string, section config.Section) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	bin := filepath.Join(home, "bin")
	write(t, filepath.Join(bin, "claude"), "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(bin, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	write(t, filepath.Join(home, "mods.json"),
		`{"marketplace": {"name": "lab", "path": "~/lab"}, "mods": {"pane": {"enabled": true}}}`)
	write(t, filepath.Join(home, "lab", ".claude-plugin", "marketplace.json"),
		`{"name": "lab", "plugins": [{"name": "pane", "source": "./pane"}]}`)
	write(t, filepath.Join(home, ".claude", "settings.json"), `{}`)
	f, err := config.Parse(
		"/cfg.yaml",
		[]byte("mods:\n  file: "+filepath.Join(home, "mods.json")+"\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return home, f.Mods
}

func collect(s config.Section) map[string]doctor.Result {
	results := map[string]doctor.Result{}
	for _, r := range doctor.Collect(context.Background(), DoctorChecks(s)).Checks {
		results[r.Name] = r
	}
	return results
}

func TestDoctorPassesOnAWorkingSetup(t *testing.T) {
	_, s := doctorHome(t)
	got := collect(s)
	for _, name := range []string{"mods-config", "mods-files", "mods-marketplaces", "mods-state", "mods-claude"} {
		if got[name].Status != doctor.StatusOK {
			t.Errorf("%s = %+v, want ok", name, got[name])
		}
	}
}

func TestDoctorReportsEachProblem(t *testing.T) {
	home, s := doctorHome(t)
	write(t, filepath.Join(home, ".claude", "settings.json"), `{"enabledPlugins": ["x"]}`)
	if err := os.RemoveAll(filepath.Join(home, "lab")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(home, "empty"))
	got := collect(s)
	for name, want := range map[string]struct{ status, detail string }{
		"mods-marketplaces": {doctor.StatusSkipped, "not on disk yet, sync skips them: lab at " + home + "/lab"},
		"mods-state":        {doctor.StatusFail, "'enabledPlugins' must be an object"},
		"mods-claude":       {doctor.StatusFail, "claude CLI not found on PATH"},
	} {
		if got[name].Status != want.status || !strings.Contains(got[name].Detail, want.detail) {
			t.Errorf("%s = %+v, want %s containing %q", name, got[name], want.status, want.detail)
		}
	}
}

func TestDoctorFailsOnAManifestSyncRejects(t *testing.T) {
	home, s := doctorHome(t)
	write(
		t,
		filepath.Join(home, "lab", ".claude-plugin", "marketplace.json"),
		`{"name": "other", "plugins": []}`,
	)
	if r := collect(s)["mods-marketplaces"]; r.Status != doctor.StatusFail ||
		!strings.Contains(r.Detail, "does not match the name 'other'") {
		t.Errorf("mods-marketplaces = %+v", r)
	}
}

func TestDoctorStopsAtABadConfig(t *testing.T) {
	f, err := config.Parse("/cfg.yaml", []byte("mods: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	checks := doctor.Collect(context.Background(), DoctorChecks(f.Mods)).Checks
	if len(checks) != 1 || checks[0].Status != doctor.StatusFail ||
		!strings.Contains(checks[0].Detail, "mods.file: required") {
		t.Errorf("checks = %+v", checks)
	}
}

func TestDoctorSkipsAnAbsentSection(t *testing.T) {
	f, err := config.Parse("/cfg.yaml", []byte("settings: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	checks := doctor.Collect(context.Background(), DoctorChecks(f.Mods)).Checks
	if len(checks) != 1 || checks[0].Name != "mods" || checks[0].Status != doctor.StatusSkipped {
		t.Errorf("checks = %+v", checks)
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
