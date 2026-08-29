package commands

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/tools/suspenders/internal/config"
)

// runConfigDocString points the global config at a temp XDG home holding
// content, runs config, and returns its output.
func runConfigDocString(t *testing.T, content string) string {
	t.Helper()
	xdgConfig(t, content)

	var b strings.Builder
	configDocCmd.SetOut(&b)
	defer configDocCmd.SetOut(nil)
	if err := runConfigDoc(configDocCmd, nil); err != nil {
		t.Fatalf("runConfigDoc: %v", err)
	}
	return b.String()
}

// TestConfigHeaderStatus covers every status the header can report, and pins
// the rule that decides the shape of this command: a config suspenders could
// not read still prints the settings it fell back to.
func TestConfigHeaderStatus(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string // expected in the header, after the resolved path
	}{
		{"loaded", "guard:\n  enabled: true\n", "(loaded)"},
		{"missing", "", "(missing, defaults in use)"},
		{"parse error", "guard: [broken", "(parse error:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := runConfigDocString(t, tc.content)

			path, err := config.Path()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "config file: "+path+" "+tc.want) {
				t.Errorf("header missing path and %q:\n%s", tc.want, out)
			}
			if !strings.Contains(out, "per-repo:    .suspenders.yaml") {
				t.Errorf("per-repo line missing:\n%s", out)
			}
			if !strings.Contains(out, "guard:") {
				t.Errorf("effective settings not printed:\n%s", out)
			}
		})
	}
}

// TestConfigBodyIsEffectiveYAML round-trips the body a caller would parse and
// checks the defaults are materialized: scan.enabled is nil in the file and
// resolves to true, so it must print as true rather than null.
func TestConfigBodyIsEffectiveYAML(t *testing.T) {
	out := runConfigDocString(t, `
guard:
  enabled: true
  blocked_words:
    - acmecorp
`)

	_, body, ok := strings.Cut(out, "\n\n")
	if !ok {
		t.Fatalf("no blank line between header and body:\n%s", out)
	}
	var got config.Config
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}

	if !got.Guard.Enabled {
		t.Error("guard.enabled = false, want true")
	}
	if len(got.Guard.BlockedWords) != 1 || got.Guard.BlockedWords[0] != "acmecorp" {
		t.Errorf("guard.blocked_words = %v, want [acmecorp]", got.Guard.BlockedWords)
	}
	if got.Scan.Enabled == nil || !*got.Scan.Enabled {
		t.Errorf("scan.enabled = %v, want an explicit true", got.Scan.Enabled)
	}
}

func TestConfigWithNoFilePrintsDefaults(t *testing.T) {
	out := runConfigDocString(t, "")

	for _, want := range []string{"dirs:", "~/code/src", "enabled: true"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestConfigHelpCarriesTheReference(t *testing.T) {
	for _, want := range []string{
		"~/.config/suspenders/config.yaml",
		"workspace_dirs",
		"blocked_words",
		"safe references",
		"replace_table",
		"Pair it with doctor",
	} {
		if !strings.Contains(configDocCmd.Long, want) {
			t.Errorf("--help text missing %q", want)
		}
	}
}

// TestConfigInitWritesOnceAndRefusesToOverwrite pins the only path that
// creates a config file: it is explicit, and it never clobbers an existing
// file (the surprise-write behavior it replaced did neither).
func TestConfigInitWritesOnceAndRefusesToOverwrite(t *testing.T) {
	xdgConfig(t, "")

	var b strings.Builder
	configInitCmd.SetOut(&b)
	defer configInitCmd.SetOut(nil)

	if err := runConfigInit(configInitCmd, nil); err != nil {
		t.Fatalf("runConfigInit: %v", err)
	}
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config init did not write %s: %v", path, err)
	}
	if !strings.Contains(string(written), "dirs:") {
		t.Errorf("written config has no dirs:\n%s", written)
	}

	err = runConfigInit(configInitCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second config init error = %v, want an already-exists refusal", err)
	}
}
