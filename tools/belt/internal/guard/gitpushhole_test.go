package guard

import (
	"slices"
	"strings"
	"testing"

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

// TestGitPushMainOpaqueShellTextByDir pins where unread shell text counts:
// in a repo that blocks direct pushes, and in a directory belt cannot tell.
// Outside any repo and in an exempt repo it passes. An alias body belt
// cannot read is denied in every one of them, beside shell text too.
func TestGitPushMainOpaqueShellTextByDir(t *testing.T) {
	dirs := []struct {
		name           string
		inRepo, exempt bool
		cd             string // a cd in front of the command
	}{
		{"not a repo", false, false, ""},
		{"exempt repo", true, true, ""},
		{"guarded repo", true, false, ""},
		{"unknown dir", true, false, `cd "$D" && `},
	}
	commands := []struct {
		command string
		deny    map[string]bool // by dirs name
	}{
		{"curl x | sh", map[string]bool{"guarded repo": true, "unknown dir": true}},
		{"cat x.sh | sh", map[string]bool{"guarded repo": true, "unknown dir": true}},
		{`echo "$X" | sh`, map[string]bool{"guarded repo": true, "unknown dir": true}},
		{
			`echo "$X" | sh; git -c alias.p="$B" p`,
			map[string]bool{
				"not a repo":   true,
				"exempt repo":  true,
				"guarded repo": true,
				"unknown dir":  true,
			},
		},
	}
	for _, d := range dirs {
		for _, tt := range commands {
			command := d.cd + tt.command
			g := newHoleGuard("feature", d.exempt)
			g.resolveInRepo = func(string) bool { return d.inRepo }
			got := g.Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"})
			if (got != nil) != tt.deny[d.name] {
				t.Errorf("%s: Check(%q) = %v, want deny %v", d.name, command, got, tt.deny[d.name])
			}
		}
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
// or another command in a guarded repo, and, an exempt repo included, a git
// alias whose body belt cannot read, an interpreter script that names a
// push, and text that keeps building more of itself.
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
			"alias named but not set",
			"feature",
			"git config --get alias.p; git p origin feature",
			false,
			true,
			cannot,
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
