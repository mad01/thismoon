package config

import (
	"path/filepath"
	"testing"
)

func TestLoadRuleSections(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", `
git_identity:
  - repos:
      - github.com/mad01/*
    email: personal@example.com
    mode: hard
  - email: work@example.com

commit_guards:
  - repos:
      - github.com/mad01/*
    always_allow:
      - github.com/mad01/dotfiles
    block_hours: "09:00-17:00"
    block_days: [mon, tue, wed, thu, fri]
    mode: soft
    override: vacation

custom_guards:
  check-branch:
    enabled: false
    event: bash
    command: [branch-lint, check]
    match: git commit
    mode: soft
`)
	cfg := LoadFrom(Paths{
		BeltYAML: filepath.Join(dir, "config.yaml"),
		BeltTOML: filepath.Join(dir, "config.toml"),
	})

	if len(cfg.GitIdentity) != 2 {
		t.Fatalf("git_identity rules = %d, want 2", len(cfg.GitIdentity))
	}
	first := cfg.GitIdentity[0]
	if first.Email != "personal@example.com" || first.Soft() || len(first.Repos) != 1 {
		t.Errorf("first identity rule = %+v", first)
	}
	if cfg.GitIdentity[1].Email != "work@example.com" {
		t.Errorf("second identity rule email = %q", cfg.GitIdentity[1].Email)
	}

	if len(cfg.CommitGuards) != 1 {
		t.Fatalf("commit_guards rules = %d, want 1", len(cfg.CommitGuards))
	}
	cg := cfg.CommitGuards[0]
	if cg.BlockHours != "09:00-17:00" || cg.Hard() || cg.Override != "vacation" ||
		len(cg.BlockDays) != 5 || len(cg.AlwaysAllow) != 1 {
		t.Errorf("commit guard rule = %+v", cg)
	}

	custom, ok := cfg.CustomGuards["check-branch"]
	if !ok {
		t.Fatal("custom guard check-branch not loaded")
	}
	if custom.Event != "bash" || !custom.Soft() || custom.Match != "git commit" ||
		len(custom.Command) != 2 || custom.Command[0] != "branch-lint" {
		t.Errorf("custom guard = %+v", custom)
	}
	if cfg.GuardEnabled("check-branch") {
		t.Error("enabled: false custom guard reported enabled")
	}
}

func TestRepoMatches(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		repo     string
		want     bool
	}{
		{"exact match", []string{"github.com/mad01/thismoon"}, "github.com/mad01/thismoon", true},
		{"exact mismatch", []string{"github.com/mad01/thismoon"}, "github.com/mad01/ralph", false},
		{"org wildcard matches", []string{"github.com/mad01/*"}, "github.com/mad01/ralph", true},
		{"org wildcard other org", []string{"github.com/mad01/*"}, "github.com/other/ralph", false},
		{"wildcard needs the slash", []string{"github.com/mad01/*"}, "github.com/mad01ish/repo", false},
		{"empty repo never matches", []string{"github.com/mad01/*"}, "", false},
		{"empty patterns never match", nil, "github.com/mad01/thismoon", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RepoMatches(tt.patterns, tt.repo); got != tt.want {
				t.Errorf("RepoMatches(%v, %q) = %v, want %v", tt.patterns, tt.repo, got, tt.want)
			}
		})
	}
}
