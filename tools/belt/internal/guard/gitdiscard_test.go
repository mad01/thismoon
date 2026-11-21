package guard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// discardFixture describes the repo state the guard sees: the canonical repo
// per directory and whether its tree is dirty. A directory missing from
// dirty reads as clean; one listed in broken makes git status fail.
type discardFixture struct {
	repo   string
	dirty  map[string]bool
	broken map[string]bool
	// ignoredDirty marks directories whose only changes are ignored files,
	// visible to `git status --porcelain --ignored` alone.
	ignoredDirty map[string]bool
}

// fakeTrees are the revs fakeIsTree resolves in any directory belt can name.
var fakeTrees = wordSet("main HEAD @{-1} origin/main")

// fakeIsTree stands in for gitIsTree: an empty dir resolves nothing.
func fakeIsTree(_ context.Context, dir, rev string) (bool, error) {
	return dir != "" && fakeTrees[rev], nil
}

// newDiscardGuard builds the guard with injected resolvers and a warning
// recorder, so no test shells out to git or dials the events service. The
// nothing-lost proof never holds here: a dirty fixture tree has unique work.
func newDiscardGuard(cfg config.Config, f discardFixture, warned *[]string) *GitDiscard {
	g := NewGitDiscard(cfg)
	g.resolveRepo = func(string) string { return f.repo }
	g.isTree = fakeIsTree
	g.prove = func(gitInvocation) error { return errors.New("`file.go` holds unique work") }
	g.status = func(dir string, extra ...string) (string, bool) {
		if f.broken[dir] {
			return "", false
		}
		if f.dirty[dir] {
			return " M file.go", true
		}
		if len(extra) > 0 && f.ignoredDirty[dir] {
			return "!! build/", true
		}
		return "", true
	}
	g.emit = func(_, _, _, message string, _ map[string]string) {
		*warned = append(*warned, message)
	}
	return g
}

func TestGitDiscardCommands(t *testing.T) {
	dirty := discardFixture{
		repo:  "github.com/mad01/thismoon",
		dirty: map[string]bool{"/repo": true},
	}
	tests := []struct {
		command  string
		wantDeny bool
	}{
		{"git stash", true},
		{"git stash -u", true},
		{"git stash push -m wip", true},
		{"git stash save wip", true},
		{"git stash list", false},
		{"git stash show -p", false},
		{"git stash apply", false},
		{"git stash pop", false},
		{"git stash branch tmp", false},
		{"git stash export --print", false},
		{"git stash import abc123", false},
		{"git reset --hard", true},
		{"git reset --hard origin/main", true},
		{"git reset --soft HEAD~1", false},
		{"git reset HEAD file.go", false},
		{"git checkout -- file.go", true},
		{"git checkout .", true},
		{"git checkout main -- file.go", true},
		{"git checkout -f main", true},
		{"git checkout -qf main", true},
		{"git checkout -fq main", true},
		{"git checkout -fb newb", true},
		{"git checkout -bf", false}, // a branch named f
		{"git checkout -b f", false},
		{"git checkout --forc main", true},
		{"git checkout --f main", true},
		{"git checkout --no-force main", false},
		{"git checkout -p", true},
		{"git checkout -qp", true},
		{"git checkout --patch", true},
		{"git checkout -p HEAD", true},
		{"git checkout -p --", true},
		{"git checkout -p main --", true},
		{"git checkout main -p", true},
		{"yes | git checkout -p", true},
		{"git checkout -bp", false}, // a branch named p
		{"git checkout main", false},
		{"git checkout -b feat", false},
		{"git checkout -b feat origin/main", false},
		{"git checkout -qb feat main", false},
		{"git checkout -", false},
		{"timeout 5 git checkout main", false},
		{"git checkout file.go", true},
		{"git checkout file.go -q", true},
		{"git checkout HEAD file.go", true},
		{"git checkout - file.go", true},
		{"git checkout -b feat file.go", true},
		{"echo file.go | xargs git checkout --", true},
		{"echo file.go | xargs git checkout", true},
		{"echo main | xargs git checkout", true},
		{"git checkout --pathspec-from-file=list", true},
		{"git checkout --pathspec-from list main", true},
		{"git switch --discard-changes main", true},
		{"git switch --disc main", true},
		{"git switch -qf main", true},
		{"git switch -fc newb", true},
		{"git switch -cf", false}, // a branch named f
		{"git switch -c f", false},
		{"git switch main", false},
		{"git restore file.go", true},
		{"git restore --staged file.go", false},
		{"git restore -S file.go", false},
		{"git restore --staged --worktree file.go", true},
		{"git restore --source=HEAD~1 file.go", true},
		{"git clean -f", true},
		{"git clean -fdx", true},
		{"git clean --force", true},
		{"git clean -n", false},
		{"git clean -fn", false},
		{"git clean -d", false},
		{"git status", false},
		{"git commit -m 'git stash'", false},
		{"echo git reset --hard", true}, // token parser: a bare git anywhere in the segment counts
		{"make test && git stash", true},
		{"git -C /repo stash", true},
		{"cd /repo && git reset --hard", true},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(config.Config{}, dirty, &warned)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			if (d != nil) != tt.wantDeny {
				t.Fatalf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
			if d != nil &&
				!strings.HasPrefix(d.Reason, "belt[git-discard]: this discards uncommitted work") {
				t.Errorf("reason = %q, want the git-discard prefix and message", d.Reason)
			}
		})
	}
}

// TestGitDiscardResetCleanOptions pins how reset and clean options are read:
// as git's parse-options reads them, with an option belt does not know, or
// a word the shell may still turn into one, counted as a discard.
func TestGitDiscardResetCleanOptions(t *testing.T) {
	dirty := discardFixture{
		repo:  "github.com/mad01/thismoon",
		dirty: map[string]bool{"/repo": true},
	}
	tests := []struct {
		command  string
		wantDeny bool
	}{
		// Any prefix of --hard is --hard, wherever it stands.
		{"git reset --h", true},
		{"git reset --ha", true},
		{"git reset --har", true},
		{"git reset --ha origin/main", true},
		{"git reset HEAD --har", true},
		{"git reset -q --ha", true},
		{"git reset --soft --hard", true},
		{"git reset --hard=x", true}, // git refuses it; counting it errs toward a deny
		// An optional value never takes the next word; a needed one takes it
		// even when it is --.
		{"git reset --recurse-submodules --hard", true},
		{"git reset --pathspec-from-file -- --hard", true},
		// An option belt does not know, or one git finds ambiguous, fails closed.
		{"git reset --frobnicate", true},
		{"git reset -x", true},
		{"git reset --m", true},
		{"git reset --no-hard", true},
		{"git reset --ha{r,}d", true},
		// --end-of-options ends the options as git reads it, so what follows
		// is an operand; a later -- still starts the paths.
		{"git reset --end-of-options HEAD~1", false},
		{"git reset --end-of-options --hard", false},
		{"git reset --hard --end-of-options HEAD", true},
		{"git clean -n --end-of-options -f", false},
		{"git clean -f --end-of-options x", true},
		{"git reset --end-of-options $X", true},
		// A word the shell may still turn into an option.
		{"git reset $MODE", true},
		{`git reset "$MODE"`, true},
		{"git reset $(echo --hard)", true},
		{"git reset {--hard,}", true},
		{"git reset *", true},
		{"git reset ''*", true},
		{`git reset '-hard;'`, true},
		{"echo --hard | xargs git reset", true},
		// Resets that keep the worktree.
		{"git reset", false},
		{"git reset --mixed", false},
		{"git reset --mi HEAD~1", false},
		{"git reset --soft HEAD~1", false},
		{"git reset --so 'HEAD@{1}'", false},
		{"git reset --keep HEAD~1", false},
		{"git reset -q HEAD~1", false},
		{"git reset -p", false},
		{"git reset -U3 -p", false},
		{"git reset --no-refresh HEAD~1", false},
		{"git reset --no-quiet", false},
		{"git reset --recurse-submodules=no HEAD~1", false},
		{"git reset --pathspec-from-file list", false},
		{"git reset -- --hard", false}, // a path after --
		{"git reset HEAD -- file.go", false},
		{"git reset src/*.go", false},
		// Any prefix of --force is --force, alone or in a bundle.
		{"git clean --f", true},
		{"git clean --fo", true},
		{"git clean --for", true},
		{"git clean -d --forc", true},
		{"git clean --forc -x", true},
		{"git clean -fd", true},
		{"git clean -xdf", true},
		{"git clean -dfX", true},
		{"git clean --force=x", true}, // git refuses it; counting it errs toward a deny
		// -e takes its pattern from the rest of the bundle or the next word,
		// so a -n there is no dry run.
		{"git clean -fen", true},
		{"git clean -e -n -f", true},
		{"git clean --exclude -n -f", true},
		// The last -n or --no-dry-run decides; a -n after -- is a path.
		{"git clean -n --no-dry-run -f", true},
		{"git clean -n --no-d -f", true},
		{"git clean -f -- -n", true},
		// -i deletes what the user picks, without -f.
		{"git clean -i", true},
		{"git clean --inter", true},
		// Fails closed, as reset does.
		{"git clean -n --frobnicate", true},
		{"git clean -nz", true},
		{"git clean -n --no-exclude", true},
		{"git clean -n $X", true},
		{"git clean -n *", true},
		{"echo -f | xargs git clean", true},
		// Cleans that delete nothing.
		{"git clean -n", false},
		{"git clean --for -n", false},
		{"git clean -n --forc", false},
		{"git clean --d -f", false}, // --d is --dry-run
		{"git clean -f --dry-run", false},
		{"git clean -fdxn", false},
		{"git clean -d", false},
		{"git clean -x", false},
		{"git clean -f --no-dry-run -n", false},
		{"git clean -nd -e '*.log'", false},
		{"git clean -n --exclude=*.log", false},
		{"git clean -nq", false},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(config.Config{}, dirty, &warned)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			if (d != nil) != tt.wantDeny {
				t.Fatalf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
		})
	}
}

// TestGitDiscardStashDrop pins that a stash drop or clear is denied on a
// clean tree and a dirty one alike, with its own reason, in any form git
// reads as one or the shell may turn into one. A push form keeps the
// clean-tree rule (pushes), and the subcommands that keep the entries' work
// pass.
func TestGitDiscardStashDrop(t *testing.T) {
	const repo = "github.com/mad01/thismoon"
	tests := []struct {
		command string
		drops   bool
		pushes  bool
	}{
		{command: "git stash drop", drops: true},
		{command: "git stash drop -q", drops: true},
		{command: "git stash drop --quiet", drops: true},
		{command: "git stash drop --no-quiet", drops: true},
		{command: "git stash drop 'stash@{1}'", drops: true},
		{command: "git stash drop 1", drops: true},
		{command: "git stash drop -- 'stash@{0}'", drops: true},
		{command: "git stash clear", drops: true},
		{command: "git stash clear --", drops: true},
		{command: "git stash 'drop'", drops: true},
		{command: `git stash dr\op`, drops: true},
		{command: "git -C /other stash drop", drops: true},
		{command: "git --no-pager stash clear", drops: true},
		{command: "cd /other && git stash drop -q", drops: true},
		{command: "git stash list && git stash drop", drops: true},
		{command: "(git stash clear)", drops: true},
		{command: "sh -c 'git stash clear'", drops: true},
		// Through an alias the command sets.
		{command: "git -c alias.x='stash clear' x", drops: true},
		{command: "git config alias.nuke 'stash drop' && git nuke", drops: true},
		{command: "git -c alias.st=stash st drop", drops: true},
		// A command in front of git that may hand it the subcommand or the
		// entry.
		{command: "echo clear | xargs git stash", drops: true},
		{command: "echo 'stash@{0}' | xargs git stash drop", drops: true},
		// A first word the shell may still turn into drop or clear.
		{command: "git stash $X", drops: true},
		{command: `git stash "$X"`, drops: true},
		{command: "git stash $(echo clear)", drops: true},
		{command: "git stash d*", drops: true},
		{command: "git stash dr{op,}", drops: true},
		// The subcommands that keep the entries' work.
		{command: "git stash list"},
		{command: "git stash show -p"},
		{command: "git stash show -p 'stash@{0}'"},
		{command: "git stash apply"},
		{command: "git stash apply 'stash@{1}'"},
		{command: "git stash pop"},
		{command: "git stash pop --index"},
		{command: "git stash branch x"},
		{command: "git stash branch x 'stash@{0}'"},
		{command: "git stash create"},
		// Text no shell runs.
		{command: "git commit -m 'git stash drop'"},
		{command: "ls # git stash drop"},
		{command: "cat <<'EOF'\ngit stash clear\nEOF"},
		// A push: an option first, so a later drop is a pathspec, a message or
		// an error, and a first word git takes as no subcommand.
		{command: "git stash", pushes: true},
		{command: "git stash -u", pushes: true},
		{command: "git stash -q drop", pushes: true},
		{command: "git stash -- drop", pushes: true},
		{command: "git stash -m drop", pushes: true},
		{command: "git stash push -m clear", pushes: true},
		{command: "echo f | xargs git stash push", pushes: true},
		{command: "git stash dro", pushes: true},
		{command: "git stash Drop", pushes: true},
	}
	fixtures := map[string]discardFixture{
		"clean": {repo: repo},
		"dirty": {repo: repo, dirty: map[string]bool{"/repo": true}},
	}
	for name, fixture := range fixtures {
		for _, tt := range tests {
			t.Run(name+"/"+tt.command, func(t *testing.T) {
				var warned []string
				g := newDiscardGuard(config.Config{}, fixture, &warned)
				d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
				var want string
				switch {
				case tt.drops:
					want = "belt[git-discard]: " + stashDropReason
				case tt.pushes && name == "dirty":
					want = "belt[git-discard]: this discards uncommitted work"
				}
				switch {
				case want == "" && d != nil:
					t.Fatalf("Check(%q) = %q, want allow", tt.command, d.Reason)
				case want != "" && d == nil:
					t.Fatalf("Check(%q) allowed, want %q", tt.command, want)
				case want != "" && !strings.HasPrefix(d.Reason, want):
					t.Errorf("Check(%q) = %q, want %q", tt.command, d.Reason, want)
				}
			})
		}
	}
	if !strings.Contains(stashDropReason, "`git stash list`") ||
		!strings.Contains(stashDropReason, "WIP commit") {
		t.Errorf("stash drop reason %q names no `git stash list` or WIP commit", stashDropReason)
	}
}

func TestGitDiscardDecision(t *testing.T) {
	const repo = "github.com/mad01/thismoon"
	tests := []struct {
		name     string
		cfg      config.Config
		fixture  discardFixture
		command  string
		cwd      string
		wantDeny bool
		wantWarn bool
	}{
		{
			name:    "clean tree allows",
			fixture: discardFixture{repo: repo},
			command: "git reset --hard", cwd: "/repo",
		},
		{
			name:    "dirty tree denies",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: "git reset --hard", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "judged in the git -C directory, not the session cwd",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/other": true}},
			command: "git -C /other stash", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "clean -C target allows despite a dirty session cwd",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: "git -C /other stash", cwd: "/repo",
		},
		{
			name:    "tracked cd places the command",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/other": true}},
			command: "cd /other && git checkout .", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "unresolvable directory denies",
			fixture: discardFixture{repo: repo},
			command: `cd "$(mktemp -d)" && git stash`, cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a checkout operand belt cannot look up reads as a path",
			fixture: discardFixture{repo: repo},
			command: `cd "$(mktemp -d)" && git checkout main`, cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a switch in $PWD on a clean tree allows",
			fixture: discardFixture{repo: repo},
			command: `git -C "$PWD" checkout main`, cwd: "/repo",
		},
		{
			name:    "a switch in $PWD on a dirty tree allows",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: `git -C "${PWD}" checkout main`, cwd: "/repo",
		},
		{
			name:    "a discard in $PWD after a cd is judged in the cd target",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: `cd /other && git -C "$PWD" checkout -- f`, cwd: "/repo",
		},
		{
			name:    "a discard in $PWD is judged there",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: `cd "$PWD" && git checkout -- f`, cwd: "/repo", wantDeny: true,
		},
		{
			name:    "$PWD after an unknown cd stays unknown",
			fixture: discardFixture{repo: repo},
			command: `cd "$(mktemp -d)" && git -C "$PWD" checkout main`, cwd: "/repo",
			wantDeny: true,
		},
		// The outer shell expands $PWD in a double-quoted script before its cd
		// runs, so the discard lands in the dirty session cwd: nested script
		// text keeps $PWD unknown.
		{
			name:    "$PWD in a sh -c script stays unknown",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: `sh -c "cd /other && git -C $PWD checkout -- ."`, cwd: "/repo",
			wantDeny: true,
		},
		{
			name:    "$PWD in a bash -c script stays unknown",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: `bash -c "cd /other; git -C \"$PWD\" checkout -- ."`, cwd: "/repo",
			wantDeny: true,
		},
		{
			name:    "$PWD in an eval stays unknown",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: `eval "cd /other; git -C $PWD checkout -- ."`, cwd: "/repo",
			wantDeny: true,
		},
		{
			name:    "$PWD in a single-quoted sh -c script stays unknown",
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: `sh -c 'cd /other && git -C "$PWD" checkout -- .'`, cwd: "/repo",
			wantDeny: true,
		},
		{
			name:    "git status failure denies",
			fixture: discardFixture{repo: repo, broken: map[string]bool{"/repo": true}},
			command: "git stash", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "clean -x checks ignored files too",
			fixture: discardFixture{repo: repo, ignoredDirty: map[string]bool{"/repo": true}},
			command: "git clean -fdx", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "clean without -x ignores ignored files",
			fixture: discardFixture{repo: repo, ignoredDirty: map[string]bool{"/repo": true}},
			command: "git clean -fd", cwd: "/repo",
		},
		{
			name:    "a prefix of --exclude checks ignored files too",
			fixture: discardFixture{repo: repo, ignoredDirty: map[string]bool{"/repo": true}},
			command: "git clean --fo --ex '!secret.env'", cwd: "/repo", wantDeny: true,
		},
		{
			name: "allow_repos exempts",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {AllowRepos: []string{"github.com/mad01/*"}},
			}},
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: "git stash", cwd: "/repo",
		},
		{
			name: "allow_repos for another repo still denies",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {AllowRepos: []string{"github.com/mad01/dotfiles"}},
			}},
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: "git stash", cwd: "/repo", wantDeny: true,
		},
		{
			name: "soft mode warns instead of denying",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {Mode: config.ModeSoft},
			}},
			fixture: discardFixture{repo: repo, dirty: map[string]bool{"/repo": true}},
			command: "git reset --hard", cwd: "/repo", wantWarn: true,
		},
		{
			name: "soft mode stays quiet on a clean tree",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {Mode: config.ModeSoft},
			}},
			fixture: discardFixture{repo: repo},
			command: "git reset --hard", cwd: "/repo",
		},
		{
			name:    "a stash drop denies on a clean tree",
			fixture: discardFixture{repo: repo},
			command: "git stash drop", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a stash drop in an unresolvable directory denies",
			fixture: discardFixture{repo: repo},
			command: `cd "$(mktemp -d)" && git stash clear`, cwd: "/repo", wantDeny: true,
		},
		{
			name: "allow_repos exempts a stash drop",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {AllowRepos: []string{"github.com/mad01/*"}},
			}},
			fixture: discardFixture{repo: repo},
			command: "git stash clear", cwd: "/repo",
		},
		{
			name: "soft mode warns on a stash drop on a clean tree",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {Mode: config.ModeSoft},
			}},
			fixture: discardFixture{repo: repo},
			command: "git stash drop", cwd: "/repo", wantWarn: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(tt.cfg, tt.fixture, &warned)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: tt.cwd})
			if (d != nil) != tt.wantDeny {
				t.Errorf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
			if (len(warned) > 0) != tt.wantWarn {
				t.Errorf("Check(%q) warnings = %v, wantWarn %v", tt.command, warned, tt.wantWarn)
			}
		})
	}
}

// TestSortCheckoutOperands pins how a checkout without `--` is read: a lone
// tree-ish is a switch, a tree-ish with more operands restores them from it,
// and operands without one are paths restored from the index.
func TestSortCheckoutOperands(t *testing.T) {
	tests := []struct {
		dir, args, want string
	}{
		{dir: "/repo", args: "", want: ""},
		{dir: "/repo", args: "main", want: "main"},
		{dir: "/repo", args: "-q main", want: "-q main"},
		{dir: "/repo", args: "-", want: "-"},
		{dir: "/repo", args: "-b feat", want: "-b feat"},
		{dir: "/repo", args: "-b feat main", want: "-b feat main"},
		{dir: "/repo", args: "-qb feat main", want: "-qb feat main"},
		{dir: "/repo", args: "-bfeat main", want: "-bfeat main"},
		{dir: "/repo", args: "--orphan new", want: "--orphan new"},
		{dir: "/repo", args: "f", want: "-- f"},
		{dir: "/repo", args: ".", want: "-- ."},
		{dir: "/repo", args: "f g", want: "-- f g"},
		{dir: "/repo", args: "f -q", want: "-q -- f"},
		{dir: "/repo", args: "HEAD f g", want: "HEAD -- f g"},
		{dir: "/repo", args: "- f", want: "@{-1} -- f"},
		{dir: "/repo", args: "-b feat f", want: "-b feat -- f"},
		{dir: "/repo", args: "$B", want: "-- $B"},
		{dir: "/repo", args: "main -- f", want: "main -- f"},
		{dir: "/repo", args: "f --", want: "f --"},
		{dir: "", args: "main", want: "-- main"},
		{dir: "/repo", args: "-p", want: "-p -- ."},
		{dir: "/repo", args: "-qp", want: "-qp -- ."},
		{dir: "/repo", args: "-p HEAD", want: "-p HEAD -- ."},
		{dir: "/repo", args: "main --patch", want: "main --patch -- ."},
		{dir: "/repo", args: "-p --", want: "-p -- ."},
		{dir: "/repo", args: "-p HEAD --", want: "-p HEAD -- ."},
		{dir: "/repo", args: "-p f", want: "-p -- f"},
		{dir: "/repo", args: "-p HEAD f", want: "-p HEAD -- f"},
		{dir: "/repo", args: "-p -- f", want: "-p -- f"},
		{dir: "/repo", args: "-bp", want: "-bp"},
	}
	g := &GitDiscard{isTree: fakeIsTree}
	for _, tt := range tests {
		t.Run(tt.dir+" "+tt.args, func(t *testing.T) {
			c := gitInvocation{dir: tt.dir, sub: "checkout", args: strings.Fields(tt.args)}
			sorted, _ := g.sortCheckoutOperands(context.Background(), c)
			got := strings.Join(sorted.args, " ")
			if got != tt.want {
				t.Errorf("sortCheckoutOperands(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// TestCheckoutLookupsShareOneBudget pins the hook's time bound: every ref
// lookup one Check makes runs under one deadline, set lookupBudget after the
// Check starts, however many checkouts the command holds.
func TestCheckoutLookupsShareOneBudget(t *testing.T) {
	var warned []string
	g := newDiscardGuard(config.Config{}, discardFixture{repo: "r"}, &warned)
	var deadlines []time.Time
	g.isTree = func(ctx context.Context, _, _ string) (bool, error) {
		d, ok := ctx.Deadline()
		if !ok {
			t.Fatal("a ref lookup ran without a deadline")
		}
		deadlines = append(deadlines, d)
		return true, nil
	}
	command := strings.Repeat("git checkout main; ", 15)
	start := time.Now()
	g.Check(Input{Event: EventBash, Command: command, Cwd: "/repo"})
	end := time.Now()
	if len(deadlines) != 15 {
		t.Fatalf("Check made %d lookups, want 15", len(deadlines))
	}
	for _, d := range deadlines {
		if !d.Equal(deadlines[0]) {
			t.Fatalf("lookups ran under deadlines %v, want one shared", deadlines)
		}
	}
	if d := deadlines[0]; d.Before(start.Add(g.lookupBudget)) || d.After(end.Add(g.lookupBudget)) {
		t.Errorf("deadline %v, want lookupBudget %v after the Check starts", d, g.lookupBudget)
	}
}

// TestCheckoutLookupPastBudget pins what a lookup that runs out of time
// leaves: the operands read as paths, and where git may restore from the
// first one instead (patch mode, more operands) the proof cannot vouch, so
// only a clean tree passes. Alone the path reading drops the most, and the
// proof judges it.
func TestCheckoutLookupPastBudget(t *testing.T) {
	tests := []struct {
		command  string
		wantDeny bool
	}{
		{"git checkout -p HEAD", true},
		{"git checkout main f", true},
		{"git checkout -p f", true},
		{"git checkout main", false},
		{"git checkout -p -- f", false},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(config.Config{}, discardFixture{
				repo: "r", dirty: map[string]bool{"/repo": true},
			}, &warned)
			g.isTree = func(context.Context, string, string) (bool, error) {
				return false, context.DeadlineExceeded
			}
			g.prove = func(gitInvocation) error { return nil }
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			if (d != nil) != tt.wantDeny {
				t.Fatalf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
			if d != nil && !strings.Contains(d.Reason, "could not look up") {
				t.Errorf("reason = %q, want it to name the lookup", d.Reason)
			}
		})
	}
}

// TestGitStatusPorcelain runs the real backend against a scratch repo: clean
// reads as clean, an untracked file as dirty, and a non-repo as a failure.
func TestGitStatusPorcelain(t *testing.T) {
	dir := t.TempDir()
	if out, ok := gitStatusPorcelain(dir); ok {
		t.Fatalf("non-repo status = %q, ok; want a failure", out)
	}
	if err := gitCmd(dir, "init", "-q").Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	if out, ok := gitStatusPorcelain(dir); !ok || out != "" {
		t.Errorf("fresh repo status = %q, %v; want clean", out, ok)
	}
	if err := os.WriteFile(filepath.Join(dir, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, ok := gitStatusPorcelain(dir); !ok || out == "" {
		t.Errorf("status with an untracked file = %q, %v; want dirty", out, ok)
	}
	if _, ok := gitStatusPorcelain(""); ok {
		t.Error("empty dir must fail, never run git in belt's own cwd")
	}
}
