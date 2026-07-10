package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/tools/suspenders/internal/config"
	"github.com/mad01/thismoon/tools/suspenders/internal/scanner"
)

// initTestRepo creates a git repo at path so rev-parse resolves a top-level.
func initTestRepo(t *testing.T, path string) {
	t.Helper()
	if err := exec.Command("git", "init", path).Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
}

func TestRepoConfigPath(t *testing.T) {
	t.Run("finds config at scan root", func(t *testing.T) {
		root := t.TempDir()
		p := filepath.Join(root, ".suspenders.yaml")
		if err := os.WriteFile(p, []byte("rules: []\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := repoConfigPath(root); got != p {
			t.Errorf("got %q, want %q", got, p)
		}
	})

	t.Run("falls back to git top-level from a subdirectory", func(t *testing.T) {
		root := t.TempDir()
		initTestRepo(t, root)
		p := filepath.Join(root, ".suspenders.yaml")
		if err := os.WriteFile(p, []byte("rules: []\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		sub := filepath.Join(root, "docs", "guide")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		got := repoConfigPath(sub)
		// macOS tempdirs live under /private symlinks; compare resolved paths.
		wantResolved, _ := filepath.EvalSymlinks(p)
		gotResolved, _ := filepath.EvalSymlinks(got)
		if gotResolved != wantResolved {
			t.Errorf("got %q, want %q", got, p)
		}
	})

	t.Run("accepts .yml extension", func(t *testing.T) {
		root := t.TempDir()
		p := filepath.Join(root, ".suspenders.yml")
		if err := os.WriteFile(p, []byte("rules: []\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := repoConfigPath(root); got != p {
			t.Errorf("got %q, want %q", got, p)
		}
	})

	t.Run("empty when no config exists", func(t *testing.T) {
		if got := repoConfigPath(t.TempDir()); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestGuardConfigFor(t *testing.T) {
	cfg := &config.Config{
		Guard: config.GuardConfig{
			Enabled:      true,
			BlockedWords: []string{"acmecorp"},
			Allowlist:    []string{"global-safe"},
		},
	}

	t.Run("appends per-repo guard overrides", func(t *testing.T) {
		root := t.TempDir()
		repoCfg := "guard:\n  allowlist:\n    - monitoring\n  blocked_words:\n    - repo-secret-name\n"
		if err := os.WriteFile(filepath.Join(root, ".suspenders.yaml"), []byte(repoCfg), 0o644); err != nil {
			t.Fatal(err)
		}

		got := guardConfigFor(root, cfg)
		wantAllow := []string{"global-safe", "monitoring"}
		wantBlocked := []string{"acmecorp", "repo-secret-name"}
		if len(got.Allowlist) != 2 || got.Allowlist[0] != wantAllow[0] ||
			got.Allowlist[1] != wantAllow[1] {
			t.Errorf("allowlist: got %v, want %v", got.Allowlist, wantAllow)
		}
		if len(got.BlockedWords) != 2 || got.BlockedWords[1] != wantBlocked[1] {
			t.Errorf("blocked_words: got %v, want %v", got.BlockedWords, wantBlocked)
		}
		// The global config's slices must be untouched.
		if len(cfg.Guard.Allowlist) != 1 || len(cfg.Guard.BlockedWords) != 1 {
			t.Errorf("global config mutated: %v %v", cfg.Guard.Allowlist, cfg.Guard.BlockedWords)
		}
	})

	t.Run("no per-repo config returns global unchanged", func(t *testing.T) {
		got := guardConfigFor(t.TempDir(), cfg)
		if len(got.Allowlist) != 1 || got.Allowlist[0] != "global-safe" {
			t.Errorf("got %v, want [global-safe]", got.Allowlist)
		}
	})
}

// TestApplyExcludeRules covers the rule-filter shared by `scan` and the
// pre-commit hook: excluded IDs are dropped, others kept in order, and an
// empty exclude list is a no-op. IDs are arbitrary labels, not secrets.
func TestGuardExempt(t *testing.T) {
	// A repo with no origin remote resolves its name as parentdir/repodir,
	// so a repo at <tmp>/myorg/dotfiles is named "myorg/dotfiles".
	parent := t.TempDir()
	root := filepath.Join(parent, "myorg", "dotfiles")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	initTestRepo(t, root)

	tests := []struct {
		name string
		cfg  config.Config
		want bool
	}{
		{
			name: "exclude matches repo name exactly",
			cfg:  config.Config{Exclude: []string{"myorg/dotfiles"}},
			want: true,
		},
		{
			name: "exclude matches repo name by glob",
			cfg:  config.Config{Exclude: []string{"*/dotfiles"}},
			want: true,
		},
		{
			name: "exclude for a different repo does not exempt",
			cfg:  config.Config{Exclude: []string{"myorg/other"}},
			want: false,
		},
		{
			name: "no excludes and no workspace dirs",
			cfg:  config.Config{},
			want: false,
		},
		{
			name: "repo inside a workspace dir",
			cfg: config.Config{
				Guard: config.GuardConfig{WorkspaceDirs: []string{parent}},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := guardExempt(root, &tt.cfg); got != tt.want {
				t.Errorf("guardExempt = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyExcludeRules(t *testing.T) {
	rules := []scanner.Rule{
		{ID: "alpha"},
		{ID: "beta"},
		{ID: "gamma"},
	}

	tests := []struct {
		name    string
		exclude []string
		want    []string
	}{
		{name: "no excludes keeps all", exclude: nil, want: []string{"alpha", "beta", "gamma"}},
		{name: "drops one", exclude: []string{"beta"}, want: []string{"alpha", "gamma"}},
		{name: "drops several", exclude: []string{"alpha", "gamma"}, want: []string{"beta"}},
		{
			name:    "unknown id is ignored",
			exclude: []string{"missing"},
			want:    []string{"alpha", "beta", "gamma"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Copy so the in-place filter never mutates the shared fixture.
			in := append([]scanner.Rule{}, rules...)
			got := applyExcludeRules(in, tt.exclude)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d rules, want %d (%v)", len(got), len(tt.want), tt.want)
			}
			for i, id := range tt.want {
				if got[i].ID != id {
					t.Errorf("rule %d: got %q, want %q", i, got[i].ID, id)
				}
			}
		})
	}
}
