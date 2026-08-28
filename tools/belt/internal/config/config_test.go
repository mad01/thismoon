package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGuardEnabled(t *testing.T) {
	off := false
	on := true
	cfg := Config{Guards: map[string]Toggle{
		"disabled-guard": {Enabled: &off},
		"enabled-guard":  {Enabled: &on},
		"nil-toggle":     {},
	}}
	tests := []struct {
		id   string
		want bool
	}{
		{"disabled-guard", false},
		{"enabled-guard", true},
		{"nil-toggle", true},
		{"unknown-guard", true},
	}
	for _, tt := range tests {
		if got := cfg.GuardEnabled(tt.id); got != tt.want {
			t.Errorf("GuardEnabled(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestLoadClaudeDenyPatterns(t *testing.T) {
	dir := t.TempDir()
	settings := writeFile(t, dir, "settings.json", `{
  "permissions": {
    "deny": [
      "Read(~/.config/secret.sh)",
      "Bash(kubectl delete:*)",
      "Bash(gcloud projects delete:*)",
      "Bash(git config:*)",
      "Grep(~/.config/brain/**)"
    ]
  }
}`)
	local := writeFile(t, dir, "settings.local.json", `{
  "permissions": {
    "deny": [
      "Bash(kubectl delete:*)",
      "Bash(bq rm:*)"
    ]
  }
}`)
	got := LoadClaudeDenyPatterns(settings, local, filepath.Join(dir, "missing.json"))
	want := []string{"kubectl delete", "gcloud projects delete", "git config", "bq rm"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func TestLoadClaudeDenyPatternsMalformed(t *testing.T) {
	path := writeFile(t, t.TempDir(), "settings.json", `{not json`)
	if got := LoadClaudeDenyPatterns(path); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// missingYAML returns a config.yaml path that does not exist in dir, so
// loadFile exercises its legacy-TOML fallback.
func missingYAML(dir string) string {
	return filepath.Join(dir, "config.yaml")
}

func TestLoadTogglesExtraPatterns(t *testing.T) {
	content := `
[guards.script-deny-list]
enabled = true
extra_patterns = ["rm -rf", "rm -fr"]
`
	dir := t.TempDir()
	path := writeFile(t, dir, "config.toml", content)
	f := loadFile(missingYAML(dir), path)
	patterns := f.Guards["script-deny-list"].ExtraPatterns
	if len(patterns) != 2 || patterns[0] != "rm -rf" || patterns[1] != "rm -fr" {
		t.Errorf("extra_patterns = %v", patterns)
	}
}

func TestLoadTogglesAllowRepos(t *testing.T) {
	content := `
[guards.git-push-main]
allow_repos = ["github.com/mad01/dotfiles"]
`
	dir := t.TempDir()
	path := writeFile(t, dir, "config.toml", content)
	f := loadFile(missingYAML(dir), path)
	repos := f.Guards["git-push-main"].AllowRepos
	if len(repos) != 1 || repos[0] != "github.com/mad01/dotfiles" {
		t.Errorf("allow_repos = %v", repos)
	}
}

func TestLoadTogglesHintsSection(t *testing.T) {
	content := `
[guards.git-push-main]
enabled = true

[hints.kof-assertions]
enabled = false
`
	dir := t.TempDir()
	path := writeFile(t, dir, "config.toml", content)
	f := loadFile(missingYAML(dir), path)
	guards, hints := f.Guards, f.Hints
	if guards["git-push-main"].Enabled == nil || !*guards["git-push-main"].Enabled {
		t.Error("guards section did not survive adding hints")
	}
	if hints["kof-assertions"].Enabled == nil || *hints["kof-assertions"].Enabled {
		t.Error("hints.kof-assertions should have decoded as disabled")
	}
}

func TestLoadTogglesYAML(t *testing.T) {
	content := `
guards:
  git-push-main:
    enabled: true
    allow_repos:
      - github.com/mad01/dotfiles
  write-internal-names:
    exclude_paths:
      - recipes/belt/
hints:
  kof-assertions:
    enabled: false
`
	dir := t.TempDir()
	yamlPath := writeFile(t, dir, "config.yaml", content)
	f := loadFile(yamlPath, missingYAML(dir))
	guards, hints := f.Guards, f.Hints
	repos := guards["git-push-main"].AllowRepos
	if len(repos) != 1 || repos[0] != "github.com/mad01/dotfiles" {
		t.Errorf("allow_repos = %v", repos)
	}
	excl := guards["write-internal-names"].ExcludePaths
	if len(excl) != 1 || excl[0] != "recipes/belt/" {
		t.Errorf("exclude_paths = %v", excl)
	}
	if hints["kof-assertions"].Enabled == nil || *hints["kof-assertions"].Enabled {
		t.Error("hints.kof-assertions should have decoded as disabled")
	}
}

func TestLoadTogglesYAMLWinsOverTOML(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeFile(t, dir, "config.yaml", "guards:\n  git-push-main:\n    enabled: false\n")
	tomlPath := writeFile(t, dir, "config.toml", "[guards.git-push-main]\nenabled = true\n")
	f := loadFile(yamlPath, tomlPath)
	if f.Guards["git-push-main"].Enabled == nil || *f.Guards["git-push-main"].Enabled {
		t.Error("YAML config should win when both files exist")
	}
}

func TestLoadTogglesBrokenYAMLYieldsDefaultsNotTOML(t *testing.T) {
	dir := t.TempDir()
	yamlPath := writeFile(t, dir, "config.yaml", "guards: [broken")
	tomlPath := writeFile(t, dir, "config.toml", "[guards.git-push-main]\nenabled = false\n")
	f := loadFile(yamlPath, tomlPath)
	if f.Guards != nil || f.Hints != nil {
		t.Errorf("broken YAML must yield defaults, not the stale TOML: guards=%v hints=%v", f.Guards, f.Hints)
	}
}

func TestHintEnabledDefaultsOn(t *testing.T) {
	cfg := Config{}
	if !cfg.HintEnabled("kof-assertions") {
		t.Error("a hint with no config entry should default to enabled")
	}
	off := false
	cfg = Config{Hints: map[string]Toggle{"kof-assertions": {Enabled: &off}}}
	if cfg.HintEnabled("kof-assertions") {
		t.Error("an explicitly disabled hint should report disabled")
	}
	if !cfg.HintEnabled("prefer-csl") {
		t.Error("disabling one hint must not disable the others")
	}
}

// fixturePaths builds a Paths pointing into dir, so LoadFrom tests control
// exactly which surfaces exist.
func fixturePaths(dir string) Paths {
	return Paths{
		BeltYAML:       filepath.Join(dir, "belt.yaml"),
		BeltTOML:       filepath.Join(dir, "belt.toml"),
		ClaudeSettings: []string{filepath.Join(dir, "settings.json")},
	}
}

func TestLoadFromInternalNamesFromBeltConfig(t *testing.T) {
	dir := t.TempDir()
	p := fixturePaths(dir)
	writeFile(t, dir, "belt.yaml", `
internal_names:
  workspace_dirs:
    - ~/workspace
  blocked_words:
    - beltword
`)

	cfg := LoadFrom(p)
	if len(cfg.Names.BlockedWords) != 1 || cfg.Names.BlockedWords[0] != "beltword" {
		t.Errorf("blocked_words = %v, want [beltword]", cfg.Names.BlockedWords)
	}
	if len(cfg.Names.WorkspaceDirs) != 1 {
		t.Errorf("workspace_dirs = %v, want one entry", cfg.Names.WorkspaceDirs)
	}
}

// TestLoadFromInternalNamesAbsentMeansEmpty pins the standalone rule
// (docs/adr/0010): with no internal_names section, the name set is empty —
// belt reads no other tool's config to fill it.
func TestLoadFromInternalNamesAbsentMeansEmpty(t *testing.T) {
	dir := t.TempDir()
	p := fixturePaths(dir)
	writeFile(t, dir, "belt.yaml", "guards:\n  git-push-main:\n    enabled: true\n")

	cfg := LoadFrom(p)
	if len(cfg.Names.BlockedWords)+len(cfg.Names.WorkspaceDirs) != 0 {
		t.Errorf("names = %+v, want empty", cfg.Names)
	}
}

// TestLoadFromClaudeSettingsGate pins the claude_settings gate
// (docs/adr/0010): the deny lists are read by default and skipped entirely
// when enabled is false.
func TestLoadFromClaudeSettingsGate(t *testing.T) {
	settings := `{"permissions": {"deny": ["Bash(kubectl delete:*)"]}}`
	tests := []struct {
		name string
		belt string
		want int
	}{
		{"default reads", "", 1},
		{"explicit true reads", "claude_settings:\n  enabled: true\n", 1},
		{"disabled skips", "claude_settings:\n  enabled: false\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			p := fixturePaths(dir)
			if tt.belt != "" {
				writeFile(t, dir, "belt.yaml", tt.belt)
			}
			writeFile(t, dir, "settings.json", settings)

			cfg := LoadFrom(p)
			if len(cfg.ClaudeDeny) != tt.want {
				t.Errorf("ClaudeDeny = %v, want %d patterns", cfg.ClaudeDeny, tt.want)
			}
			if got, want := cfg.ClaudeSettings.ReadEnabled(), tt.want == 1; got != want {
				t.Errorf("ReadEnabled() = %v, want %v", got, want)
			}
		})
	}
}

func TestExcludesPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tests := []struct {
		name string
		excl string
		path string
		want bool
	}{
		{"tilde prefix match", "~/notes", filepath.Join(home, "notes", "a.md"), true},
		{"tilde exact dir", "~/notes", filepath.Join(home, "notes"), true},
		{"tilde no partial dir", "~/notes", filepath.Join(home, "notesx", "a.md"), false},
		{"tilde trailing slash", "~/notes/", filepath.Join(home, "notes", "a.md"), true},
		{"absolute prefix", "/tmp/trusted", "/tmp/trusted/run.sh", true},
		{"absolute non-prefix", "/tmp/trusted", "/opt/tmp/trusted/run.sh", false},
		{"substring", "recipes/belt/", "/x/recipes/belt/recipe.toml", true},
		{"substring miss", "recipes/belt/", "/x/recipes/csl/recipe.toml", false},
		{"empty entry", "", "/anything", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := Toggle{ExcludePaths: []string{tt.excl}}
			if got := tg.ExcludesPath(tt.path); got != tt.want {
				t.Errorf("ExcludesPath(%q) with %q = %v, want %v", tt.path, tt.excl, got, tt.want)
			}
		})
	}
}
