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
