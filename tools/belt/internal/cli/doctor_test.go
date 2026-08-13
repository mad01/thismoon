package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
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

func TestDoctorReportsBuildMetadata(t *testing.T) {
	setBuildInfo(t, "abc1234", "abc1234def5678", "belt/v1.2.3", "2026-08-13T10:00:00Z")

	out := runDoctorString(t, doctorPaths(t.TempDir()))

	for _, want := range []string{
		"build:",
		"version      abc1234",
		"commit       abc1234def5678",
		"tag          belt/v1.2.3",
		"build time   2026-08-13T10:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorReportsUnknownBuildFields(t *testing.T) {
	setBuildInfo(t, "abc1234", "", "", "")

	out := runDoctorString(t, doctorPaths(t.TempDir()))

	if !strings.Contains(out, "tag          (unknown)") {
		t.Errorf("empty build field not rendered as unknown:\n%s", out)
	}
}

// setBuildInfo pins the linker-injected build metadata for one test. Get()
// only falls back to the toolchain's vcs stamps for fields left empty, so a
// non-empty Version keeps the fixture hermetic.
func setBuildInfo(t *testing.T, version, commit, tag, buildTime string) {
	t.Helper()
	orig := [4]string{
		buildinfo.Version,
		buildinfo.Commit,
		buildinfo.Tag,
		buildinfo.BuildTime,
	}
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit = orig[0], orig[1]
		buildinfo.Tag, buildinfo.BuildTime = orig[2], orig[3]
	})
	buildinfo.Version, buildinfo.Commit = version, commit
	buildinfo.Tag, buildinfo.BuildTime = tag, buildTime
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
