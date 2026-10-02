package guard

import (
	"slices"
	"testing"
)

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
