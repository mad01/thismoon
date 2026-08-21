package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

func gitCmd(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd
}

func newWriteGuard(t *testing.T, remote string, guardCfg config.InternalNames) *WriteInternalNames {
	t.Helper()
	g := NewWriteInternalNames(config.Config{Names: guardCfg})
	g.remoteURL = func(dir string) string { return remote }
	return g
}

func TestWriteInternalNames(t *testing.T) {
	guardCfg := config.InternalNames{
		BlockedWords: []string{"internalco"},
		Allowlist:    []string{"grpc/grpc-go"},
	}
	tests := []struct {
		name     string
		remote   string
		content  string
		wantDeny bool
	}{
		{"public repo with blocked word", "git@github.com:mad01/dotfiles.git", "uses internalco tooling", true},
		{"public repo clean content", "git@github.com:mad01/dotfiles.git", "nothing to see", false},
		{"public repo case-insensitive", "git@github.com:mad01/dotfiles.git", "InternalCo rocks", true},
		{"public repo substring no match", "git@github.com:mad01/dotfiles.git", "internalcoish is fine", false},
		{"internal host allowed", "git@git.internal.example:org/repo.git", "internalco everywhere", false},
		{"no remote allowed", "", "internalco everywhere", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newWriteGuard(t, tt.remote, guardCfg)
			d := g.Check(Input{Event: EventWrite, FilePath: "/repo/file.md", Content: tt.content})
			if (d != nil) != tt.wantDeny {
				t.Errorf("denial = %v, wantDeny %v", d, tt.wantDeny)
			}
			if d != nil && !strings.Contains(d.Reason, WriteInternalNamesID) {
				t.Errorf("reason missing guard id: %q", d.Reason)
			}
		})
	}
}

// fakeRepo creates a directory with an empty .git so repofind treats it as a
// repository; with no remote config the org/repo name falls back to the
// filesystem path.
func fakeRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestWriteInternalNamesWorkspaceDirs(t *testing.T) {
	workspace := t.TempDir()
	fakeRepo(t, filepath.Join(workspace, "secret-infra-repo"))
	fakeRepo(t, filepath.Join(workspace, ".hidden"))
	if err := os.Mkdir(filepath.Join(workspace, "plain-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	guardCfg := config.InternalNames{WorkspaceDirs: []string{workspace}}
	g := newWriteGuard(t, "git@github.com:mad01/public.git", guardCfg)

	if d := g.Check(Input{Event: EventWrite, FilePath: "/f.md", Content: "deploy secret-infra-repo now"}); d == nil {
		t.Error("expected denial for workspace-derived repo name")
	}
	if d := g.Check(Input{Event: EventWrite, FilePath: "/f.md", Content: "mention .hidden dir"}); d != nil {
		t.Errorf("hidden dirs should not become blocked names: %v", d)
	}
	if d := g.Check(Input{Event: EventWrite, FilePath: "/f.md", Content: "mention plain-dir here"}); d != nil {
		t.Errorf("non-repo dirs should not become blocked names: %v", d)
	}
}

func TestBlockedNamesNestedRepoSplitsOrgAndRepo(t *testing.T) {
	workspace := t.TempDir()
	fakeRepo(t, filepath.Join(workspace, "secretorg", "secret-repo"))

	names := BlockedNames(config.InternalNames{WorkspaceDirs: []string{workspace}})
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["secretorg"] || !found["secret-repo"] {
		t.Errorf("expected separate org and repo names, got %v", names)
	}
	if found["secretorg/secret-repo"] {
		t.Errorf("combined org/repo must not be a name, got %v", names)
	}
}

func TestBlockedNamesAllowlistDropsSegment(t *testing.T) {
	workspace := t.TempDir()
	fakeRepo(t, filepath.Join(workspace, "secretorg", "secret-repo"))

	names := BlockedNames(config.InternalNames{
		WorkspaceDirs: []string{workspace},
		Allowlist:     []string{"secretorg"},
	})
	for _, n := range names {
		if n == "secretorg" {
			t.Errorf("allowlisted org must be dropped, got %v", names)
		}
	}
}

func TestWriteInternalNamesAllowlist(t *testing.T) {
	guardCfg := config.InternalNames{
		BlockedWords: []string{"grpc-go"},
		Allowlist:    []string{"grpc/grpc-go"},
	}
	g := newWriteGuard(t, "git@github.com:mad01/public.git", guardCfg)
	if d := g.Check(Input{Event: EventWrite, FilePath: "/f.md", Content: "vendored grpc-go"}); d != nil {
		t.Errorf("allowlisted name should not deny: %v", d)
	}
}

func TestWriteInternalNamesExcludePaths(t *testing.T) {
	guardCfg := config.InternalNames{BlockedWords: []string{"internalco"}}
	g := NewWriteInternalNames(config.Config{
		Names: guardCfg,
		Guards: map[string]config.Toggle{
			WriteInternalNamesID: {ExcludePaths: []string{"recipes/ai-global-config/"}},
		},
	})
	g.remoteURL = func(dir string) string { return "git@github.com:mad01/dotfiles.git" }

	excluded := Input{Event: EventWrite, FilePath: "/repo/recipes/ai-global-config/CLAUDE.md", Content: "internalco"}
	if d := g.Check(excluded); d != nil {
		t.Errorf("excluded path should not deny: %v", d)
	}
	included := Input{Event: EventWrite, FilePath: "/repo/other/file.md", Content: "internalco"}
	if d := g.Check(included); d == nil {
		t.Error("non-excluded path should deny")
	}
}

func TestWriteInternalNamesAllowRepos(t *testing.T) {
	guardCfg := config.InternalNames{BlockedWords: []string{"internalco"}}
	g := NewWriteInternalNames(config.Config{
		Names: guardCfg,
		Guards: map[string]config.Toggle{
			WriteInternalNamesID: {AllowRepos: []string{"github.com/example/internal-overlay"}},
		},
	})

	// Allowlisted repo: internal names permitted despite a github.com remote,
	// matched by canonical identity across both SSH and HTTPS remote forms.
	for _, remote := range []string{
		"git@github.com:example/internal-overlay.git",
		"https://github.com/example/internal-overlay.git",
	} {
		g.remoteURL = func(string) string { return remote }
		in := Input{Event: EventWrite, FilePath: "/repo/f.md", Content: "internalco"}
		if d := g.Check(in); d != nil {
			t.Errorf("allowlisted repo %q should not deny: %v", remote, d)
		}
	}

	// A different github.com repo stays fail-closed.
	g.remoteURL = func(string) string { return "git@github.com:mad01/dotfiles.git" }
	if d := g.Check(Input{Event: EventWrite, FilePath: "/repo/f.md", Content: "internalco"}); d == nil {
		t.Error("non-allowlisted repo should still deny")
	}
}

func TestWriteInternalNamesAllowReposByProfile(t *testing.T) {
	newGuard := func(profiles []string) *WriteInternalNames {
		g := NewWriteInternalNames(config.Config{
			Names:    config.InternalNames{BlockedWords: []string{"internalco"}},
			Profiles: profiles,
			Guards: map[string]config.Toggle{
				WriteInternalNamesID: {
					AllowReposByProfile: map[string][]string{
						"personal": {"github.com/example/memory-store"},
					},
				},
			},
		})
		g.remoteURL = func(string) string { return "git@github.com:example/memory-store.git" }
		return g
	}
	in := Input{Event: EventWrite, FilePath: "/repo/f.md", Content: "internalco"}

	// A machine carrying the profile gets the allowlist entry.
	if d := newGuard([]string{"personal"}).Check(in); d != nil {
		t.Errorf("profile-scoped allow on a personal machine should not deny: %v", d)
	}
	// A machine without the profile stays fail-closed — the same config
	// denies there, forcing the fact into the profile's dedicated store.
	if d := newGuard([]string{"work"}).Check(in); d == nil {
		t.Error("profile-scoped allow must not apply on a work machine")
	}
	if d := newGuard(nil).Check(in); d == nil {
		t.Error("profile-scoped allow must not apply when the profile is unknown")
	}
}

func TestWriteInternalNamesEmptyContent(t *testing.T) {
	g := newWriteGuard(t, "git@github.com:mad01/public.git", config.InternalNames{BlockedWords: []string{"internalco"}})
	if d := g.Check(Input{Event: EventWrite, FilePath: "/f.md", Content: ""}); d != nil {
		t.Errorf("empty content should never deny: %v", d)
	}
}

func TestWriteInternalNamesNewDirectory(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := gitCmd(dir, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("remote", "add", "origin", "git@github.com:mad01/example.git")

	// Real remote lookup: the target's parent dirs do not exist yet (a Write
	// creates them), so the guard must walk up to the repo to find the remote.
	g := NewWriteInternalNames(config.Config{
		Names: config.InternalNames{BlockedWords: []string{"internalco"}},
	})
	target := filepath.Join(dir, "brand", "new", "doc.md")
	if d := g.Check(Input{Event: EventWrite, FilePath: target, Content: "internalco"}); d == nil {
		t.Error("write into a not-yet-created directory must still hit the public-repo check")
	}
}

func TestWriteInternalNamesTruncatesHits(t *testing.T) {
	words := []string{"aaaa", "bbbb", "cccc", "dddd", "eeee", "ffff", "gggg"}
	g := newWriteGuard(t, "git@github.com:mad01/public.git", config.InternalNames{BlockedWords: words})
	d := g.Check(Input{Event: EventWrite, FilePath: "/f.md", Content: strings.Join(words, " ")})
	if d == nil {
		t.Fatal("expected denial")
	}
	if !strings.Contains(d.Reason, "+2 more") {
		t.Errorf("reason should mark truncated hits: %q", d.Reason)
	}
}

func TestGitRemoteURLRealRepo(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := gitCmd(dir, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("remote", "add", "origin", "git@github.com:mad01/example.git")
	if got := gitRemoteURL(dir); got != "git@github.com:mad01/example.git" {
		t.Errorf("gitRemoteURL = %q", got)
	}
	if got := gitRemoteURL(t.TempDir()); got != "" {
		t.Errorf("gitRemoteURL outside repo = %q, want empty", got)
	}
}
