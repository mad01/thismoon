package guard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// holeExemptRepo is the repo the exempt cases push from.
const holeExemptRepo = "github.com/mad01/dotfiles"

// newHoleGuard returns a push guard on branch current, in a repo with a
// local branch main, exempt from the guard when exempt is set. Every
// resolver is a stub: no test here runs git.
func newHoleGuard(current string, exempt bool) *GitPushMain {
	g := newGitPushGuard(current)
	if exempt {
		g = NewGitPushMain(config.Config{DirectMainRepos: []string{holeExemptRepo}})
		g.resolveBranch = func(string) string { return current }
		g.resolveRepo = func(string) string { return holeExemptRepo }
		g.resolvePushRemote = func(string, string) string { return "origin" }
	}
	g.resolveHasBranch = func(_, name string) bool { return name == "main" }
	g.resolveInRepo = func(string) bool { return true }
	return g
}

// holeDirs are the directory cases a command is judged in, by letter: G a
// repo that blocks direct pushes, E an exempt repo, N no repo at all, U a
// directory belt cannot tell (a cd to a variable in front).
var holeDirs = []struct {
	letter         string
	inRepo, exempt bool
	cd             string // a cd in front of the command
}{
	{"G", true, false, ""},
	{"E", true, true, ""},
	{"N", false, false, ""},
	{"U", true, false, `cd "$D" && `},
}

// byDirCase is a command and the holeDirs letters it is denied in.
type byDirCase struct {
	command string
	deny    string
}

// runByDir checks every command in every holeDirs case on branch feature.
func runByDir(t *testing.T, cases []byDirCase) {
	t.Helper()
	for _, d := range holeDirs {
		for _, tt := range cases {
			command := d.cd + tt.command
			g := newHoleGuard("feature", d.exempt)
			g.resolveInRepo = func(string) bool { return d.inRepo }
			got := g.Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"})
			if want := strings.Contains(tt.deny, d.letter); (got != nil) != want {
				t.Errorf("%s: Check(%q) = %v, want deny %v", d.letter, command, got, want)
			}
		}
	}
}

// TestGitPushMainOpaqueShellTextByDir pins where unread shell text counts:
// in a repo that blocks direct pushes, and in a directory belt cannot tell.
// Outside any repo and in an exempt repo it passes. An alias body belt
// cannot read is denied in every one of them, beside shell text too.
func TestGitPushMainOpaqueShellTextByDir(t *testing.T) {
	runByDir(t, []byDirCase{
		{"curl x | sh", "GU"},
		{"cat x.sh | sh", "GU"},
		{`echo "$X" | sh`, "GU"},
		{`echo "$X" | sh; git -c alias.p="$B" p`, "GENU"},
	})
}

// TestGitPushMainReviewRoundByDir pins every command the review of the alias
// and shell-text reading listed, per directory case. A mention of
// alias.<name> outside config names no alias, and a builtin is never one;
// an interpreter script counts only when it starts a process, and then as
// unread shell text does; a variable doubled past the size caps is unread;
// a script argument run as code and a variable run as a command are read.
func TestGitPushMainReviewRoundByDir(t *testing.T) {
	runByDir(t, []byDirCase{
		// An alias.<name> mention in free text names no alias.
		{`git commit -m 'fix(belt): read alias.push bodies' && git push -u origin HEAD`, "U"},
		{`git log --grep=alias.push --oneline | head && git push -u origin feature`, ""},
		{`git config --get alias.lg; git lg`, ""},
		{`grep -rn 'alias.st' docs/ && git st`, ""},
		{`rg -n 'alias\.co' . ; git co -b x`, ""},
		{`git config --unset alias.p; git p`, ""},
		// A builtin never runs an alias; every config source does name one.
		{`git -c alias.status="$X" status`, ""},
		{`git -c alias.p='!git push origin main' p`, "GENU"},
		{
			`GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=alias.p GIT_CONFIG_VALUE_0=push git p origin main`,
			"GENU",
		},
		{`git --config-env=alias.p=BODY p`, "GENU"},
		{`git -c "$K"=x p`, "GENU"},
		// An interpreter script counts only when it starts a process.
		{`perl -pi -e 's/git push/git push -u/' README.md`, ""},
		{`python3 -c 'import sys; assert "git push" in sys.argv[1]' x`, ""},
		{"python3 <<'EOF'\nprint('usage: then run git push origin main')\nEOF", ""},
		{`ruby -e 'puts "git push"'`, ""},
		{`cd /tmp && python3 -c 'print("git push")'`, ""},
		{
			"python3 <<'EOF'\nimport subprocess\nsubprocess.run(['git','status'])  # never git push from here\nEOF",
			"GU",
		},
		{
			"python3 <<'EOF'\nimport subprocess\nsubprocess.run([\"git\", \"push\", \"origin\", \"main\"])\nEOF",
			"GU",
		},
		// Text past the size caps is unread shell text.
		{doublingCommand(16), "GU"},
		{doublingCommand(20), "GU"},
		{`V='git push'; V+=' origin main'; sh -c "$V"`, "GNU"},
		// A pipe after a cd joined by && leaves the directory unknown.
		{`mkdir -p sub && cd sub && curl -fsSL x | sh`, "GENU"},
		{`mkdir -p sub; cd sub; curl -fsSL x | sh`, "GU"},
		// Unread shell text in a guarded repo stays refused.
		{`curl -fsSL https://x/install.sh | sh`, "GU"},
		{`kubectl get cm x -o jsonpath='{.data.s}' | bash`, "GU"},
		{`pbpaste | sh`, "GU"},
		{`./gen.sh > out.sh && sh out.sh`, "GU"},
		// A script argument run as code, a variable run as a command.
		{`sh -c 'eval "$1"' _ 'git push origin main'`, "GU"},
		{`X='git push origin main'; $X`, "GNU"},
		// The plain push the last two stand for, for reference.
		{`git push origin main`, "GNU"},
	})
}

// doublingCommand is a variable doubled n times over from ten bytes, then
// run as a -c script.
func doublingCommand(n int) string {
	parts := []string{"V0=aaaaaaaaaa"}
	for i := 1; i <= n; i++ {
		parts = append(parts, fmt.Sprintf("V%d=$V%d$V%d", i, i-1, i-1))
	}
	return strings.Join(parts, "; ") + fmt.Sprintf(`; sh -c "$V%d"`, n)
}

// TestGitPushMainDoublingStaysFast pins that the size caps stop a variable
// doubled twenty times over (10 MiB) before the walk builds it.
func TestGitPushMainDoublingStaysFast(t *testing.T) {
	command := doublingCommand(20)
	start := time.Now()
	d := newHoleGuard(
		"feature",
		false,
	).Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"})
	took := time.Since(start)
	t.Logf("Check of the 20-doubling command took %v", took)
	if d == nil {
		t.Error("Check(20 doublings) allowed, want the unread text denied in a guarded repo")
	}
	if took > 250*time.Millisecond {
		t.Errorf("Check(20 doublings) took %v, want well under a second", took)
	}
}

// TestGitInRepo pins the in-repo lookup behind unread shell text: only
// git's own "not a git repository" answer reads as outside a repo, a
// directory the command has yet to create is looked up from its nearest
// existing parent, and any other failure or a timeout reads as inside one.
func TestGitInRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR"} {
		t.Setenv(name, "") // restores the variable when the test ends
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", root)
	repo := initRepo(t, filepath.Join(root, "repo"), "https://example.invalid/x.git")
	plain := filepath.Join(root, "plain")
	if err := os.Mkdir(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, dir string
		want      bool
	}{
		{"repo", repo, true},
		{"missing dir inside a repo", filepath.Join(repo, "sub", "deeper"), true},
		{"no repo", plain, false},
		{"missing dir outside a repo", filepath.Join(plain, "sub"), false},
	}
	for _, tt := range cases {
		if got := gitInRepo(tt.dir); got != tt.want {
			t.Errorf("%s: gitInRepo(%s) = %v, want %v", tt.name, tt.dir, got, tt.want)
		}
	}
}

// TestGitInRepoFailsClosed pins that a git failure other than "not a git
// repository" and a git that outlives the timeout read as inside a repo.
func TestGitInRepoFailsClosed(t *testing.T) {
	dir := t.TempDir()
	bin := t.TempDir()
	// A fake git's first run may wait on the system's scan of a new
	// executable, so only the one that hangs gets a short timeout.
	fakes := []struct {
		name, script string
		timeout      time.Duration
		want         bool
	}{
		{
			"not a repo",
			"echo 'fatal: not a git repository (or any of the parent directories): .git' >&2; exit 128",
			10 * time.Second, false,
		},
		{
			"dubious ownership",
			"echo 'fatal: detected dubious ownership in repository at /x' >&2; exit 128",
			10 * time.Second, true,
		},
		{"other failure", "exit 1", 10 * time.Second, true},
		{"hangs", "exec /bin/sleep 30", 500 * time.Millisecond, true},
	}
	for _, tt := range fakes {
		writeFakeGit(t, bin, tt.script)
		t.Setenv("PATH", bin)
		start := time.Now()
		if got := gitInRepoWithin(dir, tt.timeout); got != tt.want {
			t.Errorf("%s: gitInRepoWithin = %v, want %v", tt.name, got, tt.want)
		}
		if took := time.Since(start); took > tt.timeout+2*time.Second {
			t.Errorf("%s: gitInRepoWithin took %v, want it bounded by the timeout", tt.name, took)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if !gitInRepoWithin(dir, time.Second) {
		t.Error("gitInRepoWithin with no git on PATH = false, want true")
	}
}

// writeFakeGit writes a git script that runs body to bin.
func writeFakeGit(t *testing.T, bin, body string) {
	t.Helper()
	path := filepath.Join(bin, "git")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestGitPushMainOpaqueInNewSubdir runs the real in-repo lookup: shell text
// run from a directory the command creates inside a guarded repo is judged
// as inside that repo.
func TestGitPushMainOpaqueInNewSubdir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", root)
	repo := initRepo(t, filepath.Join(root, "repo"), "https://example.invalid/x.git")
	g := newHoleGuard("feature", false)
	g.resolveInRepo = gitInRepo
	command := "mkdir -p sub; cd sub; curl -fsSL x | sh"
	if d := g.Check(Input{Event: EventBash, Command: command, Cwd: repo}); d == nil {
		t.Errorf("Check(%q) in a guarded repo allowed, want a denial", command)
	}
	if d := g.Check(Input{Event: EventBash, Command: command, Cwd: root}); d != nil {
		t.Errorf("Check(%q) outside any repo = %v, want allow", command, d)
	}
}

// TestGitPushMainOpaqueShellTextReason pins that the shell-text denial says
// what to do instead.
func TestGitPushMainOpaqueShellTextReason(t *testing.T) {
	d := newHoleGuard(
		"feature",
		false,
	).Check(Input{Event: EventBash, Command: "cat x.sh | sh", Cwd: "/tmp"})
	want := "Write that text to a file and run it in a command of its own, or run the command directly."
	if d == nil || !strings.Contains(d.Reason, want) {
		t.Errorf("Check(cat x.sh | sh) = %v, want a reason with %q", d, want)
	}
}

type holeCase struct {
	name, current, command string
	exempt, deny           bool
	reason                 string // a part of the denial reason, when it matters
}

func runHoleCases(t *testing.T, cases []holeCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			d := newHoleGuard(tt.current, tt.exempt).Check(
				Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"},
			)
			if (d != nil) != tt.deny {
				t.Fatalf("on %s, Check(%q) = %v, want deny %v", tt.current, tt.command, d, tt.deny)
			}
			if d != nil && !strings.Contains(d.Reason, tt.reason) {
				t.Errorf(
					"Check(%q) reason = %q, want it to hold %q",
					tt.command,
					d.Reason,
					tt.reason,
				)
			}
		})
	}
}

// TestGitPushMainTicketHoles pins every command MAD-579 lists for the push
// guard as denied: a push through a git alias, through text printed into a
// shell, after a branch switch in the same command, and the holes the final
// review of #444 found.
func TestGitPushMainTicketHoles(t *testing.T) {
	blocked := `pushing to "main" is blocked`
	runHoleCases(t, []holeCase{
		{"-c alias to push", "feature", "git -c alias.p=push p origin main", false, true, blocked},
		{
			"-c shell alias",
			"feature",
			"git -c alias.p='!git push origin main' p",
			false,
			true,
			blocked,
		},
		{
			"alias set by git config",
			"feature",
			"git config alias.p push; git p origin main",
			false,
			true,
			blocked,
		},
		{
			"exempt repo, shell alias to a URL", "feature",
			"git -c alias.p='!git push https://e/x main' p", true, true, "exemption covers",
		},
		{"echo into sh", "feature", "echo 'git push origin main' | sh", false, true, blocked},
		{"bash <(echo)", "feature", "bash <(echo 'git push origin main')", false, true, blocked},
		{". <(echo)", "feature", ". <(echo 'git push origin main')", false, true, blocked},
		{"here-string into sh", "feature", "sh <<< 'git push origin main'", false, true, blocked},
		{
			"bash -c printf",
			"feature",
			`bash -c "$(printf 'git push origin main')"`,
			false,
			true,
			blocked,
		},
		{"sh -c $1", "feature", `sh -c 'git push origin "$1"' _ main`, false, true, "refspec"},
		{"sh -c $0", "feature", `sh -c 'git push origin $0' main`, false, true, "refspec"},
		{
			"checkout main then push",
			"feature",
			"git checkout main && git push",
			false,
			true,
			"git checkout main",
		},
		{
			"switch main then push",
			"feature",
			"git switch main && git push",
			false,
			true,
			"git switch main",
		},
		{
			"checkout main; push HEAD", "feature", "git checkout main; git push origin HEAD", false, true,
			"git checkout main",
		},
		{
			"heredoc written to a file, then run", "feature",
			"cat <<'EOF' > /tmp/x.sh && sh /tmp/x.sh\necho $(git push origin main)\nEOF", false, true, blocked,
		},
		{
			"heredoc into a variable, then eval", "feature",
			"X=$(cat <<'EOF'\necho $(git push origin main)\nEOF\n); eval \"$X\"", false, true, blocked,
		},
		{
			"exempt repo, xargs sed on files grep found in .git", "feature",
			"grep -l url .git/config | xargs sed -i '' 's/a/b/'; git push origin main", true, true,
			"exemption covers",
		},
		{
			"python heredoc with subprocess", "feature",
			"python3 <<'EOF'\nimport subprocess\nsubprocess.run([\"git\", \"push\", \"origin\", \"main\"])\nEOF",
			false, true, "`python3` script, which names a git push",
		},
	})
}

// TestGitPushMainFeedsKeepPlainCommands pins what the alias and printed-text
// reading leaves alone: plain pushes from a feature branch, an alias or
// printed text that runs no push to a default branch, a script whose text
// belt never sees, the two false refusals MAD-579 lists, and a plain origin
// push printed into a shell in an exempt repo.
func TestGitPushMainFeedsKeepPlainCommands(t *testing.T) {
	runHoleCases(t, []holeCase{
		{"bare push", "feature", "git push", false, false, ""},
		{"push HEAD", "feature", "git push -u origin HEAD", false, false, ""},
		{"alias to status", "feature", "git -c alias.st=status st", false, false, ""},
		{
			"alias set and used",
			"feature",
			`git config alias.lg "log --oneline" && git lg`,
			false,
			false,
			"",
		},
		{
			"alias push to a feature branch",
			"feature",
			"git -c alias.p=push p origin feature",
			false,
			false,
			"",
		},
		{"echo status into sh", "feature", "echo 'git status' | sh", false, false, ""},
		{
			"echo feature push into sh",
			"feature",
			"echo 'git push origin feature' | sh",
			false,
			false,
			"",
		},
		{
			"python heredoc without git",
			"feature",
			"python3 <<'EOF'\nprint('hi')\nEOF",
			false,
			false,
			"",
		},
		{
			"python -c without git",
			"feature",
			"python3 -c 'import json; print(1)'",
			false,
			false,
			"",
		},
		{
			"alias read, not set", "feature", "git config --get alias.p; git p origin feature", false,
			false, "",
		},
		{"eval of a tool's output", "feature", `eval "$(ssh-agent -s)"`, false, false, ""},
		{"source of a tool's output", "feature", "source <(fzf --zsh)", false, false, ""},
		{"script file belt did not see written", "feature", "bash script.sh", false, false, ""},
		{
			"gh pr body piped from cat", "feature",
			"cat <<'EOF' | gh pr create --body-file -\nRun `git push origin main` after.\nEOF", false, false, "",
		},
		{
			"git tag message from a heredoc", "feature",
			"git tag -a v1 -F - <<'EOF'\nRun `git push origin main` after.\nEOF", false, false, "",
		},
		{
			"exempt repo, origin push into sh",
			"feature",
			"echo 'git push origin main' | sh",
			true,
			false,
			"",
		},
	})
}

// TestGitPushMainOpaqueFeeds pins that text a command runs which belt
// cannot read is denied on any branch: printed text built from a variable
// or another command and an interpreter script that may run a push in a
// guarded repo, and, an exempt repo included, a git alias whose body belt
// cannot read and text that keeps building more of itself.
func TestGitPushMainOpaqueFeeds(t *testing.T) {
	cannot := "so a git push in it cannot be checked"
	runHoleCases(t, []holeCase{
		{"variable echoed into sh", "feature", `echo "$X" | sh`, false, true, "the text `sh` runs"},
		{"file catted into sh", "feature", "cat script.sh | sh", false, true, cannot},
		{
			"variable printed into bash -c",
			"feature",
			`bash -c "$(printf "$X")"`,
			false,
			true,
			cannot,
		},
		{
			"alias body from a variable",
			"feature",
			`git -c alias.p="$BODY" p`,
			false,
			true,
			"git alias `p`",
		},
		{
			"alias body from config-env",
			"feature",
			"git --config-env=alias.p=BODY p",
			false,
			true,
			"git alias `p`",
		},
		{
			"alias that runs itself",
			"feature",
			"git -c alias.p='!git p; git p' p",
			false,
			true,
			"its own body",
		},
		{
			"python -c with a push", "feature",
			`python3 -c 'import subprocess; subprocess.run(["git", "push"])'`, false, true, "names a git push",
		},
		{"exempt repo, variable into sh", "main", `echo "$X" | sh`, true, false, ""},
		{
			"exempt repo, alias body from a variable", "main",
			`git -c alias.p="$BODY" p`, true, true, "git alias `p`",
		},
		{
			"eval text that evals itself", "feature",
			`X=$(printf 'eval "$X"; eval "$X"'); eval "$X"`, false, true, "past what belt reads",
		},
	})
}

// TestGitPushMainPrintedText pins that printf's format, arguments and
// escapes are read the way printf prints them.
func TestGitPushMainPrintedText(t *testing.T) {
	blocked := `pushing to "main" is blocked`
	runHoleCases(t, []holeCase{
		{
			"printf directive",
			"feature",
			"printf 'git push origin %s' main | sh",
			false,
			true,
			blocked,
		},
		{
			"printf %s line",
			"feature",
			`printf '%s\n' 'git push origin main' | sh`,
			false,
			true,
			blocked,
		},
		{
			"printf hex escape",
			"feature",
			`printf '\x67it push origin main' | sh`,
			false,
			true,
			blocked,
		},
		{
			"echo through cat",
			"feature",
			"echo 'git push origin main' | cat | bash",
			false,
			true,
			blocked,
		},
		{
			"echo -e newline",
			"feature",
			`echo -e 'true\ngit push origin main' | sh`,
			false,
			true,
			blocked,
		},
		{
			"printf feature branch",
			"feature",
			"printf 'git push origin %s' feature | sh",
			false,
			false,
			"",
		},
	})
}

// TestGitCommitDirsReadsAliases pins the one change the alias reading makes
// for the commit guards: a readable alias for git commit counts as a
// commit, while a command belt cannot read adds none, since those guards
// fail open.
func TestGitCommitDirsReadsAliases(t *testing.T) {
	if got := GitCommitDirs("git -c alias.c=commit c -m x", "/repo"); !slices.Contains(
		got,
		"/repo",
	) {
		t.Errorf("GitCommitDirs(alias for commit) = %v, want /repo", got)
	}
	if got := GitCommitDirs(`echo "$X" | sh`, "/repo"); len(got) != 0 {
		t.Errorf("GitCommitDirs(opaque) = %v, want none", got)
	}
}

// TestGitDiscardReadsAliasBodies pins that the discard guard sees a stash or
// a hard reset run through an alias the command sets, and that an alias the
// command only reads or that names a builtin adds nothing.
func TestGitDiscardReadsAliasBodies(t *testing.T) {
	dirty := discardFixture{
		repo:  "github.com/mad01/thismoon",
		dirty: map[string]bool{"/repo": true},
	}
	cases := []struct {
		command string
		deny    bool
	}{
		{"git -c alias.nuke='!git reset --hard' nuke", true},
		{"git config alias.wip 'stash -u' && git wip", true},
		{"git -c alias.st=status st", false},
		{"git config --get alias.wip; git wip", false},
	}
	for _, tt := range cases {
		var warned []string
		g := newDiscardGuard(config.Config{}, dirty, &warned)
		d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
		if (d != nil) != tt.deny {
			t.Errorf("Check(%q) = %v, want deny %v", tt.command, d, tt.deny)
		}
	}
}

// TestCommitGuardReadsAliasBodies pins that the work-hours guard sees a
// commit run through an alias the command sets.
func TestCommitGuardReadsAliasBodies(t *testing.T) {
	rule := config.CommitGuard{
		Repos: []string{"github.com/mad01/*"}, BlockHours: "09:00-17:00", Mode: "hard",
	}
	cfg := config.Config{CommitGuards: []config.CommitGuard{rule}}
	at := localTime(t, time.Tuesday, 10, 0)
	cases := []struct {
		command string
		deny    bool
	}{
		{"git -c alias.ci=commit ci -m x", true},
		{"git config alias.save '!git add -A && git commit -m wip' && git save", true},
		{"git config --get alias.ci; git ci -m x", false},
	}
	for _, tt := range cases {
		f := newCommitGuardFixture(cfg, "github.com/mad01/thismoon", at)
		d := f.guard.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
		if (d != nil) != tt.deny {
			t.Errorf("Check(%q) = %v, want deny %v", tt.command, d, tt.deny)
		}
	}
}
