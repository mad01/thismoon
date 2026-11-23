package guard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
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
	// worktrees are the worktree paths git lists in any directory; nil makes
	// the list fail.
	worktrees []string
	// stranded maps listed worktrees to the count of commits only their HEAD
	// and HEAD reflog hold; a negative count makes the count fail.
	stranded map[string]int
	// pruned is the count of commits a `worktree prune` strands; a negative
	// count makes the count fail.
	pruned int
	// unforced marks directories where clean.requireForce reads false.
	unforced map[string]bool
	// unpushed maps a rev-list argument naming local branches to a commit
	// they hold that no remote-tracking ref holds; one missing holds none.
	// unpushedFails makes the count fail.
	unpushed      map[string]string
	unpushedFails bool
	// refspecs are the remote.<name>.fetch values of the remotes that are no
	// mirror the repository's config holds, and prunes says its fetch.prune
	// or a remote.<name>.prune may be true; refspecsFail makes the read fail.
	refspecs     []string
	prunes       bool
	refspecsFail bool
	// mirrorWorktrees marks a bare mirror with linked worktrees.
	mirrorWorktrees bool
}

// fakeAges are the dates fakeExpiry reads, as how long ago each is; a
// negative age lies in the future.
var fakeAges = map[string]time.Duration{
	"NOW":          0,
	"1.second.ago": time.Second,
	"1.day.ago":    24 * time.Hour,
	"1.week.ago":   7 * 24 * time.Hour,
	"2.weeks.ago":  14 * 24 * time.Hour,
	"3.weeks.ago":  21 * 24 * time.Hour,
	"29.days.ago":  29 * 24 * time.Hour,
	"30.days.ago":  30 * 24 * time.Hour,
	"89.days.ago":  89 * 24 * time.Hour,
	"90.days.ago":  90 * 24 * time.Hour,
	"91.days.ago":  91 * 24 * time.Hour,
	"2099-01-01":   -70 * 365 * 24 * time.Hour,
}

// fakeExpiry stands in for gitExpiry: NEVER reads as 0, as git reads it,
// a date in fakeAges as that long ago, and any other date fails, as git
// fails a date it cannot read.
func fakeExpiry(_ context.Context, date string) (uint64, error) {
	if date == "NEVER" {
		return 0, nil
	}
	age, ok := fakeAges[date]
	if !ok {
		return 0, errors.New("not a valid timestamp")
	}
	return uint64(time.Now().Add(-age).Unix()), nil
}

// fakeTrees are the revs fakeIsTree resolves in any directory belt can name.
var fakeTrees = wordSet("main HEAD @{-1} origin/main")

// fakeIsTree stands in for gitIsTree: an empty dir resolves nothing.
func fakeIsTree(_ context.Context, dir, rev string) (bool, error) {
	return dir != "" && fakeTrees[rev], nil
}

// fakeLost stands in for a stranded-commit count of n, failing when n is
// negative.
func fakeLost(n int) (lostCommits, error) {
	if n < 0 {
		return lostCommits{}, errors.New("git rev-list failed")
	}
	return lostCommits{n: n, tip: "0123456789abcdef0123"}, nil
}

// newDiscardGuard builds the guard with injected resolvers and a warning
// recorder, so no test shells out to git or dials the events service. The
// nothing-lost proof never holds here: a dirty fixture tree has unique work.
func newDiscardGuard(cfg config.Config, f discardFixture, warned *[]string) *GitDiscard {
	g := NewGitDiscard(cfg)
	g.resolveRepo = func(string) string { return f.repo }
	g.isTree = fakeIsTree
	g.worktrees = func(string) ([]listedWorktree, error) {
		if f.worktrees == nil {
			return nil, errors.New("git worktree list failed")
		}
		var list []listedWorktree
		for i, p := range f.worktrees {
			list = append(list, listedWorktree{path: p, main: i == 0})
		}
		return list, nil
	}
	g.stranded = func(_ context.Context, _ string, wt listedWorktree) (lostCommits, error) {
		return fakeLost(f.stranded[wt.path])
	}
	g.pruned = func(context.Context, string, []string) (lostCommits, error) {
		return fakeLost(f.pruned)
	}
	g.unpushed = func(_ context.Context, _, rev string) (string, error) {
		if f.unpushedFails {
			return "", errors.New("git rev-list failed")
		}
		return f.unpushed[rev], nil
	}
	g.repoFetch = func(context.Context, string) (fetchConfig, error) {
		if f.refspecsFail {
			return fetchConfig{}, errors.New("git config failed")
		}
		return fetchConfig{refspecs: f.refspecs, prune: f.prunes, mirrorWorktrees: f.mirrorWorktrees}, nil
	}
	g.expiry = fakeExpiry
	g.requireForce = func(_ context.Context, dir string) bool { return !f.unforced[dir] }
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
		// checkout and switch options read as git's parse-options reads them:
		// a prefix of one name, a bundle up to a letter that takes a value, and
		// fail closed on a word belt cannot read.
		{"git checkout --fo main", true},
		{"git checkout -mf main", true},
		{"git checkout -fB x main", true},
		{"git checkout -Bf main", false}, // a branch named f
		{"git checkout -tf", false},      // -t takes f as its value, which git refuses
		{"git checkout --no-f main", false},
		{"git checkout -f --no-force main", false},
		{"git checkout --patc", true},
		{"git checkout --pat main", true}, // ambiguous: patch or a pathspec option
		{"git checkout -U3 -p", true},
		{"git checkout -2 file.go", true},
		{"git checkout --ours -- file.go", true},
		{"git checkout --conflict=diff3 -- file.go", true},
		{"git checkout --conflict diff3 file.go", true},
		{"git checkout --pathspec-f list", true}, // ambiguous: from-file or file-nul
		{"git checkout --frobnicate main", true},
		{"git checkout $X", true},
		{"git checkout main $X", true},
		{"git checkout -b feat --track origin/main", false},
		{"git checkout --orphan new", false},
		{"git checkout --detach main", false},
		{"git checkout -b -- main", false}, // -b takes -- as its value
		{"git switch --force main", true},
		{"git switch --discard main", true},
		{"git switch --fo main", true}, // ambiguous: force or force-create
		{"git switch --d main", true},  // ambiguous: detach or discard-changes
		{"git switch -mf main", true},
		{"git switch -fC x", true},
		{"git switch -Cf x", false}, // a branch named f
		{"git switch -f --no-force main", false},
		{"git switch --no-force main", false},
		{"git switch --no-discard-changes main", false},
		{"git switch --track origin/x", false},
		{"git switch -d HEAD~1", false},
		{"git switch -c new --no-guess", false},
		{"git switch $X", true},
		{"git switch --frobnicate main", true},
		{"echo main | xargs git switch", true},
		{"xargs git -c", true}, // a fed git whose options belt cannot read
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
		// --merge drops staged edits; any prefix git reads as it counts.
		{"git reset --merge", true},
		{"git reset --mer HEAD~1", true},
		{"git reset -q --merge origin/main", true},
		{"git reset --merge --soft", true}, // the proof reads no mix of modes
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
		// A clean without -f or -i deletes once clean.requireForce is false,
		// which config the command sets may say.
		{"git -c clean.requireForce=false clean -d", true},
		{"git -c color.ui=never clean -d", true},
		{"git --config-env=clean.requireForce=V clean", true},
		{"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=clean.requireForce GIT_CONFIG_VALUE_0=false git clean -d", true},
		{`export GIT_CONFIG_PARAMETERS="'clean.requireforce=false'"; git clean -d`, true},
		{"git config clean.requireForce false && git clean -d", true},
		{"git config --local clean.requireForce false; git clean", true},
		{"HOME=/tmp/h git clean -x", true},
		{"git -c clean.requireForce=false clean -n", false},
		{"git clean -d && git config clean.requireForce false", false},
		{"git clean", false},
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

// TestGitDiscardUnforcedClean pins a clean in a repository whose config on
// disk sets clean.requireForce false: without -f or -i it deletes all the
// same, so it counts on a dirty tree and passes a clean one; a dry run
// deletes nothing.
func TestGitDiscardUnforcedClean(t *testing.T) {
	tests := []struct {
		command  string
		dirty    bool
		wantDeny bool
	}{
		{"git clean", true, true},
		{"git clean -d", true, true},
		{"git clean -x", true, true},
		{"git clean -q -- out", true, true},
		{"git -C /repo clean -d", true, true},
		{"git clean -n", true, false},
		{"git clean -d --dry-run", true, false},
		{"git clean -d", false, false},
		{"git -C /other clean -d", true, false}, // requireForce holds at /other
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var warned []string
			f := discardFixture{
				repo: "r", unforced: map[string]bool{"/repo": true},
				dirty: map[string]bool{"/repo": tt.dirty, "/other": tt.dirty},
			}
			g := newDiscardGuard(config.Config{}, f, &warned)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			if (d != nil) != tt.wantDeny {
				t.Fatalf("Check(%q) denial = %v, wantDeny %v", tt.command, d, tt.wantDeny)
			}
		})
	}
}

// TestGitDiscardTreeWriters pins which worktree remove, apply, read-tree,
// checkout-index, rm and mv forms count as a discard on a dirty tree, their
// options read as git's parse-options reads them, with an option belt does
// not know, or a word the shell may still turn into one, counted.
func TestGitDiscardTreeWriters(t *testing.T) {
	dirty := discardFixture{
		repo:      "github.com/mad01/thismoon",
		dirty:     map[string]bool{"/repo": true, "/wt": true},
		worktrees: []string{"/repo", "/wt"},
	}
	tests := []struct {
		command  string
		wantDeny bool
	}{
		// A forced remove deletes the worktree; a plain one refuses a dirty
		// tree, and both are judged in it (TestGitDiscardDecision).
		{"git worktree remove --force /wt", true},
		{"git worktree remove -f wt", true},
		{"git worktree remove -f -f ../wt", true},
		{"git worktree remove --fo /wt", true},
		{"git worktree remove --no-force -f /wt", true},
		{"git worktree remove --frobnicate /wt", true},
		{"git worktree remove -f $WT", true},
		{"git worktree $SUB -f /wt", true},
		{"echo /wt | xargs git worktree remove", true},
		{"echo remove /wt | xargs git worktree", true},
		{"git worktree remove $WT", true},
		{"git worktree remove /wt", false},
		{"git worktree remove '/wt'", false},
		{"git worktree remove -f --no-force /wt", false},
		{"git worktree list", false},
		{"git worktree prune", false},
		{"git worktree prune -n --frobnicate", true},
		{"git worktree prune $OPT", true},
		{"git worktree add ../x", false},
		// apply rewrites what its patches touch, forward or reversed.
		{"git apply -R p.diff", true},
		{"git apply p.diff", true},
		{"git apply --rev p.diff", true},
		{"git diff | git apply -R", true},
		{"git apply --index p.diff", true},
		{"git apply -3 p.diff", true},
		{"git apply --cached --no-cached p.diff", true},
		{"git apply --stat --apply p.diff", true},
		{"git apply --check --no-check p.diff", true},
		{"git apply --build-fake-ancestor=x p.diff", true},
		{"git apply --frobnicate p.diff", true},
		{"git apply --no-a p.diff", true}, // git finds it ambiguous
		{"git apply $OPT p.diff", true},
		{"git apply --cached p.diff", false},
		{"git apply --cach -R p.diff", false},
		{"git apply --check -R p.diff", false},
		{"git apply --stat p.diff", false},
		{"git apply --numstat -R p.diff", false},
		{"git apply --summary p.diff", false},
		// read-tree writes the worktree with -u.
		{"git read-tree -u --reset HEAD", true},
		{"git read-tree -m -u HEAD", true},
		{"git read-tree -mu HEAD", true},
		{"git read-tree --res -u HEAD", true},
		{"git read-tree -n --no-dry-run -m -u HEAD", true},
		{"git read-tree --frobnicate HEAD", true},
		// --index-output writes the index over the file it names.
		{"git read-tree --index-output=a.txt HEAD", true},
		{"git read-tree --index-out a.txt HEAD", true},
		{"git read-tree -n --no-dry-run --index-output=a.txt HEAD", true},
		{"git read-tree -n --index-output=a.txt HEAD", false},
		{"git read-tree --reset HEAD", false},
		{"git read-tree -m HEAD", false},
		{"git read-tree HEAD", false},
		{"git read-tree -n -m -u HEAD", false},
		{"git read-tree --dry -m -u HEAD", false},
		// checkout-index overwrites only with -f.
		{"git checkout-index -f -a", true},
		{"git checkout-index -fa", true},
		{"git checkout-index --force --all", true},
		{"git checkout-index --fo -- file.go", true},
		{"git checkout-index -f --stdin", true},
		{"git checkout-index --frobnicate -a", true},
		{"git checkout-index -a", false},
		{"git checkout-index -f --temp -a", false},
		{"git checkout-index -f --no-force -a", false},
		// rm deletes a changed file only with -f, and never with --cached.
		{"git rm -f file.go", true},
		{"git rm -rf .", true},
		{"git rm -r -f .", true},
		{"git rm --fo file.go", true},
		{"git rm -n --no-dry-run -f file.go", true},
		{"git rm -f --pathspec-from-file=list", true},
		{"git rm $OPT file.go", true},
		{"echo -f | xargs git rm", true},
		{"git rm file.go", false},
		{"git rm -r dir", false},
		{"git rm --cached -f file.go", false},
		{"git rm --ca -rf .", false},
		{"git rm -f -n file.go", false},
		{"git rm -- -f", false}, // a path after --
		// mv overwrites its destination only with -f.
		{"git mv -f a b", true},
		{"git mv --force a b", true},
		{"git mv --fo a b", true},
		{"git mv -kf a b", true},
		{"git mv --frobnicate a b", true},
		{"git mv $OPT a b", true},
		{"git mv a b", false},
		{"git mv -n -f a b", false},
		{"git mv -f --no-force a b", false},
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

// TestPruneExpiry pins which expire words a prune's dry run takes: the
// prune's own, unless a value may still change, which leaves the most a
// prune deletes.
func TestPruneExpiry(t *testing.T) {
	tests := []struct {
		args    string
		fed     bool
		want    string
		wantErr bool
	}{
		{args: "", want: ""},
		{args: "-v -n", want: ""},
		{args: "--expire 3.days.ago", want: "--expire=3.days.ago"},
		{args: "--expire=now -v", want: "--expire=now"},
		{args: "--exp never", want: "--expire=never"},
		{args: "--expire=now --no-expire", want: "--expire=now --no-expire"},
		{args: "--expire=*", want: ""},
		{args: "--expire=$WHEN", wantErr: true},
		{args: "--expire=1.day.ago", fed: true, want: ""},
		{args: "--frobnicate", wantErr: true},
		{args: "--expire", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.args, func(t *testing.T) {
			got, err := pruneExpiry(strings.Fields(tt.args), tt.fed)
			if (err != nil) != tt.wantErr {
				t.Fatalf("pruneExpiry(%q) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if strings.Join(got, " ") != tt.want {
				t.Errorf("pruneExpiry(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// TestRemoveTargets pins which listed worktrees a `worktree remove <name>`
// may delete: one whose path ends in the name at a directory boundary, or
// the one at the name as a path, both without case.
func TestRemoveTargets(t *testing.T) {
	listed := listedAt("/repo", "/x/wt", "/y/WT", "/x/other-wt")
	tests := []struct {
		dir, name string
		want      []string
	}{
		{"/repo", "/x/wt", []string{"/x/wt"}},
		{"/repo", "../x/wt", []string{"/x/wt"}},
		{"/repo", "../x/wt/", []string{"/x/wt"}},
		{"/repo", "x/wt", []string{"/x/wt"}},
		{"/repo", "wt", []string{"/x/wt", "/y/WT"}},
		{"/x", "other-wt", []string{"/x/other-wt"}},
		{"/repo", "-wt", nil},
		{"/repo", "../y/wt", []string{"/y/WT"}},
		{"/repo", "gone", nil},
		{"/repo", ".", []string{"/repo"}},
	}
	for _, tt := range tests {
		t.Run(tt.dir+" "+tt.name, func(t *testing.T) {
			got := removeTargets(tt.dir, tt.name, listed)
			if paths := worktreePaths(got); paths != strings.Join(tt.want, " ") {
				t.Errorf("removeTargets(%q, %q) = %v, want %v", tt.dir, tt.name, paths, tt.want)
			}
		})
	}
}

// TestRemoveTargetsThroughSymlinks pins that a relative name counts from
// the physical directory too: git resolves `..` from where a symlinked cwd
// points, not from the link's parent.
func TestRemoveTargetsThroughSymlinks(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"r/sub", "r/wty", "x/wty"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "x", "L2")
	if err := os.Symlink(filepath.Join(root, "r", "sub"), link); err != nil {
		t.Fatal(err)
	}
	physical, logical := filepath.Join(root, "r", "wty"), filepath.Join(root, "x", "wty")
	listed := listedAt(physical, logical)
	got := worktreePaths(removeTargets(link, "../wty", listed))
	if want := physical + " " + logical; got != want {
		t.Errorf("removeTargets(%q, ../wty) = %v, want %v", link, got, want)
	}
	got = worktreePaths(removeTargets(link, "../../r/wty", listed))
	if got != physical {
		t.Errorf("removeTargets(%q, ../../r/wty) = %v, want %v", link, got, physical)
	}
}

// listedAt lists worktrees at paths, none of them main.
func listedAt(paths ...string) []listedWorktree {
	list := make([]listedWorktree, 0, len(paths))
	for _, p := range paths {
		list = append(list, listedWorktree{path: p})
	}
	return list
}

// worktreePaths joins the paths of a worktree list with spaces.
func worktreePaths(list []listedWorktree) string {
	paths := make([]string, 0, len(list))
	for _, wt := range list {
		paths = append(paths, wt.path)
	}
	return strings.Join(paths, " ")
}

// TestGitDiscardStashDrop pins that a stash drop or clear is denied on a
// clean tree and a dirty one alike, with its own reason, in any form git
// reads as one or the shell may turn into one. A push form keeps the
// clean-tree rule (pushes), and the subcommands that keep the entries' work
// pass.
func TestGitDiscardStashDrop(t *testing.T) {
	const repo = "github.com/mad01/thismoon"
	// drops denies on any tree with the stash reason; pushes, and hidesKeeps
	// for a git call whose subcommand belt cannot see, deny a dirty tree
	// alone; hides denies a dirty tree as a discard and a clean one with the
	// stash reason.
	tests := []struct {
		command    string
		drops      bool
		pushes     bool
		hides      bool
		hidesKeeps bool
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
		// A reflog or ref delete that reaches refs/stash, which holds the
		// entries.
		{command: "git reflog delete 'stash@{0}'", drops: true},
		{command: "git reflog delete refs/stash@{1}", drops: true},
		{command: "git reflog delete --rewrite --updateref stash@{0}", drops: true},
		{command: "git reflog delete STASH@{0}", drops: true},
		{command: `git reflog delete "$R@{0}"`, drops: true},
		{command: "git reflog delete $E", drops: true},
		{command: "git reflog delete --frobnicate stash@{0}", drops: true},
		{command: "git reflog 'delete' stash@{0}", drops: true},
		{command: "git reflog $X stash@{0}", drops: true},
		{command: "git reflog d{elete,} stash@{0}", drops: true},
		{command: "git reflog expire --expire-unreachable=now refs/stash", drops: true},
		{command: "git reflog expire --expire=now --all", drops: true},
		{command: "git reflog expire --al", drops: true},
		{command: "git reflog expire stash", drops: true},
		{command: "git reflog expire --exp=now stash", drops: true}, // ambiguous
		{command: "git reflog drop refs/stash", drops: true},
		{command: "git reflog drop --all", drops: true},
		{command: "echo delete | xargs git reflog", drops: true},
		{command: "echo main | xargs git reflog expire", drops: true},
		{command: "git update-ref -d refs/stash", drops: true},
		{command: "git update-ref -d stash", drops: true},
		{command: "git update-ref -m gone --no-deref -d refs/stash", drops: true},
		{command: "git update-ref -d $R", drops: true},
		{command: "git update-ref --stdin", drops: true},
		{command: "git update-ref --std -z", drops: true},
		{command: "echo refs/stash | xargs git update-ref -d", drops: true},
		{command: "git reflog delete -n 'stash@{0}'"},
		{command: "git reflog delete -n HEAD@{1}"},
		{command: "git reflog expire --dry-run --all"},
		{command: "git reflog expire --expire=90.days.ago refs/heads/main"},
		{command: "git reflog"},
		{command: "git reflog -n 5"},
		{command: "git reflog HEAD@{1}"},
		{command: "git reflog show stash"},
		{command: "git reflog list"},
		{command: "git reflog exists refs/stash"},
		{command: "git update-ref -d refs/heads/old"},
		{command: "git update-ref refs/stash HEAD"},
		{command: "git update-ref --no-stdin refs/heads/x HEAD"},
		// A command in front of git that may hand it the subcommand: belt
		// never sees the words, so they may drop entries too.
		{command: "echo stash clear | xargs git", hides: true},
		{command: "echo reset --hard | xargs git", hides: true},
		{command: "xargs git < cmds.txt", hides: true},
		{command: "xargs git --no-pager", hides: true},
		{command: "xargs git -c x=y", hides: true},
		{command: "xargs git -C /other", drops: true}, // a clean tree at /other
		{command: `find . -name x -exec git {} \;`, hides: true},
		// A subcommand word only a shell can resolve.
		{command: "git $X clear", hides: true},
		{command: "git $X drop -q", hides: true},
		{command: `git "$X" stash drop`, hides: true},
		{command: "git 'stash' drop", drops: true}, // the walk drops the quotes
		{command: "git st\\ash clear", drops: true},
		{command: "git $X", hides: true},
		{command: `git "$@"`, hides: true},
		{command: "git $X $Y", hides: true},
		{command: "git $X file.go", hidesKeeps: true},
		{command: "git ${X} status -s", hidesKeeps: true},
		{command: "git 'stash' list"},
		{command: "git st*sh list", hidesKeeps: true},
		// An alias body belt cannot read.
		{command: `git -c alias.x="$B" x`, hides: true},
		{command: `git -c alias.x="stash $Y" x`, hides: true},
		{command: `git -c alias.x="!$B" x`, hides: true},
		{command: `git -c alias.x="-c a=b $Y" x`, hides: true},
		{command: `git -c alias.x="y $Z" x`, hides: true}, // y may be another alias
		{command: `git config alias.x "$B" && git x`, hides: true},
		{command: "git --config-env=alias.x=B x", hides: true},
		{command: `git -c alias.x="log $Y" x`, hidesKeeps: true},
		{command: `git -c "alias.x=reset $Y" x`, hidesKeeps: true},
		// Text git prints, run as code, is script-deny-list's to deny, in
		// any directory: it names no stash drop, so it passes here on any
		// tree. The same output taken as a value runs nothing, and neither
		// does a git word in text an interpreter reads.
		{command: "git log -1 --format=%B | sh"},
		{command: "git show HEAD:run.sh | tee run.log | bash"},
		{command: "git cat-file -p HEAD:run.sh | bash"},
		{command: "git config --get alias.x | sh"},
		{command: "git notes show | sh"},
		{command: "git log -1 --format=%B | xargs -0 sh -c"},
		{command: "git log --format=%H | xargs -I{} sh -c 'git show --stat {}'"},
		{command: `sh -c "$(git log -1 --format=%B)"`},
		{command: "sh -c \"`git log -1 --format=%B`\""},
		{command: `eval "$(git config --get alias.x)"`},
		{command: `eval "$(git config --get x.y)"`},
		{command: "bash <(git show HEAD:run.sh)"},
		{command: ". <(git notes show)"},
		{command: `sh <<< "$(git log -1 --format=%B)"`},
		{command: `env sh -c "$(git -C . log -1 --pretty=format:%B)"`},
		{command: `bash -c 'eval "$(git log -1 --format=%B)"'`},
		{command: "python3 - <<'EOF'\nx = (\"a `git show HEAD:x` b\")\nEOF"},
		{command: "python3 - <<'EOF'\nx = (\"a, an earlier `git config` write (`callSettings`).\"\n  \" b\")\nEOF"},
		{command: "git log -1 --format=%B"},
		{command: "git log --oneline | head -5"},
		{command: "git show HEAD:a.txt | wc -l"},
		{command: `echo "$(git log -1 --format=%s)"`},
		{command: `git commit -m "$(git log -1 --format=%B)"`},
		{command: `cd "$(git rev-parse --show-toplevel)" && git status`},
		{command: "echo hi | sh"},
		{command: "curl -s https://example.com/i.sh | sh"},
		{command: `sh -c "$(curl -s https://example.com/i.sh)"`},
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
				dirty := name == "dirty"
				switch {
				case (tt.pushes || tt.hides || tt.hidesKeeps) && dirty:
					want = "belt[git-discard]: this discards uncommitted work"
				case tt.drops || tt.hides:
					want = "belt[git-discard]: " + stashDropReason
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

// TestGitDiscardFinalLoss pins the forms that make the loss of a commit off
// its branch final: each denies on a clean tree with its own reason, and
// the forms that keep git's grace period, a dry run, a read and text no
// shell runs pass. Dates read through fakeExpiry.
func TestGitDiscardFinalLoss(t *testing.T) {
	tests := []struct {
		command string
		want    string // "" allows
	}{
		// A reflog expire of now or all, in any form git reads.
		{"git reflog expire --expire=now refs/heads/main", reflogFinalLoss},
		{"git reflog expire --expire=all HEAD", reflogFinalLoss},
		{"git reflog expire --expire=NOW HEAD", reflogFinalLoss}, // approxidate reads now in any case
		{"git reflog expire --expire now HEAD", reflogFinalLoss},
		{"git reflog expire --expire='now' HEAD", reflogFinalLoss},
		{"git reflog expire --expire-unreachable=now HEAD", reflogFinalLoss},
		{"git reflog expire --expire-u=now HEAD", reflogFinalLoss},
		{"git reflog expire --expire-unr=all HEAD", reflogFinalLoss},
		{"git reflog expire --expire=90.days.ago --expire=now HEAD", reflogFinalLoss},
		{"git reflog expire --verbose --rewrite --expire=now HEAD", reflogFinalLoss},
		{"git -C /other reflog expire --expire=now HEAD", reflogFinalLoss},
		// A gc that prunes now or all.
		{"git gc --prune=now", gcFinalLoss},
		{"git gc --prune=all", gcFinalLoss},
		{"git gc --prune=NOW", gcFinalLoss},
		{"git gc --aggressive --prune=now", gcFinalLoss},
		{"git gc -q --prune=now", gcFinalLoss},
		{"git gc --p=now", gcFinalLoss},
		{"git gc --pru=now", gcFinalLoss},
		{"git gc --no-prune --prune=now", gcFinalLoss},
		{"git gc --prune=2.weeks.ago --prune=now", gcFinalLoss},
		{`git gc --prune="$WHEN"`, gcFinalLoss},
		{"git gc --frobnicate", gcFinalLoss},
		// A date sooner than git's default, in the future, or one git cannot read.
		{"git gc --prune=2099-01-01", gcFinalLoss},
		{"git gc --prune=1.second.ago", gcFinalLoss},
		{"git gc --prune=1.week.ago", gcFinalLoss},
		{"git gc --prune=garbage", gcFinalLoss},
		{"echo --prune=now | xargs git gc", gcFinalLoss},
		// A prune with no grace period: plain git prune expires everything.
		{"git prune", pruneFinalLoss},
		{"git prune -v", pruneFinalLoss},
		{"git prune --progress", pruneFinalLoss},
		{"git prune --expire=now", pruneFinalLoss},
		{"git prune --expire now", pruneFinalLoss},
		{"git prune --exp=all", pruneFinalLoss},
		{"git prune --expire=2.weeks.ago --expire=now", pruneFinalLoss},
		{"git prune --no-expire --expire=now", pruneFinalLoss},
		{"git prune -n --no-dry-run", pruneFinalLoss},
		{"git prune --frobnicate", pruneFinalLoss},
		{"git prune --expire=2099-01-01", pruneFinalLoss},
		{"git prune --expire=1.second.ago", pruneFinalLoss},
		{"git prune --expire 1.week.ago", pruneFinalLoss},
		{"git prune --expire=garbage", pruneFinalLoss},
		{"git prune --expire=", pruneFinalLoss},
		{"git reflog expire --expire=n HEAD", reflogFinalLoss},   // git refuses the date
		{"git reflog expire --expire=ALL HEAD", reflogFinalLoss}, // git refuses the date
		{"git reflog expire --expire=89.days.ago HEAD", reflogFinalLoss},
		{"git reflog expire --expire=2.weeks.ago HEAD", reflogFinalLoss},
		{"git reflog expire --expire=2099-01-01 HEAD", reflogFinalLoss},
		{"git reflog expire --expire-unreachable=1.second.ago refs/heads/feat", reflogFinalLoss},
		{"git reflog expire --expire-unreachable=29.days.ago HEAD", reflogFinalLoss},
		{"git reflog expire --expire=91.days.ago --expire-unreachable=1.week.ago HEAD", reflogFinalLoss},
		// A reflog delete or drop of a branch or HEAD log.
		{"git reflog delete HEAD@{1}", reflogDeleteFinalLoss},
		{"git reflog delete refs/heads/feat@{0}", reflogDeleteFinalLoss},
		{"git reflog delete --rewrite --updateref main@{2}", reflogDeleteFinalLoss},
		{"git reflog delete -n --no-dry-run HEAD@{1}", reflogDeleteFinalLoss},
		{"git reflog drop main", reflogDeleteFinalLoss},
		{"git reflog drop refs/heads/feat", reflogDeleteFinalLoss},
		{"git reflog drop HEAD", reflogDeleteFinalLoss},
		{"git reflog 'drop' HEAD", reflogDeleteFinalLoss},
		// A repack of everything that deletes what no ref or reflog holds.
		{"git repack -a -d", repackFinalLoss},
		{"git repack -ad", repackFinalLoss},
		{"git repack -adq", repackFinalLoss},
		{"git repack -a -d -f --window=250 --depth=50", repackFinalLoss},
		{"git repack -a -d -k --no-keep-unreachable", repackFinalLoss},
		{"git repack -a -d --no-cruft", repackFinalLoss},
		{"git repack --cruft -d --cruft-expiration=now", repackFinalLoss},
		{"git repack --cruft -d --cruft-expiration=2099-01-01", repackFinalLoss},
		{"git repack --cruft -d --cruft-exp=1.week.ago", repackFinalLoss},
		{"git repack -A -d --unpack-unreachable=now", repackFinalLoss},
		{"git repack -Ad --unpack-unreachable 1.second.ago", repackFinalLoss},
		{"git repack -a -d --unpack-unreachable=1.day.ago", repackFinalLoss},
		{"git repack -a -d --unpack-unreachable=3.weeks.ago --no-unpack-unreachable", repackFinalLoss},
		{"git repack -A -d --unpack-unreachable=", repackFinalLoss}, // an empty date is now to pack-objects
		{"git repack -a -d --frobnicate", repackFinalLoss},
		{"xargs git repack", repackFinalLoss},
		{"xargs git prune", pruneFinalLoss},
		// A gc, prune or reflog call whose own config sets an expiry.
		{"git -c gc.pruneExpire=now gc", callConfigFinalLoss},
		{"git -c gc.pruneexpire=now gc --auto", callConfigFinalLoss},
		{"git -c gc.reflogExpire=now gc", callConfigFinalLoss},
		{"git -c gc.reflogExpireUnreachable=all gc", callConfigFinalLoss},
		{"git -c 'gc.refs/heads/*.reflogExpire=now' reflog expire refs/heads/main", callConfigFinalLoss},
		{"git -c gc.reflogExpire=now reflog expire HEAD", callConfigFinalLoss},
		{"git -c gc.pruneExpire=now prune --expire=2.weeks.ago", callConfigFinalLoss},
		{`git -c "gc.pruneExpire=$W" gc`, callConfigFinalLoss},
		{`git -c "$K=now" gc`, callConfigFinalLoss},
		{"git --config-env=gc.pruneExpire=WHEN gc", callConfigFinalLoss},
		{"git -c gc.pruneExpire=1.second.ago gc", callConfigFinalLoss},
		{"git -c gc.reflogExpire=2.weeks.ago gc", callConfigFinalLoss},
		{"git -c gc.reflogExpire=now reflog expire -n --no-dry-run HEAD", callConfigFinalLoss},
		// Any call but a read: a commit, merge, rebase or fetch may start a
		// gc, and a hook or alias may run one.
		{"git -c gc.pruneExpire=now fetch origin", callConfigFinalLoss},
		{"git -c gc.pruneExpire=now commit -m x", callConfigFinalLoss},
		{"git -c gc.reflogExpire=now merge feat", callConfigFinalLoss},
		{"git -c gc.pruneExpire=now pull", callConfigFinalLoss},
		{"git -c gc.pruneExpire=now maintenance run --task=gc", callConfigFinalLoss},
		{"git -c gc.pruneExpire=now maintenance run", callConfigFinalLoss},
		{"git -c gc.pruneExpire=now sweep", callConfigFinalLoss},
		{`git -c "$K=+main:feat" fetch`, callConfigFinalLoss}, // the key may be an expiry
		{"echo -m x | xargs git -c gc.pruneExpire=now commit", callConfigFinalLoss},
		{"echo -m x | xargs git -c gc.pruneExpire=now status", ""},
		// The same settings from the environment, in front of git or exported
		// before it.
		{"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=gc.pruneExpire GIT_CONFIG_VALUE_0=now git gc", callConfigFinalLoss},
		{"env GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=gc.pruneExpire GIT_CONFIG_VALUE_0=now git gc", callConfigFinalLoss},
		{"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=gc.reflogExpire GIT_CONFIG_VALUE_0='now' git reflog expire HEAD", callConfigFinalLoss},
		{"export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=gc.pruneExpire GIT_CONFIG_VALUE_0=now; git commit -m x", callConfigFinalLoss},
		{"export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=gc.pruneExpire GIT_CONFIG_VALUE_0=now && git maintenance run --task=gc", callConfigFinalLoss},
		{"GIT_CONFIG_KEY_3=gc.pruneExpire GIT_CONFIG_VALUE_3=1.week.ago git gc", callConfigFinalLoss},
		{"GIT_CONFIG_KEY_0=gc.pruneExpire git gc", callConfigFinalLoss}, // the value may be exported
		{"GIT_CONFIG_VALUE_0=now git gc", callConfigFinalLoss},          // the key may be exported
		{`GIT_CONFIG_KEY_0="$K" GIT_CONFIG_VALUE_0=now git gc`, callConfigFinalLoss},
		{`GIT_CONFIG_KEY_0=gc.pruneExpire GIT_CONFIG_VALUE_0="$W" git gc`, callConfigFinalLoss},
		{"GIT_CONFIG_KEY_0=gc.pruneExpire GIT_CONFIG_VALUE_0+=now git gc", callConfigFinalLoss},
		{`GIT_CONFIG_PARAMETERS="'gc.pruneExpire'='now'" git gc`, callConfigFinalLoss},
		{`GIT_CONFIG_PARAMETERS="'gc.pruneExpire=now'" git gc`, callConfigFinalLoss},
		{`GIT_CONFIG_PARAMETERS="'core.pager'='less' 'gc.reflogExpire'='now'" git commit -m x`, callConfigFinalLoss},
		{`GIT_CONFIG_PARAMETERS="$P" git gc`, callConfigFinalLoss},
		// A config write that sets one up for a later gc.
		{"git config gc.pruneExpire now", configFinalLoss},
		{"git config --global gc.reflogExpire now", configFinalLoss},
		{"git config --local gc.reflogExpireUnreachable all", configFinalLoss},
		{"git config set gc.reflogExpire now", configFinalLoss},
		{"git config --add gc.pruneExpire now", configFinalLoss},
		{"git config GC.PRUNEEXPIRE NOW", configFinalLoss},
		{"git config 'gc.refs/remotes/*.reflogExpire' now", configFinalLoss},
		{"git config --file .git/config gc.pruneExpire now", configFinalLoss},
		{"git config gc.pruneExpire 2099-01-01", configFinalLoss},
		{"git config gc.pruneExpire 1.week.ago", configFinalLoss},
		{"git config gc.reflogExpire 2.weeks.ago", configFinalLoss},
		{"git config gc.reflogExpireUnreachable 3.weeks.ago", configFinalLoss},
		{"git config gc.pruneExpire garbage", configFinalLoss},
		{`git config "$K" 2.weeks.ago`, configFinalLoss}, // held to the longest window
		{`git config gc.pruneExpire "$W"`, configFinalLoss},
		{`git config "$K" now`, configFinalLoss},
		{"echo gc.pruneExpire now | xargs git config", configFinalLoss},
		// The walk: a cd, git -C, a subshell, sh -c, a wrapper and a chain.
		{"cd /other && git gc --prune=now", gcFinalLoss},
		{"git -C /other prune --expire=now", pruneFinalLoss},
		{"(git gc --prune=now)", gcFinalLoss},
		{"sh -c 'git prune'", pruneFinalLoss},
		{"timeout 60 git gc --prune=now", gcFinalLoss},
		{"git branch -D x && git prune", pruneFinalLoss},
		{"git -c alias.sweep='gc --prune=now' sweep", gcFinalLoss},
		// Forms that keep a grace period, run dry or only read.
		{"git gc", ""},
		{"git gc --auto", ""},
		{"git gc --aggressive", ""},
		{"git gc --prune", ""},
		{"git gc --prune=2.weeks.ago", ""},
		{"git gc --prune=3.weeks.ago", ""},
		{"git gc --prune=NEVER", ""},
		{"git gc --prune=never", ""},
		{"git gc --no-prune", ""},
		{"git gc --prune=now --no-prune", ""},
		{"git reflog", ""},
		{"git reflog show main", ""},
		{"git reflog expire HEAD", ""},
		{"git reflog expire --expire=90.days.ago", ""},
		{"git reflog expire --expire=90.days.ago HEAD", ""},
		{"git reflog expire --expire=never HEAD", ""},
		{"git reflog expire --expire=NEVER HEAD", ""},
		{"git reflog expire --expire=91.days.ago HEAD", ""},
		{"git reflog expire --expire-unreachable=30.days.ago HEAD", ""},
		{"git reflog expire --expire-unreachable=91.days.ago HEAD", ""},
		{"git reflog expire -n --expire=now HEAD", ""},
		{"git reflog expire --dry-run --expire-unreachable=now HEAD", ""},
		{"git reflog delete -n HEAD@{1}", ""},
		{"git reflog delete --dry-run main@{1}", ""},
		{"git prune -n", ""},
		{"git prune --dry-run --expire=now", ""},
		{"git prune --expire=2.weeks.ago", ""},
		{"git prune --expire=3.weeks.ago", ""},
		{"git prune --expire=NEVER", ""},
		{"git prune --expire=never", ""},
		{"git prune --no-expire", ""},
		{"git prune --expire=now --no-expire", ""},
		{"git config gc.pruneExpire", ""},
		{"git config --get gc.pruneExpire", ""},
		{"git config gc.pruneExpire 2.weeks.ago", ""},
		{"git config gc.pruneExpire 3.weeks.ago", ""},
		{"git config gc.reflogExpireUnreachable 91.days.ago", ""},
		{"git config gc.reflogExpire 90.days.ago", ""},
		{"git config gc.reflogExpire never", ""},
		{"git config --unset gc.reflogExpire", ""},
		{"git config gc.auto now", ""},
		{"git config user.name now", ""},
		{"echo gc.pruneExpire | xargs git config --get", ""},
		{"git -c gc.pruneExpire=2.weeks.ago gc", ""},
		{"git -c core.pager=less gc", ""},
		{"git -c gc.pruneExpire=now status", ""},
		{"git -c gc.pruneExpire=3.weeks.ago gc", ""},
		{"git -c gc.pruneExpire=3.weeks.ago commit -m x", ""},
		{"git -c gc.auto=0 commit -m x", ""},
		{"git -c gc.pruneExpire=now log --oneline", ""},
		{"git -c gc.pruneExpire=now config --get gc.pruneExpire", ""},
		// A read or a dry run applies no expiry from its settings.
		{"git -c gc.reflogExpire=now reflog show", ""},
		{"git -c gc.reflogExpire=now reflog", ""},
		{"git -c gc.reflogExpire=now reflog list", ""},
		{"git -c gc.reflogExpire=now reflog HEAD@{1}", ""},
		{"git -c gc.reflogExpire=now reflog expire -n HEAD", ""},
		{"git -c gc.pruneExpire=now prune -n", ""},
		{"git -c gc.pruneExpire=now prune --dry-run --expire=3.weeks.ago", ""},
		{"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=gc.pruneExpire GIT_CONFIG_VALUE_0=now git status", ""},
		{"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=gc.auto GIT_CONFIG_VALUE_0=0 git commit -m x", ""},
		{"GIT_CONFIG_KEY_0=gc.pruneExpire GIT_CONFIG_VALUE_0=3.weeks.ago git gc", ""},
		{`GIT_CONFIG_PARAMETERS="'core.pager'='less'" git commit -m x`, ""},
		{"GIT_CONFIG_PARAMETERS= git commit -m x", ""},
		{"GIT_CONFIG_GLOBAL=/dev/null git commit -m x", ""},
		{"git commit -m x; GIT_CONFIG_KEY_0=gc.pruneExpire GIT_CONFIG_VALUE_0=now git status", ""},
		{"echo 'GIT_CONFIG_KEY_0=gc.pruneExpire' && git commit -m x", ""},
		{"git maintenance run --task=gc", ""},
		{"git maintenance run", ""},
		// A repack that keeps what no ref holds, or deletes no pack.
		{"git repack", ""},
		{"git repack -d", ""},
		{"git repack -a", ""},
		{"git repack -A -d", ""},
		{"git repack -a -A -d", ""},
		{"git repack -a -d -k", ""},
		{"git repack -ad --keep-unreachable", ""},
		{"git repack --cruft -d", ""},
		{"git repack -a -d --cruft", ""},
		{"git repack --cruft --no-cruft -d", ""},
		{"git repack --cruft -d --cruft-expiration=3.weeks.ago", ""},
		{"git repack --cruft -d --no-cruft-expiration", ""},
		{"git repack -A -d --unpack-unreachable=3.weeks.ago", ""},
		{"git repack -A -d --no-unpack-unreachable", ""},
		{"git repack -a -d --unpack-unreachable=3.weeks.ago", ""},
		{"git repack -ad --unpack-unreachable 2.weeks.ago", ""},
		{"git repack -d -g 2", ""},
		{"git repack -d --geometric=2 -m", ""},
		{"git repack -adkq --write-midx", ""},
		// Commands whose names sound like a loss but keep git's defaults.
		{"git fetch --prune", ""},
		{"git remote prune origin", ""},
		{"git worktree prune", ""},
		// The reflog-recoverable forms stay with the user.
		{"git branch -D x", ""},
		{"git branch -f x main", ""},
		{"git branch -M x y", ""},
		{"git update-ref -d refs/heads/x", ""},
		{"git checkout -B x", ""},
		{"git switch -C x", ""},
		// Text no shell runs.
		{"git commit -m 'git gc --prune=now'", ""},
		{"ls # git prune", ""},
		{"cat <<'EOF'\ngit prune\nEOF", ""},
	}
	clean := discardFixture{repo: "github.com/mad01/thismoon"}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(config.Config{}, clean, &warned)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			switch want := "belt[git-discard]: " + finalLossReason(tt.want); {
			case tt.want == "" && d != nil:
				t.Fatalf("Check(%q) = %q, want allow", tt.command, d.Reason)
			case tt.want != "" && d == nil:
				t.Fatalf("Check(%q) allowed, want %q", tt.command, want)
			case tt.want != "" && d.Reason != want:
				t.Errorf("Check(%q) = %q, want %q", tt.command, d.Reason, want)
			}
		})
	}
	if !strings.Contains(finalLossReason(gcFinalLoss), "`! <command>`") {
		t.Errorf("final loss reason %q does not hand over `! <command>`", finalLossReason(gcFinalLoss))
	}
}

// TestGitDiscardForcedFetch pins the forced fetch or pull into a local
// branch: it denies when a branch it may move holds a commit no
// remote-tracking ref holds, and passes a fetch that forces nothing into a
// branch, a branch with nothing unpushed and a branch that does not exist.
// The refspecs come from the words, the call's settings and the
// repository's config, which holds git's default one here.
func TestGitDiscardForcedFetch(t *testing.T) {
	const tip = "85cb511cd261ccf4399a7b52ad32a21c9f9b18f1"
	fixture := discardFixture{
		repo: "github.com/mad01/thismoon",
		unpushed: map[string]string{
			"refs/heads/feat":         tip,
			"--glob=refs/heads/*":     tip,
			"--glob=refs/heads/fe*":   tip,
			"--branches":              tip,
			"refs/heads/fully-pushed": "",
		},
		refspecs: []string{"+refs/heads/*:refs/remotes/origin/*"},
	}
	tests := []struct {
		command string
		want    string // the branches the reason names; "" allows
	}{
		{"git fetch origin +main:feat", "the branch `feat`"},
		{"git fetch -f origin main:feat", "the branch `feat`"},
		{"git fetch --force origin main:feat", "the branch `feat`"},
		{"git fetch --forc origin main:feat", "the branch `feat`"},
		{"git fetch -qf origin main:feat", "the branch `feat`"},
		{"git fetch origin +main:refs/heads/feat", "the branch `feat`"},
		{"git fetch origin +main:heads/feat", "the branch `feat`"},
		{"git fetch origin +refs/heads/main:refs/heads/feat", "the branch `feat`"},
		{"git fetch origin '+main:feat'", "the branch `feat`"},
		{"git fetch origin main +main:feat", "the branch `feat`"},
		{"git fetch origin tag v1 +main:feat", "the branch `feat`"},
		{"git fetch -- origin +main:feat", "the branch `feat`"},
		{"git fetch origin '+refs/heads/*:refs/heads/*'", "the branches `refs/heads/*`"},
		{"git fetch origin '+refs/heads/*:refs/heads/fe*'", "the branches `refs/heads/fe*`"},
		{"git fetch --refmap='+refs/heads/*:refs/heads/*' origin main", "the branches `refs/heads/*`"},
		{"git -C /other fetch -f origin main:feat", "the branch `feat`"},
		{"cd /other && git fetch origin +main:feat", "the branch `feat`"},
		{"sh -c 'git fetch origin +main:feat'", "the branch `feat`"},
		// Destinations belt cannot read may be any branch.
		{`git fetch origin "+main:$B"`, anyBranch.what},
		{"git fetch origin +main:fe{at,x}", anyBranch.what},
		{"git fetch -f origin main:feat..x", anyBranch.what},
		{"git fetch --stdin origin", anyBranch.what},
		{"git fetch --frobnicate origin +main:feat", anyBranch.what},
		{"echo +main:feat | xargs git fetch origin", anyBranch.what},
		{"git fetch origin '^refs/heads/x'", anyBranch.what}, // a leading ^ may be a zsh glob
		// A pull hands its refspecs and -f to fetch.
		{"git pull origin +main:feat", "the branch `feat`"},
		{"git pull --force origin main:feat", "the branch `feat`"},
		{"git pull -f origin main:feat", "the branch `feat`"},
		{"git pull --forc origin main:feat", "the branch `feat`"},
		{"git pull --rebase origin +main:feat", "the branch `feat`"},
		{"git pull -r --ff-only -j4 --log=3 origin +main:feat", "the branch `feat`"},
		{"git pull origin '+refs/heads/*:refs/heads/*'", "the branches `refs/heads/*`"},
		{"git pull --refmap='+refs/heads/*:refs/heads/*' origin main", "the branches `refs/heads/*`"},
		{"git pull --frobnicate origin main", anyBranch.what},
		{"echo +main:feat | xargs git pull origin", anyBranch.what},
		// A remote.<name>.fetch setting is a refspec too, with refspecs on the
		// command line or none.
		{"git -c remote.origin.fetch=+refs/heads/*:refs/heads/* fetch origin", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=+refs/heads/*:refs/heads/*' fetch --all", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=+refs/heads/*:refs/heads/*' pull", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=+refs/heads/*:refs/heads/*' fetch origin main", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' fetch -f origin", "the branches `refs/heads/*`"},
		{"git -c Remote.Origin.Fetch=+main:feat fetch origin", "the branch `feat`"},
		{"git -c 'remote.origin.fetch=+refs/*:refs/*' fetch origin", "every branch `refs/*` covers"},
		{"git fetch origin '+refs/*:refs/*'", "every branch `refs/*` covers"},
		{"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=remote.origin.fetch GIT_CONFIG_VALUE_0=+main:feat git fetch origin", "the branch `feat`"},
		{"export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=remote.origin.fetch GIT_CONFIG_VALUE_0=+main:feat; git pull", "the branch `feat`"},
		{`GIT_CONFIG_PARAMETERS="'remote.origin.fetch'='+main:feat'" git fetch origin`, "the branch `feat`"},
		{"git --config-env=remote.origin.fetch=SPEC fetch origin", anyBranch.what},
		{`git -c "remote.origin.fetch=$S" fetch`, anyBranch.what},
		// Nothing forced into a branch, or nothing there to lose.
		{"git fetch", ""},
		{"git fetch origin", ""},
		{"git fetch origin main", ""},
		{"git fetch -f origin", ""},
		{"git fetch -f origin main", ""},
		{"git fetch origin +main", ""},
		{"git fetch origin main:feat", ""}, // git refuses a non-fast-forward
		{"git fetch origin main:refs/remotes/origin/main", ""},
		{"git fetch origin +main:refs/remotes/origin/main", ""},
		{"git fetch -f origin main:remotes/origin/x", ""},
		{"git fetch origin '+refs/heads/*:refs/remotes/origin/*'", ""},
		{"git fetch -f origin main:refs/tags/x", ""},
		{"git fetch -f origin tag v1", ""},
		{"git fetch --multiple origin upstream", ""},
		{"git fetch --all --prune", ""},
		{"git fetch --depth 1 origin main", ""},
		{"git fetch -j 4 -o x origin main", ""},
		{"git fetch origin +main:new-branch", ""},
		{"git fetch -f origin main:fully-pushed", ""},
		{"git fetch --dry-run origin +main:feat", ""},
		{"git fetch --prune", ""},
		{"git pull", ""},
		{"git pull --rebase", ""},
		{"git pull --ff-only", ""},
		{"git pull origin main", ""},
		{"git pull -f origin main", ""},
		{"git pull -r -j4 --log=3 -S origin main", ""},
		{"git pull --dry-run origin +main:feat", ""},
		{"git pull origin +main:fully-pushed", ""},
		{"git pull origin +main:refs/remotes/origin/main", ""},
		{"git -c remote.origin.fetch=+refs/heads/*:refs/remotes/origin/* fetch origin", ""},
		{"git -c remote.origin.fetch=refs/heads/*:refs/heads/* fetch origin", ""}, // git refuses a non-fast-forward
		{"git -c remote.origin.url=/x fetch origin", ""},
		{"git -c remote.origin.fetch=+main:fully-pushed pull", ""},
		{"GIT_CONFIG_KEY_0=remote.origin.prune GIT_CONFIG_VALUE_0=true git fetch", ""},
		{"git -c 'remote.origin.fetch=+refs/*:refs/*' fetch --dry-run origin", ""},
		{"git commit -m 'git fetch -f origin main:feat'", ""},
		// A remote update fetches with the settings and the repository's
		// refspecs, as a fetch --all does.
		{"git -c 'remote.origin.fetch=+refs/heads/*:refs/heads/*' remote update", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=+refs/heads/*:refs/heads/*' remote update origin", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=+main:feat' remote -v update --prune", "the branch `feat`"},
		{"GIT_CONFIG_PARAMETERS=\"'remote.origin.fetch'='+main:feat'\" git remote update", "the branch `feat`"},
		{`git -c "remote.origin.fetch=$S" remote update`, anyBranch.what},
		{"git remote update", ""},
		{"git remote update --prune", ""},
		{"git remote update -p origin upstream", ""},
		{"git remote -v", ""},
		{"git remote add upstream /x", ""},
		{"git remote $X", ""},
		{"git -c 'remote.origin.fetch=+main:fully-pushed' remote update", ""},
		// A refspec an earlier step writes to a config file counts.
		{"git config --add remote.origin.fetch '+refs/heads/*:refs/heads/*' && git fetch", "the branches `refs/heads/*`"},
		{"git config --add remote.origin.fetch +refs/heads/*:refs/heads/* && git fetch", "the branches `refs/heads/*`"},
		{"git config remote.origin.fetch +main:feat && git pull", "the branch `feat`"},
		{"git config --replace-all remote.origin.fetch +main:feat && git fetch origin", "the branch `feat`"},
		{"git config set --append remote.origin.fetch +main:feat; git fetch", "the branch `feat`"},
		{"git config --global --add remote.origin.fetch +main:feat && git remote update", "the branch `feat`"},
		{"git config --add remote.origin.fetch \"$S\" && git fetch", anyBranch.what},
		{"git remote add --mirror=fetch backup /x && git fetch backup", "every branch `refs/*` covers"},
		{"git remote add --mirror backup /x && git fetch backup", "every branch `refs/*` covers"},
		{"git remote add -t heads/feat --mirror=fetch backup /x && git fetch backup", "the branch `feat`"},
		{"git remote add -t \"$B\" --mirror=fetch backup /x && git fetch backup", "every branch `refs/*` covers"},
		{"git remote add --mirror=push backup /x && git fetch backup", ""},
		{"git remote add backup /x && git fetch backup", ""},
		// A remote add -f fetches through the refspecs it writes.
		{"git remote add -f --mirror=fetch backup /x", "every branch `refs/*` covers"},
		{"git remote add --fetch --mirror backup /x", "every branch `refs/*` covers"},
		{"git remote add --mirror -f backup /x", "every branch `refs/*` covers"},
		{"git remote -v add -f --mirror=fetch backup /x", "every branch `refs/*` covers"},
		{"git remote add -f -t heads/feat --mirror=fetch backup /x", "the branch `feat`"},
		{"git remote add -f --mirror=fetch --frobnicate backup /x", "every branch `refs/*` covers"},
		{"echo backup /x | xargs git remote add -f", "every branch `refs/*` covers"},
		{"git -c remote.origin.fetch=+main:feat remote add -f backup /x", "the branch `feat`"},
		{"git remote add -f backup /x", ""},
		{"git remote add -f -t main backup /x", ""},
		{"git remote add -f --mirror=push backup /x", ""},
		{"git remote add -f --no-fetch --mirror backup /x", ""},
		{"git remote add -f -t heads/fully-pushed --mirror=fetch backup /x", ""},
		{"git remote set-branches origin feat && git fetch", ""},
		{"git config --get remote.origin.fetch && git fetch", ""},
		{"git config --unset remote.origin.fetch && git fetch", ""},
		{"git fetch && git config --add remote.origin.fetch +main:feat", ""},
		{"git config --add remote.origin.fetch +main:fully-pushed && git fetch", ""},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(config.Config{}, fixture, &warned)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			switch {
			case tt.want == "" && d != nil:
				t.Fatalf("Check(%q) = %q, want allow", tt.command, d.Reason)
			case tt.want != "" && d == nil:
				t.Fatalf("Check(%q) allowed, want a deny naming %s", tt.command, tt.want)
			case tt.want == "":
			case !strings.HasPrefix(d.Reason, "belt[git-discard]: this force-fetches into a local branch: "+
				"it may move "+tt.want+" off commit "+tip[:shortOIDLen]):
				t.Errorf("Check(%q) = %q, want it to name %s and the commit", tt.command, d.Reason, tt.want)
			case !strings.Contains(d.Reason, "`git branch <name> "+tip[:shortOIDLen]+"`"):
				t.Errorf("Check(%q) = %q, want the branch command that keeps the commit", tt.command, d.Reason)
			}
		})
	}
}

// TestGitDiscardFetchPrune pins the pruning fetch: one that prunes with a
// pattern refspec into refs/heads deletes the local branches the remote
// lacks, forced or not, so it denies when a branch the destination matches
// holds a commit no remote-tracking ref holds. The prune comes from the
// words, any setting the call runs with or an earlier write; the
// repository's config holds git's default refspec here, and its own prune
// keys are pinned in TestGitDiscardDecision.
func TestGitDiscardFetchPrune(t *testing.T) {
	const tip = "85cb511cd261ccf4399a7b52ad32a21c9f9b18f1"
	fixture := discardFixture{
		repo: "github.com/mad01/thismoon",
		unpushed: map[string]string{
			"--glob=refs/heads/*":   tip,
			"--glob=refs/heads/fe*": tip,
			"--branches":            tip,
			"refs/heads/feat":       tip,
		},
		refspecs: []string{"+refs/heads/*:refs/remotes/origin/*"},
	}
	const heads = "'refs/heads/*:refs/heads/*'"
	tests := []struct {
		command string
		want    string // the branches the reason names; "" allows
	}{
		{"git fetch --prune origin " + heads, "the branches `refs/heads/*`"},
		{"git fetch -p origin " + heads, "the branches `refs/heads/*`"},
		{"git fetch -qp origin " + heads, "the branches `refs/heads/*`"},
		{"git fetch --no-prune --prune origin " + heads, "the branches `refs/heads/*`"},
		{"git fetch --prune origin 'refs/heads/fe*:refs/heads/fe*'", "the branches `refs/heads/fe*`"},
		{"git fetch --prune origin 'refs/*:refs/*'", "every branch `refs/*` covers"},
		{"git fetch --prune --refmap=" + heads + " origin main", "the branches `refs/heads/*`"},
		{"git pull --prune origin " + heads, "the branches `refs/heads/*`"},
		{"git -c fetch.prune=true fetch origin " + heads, "the branches `refs/heads/*`"},
		{"git -c fetch.prune fetch origin " + heads, "the branches `refs/heads/*`"}, // no value is true
		{"git -c Fetch.Prune=1 pull origin " + heads, "the branches `refs/heads/*`"},
		{"git -c remote.origin.prune=yes fetch origin " + heads, "the branches `refs/heads/*`"},
		{`git -c "fetch.prune=$P" fetch origin ` + heads, "the branches `refs/heads/*`"},
		{"git --config-env=fetch.prune=P fetch origin " + heads, "the branches `refs/heads/*`"},
		{"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=fetch.prune GIT_CONFIG_VALUE_0=true git fetch origin " + heads, "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' fetch --prune", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' -c fetch.prune=true remote update", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote update --prune", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote update -p origin", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote update --frobnicate", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote $X", "the branches `refs/heads/*`"},
		{"git config fetch.prune true && git fetch origin " + heads, "the branches `refs/heads/*`"},
		{"git config --add remote.origin.fetch " + heads + " && git fetch --prune", "the branches `refs/heads/*`"},
		{"git config remote.origin.prune true; git config --add remote.origin.fetch " + heads + "; git remote update", "the branches `refs/heads/*`"},
		// A remote prune deletes through the refspecs, a + one too, and
		// through an exact one written into refs/heads/.
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote prune origin", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=+refs/heads/*:refs/heads/*' remote prune origin", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=+refs/*:refs/*' remote prune origin", "every branch `refs/*` covers"},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote -v prune origin upstream", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote prune --no-dry-run origin", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote prune -n --no-dry-run origin", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote prune --frobnicate origin", "the branches `refs/heads/*`"},
		{"git -c 'remote.origin.fetch=main:refs/heads/feat' remote prune origin", "the branch `feat`"},
		{"git -c 'remote.origin.fetch=+refs/heads/gone:refs/heads/feat' remote prune origin", "the branch `feat`"},
		{"git config --add remote.origin.fetch " + heads + " && git remote prune origin", "the branches `refs/heads/*`"},
		{"echo origin | xargs git -c remote.origin.fetch=" + heads + " remote prune -n", "the branches `refs/heads/*`"},
		// A fetch that prunes nothing, or nothing in refs/heads.
		{"git fetch --prune", ""},
		{"git fetch -p origin", ""},
		{"git fetch --all --prune", ""},
		{"git fetch --prune origin main", ""},
		{"git fetch origin " + heads, ""}, // git refuses a non-fast-forward
		{"git fetch --prune --no-prune origin " + heads, ""},
		{"git -c fetch.prune=true fetch --no-prune origin " + heads, ""},
		{"git -c fetch.prune=false fetch origin " + heads, ""},
		{"git -c fetch.prune=off fetch origin " + heads, ""},
		{"git -c fetch.prune=0 fetch origin " + heads, ""},
		{"git -c fetch.prune= fetch origin " + heads, ""},
		{"git -c fetch.prunetags=true fetch origin " + heads, ""},
		{"git fetch -P origin " + heads, ""}, // --prune-tags alone prunes nothing
		{"git fetch --prune --prune-tags", ""},
		{"git fetch --prune origin refs/heads/main:refs/heads/feat", ""}, // an exact refspec prunes nothing
		{"git fetch --prune origin 'refs/heads/*:refs/remotes/origin/*'", ""},
		{"git fetch --prune origin 'refs/tags/*:refs/tags/*'", ""},
		{"git fetch --prune origin 'refs/heads/*:refs/heads/pushed/*'", ""},
		{"git fetch --dry-run --prune origin " + heads, ""},
		{"git pull --prune", ""},
		{"git pull --rebase", ""},
		{"git -c fetch.prune=true pull --rebase", ""},
		{"git remote update --prune", ""},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote update", ""},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote update --no-prune", ""},
		{"git config fetch.prune false && git fetch origin " + heads, ""},
		{"git remote prune origin", ""},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote prune -n origin", ""},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' remote prune --dry-run origin", ""},
		{"git -c 'remote.origin.fetch=refs/heads/*:refs/remotes/origin/*' remote prune origin", ""},
		{"git -c 'remote.origin.fetch=+main:feat' remote prune origin", ""}, // git compares refs/heads/feat with feat
		{"git -c 'remote.origin.fetch=refs/heads/main:heads/feat' remote prune origin", ""},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(config.Config{}, fixture, &warned)
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			switch {
			case tt.want == "" && d != nil:
				t.Fatalf("Check(%q) = %q, want allow", tt.command, d.Reason)
			case tt.want != "" && d == nil:
				t.Fatalf("Check(%q) allowed, want a deny naming %s", tt.command, tt.want)
			case tt.want == "":
			case d.Reason != "belt[git-discard]: "+fetchLossReason(prunedReason(tt.want, tip)):
				t.Errorf("Check(%q) = %q, want it to name %s and the commit", tt.command, d.Reason, tt.want)
			}
		})
	}
}

// prunedReason is the deny reason for a prune that may delete the
// branches what names, one or many, with commit tip.
func prunedReason(what, tip string) error {
	reflogs := " and their reflogs where the remote lacks them"
	if strings.HasPrefix(what, "the branch `") {
		reflogs = " and its reflog where the remote lacks it"
	}
	return errors.New("prunes local branches: it may delete " + what + reflogs + ", with commit " +
		tip[:shortOIDLen] + ", which no remote-tracking ref holds; push it first")
}

// TestGitDiscardFetchCalls pins the git calls a fetch costs: a plain fetch,
// a pruning one and a remote update with git's default refspec read the
// config once and count nothing, and a dry run or another remote
// subcommand reads nothing.
func TestGitDiscardFetchCalls(t *testing.T) {
	tests := []struct {
		command         string
		reads, unpushed int
	}{
		{"git fetch", 1, 0},
		{"git fetch --prune", 1, 0},
		{"git -c fetch.prune=true fetch", 1, 0},
		{"git remote update --prune", 1, 0},
		{"git pull --prune", 1, 0},
		{"git fetch --dry-run --prune", 0, 0},
		{"git remote add upstream /x", 0, 0},
		{"git fetch --prune origin 'refs/heads/*:refs/heads/*'", 1, 1},
		{"git remote prune origin", 1, 0},
		{"git remote prune -n origin", 0, 0},
		{"git remote add -f upstream /x", 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var warned []string
			g := newDiscardGuard(config.Config{}, discardFixture{
				repo:     "github.com/mad01/thismoon",
				refspecs: []string{"+refs/heads/*:refs/remotes/origin/*"},
			}, &warned)
			var reads, unpushed int
			read, count := g.repoFetch, g.unpushed
			g.repoFetch = func(ctx context.Context, dir string) (fetchConfig, error) {
				reads++
				return read(ctx, dir)
			}
			g.unpushed = func(ctx context.Context, dir, rev string) (string, error) {
				unpushed++
				return count(ctx, dir, rev)
			}
			if d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"}); d != nil {
				t.Fatalf("Check(%q) = %q, want allow", tt.command, d.Reason)
			}
			if reads != tt.reads || unpushed != tt.unpushed {
				t.Errorf("Check(%q) read the config %d times and counted %d, want %d and %d",
					tt.command, reads, unpushed, tt.reads, tt.unpushed)
			}
		})
	}
}

// TestParseFetchConfig pins how the repository's fetch config reads: a
// remote whose last remote.<name>.mirror is true holds its refspecs back
// for the bare check when the last core.bare is true, and counts them
// otherwise, and the last value of each prune key decides, a key with no
// value being true.
func TestParseFetchConfig(t *testing.T) {
	const bare = "core.bare\ntrue"
	mirrorSpec := []string{"+refs/*:refs/*"}
	tests := []struct {
		name    string
		entries []string
		want    fetchConfig
	}{
		{
			name:    "a mirror clone",
			entries: []string{bare, "remote.origin.fetch\n+refs/*:refs/*", "remote.origin.mirror\ntrue"},
			want:    fetchConfig{bareMirrors: mirrorSpec},
		},
		{
			name:    "the mirror key first",
			entries: []string{"remote.origin.mirror\ntrue", "remote.origin.fetch\n+refs/*:refs/*", bare},
			want:    fetchConfig{bareMirrors: mirrorSpec},
		},
		{
			name:    "a mirror key with no value",
			entries: []string{bare, "remote.origin.fetch\n+refs/*:refs/*", "remote.origin.mirror"},
			want:    fetchConfig{bareMirrors: mirrorSpec},
		},
		{
			name:    "a core.bare key with no value",
			entries: []string{"core.bare", "remote.origin.fetch\n+refs/*:refs/*", "remote.origin.mirror"},
			want:    fetchConfig{bareMirrors: mirrorSpec},
		},
		{
			name:    "a remote name with a dot",
			entries: []string{bare, "remote.a.b.fetch\n+refs/*:refs/*", "remote.a.b.mirror\nyes"},
			want:    fetchConfig{bareMirrors: mirrorSpec},
		},
		{
			name: "a later false mirror key",
			entries: []string{
				bare, "remote.origin.fetch\n+refs/*:refs/*", "remote.origin.mirror\ntrue", "remote.origin.mirror\nfalse",
			},
			want: fetchConfig{refspecs: mirrorSpec},
		},
		{
			name:    "a mirror remote with a work tree",
			entries: []string{"core.bare\nfalse", "remote.m.fetch\n+refs/*:refs/*", "remote.m.mirror\ntrue"},
			want:    fetchConfig{refspecs: mirrorSpec},
		},
		{
			name:    "a mirror remote with no core.bare",
			entries: []string{"remote.m.fetch\n+refs/*:refs/*", "remote.m.mirror\ntrue"},
			want:    fetchConfig{refspecs: mirrorSpec},
		},
		{
			name:    "a later false core.bare",
			entries: []string{bare, "remote.m.fetch\n+refs/*:refs/*", "remote.m.mirror\ntrue", "core.bare\nno"},
			want:    fetchConfig{refspecs: mirrorSpec},
		},
		{
			name: "a mirror beside a remote that is none",
			entries: []string{
				bare, "remote.origin.fetch\n+refs/*:refs/*", "remote.origin.mirror\ntrue",
				"remote.up.fetch\n+refs/heads/*:refs/heads/*", "remote.up.fetch\n+refs/tags/*:refs/tags/*",
			},
			want: fetchConfig{
				refspecs:    []string{"+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*"},
				bareMirrors: mirrorSpec,
			},
		},
		{
			name:    "a bare repository with no mirror",
			entries: []string{bare, "remote.origin.fetch\n+refs/heads/*:refs/heads/*"},
			want:    fetchConfig{refspecs: []string{"+refs/heads/*:refs/heads/*"}},
		},
		{
			name:    "git's default refspec",
			entries: []string{"core.bare\nfalse", "remote.origin.fetch\n+refs/heads/*:refs/remotes/origin/*"},
			want:    fetchConfig{refspecs: []string{"+refs/heads/*:refs/remotes/origin/*"}},
		},
		{name: "fetch.prune", entries: []string{"fetch.prune\ntrue"}, want: fetchConfig{prune: true}},
		{name: "fetch.prune with no value", entries: []string{"fetch.prune"}, want: fetchConfig{prune: true}},
		{name: "a later false fetch.prune", entries: []string{"fetch.prune\ntrue", "fetch.prune\nfalse"}},
		{name: "remote.<name>.prune", entries: []string{"remote.origin.prune\n1"}, want: fetchConfig{prune: true}},
		{name: "a false remote.<name>.prune", entries: []string{"remote.origin.prune\noff"}},
		{
			name:    "a remote named x.fetch",
			entries: []string{"remote.x.fetch.prune\ntrue"},
			want:    fetchConfig{prune: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFetchConfig(tt.entries)
			if !slices.Equal(got.refspecs, tt.want.refspecs) || got.prune != tt.want.prune ||
				!slices.Equal(got.bareMirrors, tt.want.bareMirrors) {
				t.Errorf("parseFetchConfig(%q) = %+v, want %+v", tt.entries, got, tt.want)
			}
		})
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
			name:    "the new tree writers pass on a clean tree",
			fixture: discardFixture{repo: repo},
			command: "git apply -R p.diff; git read-tree -u --reset HEAD; " +
				"git checkout-index -f -a; git rm -rf .; git mv -f a b",
			cwd: "/repo",
		},
		{
			name: "a forced worktree remove is judged in the worktree, not the cwd",
			fixture: discardFixture{
				repo: repo, dirty: map[string]bool{"/repo": true}, worktrees: []string{"/repo", "/wt"},
			},
			command: "git worktree remove -f ../wt", cwd: "/repo",
		},
		{
			name: "a forced remove of a dirty worktree denies from a clean cwd",
			fixture: discardFixture{
				repo: repo, dirty: map[string]bool{"/wt": true}, worktrees: []string{"/repo", "/wt"},
			},
			command: "git -C /repo worktree remove --force /wt", cwd: "/elsewhere", wantDeny: true,
		},
		{
			name: "every worktree a suffix may name is judged",
			fixture: discardFixture{
				repo:      repo,
				dirty:     map[string]bool{"/b/WT": true},
				worktrees: []string{"/repo", "/a/wt", "/b/WT"},
			},
			command: "git worktree remove -f wt", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a worktree name that matches no worktree denies",
			fixture: discardFixture{repo: repo, worktrees: []string{"/repo"}},
			command: "git worktree remove -f gone", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a worktree list that fails denies",
			fixture: discardFixture{repo: repo},
			command: "git worktree remove -f /wt", cwd: "/repo", wantDeny: true,
		},
		{
			name: "a plain remove of a detached HEAD with commits only it holds denies",
			fixture: discardFixture{
				repo: repo, worktrees: []string{"/repo", "/wt"}, stranded: map[string]int{"/wt": 2},
			},
			command: "git worktree remove /wt", cwd: "/repo", wantDeny: true,
		},
		{
			name: "a forced remove of a clean detached HEAD with commits only it holds denies",
			fixture: discardFixture{
				repo: repo, worktrees: []string{"/repo", "/wt"}, stranded: map[string]int{"/wt": 1},
			},
			command: "git worktree remove -f /wt", cwd: "/repo", wantDeny: true,
		},
		{
			name: "a detached HEAD whose commits belt cannot count denies",
			fixture: discardFixture{
				repo: repo, worktrees: []string{"/repo", "/wt"}, stranded: map[string]int{"/wt": -1},
			},
			command: "git worktree remove /wt", cwd: "/repo", wantDeny: true,
		},
		{
			name: "a detached HEAD that a branch holds passes",
			fixture: discardFixture{
				repo:      repo,
				dirty:     map[string]bool{"/wt": true},
				worktrees: []string{"/repo", "/wt"},
				stranded:  map[string]int{"/wt": 0},
			},
			command: "git worktree remove /wt", cwd: "/repo",
		},
		{
			name: "a remove of a branch whose HEAD reflog alone holds commits denies",
			fixture: discardFixture{
				repo: repo, worktrees: []string{"/repo", "/wt"}, stranded: map[string]int{"/wt": 3},
			},
			command: "git worktree remove /wt", cwd: "/repo", wantDeny: true,
		},
		{
			name: "a remove of the main worktree is not counted, since git refuses it",
			fixture: discardFixture{
				repo: repo, worktrees: []string{"/repo", "/wt"}, stranded: map[string]int{"/repo": -1},
			},
			command: "git worktree remove /repo", cwd: "/wt",
		},
		{
			name:    "a prune that strands commits denies",
			fixture: discardFixture{repo: repo, pruned: 2},
			command: "git worktree prune -v", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a prune whose commits belt cannot count denies",
			fixture: discardFixture{repo: repo, pruned: -1},
			command: "git worktree prune", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a dry-run prune passes",
			fixture: discardFixture{repo: repo, pruned: -1},
			command: "git worktree prune -n; git worktree prune --no-dry-run --dry-run", cwd: "/repo",
		},
		{
			name:    "a dry-run prune a command in front of git may hand more words denies",
			fixture: discardFixture{repo: repo, pruned: 1},
			command: "echo --no-dry-run | xargs git worktree prune -n", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a prune that strands nothing passes",
			fixture: discardFixture{repo: repo},
			command: "git worktree prune --expire 3.days.ago", cwd: "/repo",
		},
		{
			name:    "a plain remove belt cannot place denies",
			fixture: discardFixture{repo: repo, worktrees: []string{"/repo", "/wt"}},
			command: "git worktree remove gone", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a forced mv checks ignored files too",
			fixture: discardFixture{repo: repo, ignoredDirty: map[string]bool{"/repo": true}},
			command: "git mv -f a.txt build.log", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a checkout-index --prefix export denies on a clean tree",
			fixture: discardFixture{repo: repo},
			command: "git checkout-index -f -a --prefix=/tmp/export/", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "an apply outside the worktree denies on a clean tree",
			fixture: discardFixture{repo: repo},
			command: "git apply --unsafe-paths p.diff", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a read-tree into another index file denies on a clean tree",
			fixture: discardFixture{repo: repo},
			command: "git read-tree -m -u --index-output=/tmp/idx HEAD", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a read-tree into another index file without -u denies on a clean tree",
			fixture: discardFixture{repo: repo},
			command: "git read-tree --index-output=a.txt HEAD", cwd: "/repo", wantDeny: true,
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
		{
			name: "allow_repos exempts a final loss",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {AllowRepos: []string{"github.com/mad01/*"}},
			}},
			fixture: discardFixture{repo: repo},
			command: "git gc --prune=now", cwd: "/repo",
		},
		{
			name: "soft mode warns on a final loss",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {Mode: config.ModeSoft},
			}},
			fixture: discardFixture{repo: repo},
			command: "git prune", cwd: "/repo", wantWarn: true,
		},
		{
			name:    "a final loss in an unresolvable directory denies",
			fixture: discardFixture{repo: repo},
			command: `cd "$(mktemp -d)" && git prune`, cwd: "/repo", wantDeny: true,
		},
		{
			name: "allow_repos exempts a forced fetch",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {AllowRepos: []string{"github.com/mad01/*"}},
			}},
			fixture: discardFixture{repo: repo, unpushed: map[string]string{"refs/heads/feat": "85cb511"}},
			command: "git fetch origin +main:feat", cwd: "/repo",
		},
		{
			name: "soft mode warns on a forced fetch",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {Mode: config.ModeSoft},
			}},
			fixture: discardFixture{repo: repo, unpushed: map[string]string{"refs/heads/feat": "85cb511"}},
			command: "git fetch -f origin main:feat", cwd: "/repo", wantWarn: true,
		},
		{
			name:    "a forced fetch in an unresolvable directory denies",
			fixture: discardFixture{repo: repo},
			command: `cd "$(mktemp -d)" && git fetch origin +main:feat`, cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a forced fetch whose count fails denies",
			fixture: discardFixture{repo: repo, unpushedFails: true},
			command: "git fetch origin +main:feat", cwd: "/repo", wantDeny: true,
		},
		{
			name: "a forced refspec in the repository's config denies a plain fetch",
			fixture: discardFixture{
				repo: repo, refspecs: []string{"+refs/heads/*:refs/heads/*"},
				unpushed: map[string]string{"--glob=refs/heads/*": "85cb511"},
			},
			command: "git fetch origin", cwd: "/repo", wantDeny: true,
		},
		{
			name: "a forced refspec in the repository's config denies a pull",
			fixture: discardFixture{
				repo: repo, refspecs: []string{"+refs/heads/main:refs/heads/feat"},
				unpushed: map[string]string{"refs/heads/feat": "85cb511"},
			},
			command: "git pull --rebase", cwd: "/repo", wantDeny: true,
		},
		{
			name: "a forced refspec in the repository's config onto pushed branches passes",
			fixture: discardFixture{
				repo: repo, refspecs: []string{"+refs/heads/*:refs/heads/*"},
			},
			command: "git fetch origin", cwd: "/repo",
		},
		{
			name: "a failed config read denies a fetch when a branch holds unpushed work",
			fixture: discardFixture{
				repo: repo, refspecsFail: true, unpushed: map[string]string{"--branches": "85cb511"},
			},
			command: "git fetch", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a failed config read passes a fetch when nothing is unpushed",
			fixture: discardFixture{repo: repo, refspecsFail: true},
			command: "git fetch", cwd: "/repo",
		},
		{
			name: "a fetch in an unresolvable directory that forces nothing passes",
			fixture: discardFixture{
				repo: repo, refspecsFail: true, unpushed: map[string]string{"--branches": "85cb511"},
			},
			command: `cd "$(mktemp -d)" && git pull`, cwd: "/repo",
		},
		{
			name: "fetch.prune in the repository's config denies a fetch with its pattern refspec into refs/heads",
			fixture: discardFixture{
				repo: repo, refspecs: []string{"refs/heads/*:refs/heads/*"}, prunes: true,
				unpushed: map[string]string{"--glob=refs/heads/*": "85cb511"},
			},
			command: "git fetch", cwd: "/repo", wantDeny: true,
		},
		{
			name: "fetch.prune in the repository's config denies a remote update",
			fixture: discardFixture{
				repo: repo, refspecs: []string{"refs/heads/*:refs/heads/*"}, prunes: true,
				unpushed: map[string]string{"--glob=refs/heads/*": "85cb511"},
			},
			command: "git remote update origin", cwd: "/repo", wantDeny: true,
		},
		{
			name: "--no-prune overrides fetch.prune in the repository's config",
			fixture: discardFixture{
				repo: repo, refspecs: []string{"refs/heads/*:refs/heads/*"}, prunes: true,
				unpushed: map[string]string{"--glob=refs/heads/*": "85cb511"},
			},
			command: "git fetch --no-prune", cwd: "/repo",
		},
		{
			name: "a pattern refspec into refs/heads in the repository's config passes a fetch that does not prune",
			fixture: discardFixture{
				repo: repo, refspecs: []string{"refs/heads/*:refs/heads/*"},
				unpushed: map[string]string{"--glob=refs/heads/*": "85cb511"},
			},
			command: "git remote update", cwd: "/repo",
		},
		{
			name: "a remote update that may get -p from xargs denies with a pattern refspec into refs/heads",
			fixture: discardFixture{
				repo: repo, refspecs: []string{"refs/heads/*:refs/heads/*"},
				unpushed: map[string]string{"--glob=refs/heads/*": "85cb511"},
			},
			command: "echo -p | xargs git remote update", cwd: "/repo", wantDeny: true,
		},
		{
			name: "a pruning fetch whose count fails denies",
			fixture: discardFixture{
				repo: repo, refspecs: []string{"refs/heads/*:refs/heads/*"}, unpushedFails: true,
			},
			command: "git fetch --prune", cwd: "/repo", wantDeny: true,
		},
		{
			name: "soft mode warns on a pruning fetch",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {Mode: config.ModeSoft},
			}},
			fixture: discardFixture{
				repo: repo, refspecs: []string{"refs/heads/*:refs/heads/*"},
				unpushed: map[string]string{"--glob=refs/heads/*": "85cb511"},
			},
			command: "git fetch -p", cwd: "/repo", wantWarn: true,
		},
		{
			name: "allow_repos exempts a pruning fetch",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {AllowRepos: []string{"github.com/mad01/*"}},
			}},
			fixture: discardFixture{
				repo: repo, refspecs: []string{"refs/heads/*:refs/heads/*"},
				unpushed: map[string]string{"--glob=refs/heads/*": "85cb511"},
			},
			command: "git fetch -p", cwd: "/repo",
		},
		{
			name:    "a fetch --prune in a bare mirror with linked worktrees denies with nothing counted",
			fixture: discardFixture{repo: repo, mirrorWorktrees: true},
			command: "git fetch --prune", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a fetch --prune of a named remote in a bare mirror with linked worktrees denies",
			fixture: discardFixture{repo: repo, mirrorWorktrees: true},
			command: "git fetch --prune origin", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a remote update --prune in a bare mirror with linked worktrees denies",
			fixture: discardFixture{repo: repo, mirrorWorktrees: true},
			command: "git remote update --prune", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a remote prune in a bare mirror with linked worktrees denies",
			fixture: discardFixture{repo: repo, mirrorWorktrees: true},
			command: "git remote prune origin", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "fetch.prune in a bare mirror with linked worktrees denies a plain fetch",
			fixture: discardFixture{repo: repo, mirrorWorktrees: true, prunes: true},
			command: "git fetch", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a -c fetch.prune in a bare mirror with linked worktrees denies a plain fetch",
			fixture: discardFixture{repo: repo, mirrorWorktrees: true},
			command: "git -c fetch.prune=true fetch", cwd: "/repo", wantDeny: true,
		},
		{
			name:    "a fetch that does not prune passes in a bare mirror with linked worktrees",
			fixture: discardFixture{repo: repo, mirrorWorktrees: true},
			command: "git fetch", cwd: "/repo",
		},
		{
			name:    "--no-prune passes in a bare mirror with linked worktrees and fetch.prune",
			fixture: discardFixture{repo: repo, mirrorWorktrees: true, prunes: true},
			command: "git fetch --no-prune", cwd: "/repo",
		},
		{
			name:    "a dry-run prune passes in a bare mirror with linked worktrees",
			fixture: discardFixture{repo: repo, mirrorWorktrees: true},
			command: "git fetch --dry-run --prune", cwd: "/repo",
		},
		{
			name: "soft mode warns on a pruning fetch in a bare mirror with linked worktrees",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {Mode: config.ModeSoft},
			}},
			fixture: discardFixture{repo: repo, mirrorWorktrees: true},
			command: "git fetch --prune", cwd: "/repo", wantWarn: true,
		},
		{
			name: "allow_repos exempts a pruning fetch in a bare mirror with linked worktrees",
			cfg: config.Config{Guards: map[string]config.Toggle{
				GitDiscardID: {AllowRepos: []string{"github.com/mad01/*"}},
			}},
			fixture: discardFixture{repo: repo, mirrorWorktrees: true},
			command: "git fetch --prune", cwd: "/repo",
		},
		{
			name:    "a pruning fetch in an unresolvable directory denies when a setting maps into refs/heads",
			fixture: discardFixture{repo: repo},
			command: `cd "$(mktemp -d)" && git -c 'remote.origin.fetch=refs/heads/*:refs/heads/*' fetch -p`,
			cwd:     "/repo", wantDeny: true,
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
		{dir: "/repo", args: "$B", want: "$B"}, // a word readGitWords refuses stays as written
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
// reads as clean, an untracked file as dirty even when the repo's config
// hides untracked files, and a non-repo as a failure.
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
	if err := gitCmd(dir, "config", "status.showUntrackedFiles", "no").Run(); err != nil {
		t.Fatal(err)
	}
	if out, ok := gitStatusPorcelain(dir); !ok || out == "" {
		t.Errorf("status with untracked files hidden by config = %q, %v; want dirty", out, ok)
	}
	if _, ok := gitStatusPorcelain(""); ok {
		t.Error("empty dir must fail, never run git in belt's own cwd")
	}
}
