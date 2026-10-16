package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDenyRulesCoverTheUsersCheckouts(t *testing.T) {
	iso := Isolation{Home: "/Users/me", Source: "/Users/me/code/src/github.com/mad01/thismoon"}
	got := iso.DenyRules()
	want := []string{
		"Edit(//Users/me/code/**)",
		"Edit(//Users/me/.worktrees/**)",
		"Bash(curl *)",
		"Bash(git -C /Users/me/code*)",
		"Bash(git -C ~/code*)",
		"Bash(git -C /Users/me/.worktrees*)",
		"Bash(git -C ~/.worktrees*)",
	}
	if !slices.Equal(got, want) {
		t.Errorf("DenyRules() =\n%q\nwant\n%q", got, want)
	}
	for _, src := range []struct{ source, edit, abs, home string }{
		{"/srv/thismoon", "Edit(//srv/thismoon/**)", "Bash(git -C /srv/thismoon*)", ""},
		{
			"/Users/me/eval-source", "Edit(//Users/me/eval-source/**)",
			"Bash(git -C /Users/me/eval-source*)", "Bash(git -C ~/eval-source*)",
		},
	} {
		rules := Isolation{Home: "/Users/me", Source: src.source}.DenyRules()
		for _, rule := range []string{src.edit, src.abs, src.home} {
			if rule != "" && !slices.Contains(rules, rule) {
				t.Errorf("a source at %s gets no %s rule: %q", src.source, rule, rules)
			}
		}
	}
}

// TestDenyRulesHoldNoWriteRules guards against dead rules: Claude checks
// the Write tool against Edit rules and warns about Write path rules.
func TestDenyRulesHoldNoWriteRules(t *testing.T) {
	for _, rule := range (Isolation{Home: "/Users/me", Source: "/srv/thismoon"}).DenyRules() {
		if strings.HasPrefix(rule, "Write(") {
			t.Errorf("DenyRules() holds %s, which Claude never consults", rule)
		}
	}
}

func TestCoversRefusesTempDirsTheArmCouldNotEdit(t *testing.T) {
	iso := Isolation{Home: "/Users/me", Source: "/Users/me/code/src/github.com/mad01/thismoon"}
	for p, want := range map[string]bool{
		"/Users/me/code/tmp":             true,
		"/Users/me/.worktrees":           true,
		"/Users/me/codex":                false,
		"/tmp":                           false,
		"/var/folders/xy/T":              false,
		"/Users/me/.worktrees-elsewhere": false,
	} {
		if got := iso.Covers(p); got != want {
			t.Errorf("Covers(%s) = %v, want %v", p, got, want)
		}
	}
}

func TestWriteSettingsHoldsTheDenyList(t *testing.T) {
	dir := t.TempDir()
	iso := Isolation{Home: "/Users/me", Source: "/Users/me/code/thismoon"}
	p, err := iso.WriteSettings(dir)
	if err != nil {
		t.Fatalf("WriteSettings() error = %v", err)
	}
	if p != filepath.Join(dir, SettingsFileName) {
		t.Errorf("path = %s, want it in the run dir", p)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &s); err != nil ||
		!slices.Equal(s.Permissions.Deny, iso.DenyRules()) {
		t.Errorf("settings file = %s (%v), want permissions.deny = DenyRules()", raw, err)
	}
	if info, err := os.Stat(p); err != nil || info.Mode().Perm()&0o222 != 0 {
		t.Errorf("settings file mode = %v, %v; want read-only", info.Mode(), err)
	}
}
