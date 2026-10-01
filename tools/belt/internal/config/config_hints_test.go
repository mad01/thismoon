package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadCustomHints(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", `
custom_hints:
  daily-journal:
    enabled: false
    event: session-start
    command: [journal, open-actions, --since, yesterday]
    timeout_ms: 250
    exclude_repos:
      - github.com/you/scratch
  plain:
    event: session-start
    command: [true]
`)
	cfg := mustLoad(t, Paths{
		BeltYAML: filepath.Join(dir, "config.yaml"),
		BeltTOML: filepath.Join(dir, "config.toml"),
	})

	journal, ok := cfg.CustomHints["daily-journal"]
	if !ok {
		t.Fatal("custom hint daily-journal not loaded")
	}
	if journal.Event != HintEventSessionStart || len(journal.Command) != 4 ||
		journal.Command[0] != "journal" || journal.TimeoutMS != 250 ||
		len(journal.ExcludeRepos) != 1 {
		t.Errorf("custom hint = %+v", journal)
	}
	if cfg.HintEnabled("daily-journal") {
		t.Error("enabled: false custom hint reported enabled")
	}
	if !cfg.HintEnabled("plain") {
		t.Error("custom hint without enabled must default to on")
	}
	if !cfg.HintRepoExcluded("daily-journal", "github.com/you/scratch") {
		t.Error("entry-level exclude_repos not read")
	}
	if got := journal.Timeout(); got != 250*time.Millisecond {
		t.Errorf("Timeout() = %s, want 250ms", got)
	}
	if got := cfg.CustomHints["plain"].Timeout(); got != CustomHintTimeout {
		t.Errorf("default Timeout() = %s, want %s", got, CustomHintTimeout)
	}
}

// TestValidateRejectsUnusableCustomHints mirrors the custom_guards rules:
// every case parses as YAML and would then advise nothing, or advise under
// a built-in's name.
func TestValidateRejectsUnusableCustomHints(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			"unsupported event",
			"custom_hints:\n  journal:\n    event: prompt\n    command: [journal]\n",
			`custom_hints.journal.event is "prompt" (supported: session-start)`,
		},
		{
			"missing event",
			"custom_hints:\n  journal:\n    command: [journal]\n",
			`custom_hints.journal.event is "" (supported: session-start)`,
		},
		{
			"missing command",
			"custom_hints:\n  journal:\n    event: session-start\n",
			"custom_hints.journal.command is empty",
		},
		{
			"blank command",
			"custom_hints:\n  journal:\n    event: session-start\n    command: [\"\"]\n",
			"custom_hints.journal.command is empty",
		},
		{
			"shadowed built-in name",
			"custom_hints:\n  kof-consult:\n    event: session-start\n    command: [journal]\n",
			"custom_hints.kof-consult shadows the built-in hint of that name",
		},
		{
			"negative timeout",
			"custom_hints:\n  journal:\n    event: session-start\n    command: [journal]\n" +
				"    timeout_ms: -1\n",
			"custom_hints.journal.timeout_ms is -1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			p := Paths{BeltYAML: writeFile(t, dir, "config.yaml", tt.content)}
			_, err := LoadFrom(p)
			if err == nil {
				t.Fatal("want a validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %v does not mention %q", err, tt.want)
			}
		})
	}
}

// TestCustomHintTogglePrecedence pins the switch order shared with custom
// guards: the entry's own enabled wins, and a hints: toggle of the same
// name is the fallback.
func TestCustomHintTogglePrecedence(t *testing.T) {
	off, on := false, true
	cfg := Config{
		CustomHints: map[string]CustomHint{
			"entry-off":   {Enabled: &off},
			"entry-unset": {},
		},
		Hints: map[string]Toggle{
			"entry-off":   {Enabled: &on},
			"entry-unset": {Enabled: &off},
		},
	}
	if cfg.HintEnabled("entry-off") {
		t.Error("entry enabled: false must win over the hints: toggle")
	}
	if cfg.HintEnabled("entry-unset") {
		t.Error("an unset entry must fall back to the hints: toggle")
	}
}

// TestHintRepoExcludedUnionsCustomLists: a custom hint's opt-outs may sit on
// the entry or on the hints: toggle, and either one excludes.
func TestHintRepoExcludedUnionsCustomLists(t *testing.T) {
	cfg := Config{
		CustomHints: map[string]CustomHint{
			"journal": {ExcludeRepos: []string{"github.com/you/entry"}},
		},
		Hints: map[string]Toggle{
			"journal": {ExcludeRepos: []string{"github.com/you/toggle"}},
		},
	}
	for _, repo := range []string{"github.com/you/entry", "github.com/you/toggle"} {
		if !cfg.HintRepoExcluded("journal", repo) {
			t.Errorf("%s should be excluded", repo)
		}
	}
	if cfg.HintRepoExcluded("journal", "github.com/you/other") {
		t.Error("an unlisted repo must not be excluded")
	}
}
