package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// xdgConfig points config.Path() at a temp XDG home, holding content as the
// global config file when content is non-empty. Keeps a test off the real
// user config, which the commands under test would otherwise read.
func xdgConfig(t *testing.T, content string) {
	t.Helper()
	xdg := t.TempDir()
	if content != "" {
		dir := filepath.Join(xdg, "suspenders")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
}

// runDoctorString points the global config at a temp XDG home holding content,
// runs doctor against root, and returns its output.
func runDoctorString(t *testing.T, content, root string) string {
	t.Helper()
	xdgConfig(t, content)

	var b strings.Builder
	doctorCmd.SetOut(&b)
	defer doctorCmd.SetOut(nil)
	if err := runDoctor(doctorCmd, []string{root}); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}
	return b.String()
}

func TestDoctorReportsConfigAndBlockedNames(t *testing.T) {
	workspace := t.TempDir()
	internal := filepath.Join(workspace, "internalorg", "internalco")
	if err := os.MkdirAll(internal, 0o755); err != nil {
		t.Fatal(err)
	}
	initTestRepo(t, internal)

	target := t.TempDir()
	repoCfg := "guard:\n  blocked_words:\n    - repo-secret-name\n"
	if err := os.WriteFile(filepath.Join(target, ".suspenders.yaml"), []byte(repoCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	content := `
guard:
  enabled: true
  workspace_dirs:
    - ` + workspace + `
  blocked_words:
    - acmecorp
  allowlist:
    - grpc/grpc-go
`
	out := runDoctorString(t, content, target)

	for _, want := range []string{
		"guard: enabled",
		"workspace dirs: 1, blocked words: 1, safe references: 1",
		"per-repo config: " + filepath.Join(target, ".suspenders.yaml"),
		"+0 safe references, +1 blocked words",
		"guard exempt: no",
		"acmecorp",
		"internalco",
		"repo-secret-name",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorReportsWorkspaceExemption(t *testing.T) {
	workspace := t.TempDir()
	inside := filepath.Join(workspace, "somerepo")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}

	content := "guard:\n  enabled: true\n  workspace_dirs:\n    - " + workspace + "\n"
	out := runDoctorString(t, content, inside)

	if !strings.Contains(out, "guard exempt: yes (inside a workspace dir") {
		t.Errorf("workspace exemption not reported:\n%s", out)
	}
}

func TestDoctorWithDefaultConfig(t *testing.T) {
	out := runDoctorString(t, "", t.TempDir())

	for _, want := range []string{
		"guard: disabled",
		"per-repo config: none",
		"blocked names (0)",
		"the guard matches nothing",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
