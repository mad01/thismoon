package cli

import (
	"errors"
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
		ClaudeSettings: []string{filepath.Join(dir, "settings.json")},
	}
}

func runDoctorString(t *testing.T, p config.Paths) string {
	t.Helper()
	var b strings.Builder
	runDoctor(&b, p, stubKofProbe(3, nil))
	return b.String()
}

// stubKofProbe keeps doctor tests off the network: the real probe dials the
// kof port, which may or may not have a server behind it on a dev machine.
func stubKofProbe(count int, err error) kofProbe {
	return func() (string, int, error) {
		return "http://127.0.0.1:7431", count, err
	}
}

func TestDoctorReportsLoadedSurfacesAndBlockedNames(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "secretorg", "internalco", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "config.yaml", `
guards:
  git-push-main:
    allow_repos: [github.com/mad01/dotfiles]
  script-deny-list:
    enabled: false
internal_names:
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
		"workspace dirs: 1, blocked words: 1, allowlist: 1",
		"(from belt config internal_names",
		"blocked names (3)",
		"acmecorp",
		"secretorg",
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
		"write-internal-names has no names to match",
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

func TestDoctorReportsKofReachability(t *testing.T) {
	tests := []struct {
		name  string
		count int
		err   error
		want  string
	}{
		{"populated", 27, nil, "reachable, 27 assertions stored"},
		{
			"empty store",
			0,
			nil,
			"reachable, 0 assertions stored — kof-* hints stay silent until something deposits",
		},
		{
			"unreachable",
			0,
			errors.New("dial tcp 127.0.0.1:7431: connection refused"),
			"UNREACHABLE (dial tcp 127.0.0.1:7431: connection refused) — kof-* hints stay silent",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			runDoctor(&b, doctorPaths(t.TempDir()), stubKofProbe(tt.count, tt.err))
			out := b.String()
			if !strings.Contains(out, "kof serve — backs the kof-* hints:") {
				t.Errorf("kof section header missing:\n%s", out)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("output missing %q:\n%s", tt.want, out)
			}
		})
	}
}

func TestDoctorReportsCustomGuardsAndRules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", `
git_identity:
  - repos: [github.com/mad01/*]
    email: personal@example.com
commit_guards:
  - repos: [github.com/mad01/*]
    block_hours: "09:00-17:00"
custom_guards:
  branch-check:
    event: bash
    command: [definitely-not-on-path-xyz, check]
    match: git commit
  extra-scan:
    enabled: false
    event: write
    command: [ls]
    mode: soft
`)
	out := runDoctorString(t, doctorPaths(dir))

	for _, want := range []string{
		"git-identity",
		"(git_identity rules: 1)",
		"commit-guard",
		"(commit_guards rules: 1)",
		"custom guards — external commands",
		"branch-check",
		"command: definitely-not-on-path-xyz check",
		`match: "git commit"`,
		"UNREACHABLE",
		"extra-scan",
		"soft",
		"DISABLED",
		"overrides — rules naming one stop applying",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorReportsRulelessGuardsAsNoOps(t *testing.T) {
	out := runDoctorString(t, doctorPaths(t.TempDir()))
	for _, want := range []string{
		"(no git_identity rules — guard is a no-op)",
		"(no commit_guards rules — guard is a no-op)",
		"(none configured)",
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

	// A broken config is the state where the rest of the report describes
	// something belt is not enforcing, so doctor has to say both things: the
	// file is broken, and the hooks are denying because of it.
	t.Run("broken yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "config.yaml", "guards: [broken")
		out := runDoctorString(t, doctorPaths(dir))
		for _, want := range []string{"BROKEN", "DENIES every guarded tool call"} {
			if !strings.Contains(out, want) {
				t.Errorf("broken YAML report missing %q:\n%s", want, out)
			}
		}
	})

	// An invalid-but-parseable config lands in the same state: doctor is
	// where the key that failed validation gets named.
	t.Run("invalid custom guard event", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "config.yaml", "custom_guards:\n  lint:\n    event: Write\n    command: [lint]\n")
		out := runDoctorString(t, doctorPaths(dir))
		if !strings.Contains(out, "custom_guards.lint.event") {
			t.Errorf("invalid custom guard event not named:\n%s", out)
		}
	})
}
