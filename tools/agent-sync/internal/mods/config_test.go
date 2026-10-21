package mods

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
)

func parse(t *testing.T, yaml string) (Config, error) {
	t.Helper()
	t.Setenv("HOME", "/home/tester")
	f, err := config.Parse("/cfg.yaml", []byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	return ParseConfig(f.Mods)
}

func TestParseConfigDefaults(t *testing.T) {
	c, err := parse(t, "mods:\n  file: ~/data/mods.json\n")
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		File:         "/home/tester/data/mods.json",
		Dropins:      "/home/tester/.config/claude-mods/mods.d",
		ProfilesFile: "/home/tester/.config/ralph/config.local.toml",
	}
	if c != want {
		t.Errorf("ParseConfig = %+v, want %+v", c, want)
	}
}

func TestParseConfigEveryKey(t *testing.T) {
	c, err := parse(t, "mods:\n  file: /data/mods.json\n  dropins: ~/mods.d/\n"+
		"  profiles_file: /etc/ralph.toml\n")
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		File:         "/data/mods.json",
		Dropins:      "/home/tester/mods.d",
		ProfilesFile: "/etc/ralph.toml",
	}
	if c != want {
		t.Errorf("ParseConfig = %+v, want %+v", c, want)
	}
}

func TestParseConfigErrors(t *testing.T) {
	for name, c := range map[string]struct{ yaml, want string }{
		"no section":     {"settings: {}\n", "config /cfg.yaml: mods: section missing"},
		"no file":        {"mods: {}\n", "config /cfg.yaml:1: mods.file: required"},
		"unknown key":    {"mods:\n  file: /m.json\n  files: []\n", "config /cfg.yaml:3: mods.files: unknown key (want file, dropins, profiles_file)"},
		"relative file":  {"mods:\n  file: mods.json\n", `mods.file: want an absolute or ~/ path, got "mods.json"`},
		"dropins null":   {"mods:\n  file: /m.json\n  dropins:\n", "mods.dropins: want a string, got null"},
		"profiles bool":  {"mods:\n  file: /m.json\n  profiles_file: true\n", "mods.profiles_file: want a string, got bool true"},
		"not a mapping":  {"mods: [a]\n", "mods: want a mapping, got a list"},
		"duplicate file": {"mods:\n  file: /a.json\n  file: /b.json\n", "mods.file: key appears twice"},
	} {
		_, err := parse(t, c.yaml)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, c.want)
		}
	}
}
