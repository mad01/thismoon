package guard

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// TestMain clears CDPATH, which the cd tracking reads from belt's own
// environment (walkSegments), so no test depends on the shell running it.
func TestMain(m *testing.M) {
	if err := os.Unsetenv("CDPATH"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// invocationDirs lists the directory of each invocation, in order.
func invocationDirs(cmds []gitInvocation) []string {
	var dirs []string
	for _, c := range cmds {
		dirs = append(dirs, c.dir)
	}
	return dirs
}

// TestGitCommandsAt pins where the parser places an invocation: the tracked
// shell directory for a bare one (cwd, or where a `cd`/`pushd` before it
// landed, MAD-366), the -C target for the rest, expanded the way the shell
// would (resolveDir) so ~, $HOME, and the absolute path all name one
// directory (MAD-370). A target only a shell can expand is unknown ("")
// rather than a path git would never see.
func TestGitCommandsAt(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	const home = "/Users/tester"
	tests := []struct {
		name    string
		command string
		cwd     string
		sub     string
		want    []string // dir per matching invocation, in order
	}{
		{"bare runs in cwd", "git commit -m x", "/session", "commit", []string{"/session"}},
		{
			"absolute -C",
			"git -C /other/repo commit -m x",
			"/session",
			"commit",
			[]string{"/other/repo"},
		},
		{"tilde -C", "git -C ~ push", "/session", "push", []string{home}},
		{
			"tilde path -C",
			"git -C ~/code/worklog push origin main",
			"/session",
			"push",
			[]string{home + "/code/worklog"},
		},
		{
			"HOME -C",
			"git -C $HOME/code/worklog push origin main",
			"/session",
			"push",
			[]string{home + "/code/worklog"},
		},
		{
			"braced HOME -C",
			"git -C ${HOME}/code/worklog push",
			"/session",
			"push",
			[]string{home + "/code/worklog"},
		},
		{
			"double-quoted HOME -C",
			`git -C "$HOME/code/worklog" push`,
			"/session",
			"push",
			[]string{home + "/code/worklog"},
		},
		{
			"double-quoted absolute -C",
			`git -C "/other/repo" push`,
			"/session",
			"push",
			[]string{"/other/repo"},
		},
		{
			"single-quoted HOME -C is literal",
			`git -C '$HOME/code' push`,
			"/session",
			"push",
			[]string{""},
		},
		{"quoted tilde -C is literal", `git -C "~/code" push`, "/session", "push", []string{""}},
		{
			"relative -C joins cwd",
			"git -C sub/repo commit",
			"/session",
			"commit",
			[]string{"/session/sub/repo"},
		},
		{
			"dot-dot -C stays as written for git",
			"git -C ../other commit",
			"/session/repo",
			"commit",
			[]string{"/session/repo/../other"},
		},
		{
			"dot-dot cd is logical",
			"cd ../other && git commit -m x",
			"/session/repo",
			"commit",
			[]string{"/session/other"},
		},
		{"variable -C is unknown", "git -C $TARGET push", "/session", "push", []string{""}},
		{"substitution -C is unknown", `git -C "$(pwd)" push`, "/session", "push", []string{""}},
		{"chained -C", "git -C /a -C b commit", "/session", "commit", []string{"/a/b"}},
		{
			"-C after -c",
			"git -c user.email=x@example.com -C ~/repo commit",
			"/session",
			"commit",
			[]string{home + "/repo"},
		},
		{
			"two commits",
			"git -C /a commit -m x && git commit -m y",
			"/session",
			"commit",
			[]string{"/a", "/session"},
		},
		{
			"cd then bare commit",
			"cd /other/repo && git commit -m x",
			"/session",
			"commit",
			[]string{"/other/repo"},
		},
		{
			"cd relative then bare commit",
			"cd repo && git commit -m x",
			"/session",
			"commit",
			[]string{"/session/repo"},
		},
		{
			"tilde cd then bare commit",
			"cd ~/.worktrees/repo/slug && git commit -m x",
			"/session",
			"commit",
			[]string{home + "/.worktrees/repo/slug"},
		},
		{
			"HOME cd then bare commit",
			"cd $HOME/repo && git commit -m x",
			"/session",
			"commit",
			[]string{home + "/repo"},
		},
		{
			"pushd then bare commit",
			"pushd /other/repo && git commit -m x",
			"/session",
			"commit",
			[]string{"/other/repo"},
		},
		{"bare cd goes home", "cd && git commit -m x", "/session", "commit", []string{home}},
		{
			"cd then relative -C",
			"cd /other && git -C repo commit -m x",
			"/session",
			"commit",
			[]string{"/other/repo"},
		},
		{
			"variable cd is unknown",
			"cd $TARGET && git commit -m x",
			"/session",
			"commit",
			[]string{""},
		},
		{"cd dash is unknown", "cd - && git commit -m x", "/session", "commit", []string{""}},
		{
			"absolute -C after variable cd",
			"cd $TARGET && git -C /other/repo commit -m x",
			"/session",
			"commit",
			[]string{"/other/repo"},
		},
		{
			"cd persists across segments",
			"cd /a; git commit -m x; git commit -m y",
			"/session",
			"commit",
			[]string{"/a", "/a"},
		},
		{
			"popd is unknown",
			"pushd /a && popd && git commit -m x",
			"/session",
			"commit",
			[]string{""},
		},
		{"bare pushd is unknown", "pushd && git commit -m x", "/session", "commit", []string{""}},
		{
			"popd then absolute -C is known",
			"pushd /a && popd && git -C /b commit -m x",
			"/session",
			"commit",
			[]string{"/b"},
		},
		{
			"--git-dir is unknown",
			"git --git-dir /r/.git commit -m x",
			"/session",
			"commit",
			[]string{""},
		},
		{
			"--work-tree= is unknown",
			"git --work-tree=/r commit -m x",
			"/session",
			"commit",
			[]string{""},
		},
		{
			"--git-dir then -C is still unknown",
			"git --git-dir=/r/.git -C /x commit -m x",
			"/session",
			"commit",
			[]string{""},
		},
		{
			"global flag before the subcommand",
			"git --no-pager commit -m x",
			"/session",
			"commit",
			[]string{"/session"},
		},
		{
			"short global flag before -C",
			"git -p -C /a commit -m x",
			"/session",
			"commit",
			[]string{"/a"},
		},
		{
			"value flag consumes its value",
			"git --namespace ns commit -m x",
			"/session",
			"commit",
			[]string{"/session"},
		},
		{
			"equals-form value flag",
			"git --config-env=user.email=MAIL commit -m x",
			"/session",
			"commit",
			[]string{"/session"},
		},
		{
			"value flag value is not the subcommand",
			"git --namespace commit status",
			"/session",
			"commit",
			nil,
		},
		{"cd -P is unknown", "cd -P /a && git commit -m x", "/session", "commit", []string{""}},
		{
			"cd -L stays logical",
			"cd -L /a && git commit -m x",
			"/session",
			"commit",
			[]string{"/a"},
		},
		{
			"cd -- then a target",
			"cd -- /a && git commit -m x",
			"/session",
			"commit",
			[]string{"/a"},
		},
		{
			"cd with output redirected",
			"cd /a >/dev/null 2>&1 && git commit -m x",
			"/session",
			"commit",
			[]string{"/a"},
		},
		{
			"quiet pushd",
			"pushd /a > /dev/null && git commit -m x",
			"/session",
			"commit",
			[]string{"/a"},
		},
		{
			"cd after && holds through the && chain",
			"git fetch && cd /a && git status && git commit -m x",
			"/session",
			"commit",
			[]string{"/a"},
		},
		{
			"a quote in a comment opens no string",
			"# don't push from here\ncd /a && git commit -m x",
			"/session",
			"commit",
			[]string{"/a"},
		},
		{
			"cd or exit holds after ;",
			"cd /a || exit 1; git commit -m x",
			"/session",
			"commit",
			[]string{"/a"},
		},
		{
			"CDPATH set, cd to a ./ path",
			"CDPATH=/p; cd ./repo && git commit -m x",
			"/session",
			"commit",
			[]string{"/session/repo"},
		},
		{
			"CDPATH set, absolute cd",
			"export CDPATH=/p; cd /a && git commit -m x",
			"/session",
			"commit",
			[]string{"/a"},
		},
		{
			"GIT_DIR prefix stays on its own command",
			"GIT_DIR=/r/.git git status; git commit -m x",
			"/session",
			"commit",
			[]string{"/session"},
		},
		{
			"other git env prefix",
			`GIT_SSH_COMMAND="ssh -i k" git commit -m x`,
			"/session",
			"commit",
			[]string{"/session"},
		},
		{"other subcommand ignored", "git -C /a status", "/session", "commit", nil},
		{"quoted mention ignored", `echo "git commit -m x"`, "/session", "commit", nil},
		{"no subcommand", "git -C /a", "/session", "commit", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := invocationDirs(gitCommandsAt(tt.command, tt.cwd, tt.sub))
			if !slices.Equal(got, tt.want) {
				t.Errorf(
					"gitCommandsAt(%q, %q, %q) dirs = %q, want %q",
					tt.command,
					tt.cwd,
					tt.sub,
					got,
					tt.want,
				)
			}
		})
	}
}

// TestGitCommandsAtLiteralVars pins the variable resolution of MAD-554: a
// variable the command sets once, to a plain literal, in a segment of its own
// before the use is substituted the way the shell would, in a cd and a `git
// -C` target alike. Anything a shell must evaluate (a substitution, another
// variable, a second assignment, a pipeline) leaves the directory unknown.
func TestGitCommandsAtLiteralVars(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	const home = "/Users/tester"
	tests := []struct {
		name    string
		command string
		want    string // dir of the one push; "" for unknown
	}{
		{"tilde literal -C", "S=~/x; git -C $S push", home + "/x"},
		{"braced use", "S=~/x; git -C ${S} push", home + "/x"},
		{"double-quoted use", `S=~/x; git -C "$S" push`, home + "/x"},
		{"double-quoted literal", `S="/abs/repo"; git -C $S push`, "/abs/repo"},
		{"single-quoted literal", `S='/abs/repo'; git -C $S push`, "/abs/repo"},
		{"HOME in the literal", "S=$HOME/x; git -C $S push", home + "/x"},
		{"path after the variable", "S=~/code; git -C $S/repo push", home + "/code/repo"},
		{"cd to the variable", "S=~/x && cd $S && git push", home + "/x"},
		{"relative literal joins the cd", "S=repo; cd /a && git -C $S push", "/a/repo"},
		{"two literals in one segment", "A=/a B=/b; git -C $B push", "/b"},
		{"newline separated", "S=/abs\ngit -C $S push", "/abs"},
		{
			"a commit and push sequence",
			`R=~/code/repo; git -C $R add .; git -C $R commit -S -m "fix: read the set"; ` +
				"git -C $R push",
			home + "/code/repo",
		},
		{"tests before the push", "S=~/x; cd $S && go test ./... && git push", home + "/x"},
		{"substitution", "S=$(pwd); git -C $S push", ""},
		{"backticks", "S=`pwd`; git -C $S push", ""},
		{"from another variable", "T=/a; S=$T; git -C $S push", ""},
		{"reassigned", "S=/a; S=/b; git -C $S push", ""},
		{"reassigned with the same value", "S=/a; S=/a; git -C $S push", ""},
		{"appended to", "S=/a; S+=/b; git -C $S push", ""},
		{"reassigned by read", "S=/a; read S; git -C $S push", ""},
		{"reassigned by export", "S=/a; export S=/b; git -C $S push", ""},
		{"env prefix sets nothing in the shell", "S=/a git -C $S push", ""},
		{"used before it is set", "git -C $S push; S=/a", ""},
		{"set in a pipeline", "S=/a | true; git -C $S push", ""},
		{"set after ||", "false || S=/a; git -C $S push", ""},
		{"single-quoted use is literal", "S=/a; git -C '$S' push", ""},
		{"quoted tilde literal", `S="~/x"; git -C $S push`, ""},
		{"glob in the literal", "S=/a/*; git -C $S push", ""},
		{"empty literal", `S=; git -C "$S" push`, ""},
		{"operator form", "S=/a; git -C ${S:-/b} push", ""},
		{"longer name", "S=/a; git -C $SS push", ""},
		{"name also a word in a path", "repo=~/code/repo; git -C $repo push", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := invocationDirs(gitCommandsAt(tt.command, "/session", "push"))
			if want := []string{tt.want}; !slices.Equal(got, want) {
				t.Errorf("gitCommandsAt(%q) dirs = %q, want %q", tt.command, got, want)
			}
		})
	}
}

// unvouchedVarCommands each end in a push from a directory that a variable
// names, where the shell may leave the variable empty or set it to something
// else before the push runs. An empty one sends `git -C ""` and `cd ""` to
// the cwd. The first group is from the MAD-554 review; the rest are further
// ways around reading the assignment as text. Every guard that walks
// segments must treat the directory as unknown.
var unvouchedVarCommands = []struct{ name, command string }{
	// The assignment may never run in this shell.
	{"short-circuit", `[ -d ~/feat ] && S=~/feat; git -C "$S" push`},
	{"if block", "if [ -d ~/feat ]; then\n  S=~/feat\nfi\ngit -C \"$S\" push"},
	{"heredoc body", "cat > /tmp/n <<'EOF'\nS=~/feat\nEOF\ngit -C \"$S\" push"},
	{"inside a quoted string", `echo "a; S=~/feat; b"; git -C "$S" push`},
	{"inside a comment", "# note; S=~/feat\ngit -C \"$S\" push"},
	{"subshell", "(\n  S=~/feat\n)\ngit -C \"$S\" push"},
	{"uncalled function body", "f() {\n  S=~/feat\n}\ngit -C \"$S\" push"},
	{"line continuation", "echo \\\nS=~/feat; git -C \"$S\" push"},
	{"zsh short-circuit before cd", `false && S=~/feat; cd "$S" && git push`},
	{"backgrounded", "S=~/feat&\ngit -C \"$S\" push"},
	{"no-break space joins two assignments", "X=1 S=~/feat; git -C \"$S\" push"},
	// Something after the assignment may set the variable again.
	{"eval with a quoted name", `S=~/feat; eval 'S'=/session; git -C "$S" push`},
	{"eval with an escaped =", `S=~/feat; eval S\=/session; git -C "$S" push`},
	{"unset", `S=~/feat; unset S; git -C "$S" push`},
	{"source", `S=~/feat; source /tmp/env; git -C "$S" push`},
	{"dot", `S=~/feat; . /tmp/env; git -C "$S" push`},
	{"bundled print -v", `REPO=~/feat; print -vREPO /session; git -C "$REPO" push`},
	{
		"command word built from variables",
		`A=RE; B=PO; C=pr; D=int; REPO=~/feat; $C$D -v $A$B /session; git -C "$REPO" push`,
	},
	{"command word from a glob", `A=RE; B=PO; REPO=~/feat; rea? $A$B </tmp/n; git -C "$REPO" push`},
	{
		"command word from an extended glob",
		`REPO=~/feat; prin^z -vREPO /session; git -C "$REPO" push`,
	},
	{"read sets REPLY", `REPLY=~/feat; read </tmp/n; git -C "$REPLY" push`},
	{"getopts sets OPTARG", `OPTARG=~/feat; getopts a: o -a /session; git -C "$OPTARG" push`},
	{"=~ sets MATCH", `MATCH=~/feat; [[ /session =~ .* ]]; git -C "$MATCH" push`},
	{"shift decrements a number", "S=5; shift -- --S; git -C ~/repo$S push"},
	{"shift divides a number", "S=8; shift -- S/=2; git -C ~/repo$S push"},
	{"cd rewrites PWD", `PWD=~/feat; cd /session; git -C "$PWD" push`},
	{"every command rewrites _", `_=~/feat; git -C "$_" push`},
	{"HOME changes what ~ means", `HOME=/session; S=~/feat; git -C "$S" push`},
	// The reference does not expand to the plain value.
	{"zsh modifier", "S=~/main/sub; git -C $S:h push"},
	{"zsh modifier after braces", "S=~/main/sub; git -C ${S}:h push"},
	{"zsh subscript", "S=~/feat; git -C $S[1] push"},
	{"tilde before the reference", "S=/feat; git -C ~$S push"},
}

// TestGitCommandsAtUnvouchedVars pins the resolver side of the MAD-554
// review: no variable in unvouchedVarCommands resolves, so the push
// directory is unknown.
func TestGitCommandsAtUnvouchedVars(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	for _, tt := range unvouchedVarCommands {
		t.Run(tt.name, func(t *testing.T) {
			got := invocationDirs(gitCommandsAt(tt.command, "/session", "push"))
			if want := []string{""}; !slices.Equal(got, want) {
				t.Errorf("gitCommandsAt(%q) dirs = %q, want %q", tt.command, got, want)
			}
		})
	}
}

// TestUnvouchedVarsReachEveryWalker pins finding 4 of the MAD-554 review:
// the resolution sits in walkSegments, so every guard that walks segments
// gets the same answer. git-discard, with only the session cwd dirty, denies
// each unvouched command with its push swapped for a reset, and still allows
// a reset in a clean directory a plain variable names. The gh walk that
// publish-internal-names shares (GhCalls) places a call after `cd "$S"`
// nowhere.
func TestUnvouchedVarsReachEveryWalker(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	var warned []string
	discard := newDiscardGuard(
		config.Config{},
		discardFixture{repo: "github.com/mad01/thismoon", dirty: map[string]bool{"/session": true}},
		&warned,
	)
	for _, tt := range unvouchedVarCommands {
		t.Run(tt.name, func(t *testing.T) {
			command := strings.TrimSuffix(tt.command, "push") + "reset --hard"
			if d := discard.Check(Input{Event: EventBash, Command: command, Cwd: "/session"}); d == nil {
				t.Errorf("git-discard allowed %q", command)
			}
		})
	}
	plain := `S=/clean; git -C "$S" reset --hard`
	if d := discard.Check(Input{Event: EventBash, Command: plain, Cwd: "/session"}); d != nil {
		t.Errorf("git-discard denied %q: %v", plain, d)
	}
	calls := GhCalls(`[ -d /x ] && S=/x; cd "$S" && gh pr create --body b`, "/session")
	if len(calls) != 1 || calls[0].Dir != "" {
		t.Errorf("GhCalls dirs = %+v, want one call with an unknown dir", calls)
	}
}

// shellDirGaps (MAD-569) each end in a push from /session that the walk once
// placed in decoy, a directory zsh or bash does not run it in. decoy is ""
// where the push was not seen at all. want is where the walk places it now:
// the directory the shell really uses, or "" for unknown. The shell
// behaviour behind each row was checked in zsh 5.9 and bash 5.3.
var shellDirGaps = []struct{ name, command, decoy, want string }{
	// Gap 1: the directory stack, and flags read as a target.
	{"cd +1 is a zsh stack entry", "cd +1; git push", "/session/+1", ""},
	{"cd -1 is a zsh stack entry", "cd -1; git push", "/session/-1", ""},
	{"cd +1 after two pushd", "pushd /a && pushd /b && cd +1 && git push", "/b/+1", ""},
	{"cd -1 after two pushd", "pushd /a && pushd /b && cd -1 && git push", "/b/-1", ""},
	{"pushd +1 rotates the stack", "pushd /a && pushd /b && pushd +1 && git push", "/b/+1", ""},
	{"cd -- -1 is still the stack in zsh", "cd -- -1; git push", "/session/--", ""},
	{"cd -q is a zsh flag", "cd -q /x && git push", "/session/-q", ""},
	{"combined cd flags", "cd -LP /x && git push", "/session/-LP", ""},
	// Gap 2: cd spelled another way, and words before it.
	{"chdir is cd in zsh only", "chdir /x && git push", "/session", ""},
	{"builtin cd", "builtin cd /x && git push", "/session", "/x"},
	{"builtin pushd", "builtin pushd /x && git push", "/session", "/x"},
	{"builtin chdir", "builtin chdir /x && git push", "/session", ""},
	{"command cd is the builtin in bash", "command cd /x; git push", "/session", ""},
	{"command pushd is the builtin in bash", "command pushd /x; git push", "/session", ""},
	{"noglob cd", "noglob cd /x && git push", "/session", ""},
	{"nocorrect cd", "nocorrect cd /x && git push", "/session", ""},
	{"dash precommand cd", "- cd /x && git push", "/session", ""},
	{"assignment before cd", "X=1 cd /x && git push", "/session", ""},
	{"HOME before a bare cd", "HOME=/x cd && git push", "/session", ""},
	{"time cd", "time cd /x && git push", "/session", ""},
	{"cd in a brace group", "{ cd /x; }; git push", "/session", ""},
	{"cd in an if block", "if true; then cd /x; fi; git push", "/session", ""},
	{"negated cd", "! cd /x; git push", "/session", ""},
	{"cd at the end of a pipeline is a bash subshell", "true | cd /x; git push", "/x", ""},
	{"the .. alias", ".. && git push", "/session", ""},
	// Gap 3: a space strings.Fields splits at and the shell does not.
	{"no-break space after cd", "cd /x; git push", "/x", ""},
	{"no-break space after the target", "cd /x ; git push", "/x", ""},
	{"em space after cd", "cd /x; git push", "/x", ""},
	{"ideographic space after cd", "cd　/x; git push", "/x", ""},
	{"next-line character after cd", "cd\u0085/x; git push", "/x", ""},
	{"vertical tab after cd", "cd\v/x; git push", "/x", ""},
	{"form feed after cd", "cd\f/x; git push", "/x", ""},
	{"carriage return after the target", "cd /x\r; git push", "/x", ""},
	// Gap 4: git told to use another repository.
	{"GIT_DIR prefix", "GIT_DIR=/x/.git git push", "/session", ""},
	{"GIT_WORK_TREE prefix", "GIT_WORK_TREE=/x git push", "/session", ""},
	{"GIT_COMMON_DIR prefix", "GIT_COMMON_DIR=/x/.git git push", "/session", ""},
	{"GIT_DIR through env", "env GIT_DIR=/x/.git git push", "/session", ""},
	{"exported GIT_DIR", "export GIT_DIR=/x/.git && git push", "/session", ""},
	{"assigned GIT_DIR", "GIT_DIR=/x/.git; git push", "/session", ""},
	{"exported GIT_WORK_TREE before a cd", "export GIT_WORK_TREE=/x; cd /y && git push", "/y", ""},
	{"env -C", "env -C /x git push", "/session", ""},
	{"env --chdir", "env --chdir=/x git push", "/session", ""},
	// Gap 5: a CDPATH search.
	{"CDPATH assigned", "CDPATH=/p; cd repo && git push", "/session/repo", ""},
	{"CDPATH exported", "export CDPATH=/p && cd repo && git push", "/session/repo", ""},
	{"CDPATH prefix on cd", "CDPATH=/p cd repo && git push", "/session", ""},
	{"zsh cdpath array", "cdpath=(/p); cd repo && git push", "/session/repo", ""},
	{"CDPATH and pushd", "CDPATH=/p; pushd repo && git push", "/session/repo", ""},
	// Neighbours: a cd the shell may not run in this shell, or at all.
	{"cd backgrounded before the push", "cd /x & git push", "", ""},
	{"cd backgrounded, glued", "cd /x& git push", "", ""},
	{"cd piped into another command", "cd /x | cat; git push", "/x", ""},
	{"cd after a test that may fail", "false && cd /x; git push", "/x", ""},
	{"cd after ||", "true || cd /x; git push", "/x", ""},
	{"zsh two-argument cd", "cd /x/only && cd only repo && git push", "/x/only/only", ""},
	{"cd in a heredoc body", "cat > /tmp/n <<'EOF'\ncd /x\nEOF\ngit push", "/x", ""},
	// The quote-aware reader (MAD-570) reads the string as one echo word, so
	// the push is placed where the shell runs it.
	{"cd in a quoted string", "echo \"a\ncd /x\nb\"; git push", "/x", "/session"},
	{"cd in a comment", "# a; cd /x\ngit push", "/x", ""},
}

// TestGitCommandsAtShellDirGaps pins the walk side of MAD-569: each push in
// shellDirGaps is placed where the shell runs it, or nowhere.
func TestGitCommandsAtShellDirGaps(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	for _, tt := range shellDirGaps {
		t.Run(tt.name, func(t *testing.T) {
			got := invocationDirs(gitCommandsAt(tt.command, "/session", "push"))
			if want := []string{tt.want}; !slices.Equal(got, want) {
				t.Errorf("gitCommandsAt(%q) dirs = %q, want %q", tt.command, got, want)
			}
		})
	}
}

// TestShellDirGapsReachEveryWalker runs shellDirGaps through git-discard,
// with the push swapped for a reset: every directory is dirty but the decoy,
// so a reset judged in the decoy is the only way through.
func TestShellDirGapsReachEveryWalker(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	for _, tt := range shellDirGaps {
		t.Run(tt.name, func(t *testing.T) {
			discard := NewGitDiscard(config.Config{})
			discard.resolveRepo = func(string) string { return "github.com/mad01/thismoon" }
			discard.emit = func(_, _, _, _ string, _ map[string]string) {}
			discard.status = func(dir string, _ ...string) (string, bool) {
				if dir == tt.decoy {
					return "", true
				}
				return " M file.go", true
			}
			command := strings.TrimSuffix(tt.command, "push") + "reset --hard"
			if d := discard.Check(Input{Event: EventBash, Command: command, Cwd: "/session"}); d == nil {
				t.Errorf("git-discard allowed %q", command)
			}
		})
	}
}

// TestCdpathFromEnvironment pins the CDPATH belt inherits: when it is set, a
// relative cd target the shell would search it for is unknown, and targets
// it never searches (absolute, ./ or ../ led) still resolve.
func TestCdpathFromEnvironment(t *testing.T) {
	tests := []struct{ cdpath, command, want string }{
		{"/p", "cd repo && git push", ""},
		{"/p", "pushd repo && git push", ""},
		{"/p", "cd ./repo && git push", "/session/repo"},
		{"/p", "cd ../x && git push", "/x"},
		{"/p", "cd /a && git push", "/a"},
		{"", "cd repo && git push", "/session/repo"},
	}
	for _, tt := range tests {
		t.Setenv("CDPATH", tt.cdpath)
		got := invocationDirs(gitCommandsAt(tt.command, "/session", "push"))
		if want := []string{tt.want}; !slices.Equal(got, want) {
			t.Errorf(
				"CDPATH=%q gitCommandsAt(%q) dirs = %q, want %q",
				tt.cdpath,
				tt.command,
				got,
				want,
			)
		}
	}
}

// TestResolveDir covers the static directory resolution on its own: what the
// shell would expand, belt expands the same way; what needs a shell is
// unknown.
func TestResolveDir(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	tests := []struct {
		cwd, target, want string
	}{
		{"/work", "/abs/dir", "/abs/dir"},
		{"/work", "sub/dir", "/work/sub/dir"},
		{"/work", "..", "/"},
		{"", "sub", ""},
		{"/work", "~", "/Users/tester"},
		{"/work", "~/code", "/Users/tester/code"},
		{"/work", "$HOME", "/Users/tester"},
		{"/work", "${HOME}/code", "/Users/tester/code"},
		{"/work", `"$HOME/code"`, "/Users/tester/code"},
		{"/work", `"/abs/dir"`, "/abs/dir"},
		{"/work", `'/abs/dir'`, "/abs/dir"},
		{"/work", `'$HOME/code'`, ""}, // single quotes expand nothing
		{"/work", `"~/code"`, ""},     // a quoted tilde is literal
		{"/work", "~other/code", ""},  // another user's home
		{"/work", "$HOMEBREW_PREFIX/x", ""},
		{"/work", "$TARGET", ""},
		{"/work", `"$(pwd)"`, ""},
		{"/work", "`pwd`", ""},
		{"/work", "-", ""},
		{"/work", "*/dir", ""},
		{"/work", `"/unbalanced`, ""},
	}
	for _, tt := range tests {
		if got := resolveDir(tt.cwd, tt.target); got != tt.want {
			t.Errorf("resolveDir(%q, %q) = %q, want %q", tt.cwd, tt.target, got, tt.want)
		}
	}
}

// TestResolveDirWithoutHome pins that a tilde stays unknown when there is no
// home directory to expand it to, rather than becoming a relative path.
func TestResolveDirWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	for _, target := range []string{"~", "~/code", "$HOME/code"} {
		if got := resolveDir("/work", target); got != "" {
			t.Errorf("resolveDir(%q) without HOME = %q, want unknown", target, got)
		}
	}
}

// TestPlaceDir pins the uncleaned placement a `git -C` target gets: the path
// reaches git as typed, joined onto the tracked directory when relative, so
// the OS resolves `..` physically the way git's chdir does.
func TestPlaceDir(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")
	tests := []struct {
		cwd, target, want string
	}{
		{"/work", "..", "/work/.."},
		{"/work", "link/../repo", "/work/link/../repo"},
		{"/", "repo", "/repo"},
		{"/work", "/abs/../dir", "/abs/../dir"},
		{"/work", "~/code/..", "/Users/tester/code/.."},
		{"", "repo", ""},
		{"/work", "-", ""},
		{"/work", "$TARGET", ""},
	}
	for _, tt := range tests {
		if got := placeDir(tt.cwd, tt.target); got != tt.want {
			t.Errorf("placeDir(%q, %q) = %q, want %q", tt.cwd, tt.target, got, tt.want)
		}
	}
}
