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

func TestLoadProfiles(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"work profile", `profiles = ["work"]`, []string{"work"}},
		{"multiple", `profiles = ["personal", "laptop"]`, []string{"personal", "laptop"}},
		{"empty file", ``, nil},
		{"malformed", `profiles = [`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, t.TempDir(), "config.local.toml", tt.content)
			got := LoadProfiles(path)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestLoadProfilesMissingFile(t *testing.T) {
	if got := LoadProfiles(filepath.Join(t.TempDir(), "nope.toml")); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestLoadSuspendersGuard(t *testing.T) {
	content := `
guard:
  enabled: true
  workspace_dirs:
    - ~/workspace
  blocked_words:
    - internalco
  allowlist:
    - grpc/grpc-go
`
	path := writeFile(t, t.TempDir(), "config.yaml", content)
	got := LoadSuspendersGuard(path)
	if len(got.BlockedWords) != 1 || got.BlockedWords[0] != "internalco" {
		t.Errorf("blocked_words = %v", got.BlockedWords)
	}
	if len(got.WorkspaceDirs) != 1 || got.WorkspaceDirs[0] != "~/workspace" {
		t.Errorf("workspace_dirs = %v", got.WorkspaceDirs)
	}
	if len(got.Allowlist) != 1 {
		t.Errorf("allowlist = %v", got.Allowlist)
	}
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

func TestLoadTogglesExtraPatterns(t *testing.T) {
	content := `
[guards.script-deny-list]
enabled = true
extra_patterns = ["rm -rf", "rm -fr"]
`
	path := writeFile(t, t.TempDir(), "config.toml", content)
	got, _ := loadToggles(path)
	patterns := got["script-deny-list"].ExtraPatterns
	if len(patterns) != 2 || patterns[0] != "rm -rf" || patterns[1] != "rm -fr" {
		t.Errorf("extra_patterns = %v", patterns)
	}
}

func TestLoadTogglesAllowRepos(t *testing.T) {
	content := `
[guards.git-push-main]
allow_repos = ["github.com/mad01/dotfiles"]
`
	path := writeFile(t, t.TempDir(), "config.toml", content)
	got, _ := loadToggles(path)
	repos := got["git-push-main"].AllowRepos
	if len(repos) != 1 || repos[0] != "github.com/mad01/dotfiles" {
		t.Errorf("allow_repos = %v", repos)
	}
}

func TestLoadTogglesHintsSection(t *testing.T) {
	content := `
[guards.git-push-main]
enabled = true

[hints.keep-assertions]
enabled = false
`
	path := writeFile(t, t.TempDir(), "config.toml", content)
	guards, hints := loadToggles(path)
	if guards["git-push-main"].Enabled == nil || !*guards["git-push-main"].Enabled {
		t.Error("guards section did not survive adding hints")
	}
	if hints["keep-assertions"].Enabled == nil || *hints["keep-assertions"].Enabled {
		t.Error("hints.keep-assertions should have decoded as disabled")
	}
}

func TestHintEnabledDefaultsOn(t *testing.T) {
	cfg := Config{}
	if !cfg.HintEnabled("keep-assertions") {
		t.Error("a hint with no config entry should default to enabled")
	}
	off := false
	cfg = Config{Hints: map[string]Toggle{"keep-assertions": {Enabled: &off}}}
	if cfg.HintEnabled("keep-assertions") {
		t.Error("an explicitly disabled hint should report disabled")
	}
	if !cfg.HintEnabled("prefer-csl") {
		t.Error("disabling one hint must not disable the others")
	}
}

func TestHasProfile(t *testing.T) {
	cfg := Config{Profiles: []string{"work"}}
	if !cfg.HasProfile("work") {
		t.Error("expected work profile")
	}
	if cfg.HasProfile("personal") {
		t.Error("did not expect personal profile")
	}
}
