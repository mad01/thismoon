package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// doctorPaths builds a Paths fixture rooted in dir; files the test does not
// write stay missing, exercising doctor's missing-surface reporting.
func doctorPaths(dir string) config.Paths {
	return config.Paths{
		BeltYAML:       filepath.Join(dir, "config.yaml"),
		BeltTOML:       filepath.Join(dir, "config.toml"),
		Ralph:          filepath.Join(dir, "config.local.toml"),
		Suspenders:     filepath.Join(dir, "suspenders.yaml"),
		ClaudeSettings: []string{filepath.Join(dir, "settings.json")},
	}
}

func runDoctorString(t *testing.T, p config.Paths) string {
	t.Helper()
	var b strings.Builder
	runDoctor(&b, p)
	return b.String()
}

func TestDoctorReportsLoadedSurfacesAndBlockedNames(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "internalco"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "config.yaml", `
guards:
  git-push-main:
    allow_repos: [github.com/mad01/dotfiles]
  script-deny-list:
    enabled: false
`)
	writeFile(t, dir, "config.local.toml", `profiles = ["work"]`)
	writeFile(t, dir, "suspenders.yaml", `
guard:
  workspace_dirs:
    - `+workspace+`
  blocked_words:
    - acmecorp
  allowlist:
    - grpc/grpc-go
`)

	out := runDoctorString(t, doctorPaths(dir))

	for _, want := range []string{
		"config.yaml  loaded",
		"profiles: work",
		"workspace dirs: 1, blocked words: 1, safe references: 1",
		"blocked names (2)",
		"acmecorp",
		"internalco",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "script-deny-list") || !strings.Contains(out, "DISABLED") {
		t.Errorf("disabled guard not reported:\n%s", out)
	}
	if strings.Contains(out, "git-push-main         bash    DISABLED") {
		t.Errorf("git-push-main should be enabled:\n%s", out)
	}
	if !strings.Contains(out, "allow_repos: 1") {
		t.Errorf("toggle detail missing:\n%s", out)
	}
}

func TestDoctorReportsMissingConfigs(t *testing.T) {
	out := runDoctorString(t, doctorPaths(t.TempDir()))

	for _, want := range []string{
		"missing — defaults, everything enabled",
		"no profiles — git-push-main fails closed",
		"missing — write-internal-names has no names to match",
		"blocked names (0)",
		"allows every write",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorReportsLegacyTOMLAndParseErrors(t *testing.T) {
	t.Run("legacy toml", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "config.toml", "[guards.git-push-main]\nenabled = true\n")
		out := runDoctorString(t, doctorPaths(dir))
		if !strings.Contains(out, "legacy TOML — rename to config.yaml") {
			t.Errorf("legacy TOML not flagged:\n%s", out)
		}
	})

	t.Run("broken yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "config.yaml", "guards: [broken")
		out := runDoctorString(t, doctorPaths(dir))
		if !strings.Contains(out, "PARSE ERROR") {
			t.Errorf("broken YAML not flagged:\n%s", out)
		}
	})
}

func TestConfigDocPrintsPathsAndReference(t *testing.T) {
	cmd := configDocCmd()
	var b strings.Builder
	cmd.SetOut(&b)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("config: %v", err)
	}
	out := b.String()
	for _, want := range []string{"config.yaml", "legacy fallback", "allow_repos", "exclude_paths", "blocked-name source"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}
