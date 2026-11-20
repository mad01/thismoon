package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// mainCheckoutTree is a hermetic layout for the main-checkout tests: a main
// checkout repo with a linked worktree of it, a store checkout with no
// remote, a directory with no repo, and a link from that directory into
// the main checkout. No git runs: the repos are laid out by hand in the
// shape git leaves them (mkRepo, mkWorktree).
type mainCheckoutTree struct {
	main, worktree, store, plain string
}

func newMainCheckoutTree(t *testing.T) mainCheckoutTree {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tree := mainCheckoutTree{
		main:     filepath.Join(home, "src", "repo"),
		worktree: filepath.Join(home, "wt", "repo", "feat"),
		store:    filepath.Join(home, "store"),
		plain:    filepath.Join(home, "plain"),
	}
	mkRepo(t, tree.main)
	mkWorktree(t, tree.main, tree.worktree, "feat")
	mkRepo(t, tree.store)
	mkdirs(t, tree.plain)
	writeFiles(t, map[string]string{
		filepath.Join(tree.main, "README.md"):     "main\n",
		filepath.Join(tree.worktree, "README.md"): "worktree\n",
	})
	symlink(t, filepath.Join(tree.main, "README.md"), filepath.Join(tree.plain, "linked.md"))
	return tree
}

// mainCheckoutGuard builds the guard for one event in the given mode with
// fake git backends: every checkout is on branch, and the repo at dir is
// github.com/you/<dir's base name>.
func mainCheckoutGuard(event string, toggle config.Toggle, branch string) *MainCheckout {
	g := NewMainCheckout(
		config.Config{Guards: map[string]config.Toggle{MainCheckoutID: toggle}},
		event,
	)
	g.currentBranch = func(string) string { return branch }
	g.resolveRepo = func(dir string) string { return "github.com/you/" + filepath.Base(dir) }
	return g
}

var denyMode = config.Toggle{Mode: config.ModeDeny}

func TestMainCheckoutWrite(t *testing.T) {
	tree := newMainCheckoutTree(t)
	tests := []struct {
		name   string
		file   string
		toggle config.Toggle
		want   []string // substrings of the reason; nil means allowed
	}{
		{
			"a file in the main checkout", filepath.Join(tree.main, "README.md"), denyMode,
			[]string{
				"belt[main-checkout]: refused: " + filepath.Join(tree.main, "README.md"),
				"git -C " + tree.main + " worktree add ~/.worktrees/repo/<slug> -b <branch>",
				"then edit ~/.worktrees/repo/<slug>/README.md",
			},
		},
		{
			"a new file in a new directory", filepath.Join(tree.main, "docs", "new", "x.md"), denyMode,
			[]string{"edit ~/.worktrees/repo/<slug>/docs/new/x.md"},
		},
		{
			"a file reached through a link into the main checkout",
			filepath.Join(tree.plain, "linked.md"), denyMode,
			[]string{"in the main checkout " + tree.main},
		},
		{
			"worktree_root names the place for worktrees", filepath.Join(tree.main, "README.md"),
			config.Toggle{Mode: config.ModeDeny, WorktreeRoot: "/work/trees/"},
			[]string{"worktree add /work/trees/repo/<slug> -b <branch>"},
		},
		{"a file in a linked worktree", filepath.Join(tree.worktree, "README.md"), denyMode, nil},
		{"a file outside any checkout", filepath.Join(tree.plain, "x.md"), denyMode, nil},
		{"a relative path", "README.md", denyMode, nil},
		{
			"exclude_paths covers a store with no remote", filepath.Join(tree.store, "fact.md"),
			config.Toggle{Mode: config.ModeDeny, ExcludePaths: []string{tree.store}},
			nil,
		},
		{
			"allow_repos covers the checkout's origin", filepath.Join(tree.store, "fact.md"),
			config.Toggle{Mode: config.ModeDeny, AllowRepos: []string{"github.com/you/store"}},
			nil,
		},
		{
			"allow_repos names another repo", filepath.Join(tree.store, "fact.md"),
			config.Toggle{Mode: config.ModeDeny, AllowRepos: []string{"github.com/you/other"}},
			[]string{"in the main checkout " + tree.store},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := mainCheckoutGuard(EventWrite, tt.toggle, "main")
			d := g.Check(Input{Event: EventWrite, FilePath: tt.file, Content: "x"})
			wantDenial(t, d, tt.want)
		})
	}
}

func TestMainCheckoutBash(t *testing.T) {
	tree := newMainCheckoutTree(t)
	m, wt, s := tree.main, tree.worktree, tree.store
	// The store's origin/HEAD names main, so master is not its default.
	writeFiles(t, map[string]string{
		filepath.Join(s, ".git", "refs", "remotes", "origin", "HEAD"): "ref: refs/remotes/origin/main\n",
	})
	tests := []struct {
		name, command, cwd string
		branch             string
		want               []string // substrings of the reason; nil means allowed
	}{
		{"commit in the session cwd", "git commit -m x", m, "main", []string{
			"refused: git commit changes the main checkout " + m,
			"git -C " + m + " worktree add ~/.worktrees/repo/<slug> -b <branch>",
			"then cd ~/.worktrees/repo/<slug>",
		}},
		{
			"git -C into the main checkout", "git -C " + m + " commit -m x", tree.plain, "main",
			[]string{"git commit changes the main checkout"},
		},
		{
			"cd into the main checkout", "cd " + m + " && git commit -m x", tree.plain, "main",
			[]string{"git commit changes"},
		},
		{
			"a subshell cd", "(cd " + m + " && git commit -m x)", tree.plain, "main",
			[]string{"git commit changes"},
		},
		{
			"an sh -c script", "sh -c 'cd " + m + " && git commit -m x'", tree.plain, "main",
			[]string{"git commit changes"},
		},
		{
			"a substitution", "echo $(git -C " + m + " commit -m x)", tree.plain, "main",
			[]string{"git commit changes"},
		},
		{
			"switch to another branch",
			"git checkout feature",
			m,
			"main",
			[]string{"git checkout changes"},
		},
		{"create a branch", "git switch -c feature", m, "main", []string{"git switch changes"}},
		{
			"a path checkout",
			"git checkout -- README.md",
			m,
			"main",
			[]string{"git checkout changes"},
		},
		{"reset", "git reset --hard origin/main", m, "main", []string{"git reset changes"}},
		{"merge", "git merge feature", m, "main", []string{"git merge changes"}},
		{"rebase", "git rebase origin/main", m, "main", []string{"git rebase changes"}},
		{"stash", "git stash", m, "main", []string{"git stash changes"}},
		{"stash pop", "git stash pop", m, "main", []string{"git stash changes"}},
		{"add", "git add README.md", m, "main", []string{"git add changes"}},
		{"forced clean", "git clean -fd", m, "main", []string{"git clean changes"}},
		{"apply", "git apply fix.patch", m, "main", []string{"git apply changes"}},
		{"pull off the default branch", "git pull", m, "feature", []string{"git pull changes"}},
		{
			"pull of another branch",
			"git pull origin feature",
			m,
			"main",
			[]string{"git pull changes"},
		},
		{"pull with a merge commit", "git pull --no-ff", m, "main", []string{"git pull changes"}},
		{
			"pull with a squash",
			"git pull --squash origin main",
			m,
			"main",
			[]string{"git pull changes"},
		},
		{
			"pull with an attached strategy option",
			"git pull -Xtheirs origin main",
			m,
			"main",
			[]string{"git pull changes"},
		},
		{
			"pull with an attached strategy",
			"git pull -qsours origin main",
			m,
			"main",
			[]string{"git pull changes"},
		},
		{
			"rename the checked-out branch",
			"git branch -m main foo",
			m,
			"main",
			[]string{"git branch changes"},
		},
		{
			"rename the checked-out branch by its new name",
			"git branch -fM foo",
			m,
			"main",
			[]string{"git branch changes"},
		},
		{
			"sparse-checkout set",
			"git sparse-checkout set x",
			m,
			"main",
			[]string{"git sparse-checkout changes"},
		},
		{
			"update-index",
			"git update-index --assume-unchanged a",
			m,
			"main",
			[]string{"git update-index changes"},
		},
		{
			"submodule update",
			"git submodule update --init",
			m,
			"main",
			[]string{"git submodule changes"},
		},
		{
			"switch to master where origin/HEAD names main",
			"git checkout master",
			s,
			"main",
			[]string{"git checkout changes"},
		},

		{"status", "git status", m, "main", nil},
		{"log and diff", "git log -1 && git diff HEAD~1", m, "main", nil},
		{"fetch", "git fetch origin", m, "main", nil},
		{
			"make a worktree",
			"git -C " + m + " worktree add ~/.worktrees/repo/x -b x",
			tree.plain,
			"main",
			nil,
		},
		{"back to the default branch", "git checkout main", m, "feature", nil},
		{"switch back quietly", "git switch -q master", m, "feature", nil},
		{"stash list", "git stash list", m, "main", nil},
		{"dry-run clean", "git clean -n", m, "main", nil},
		{"apply check", "git apply --check fix.patch", m, "main", nil},
		{"bare pull on the default branch", "git pull", m, "main", nil},
		{"fast-forward pull", "git pull --ff-only", m, "main", nil},
		{"pull of the default branch", "git pull --rebase origin main", m, "main", nil},
		{"pull with a depth", "git pull --depth 1 origin main", m, "main", nil},
		{"pull on a branch git cannot read", "git pull --ff-only", m, "", nil},
		{"rename another branch", "git branch -m old new", m, "main", nil},
		{"list branches", "git branch -a", m, "main", nil},
		{"sparse-checkout list", "git sparse-checkout list", m, "main", nil},
		{"submodule status", "git submodule --quiet status", m, "main", nil},
		{"switch to origin/HEAD's branch", "git checkout main", s, "feature", nil},
		{"commit in a linked worktree", "git commit -m x", wt, "feat", nil},
		{"git -C into a linked worktree", "git -C " + wt + " commit -m x", m, "feat", nil},
		{
			"cd from the main checkout to a worktree",
			"cd " + wt + " && git commit -m x",
			m,
			"feat",
			nil,
		},
		{"commit outside any checkout", "git commit -m x", tree.plain, "main", nil},
		{"a commit named in text", "echo 'git -C " + m + " commit'", tree.plain, "main", nil},
		{
			"a commit in a heredoc body",
			"cat <<'EOF'\ngit -C " + m + " commit -m x\nEOF",
			tree.plain,
			"main",
			nil,
		},
		{"a directory only a shell can tell", "cd $(pick) && git commit -m x", m, "main", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := mainCheckoutGuard(EventBash, denyMode, tt.branch)
			wantDenial(t, g.Check(bashCall(tt.command, tt.cwd)), tt.want)
		})
	}
}

// TestMainCheckoutWorktreeCommands: a main checkout at or under a
// worktree_commands path gets that entry's command, filled in for the
// checkout, with the longest path winning; the reason names the new
// worktree without a path, since the tool picks it. Every other checkout
// still gets git worktree add.
func TestMainCheckoutWorktreeCommands(t *testing.T) {
	tree := newMainCheckoutTree(t)
	home := filepath.Dir(filepath.Dir(tree.main))
	src := filepath.Join(home, "src")
	symlink(t, src, filepath.Join(home, "linked-src"))
	cmds := []config.WorktreeCommand{
		{Path: home, Command: "outer {repo}"},
		{Path: src + "/", Command: "mkwt new {slug} {repo} --from {checkout} --branch {branch}"},
	}
	tests := []struct {
		name  string
		cmds  []config.WorktreeCommand
		event string
		in    Input
		want  []string
		not   []string
	}{
		{
			"an edit under the longest path", cmds, EventWrite,
			Input{Event: EventWrite, FilePath: filepath.Join(tree.main, "docs", "x.md")},
			[]string{
				"edit the file there: `mkwt new <slug> repo --from " + tree.main +
					" --branch <branch>`, then edit docs/x.md in the new worktree.",
			},
			[]string{"outer", "git -C", "~/.worktrees"},
		},
		{
			"a git command under the longest path", cmds, EventBash,
			bashCall("git commit -m x", tree.main),
			[]string{
				"Run it in a linked worktree: `mkwt new <slug> repo --from " + tree.main +
					" --branch <branch>`, then cd into the new worktree.",
			},
			[]string{"outer", "git -C"},
		},
		{
			"a checkout under the shorter path only", cmds, EventWrite,
			Input{Event: EventWrite, FilePath: filepath.Join(tree.store, "fact.md")},
			[]string{"`outer store`, then edit fact.md in the new worktree."},
			nil,
		},
		{
			"a path reached through a link",
			[]config.WorktreeCommand{
				{Path: filepath.Join(home, "linked-src"), Command: "mkwt {repo}"},
			},
			EventWrite,
			Input{Event: EventWrite, FilePath: filepath.Join(tree.main, "README.md")},
			[]string{"`mkwt repo`"},
			nil,
		},
		{
			"a checkout under no path",
			[]config.WorktreeCommand{
				{Path: filepath.Join(home, "elsewhere"), Command: "mkwt {repo}"},
			},
			EventWrite,
			Input{Event: EventWrite, FilePath: filepath.Join(tree.main, "README.md")},
			[]string{
				"git -C " + tree.main + " worktree add ~/.worktrees/repo/<slug> -b <branch>",
				"then edit ~/.worktrees/repo/<slug>/README.md",
			},
			[]string{"mkwt", "in the new worktree"},
		},
		{
			"a git command under no path", nil, EventBash,
			bashCall("git commit -m x", tree.main),
			[]string{"then cd ~/.worktrees/repo/<slug>."},
			[]string{"into the new worktree"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toggle := config.Toggle{Mode: config.ModeDeny, WorktreeCommands: tt.cmds}
			d := mainCheckoutGuard(tt.event, toggle, "main").Check(tt.in)
			wantDenial(t, d, tt.want)
			for _, n := range tt.not {
				if strings.Contains(d.Reason, n) {
					t.Errorf("reason %q holds %q", d.Reason, n)
				}
			}
		})
	}
}

// TestMainCheckoutHintMode: with no mode set the guard lets the call
// through and gives the reason as advice, saying the call ran; RunAdvised
// reports that pass as not clean, so no allow acts on it.
func TestMainCheckoutHintMode(t *testing.T) {
	tree := newMainCheckoutTree(t)
	g := mainCheckoutGuard(EventWrite, config.Toggle{}, "main")
	file := filepath.Join(tree.main, "README.md")
	d, advice := Verdict(g, Input{Event: EventWrite, FilePath: file, Content: "x"})
	if d != nil {
		t.Fatalf("hint mode denied: %s", d.Reason)
	}
	if len(advice) != 1 {
		t.Fatalf("advice = %+v, want one piece", advice)
	}
	text := advice[0].Text
	for _, want := range []string{
		"belt[main-checkout]: " + file + " is in the main checkout " + tree.main,
		"worktree add ~/.worktrees/repo/<slug> -b <branch>",
		"belt let this call through (main-checkout is in hint mode)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("advice %q does not hold %q", text, want)
		}
	}
	if advice[0].Guard != MainCheckoutID {
		t.Errorf("advice guard = %q, want %q", advice[0].Guard, MainCheckoutID)
	}

	cfg := config.Config{}
	in := Input{Event: EventWrite, FilePath: file, Content: "x"}
	d, clean, advice := RunAdvised(in, cfg)
	if d != nil || clean || len(advice) != 1 {
		t.Errorf("RunAdvised = %+v, clean %v, advice %+v; want no denial, not clean, one advice",
			d, clean, advice)
	}
	if _, advice := Verdict(g, Input{Event: EventWrite, FilePath: filepath.Join(tree.worktree, "README.md")}); advice != nil {
		t.Errorf("a worktree file got advice %+v", advice)
	}
}

// TestMainCheckoutAgreesWithGit runs the guard on a checkout and a linked
// worktree git itself made, so the file read of the layout is checked
// against the real thing, not only the hand-laid fixtures.
func TestMainCheckoutAgreesWithGit(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := initRepo(t, filepath.Join(dir, "repo"), "git@github.com:you/repo.git")
	wt := filepath.Join(dir, "wt", "repo", "feat")
	gitIn(t, main, "worktree", "add", "-q", wt, "-b", "feat")
	g := NewMainCheckout(
		config.Config{Guards: map[string]config.Toggle{MainCheckoutID: denyMode}},
		EventWrite,
	)
	if d := g.Check(Input{Event: EventWrite, FilePath: filepath.Join(main, "x.go")}); d == nil {
		t.Error("a write in the main checkout git made was allowed")
	}
	if d := g.Check(Input{Event: EventWrite, FilePath: filepath.Join(wt, "x.go")}); d != nil {
		t.Errorf("a write in the linked worktree git made was denied: %s", d.Reason)
	}
	b := NewMainCheckout(
		config.Config{Guards: map[string]config.Toggle{MainCheckoutID: denyMode}},
		EventBash,
	)
	if d := b.Check(bashCall("git pull --ff-only", main)); d != nil {
		t.Errorf("a fast-forward pull on main was denied: %s", d.Reason)
	}
	if d := b.Check(bashCall("git commit -m x", main)); d == nil {
		t.Error("a commit in the main checkout git made was allowed")
	}
	if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
		t.Fatal(err)
	}
}

// wantDenial checks d against the substrings its reason must hold; want nil
// means the call must be allowed.
func wantDenial(t *testing.T, d *Denial, want []string) {
	t.Helper()
	if want == nil {
		if d != nil {
			t.Errorf("denied, want allowed: %s", d.Reason)
		}
		return
	}
	if d == nil {
		t.Fatalf("allowed, want a denial holding %q", want)
	}
	for _, w := range want {
		if !strings.Contains(d.Reason, w) {
			t.Errorf("reason %q does not hold %q", d.Reason, w)
		}
	}
}
