package settings

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
)

func parseSettings(t *testing.T, yaml string) (Config, error) {
	t.Helper()
	f, err := config.Parse("/cfg/config.yaml", []byte(yaml))
	if err != nil {
		t.Fatalf("config.Parse: %v", err)
	}
	return ParseConfig(f.Settings)
}

func TestParseConfigResolvesPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cases := []struct {
		name, yaml, claudeDir string
		want                  Config
	}{
		{
			name: "defaults beside ~/.claude",
			yaml: "settings:\n  template: ~/dotfiles/settings.json\n",
			want: Config{
				Template: filepath.Join(home, "dotfiles/settings.json"),
				Live:     filepath.Join(home, ".claude/settings.json"),
				Overlay:  filepath.Join(home, ".claude/settings.local.json"),
			},
		},
		{
			name:      "CLAUDE_CONFIG_DIR moves live and overlay",
			yaml:      "settings:\n  template: /data/settings.json\n",
			claudeDir: "/alt/claude",
			want: Config{
				Template: "/data/settings.json",
				Live:     "/alt/claude/settings.json",
				Overlay:  "/alt/claude/settings.local.json",
			},
		},
		{
			name: "explicit live moves the default overlay with it",
			yaml: "settings:\n  template: /t.json\n  live: ~/x/settings.json\n",
			want: Config{
				Template: "/t.json",
				Live:     filepath.Join(home, "x/settings.json"),
				Overlay:  filepath.Join(home, "x/settings.local.json"),
			},
		},
		{
			name: "explicit overlay and prune",
			yaml: `settings:
  template: /t.json
  overlay: /o/local.json
  prune:
    - [includeCoAuthoredBy]  # superseded by attribution
    - ["hooks", "Notification"]
    - [modelSettings, us.anthropic.old]
`,
			want: Config{
				Template: "/t.json",
				Live:     filepath.Join(home, ".claude/settings.json"),
				Overlay:  "/o/local.json",
				Prune: []KeyPath{
					{"includeCoAuthoredBy"},
					{"hooks", "Notification"},
					{"modelSettings", "us.anthropic.old"},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(ClaudeConfigDirEnv, tc.claudeDir)
			got, err := parseSettings(t, tc.yaml)
			if err != nil {
				t.Fatalf("ParseConfig error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseConfig =\n %+v\nwant\n %+v", got, tc.want)
			}
		})
	}
}

// The first three rows port PruneTest.test_malformed_entries_are_reported:
// the Python list's "missing comma", "empty entry" and "non-str key". Its
// fourth row, a list instead of a tuple, has no YAML counterpart: lists are
// the config's format.
func TestParseConfigRejects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(ClaudeConfigDirEnv, "")
	cases := []struct {
		name, yaml string
		want       []string // substrings of the error
	}{
		{
			"prune entry is a key, not a key path",
			"settings:\n  template: /t.json\n  prune: [hooks, Retired]\n",
			[]string{"config.yaml:3: settings.prune[0]", "list of keys", `the string "hooks"`},
		},
		{
			"prune entry is empty",
			"settings:\n  template: /t.json\n  prune: [[]]\n",
			[]string{"settings.prune[0]", "empty list"},
		},
		{
			"prune key is a number",
			"settings:\n  template: /t.json\n  prune: [[hooks, 1]]\n",
			[]string{"settings.prune[0][1]", "want a string, got int 1"},
		},
		{
			"prune key is a bool",
			"settings:\n  template: /t.json\n  prune:\n    - [hooks, true]\n",
			[]string{"config.yaml:4: settings.prune[0][1]", "bool true"},
		},
		{
			"prune is not a list",
			"settings:\n  template: /t.json\n  prune: hooks.Retired\n",
			[]string{"settings.prune", "want a list of key paths"},
		},
		{
			"section missing",
			"mods: {}\n",
			[]string{"settings: section missing", "settings.template"},
		},
		{
			"template missing",
			"settings:\n  live: /l.json\n",
			[]string{"settings.template: required"},
		},
		{
			"unknown key",
			"settings:\n  template: /t.json\n  templates: /u.json\n",
			[]string{
				"config.yaml:3: settings.templates: unknown key",
				"template, live, overlay, prune",
			},
		},
		{
			"relative path",
			"settings:\n  template: dotfiles/settings.json\n",
			[]string{"settings.template", "absolute or ~/ path", `"dotfiles/settings.json"`},
		},
		{
			"template is not a string",
			"settings:\n  template: [a]\n",
			[]string{"settings.template", "want a string, got a list"},
		},
		{
			"section is not a mapping",
			"settings: /t.json\n",
			[]string{"config.yaml:1: settings: want a mapping"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSettings(t, tc.yaml)
			if err == nil {
				t.Fatal("ParseConfig error = nil, want one")
			}
			for _, s := range tc.want {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("ParseConfig error = %q, want it to contain %q", err, s)
				}
			}
		})
	}
}

func TestParseConfigRejectsRelativeClaudeConfigDir(t *testing.T) {
	t.Setenv(ClaudeConfigDirEnv, "relative/claude")
	_, err := parseSettings(t, "settings:\n  template: /t.json\n")
	if err == nil || !strings.Contains(err.Error(), "CLAUDE_CONFIG_DIR") {
		t.Errorf("ParseConfig error = %v, want one naming CLAUDE_CONFIG_DIR", err)
	}
}
