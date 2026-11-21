package guard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// bareIn sets up an ignored ign/ folder that holds a bare repository at
// ign/only/b.git, with the seed's main pushed into it and no remote.
func bareIn(r discardRepos) {
	r.write(r.clone, ".git/info/exclude", "ign/\n")
	bare := filepath.Join(r.clone, "ign", "only", "b.git")
	r.git(r.clone, "init", "-q", "--bare", "-b", "main", bare)
	r.git(r.seed, "push", "-q", bare, "main")
}

// upstreamPath is the bare upstream every clone in discardRepos fetches.
func upstreamPath(r discardRepos) string { return filepath.Join(filepath.Dir(r.clone), "up.git") }

// offBranchTag pushes a tag v2 to the upstream on a commit no branch
// holds, as a release tag cut from a branch since deleted.
func offBranchTag(r discardRepos) {
	r.git(r.seed, "checkout", "-q", "-b", "rel")
	r.write(r.seed, "r.txt", "r\n")
	r.git(r.seed, "add", "r.txt")
	r.git(r.seed, "commit", "-q", "-m", "release")
	r.git(r.seed, "tag", "v2")
	r.git(r.seed, "push", "-q", "origin", "v2")
	r.git(r.seed, "checkout", "-q", "main")
	r.git(r.seed, "branch", "-q", "-D", "rel")
}

// depIn sets up an ignored ign/ folder that holds a clone of the upstream
// at ign/dep, then runs change on it, if any, and points its origin across
// the network (overNetwork).
func depIn(change func(r discardRepos, dep string)) func(r discardRepos) {
	return func(r discardRepos) {
		r.write(r.clone, ".git/info/exclude", "ign/\n*.pyc\n")
		r.local(map[string]string{"a.txt": "mine\n", "x.pyc": "p"})
		dep := filepath.Join(r.clone, "ign", "dep")
		r.git(r.clone, "clone", "-q", upstreamPath(r), dep)
		if change != nil {
			change(r, dep)
		}
		overNetwork(r, dep)
	}
}

// localOrigin points the origin of the clone at rel back at the upstream's
// folder, after a setup that pointed it across the network.
func localOrigin(setup func(r discardRepos), rel string) func(r discardRepos) {
	return func(r discardRepos) {
		setup(r)
		r.git(filepath.Join(r.clone, rel), "remote", "set-url", "origin", upstreamPath(r))
	}
}

// gone runs change, which adds a linked worktree outside the drop at wt
// beside the clone, then deletes that worktree's folder, so only the
// clone's .git keeps what the worktree held.
func gone(change func(r discardRepos, dep string)) func(r discardRepos, dep string) {
	return func(r discardRepos, dep string) {
		change(r, dep)
		if err := os.RemoveAll(filepath.Join(filepath.Dir(r.clone), "wt")); err != nil {
			r.t.Fatal(err)
		}
	}
}

// TestFileDiscardHiddenRepos: a repository inside a folder git status
// lists as one entry. A bare one has no tree, so its refs must sit on a
// remote branch of its own; a tag counts as the clone's own; find -prune
// reads as a neutral test; and a find filter reaches into a nested clone,
// which is judged as a work tree of its own.
func TestFileDiscardHiddenRepos(t *testing.T) {
	runRepoRows(t, []repoRow{
		{"bare repository in an ignored folder", bareIn, "rm -rf ign",
			[]string{"/ign/only/b.git`, a git repository", "is on no remote branch"}},
		{"bare repository under a dropped folder", bareIn, "rm -rf ign/only", []string{"/ign/only/b.git`"}},
		{"bare repository's data", bareIn, "rm -rf ign/only/b.git/objects", []string{"git's own data"}},
		{"find in a bare repository", bareIn, "find ign -name HEAD -delete",
			[]string{"/ign/only/b.git/HEAD`", "git's own data"}},
		// A mirror's refs are its own unless it is SwiftPM's cache.
		{"mirror in an ignored folder", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "ign/\n")
			m := filepath.Join(r.clone, "ign", "m.git")
			r.git(r.clone, "clone", "-q", "--mirror", upstreamPath(r), m)
			overNetwork(r, m)
		}, "rm -rf ign", []string{"/ign/m.git`", "is on no remote branch"}},
		{"bare clone with its remote branches", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "ign/\n")
			bare := filepath.Join(r.clone, "ign", "b.git")
			r.git(r.clone, "init", "-q", "--bare", "-b", "main", bare)
			r.git(bare, "remote", "add", "origin", upstreamPath(r))
			r.git(bare, "fetch", "-q", "origin")
			r.git(bare, "update-ref", "HEAD", "refs/remotes/origin/main")
			overNetwork(r, bare)
		}, "rm -rf ign", nil},
		// A tag holds a commit only the clone may hold; fetched tags on the
		// remote's branches pass.
		{"clone with a local tag", swiftCheckout(func(r discardRepos, dep string) {
			r.write(dep, "t.txt", "t\n")
			r.git(dep, "add", "t.txt")
			r.git(dep, "commit", "-q", "-m", "tagged")
			r.git(dep, "tag", "keep")
			r.git(dep, "reset", "-q", "--hard", "origin/main")
		}), "rm -rf .build", []string{"/.build/checkouts/dep`", "is on no remote branch"}},
		{"clone with fetched tags", func(r discardRepos) {
			r.git(r.seed, "tag", "v1")
			r.git(r.seed, "push", "-q", "origin", "v1")
			swiftCheckout(nil)(r)
		}, "rm -rf .build", nil},
		{"clone with a fetched tag off every branch", func(r discardRepos) {
			offBranchTag(r)
			swiftCheckout(nil)(r)
		}, "rm -rf .build", []string{"/.build/checkouts/dep`", "is on no remote branch"}},
		{"clone whose only remote is a local folder", localOrigin(swiftCheckout(nil), ".build/checkouts/dep"),
			"rm -rf .build", []string{"/.build/checkouts/dep`", "a remote in a local folder does not count"}},
		// -prune is a test that is always true.
		{"find prunes an ignored folder", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "node_modules/\n")
			r.local(map[string]string{"a.txt": "mine\n", "web/node_modules/p/i.js": "x"})
		}, "find . -name node_modules -prune -exec rm -rf {} +", nil},
		{"find prunes a dirty folder", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			"find . -name d -prune -exec rm -rf {} +", []string{"`d/f.txt`"}},
		{"find prune then a delete", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "node_modules/\n")
			r.local(map[string]string{"a.txt": "mine\n"})
		}, "find . -name node_modules -prune -o -name '*.txt' -delete", []string{"`a.txt`"}},
		{"find prune then a delete, clean", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "node_modules/\n*.pyc\n")
			r.local(map[string]string{"a.txt": "mine\n", "x.pyc": "p"})
		}, "find . -name node_modules -prune -o -name '*.pyc' -delete", nil},
		{"find or in parentheses", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			`find . \( -name a.txt -o -name b.txt \) -delete`, []string{"the top of a git work tree"}},
		{"find or as a feeder", nil, "find . -name a.txt -o -name b.txt | xargs rm", []string{"the top of a git work tree"}},
		// A find filter takes only its matches from a nested clone.
		{"find filter over a clone with a local commit", depIn(func(r discardRepos, dep string) {
			r.write(dep, "c.txt", "c\n")
			r.git(dep, "add", "c.txt")
			r.git(dep, "commit", "-q", "-m", "local")
		}), "find . -name '*.pyc' -type f -delete", nil},
		{"whole drop of a clone with a local commit", depIn(func(r discardRepos, dep string) {
			r.write(dep, "c.txt", "c\n")
			r.git(dep, "add", "c.txt")
			r.git(dep, "commit", "-q", "-m", "local")
		}), "rm -rf ign", []string{"/ign/dep`", "is on no remote branch"}},
		{"find filter over an untracked match in a clone", depIn(func(r discardRepos, dep string) {
			r.write(dep, "y.pyc", "p")
		}), "find . -name '*.pyc' -type f -delete", []string{"/ign/dep/y.pyc`, which is untracked"}},
		{"find filter over an ignored match in a clone", depIn(func(r discardRepos, dep string) {
			r.write(dep, ".git/info/exclude", "*.pyc\n")
			r.write(dep, "y.pyc", "p")
		}), "find . -name '*.pyc' -type f -delete", nil},
		{"find filter over a dirty match in a clone", depIn(func(r discardRepos, dep string) {
			r.write(dep, "b.txt", "mine\n")
		}), "find . -name b.txt -delete", []string{"`b.txt` holds content no commit has"}},
		// A linked worktree outside the drop keeps its index, HEAD and log
		// in the clone's .git, which the drop takes, so one whose folder is
		// there denies, clean or not.
		{"clone whose outside worktree holds a detached commit", depIn(detachedOutside(false)),
			"rm -rf ign", liveTree},
		{"clone whose outside worktree log holds a commit", depIn(detachedOutside(true)),
			"rm -rf ign", liveTree},
		{"clone with a clean outside worktree", depIn(func(r discardRepos, dep string) {
			r.git(dep, "worktree", "add", "-q", "--detach", filepath.Join(filepath.Dir(r.clone), "wt"))
		}), "rm -rf ign", liveTree},
	})
}

// liveTree is the reason for a clone at ign/dep whose linked worktree at
// wt beside the clone still has its folder.
var liveTree = []string{"/ign/dep`, a git repository", "its linked worktree `", "/wt` still exists",
	"/ign/dep worktree remove ", "/wt`"}

// detachedOutside adds a linked worktree of the clone at dep outside the
// drop, on a detached HEAD, and commits there; with back, it then checks
// out the upstream's main again, so only the worktree's HEAD log holds the
// commit.
func detachedOutside(back bool) func(r discardRepos, dep string) {
	return func(r discardRepos, dep string) {
		wt := filepath.Join(filepath.Dir(r.clone), "wt")
		r.git(dep, "worktree", "add", "-q", "--detach", wt)
		r.write(wt, "w.txt", "w\n")
		r.git(wt, "add", "w.txt")
		r.git(wt, "commit", "-q", "-m", "detached")
		if back {
			r.git(wt, "checkout", "-q", "--detach", "origin/main")
		}
	}
}

// TestFileDiscardKeptRevs: a clone under a dropped folder goes only when
// no commit git keeps there is held there alone, for each ref backend git
// has. Each loss row leaves one commit in one place git keeps it: a reflog,
// a ref outside refs/heads, or the own HEAD or refs of a linked worktree
// whose folder is gone. A worktree whose folder is there denies. The pass
// rows hold nothing of their own.
func TestFileDiscardKeptRevs(t *testing.T) {
	lost := []string{"/ign/dep`, a git repository", "is on no remote branch"}
	rebasing := []string{"/ign/dep`, a git repository", "it stopped a rebase", "autostash"}
	for _, format := range refFormats(t) {
		inDep := func(change func(r discardRepos, dep string)) func(r discardRepos) {
			return cloneIn(format, "ign/", "ign/dep", change)
		}
		rows := []repoRow{
			{"commit only HEAD's log holds", inDep(func(r discardRepos, dep string) {
				r.git(dep, "checkout", "-q", "--detach")
				commitFile(r, dep, "n.txt")
				r.git(dep, "checkout", "-q", "main")
			}), "rm -rf ign", lost},
			{"commit only a branch log holds", inDep(func(r discardRepos, dep string) {
				commitFile(r, dep, "n.txt")
				r.git(dep, "reset", "-q", "--hard", "origin/main")
			}), "rm -rf ign", lost},
			{"stash", inDep(func(r discardRepos, dep string) {
				r.write(dep, "a.txt", "mine\n")
				r.git(dep, "stash", "-q")
			}), "rm -rf ign", lost},
			{"refs/bisect", inDep(lonelyRef("refs/bisect/bad")), "rm -rf ign", lost},
			{"refs/original", inDep(lonelyRef("refs/original/refs/heads/main")), "rm -rf ign", lost},
			{"refs/jj", inDep(lonelyRef("refs/jj/keep/one")), "rm -rf ign", lost},
			{"git notes", inDep(func(r discardRepos, dep string) {
				r.git(dep, "notes", "add", "-m", "note", "HEAD")
			}), "rm -rf ign", lost},
			// A worktree whose folder is gone keeps its HEAD, refs and logs
			// in the clone's .git alone.
			{"gone worktree's detached commit", inDep(gone(detachedOutside(false))), "rm -rf ign", lost},
			{"gone worktree's HEAD log", inDep(gone(detachedOutside(true))), "rm -rf ign", lost},
			{"gone worktree's refs/worktree", inDep(gone(inOutsideWorktree(lonelyRef("refs/worktree/keep")))),
				"rm -rf ign", lost},
			{"gone worktree's refs/bisect", inDep(gone(inOutsideWorktree(lonelyRef("refs/bisect/bad")))),
				"rm -rf ign", lost},
			{"gone worktree's ref log", inDep(gone(inOutsideWorktree(func(r discardRepos, wt string) {
				lonelyRef("refs/bisect/bad", "--create-reflog")(r, wt)
				r.git(wt, "update-ref", "refs/bisect/bad", "origin/main")
			}))), "rm -rf ign", lost},
			{"rebase stopped with an autostash", inDep(stoppedRebase), "rm -rf ign", rebasing},
			{"rebase --apply stopped with an autostash", inDep(appliedRebase), "rm -rf ign", rebasing},
			{"merge stopped with an autostash", inDep(mergeAutostash), "rm -rf ign",
				[]string{"/ign/dep`, a git repository", "it stopped a merge"}},
			{"gone worktree's rebase autostash", inDep(gone(inOutsideWorktree(stoppedRebase))), "rm -rf ign",
				[]string{"its worktree wt stopped a rebase"}},
			{"gone worktree's merge autostash", inDep(gone(inOutsideWorktree(mergeAutostash))), "rm -rf ign",
				[]string{"its worktree wt stopped a merge"}},
			// A worktree whose folder is there keeps its index in the clone's
			// .git, so a change it staged and then undid in its files goes.
			{"outside worktree's staged change", inDep(inOutsideWorktree(func(r discardRepos, wt string) {
				r.write(wt, "a.txt", "staged\n")
				r.git(wt, "add", "a.txt")
				r.write(wt, "a.txt", "a\n")
			})), "rm -rf ign", liveTree},
			{"clean outside worktree", inDep(inOutsideWorktree(nil)), "rm -rf ign", liveTree},
			{"prefetched commit", inDep(func(r discardRepos, dep string) {
				commitFile(r, r.seed, "p.txt")
				r.git(r.seed, "push", "-q", "origin", "main")
				r.git(dep, "fetch", "-q", "--prefetch", "origin")
				prefetched(r, dep)
			}), "rm -rf ign", nil},
			{"clean clone", inDep(nil), "rm -rf ign", nil},
			{"clone whose work is all pushed", inDep(func(r discardRepos, dep string) {
				r.git(dep, "checkout", "-q", "-b", "topic")
				commitFile(r, dep, "n.txt")
				r.git(dep, "push", "-q", "origin", "topic")
			}), "rm -rf ign", nil},
			{"clone with only remote branches", inDep(func(r discardRepos, dep string) {
				r.git(dep, "checkout", "-q", "--detach", "origin/main")
				r.git(dep, "branch", "-q", "-D", "main")
			}), "rm -rf ign", nil},
			{"clone after an upstream force-push", inDep(func(r discardRepos, dep string) {
				forcePush(r, func() { r.git(dep, "fetch", "-q") })
				r.git(dep, "fetch", "-q")
				r.git(dep, "reset", "-q", "--hard", "origin/main")
				remoteLogHolds(r, dep)
			}), "rm -rf ign", nil},
			{"SwiftPM checkout after an update", swiftPackage(format), "rm -rf .build", nil},
			{"node_modules clone", cloneIn(format, "node_modules/", "node_modules/dep", nil),
				"rm -rf node_modules", nil},
		}
		for i := range rows {
			rows[i].name += " (" + format + ")"
		}
		runRepoRows(t, rows)
	}
}

// refFormats are the ref backends the clones in a test use: files, and
// reftable when git has it, from 2.45 on.
func refFormats(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "version").Output()
	if err != nil {
		t.Skip("git not on PATH")
	}
	var major, minor int
	if _, err := fmt.Sscanf(string(out), "git version %d.%d", &major, &minor); err != nil {
		t.Fatalf("git version printed %q", out)
	}
	if major > 2 || major == 2 && minor >= 45 {
		return []string{"files", "reftable"}
	}
	return []string{"files"}
}

// cloneIn sets up a folder the clone ignores by the pattern ignored that
// holds a clone of the upstream at rel with the ref backend format, then
// runs change on it, if any, and points its origin across the network.
func cloneIn(format, ignored, rel string, change func(r discardRepos, dep string)) func(r discardRepos) {
	return func(r discardRepos) {
		r.write(r.clone, ".git/info/exclude", ignored+"\n")
		dep := filepath.Join(r.clone, rel)
		r.git(r.clone, slices.Concat([]string{"clone", "-q"}, formatFlag(format), []string{upstreamPath(r), dep})...)
		if change != nil {
			change(r, dep)
		}
		overNetwork(r, dep)
	}
}

// formatFlag is the git clone option for the ref backend format, none for
// files, which a git older than 2.45 has no option for.
func formatFlag(format string) []string {
	if format == "files" {
		return nil
	}
	return []string{"--ref-format=" + format}
}

// commitFile commits a new file name in the work tree at dir.
func commitFile(r discardRepos, dir, name string) {
	r.t.Helper()
	r.write(dir, name, name+"\n")
	r.git(dir, "add", name)
	r.git(dir, "commit", "-q", "-m", name)
}

// lonelyRef points ref, in the work tree it runs in, at a new commit on top of
// HEAD that nothing else holds, with the update-ref options opts.
func lonelyRef(ref string, opts ...string) func(r discardRepos, dir string) {
	return func(r discardRepos, dir string) {
		r.t.Helper()
		out, err := gitCmd(dir, "commit-tree", "HEAD^{tree}", "-p", "HEAD", "-m", ref).Output()
		if err != nil {
			r.t.Fatalf("git commit-tree: %v", err)
		}
		r.git(dir, slices.Concat([]string{"update-ref"}, opts, []string{ref, strings.TrimSpace(string(out))})...)
	}
}

// stoppedRebase leaves the work tree at dir in a rebase it started with
// --autostash and stopped on an exec that fails: its own commit pushed,
// its work tree clean, and its change kept only in the commit git names
// in rebase-merge/autostash.
func stoppedRebase(r discardRepos, dir string) {
	r.t.Helper()
	commitFile(r, dir, "n.txt")
	r.git(dir, "push", "-q", "origin", "HEAD:refs/heads/rebased")
	r.write(dir, "a.txt", "mine\n")
	if err := gitCmd(dir, "rebase", "-q", "--autostash", "--exec", "false", "HEAD~1").Run(); err == nil {
		r.t.Fatal("the rebase did not stop")
	}
	clean(r, dir)
}

// appliedRebase leaves the clone at dir in a rebase --apply it started
// with --autostash, stopped on a conflict the user took the upstream's
// side of: its own commit pushed, its work tree clean, and its change
// kept only in the commit git names in rebase-apply/autostash.
func appliedRebase(r discardRepos, dir string) {
	r.t.Helper()
	r.write(dir, "b.txt", "mine\n")
	r.git(dir, "commit", "-q", "-am", "mine")
	r.git(dir, "push", "-q", "origin", "HEAD:refs/heads/mine")
	r.write(r.seed, "b.txt", "theirs\n")
	r.git(r.seed, "commit", "-q", "-am", "theirs")
	r.git(r.seed, "push", "-q", "origin", "main")
	r.git(dir, "fetch", "-q")
	r.write(dir, "a.txt", "mine\n")
	if err := gitCmd(dir, "rebase", "-q", "--apply", "--autostash", "origin/main").Run(); err == nil {
		r.t.Fatal("the rebase did not stop")
	}
	r.git(dir, "checkout", "-q", "HEAD", "--", "b.txt")
	clean(r, dir)
}

// mergeAutostash leaves the work tree at dir as a merge it started with
// --autostash leaves it: its change in the commit MERGE_AUTOSTASH names,
// written as git writes it for either ref backend, and its work tree
// clean.
func mergeAutostash(r discardRepos, dir string) {
	r.t.Helper()
	r.write(dir, "a.txt", "mine\n")
	out, err := gitCmd(dir, "stash", "create").Output()
	if err != nil || len(out) == 0 {
		r.t.Fatalf("git stash create: %v", err)
	}
	r.git(dir, "update-ref", "MERGE_AUTOSTASH", strings.TrimSpace(string(out)))
	r.git(dir, "checkout", "-q", "--", "a.txt")
	clean(r, dir)
}

// clean fails the test unless git status lists nothing in the work tree at
// dir, so only the state a row sets up can deny it.
func clean(r discardRepos, dir string) {
	r.t.Helper()
	if out, err := gitCmd(dir, "status", "--porcelain").Output(); err != nil || len(out) > 0 {
		r.t.Fatalf("git status in %s: %v\n%s", dir, err, out)
	}
}

// prefetched fails the test unless a ref git fetch --prefetch fills holds
// a commit no remote branch has.
func prefetched(r discardRepos, dir string) {
	r.t.Helper()
	out, err := gitCmd(dir, "rev-list", "--glob=refs/prefetch/", "--not", "--remotes").Output()
	if err != nil || len(out) == 0 {
		r.t.Fatalf("no prefetched ref of %s holds a commit off the remote branches (%v)", dir, err)
	}
}

// inOutsideWorktree adds a linked worktree of the clone at dep outside the
// drop, on a detached HEAD, and runs change in it, if any.
func inOutsideWorktree(change func(r discardRepos, wt string)) func(r discardRepos, dep string) {
	return func(r discardRepos, dep string) {
		wt := filepath.Join(filepath.Dir(r.clone), "wt")
		r.git(dep, "worktree", "add", "-q", "--detach", wt)
		if change != nil {
			change(r, wt)
		}
	}
}

// forcePush pushes a commit to the upstream's main, runs fetched, then
// replaces that commit with another on its parent, so a clone that fetched
// the old one keeps it only in the log of its remote branch.
func forcePush(r discardRepos, fetched func()) {
	r.t.Helper()
	commitFile(r, r.seed, "old.txt")
	r.git(r.seed, "push", "-q", "origin", "main")
	fetched()
	r.git(r.seed, "reset", "-q", "--hard", "HEAD~")
	commitFile(r, r.seed, "new.txt")
	r.git(r.seed, "push", "-q", "-f", "origin", "main")
}

// swiftPackage sets up what SwiftPM leaves after an update: a mirror of
// the upstream in .build/repositories whose origin lies across the
// network, and a `clone --shared` of the mirror in .build/checkouts with
// the ref backend format, moved from tag v1 to v2 across an upstream
// force-push of main. SwiftPM writes the mirror's folder as the
// checkout's origin.
func swiftPackage(format string) func(r discardRepos) {
	return func(r discardRepos) {
		mirror, checkout := swiftLayout(r, format, ".build")
		r.git(checkout, "checkout", "-q", "--detach", "v1")
		update := func() {
			r.git(mirror, "fetch", "-q")
			r.git(checkout, "fetch", "-q", "--tags")
		}
		forcePush(r, update)
		r.git(r.seed, "tag", "v2")
		r.git(r.seed, "push", "-q", "origin", "v2")
		update()
		r.git(checkout, "checkout", "-q", "--detach", "v2")
		remoteLogHolds(r, checkout)
		overNetwork(r, mirror)
	}
}

// swiftLayout makes SwiftPM's layout for one package with tag v1 on the
// upstream, in the scratch folder at scratch under an ignored .build, as
// `swift package resolve` leaves it: its workspace-state.json, a mirror of
// the upstream at repositories/dep-1, and a `clone --shared --no-checkout`
// of the mirror at checkouts/dep. The mirror's origin is still the
// upstream's folder.
func swiftLayout(r discardRepos, format, scratch string) (mirror, checkout string) {
	r.t.Helper()
	r.write(r.clone, ".git/info/exclude", ".build/\n")
	r.write(r.clone, filepath.Join(scratch, "workspace-state.json"), "{}\n")
	mirror = filepath.Join(r.clone, scratch, "repositories", "dep-1")
	checkout = filepath.Join(r.clone, scratch, "checkouts", "dep")
	r.git(r.seed, "tag", "v1")
	r.git(r.seed, "push", "-q", "origin", "v1")
	r.git(r.clone, "clone", "-q", "--mirror", upstreamPath(r), mirror)
	r.git(r.clone, slices.Concat([]string{"clone", "-q", "--shared", "--no-checkout"}, formatFlag(format),
		[]string{mirror, checkout})...)
	return mirror, checkout
}

// remoteLogHolds fails the test unless a reflog of the clone at dir holds
// a commit no remote branch has, as a remote branch's log does after a
// fetch of a force-push, so the row checks that such a log is left out.
func remoteLogHolds(r discardRepos, dir string) {
	r.t.Helper()
	out, err := gitCmd(dir, "rev-list", "--all", "--reflog", "--not", "--remotes").Output()
	if err != nil || len(out) == 0 {
		r.t.Fatalf("no reflog of %s holds a commit off the remote branches (%v)", dir, err)
	}
}

// TestFileDiscardNodeModules: a node_modules folder is searched for a
// repository down to its packages only, so one far past the folder cap
// passes, while a clone that is a package is still judged.
func TestFileDiscardNodeModules(t *testing.T) {
	r := newDiscardRepos(t)
	r.write(r.clone, ".git/info/exclude", "node_modules/\n")
	files := map[string]string{}
	for i := range 40 {
		files[fmt.Sprintf("node_modules/p%d/lib/a/b/c/d/i.js", i)] = "x"
		files[fmt.Sprintf("node_modules/@s/q%d/lib/a/i.js", i)] = "x"
	}
	r.local(files)
	g := NewFileDiscard(config.Config{})
	g.scanDirs = 100
	check := func(command string) *Denial {
		return g.Check(Input{Event: EventBash, Command: command, Cwd: r.clone})
	}
	if d := check("rm -rf node_modules"); d != nil {
		t.Errorf("rm -rf node_modules = %q, want a pass", d.Reason)
	}
	if d := check("find node_modules -name '*.js' -delete"); d != nil {
		t.Errorf("find -delete in node_modules = %q, want a pass", d.Reason)
	}
	// A name check reads every folder, so the cap holds there.
	if d := check("find node_modules -name '*.js' | xargs rm"); d == nil ||
		!strings.Contains(d.Reason, "could not check every") {
		t.Errorf("xargs over node_modules = %v, want the scan cap", d)
	}
	for _, pkg := range []string{"node_modules/dep", "node_modules/@s/dep"} {
		dep := filepath.Join(r.clone, pkg)
		r.git(r.clone, "clone", "-q", upstreamPath(r), dep)
		r.write(dep, "c.txt", pkg+"\n") // one commit per package, not one hash for both
		r.git(dep, "add", "c.txt")
		r.git(dep, "commit", "-q", "-m", "local")
		if d := check("rm -rf node_modules"); d == nil || !strings.Contains(d.Reason, "/"+pkg+"`") {
			t.Errorf("a clone at %s with a local commit = %v, want a deny naming it", pkg, d)
		}
		r.git(dep, "push", "-q", "origin", "HEAD:refs/heads/"+strings.ReplaceAll(pkg, "@", ""))
		overNetwork(r, dep)
	}
}

// submoduleIn sets up an ignored vendor/ folder that holds S, a clone of
// the upstream with a submodule at sub, under the name name, cloned from a
// bare upstream of its own, the commit that adds it pushed. Then it runs
// change on S, if any. S's origin and the submodule's then point across
// the network.
func submoduleIn(name string, change func(r discardRepos, s string)) func(r discardRepos) {
	return func(r discardRepos) {
		r.t.Helper()
		r.write(r.clone, ".git/info/exclude", "vendor/\n")
		root := filepath.Dir(r.clone)
		seed, up := filepath.Join(root, "sm-seed"), filepath.Join(root, "sm.git")
		r.git(root, "init", "-q", "--bare", "-b", "main", up)
		r.git(root, "init", "-q", "-b", "main", seed)
		commitFile(r, seed, "f")
		r.git(seed, "push", "-q", up, "main")
		s := filepath.Join(r.clone, "vendor", "S")
		r.git(r.clone, "clone", "-q", upstreamPath(r), s)
		r.git(s, "-c", "protocol.file.allow=always", "submodule", "add", "-q", "--name", name, up, "sub")
		r.git(s, "commit", "-q", "-m", "add sub")
		r.git(s, "push", "-q", "origin", "main")
		if change != nil {
			change(r, s)
		}
		overNetwork(r, s)
		r.git(s, "config", "--file", filepath.Join(s, ".git", "modules", name, "config"),
			"remote.origin.url", "https://example.invalid/sm.git")
	}
}

// deinit commits a new file in S's submodule, pushes it to the branch push
// on the submodule's upstream unless push is "", and then takes the
// submodule out of S's work tree with git submodule deinit -f, so only
// S/.git/modules keeps its repository.
func deinit(push string) func(r discardRepos, s string) {
	return func(r discardRepos, s string) {
		r.t.Helper()
		sub := filepath.Join(s, "sub")
		commitFile(r, sub, "n.txt")
		if push != "" {
			r.git(sub, "push", "-q", "origin", "HEAD:refs/heads/"+push)
		}
		r.git(s, "submodule", "deinit", "-q", "-f", "sub")
		clean(r, s)
	}
}

// TestFileDiscardSubmoduleRepos: a repository a drop takes whole keeps
// each submodule's repository in .git/modules, which the drop takes too, so
// each one is judged by the rules for a repository with no work tree. One
// that git submodule deinit or git rm took out of the work tree is seen
// nowhere else.
func TestFileDiscardSubmoduleRepos(t *testing.T) {
	lost := []string{"/vendor/S`, a git repository", "submodule's repository at `", "is on no remote branch"}
	runRepoRows(t, []repoRow{
		{"deinited submodule with a local commit", submoduleIn("sub", deinit("")), "rm -rf vendor",
			append(slices.Clone(lost), "/vendor/S/.git/modules/sub`")},
		{"deinited submodule under a slashed name", submoduleIn("lib/x", deinit("")), "rm -rf vendor",
			append(slices.Clone(lost), "/vendor/S/.git/modules/lib/x`")},
		{"submodule git rm took out", submoduleIn("sub", func(r discardRepos, s string) {
			commitFile(r, filepath.Join(s, "sub"), "n.txt")
			r.git(s, "rm", "-q", "-f", "sub")
			r.git(s, "commit", "-q", "-m", "drop sub")
			r.git(s, "push", "-q", "origin", "main")
			clean(r, s)
		}), "rm -rf vendor", lost},
		{"deinited submodule with a staged change", submoduleIn("sub", func(r discardRepos, s string) {
			r.write(filepath.Join(s, "sub"), "f", "staged\n")
			r.git(filepath.Join(s, "sub"), "add", "f")
			r.git(s, "submodule", "deinit", "-q", "-f", "sub")
			clean(r, s)
		}), "rm -rf vendor", []string{"submodule's repository at `", "its index holds changes no commit has"}},
		{"gone worktree's submodule with a local commit", submoduleIn("sub", func(r discardRepos, s string) {
			wt := filepath.Join(filepath.Dir(r.clone), "wt")
			r.git(s, "worktree", "add", "-q", "--detach", wt)
			r.git(wt, "-c", "protocol.file.allow=always", "submodule", "update", "-q", "--init")
			commitFile(r, filepath.Join(wt, "sub"), "n.txt")
			if err := os.RemoveAll(wt); err != nil {
				r.t.Fatal(err)
			}
		}), "rm -rf vendor", append(slices.Clone(lost), "/vendor/S/.git/worktrees/wt/modules/sub`")},
		{"checked-out submodule with a local commit", submoduleIn("sub", func(r discardRepos, s string) {
			commitFile(r, filepath.Join(s, "sub"), "n.txt")
		}), "rm -rf vendor", []string{"/vendor/S`, a git repository", "changes no commit has"}},
		{"deinited submodule whose commits are pushed", submoduleIn("sub", deinit("topic")), "rm -rf vendor", nil},
		{"clean checked-out submodule", submoduleIn("sub", nil), "rm -rf vendor", nil},
	})
}

// flagged marks name in the work tree at dir with the update-index option
// flag (--assume-unchanged or --skip-worktree) and then writes content to
// it, "" for no write, so git status shows no change.
func flagged(r discardRepos, dir, flag, name, content string) {
	r.t.Helper()
	r.git(dir, "update-index", flag, name)
	if content != "" {
		r.write(dir, name, content)
	}
	clean(r, dir)
}

// TestFileDiscardFlaggedFiles: git status does not read a file marked
// assume-unchanged or skip-worktree, so belt compares each such file that
// exists with its index entry, at the top and in a repository a drop takes
// whole. A file a sparse checkout leaves out is missing and passes.
func TestFileDiscardFlaggedFiles(t *testing.T) {
	hidden := []string{"/.build/checkouts/dep`, a git repository", "changes no commit has",
		"`a.txt` is marked assume-unchanged or skip-worktree"}
	runRepoRows(t, []repoRow{
		{"assume-unchanged edit in a clone", swiftCheckout(func(r discardRepos, dep string) {
			flagged(r, dep, "--assume-unchanged", "a.txt", "mine\n")
		}), "rm -rf .build", hidden},
		{"skip-worktree edit in a clone", swiftCheckout(func(r discardRepos, dep string) {
			flagged(r, dep, "--skip-worktree", "a.txt", "mine\n")
		}), "rm -rf .build", hidden},
		{"assume-unchanged file as committed in a clone", swiftCheckout(func(r discardRepos, dep string) {
			flagged(r, dep, "--assume-unchanged", "a.txt", "")
		}), "rm -rf .build", nil},
		{"sparse clone", swiftCheckout(func(r discardRepos, dep string) {
			r.git(dep, "sparse-checkout", "set", "--no-cone", "/a.txt")
			if exists(filepath.Join(dep, "d", "f.txt")) {
				r.t.Fatal("the sparse checkout kept d/f.txt")
			}
		}), "rm -rf .build", nil},
		{"assume-unchanged edit at the top", func(r discardRepos) {
			flagged(r, r.clone, "--assume-unchanged", "a.txt", "mine\n")
		}, "rm a.txt", []string{"`a.txt` holds content no commit has"}},
		{"skip-worktree edit at the top", func(r discardRepos) {
			flagged(r, r.clone, "--skip-worktree", "d/f.txt", "mine\n")
		}, "rm -rf d", []string{"`d/f.txt` holds content no commit has"}},
		{"assume-unchanged edit a find takes", func(r discardRepos) {
			flagged(r, r.clone, "--assume-unchanged", "a.txt", "mine\n")
		}, "find . -name '*.txt' -delete", []string{"`a.txt` holds content no commit has"}},
		{"assume-unchanged file as committed at the top", func(r discardRepos) {
			flagged(r, r.clone, "--assume-unchanged", "a.txt", "")
		}, "rm a.txt", nil},
		{"sparse checkout at the top", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "*.log\n")
			r.git(r.clone, "sparse-checkout", "set", "--no-cone", "/a.txt", "/b.txt")
			r.local(map[string]string{"d/x.log": "l"})
		}, "rm -rf d", nil},
	})
}
