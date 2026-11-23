package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fsmonitorPushBodies are heredoc bodies with a push the token walk misses:
// one eval builds, and one with a split word. Read as code, both push.
var fsmonitorPushBodies = []string{
	`eval "git pu""sh origin main"`,
	"git pu''sh origin main",
}

// isolateFsmonitor drops the answers repoFsmonitor cached, now and when the
// test ends, so a test that sets git config in its environment reads its
// own answers and leaves none behind for a later test. TestMain already
// keeps the machine's git config out (isolateGitConfig).
func isolateFsmonitor(t *testing.T) {
	t.Helper()
	resetFsmonitorLookups()
	t.Cleanup(resetFsmonitorLookups)
}

// resetFsmonitorLookups drops every answer repoFsmonitor cached.
func resetFsmonitorLookups() {
	fsmonitorLookups.Lock()
	defer fsmonitorLookups.Unlock()
	clear(fsmonitorLookups.byDir)
}

// fsmonitorRepo makes a git repo in a fresh directory, so no answer belt
// cached for another row applies, with core.fsmonitor set to value, or
// unset for "".
func fsmonitorRepo(t *testing.T, value string) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	if value != "" {
		gitIn(t, dir, "config", "core.fsmonitor", value)
	}
	return dir
}

// fsmonitorRow is a git message heredoc run from cwd: head is the command
// up to the line after `<<'EOF2'`, and deny says whether the body is code.
type fsmonitorRow struct {
	name, cwd, head string
	deny            bool
}

// heredoc closes head with body as its heredoc.
func (r fsmonitorRow) heredoc(body string) string {
	return r.head + "\n" + body + "\nEOF2"
}

// TestFsmonitorCommitBodyIsCode pins that a git commit or merge heredoc is
// code when core.fsmonitor may name a command: git runs that command while
// it refreshes the index, with the heredoc on its standard input. The same
// command may set it, git may read it from the repo's config, or belt may
// not be able to tell, which fails closed. A git boolean, an unset key, and
// a git tag body keep the prose reading, which misses these pushes.
func TestFsmonitorCommitBodyIsCode(t *testing.T) {
	isolateFsmonitor(t)
	rows := []fsmonitorRow{
		{"git config", "", "git config core.fsmonitor 'sh -s' && git commit -m x <<'EOF2'", true},
		{"git config --local", "", "git config --local core.fsmonitor 'sh -s' && git commit -F - <<'EOF2'", true},
		{"git config --global", "", "git config --global core.fsmonitor 'sh -s'; git commit -F - <<'EOF2'", true},
		{
			"git config --file",
			"",
			"git config --file .git/config core.fsmonitor 'sh -s' && git commit -F - <<'EOF2'",
			true,
		},
		{"git config --worktree", "", "git config --worktree core.fsmonitor 'sh -s' && git commit <<'EOF2'", true},
		{"git config set", "", "git config set core.fsmonitor 'sh -s' && git commit -F - <<'EOF2'", true},
		{"git config, key in any case", "", "git config Core.FSMonitor 'sh -s' && git commit -F - <<'EOF2'", true},
		{"git config, a variable value", "", `git config core.fsmonitor "$HOOK" && git commit -F - <<'EOF2'`, true},
		{"git config, a variable key", "", `git config "$KEY" 'sh -s' && git commit -F - <<'EOF2'`, true},
		{"git config, an include", "", "git config include.path /tmp/x.inc && git commit -F - <<'EOF2'", true},
		{"git config inside sh -c", "", `sh -c "git config core.fsmonitor 'sh -s'"; git commit -F - <<'EOF2'`, true},
		{"git config before a merge -F -", "", "git config core.fsmonitor 'sh -s'; git merge -F - side <<'EOF2'", true},
		{"git -c", "", "git -c core.fsmonitor='sh -s' commit -F - <<'EOF2'", true},
		{"git -c, a variable", "", `git -c core.fsmonitor="$HOOK" commit -F - <<'EOF2'`, true},
		{"git -c, an include", "", "git -c include.path=/tmp/x.inc commit -F - <<'EOF2'", true},
		{"git --config-env", "", "git --config-env=core.fsmonitor=HOOK commit -F - <<'EOF2'", true},
		{"git -c on merge --file=-", "", "git -c core.fsmonitor='sh -s' merge --file=- side <<'EOF2'", true},
		{
			"GIT_CONFIG_COUNT in front",
			"",
			"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.fsmonitor GIT_CONFIG_VALUE_0='sh -s' git commit -F - <<'EOF2'",
			true,
		},
		{
			"GIT_CONFIG_COUNT exported",
			"",
			"export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.fsmonitor GIT_CONFIG_VALUE_0='sh -s'\n" +
				"git commit -F - <<'EOF2'",
			true,
		},
		{"GIT_CONFIG_VALUE alone", "", "GIT_CONFIG_VALUE_0='sh -s' git commit -F - <<'EOF2'", true},
		{
			"GIT_CONFIG_PARAMETERS in front",
			"",
			`GIT_CONFIG_PARAMETERS="'core.fsmonitor'='sh -s'" git commit -F - <<'EOF2'`,
			true,
		},
		{
			"GIT_CONFIG_PARAMETERS through env",
			"",
			`env GIT_CONFIG_PARAMETERS="'core.fsmonitor'='sh -s'" git commit -F - <<'EOF2'`,
			true,
		},
		{"GIT_TEST_FSMONITOR in front", "", "GIT_TEST_FSMONITOR='sh -s' git commit -F - <<'EOF2'", true},
		{"GIT_CONFIG_GLOBAL in front", "", "GIT_CONFIG_GLOBAL=/tmp/g git commit -F - <<'EOF2'", true},
		{"a directory belt cannot place", "", `cd "$D" && git commit -F - <<'EOF2'`, true},
		{"a cd that may not have run", "", "cd /nonexistent-belt-dir; git commit -F - <<'EOF2'", true},
		{"repo config", "sh -s", "git commit -F - <<'EOF2'", true},
		{"repo config, -m", "sh -s", "git commit --allow-empty -m x <<'EOF2'", true},
		{"repo config, a hook path", ".git/hooks/fsmonitor-watchman", "git commit -F - <<'EOF2'", true},
		{"repo config, merge -F -", "sh -s", "git merge -F - side <<'EOF2'", true},
		{"repo config, the value 2", "2", "git commit -F - <<'EOF2'", true},
		{"unset", "", "git commit -F - <<'EOF2'", false},
		{"repo true", "true", "git commit -F - <<'EOF2'", false},
		{"repo false", "false", "git commit -F - <<'EOF2'", false},
		{"repo off", "OFF", "git commit -F - <<'EOF2'", false},
		{"git -c false over a repo command", "sh -s", "git -c core.fsmonitor=false commit -F - <<'EOF2'", false},
		{"git -c with no value is true", "sh -s", "git -c core.fsmonitor commit -F - <<'EOF2'", false},
		{"git config true", "", "git config core.fsmonitor true && git commit -F - <<'EOF2'", false},
		{"git -c another key", "", "git -c user.name=x commit -F - <<'EOF2'", false},
		{
			"GIT_CONFIG_COUNT of another key",
			"",
			"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=user.name GIT_CONFIG_VALUE_0=x git commit -F - <<'EOF2'",
			false,
		},
		{
			"GIT_CONFIG_COUNT of fsmonitor false",
			"",
			"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.fsmonitor GIT_CONFIG_VALUE_0=false git commit -F - <<'EOF2'",
			false,
		},
		{"a git config read", "", "git config --get core.fsmonitor && git commit -F - <<'EOF2'", false},
		{"tag with a repo command", "sh -s", "git tag -a v1 -F - <<'EOF2'", false},
		{
			"a message that names a config write",
			"",
			"git commit -F - <<'EOF3'\ngit config core.fsmonitor 'sh -s'\nEOF3\ngit commit -F - <<'EOF2'",
			false,
		},
	}
	for _, tt := range rows {
		for _, body := range fsmonitorPushBodies {
			command := tt.heredoc(body)
			cwd := fsmonitorRepo(t, tt.cwd)
			d := newGitPushGuard("feature").Check(Input{Event: EventBash, Command: command, Cwd: cwd})
			if (d != nil) != tt.deny {
				t.Errorf("%s: git-push-main Check(%q) = %v, want deny %v", tt.name, command, d, tt.deny)
			}
		}
	}
}

// hookC is the -c option that makes a git call's core.fsmonitor a command.
const hookC = "git -c core.fsmonitor='sh -s' "

// TestFsmonitorCommitStdinIsCode pins that the standard input of a git
// commit or merge that may run a core.fsmonitor command is code, wherever it
// comes from: a here-string, and what echo, printf, a cat of a heredoc or a
// file the command wrote hands it, are walked as code. A < file belt did
// not see written and a pipe from a command belt cannot read are unread, so
// the push guard denies them. With no command that may read it, every form
// reads as before and passes. In each form BODY stands for the body,
// quoted with ' where the form quotes it.
func TestFsmonitorCommitStdinIsCode(t *testing.T) {
	isolateFsmonitor(t)
	rows := []struct {
		name, repo, command string
		deny                bool
	}{
		{"echo into -c", "", "echo 'BODY' | " + hookC + "commit -F -", true},
		{"printf into -c", "", `printf '%s\n' 'BODY' | ` + hookC + "commit -F -", true},
		{"echo through cat", "", "echo 'BODY' | cat | " + hookC + "commit -F -", true},
		{"echo into -c with -m", "", "echo 'BODY' | " + hookC + "commit -m x", true},
		{"a cat heredoc into -c", "", "cat <<'EOF2' | " + hookC + "commit -F -\nBODY\nEOF2", true},
		{"a here-string on -c", "", hookC + "commit -F - <<< 'BODY'", true},
		{"a here-string on a cat into -c", "", "cat <<< 'BODY' | " + hookC + "commit -F -", true},
		{"a here-string on merge -c", "", hookC + "merge -F - side <<< 'BODY'", true},
		{"a < <( ) echo on -c", "", hookC + "commit -F - < <(echo 'BODY')", true},
		{"a written file on -c", "", `printf '%s\n' 'BODY' > m.txt && ` + hookC + "commit -F - < m.txt", true},
		{"an unread file on -c", "", hookC + "commit -F - < msg.txt", true},
		{"an unread 0< file on -c", "", hookC + "commit -F - 0< msg.txt", true},
		{"an unread pipe into -c", "", "git log -1 --format=%B | " + hookC + "commit -F -", true},
		{"a here-string after /dev/null on -c", "", hookC + "commit -m x < /dev/null <<< 'BODY'", true},
		{"echo into a repo command", "sh -s", "echo 'BODY' | git commit -F -", true},
		{"a here-string in a repo command", "sh -s", "git commit -F - <<< 'BODY'", true},
		{"an unread file in a repo command", "sh -s", "git commit -F - < msg.txt", true},
		{"echo, unset", "", "echo 'BODY' | git commit -F -", false},
		{"printf, unset", "", `printf '%s\n' 'BODY' | git commit -F -`, false},
		{"a here-string, unset", "", "git commit -F - <<< 'BODY'", false},
		{"a here-string on merge, unset", "", "git merge -F - side <<< 'BODY'", false},
		{"a < <( ) echo, unset", "", "git commit -F - < <(echo 'BODY')", false},
		{"an unread file, unset", "", "git commit -F - < msg.txt", false},
		{"an unread pipe, unset", "", "git log -1 --format=%B | git commit -F -", false},
		{"a < /dev/null on -c", "", hookC + "commit -m x < /dev/null", false},
		{"/dev/null after a here-string on -c", "", hookC + "commit -m x <<< 'BODY' < /dev/null", false},
		{"echo, repo true", "true", "echo 'BODY' | git commit -F -", false},
		{"echo into -c false", "sh -s", "echo 'BODY' | git -c core.fsmonitor=false commit -F -", false},
		{"a here-string into a tag, repo command", "sh -s", "git tag -a v1 -F - <<< 'BODY'", false},
		{"echo into a tag, repo command", "sh -s", "echo 'BODY' | git tag -a v1 -F -", false},
	}
	for _, tt := range rows {
		cwd := fsmonitorRepo(t, tt.repo)
		for _, body := range fsmonitorPushBodies {
			command := strings.ReplaceAll(tt.command, "BODY", body)
			d := newGitPushGuard("feature").Check(Input{Event: EventBash, Command: command, Cwd: cwd})
			if (d != nil) != tt.deny {
				t.Errorf("%s: git-push-main Check(%q) = %v, want deny %v", tt.name, command, d, tt.deny)
			}
		}
	}
}

// TestFsmonitorStdinGuardsReadTheBody pins that git-discard and file-discard
// read a hooked commit's here-string and piped text as code, as they read
// its heredoc (TestFsmonitorGuardsReadTheBody), and pass both with
// core.fsmonitor unset.
func TestFsmonitorStdinGuardsReadTheBody(t *testing.T) {
	isolateFsmonitor(t)
	home := t.TempDir()
	writeTree(t, home, map[string]string{"notes/a.md": "x"})
	files := newFileDiscardGuard(t, home)
	bodies := []struct {
		guard Guard
		body  string
	}{
		{dirtyDiscardGuard(), `eval "git reset --ha""rd"`},
		{files, "rm -rf " + filepath.Join(home, "notes")},
	}
	for _, value := range []string{"", "sh -s"} {
		deny := value != ""
		for _, b := range bodies {
			for _, command := range []string{
				"git commit -F - <<< '" + b.body + "'",
				"echo '" + b.body + "' | git commit -F -",
			} {
				d := b.guard.Check(Input{Event: EventBash, Command: command, Cwd: fsmonitorRepo(t, value)})
				if (d != nil) != deny {
					t.Errorf("core.fsmonitor=%q: %s Check(%q) = %v, want deny %v", value, b.guard.ID(), command, d, deny)
				}
			}
		}
	}
}

// TestFsmonitorConfigFromGit pins the config belt reads by asking git in
// the directory the commit runs in: global and included config, a git -C
// or cd target, and GIT_TEST_FSMONITOR, which git reads when the key is
// unset.
func TestFsmonitorConfigFromGit(t *testing.T) {
	body := fsmonitorPushBodies[0]
	commit := "git commit -F - <<'EOF2'\n" + body + "\nEOF2"
	check := func(t *testing.T, command, cwd string, deny bool) {
		t.Helper()
		d := newGitPushGuard("feature").Check(Input{Event: EventBash, Command: command, Cwd: cwd})
		if (d != nil) != deny {
			t.Errorf("git-push-main Check(%q) from %s = %v, want deny %v", command, cwd, d, deny)
		}
	}
	t.Run("global config", func(t *testing.T) {
		isolateFsmonitor(t)
		global := filepath.Join(t.TempDir(), "gitconfig")
		if err := os.WriteFile(global, []byte("[core]\n\tfsmonitor = sh -s\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GIT_CONFIG_GLOBAL", global)
		check(t, commit, fsmonitorRepo(t, ""), true)
	})
	t.Run("included config", func(t *testing.T) {
		isolateFsmonitor(t)
		inc := filepath.Join(t.TempDir(), "fsmonitor.inc")
		if err := os.WriteFile(inc, []byte("[core]\n\tfsmonitor = sh -s\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		repo := fsmonitorRepo(t, "")
		gitIn(t, repo, "config", "include.path", inc)
		check(t, commit, repo, true)
	})
	t.Run("git -C and cd targets", func(t *testing.T) {
		isolateFsmonitor(t)
		repo := fsmonitorRepo(t, "sh -s")
		other := fsmonitorRepo(t, "")
		check(t, "git -C "+repo+" "+strings.TrimPrefix(commit, "git "), other, true)
		check(t, "cd "+repo+" && "+commit, fsmonitorRepo(t, ""), true)
		check(t, "git -C "+fsmonitorRepo(t, "")+" "+strings.TrimPrefix(commit, "git "), repo, false)
	})
	t.Run("GIT_TEST_FSMONITOR in belt's environment", func(t *testing.T) {
		isolateFsmonitor(t)
		t.Setenv("GIT_TEST_FSMONITOR", "sh -s")
		check(t, commit, fsmonitorRepo(t, ""), true)
		check(t, commit, fsmonitorRepo(t, "false"), false)
	})
}

// TestFsmonitorGuardsReadTheBody pins that every guard that read a commit
// heredoc as a message reads it as code when core.fsmonitor names a
// command: git-discard sees a discard eval builds, file-discard a delete
// named in plain words, and script-deny-list a -c script it once masked.
// With the key unset, true or false, each body still passes.
func TestFsmonitorGuardsReadTheBody(t *testing.T) {
	isolateFsmonitor(t)
	home := t.TempDir()
	writeTree(t, home, map[string]string{"notes/a.md": "x"})
	files := newFileDiscardGuard(t, home)
	scripts := NewScriptDenyList(scriptDenyConfig([]string{"kubectl delete"}, nil))
	bodies := []struct {
		guard Guard
		body  string
	}{
		{dirtyDiscardGuard(), `eval "git reset --ha""rd"`},
		{files, "rm -rf " + filepath.Join(home, "notes")},
		{scripts, "sh -c 'kubectl delete pod x'"},
	}
	for _, value := range []string{"", "true", "false", "sh -s"} {
		deny := value == "sh -s"
		for _, b := range bodies {
			command := "git commit -F - <<'EOF2'\nfix: so\n" + b.body + "\nEOF2"
			d := b.guard.Check(Input{Event: EventBash, Command: command, Cwd: fsmonitorRepo(t, value)})
			if (d != nil) != deny {
				t.Errorf("core.fsmonitor=%q: %s Check(%q) = %v, want deny %v", value, b.guard.ID(), command, d, deny)
			}
		}
	}
}

// TestFsmonitorMessagesStayPlain pins the plain message shapes that pass
// with core.fsmonitor unset, from a repo and from a directory outside one.
func TestFsmonitorMessagesStayPlain(t *testing.T) {
	isolateFsmonitor(t)
	for _, cwd := range []string{fsmonitorRepo(t, ""), t.TempDir()} {
		for _, command := range []string{
			"git commit -F - <<'EOF2'\nfix(belt): read `x` and \"y\"\n\nIt's `z`.\nEOF2",
			"git commit -F - <<'EOF2'\n## What's new\n1) a fix for `git checkout -- f`\nEOF2",
			"git add . && git commit -q -F - <<'EOF2'\nfix: rm -rf build no more\nEOF2",
		} {
			for _, g := range []Guard{newGitPushGuard("feature"), dirtyDiscardGuard()} {
				if d := g.Check(Input{Event: EventBash, Command: command, Cwd: cwd}); d != nil {
					t.Errorf("%s denied %q from %s: %v", g.ID(), command, cwd, d)
				}
			}
		}
	}
}
