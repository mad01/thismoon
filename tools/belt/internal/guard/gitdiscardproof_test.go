package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// discardRepos is a clone with an upstream: a bare origin seeded from a
// second clone, so the clone has @{u} and origin/HEAD the way a real one
// does. The seed pushes the upstream changes a pull would bring.
type discardRepos struct {
	t     *testing.T
	seed  string
	clone string
}

// newDiscardRepos builds the repos with no system or global git config, so
// the user's hooks and settings never reach the test.
func newDiscardRepos(t *testing.T) discardRepos {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if gitEnvRedirected() {
		t.Skip("a GIT_DIR-style variable is set; the proof refuses there by design")
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "t")
		t.Setenv(k+"_EMAIL", "t@example.com")
	}
	root := t.TempDir()
	r := discardRepos{
		t:     t,
		seed:  filepath.Join(root, "seed"),
		clone: filepath.Join(root, "clone"),
	}
	up := filepath.Join(root, "up.git")
	r.git(root, "init", "-q", "--bare", "-b", "main", up)
	r.git(root, "init", "-q", "-b", "main", r.seed)
	r.write(r.seed, "a.txt", "a\n")
	r.write(r.seed, "b.txt", "b\n")
	r.write(r.seed, "d/f.txt", "f\n")
	r.git(r.seed, "add", "-A")
	r.git(r.seed, "commit", "-q", "-m", "init")
	r.git(r.seed, "remote", "add", "origin", up)
	r.git(r.seed, "push", "-q", "origin", "main")
	r.git(root, "clone", "-q", up, r.clone)
	return r
}

func (r discardRepos) git(dir string, args ...string) {
	r.t.Helper()
	if out, err := gitCmd(dir, args...).CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r discardRepos) write(dir, name, content string) {
	r.t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// upstream commits files in the seed, pushes them and fetches them into the
// clone: the state a pull starts from.
func (r discardRepos) upstream(files map[string]string) {
	r.t.Helper()
	for name, content := range files {
		r.write(r.seed, name, content)
	}
	r.git(r.seed, "add", "-A")
	r.git(r.seed, "commit", "-q", "-m", "upstream")
	r.git(r.seed, "push", "-q", "origin", "main")
	r.git(r.clone, "fetch", "-q")
}

// remove deletes a file or directory from the clone's worktree.
func (r discardRepos) remove(name string) {
	r.t.Helper()
	if err := os.RemoveAll(filepath.Join(r.clone, name)); err != nil {
		r.t.Fatal(err)
	}
}

// local writes files into the clone's worktree.
func (r discardRepos) local(files map[string]string) {
	r.t.Helper()
	for name, content := range files {
		r.write(r.clone, name, content)
	}
}

// submodule commits a submodule at sm, cloned from a bare upstream of its
// own and holding one file f, with `ignore = dirty` in .gitmodules so git
// status hides edits inside it unless told otherwise.
func (r discardRepos) submodule() {
	r.t.Helper()
	root := filepath.Dir(r.clone)
	seed, up := filepath.Join(root, "sm-seed"), filepath.Join(root, "sm.git")
	r.git(root, "init", "-q", "--bare", "-b", "main", up)
	r.git(root, "init", "-q", "-b", "main", seed)
	r.write(seed, "f", "f\n")
	r.git(seed, "add", "-A")
	r.git(seed, "commit", "-q", "-m", "init")
	r.git(seed, "push", "-q", up, "main")
	r.git(r.clone, "-c", "protocol.file.allow=always", "submodule", "add", "-q", up, "sm")
	r.git(r.clone, "config", "-f", ".gitmodules", "submodule.sm.ignore", "dirty")
	r.git(r.clone, "commit", "-q", "-am", "add sm")
}

// worktree adds a linked worktree of the clone at ../wt on a new branch and
// returns its path.
func (r discardRepos) worktree() string {
	r.t.Helper()
	wt := filepath.Join(filepath.Dir(r.clone), "wt")
	r.git(r.clone, "worktree", "add", "-q", "-b", "wt", wt)
	return wt
}

// detached adds a linked worktree of the clone at ../wd with a detached
// HEAD at the clone's HEAD and returns its path.
func (r discardRepos) detached() string {
	r.t.Helper()
	wd := filepath.Join(filepath.Dir(r.clone), "wd")
	r.git(r.clone, "worktree", "add", "-q", "--detach", wd)
	return wd
}

// orphan adds a linked worktree of the clone at ../wo on a new unborn
// branch and returns its path.
func (r discardRepos) orphan() string {
	r.t.Helper()
	wo := filepath.Join(filepath.Dir(r.clone), "wo")
	r.git(r.clone, "worktree", "add", "-q", "--orphan", "-b", "wo", wo)
	return wo
}

// commitIn commits a new file name in the worktree at dir.
func (r discardRepos) commitIn(dir, name string) {
	r.t.Helper()
	r.write(dir, name, name+"\n")
	r.git(dir, "add", name)
	r.git(dir, "commit", "-q", "-m", name)
}

// removeAll deletes a folder by hand, the way a worktree goes missing.
func (r discardRepos) removeAll(dir string) {
	r.t.Helper()
	if err := os.RemoveAll(dir); err != nil {
		r.t.Fatal(err)
	}
}

// link is the path next to the clone of a symlink a test may make.
func (r discardRepos) link() string {
	return filepath.Join(filepath.Dir(r.clone), "L")
}

// patches writes the test patches next to the clone: rev.diff turns a.txt
// from a into mine, fwd.diff turns mine into x, b.diff turns b.txt into b2
// and rename.diff moves a.txt to x.txt.
func (r discardRepos) patches() {
	r.t.Helper()
	root := filepath.Dir(r.clone)
	hunk := func(file, from, to string) string {
		return "diff --git a/" + file + " b/" + file + "\n--- a/" + file + "\n+++ b/" + file +
			"\n@@ -1 +1 @@\n-" + from + "\n+" + to + "\n"
	}
	r.write(root, "rev.diff", hunk("a.txt", "a", "mine"))
	r.write(root, "fwd.diff", hunk("a.txt", "mine", "x"))
	r.write(root, "b.diff", hunk("b.txt", "b", "b2"))
	r.write(root, "rename.diff", "diff --git a/a.txt b/x.txt\nsimilarity index 100%\n"+
		"rename from a.txt\nrename to x.txt\n")
}

// TestGitDiscardNothingLost runs the guard with its real git backends over a
// clone and its upstream. A discard passes when every file version it drops
// is committed in HEAD, the upstream or origin's default branch, and is
// refused with the first lossy path otherwise.
func TestGitDiscardNothingLost(t *testing.T) {
	pulled := map[string]string{"a.txt": "a2\n", "new.txt": "new\n", "d/f.txt": "f2\n"}
	tests := []struct {
		name  string
		setup func(r discardRepos)
		// cwd is where the commands run; nil means the clone.
		cwd      func(r discardRepos) string
		maxPaths int
		commands []string
		wantDeny string // a part of the reason; "" means the command passes
	}{
		{
			name: "untracked files equal to upstream that block a pull",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"new.txt": "new\n"})
			},
			commands: []string{
				"git clean -f",
				"git clean -f -- new.txt",
				"git clean -fd",
				"git clean --fo",
				"git clean -d --forc",
				"git reset --hard origin/main",
				"git reset --hard @{u}",
				"git reset --ha origin/main",
			},
		},
		{
			name: "a modified file equal to upstream",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n", "d/f.txt": "f2\n"})
			},
			commands: []string{
				"git checkout -- a.txt",
				"git checkout .",
				"git restore a.txt d",
				"git reset --hard",
				"git reset --hard origin/main",
				"cd d && git checkout -- f.txt",
				"git -C d restore .",
				"git read-tree -u --reset HEAD",
				"git read-tree -m -u HEAD",
				"git read-tree -u --reset origin/main",
				"git checkout-index -f -a",
				"git checkout-index -f -- a.txt",
				"cd d && git checkout-index -f -a",
				"git rm -f a.txt",
				"git rm -rf d",
				"git mv -f b.txt a.txt",
			},
		},
		{
			name:  "a modified file with unique content",
			setup: func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			commands: []string{
				"git checkout -- a.txt", "git restore .", "git reset --hard", "git reset --har",
				"git read-tree -u --reset HEAD", "git read-tree --res -u origin/main",
				"git checkout-index -f -a", "git checkout-index --fo -- a.txt",
				"git rm -f a.txt", "git rm -rf .", "git mv -f b.txt a.txt",
			},
			wantDeny: "`a.txt` holds content no commit has",
		},
		{
			name:     "a forced mv over an untracked file",
			setup:    func(r discardRepos) { r.local(map[string]string{"mine.txt": "mine\n"}) },
			commands: []string{"git mv -f b.txt mine.txt", "cd d && git mv -f f.txt ../mine.txt"},
			wantDeny: "`mine.txt` holds content no commit has",
		},
		{
			name:     "a forced mv into a directory over an untracked file",
			setup:    func(r discardRepos) { r.local(map[string]string{"d/a.txt": "mine\n"}) },
			commands: []string{"git mv -f a.txt d", "git mv -f a.txt b.txt d/"},
			wantDeny: "`d/a.txt` holds content no commit has",
		},
		{
			name:  "a forced mv whose destinations hold nothing to lose",
			setup: func(r discardRepos) { r.local(map[string]string{"u.txt": "u\n"}) },
			commands: []string{
				"git mv -f a.txt fresh.txt",
				"git mv -f a.txt b.txt d",
				"git mv -f d e",
				"git mv -f a.txt d/f.txt",
			},
		},
		{
			name: "an apply over a file with unique content",
			setup: func(r discardRepos) {
				r.patches()
				r.local(map[string]string{"a.txt": "mine\n"})
			},
			commands: []string{
				"git apply -R ../rev.diff",
				"git apply ../fwd.diff",
				"git apply --index --rev ../rev.diff",
				"git apply -R -- ../b.diff ../rev.diff",
			},
			wantDeny: "`a.txt` holds content no commit has",
		},
		{
			name: "an apply whose patch belt cannot list",
			setup: func(r discardRepos) {
				r.patches()
				r.local(map[string]string{"a.txt": "mine\n"})
			},
			commands: []string{"git diff | git apply -R", "git apply -R -", "git apply ../rename.diff"},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "an apply that touches only committed content",
			setup: func(r discardRepos) {
				r.patches()
				r.local(map[string]string{"a.txt": "mine\n"})
			},
			commands: []string{
				"git apply ../b.diff",
				"git apply --index ../b.diff",
				"git apply --exclude=a.txt -R ../rev.diff",
				"cd d && git apply -R ../../rev.diff",
			},
		},
		{
			name: "a forced worktree remove over unique work in that worktree",
			setup: func(r discardRepos) {
				r.write(r.worktree(), "a.txt", "mine\n")
			},
			commands: []string{"git worktree remove -f ../wt", "git worktree remove --force wt"},
			wantDeny: "`a.txt` holds content no commit has",
		},
		{
			name: "a forced worktree remove over an untracked file in that worktree",
			setup: func(r discardRepos) {
				r.write(r.worktree(), "mine.txt", "mine\n")
			},
			commands: []string{"git worktree remove -f -f ../wt"},
			wantDeny: "`mine.txt` holds content no commit has",
		},
		{
			name: "a forced worktree remove of files equal to upstream, from a dirty cwd",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				wt := r.worktree()
				r.write(wt, "a.txt", "a2\n")
				r.write(wt, "new.txt", "new\n")
				r.local(map[string]string{"a.txt": "mine\n"})
			},
			commands: []string{"git worktree remove -f ../wt", "git worktree remove --force wt"},
		},
		{
			name: "a forced worktree remove that git places from the physical cwd",
			setup: func(r discardRepos) {
				r.worktree()
				inner := filepath.Join(r.clone, "wt")
				r.git(r.clone, "worktree", "add", "-q", "-b", "inner", inner)
				r.write(inner, "a.txt", "mine\n")
				if err := os.Symlink(filepath.Join(r.clone, "d"), r.link()); err != nil {
					r.t.Fatal(err)
				}
			},
			cwd:      func(r discardRepos) string { return r.link() },
			commands: []string{"git worktree remove --force ../wt"},
			wantDeny: "`a.txt` holds content no commit has",
		},
		{
			name: "a worktree remove of a detached HEAD with a commit no ref holds",
			setup: func(r discardRepos) {
				wd := r.detached()
				r.write(wd, "x.txt", "x\n")
				r.git(wd, "add", "x.txt")
				r.git(wd, "commit", "-q", "-m", "x")
			},
			commands: []string{"git worktree remove ../wd", "git worktree remove -f ../wd"},
			wantDeny: "it deletes that worktree's HEAD and HEAD reflog, the last refs that hold 1 commit(s)",
		},
		{
			name:     "a worktree remove of a detached HEAD a branch holds",
			setup:    func(r discardRepos) { r.detached() },
			commands: []string{"git worktree remove ../wd", "git worktree remove -f ../wd"},
		},
		{
			name: "a worktree remove of a detached HEAD whose reflog alone holds a commit",
			setup: func(r discardRepos) {
				wd := r.detached()
				r.commitIn(wd, "x.txt")
				r.git(wd, "checkout", "-q", "--detach", "origin/main")
			},
			commands: []string{"git worktree remove ../wd", "git worktree remove -f ../wd"},
			wantDeny: "the last refs that hold 1 commit(s)",
		},
		{
			name: "a worktree remove of a branch whose HEAD reflog alone holds a commit",
			setup: func(r discardRepos) {
				wt := r.worktree()
				r.git(wt, "checkout", "-q", "--detach")
				r.commitIn(wt, "x.txt")
				r.git(wt, "checkout", "-q", "wt")
			},
			commands: []string{"git worktree remove ../wt", "git worktree remove -f wt"},
			wantDeny: "the last refs that hold 1 commit(s)",
		},
		{
			name: "a worktree remove whose reflog commits a branch holds, after a rebase",
			setup: func(r discardRepos) {
				wt := r.worktree()
				r.commitIn(wt, "w1.txt")
				r.commitIn(wt, "w2.txt")
				r.git(wt, "checkout", "-q", "--detach")
				r.commitIn(wt, "x.txt")
				r.git(wt, "branch", "saved")
				r.git(wt, "checkout", "-q", "wt")
				r.upstream(pulled)
				r.git(wt, "rebase", "-q", "origin/main")
			},
			commands: []string{"git worktree remove ../wt", "git worktree remove -f wt"},
		},
		{
			name: "a worktree remove whose reflog commits are pushed",
			setup: func(r discardRepos) {
				wd := r.detached()
				r.commitIn(wd, "x.txt")
				r.git(wd, "push", "-q", "origin", "HEAD:refs/heads/pushed")
				r.git(wd, "checkout", "-q", "--detach", "origin/main")
			},
			commands: []string{"git worktree remove ../wd", "git worktree remove -f ../wd"},
		},
		{
			name: "a worktree remove of a reflog commit another worktree's HEAD holds",
			setup: func(r discardRepos) {
				wd := r.detached()
				r.commitIn(wd, "x.txt")
				r.git(r.worktree(), "checkout", "-q", "--detach", "worktrees/wd/HEAD")
				r.git(wd, "checkout", "-q", "--detach", "origin/main")
			},
			commands: []string{"git worktree remove ../wd"},
		},
		{
			name: "a worktree remove of a reflog commit another worktree's HEAD reflog holds",
			setup: func(r discardRepos) {
				wd := r.detached()
				r.commitIn(wd, "x.txt")
				wt := r.worktree()
				r.git(wt, "checkout", "-q", "--detach", "worktrees/wd/HEAD")
				r.git(wt, "checkout", "-q", "wt")
				r.git(wd, "checkout", "-q", "--detach", "origin/main")
			},
			commands: []string{"git worktree remove ../wd", "git worktree remove ../wt"},
		},
		{
			name:     "a worktree remove of an unborn branch",
			setup:    func(r discardRepos) { r.orphan() },
			commands: []string{"git worktree remove ../wo", "git worktree remove -f ../wo"},
		},
		{
			name:     "a worktree prune of a deleted folder on an unborn branch",
			setup:    func(r discardRepos) { r.removeAll(r.orphan()) },
			commands: []string{"git worktree prune", "git worktree prune --expire now"},
		},
		{
			name: "a worktree remove of an unborn branch whose HEAD reflog holds a commit",
			setup: func(r discardRepos) {
				wd := r.detached()
				r.commitIn(wd, "x.txt")
				r.git(wd, "checkout", "-q", "--orphan", "fresh")
				r.git(wd, "rm", "-rfq", ".")
			},
			commands: []string{"git worktree remove ../wd"},
			wantDeny: "worktrees/wd/HEAD does not resolve and keeps a reflog",
		},
		{
			name: "a worktree prune of a deleted folder whose HEAD alone holds a commit",
			setup: func(r discardRepos) {
				wd := r.detached()
				r.commitIn(wd, "x.txt")
				r.removeAll(wd)
			},
			commands: []string{
				"git worktree prune",
				"git worktree prune -v",
				"git worktree prune --expire now",
				"git worktree prune -n --no-dry-run",
			},
			wantDeny: "it prunes each worktree whose folder is gone, with its HEAD and HEAD reflog, " +
				"the last refs that hold 1 commit(s)",
		},
		{
			name: "a worktree prune that leaves a stranded commit in a folder still there",
			setup: func(r discardRepos) {
				r.commitIn(r.detached(), "x.txt")
				r.removeAll(r.worktree())
			},
			commands: []string{"git worktree prune", "git worktree prune -v --expire=now"},
		},
		{
			name: "a dry-run prune, and a prune whose expiry keeps the worktree",
			setup: func(r discardRepos) {
				wd := r.detached()
				r.commitIn(wd, "x.txt")
				r.removeAll(wd)
			},
			commands: []string{
				"git worktree prune -n",
				"git worktree prune --dry-run -v",
				"git worktree prune --expire 1.day.ago",
			},
		},
		{
			name: "untracked files the repo's config hides from git status",
			setup: func(r discardRepos) {
				r.git(r.clone, "config", "status.showUntrackedFiles", "no")
				r.write(r.worktree(), "mine.txt", "mine\n")
				r.local(map[string]string{"u.txt": "u\n"})
			},
			commands: []string{"git worktree remove -f ../wt", "git clean -fd"},
			wantDeny: "holds content no commit has",
		},
		{
			name: "a worktree remove by a path under home, of a clean worktree",
			setup: func(r discardRepos) {
				r.t.Setenv("HOME", filepath.Dir(r.clone))
				r.worktree()
			},
			commands: []string{"git worktree remove ~/wt", "git worktree remove -f ~/wt/"},
		},
		{
			name: "a forced worktree remove by a path under home, over unique work",
			setup: func(r discardRepos) {
				r.t.Setenv("HOME", filepath.Dir(r.clone))
				r.write(r.worktree(), "a.txt", "mine\n")
			},
			commands: []string{"git worktree remove -f ~/wt"},
			wantDeny: "`a.txt` holds content no commit has",
		},
		{
			name:     "a read-tree that writes its index over a file with unique content",
			setup:    func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			commands: []string{"git read-tree --index-output=a.txt HEAD"},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "a checkout of paths without --, equal to upstream",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n", "d/f.txt": "f2\n"})
			},
			commands: []string{
				"git checkout a.txt",
				"git checkout a.txt d/f.txt",
				"git checkout a.txt -q",
				"git checkout HEAD a.txt",
				"git checkout origin/main a.txt",
				"cd d && git checkout f.txt",
			},
		},
		{
			name:  "a checkout of paths without -- over unique content",
			setup: func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			commands: []string{
				"git checkout a.txt",
				"git checkout a.txt -q",
				"git checkout HEAD a.txt",
				"git checkout main b.txt a.txt",
				"cd d && git checkout ../a.txt",
			},
			wantDeny: "`a.txt` holds content no commit has",
		},
		{
			name: "a branch switch over a dirty tree",
			setup: func(r discardRepos) {
				r.git(r.seed, "push", "-q", "origin", "main:feature")
				r.git(r.clone, "fetch", "-q")
				r.git(r.clone, "branch", "other")
				r.git(r.clone, "branch", "b.txt")
				r.local(map[string]string{"a.txt": "mine\n"})
			},
			commands: []string{
				"git checkout other",
				"git checkout -q other",
				"git checkout -b fresh",
				"git checkout -b fresh origin/main",
				"git checkout origin/main",
				"git checkout feature", // only origin/feature: git creates the branch
				"git checkout b.txt",   // a branch and a file: git switches
				"git checkout -bf",     // a branch named f
				`cd "$(git rev-parse --show-toplevel)" && git checkout other`,
				`git -C "$PWD" checkout other`,
			},
		},
		{
			name:  "a forced switch over a dirty tree",
			setup: func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			commands: []string{
				"git checkout -qf main",
				"git checkout -fb fresh",
				"git checkout --forc main",
				"git switch -qf main",
				"git switch --disc main",
			},
			wantDeny: "this discards uncommitted work",
		},
		{
			name: "a patch checkout of the whole tree, equal to upstream",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n", "d/f.txt": "f2\n"})
			},
			commands: []string{
				"git checkout -p",
				"git checkout -qp --",
				"git checkout -p HEAD",
				"git checkout --patch origin/main",
				"git checkout -p -- a.txt",
				"cd d && git checkout -p",
			},
		},
		{
			name:  "a patch checkout of the whole tree over unique content",
			setup: func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			commands: []string{
				"git checkout -p",
				"git checkout -qp",
				"git checkout -p HEAD",
				"git checkout -p HEAD --",
			},
			wantDeny: "`d/f.txt` holds content no commit has",
		},
		{
			name: "a patch checkout from a tree drops a file only the index has",
			setup: func(r discardRepos) {
				r.local(map[string]string{"n.txt": "mine\n"})
				r.git(r.clone, "add", "n.txt")
			},
			commands: []string{"git checkout -p HEAD"},
			wantDeny: "`n.txt`",
		},
		{
			name: "a discard in a directory only the shell names, equal to upstream",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n", "d/f.txt": "f2\n"})
			},
			commands: []string{
				`cd "$(git rev-parse --show-toplevel)" && git checkout -- a.txt`,
				`cd d && cd "$(git rev-parse --show-toplevel)" && git checkout -- a.txt`,
				`git -C "$(git -C d rev-parse --show-toplevel)" checkout -- a.txt`,
				`git -C "$PWD" checkout -- a.txt`,
				`cd d && git -C "${PWD}" checkout -- f.txt`,
			},
		},
		{
			name:  "a discard in a directory only the shell names, over unique content",
			setup: func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			commands: []string{
				`cd d && cd "$(git rev-parse --show-toplevel)" && git checkout -- a.txt`,
				`git -C "$PWD" checkout -- a.txt`,
			},
			wantDeny: "`a.txt` holds content no commit has",
		},
		{
			name: "a checkout fed paths belt cannot list",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n"})
			},
			commands: []string{
				"echo a.txt | xargs git checkout --",
				"echo a.txt | xargs git checkout",
				"git checkout --pathspec-from-file=list.txt",
				"git checkout --pathspec-from-file list.txt",
			},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "a mix names the unique file",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n", "b.txt": "mine\n", "new.txt": "new\n"})
			},
			commands: []string{"git checkout -- .", "git reset --hard origin/main"},
			wantDeny: "`b.txt` holds content no commit has",
		},
		{
			name: "an untracked file with unique content",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"new.txt": "new\n", "notes.txt": "mine\n"})
			},
			commands: []string{"git clean -f"},
			wantDeny: "`notes.txt` holds content no commit has",
		},
		{
			name: "an untracked file a reset writes over",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"new.txt": "mine\n", "notes.txt": "kept\n"})
			},
			commands: []string{"git reset --hard origin/main"},
			wantDeny: "`new.txt` holds content no commit has",
		},
		{
			name: "untracked files a reset leaves alone",
			setup: func(r discardRepos) {
				r.local(map[string]string{"notes.txt": "kept\n"})
			},
			commands: []string{"git reset --hard"},
		},
		{
			name: "a staged copy with unique content",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "mine\n"})
				r.git(r.clone, "add", "a.txt")
				r.local(map[string]string{"a.txt": "a2\n"})
			},
			commands: []string{"git reset --hard origin/main", "git checkout origin/main -- a.txt"},
			wantDeny: "the staged copy of `a.txt` is in no commit",
		},
		{
			name: "a hard reset that drops a local commit",
			setup: func(r discardRepos) {
				r.local(map[string]string{"c.txt": "c\n"})
				r.git(r.clone, "add", "c.txt")
				r.git(r.clone, "commit", "-q", "-m", "local")
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n"})
			},
			commands: []string{"git reset --hard origin/main"},
			wantDeny: "moves the branch off 1 commit(s)",
		},
		{
			name: "a path set belt cannot resolve",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n"})
			},
			commands: []string{
				"git checkout -- '*.txt'",
				"git checkout -- ':(glob)a.txt'",
				`git restore "$F"`,
				"git checkout -m -- a.txt",
				"git restore --ours a.txt",
				"git clean -fi",
			},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "more paths than the cap",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n", "d/f.txt": "f2\n"})
			},
			maxPaths: 1,
			commands: []string{"git checkout -- ."},
			wantDeny: "more than 1 paths",
		},
		{
			name:     "a stash, which the proof does not read",
			setup:    func(r discardRepos) { r.local(map[string]string{"a.txt": "a\n", "x": "x\n"}) },
			commands: []string{"git stash -u"},
			wantDeny: "passes only on a clean tree",
		},
		{
			name: "a file where a restored path needs a directory",
			setup: func(r discardRepos) {
				r.remove("d")
				r.local(map[string]string{"d": "WORK\n"})
			},
			commands: []string{
				"git checkout -- d/f.txt",
				"git restore d/f.txt",
				"git reset --hard",
			},
			wantDeny: "`d` holds content no commit has",
		},
		{
			name: "a clean whose -e brings an ignored file in",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{".git/info/exclude": "secret.env\n"})
				r.local(map[string]string{"new.txt": "new\n", "secret.env": "SECRET\n"})
			},
			commands: []string{
				"git clean -f -e '!secret.env'",
				"git clean -f --exclude='!secret.env'",
			},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "a clean whose -e brings in the only ignored file",
			setup: func(r discardRepos) {
				r.local(map[string]string{".git/info/exclude": "secret.env\n"})
				r.local(map[string]string{"secret.env": "SECRET\n"})
			},
			commands: []string{"git clean -f -e '!secret.env'"},
			wantDeny: "belt could not list what it drops",
		},
		{
			name:     "a clean -X whose -e brings an untracked file in",
			setup:    func(r discardRepos) { r.local(map[string]string{"keepme.txt": "mine\n"}) },
			commands: []string{"git clean -fX -e keepme.txt"},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "an earlier step that changes what the discard drops",
			setup: func(r discardRepos) {
				r.local(map[string]string{"untracked": "u\n"})
			},
			commands: []string{
				"mv untracked a.txt && git checkout -- a.txt",
				"cp untracked a.txt; git restore a.txt",
				"echo x >a.txt && git reset --hard",
				"git checkout -- a.txt & mv untracked a.txt",
				"git checkout -- a.txt | mv untracked a.txt",
				"F=x; git checkout -- a.txt",
			},
			wantDeny: "may change what it drops",
		},
		{
			name: "an earlier git write that strands a commit",
			setup: func(r discardRepos) {
				r.local(map[string]string{"c.txt": "c\n"})
				r.git(r.clone, "add", "c.txt")
				r.git(r.clone, "commit", "-q", "-m", "local")
				r.local(map[string]string{"notes.txt": "kept\n"})
			},
			commands: []string{
				"git reset --soft HEAD~1 && git reset --hard",
				"git checkout -- . && git reset --hard",
			},
			wantDeny: "may change what it drops",
		},
		{
			name: "read-only steps around a discard",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n", "notes.txt": "kept\n"})
			},
			commands: []string{
				"git status --short && git checkout -- a.txt",
				"cat a.txt; git diff --stat && git checkout -- a.txt && git pull",
				"git reset --hard origin/main 2>&1 | tail -1",
				"git checkout -- a.txt >/dev/null",
			},
		},
		{
			name:  "a global pathspec option or environment",
			setup: func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			commands: []string{
				"git --icase-pathspecs checkout -- A.TXT",
				"GIT_ICASE_PATHSPECS=1 git checkout -- A.TXT",
				"git -C . --icase-pathspecs restore A.TXT",
				"env GIT_ICASE_PATHSPECS=1 git restore A.TXT",
			},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "an assume-unchanged edit a reset drops",
			setup: func(r discardRepos) {
				r.git(r.clone, "update-index", "--assume-unchanged", "a.txt")
				r.local(map[string]string{"a.txt": "a\nWORK\n", "untracked": "u\n"})
			},
			commands: []string{"git reset --hard", "git checkout ."},
			wantDeny: "assume-unchanged or skip-worktree",
		},
		{
			name: "a skip-worktree edit a reset drops",
			setup: func(r discardRepos) {
				r.git(r.clone, "update-index", "--skip-worktree", "b.txt")
				r.local(map[string]string{"b.txt": "b\nWORK\n", "untracked": "u\n"})
			},
			commands: []string{"git reset --hard", "git restore b.txt"},
			wantDeny: "assume-unchanged or skip-worktree",
		},
		{
			name: "an origin/HEAD that points at a local branch",
			setup: func(r discardRepos) {
				r.git(r.clone, "checkout", "-q", "-b", "scratch")
				r.local(map[string]string{"a.txt": "mine\n"})
				r.git(r.clone, "commit", "-q", "-am", "wip")
				r.git(r.clone, "checkout", "-q", "main")
				r.git(r.clone, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/heads/scratch")
				r.local(map[string]string{"a.txt": "mine\n"})
			},
			commands: []string{"git checkout -- a.txt"},
			wantDeny: "`a.txt` holds content no commit has",
		},
		{
			name: "a submodule edit status hides that a recursing reset drops",
			setup: func(r discardRepos) {
				r.submodule()
				r.git(r.clone, "config", "submodule.recurse", "true")
				r.local(map[string]string{"sm/f": "f\nSUBWORK\n", "untracked": "u\n"})
			},
			commands: []string{"git reset --hard"},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "a submodule edit status hides, in an otherwise clean tree",
			setup: func(r discardRepos) {
				r.submodule()
				r.git(r.clone, "config", "submodule.recurse", "true")
				r.local(map[string]string{"sm/f": "f\nSUBWORK\n"})
			},
			commands: []string{"git reset --hard"},
			wantDeny: "belt could not list what it drops",
		},
		{
			name: "a submodule edit status hides, without submodule.recurse",
			setup: func(r discardRepos) {
				r.submodule()
				r.local(map[string]string{"sm/f": "f\nSUBWORK\n", "untracked": "u\n"})
			},
			commands: []string{"git reset --hard"},
			wantDeny: "a submodule at sm",
		},
		{
			name: "a clean submodule a recursing reset reaches",
			setup: func(r discardRepos) {
				r.submodule()
				r.git(r.clone, "config", "submodule.recurse", "true")
				r.local(map[string]string{"untracked": "u\n"})
			},
			commands: []string{"git reset --hard"},
			wantDeny: "submodule.recurse",
		},
		{
			name: "a clean submodule a plain reset leaves alone",
			setup: func(r discardRepos) {
				r.submodule()
				r.local(map[string]string{"untracked": "u\n"})
			},
			commands: []string{"git reset --hard"},
		},
		{
			name: "an unforced clean under a repo config that turns clean.requireForce off",
			setup: func(r discardRepos) {
				r.git(r.clone, "config", "clean.requireForce", "false")
				r.local(map[string]string{"mine.txt": "mine\n"})
			},
			commands: []string{"git clean", "git clean -d", "git clean -q -- mine.txt"},
			wantDeny: "`mine.txt` holds content no commit has",
		},
		{
			name: "an unforced clean under a clean.requireForce git cannot read",
			setup: func(r discardRepos) {
				r.git(r.clone, "config", "clean.requireForce", "maybe")
				r.local(map[string]string{"mine.txt": "mine\n"})
			},
			commands: []string{"git clean -d"},
			wantDeny: "`mine.txt` holds content no commit has",
		},
		{
			name: "an unforced clean of files equal to upstream, clean.requireForce off",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.git(r.clone, "config", "clean.requireForce", "false")
				r.local(map[string]string{"new.txt": "new\n"})
			},
			commands: []string{"git clean", "git clean -d"},
		},
		{
			name:     "an unforced clean git refuses under the default clean.requireForce",
			setup:    func(r discardRepos) { r.local(map[string]string{"mine.txt": "mine\n"}) },
			commands: []string{"git clean", "git clean -d", "git clean -x", "git -C d clean -d"},
		},
		{
			name:  "an unforced clean whose own config may turn clean.requireForce off",
			setup: func(r discardRepos) { r.local(map[string]string{"mine.txt": "mine\n"}) },
			commands: []string{
				"git -c clean.requireForce=false clean -d",
				"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=clean.requireForce " +
					"GIT_CONFIG_VALUE_0=false git clean -d",
			},
			wantDeny: "belt could not list what it drops",
		},
		{
			name:     "an unforced clean after a config write in the same command",
			setup:    func(r discardRepos) { r.local(map[string]string{"mine.txt": "mine\n"}) },
			commands: []string{"git config clean.requireForce false && git clean -d"},
			wantDeny: "runs in the same command",
		},
		{
			name: "a staged edit with unique content a merge reset drops",
			setup: func(r discardRepos) {
				r.local(map[string]string{"a.txt": "mine\n"})
				r.git(r.clone, "add", "a.txt")
			},
			commands: []string{"git reset --merge", "git reset --mer HEAD"},
			wantDeny: "the staged copy of `a.txt` is in no commit",
		},
		{
			name: "a staged new file a merge reset deletes",
			setup: func(r discardRepos) {
				r.local(map[string]string{"n.txt": "mine\n"})
				r.git(r.clone, "add", "n.txt")
			},
			commands: []string{"git reset --merge"},
			wantDeny: "the staged copy of `n.txt` is in no commit",
		},
		{
			name: "a staged edit a merge reset drops beside an unstaged one it keeps",
			setup: func(r discardRepos) {
				r.local(map[string]string{"a.txt": "mine\n", "b.txt": "mine\n"})
				r.git(r.clone, "add", "a.txt")
			},
			commands: []string{"git reset --merge"},
			wantDeny: "the staged copy of `a.txt` is in no commit",
		},
		{
			name: "unstaged edits and untracked files a merge reset keeps",
			setup: func(r discardRepos) {
				r.local(map[string]string{"a.txt": "mine\n", "d/f.txt": "mine\n", "u.txt": "u\n"})
			},
			commands: []string{"git reset --merge", "git reset --merge HEAD"},
		},
		{
			name: "a staged edit git refuses to merge-reset over an unstaged one",
			setup: func(r discardRepos) {
				r.local(map[string]string{"a.txt": "staged\n", "b.txt": "staged\n"})
				r.git(r.clone, "add", "a.txt", "b.txt")
				r.local(map[string]string{"b.txt": "unstaged\n"})
			},
			commands: []string{"git reset --merge"},
		},
		{
			name: "an unstaged edit git refuses to merge-reset to another commit",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "mine\n"})
			},
			commands: []string{"git reset --merge origin/main"},
		},
		{
			name: "a staged edit equal to upstream a merge reset drops",
			setup: func(r discardRepos) {
				r.upstream(pulled)
				r.local(map[string]string{"a.txt": "a2\n"})
				r.git(r.clone, "add", "a.txt")
			},
			commands: []string{"git reset --merge"},
		},
		{
			name: "a conflicted merge a merge reset rewrites",
			setup: func(r discardRepos) {
				r.git(r.clone, "checkout", "-q", "-b", "other")
				r.local(map[string]string{"a.txt": "other\n"})
				r.git(r.clone, "commit", "-q", "-am", "other")
				r.git(r.clone, "checkout", "-q", "main")
				r.local(map[string]string{"a.txt": "main\n"})
				r.git(r.clone, "commit", "-q", "-am", "main")
				if err := gitCmd(r.clone, "merge", "-q", "other").Run(); err == nil {
					r.t.Fatal("the merge did not conflict")
				}
			},
			commands: []string{"git reset --merge"},
			wantDeny: "a.txt is conflicted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newDiscardRepos(t)
			tt.setup(r)
			g := NewGitDiscard(config.Config{})
			g.emit = func(_, _, _, _ string, _ map[string]string) {}
			if tt.maxPaths > 0 {
				g.prove = prover{maxPaths: tt.maxPaths, budget: proofBudget}.prove
			}
			cwd := r.clone
			if tt.cwd != nil {
				cwd = tt.cwd(r)
			}
			for _, command := range tt.commands {
				d := g.Check(Input{Event: EventBash, Command: command, Cwd: cwd})
				switch {
				case tt.wantDeny == "" && d != nil:
					t.Errorf("Check(%q) = %q, want it to pass", command, d.Reason)
				case tt.wantDeny != "" && d == nil:
					t.Errorf("Check(%q) passed, want a deny naming %q", command, tt.wantDeny)
				case d != nil && !strings.Contains(d.Reason, tt.wantDeny):
					t.Errorf("Check(%q) reason = %q, want it to contain %q",
						command, d.Reason, tt.wantDeny)
				}
			}
		})
	}
}

// TestReadDiscardSpec pins how the proof reads each command's arguments.
func TestReadDiscardSpec(t *testing.T) {
	tests := []specCase{
		{sub: "checkout", args: "-- a b", want: discardSpec{
			kind: pathRestore, overlay: true, pathspecs: []string{"a", "b"},
		}},
		{sub: "checkout", args: "-q main -- a", want: discardSpec{
			kind: pathRestore, overlay: true, source: "main", staged: true,
			pathspecs: []string{"a"},
		}},
		{sub: "checkout", args: ".", want: discardSpec{
			kind: pathRestore, overlay: true, pathspecs: []string{"."},
		}},
		{sub: "checkout", args: "-f main", wantErr: true},
		{sub: "checkout", args: "-p -- .", want: discardSpec{
			kind: pathRestore, overlay: true, pathspecs: []string{"."},
		}},
		{sub: "checkout", args: "-qp HEAD -- .", want: discardSpec{
			kind: pathRestore, source: "HEAD", staged: true, pathspecs: []string{"."},
		}},
		{sub: "checkout", args: "--patch main -- a", want: discardSpec{
			kind: pathRestore, source: "main", staged: true, pathspecs: []string{"a"},
		}},
		{sub: "checkout", args: "-pf main -- a", wantErr: true},
		{sub: "checkout", args: "main .", wantErr: true},
		{sub: "restore", args: "--source=HEAD~1 a", want: discardSpec{
			kind: pathRestore, source: "HEAD~1", pathspecs: []string{"a"},
		}},
		{sub: "restore", args: "-S -W a", want: discardSpec{
			kind: pathRestore, source: "HEAD", staged: true, pathspecs: []string{"a"},
		}},
		{sub: "restore", args: "-W", wantErr: true},
		{sub: "clean", args: "-fdx -- out", want: discardSpec{
			kind: untrackedClean, scope: cleanWithIgnored, pathspecs: []string{"out"},
		}},
		{sub: "clean", args: "-fdx -e keep -- out", wantErr: true},
		{sub: "clean", args: "-fekeep", wantErr: true},
		{sub: "clean", args: "-f --exclude=keep", wantErr: true},
		{sub: "clean", args: "-f --exclude keep", wantErr: true},
		{
			sub:  "clean",
			args: "-fX",
			want: discardSpec{kind: untrackedClean, scope: cleanOnlyIgnored},
		},
		{sub: "reset", args: "--hard", want: discardSpec{kind: hardReset, source: "HEAD"}},
		{sub: "reset", args: "--hard @{u}", want: discardSpec{kind: hardReset, source: "@{u}"}},
		{sub: "reset", args: "--hard -- a", wantErr: true},
		{sub: "reset", args: "--hard --recurse-submodules", wantErr: true},
		{sub: "checkout", args: "-- d/*.go", wantErr: true},
		{sub: "checkout", args: `-- a\b`, wantErr: true},
		{sub: "reset", args: "--hard -x", wantErr: true},
		{sub: "reset", args: "--ha", want: discardSpec{kind: hardReset, source: "HEAD"}},
		{
			sub:  "reset",
			args: "-q --har origin/main",
			want: discardSpec{kind: hardReset, source: "origin/main"},
		},
		{sub: "reset", args: "origin/main --h", want: discardSpec{
			kind: hardReset, source: "origin/main",
		}},
		{sub: "reset", args: "--hard --soft", wantErr: true},
		{sub: "reset", args: "--hard=x", wantErr: true},
		{sub: "reset", args: "--hard HEAD --", wantErr: true},
		{sub: "reset", args: "--hard $REF", wantErr: true},
		{sub: "reset", args: "--soft", wantErr: true},
		{sub: "reset", args: "--merge", want: discardSpec{kind: mergeReset, source: "HEAD"}},
		{sub: "reset", args: "-q --mer @{u}", want: discardSpec{kind: mergeReset, source: "@{u}"}},
		{sub: "reset", args: "--hard --merge", want: discardSpec{kind: mergeReset, source: "HEAD"}},
		{sub: "reset", args: "--merge --hard", want: discardSpec{kind: hardReset, source: "HEAD"}},
		{sub: "reset", args: "--merge -- a", wantErr: true},
		{sub: "reset", args: "--merge --keep", wantErr: true},
		{sub: "checkout", args: "--pa main -- a", wantErr: true}, // ambiguous
		{sub: "checkout", args: "--patc main -- a", want: discardSpec{
			kind: pathRestore, source: "main", staged: true, pathspecs: []string{"a"},
		}},
		{sub: "checkout", args: "-p --no-patch main -- a", want: discardSpec{
			kind: pathRestore, overlay: true, source: "main", staged: true,
			pathspecs: []string{"a"},
		}},
		{sub: "checkout", args: "--qui -- a", want: discardSpec{
			kind: pathRestore, overlay: true, pathspecs: []string{"a"},
		}},
		{sub: "checkout", args: "-b x -- a", wantErr: true},
		{sub: "checkout", args: "--fo -- a", wantErr: true},
		{sub: "checkout", args: "-- $F", wantErr: true},
		{sub: "clean", args: "--fo -d", want: discardSpec{kind: untrackedClean}},
		{sub: "clean", args: "--forc -x -- out", want: discardSpec{
			kind: untrackedClean, scope: cleanWithIgnored, pathspecs: []string{"out"},
		}},
		{sub: "clean", args: "-f out", want: discardSpec{
			kind: untrackedClean, pathspecs: []string{"out"},
		}},
		{sub: "clean", args: "-fen", wantErr: true},
		{sub: "clean", args: "-f --ex keep", wantErr: true},
		{sub: "clean", args: "-f --no-force", wantErr: true},
		{sub: "clean", args: "-fi", wantErr: true},
	}
	tests = append(tests, treeWriterSpecs...)
	for _, tt := range tests {
		t.Run(tt.sub+" "+tt.args, func(t *testing.T) {
			got, err := readDiscardSpec(gitInvocation{sub: tt.sub, args: strings.Fields(tt.args)})
			if (err != nil) != tt.wantErr {
				t.Fatalf("readDiscardSpec = %+v, %v; wantErr %v", got, err, tt.wantErr)
			}
			if !tt.wantErr && !specEqual(got, tt.want) {
				t.Errorf("readDiscardSpec = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// specCase is one TestReadDiscardSpec case.
type specCase struct {
	sub, args string
	want      discardSpec
	wantErr   bool
}

// treeWriterSpecs pins how the proof reads read-tree, checkout-index, rm,
// mv, apply and worktree remove.
var treeWriterSpecs = []specCase{
	{sub: "read-tree", args: "-u --reset HEAD", want: discardSpec{kind: treeRead, source: "HEAD"}},
	{sub: "read-tree", args: "-m -u -q origin/main", want: discardSpec{
		kind: treeRead, source: "origin/main",
	}},
	{sub: "read-tree", args: "-mu HEAD", want: discardSpec{kind: treeRead, source: "HEAD"}},
	{sub: "read-tree", args: "-u HEAD", wantErr: true},
	{sub: "read-tree", args: "-m -u HEAD main", wantErr: true},
	{sub: "read-tree", args: "-u --reset --prefix=x/ HEAD", wantErr: true},
	{sub: "read-tree", args: "-m -u --aggressive HEAD", wantErr: true},
	{sub: "read-tree", args: "-m -u $REF", wantErr: true},
	{sub: "checkout-index", args: "-f -a", want: discardSpec{
		kind: pathRestore, overlay: true, pathspecs: []string{"."},
	}},
	{sub: "checkout-index", args: "-fqu -- a b", want: discardSpec{
		kind: pathRestore, overlay: true, pathspecs: []string{"a", "b"},
	}},
	{sub: "checkout-index", args: "-f", wantErr: true},
	{sub: "checkout-index", args: "-f --stdin", wantErr: true},
	{sub: "checkout-index", args: "-f -a --prefix=out/", wantErr: true},
	{sub: "checkout-index", args: "-f --stage=2 a", wantErr: true},
	{sub: "rm", args: "-rf .", want: discardSpec{kind: trackedRemoval, pathspecs: []string{"."}}},
	{sub: "rm", args: "-f -q -- a", want: discardSpec{kind: trackedRemoval, pathspecs: []string{"a"}}},
	{sub: "rm", args: "-f", wantErr: true},
	{sub: "rm", args: "-f d/*.go", wantErr: true},
	{sub: "rm", args: "-f --pathspec-from-file=list", wantErr: true},
	{sub: "rm", args: "-f --sparse a", wantErr: true},
	{sub: "mv", args: "-f a b", want: discardSpec{
		kind: forcedMove, pathspecs: []string{"a"}, dest: "b",
	}},
	{sub: "mv", args: "-fk a b dir", want: discardSpec{
		kind: forcedMove, pathspecs: []string{"a", "b"}, dest: "dir",
	}},
	{sub: "mv", args: "-f a", wantErr: true},
	{sub: "mv", args: "-f a $DEST", wantErr: true},
	{sub: "mv", args: "-f --sparse a b", wantErr: true},
	{sub: "apply", args: "-R p.diff", want: discardSpec{
		kind: patchApply, patches: []string{"p.diff"}, patchOpts: []string{"--reverse"},
	}},
	{sub: "apply", args: "-Rp0 --directory sub --index -- x.diff y.diff", want: discardSpec{
		kind: patchApply, staged: true, patches: []string{"x.diff", "y.diff"},
		patchOpts: []string{"--reverse", "-p0", "--directory=sub"},
	}},
	{sub: "apply", args: "-3 --exclude=d/f -v p.diff", want: discardSpec{
		kind: patchApply, staged: true, patches: []string{"p.diff"}, patchOpts: []string{"--exclude=d/f"},
	}},
	{sub: "apply", args: "-R", wantErr: true},
	{sub: "apply", args: "-R -", wantErr: true},
	{sub: "apply", args: "--reject p.diff", wantErr: true},
	{sub: "apply", args: "--unsafe-paths p.diff", wantErr: true},
	{sub: "apply", args: "--include=*.go p.diff", wantErr: true},
	{sub: "apply", args: "--directory= p.diff", wantErr: true},
	{sub: "apply", args: "../$P.diff", wantErr: true},
	{sub: "worktree", args: "remove -f ../wt", want: discardSpec{kind: worktreeRemoval}},
}

func specEqual(a, b discardSpec) bool {
	return a.kind == b.kind && a.source == b.source && a.staged == b.staged &&
		a.overlay == b.overlay && a.scope == b.scope && a.dest == b.dest &&
		strings.Join(a.pathspecs, "\x00") == strings.Join(b.pathspecs, "\x00") &&
		strings.Join(a.patches, "\x00") == strings.Join(b.patches, "\x00") &&
		strings.Join(a.patchOpts, "\x00") == strings.Join(b.patchOpts, "\x00")
}
