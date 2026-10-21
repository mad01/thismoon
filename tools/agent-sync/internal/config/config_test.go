package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cases := []struct {
		name, override, xdg, want string
	}{
		{"default under ~/.config", "", "", filepath.Join(home, ".config/agent-sync/config.yaml")},
		{"XDG_CONFIG_HOME wins over ~/.config", "", "/xdg", "/xdg/agent-sync/config.yaml"},
		{
			"relative XDG_CONFIG_HOME is ignored",
			"",
			"rel",
			filepath.Join(home, ".config/agent-sync/config.yaml"),
		},
		{"override expands ~", "~/c.yaml", "/xdg", filepath.Join(home, "c.yaml")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", tc.xdg)
			got, err := Path(tc.override)
			if err != nil || got != tc.want {
				t.Errorf("Path(%q) = %q, %v; want %q", tc.override, got, err, tc.want)
			}
		})
	}
}

func TestLoadMissingFileIsErrNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Load error = %v, want ErrNotFound", err)
	}
}

func TestLoadReadsSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "settings:\n  template: /t.json\nmcp: {}\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if !f.Settings.Present() || f.Mods.Present() || !f.MCP.Present() {
		t.Errorf("present = settings %v, mods %v, mcp %v; want true, false, true",
			f.Settings.Present(), f.Mods.Present(), f.MCP.Present())
	}
	fields, err := f.Settings.Fields("template")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := f.Settings.String(fields["template"], "template"); err != nil ||
		got != "/t.json" {
		t.Errorf("template = %q, %v; want /t.json", got, err)
	}
}

func TestParseEmptyFileHasNoSections(t *testing.T) {
	for _, data := range []string{"", "# only a comment\n", "~\n"} {
		f, err := Parse("c.yaml", []byte(data))
		if err != nil {
			t.Errorf("Parse(%q) error: %v", data, err)
			continue
		}
		if f.Settings.Present() || f.Mods.Present() || f.MCP.Present() {
			t.Errorf("Parse(%q) found a section", data)
		}
	}
}

func TestParseRejects(t *testing.T) {
	cases := []struct {
		name, data, want string
	}{
		{
			"unknown section",
			"settings: {}\nmodz: {}\n",
			"c.yaml:2: unknown section \"modz\" (want settings, mods, mcp)",
		},
		{"duplicate section", "mcp: {}\nmcp: {}\n", "c.yaml:2: section \"mcp\" appears twice"},
		{"top level is a list", "- settings\n", "c.yaml:1: want a mapping of sections"},
		{"two documents", "settings: {}\n---\nmods: {}\n", "want one YAML document"},
		{"bad YAML", "settings: [\n", "config c.yaml: yaml:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse("c.yaml", []byte(tc.data))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Parse error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestSectionFieldsRejectsDuplicateKey(t *testing.T) {
	f, err := Parse("c.yaml", []byte("settings:\n  template: /a\n  template: /b\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Settings.Fields("template")
	if err == nil ||
		!strings.Contains(err.Error(), "c.yaml:3: settings.template: key appears twice") {
		t.Errorf("Fields error = %v, want the duplicate key named", err)
	}
}

func TestSectionErrorfWraps(t *testing.T) {
	f, err := Parse("c.yaml", []byte("settings:\n  template: /a\n"))
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("cause")
	err = f.Settings.Errorf(f.Settings.Node(), "template", "bad: %w", cause)
	if !errors.Is(err, cause) || err.Error() != "config c.yaml:2: settings.template: bad: cause" {
		t.Errorf("Errorf = %v; want it to wrap the cause and name the line", err)
	}
}

func TestSectionBool(t *testing.T) {
	cases := []struct {
		value string
		want  bool
		err   string // "" when the value parses
	}{
		{"true", true, ""},
		{"false", false, ""},
		{`"true"`, false, `c.yaml:1: mcp.prune: want true or false, got the string "true"`},
		{"yes", false, `want true or false, got the string "yes"`},
		{"1", false, "want true or false, got int 1"},
		{"", false, "want true or false, got null"},
		{"[true]", false, "want true or false, got a list"},
	}
	for _, tc := range cases {
		f, err := Parse("c.yaml", []byte("mcp: {prune: "+tc.value+"}\n"))
		if err != nil {
			t.Fatal(err)
		}
		fields, err := f.MCP.Fields("prune")
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.MCP.Bool(fields["prune"], "prune")
		switch {
		case tc.err == "" && (err != nil || got != tc.want):
			t.Errorf("Bool(%s) = %t, %v; want %t", tc.value, got, err, tc.want)
		case tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)):
			t.Errorf("Bool(%s) error = %v, want it to contain %q", tc.value, err, tc.err)
		}
	}
}

func TestSectionPathOr(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	f, err := Parse("c.yaml", []byte("mods:\n  dropins: /set/mods.d/\n"))
	if err != nil {
		t.Fatal(err)
	}
	fields, err := f.Mods.Fields("dropins", "profiles_file")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := f.Mods.PathOr(fields["dropins"], "dropins", "~/d"); err != nil ||
		got != "/set/mods.d" {
		t.Errorf("PathOr(set) = %q, %v; want the configured path", got, err)
	}
	want := filepath.Join(home, ".config/ralph/config.local.toml")
	got, err := f.Mods.PathOr(fields["profiles_file"], "profiles_file",
		"~/.config/ralph/config.local.toml")
	if err != nil || got != want {
		t.Errorf("PathOr(absent) = %q, %v; want %q", got, err, want)
	}
}
