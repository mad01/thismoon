package cli

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

func runConfigDocString(t *testing.T, p config.Paths) string {
	t.Helper()
	var b strings.Builder
	if err := runConfigDoc(&b, p); err != nil {
		t.Fatalf("runConfigDoc: %v", err)
	}
	return b.String()
}

// TestConfigHeaderStatus covers every status the header can report, and pins
// the rule that decides the shape of this command: a config belt could not
// read still prints the settings belt fell back to.
func TestConfigHeaderStatus(t *testing.T) {
	cases := []struct {
		name string
		file string // config file to write; empty writes nothing
		body string
		want string // expected in the header, after the resolved path
	}{
		{"loaded", "config.yaml", "guards: {}\n", "config.yaml (loaded)"},
		{"missing", "", "", "config.yaml (missing, defaults in use)"},
		{"parse error", "config.yaml", "guards: [broken", "config.yaml (parse error:"},
		{"legacy toml", "config.toml", "[guards.git-push-main]\n", "config.toml (loaded)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.file != "" {
				writeFile(t, dir, tc.file, tc.body)
			}
			out := runConfigDocString(t, doctorPaths(dir))

			if !strings.Contains(out, tc.want) {
				t.Errorf("header missing %q:\n%s", tc.want, out)
			}
			if !strings.Contains(out, "guards:") {
				t.Errorf("effective settings not printed:\n%s", out)
			}
		})
	}
}

func TestConfigPrintsPathsAndEffectiveValues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", `
guards:
  script-deny-list:
    enabled: false
    extra_patterns:
      - rm -rf
`)
	p := doctorPaths(dir)

	out := runConfigDocString(t, p)

	for _, want := range []string{
		"config file:  " + p.BeltYAML,
		"legacy fallback " + p.BeltTOML,
		p.Suspenders + "  (missing, fallback empty — guard: section, used only when internal_names is unset here)",
		p.Ralph + "  (missing, fallback empty — profiles list, used only when profiles is unset here)",
		"script-deny-list:",
		"enabled: false",
		"rm -rf",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "exclude_paths") {
		t.Errorf("empty toggle lists should be omitted:\n%s", out)
	}
}

// TestConfigBodyIsEffectiveYAML round-trips the body a caller would parse and
// checks the defaults are materialized: with no config file at all, every
// guard still prints as explicitly enabled.
func TestConfigBodyIsEffectiveYAML(t *testing.T) {
	out := runConfigDocString(t, doctorPaths(t.TempDir()))

	_, body, ok := strings.Cut(out, "\n\n")
	if !ok {
		t.Fatalf("no blank line between header and body:\n%s", out)
	}
	var got effectiveConfig
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}

	tog, ok := got.Guards[guard.GitPushMainID]
	if !ok {
		t.Fatalf("guards = %v, want an entry for %s", got.Guards, guard.GitPushMainID)
	}
	if tog.Enabled == nil || !*tog.Enabled {
		t.Errorf("%s enabled = %v, want true", guard.GitPushMainID, tog.Enabled)
	}
	if len(got.Hints) == 0 {
		t.Error("hints = empty, want the registered hints")
	}
	for _, key := range []string{"profiles:", "internal_names:", "claude_deny:"} {
		if !strings.Contains(body, key) {
			t.Errorf("body missing the %s section:\n%s", key, body)
		}
	}
}

// TestConfigRendersResolvedFallbacksAndClaudeDeny pins the full-render rule:
// values belt resolved from other files (ralph profiles, suspenders names,
// the Claude deny patterns) appear in the body, and the fallback header lines
// say which file was actually read.
func TestConfigRendersResolvedFallbacksAndClaudeDeny(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.local.toml", `profiles = ["work"]`)
	writeFile(t, dir, "suspenders.yaml", `
guard:
  workspace_dirs:
    - ~/workspace
  blocked_words:
    - acmecorp
  allowlist:
    - grpc/grpc-go
`)
	writeFile(t, dir, "settings.json", `{
  "permissions": {"deny": ["Bash(kubectl delete:*)", "WebFetch"]}
}`)
	p := doctorPaths(dir)

	out := runConfigDocString(t, p)

	for _, want := range []string{
		p.Suspenders + "  (in use — guard: section",
		p.Ralph + "  (in use — profiles list",
		"- work",
		"- acmecorp",
		"- kubectl delete",
		"claude_deny:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// TestConfigWarnsOnUnknownToggleKeys pins the typo guard: a config key no
// registered guard or hint answers to prints a warning instead of silently
// vanishing from the effective output.
func TestConfigWarnsOnUnknownToggleKeys(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", `
guards:
  git-push-mian:
    enabled: false
hints:
  prefer-csl:
    enabled: true
`)

	out := runConfigDocString(t, doctorPaths(dir))

	if !strings.Contains(out, `warning:      unknown guard "git-push-mian"`) {
		t.Errorf("unknown guard key not warned about:\n%s", out)
	}
	if strings.Contains(out, `unknown hint "prefer-csl"`) {
		t.Errorf("registered hint flagged as unknown:\n%s", out)
	}
	if strings.Contains(out, "git-push-mian:") {
		t.Errorf("unknown key should not appear in the effective body:\n%s", out)
	}
}

func TestConfigHelpCarriesTheReference(t *testing.T) {
	long := configDocCmd().Long

	for _, want := range []string{
		"~/.config/belt/config.yaml",
		"allow_repos",
		"exclude_paths",
		"extra_patterns",
		"kof-assertions",
		"Pair it with doctor",
	} {
		if !strings.Contains(long, want) {
			t.Errorf("--help text missing %q", want)
		}
	}
}
