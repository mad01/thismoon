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

// gitConfigClone is the config git writes for a clone on macOS. Every repo
// the tests lay out gets it unless a row says otherwise.
const gitConfigClone = "[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n" +
	"\tbare = false\n\tlogallrefupdates = true\n\tignorecase = true\n\tprecomposeunicode = true\n" +
	"[remote \"origin\"]\n\turl = https://example.com/x.git\n" +
	"\tfetch = +refs/heads/*:refs/remotes/origin/*\n" +
	"[branch \"main\"]\n\tremote = origin\n\tmerge = refs/heads/main\n"

// writeFiles creates each file with its content, parent directories first.
func writeFiles(t *testing.T, files map[string]string) {
	t.Helper()
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// mkdirs creates each directory and its parents.
func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// symlink creates link pointing at target.
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// mkGitDir lays out what git checks before it accepts a directory as a git
// directory, the way git init leaves it: HEAD naming a branch, objects/ and
// refs/, and the given config. No git runs.
func mkGitDir(t *testing.T, dir, cfg string) {
	t.Helper()
	writeFiles(t, map[string]string{
		filepath.Join(dir, "HEAD"):                     "ref: refs/heads/main\n",
		filepath.Join(dir, "config"):                   cfg,
		filepath.Join(dir, "objects", "info", ".keep"): "",
		filepath.Join(dir, "refs", "heads", ".keep"):   "",
	})
}

// mkRepo lays out a normal repo at top: a work tree with a .git directory.
func mkRepo(t *testing.T, top string) {
	t.Helper()
	mkGitDir(t, filepath.Join(top, ".git"), gitConfigClone)
}

// mkWorktree lays out a linked worktree of the repo at main, as git
// worktree add does: wt/.git is a file naming main/.git/worktrees/<name>,
// which holds HEAD, a commondir back to main/.git and a gitdir back to
// wt/.git.
func mkWorktree(t *testing.T, main, wt, name string) {
	t.Helper()
	gd := filepath.Join(main, ".git", "worktrees", name)
	writeFiles(t, map[string]string{
		filepath.Join(gd, "HEAD"):      "ref: refs/heads/" + name + "\n",
		filepath.Join(gd, "commondir"): "../..\n",
		filepath.Join(gd, "gitdir"):    filepath.Join(wt, ".git") + "\n",
		filepath.Join(wt, ".git"):      "gitdir: " + gd + "\n",
	})
}

// bashCall is the guard input for a Bash tool call the way the hook builds
// it: the tool name and its input beside the command.
func bashCall(command, cwd string) Input {
	return Input{
		Event:     EventBash,
		Command:   command,
		Cwd:       cwd,
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": command},
	}
}

// gitCReadTree is a hermetic home for the git-c-read tests. The configured
// root ~/code/src holds a repo x (with a sub directory), a directory with
// no repo, and a symlink to a repo outside home. Inside x, the symlink out
// points into that outside repo, so out/.. is x when read lexically and
// the outside repo when resolved the way git -C does. The session runs in
// its own repo, ~/session, outside the root. No git runs: the repos are
// laid out by hand in the shape git leaves them.
type gitCReadTree struct {
	home, root, repo, session, outside string
}

func newGitCReadTree(t *testing.T) gitCReadTree {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, k := range gitEnvRedirects {
		t.Setenv(k, "")
	}
	outside := t.TempDir()
	root := filepath.Join(home, "code", "src")
	repo := filepath.Join(root, "x")
	session := filepath.Join(home, "session")
	for _, top := range []string{outside, repo, session} {
		mkRepo(t, top)
	}
	mkdirs(t,
		filepath.Join(outside, "inner"),
		filepath.Join(repo, "sub"),
		filepath.Join(root, "plain"),
		filepath.Join(session, "sub"),
	)
	symlink(t, outside, filepath.Join(root, "escape"))
	symlink(t, filepath.Join(outside, "inner"), filepath.Join(repo, "out"))
	symlink(t, root, filepath.Join(home, "src-link"))
	return gitCReadTree{home: home, root: root, repo: repo, session: session, outside: outside}
}

// gitCReadOn is a config with the allow on and ~/code/src as its root.
func gitCReadOn(roots ...string) config.Config {
	if roots == nil {
		roots = []string{"~/code/src"}
	}
	return config.Config{Allows: map[string]config.Allow{
		GitCReadID: {Enabled: true, Roots: roots},
	}}
}

// TestGitCReadApproves pins what the allow answers, run from the session
// repo. {repo} and {outside} stand for the absolute paths.
func TestGitCReadApproves(t *testing.T) {
	tree := newGitCReadTree(t)
	tests := []struct {
		name, command string
		want          bool
	}{
		// What passes.
		{"status in a repo inside the root", "git -C ~/code/src/x status", true},
		{"absolute dir", "git -C {repo} diff HEAD~1..HEAD --stat", true},
		{"quoted absolute dir", "git -C '{repo}' status --short", true},
		{"subdirectory of a repo", "git -C ~/code/src/x/sub log --oneline -5", true},
		{"relative dir in the session repo", "git -C sub status", true},
		{"the session repo itself", "git -C . rev-parse --show-toplevel", true},
		{
			"two safe commands chained",
			"git -C ~/code/src/x status && git -C ~/code/src/x log --oneline -3",
			true,
		},
		{"semicolon and newline joins", "git -C sub status; git -C sub diff\ngit -C . show", true},
		{"or join", "git -C sub status || git -C . status", true},
		{"a quoted ; inside an arg", "git -C ~/code/src/x log --grep='a;b'", true},
		{"a quoted | inside an arg", `git -C ~/code/src/x log --grep="fix: a | b"`, true},
		{"quoted glob is git's pathspec", "git -C ~/code/src/x ls-files -- '*.go'", true},
		{"tilde after a revision", "git -C ~/code/src/x show HEAD~1", true},
		{"negated program options", "git -C ~/code/src/x diff --no-ext-diff --no-textconv", true},
		{"trailing newline", "git -C ~/code/src/x status\n", true},
		{"bare branch", "git -C ~/code/src/x branch", true},
		{"branch listing flags", "git -C ~/code/src/x branch -a -vv", true},
		{"branch pattern with --list", "git -C ~/code/src/x branch --list 'feat*'", true},
		{"branch --show-current", "git -C ~/code/src/x branch --show-current", true},
		{"bare remote", "git -C ~/code/src/x remote", true},
		{"remote -v", "git -C ~/code/src/x remote -v", true},
		{"worktree list", "git -C ~/code/src/x worktree list --porcelain", true},
		{"dir through a symlinked root", "git -C ~/src-link/x status", true},
		{"quoted path after an unquoted ~/", "git -C ~/'code/src/x' status", true},
		{"long option with a relative value", "git -C ~/code/src/x diff --relative=sub", true},

		// The command shape.
		{"-c before the subcommand", "git -C ~/code/src/x -c core.fsmonitor=y status", false},
		{"-c on a relative dir", "git -C x -c core.fsmonitor=y status", false},
		{"a second -C", "git -C ~/code/src/x -C sub status", false},
		{"--git-dir first", "git --git-dir=x -C ~/code/src/x status", false},
		{"global option after -C", "git -C ~/code/src/x --no-pager status", false},
		{"no -C", "git status", false},
		{"empty command", "", false},
		{"quoted git", `"git" -C ~/code/src/x status`, false},
		{"env assignment prefix", "FOO=1 git -C ~/code/src/x status", false},
		{"sudo", "sudo git -C ~/code/src/x status", false},
		{"command", "command git -C ~/code/src/x status", false},
		{"env", "env git -C ~/code/src/x status", false},
		{"time", "time git -C ~/code/src/x status", false},

		// Shell syntax.
		{"chained with curl", "git -C ~/code/src/x status && curl x", false},
		{"a ; outside the quotes", "git -C ~/code/src/x log --grep='a';curl x", false},
		{"pipe into sh", "git -C ~/code/src/x status | sh", false},
		{"redirect", "git -C ~/code/src/x log > file", false},
		{"stderr redirect", "git -C ~/code/src/x status 2>/dev/null", false},
		{"background", "git -C ~/code/src/x status &", false},
		{"trailing operator", "git -C ~/code/src/x status;", false},
		{"leading operator", "&& git -C ~/code/src/x status", false},
		{"command substitution", "git -C $(pwd) status", false},
		{"backticks", "git -C `pwd` status", false},
		{"variable", "git -C $S status", false},
		{"variable in double quotes", `git -C ~/code/src/x log "$HOME"`, false},
		{"unterminated quote", "git -C ~/code/src/x log 'oops", false},
		{"subshell", "(git -C ~/code/src/x status)", false},
		{"group", "{ git -C ~/code/src/x status; }", false},
		{"comment", "git -C ~/code/src/x status # note", false},
		{"line continuation", "git -C ~/code/src/x \\\nstatus", false},
		{"heredoc", "git -C ~/code/src/x log <<EOF\nx\nEOF", false},
		{"process substitution", "git -C ~/code/src/x diff <(echo a)", false},
		{"glob", "git -C ~/code/src/x log *.go", false},
		{"caret glob in zsh", "git -C ~/code/src/x log HEAD^", false},
		{"brace expansion", "git -C ~/code/src/x log {a,b}", false},
		{"tilde after =", "git -C ~/code/src/x log a=~/b", false},
		{"non-ASCII byte", "git -C ~/code/src/x status\u00a0", false},
		// Review finding 5: zsh expands a ~ whose following / is quoted,
		// bash leaves the word alone, so the dir differs between shells.
		{"quoted slash after ~ (review finding 5)", "git -C ~'/code/src/x' status", false},
		{"double-quoted slash after ~", `git -C ~"/code/src/x" status`, false},
		{"empty quotes after ~", "git -C ~''/code/src/x status", false},

		// The directory.
		{"dir outside the roots", "git -C {outside} status", false},
		{"symlink that leaves the root", "git -C ~/code/src/escape status", false},
		{"symlink inside a repo that leaves the root", "git -C ~/code/src/x/out status", false},
		{"symlink then .. resolves physically", "git -C ~/code/src/x/out/.. status", false},
		{"dir with no work tree", "git -C ~/code/src/plain status", false},
		{"dir that does not exist", "git -C ~/code/src/missing status", false},
		{"~user", "git -C ~other/x status", false},
		{"~+", "git -C ~+/x status", false},
		{"quoted tilde is literal", `git -C "~/code/src/x" status`, false},
		{"zsh =cmd", "git -C =git status", false},

		// The subcommand and its arguments.
		{"push", "git -C ~/code/src/x push", false},
		{"fetch", "git -C ~/code/src/x fetch", false},
		{"checkout", "git -C ~/code/src/x checkout main", false},
		{"commit", "git -C ~/code/src/x commit -m x", false},
		{"diff --output=", "git -C ~/code/src/x diff --output=/tmp/f", false},
		{"diff --output file", "git -C ~/code/src/x diff --output f", false},
		{"diff --ext-diff", "git -C ~/code/src/x diff --ext-diff", false},
		{"log --textconv", "git -C ~/code/src/x log -p --textconv", false},
		{"diff --no-index", "git -C ~/code/src/x diff --no-index a b", false},
		{"diff -O", "git -C ~/code/src/x diff -Oorder", false},
		{"diff -O bundled", "git -C ~/code/src/x diff -pOorder", false},
		{"--exec", "git -C ~/code/src/x log --exec=x", false},
		{"--upload-pack", "git -C ~/code/src/x log --upload-pack=x", false},
		{"--help", "git -C ~/code/src/x status --help", false},
		{"abbreviated --help", "git -C ~/code/src/x status --hel", false},
		{"--show-signature", "git -C ~/code/src/x log --show-signature", false},
		{"signature placeholder", "git -C ~/code/src/x log --format=%G?", false},
		{"absolute path argument", "git -C ~/code/src/x diff /etc/passwd README", false},
		{"climbing path argument", "git -C ~/code/src/x diff ../../.ssh/id x", false},
		{"branch -D", "git -C ~/code/src/x branch -D y", false},
		{"branch creates", "git -C ~/code/src/x branch newname", false},
		{"branch abbreviated --delete", "git -C ~/code/src/x branch --del y", false},
		{"branch --set-upstream-to", "git -C ~/code/src/x branch --set-upstream-to=o/m", false},
		{"branch -m", "git -C ~/code/src/x branch -m a b", false},
		{"remote add", "git -C ~/code/src/x remote add o u", false},
		{"remote show contacts the remote", "git -C ~/code/src/x remote show origin", false},
		{"worktree add", "git -C ~/code/src/x worktree add y", false},
		{"bare worktree", "git -C ~/code/src/x worktree", false},
		// Review finding 7: options that read a file named by their value.
		{
			"ls-files --exclude-from=",
			"git -C ~/code/src/x ls-files --exclude-from=/etc/hosts",
			false,
		},
		{"ls-files -X attached", "git -C ~/code/src/x ls-files -X/etc/hosts", false},
		{
			"rev-parse --resolve-git-dir=",
			"git -C ~/code/src/x rev-parse --resolve-git-dir=/etc",
			false,
		},
		{"long option value outside the repo", "git -C ~/code/src/x diff --relative=../y", false},
	}
	repl := strings.NewReplacer("{repo}", tree.repo, "{outside}", tree.outside)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := bashCall(repl.Replace(tt.command), tree.session)
			got := Approve(in, gitCReadOn())
			if (got != nil) != tt.want {
				t.Errorf("Approve(%q) = %+v, want an answer: %v", in.Command, got, tt.want)
			}
			if got != nil && !strings.HasPrefix(got.Reason, "belt[git-c-read]: ") {
				t.Errorf("reason = %q, want the belt[git-c-read] prefix", got.Reason)
			}
		})
	}
}

// TestGitCReadToolInput: the allow answers only a Bash tool call that runs
// in the sandbox in the foreground (review finding 4). A matcher widened to
// another tool, or a Bash call that switches the sandbox off, runs in the
// background, or carries an input key belt does not know, gets no answer.
func TestGitCReadToolInput(t *testing.T) {
	tree := newGitCReadTree(t)
	const cmd = "git -C ~/code/src/x status"
	tests := []struct {
		name  string
		tool  string
		input map[string]any
		want  bool
	}{
		{"plain Bash call", "Bash", map[string]any{"command": cmd}, true},
		{
			"description and timeout",
			"Bash",
			map[string]any{"command": cmd, "description": "status", "timeout": 1000.0},
			true,
		},
		{
			"switches set to false",
			"Bash",
			map[string]any{
				"command":                   cmd,
				"dangerouslyDisableSandbox": false,
				"run_in_background":         false,
			},
			true,
		},
		{"another tool with a command field", "Monitor", map[string]any{"command": cmd}, false},
		{"no tool name", "", map[string]any{"command": cmd}, false},
		{"lowercase bash", "bash", map[string]any{"command": cmd}, false},
		{
			"sandbox switched off",
			"Bash",
			map[string]any{"command": cmd, "dangerouslyDisableSandbox": true},
			false,
		},
		{
			"sandbox switch that is not a bool",
			"Bash",
			map[string]any{"command": cmd, "dangerouslyDisableSandbox": "false"},
			false,
		},
		{
			"background run",
			"Bash",
			map[string]any{"command": cmd, "run_in_background": true},
			false,
		},
		{"unknown input key", "Bash", map[string]any{"command": cmd, "shell": "/bin/sh"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := bashCall(cmd, tree.session)
			in.ToolName, in.ToolInput = tt.tool, tt.input
			if got := Approve(in, gitCReadOn()); (got != nil) != tt.want {
				t.Errorf(
					"Approve(%s %v) = %+v, want an answer: %v",
					tt.tool,
					tt.input,
					got,
					tt.want,
				)
			}
		})
	}
}

// evilGitDir is review finding 1: a directory a clone can carry as tracked
// files that git accepts as a git directory, config and all.
func evilGitDir(t *testing.T, dir string) {
	t.Helper()
	writeFiles(t, map[string]string{
		filepath.Join(dir, "HEAD"): "ref: refs/heads/main\n",
		filepath.Join(dir, "config"): "[core]\n\tbare = false\n\tworktree = ..\n" +
			"\tfsmonitor = \"touch pwned; false\"\n",
		filepath.Join(dir, "refs", "heads", ".keep"):   "",
		filepath.Join(dir, "objects", "info", ".keep"): "",
	})
}

// repoWithConfig lays out a normal repo whose .git/config is cfg.
func repoWithConfig(cfg string) func(*testing.T, string) {
	return func(t *testing.T, d string) {
		t.Helper()
		mkGitDir(t, filepath.Join(d, ".git"), cfg)
	}
}

// TestGitCReadRepoDiscovery pins the repo the allow vouches for: the one
// git itself would find from the -C dir. Each row lays out its own tree in
// a fresh directory inside the root, and the call is git -C <dir>/<rel>
// status. A row that wants no answer is a layout where git would read a
// git directory or config the allow cannot vouch for.
func TestGitCReadRepoDiscovery(t *testing.T) {
	tree := newGitCReadTree(t)
	tests := []struct {
		name  string
		build func(t *testing.T, d string)
		rel   string
		want  bool
	}{
		{"a normal repo", func(t *testing.T, d string) { mkRepo(t, d) }, ".", true},
		{
			"a directory inside a normal repo",
			func(t *testing.T, d string) { mkRepo(t, d); mkdirs(t, filepath.Join(d, "a", "b")) },
			"a/b",
			true,
		},
		{
			"a repo nested in a repo answers for the inner one",
			func(t *testing.T, d string) { mkRepo(t, d); mkRepo(t, filepath.Join(d, "inner")) },
			"inner",
			true,
		},
		{
			"config values quoted and commented",
			repoWithConfig("[core]\n\tbare = \"false\" ; set by hand\n\tworktree-ish = 1\n"),
			".",
			true,
		},

		// Review finding 1: an embedded git directory in a clone.
		{
			"embedded git directory (review finding 1)",
			func(t *testing.T, d string) { mkRepo(t, d); evilGitDir(t, filepath.Join(d, "evil.git")) },
			"evil.git",
			false,
		},
		{
			"inside an embedded git directory",
			func(t *testing.T, d string) { mkRepo(t, d); evilGitDir(t, filepath.Join(d, "evil.git")) },
			"evil.git/refs/heads",
			false,
		},
		{
			"HEAD and a commondir make a git directory",
			func(t *testing.T, d string) {
				mkRepo(t, d)
				writeFiles(t, map[string]string{
					filepath.Join(d, "fake", "HEAD"):      "ref: refs/heads/main\n",
					filepath.Join(d, "fake", "commondir"): tree.outside + "/.git\n",
				})
			},
			"fake",
			false,
		},
		{
			"the repo's own .git directory",
			func(t *testing.T, d string) { mkRepo(t, d) },
			".git",
			false,
		},

		// Review finding 2B and other .git entries git follows elsewhere.
		{
			"gitfile naming a repo outside the roots (review finding 2B)",
			func(t *testing.T, d string) {
				secret := filepath.Join(tree.outside, "secret")
				mkRepo(t, secret)
				writeFiles(t, map[string]string{
					filepath.Join(d, ".git"): "gitdir: " + filepath.Join(secret, ".git") + "\n",
				})
			},
			".",
			false,
		},
		{
			"gitfile naming a repo inside the root",
			func(t *testing.T, d string) {
				writeFiles(t, map[string]string{
					filepath.Join(d, ".git"): "gitdir: " + filepath.Join(tree.repo, ".git") + "\n",
				})
			},
			".",
			false,
		},
		{
			".git symlink to a repo inside the root",
			func(t *testing.T, d string) {
				mkdirs(t, d)
				symlink(t, filepath.Join(tree.repo, ".git"), filepath.Join(d, ".git"))
			},
			".",
			false,
		},
		{
			"submodule gitfile",
			func(t *testing.T, d string) {
				mkRepo(t, d)
				mkGitDir(t, filepath.Join(d, ".git", "modules", "mod"),
					"[core]\n\tbare = false\n\tworktree = ../../../mod\n")
				writeFiles(t, map[string]string{
					filepath.Join(d, "mod", ".git"): "gitdir: ../.git/modules/mod\n",
				})
			},
			"mod",
			false,
		},
		{
			"empty .git directory, which git walks past",
			func(t *testing.T, d string) { mkdirs(t, filepath.Join(d, ".git"), filepath.Join(d, "a")) },
			"a",
			false,
		},
		{
			".git whose HEAD git rejects",
			func(t *testing.T, d string) {
				mkRepo(t, d)
				writeFiles(t, map[string]string{filepath.Join(d, ".git", "HEAD"): "main\n"})
			},
			".",
			false,
		},
		{
			".git with a commondir of its own",
			func(t *testing.T, d string) {
				mkRepo(t, d)
				writeFiles(t, map[string]string{
					filepath.Join(d, ".git", "commondir"): tree.outside + "/.git\n",
				})
			},
			".",
			false,
		},
		{
			".git objects is a symlink",
			func(t *testing.T, d string) {
				mkRepo(t, d)
				objects := filepath.Join(d, ".git", "objects")
				if err := os.RemoveAll(objects); err != nil {
					t.Fatal(err)
				}
				symlink(t, filepath.Join(tree.outside, ".git", "objects"), objects)
			},
			".",
			false,
		},
		{
			"a work tree directory that looks like a git directory",
			func(t *testing.T, d string) {
				mkRepo(t, d)
				writeFiles(t, map[string]string{
					filepath.Join(d, "data", "HEAD"):   "notes\n",
					filepath.Join(d, "data", "config"): "x\n",
				})
			},
			"data",
			false,
		},

		// The repo's own config.
		{"core.worktree", repoWithConfig("[core]\n\tworktree = /elsewhere\n"), ".", false},
		{"core.bare true", repoWithConfig("[core]\n\tbare = true\n"), ".", false},
		{"core.bare with no value", repoWithConfig("[core]\n\tbare\n"), ".", false},
		{"core.hooksPath", repoWithConfig("[core]\n\thooksPath = .githooks\n"), ".", false},
		{"upper-case section and key", repoWithConfig("[CORE]\n\tWorkTree = ..\n"), ".", false},
		{"include", repoWithConfig("[include]\n\tpath = ../x.inc\n"), ".", false},
		{"includeIf", repoWithConfig("[includeIf \"gitdir:~/\"]\n\tpath = x.inc\n"), ".", false},
		{"core key on the header line", repoWithConfig("[core] worktree = /tmp\n"), ".", false},
		{
			"a line continuation that hides a section",
			repoWithConfig("[core]\n\tx = a\\\n[alias]\n\tworktree = ..\n"),
			".",
			false,
		},
		{"a malformed header", repoWithConfig("[core\n\tbare = false\n"), ".", false},
		{
			"config.worktree sets core.worktree",
			func(t *testing.T, d string) {
				mkRepo(t, d)
				writeFiles(t, map[string]string{
					filepath.Join(d, ".git", "config.worktree"): "[core]\n\tworktree = /elsewhere\n",
				})
			},
			".",
			false,
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := filepath.Join(tree.root, fmt.Sprintf("case%02d", i))
			tt.build(t, d)
			cmd := "git -C " + filepath.Join(d, tt.rel) + " status"
			if got := Approve(bashCall(cmd, tree.session), gitCReadOn()); (got != nil) != tt.want {
				t.Errorf("Approve(%q) = %+v, want an answer: %v", cmd, got, tt.want)
			}
		})
	}
}

// TestGitCReadWorktrees: a linked worktree is approved only when its own
// folder and its main repo's .git both sit inside a root. This machine
// keeps worktrees under ~/.worktrees and the main checkouts under
// ~/code/src, so both must be roots.
func TestGitCReadWorktrees(t *testing.T) {
	tree := newGitCReadTree(t)
	wts := filepath.Join(tree.home, ".worktrees")
	main := filepath.Join(tree.root, "main")
	mkRepo(t, main)
	mkWorktree(t, main, filepath.Join(wts, "main", "feat"), "feat")
	mkdirs(t, filepath.Join(wts, "main", "feat", "sub"))
	far := filepath.Join(tree.outside, "far")
	mkRepo(t, far)
	mkWorktree(t, far, filepath.Join(wts, "far", "wt"), "wt")
	stale := filepath.Join(wts, "main", "stale")
	mkWorktree(t, main, stale, "stale")
	writeFiles(t, map[string]string{
		filepath.Join(main, ".git", "worktrees", "stale", "gitdir"): "/moved/away/.git\n",
	})
	relative := filepath.Join(wts, "main", "rel")
	mkWorktree(t, main, relative, "rel")
	relGD, err := filepath.Rel(relative, filepath.Join(main, ".git", "worktrees", "rel"))
	if err != nil {
		t.Fatal(err)
	}
	relBack, err := filepath.Rel(
		filepath.Join(main, ".git", "worktrees", "rel"),
		filepath.Join(relative, ".git"),
	)
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, map[string]string{
		filepath.Join(relative, ".git"):                           "gitdir: " + relGD + "\n",
		filepath.Join(main, ".git", "worktrees", "rel", "gitdir"): relBack + "\n",
	})
	elsewhere := filepath.Join(wts, "main", "redirected")
	mkWorktree(t, main, elsewhere, "redirected")
	writeFiles(t, map[string]string{
		filepath.Join(main, ".git", "worktrees", "redirected", "commondir"): filepath.Join(
			far,
			".git",
		) + "\n",
	})
	both := []string{"~/code/src", "~/.worktrees"}
	tests := []struct {
		name  string
		roots []string
		cwd   string
		dir   string
		want  bool
	}{
		{"worktree and main repo under roots", both, tree.session, "~/.worktrees/main/feat", true},
		{"inside the worktree", both, tree.session, "~/.worktrees/main/feat/sub", true},
		{"worktree with relative paths", both, tree.session, "~/.worktrees/main/rel", true},
		{"main repo outside every root", both, tree.session, "~/.worktrees/far/wt", false},
		{
			"only the worktree folder is a root",
			[]string{"~/.worktrees"},
			tree.session,
			"~/.worktrees/main/feat",
			false,
		},
		{
			"only the main checkouts are a root",
			[]string{"~/code/src"},
			tree.session,
			"~/.worktrees/main/feat",
			false,
		},
		{
			"gitdir back-pointer names another folder",
			both,
			tree.session,
			"~/.worktrees/main/stale",
			false,
		},
		{"commondir names another repo", both, tree.session, "~/.worktrees/main/redirected", false},
		{
			"session in the worktree, main repo under no root",
			[]string{},
			filepath.Join(wts, "main", "feat"),
			".",
			false,
		},
		{
			"session in the worktree, main repo under a root",
			[]string{"~/code/src"},
			filepath.Join(wts, "main", "feat"),
			"sub",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := "git -C " + tt.dir + " status"
			if got := Approve(bashCall(cmd, tt.cwd), gitCReadOn(tt.roots...)); (got != nil) != tt.want {
				t.Errorf("Approve(%q from %q, roots %v) = %+v, want an answer: %v",
					cmd, tt.cwd, tt.roots, got, tt.want)
			}
		})
	}
}

// TestGitCReadIsOffByDefault: with no allows entry, or one that does not
// set enabled, nothing is ever approved.
func TestGitCReadIsOffByDefault(t *testing.T) {
	tree := newGitCReadTree(t)
	in := bashCall("git -C ~/code/src/x status", tree.session)
	for name, cfg := range map[string]config.Config{
		"no config": {},
		"roots but not enabled": {Allows: map[string]config.Allow{
			GitCReadID: {Roots: []string{"~/code/src"}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Approve(in, cfg); got != nil {
				t.Errorf("Approve = %+v, want nil while the allow is off", got)
			}
		})
	}
}

// TestGitCReadRoots: the session repo is always a root, a relative roots
// entry is dropped, and a root reached through a symlink still counts. A
// root that covers home is dropped, and so is a repo the root sits in
// (review findings 2A and 6).
func TestGitCReadRoots(t *testing.T) {
	tree := newGitCReadTree(t)
	outer := filepath.Join(tree.root, "outer")
	mkRepo(t, outer)
	mkdirs(t, filepath.Join(outer, "plain"))
	tests := []struct {
		name    string
		roots   []string
		cwd     string
		command string
		want    bool
	}{
		{
			"no roots still covers the session repo",
			[]string{},
			tree.session,
			"git -C sub status",
			true,
		},
		{
			"no roots covers nothing else",
			[]string{},
			tree.session,
			"git -C ~/code/src/x status",
			false,
		},
		{
			"relative entry is dropped",
			[]string{"code/src"},
			tree.session,
			"git -C ~/code/src/x status",
			false,
		},
		{
			"symlinked root",
			[]string{"~/src-link"},
			tree.session,
			"git -C ~/code/src/x status",
			true,
		},
		{"cwd outside any repo", []string{}, tree.home, "git -C session status", false},
		{"relative dir with no cwd", nil, "", "git -C x status", false},
		{
			"a root inside a repo reads the repo above it (review finding 2A)",
			[]string{"~/code/src/outer/plain"},
			tree.session,
			"git -C ~/code/src/outer/plain log",
			false,
		},
		{
			"root / is dropped (review finding 6)",
			[]string{"/"},
			tree.session,
			"git -C ~/code/src/x status",
			false,
		},
		{"root ~ is dropped", []string{"~"}, tree.session, "git -C ~/code/src/x status", false},
		{
			"root ~/.. is dropped",
			[]string{"~/.."},
			tree.session,
			"git -C ~/code/src/x status",
			false,
		},
		{
			"root that climbs back into home is dropped",
			[]string{"~/code/../.."},
			tree.session,
			"git -C ~/code/src/x status",
			false,
		},
		{
			// ~/src-link/.. is ~/code physically and home when cleaned
			// lexically, which would drop it.
			"root through a symlink then .. resolves physically",
			[]string{"~/src-link/.."},
			tree.session,
			"git -C ~/code/src/x status",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := bashCall(tt.command, tt.cwd)
			if got := Approve(in, gitCReadOn(tt.roots...)); (got != nil) != tt.want {
				t.Errorf(
					"Approve(%q from %q) = %+v, want an answer: %v",
					tt.command,
					tt.cwd,
					got,
					tt.want,
				)
			}
		})
	}
}

// TestGitCReadHomeIsARepo: when home itself is a work tree, the session's
// repo for a cwd in home is home, which would make every repo under home a
// root, and a root's plain directory reads home's repo (review findings 2A
// and 6). Neither is approved.
func TestGitCReadHomeIsARepo(t *testing.T) {
	tree := newGitCReadTree(t)
	mkRepo(t, tree.home)
	tests := []struct {
		name, cwd, command string
		roots              []string
	}{
		{"session in home", tree.home, "git -C code/src/x status", []string{}},
		{"plain dir under a root reads home", tree.session, "git -C ~/code/src/plain status", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Approve(bashCall(tt.command, tt.cwd), gitCReadOn(tt.roots...)); got != nil {
				t.Errorf("Approve(%q from %q) = %+v, want no answer", tt.command, tt.cwd, got)
			}
		})
	}
}

// TestGitCReadGitEnvironment: a GIT_DIR or similar in the hook's
// environment makes git read another repo than discovery finds, so the
// allow stays silent while one is set.
func TestGitCReadGitEnvironment(t *testing.T) {
	tree := newGitCReadTree(t)
	in := bashCall("git -C ~/code/src/x status", tree.session)
	for _, k := range gitEnvRedirects {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, filepath.Join(tree.outside, ".git"))
			if got := Approve(in, gitCReadOn()); got != nil {
				t.Errorf("Approve with %s set = %+v, want no answer", k, got)
			}
		})
	}
	if got := Approve(in, gitCReadOn()); got == nil {
		t.Error("Approve with no redirect set = nil, want an answer")
	}
}

// TestGitCReadRealGitLayouts checks the allow against repos git itself
// made, so the hand-built layouts above cannot drift from git's: a clone's
// main checkout and a linked worktree under the roots are approved, and a
// worktree whose main checkout is under no root is not. It needs git on
// PATH and runs it with no system or global config.
func TestGitCReadRealGitLayouts(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	tree := newGitCReadTree(t)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	// The tree empties the redirect variables; git reads an empty GIT_DIR
	// as set, so they leave its environment entirely.
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !slices.Contains(gitEnvRedirects, k) {
			env = append(env, kv)
		}
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command(gitBin, args...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	main := filepath.Join(tree.root, "real")
	git("init", "-q", "-b", "main", main)
	git("-C", main, "-c", "user.name=t", "-c", "user.email=t@example.com",
		"commit", "-q", "--allow-empty", "-m", "first")
	git("-C", main, "worktree", "add", "-q", "-b", "feat",
		filepath.Join(tree.home, ".worktrees", "real", "feat"))
	far := filepath.Join(tree.outside, "far")
	git("init", "-q", "-b", "main", far)
	git("-C", far, "-c", "user.name=t", "-c", "user.email=t@example.com",
		"commit", "-q", "--allow-empty", "-m", "first")
	git("-C", far, "worktree", "add", "-q", "-b", "feat",
		filepath.Join(tree.home, ".worktrees", "far", "feat"))
	roots := []string{"~/code/src", "~/.worktrees"}
	for _, tt := range []struct {
		dir  string
		want bool
	}{
		{"~/code/src/real", true},
		{"~/.worktrees/real/feat", true},
		{"~/.worktrees/far/feat", false},
	} {
		cmd := "git -C " + tt.dir + " status"
		if got := Approve(bashCall(cmd, tree.session), gitCReadOn(roots...)); (got != nil) != tt.want {
			t.Errorf("Approve(%q) = %+v, want an answer: %v", cmd, got, tt.want)
		}
	}
}

// TestPlainSegmentsWords pins the reader's word building: adjacent quoted
// and unquoted parts join into one word, and quotes mark it.
func TestPlainSegmentsWords(t *testing.T) {
	segs, ok := plainSegments(`git -C "a b"'c'd log; git -C ~/x status`)
	if !ok || len(segs) != 2 {
		t.Fatalf("plainSegments = %v, %v; want two commands", segs, ok)
	}
	if w := segs[0][2]; w.text != "a bcd" || !w.quoted || w.tilde {
		t.Errorf("dir word = %+v, want one quoted word a bcd", w)
	}
	if w := segs[1][2]; w.text != "~/x" || w.quoted || !w.tilde {
		t.Errorf("dir word = %+v, want an unquoted ~/x marked tilde", w)
	}
}
