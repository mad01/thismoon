package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// maskDenyPatterns are the deny-list entries the mask cases run against: a
// multi-word push, a kubectl verb, a git config write, and the rm -rf extra.
var (
	maskClaudeDeny = []string{"git push origin main", "kubectl delete", "git config"}
	maskExtra      = []string{"rm -rf"}
)

// TestScriptDenyMaskPasses are the messages and prints that only name a denied
// command. The guard must let them through: nothing in them runs the command.
func TestScriptDenyMaskPasses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CDPATH", "")
	if err := os.MkdirAll(filepath.Join(home, ".tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	g := NewScriptDenyList(scriptDenyConfig(maskClaudeDeny, maskExtra))
	heredoc := func(head, body string) string {
		return head + " <<'EOF'\n" + body + "\nEOF"
	}
	cases := []scriptDenyCase{
		{
			"git commit message substitution",
			`git commit -m "$(cat <<'EOF'` + "\ngit push origin main\nEOF\n)\"",
			"",
			false,
		},
		{
			"gh pr body substitution",
			`gh pr create --title x --body "$(cat <<'EOF'` + "\nrun git push origin main\nEOF\n)\"",
			"",
			false,
		},
		{
			"git commit --message substitution",
			`git commit --message "$(cat <<'EOF'` + "\nkubectl delete pod x\nEOF\n)\"",
			"",
			false,
		},
		{
			"git commit reads the body from a heredoc",
			heredoc("git commit -F -", "git push origin main"),
			"",
			false,
		},
		{
			"gh issue body from a heredoc",
			heredoc("gh issue create --title x -F -", "then run kubectl delete pod x"),
			"",
			false,
		},
		{"echo only prints the command", `echo "git push origin main"`, "", false},
		{"printf only prints the command", `printf 'git push origin main\n'`, "", false},
		{"echo with a quoted semicolon", `echo "done; git push origin main"`, "", false},
		{"echo with a quoted pipe then xargs", `echo "x | xargs rm -rf"`, "", false},
		{"echo with a quoted bash -c", `echo "then: bash -c 'kubectl delete pod x'"`, "", false},
		{
			"echo after an earlier inline script",
			`bash -c 'echo hi' && echo "; git push origin main"`,
			"",
			false,
		},
		{
			"echo after an interpreter heredoc",
			"bash <<'EOF'\necho hi\nEOF\necho \"git push origin main\"",
			"",
			false,
		},
		{
			"note written under the home temp folder",
			"cd ~/.tmp && cat > x.md <<'EOF'\ngit push origin main\nEOF",
			"",
			false,
		},
	}
	runMaskCases(t, g, cases)
}

// TestScriptDenyMaskRefused are the shapes that still run a denied command.
// Masking a message must not open any of them.
func TestScriptDenyMaskRefused(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CDPATH", "")
	dir := t.TempDir()
	g := NewScriptDenyList(scriptDenyConfig(maskClaudeDeny, maskExtra))
	cases := []scriptDenyCase{
		{"print piped into sh", `echo "git push origin main" | sh`, "", true},
		{"printf piped into bash", `printf 'rm -rf /tmp/x\n' | bash`, "", true},
		{"print piped through cat into sh", `echo "rm -rf /tmp/x" | cat | sh`, "", true},
		{
			"print piped through tee into sh",
			`echo "kubectl delete pod x" | tee s.sh | sh`,
			dir,
			true,
		},
		{"eval of the command", `eval "git push origin main"`, "", true},
		{"substitution inside an echo string", `echo "$(git push origin main)"`, "", true},
		{"backtick substitution inside echo", "echo \"`rm -rf /tmp/x`\"", "", true},
		{
			"escaped quotes in backticks inside echo",
			"echo \"`kubectl \\\"delete\\\" pod x`\"",
			"",
			true,
		},
		{"$\\ joins ( inside an echo string", "echo \"$\\\n(kubectl delete pod x)\"", "", true},
		{"assignment from a substitution", `x=$(rm -rf /tmp/x)`, "", true},
		{
			"unquoted commit heredoc runs a substitution",
			`git commit -m "$(cat <<EOF` + "\n$(git push origin main)\nEOF\n)\"",
			"",
			true,
		},
		{
			"unquoted gh heredoc runs a substitution",
			"gh pr create -F - <<EOF\n$(kubectl delete pod x)\nEOF",
			"",
			true,
		},
		{
			"write then run the file",
			`echo "rm -rf /tmp/x" > ` + filepath.Join(
				dir,
				"gen.sh",
			) + " && sh " + filepath.Join(
				dir,
				"gen.sh",
			),
			dir,
			true,
		},
		{
			"write then chmod then run",
			`echo "git push origin main" > run.sh && chmod +x run.sh`,
			dir,
			true,
		},
	}
	runMaskCases(t, g, cases)
}

// TestScriptDenyMaskRunsAlongside are the shapes where something else in the
// command runs beside the masked write or after the command ends, so it can
// run the text the write puts in a file. The mask must stay off, and the
// inline-script scan then reads the payload as before.
func TestScriptDenyMaskRunsAlongside(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CDPATH", "")
	g := NewScriptDenyList(scriptDenyConfig(maskClaudeDeny, maskExtra))
	cases := []scriptDenyCase{
		{
			"background interpreter reads the echo's file",
			`bash -c 'sleep 1; sh /tmp/f' & echo "rm -rf /x" > /tmp/f`,
			"",
			true,
		},
		{
			"nohup background interpreter",
			`nohup bash -c 'sleep 1; sh /tmp/f' >/dev/null 2>&1 & echo "rm -rf /x" > /tmp/f`,
			"",
			true,
		},
		{
			"background python",
			`python3 -c 'import time,os; time.sleep(1); os.system("sh /tmp/f")' & echo "rm -rf /x" > /tmp/f`,
			"",
			true,
		},
		{
			"background sh sources the printf's file",
			`sh -c 'sleep 1; . /tmp/f' & printf '%s' "rm -rf /x" > /tmp/f`,
			"",
			true,
		},
		{
			"background interpreter runs the commit message file",
			`bash -c 'sleep 2; sh .git/COMMIT_EDITMSG' & git commit --allow-empty -m "rm -rf /x"`,
			"",
			true,
		},
		{
			"& then a newline",
			"bash -c 'sleep 1; sh /tmp/f' &\necho \"rm -rf /x\" > /tmp/f",
			"",
			true,
		},
		{
			"& then a newline before a commit",
			"bash -c 'sleep 2; sh .git/COMMIT_EDITMSG' &\ngit commit --allow-empty -m \"rm -rf /x\"",
			"",
			true,
		},
		{
			"background job inside the -c script",
			`bash -c '(sleep 1; sh /tmp/f) &'; echo "rm -rf /x" > /tmp/f`,
			"",
			true,
		},
		{
			"trap set earlier runs the file at exit",
			`trap 'sh /tmp/f' EXIT; bash -c 'echo hi'; echo "rm -rf /x" > /tmp/f`,
			"",
			true,
		},
		{
			"python starts a child that outlives it",
			`python3 -c 'import subprocess; subprocess.Popen(["sh", "-c", "sleep 1; sh /tmp/f"])'; ` +
				`echo "rm -rf /x" > /tmp/f`,
			"",
			true,
		},
	}
	runMaskCases(t, g, cases)
}

// TestMaskDataRefusesLaterRunners pins the mask itself: a command with any
// part that runs beside the masked text or after the command ends comes
// back unchanged, so no check reads less than it did before masking.
func TestMaskDataRefusesLaterRunners(t *testing.T) {
	commands := []string{
		`sleep 1 & echo "rm -rf /x" > /tmp/f`,
		"sleep 1 &\necho \"rm -rf /x\" > /tmp/f",
		`(sleep 1; sh /tmp/f) & echo "rm -rf /x" > /tmp/f`,
		`echo "rm -rf /x" > /tmp/f & sh /tmp/f`,
		`trap 'sh /tmp/f' EXIT; echo "rm -rf /x" > /tmp/f`,
		`coproc cat; echo "rm -rf /x"`,
		`sh -c 'sh /tmp/f & disown'; echo "rm -rf /x" > /tmp/f`,
		`setsid sh /tmp/f; echo "rm -rf /x" > /tmp/f`,
		`echo "x" > /tmp/f; at now < /tmp/f; echo "rm -rf /x" > /tmp/f`,
	}
	for _, cmd := range commands {
		if got := maskData(cmd); got != cmd {
			t.Errorf("maskData(%q) = %q, want it unchanged", cmd, got)
		}
	}
	// An & that is data, or part of &&, &> or >&, blocks nothing.
	for _, cmd := range []string{
		`echo "a & b; rm -rf /x"`,
		`true && echo "rm -rf /x" 2>&1`,
		`git commit -m "fix a & b: rm -rf /x" &>/dev/null`,
	} {
		if got := maskData(cmd); got == cmd {
			t.Errorf("maskData(%q) left it unchanged, want the message masked", cmd)
		}
	}
}

// delimPrefixed is the review's bash-only shape: inside $( ), bash ends a
// heredoc at the line `EOF)` and runs what follows it, while zsh reads on to
// the line `EOF`. %s is the command after the early close.
const delimPrefixed = "git commit -m \"$(cat <<'EOF'\nmsg\nEOF)\" ; %s\nEOF\n)\""

// TestScriptDenyMaskDelimiterPrefix pins that a heredoc body with a line
// that starts with the delimiter but is not the delimiter is never masked:
// shells disagree on where such a body ends.
func TestScriptDenyMaskDelimiterPrefix(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CDPATH", "")
	g := NewScriptDenyList(scriptDenyConfig(maskClaudeDeny, maskExtra))
	runMaskCases(t, g, []scriptDenyCase{
		{
			"EOF) closes the heredoc in bash",
			fmt.Sprintf(delimPrefixed, "bash -c 'rm -rf /x'"),
			"",
			true,
		},
	})
	for _, cmd := range []string{
		fmt.Sprintf(delimPrefixed, "echo 'rm -rf /x'"),
		"git commit -F - <<'EOF'\nEOFx rm -rf /x\nEOF",
		"git commit -F - <<-'EOF'\n\tEOF) rm -rf /x\n\tEOF",
	} {
		if got := maskData(cmd); !strings.Contains(got, "rm -rf /x") {
			t.Errorf("maskData(%q) = %q, want the body left as written", cmd, got)
		}
	}
}

// earlyClose is the bash 3.2 shape (MAD-671): bash 3.2, macOS's /bin/bash,
// ends a $( ) at the first ) its scans cannot match and knows no heredoc
// there, so it runs the command after `)"`, while bash 5 and zsh
// read the whole body as message text. The first %s is the command head,
// the second the command bash 3.2 runs.
const earlyClose = "%s \"$(cat <<'EOF'\nx\n)\"; %s; echo \"(\nEOF\n)\""

// TestScriptDenyMaskBash32Close pins that a `$(cat <<'EOF' ... EOF)` body is
// masked only when bash 3.2 runs no more of it than cat (bash32Closes). A
// body it cannot vouch for is scanned like any other substitution body, so
// naming a denied command in it denies, while the same body in a -F -
// heredoc stays masked.
func TestScriptDenyMaskBash32Close(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CDPATH", "")
	g := NewScriptDenyList(scriptDenyConfig(maskClaudeDeny, maskExtra))
	cat := func(head, body string) string {
		return head + " \"$(cat <<'EOF'\n" + body + "\nEOF\n)\""
	}
	runMaskCases(t, g, []scriptDenyCase{
		{
			"commit body ends early in bash 3.2",
			fmt.Sprintf(earlyClose, "git commit -m", "kubectl delete pod x"),
			"",
			true,
		},
		{
			"pr body ends early in bash 3.2",
			fmt.Sprintf(earlyClose, "gh pr create --title t --body", "rm -rf /x"),
			"",
			true,
		},
		{"echo ends early in bash 3.2", fmt.Sprintf(earlyClose, "echo", "git push origin main"), "", true},
		{
			"a quoted ) before the early close",
			cat("git commit -m", "x ')' y\n)\"; kubectl delete pod x; echo \"("),
			"",
			true,
		},
		{"parens in a commit subject", cat("git commit -m", "fix(belt): kubectl delete pod x is text"), "", false},
		{
			"nested parens in a pr body",
			cat("gh pr create --title t --body", "keep (rm -rf (in the guard)) as text"),
			"",
			false,
		},
		{"paired quotes", cat("git commit -m", "it's 'kubectl delete', it's text"), "", false},
		// A lone apostrophe bash 3.2's parse step never closes is a syntax
		// error there, so it runs nothing; one a later word closes is
		// scanned, and so is one bash 3.2 may reach another way than belt.
		{"a lone apostrophe", cat("git commit -m", "don't let kubectl delete through"), "", false},
		{
			"a lone apostrophe in a pr body",
			cat("gh pr create --title \"fix(belt): kubectl\" --body", "belt denies `kubectl delete`.\n\nIt's tested."),
			"",
			false,
		},
		{
			"a lone apostrophe, then a second message",
			cat("git commit -m", "don't let kubectl delete through") + ` -m "second (one)"`,
			"",
			false,
		},
		{
			"a lone apostrophe a later word closes",
			cat("git commit -m", "don't let kubectl delete through") + ` -m "it's done)"`,
			"",
			true,
		},
		{
			"a lone apostrophe after an earlier message",
			cat("git commit -m", "don't") + cat(" -m", "it's x)\"; kubectl delete pod x; echo \""),
			"",
			true,
		},
		{
			"a lone apostrophe behind a $",
			cat("git -C \"$HOME/x\" commit -m", "don't let kubectl delete through"),
			"",
			true,
		},
		{"a # heading in a pr body", cat("gh pr create --title t --body", "## Summary\n- block rm -rf"), "", false},
		{"a lone apostrophe, no denied name", cat("git commit -m", "don't count twice"), "", false},
		{"a backtick pair", cat("git commit -m", "fix(belt): read `rm -rf` behind sudo"), "", false},
		{"an issue ref", cat("git commit -m", "fix(belt): deny kubectl delete\n\nCloses #12"), "", false},
		{
			"a numbered list bash 3.2 ends early",
			cat("git commit -m", "fix(belt): two cases\n\n1) rm -rf with a long option\n2) a wrapper"),
			"",
			false,
		},
		{"a list then a substitution", cat("git commit -m", "a\n1) x $(rm -rf /x)"), "", true},
		{"a list then backticks", cat("git commit -m", "a\n1) x `rm -rf /x`"), "", true},
		{"a backslash-newline", cat("git commit -m", "EO\\\nF\nkubectl delete pod x"), "", true},
		{
			"an unquoted body with a comment bash 3.2 never closes",
			"git commit -m $(cat <<'EOF'\nfix (see #12) kubectl delete\nEOF\n)",
			"",
			false,
		},
		// bash 3.2's expansion takes a # after a blank for a comment inside
		// "$( )" too, while its parse step does not, so a quote or ) on a #
		// line counts in one scan and not the other.
		{"the quoted form of that body", cat("git commit -m", "fix (see #12) kubectl delete"), "", true},
		{
			"an apostrophe on a # line, then backticks",
			cat("gh pr create --title t --body", "## What's new\n1) a fix for `kubectl delete pod x`\n- don't break it"),
			"",
			true,
		},
		{
			"an apostrophe on a # line, then a substitution",
			cat("git commit -m", "# it's\n1) $(kubectl delete pod x)\nx'"),
			"",
			true,
		},
		{"a ) on a # line", cat("git commit -m", "## step 1)\n- kubectl delete pod x"), "", false},
		{
			"a summary heading, then backticks",
			cat("gh pr create --title t --body", "## Summary\n- deny `kubectl delete` behind `sudo`"),
			"",
			false,
		},
		{"a $ in a backtick span", cat("git commit -m", "docs: note `$HOME` handling, see #40"), "", false},
		{
			"the --body-file - form of an apostrophe on a # line",
			"gh pr create --title t --body-file - <<'EOF'\n## What's new\n1) a fix for `kubectl delete pod x`\nEOF",
			"",
			false,
		},
		{
			"the -F - form of an apostrophe on a # line",
			"git commit -F - <<'EOF'\n# it's\n1) $(kubectl delete pod x)\nx'\nEOF",
			"",
			false,
		},
		{"a backtick body", "git commit -m \"`cat <<'EOF'\nfix (see #12) kubectl delete\nEOF\n`\"", "", false},
		{
			"a backslash in a backtick body",
			"git commit -m \"`cat <<'EOF'\nEO\\\nF\nkubectl delete pod x\nEOF\n`\"",
			"",
			true,
		},
		{
			"the -F - form of a lone apostrophe",
			"git commit -F - <<'EOF'\ndon't let kubectl delete through\nEOF",
			"",
			false,
		},
	})
}

// TestScriptDenyBash32Tail pins the text after a `$(cat <<'EOF' ... EOF)`
// bash 3.2 may end in another place than belt (bash32Tail). A quote of the
// body stays open into the text after it, which bash 3.2 then runs where
// belt reads data, so nothing there is masked and all of it is scanned as
// code, comments and heredoc bodies included. A body bash 3.2 ends early
// whose rest it reads as text of the same word, and a message that takes in
// all the rest, leave the text after it as belt reads it.
func TestScriptDenyBash32Tail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CDPATH", "")
	g := NewScriptDenyList(scriptDenyConfig(maskClaudeDeny, maskExtra))
	cat := func(head, body string) string {
		return head + " \"$(cat <<'EOF'\n" + body + "\nEOF\n)\""
	}
	tests := []struct {
		name, command string
		deny          bool
	}{
		{"a later word closes the quote", cat("git commit -m", "It's done") + "; echo '\n)\" ; kubectl delete pod x ; #'", true},
		{"a later heredoc closes the quote", cat("git commit -m", "It's done") + "; cat <<'Y'\n')\" ; rm -rf /x ; #'\nY", true},
		{"a later comment closes the quote", cat("git commit -m", "It's done") + "; echo 'x'\n# ')\" ; kubectl delete pod x ; #'", true},
		{"an assignment's message", "x=\"$(cat <<'EOF'\nIt's done\nEOF\n)\"; echo '\n)\" ; kubectl delete pod x ; #'", true},
		{"a message in a subshell", "(" + cat("git commit -m", "It's done") + "); echo '\n)\") ; rm -rf /x ; #'", true},
		{"an unquoted list", "git commit -m $(cat <<'EOF'\n1) it's\nEOF\n); echo '\nkubectl delete pod x ; #'", true},
		{
			"text after a message that takes in the rest",
			cat("git commit -m", "It's x") + " && cat <<'Y'\n') ; echo z ; echo \"z\nY\nkubectl delete pod x",
			true,
		},
		{"a list, then quoted words", cat("git commit -m", "x\n\n1) renames \"a\" to \"b\"") + " && echo 'kubectl delete'", false},
		{"a list, then backticks", cat("git commit -m", "x\n\n1) adds `--quiet`\n2) docs") + " && echo 'rm -rf /x'", false},
		{
			"a message that takes in the rest",
			cat("git commit -m", "It's x") + " && cat <<'Y'\n') ; kubectl delete pod x ; echo \"z",
			false,
		},
		{"a later ) in a later word", cat("git commit -m", "- fix (part 1") + ` -m "next ) ; kubectl delete pod x ; echo ("`, false},
		// bash 3.2's expansion reads the word again and may end the $( ) in
		// another place than its parse step, which flips the quotes after it.
		{"the expansion ends it earlier, then a <( )", "git commit -m \"$(cat <<'EOF'\na # (\nb ) '\"' ) 'z\nEOF\n)<(rm -rf /x)\"", true},
		{"a message whose expansion runs a later comment", cat("git commit -m", "# It's x") + " && echo 'q\nZ' # $(rm -rf /x) ')\"", true},
		{"a message nested past the reader's depth", "echo \"" + deepMessage("It's done") + "\" '\n)\" ; rm -rf /x ; #'", true},
	}
	for _, tt := range tests {
		d := g.Check(Input{Event: EventBash, Command: tt.command})
		switch {
		case (d != nil) != tt.deny:
			t.Errorf("%s: Check(%q) = %v, want deny %v", tt.name, tt.command, d, tt.deny)
		case d != nil && !strings.HasSuffix(d.Reason, bash32Hint):
			t.Errorf("%s: Check(%q) reason %q, want it to end in bash32Hint", tt.name, tt.command, d.Reason)
		}
	}
	tail := cat("git commit -m", "It's done") + "; echo '\n)\" ; x ; #'"
	if got := maskData(tail); got != tail {
		t.Errorf("maskData(%q) = %q, want the text after the message as written", tail, got)
	}
}

// deepMessage returns a `git commit -m "$(cat <<'EOF' ... EOF)"` with body
// nested past maxShellNesting: $( ) levels, a ${ } whose body the reader
// does not follow, then more $( ) levels around the commit.
func deepMessage(body string) string {
	levels := strings.Repeat("$(echo ", maxShellNesting-1)
	ends := strings.Repeat(")", maxShellNesting-1)
	commit := "git commit -m \"$(cat <<'EOF'\n" + body + "\nEOF\n)\""
	return levels + "${x:-" + levels + commit + ends + "}" + ends
}

// TestScriptDenyBash32Reason pins the reason for a message belt reads as
// code only because bash 3.2 may run part of it: it names the forms that
// stay data (bash32Hint). Every other denial keeps its own reason.
func TestScriptDenyBash32Reason(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CDPATH", "")
	g := NewScriptDenyList(scriptDenyConfig(maskClaudeDeny, maskExtra))
	for _, tt := range []struct {
		name, command string
		bash32        bool
	}{
		{
			"a lone apostrophe a later word closes",
			"git commit -m \"$(cat <<'EOF'\ndon't let kubectl delete through\nEOF\n)\" -m \"it's done)\"",
			true,
		},
		{"the early close", fmt.Sprintf(earlyClose, "git commit -m", "kubectl delete pod x"), true},
		{
			"an apostrophe on a # line",
			"gh pr create --title t --body \"$(cat <<'EOF'\n## What's new\n1) a fix for `kubectl delete pod x`\n- don't break it\nEOF\n)\"",
			true,
		},
		{"a substitution", "echo \"$(kubectl delete pod x)\"", false},
		{
			"a message beside a substitution",
			"git commit -m \"$(cat <<'EOF'\ndon't\nEOF\n)$(kubectl delete pod x)\"",
			false,
		},
	} {
		d := g.Check(Input{Event: EventBash, Command: tt.command})
		switch {
		case d == nil:
			t.Errorf("%s: Check(%q) allowed, want a denial", tt.name, tt.command)
		case strings.HasSuffix(d.Reason, bash32Hint) != tt.bash32:
			t.Errorf("%s: Check(%q) reason %q, want bash32Hint %v", tt.name, tt.command, d.Reason, tt.bash32)
		case tt.bash32 && strings.Contains(d.Reason, directHint):
			t.Errorf("%s: Check(%q) reason %q still holds directHint", tt.name, tt.command, d.Reason)
		case tt.bash32 && strings.Contains(d.Reason, askUserToRun):
			t.Errorf("%s: Check(%q) reason %q still holds askUserToRun", tt.name, tt.command, d.Reason)
		}
	}
}

// TestMaskDataMessageFlagsOnly pins what a git commit or gh call masks: the
// values of its message flags, and a heredoc only where -F - (or its long
// form) reads the message from it. Every other word stays as written.
func TestMaskDataMessageFlagsOnly(t *testing.T) {
	tests := []struct{ cmd, want string }{
		{
			`git commit -m "msg one" --author="A <a@b>" file.txt`,
			`git commit -m           --author="A <a@b>" file.txt`,
		},
		{`git commit -am "msg"`, `git commit -am      `},
		{`git commit --message="msg" -q`, `git commit                 -q`},
		{`git -C /r commit -m"msg" x`, `git -C /r commit         x`},
		{"git commit -F - <<'EOF'\nbody\nEOF", "git commit -F - <<'EOF'\n    \nEOF"},
		{"git commit --file=- <<'EOF'\nbody\nEOF", "git commit --file=- <<'EOF'\n    \nEOF"},
		{"git commit -F msg.txt <<'EOF'\nbody\nEOF", "git commit -F msg.txt <<'EOF'\nbody\nEOF"},
		{
			`gh pr create --title "t x" --body "b y" --repo o/r`,
			`gh pr create --title       --body       --repo o/r`,
		},
		{
			"gh issue comment 3 -F - <<'EOF'\nbody\nEOF",
			"gh issue comment 3 -F - <<'EOF'\n    \nEOF",
		},
		{`gh release create v1 -n "notes" a.tgz`, `gh release create v1 -n         a.tgz`},
		{`echo "a b" c`, "echo" + strings.Repeat(" ", 8)},
	}
	for _, tt := range tests {
		if got := maskData(tt.cmd); got != tt.want {
			t.Errorf("maskData(%q)\n got %q\nwant %q", tt.cmd, got, tt.want)
		}
	}
}

func runMaskCases(t *testing.T, g *ScriptDenyList, cases []scriptDenyCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: tt.cwd})
			if got := d != nil; got != tt.deny {
				t.Fatalf("deny = %v, want %v (denial: %+v)\n%s", got, tt.deny, d, tt.command)
			}
		})
	}
}
