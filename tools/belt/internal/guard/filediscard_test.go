package guard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// newFileDiscardGuard builds the guard with home as the home directory, a
// toss-bin that is always found, and git seams that fail the test: a row
// outside every work tree must never reach git.
func newFileDiscardGuard(t *testing.T, home string) *FileDiscard {
	t.Helper()
	t.Setenv("HOME", home)
	g := NewFileDiscard(config.Config{})
	g.home = func() string { return home }
	g.lookPath = func(string) (string, error) { return "/usr/local/bin/toss-bin", nil }
	g.status = func(context.Context, string, []string) ([]byte, error) {
		t.Fatal("git status ran outside a work tree")
		return nil, nil
	}
	g.tracked = func(context.Context, string, []string) ([]byte, error) {
		t.Fatal("git ls-files ran outside a work tree")
		return nil, nil
	}
	g.prove = func(context.Context, string, []string) error {
		t.Fatal("the proof ran outside a work tree")
		return nil
	}
	g.worktrees = func(context.Context, string) ([]string, error) {
		t.Fatal("git worktree list ran outside a work tree")
		return nil, nil
	}
	g.keeps = func(context.Context, string) error { // runs off the test goroutine
		t.Error("a nested repository was judged outside a work tree")
		return nil
	}
	g.emit = func(string, string, string, string, map[string]string) {
		t.Fatal("emit called in hard mode")
	}
	return g
}

// writeTree writes each file under dir, "" content for an empty file and a
// trailing / for an empty directory.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// tempFixture makes a folder under /tmp, a temp root, removed after the test.
func tempFixture(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "belt-fd-")
	if err != nil {
		t.Skipf("no /tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// fileDiscardRow is one command run from cwd, where `~` stands for the
// test home; want lists what the reason holds, and nil means a pass.
type fileDiscardRow struct {
	name, cwd, command string
	want, not          []string
}

func runFileDiscardRows(t *testing.T, g *FileDiscard, home string, rows []fileDiscardRow) {
	t.Helper()
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			cwd := tt.cwd
			if rest, ok := strings.CutPrefix(cwd, "~"); ok {
				cwd = home + rest
			}
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: cwd})
			switch {
			case tt.want == nil && d != nil:
				t.Fatalf("Check(%q) = %q, want a pass", tt.command, d.Reason)
			case tt.want == nil:
				return
			case d == nil:
				t.Fatalf("Check(%q) passed, want a deny holding %q", tt.command, tt.want)
			}
			if !strings.HasPrefix(d.Reason, "belt[file-discard]: ") {
				t.Errorf("reason %q lacks the guard prefix", d.Reason)
			}
			for _, w := range tt.want {
				if !strings.Contains(d.Reason, w) {
					t.Errorf("reason %q does not hold %q", d.Reason, w)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(d.Reason, n) {
					t.Errorf("reason %q holds %q", d.Reason, n)
				}
			}
		})
	}
}

// TestFileDiscardOutsideWorkTrees covers every row that needs no git: the
// home directory and the root, words belt cannot resolve, the cheap passes,
// the wrapped and nested forms, and the deletes outside any work tree.
func TestFileDiscardOutsideWorkTrees(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{
		"x/f": "x", "notes.txt": "n", "zero": "", "empty/": "",
	})
	if err := os.Symlink(filepath.Join(home, "notes.txt"), filepath.Join(home, "ln")); err != nil {
		t.Fatal(err)
	}
	tmp := tempFixture(t)
	if err := os.Symlink(home, filepath.Join(tmp, "link")); err != nil {
		t.Fatal(err)
	}
	writeTree(t, tmp, map[string]string{"real/a/f": "f"})
	userOnly := []string{"the home directory", "only the user may run it, with `! <command>`"}
	outside := []string{"in no git work tree and no temp folder"}
	rows := []fileDiscardRow{
		// The home directory and the root are the user's alone.
		{"tilde", "~", "rm -rf ~", userOnly, []string{"toss-bin"}},
		{"home var", "~", `rm -rf "$HOME"`, userOnly, nil},
		{"home braces", "/", "rm -rf ${HOME}", userOnly, nil},
		{"tilde slash", "~", "rm -rf ~/", userOnly, nil},
		{"root", "~", "rm -rf /", []string{"the root of the file system"}, nil},
		{"glob from home", "~", "rm -rf *", userOnly, nil},
		{"glob under home", "/", "rm -rf ~/*", userOnly, nil},
		{"dot dot", "~/x", "rm -rf ..", userOnly, nil},
		{"temp link to home", "/", "rm -rf " + tmp + "/link/", userOnly, nil},
		{"temp glob through a link", "/", "rm -rf " + tmp + "/*/ build", userOnly, nil},
		{"temp glob part through a link", "/", "rm -rf " + tmp + "/*/notes.txt", outside, nil},
		{"home file without -r", "~", "rm ~", userOnly, nil},
		// Words only a shell can resolve.
		{"quoted tilde is a name", "~/x", "rm -rf '~'", nil, nil},
		{"unset var", "~", "rm -rf $UNSET/", []string{"which path `$UNSET/` names"}, nil},
		{"braces", "~", "rm -rf {a,b}", []string{"`{a,b}`"}, nil},
		{"dot glob", "~", "rm -rf .*", []string{"`.*` can match `..`"}, nil},
		{"dot glob part", "~", "rm -rf x/.*/f", []string{"`.*` can match `..`"}, nil},
		{"zsh links glob", "~", "rm -rf ***/f", []string{"follows symlinks"}, nil},
		{"posix class", "~", "rm -rf [[:alpha:]]*", []string{"bracket class"}, nil},
		{"dot dot after a glob", "~", "rm -rf */../x", []string{"`..` after a glob"}, nil},
		{"pwd set", "~", `PWD=/; rm -rf "$PWD/x"`, []string{"`\"$PWD/x\"`"}, nil},
		{"pwd in nested text", "~", `bash -c 'rm -rf "$PWD/x"'`, []string{"`\"$PWD/x\"`"}, nil},
		{"pwd after cd ;", "~", `cd x; rm -rf "$PWD/f"`, []string{"`\"$PWD/f\"`"}, nil},
		{"tilde user", "~", "rm -rf ~nobody/x", []string{"`~nobody/x`"}, nil},
		{"substitution", "~", "rm -rf $(cat list)", []string{"`$(cat list)`"}, nil},
		{"unknown cwd", "", "rm -rf x", []string{"`x`"}, nil},
		{"after failed cd", "/", "cd /nowhere || rm -rf x", []string{"the cd before it may not have run"}, nil},
		{"after cd and ;", "/", "cd ~/empty; rm -rf *", []string{"the cd before it may not have run"}, nil},
		{"new && run after cd", "/", "cd ~/empty; true && rm -rf *", []string{"may not have run"}, nil},
		{"cd && subshell", "~", "cd empty && (rm -rf *)", nil, nil},
		{"cd ; subshell", "~", "cd empty; (rm -rf *)", []string{"may not have run"}, nil},
		{"cd && absolute", "/", "cd /nowhere; rm -rf ~/missing", nil, nil},
		// Nothing is lost.
		{"empty file", "~", "rm zero", nil, nil},
		{"empty dir", "~", "rm -r empty", nil, nil},
		{"missing", "~", "rm -rf missing", nil, nil},
		{"dir without -r", "~", "rm x", nil, nil},
		{"symlink itself", "~", "rm ln", nil, nil},
		{"unlink symlink", "~", "unlink ln", nil, nil},
		{"temp link itself", "/", "rm -rf " + tmp + "/link", nil, nil},
		{"temp path", "/", "rm -rf " + tmp + "/x", nil, nil},
		{"temp glob", "/", "rm -rf " + tmp + "/*", nil, nil},
		{"temp glob of folders", "/", "rm -rf " + tmp + "/real/*/", nil, nil},
		{"dot glob with no match", "~/x", "rm -rf .[!.]*", nil, nil},
		{"pwd missing", "~", `rm -rf "$PWD/missing"`, nil, nil},
		{"mktemp var", "~", `D=$(mktemp -d); rm -rf "$D"`, nil, nil},
		{"mktemp sub", "~", `D=$(mktemp -d /tmp/fd.XXXXXX); rm -rf "$D/sub"`, nil, nil},
		{"shred value", "~", "shred -n notes.txt zero", nil, nil},
		{"no operand", "~", "rm -rf", nil, nil},
		// Deletes outside every work tree.
		{"file", "~", "rm notes.txt", append(outside, "`toss-bin --safe-mode -r notes.txt`"), nil},
		{"tree", "~", "rm -rf x", outside, nil},
		{"unlink", "~", "unlink notes.txt", outside, nil},
		{"shred", "~", "shred -n 3 -u notes.txt", outside, nil},
		{"shred through link", "~", "shred ln", outside, nil},
		{"mixed temp", "~", "rm -rf " + tmp + "/x ~/x", outside, nil},
		{"pwd", "~", `rm -rf "$PWD/x"`, append(outside, "`"+physical(home)+"/x`"), nil},
		{"pwd braces", "~", "rm -rf ${PWD}/x", outside, nil},
		{"pwd after cd &&", "/", `cd ~/x && rm -rf "$PWD/f"`, outside, nil},
		{"recursive glob", "~", "rm -rf x/**", outside, nil},
		{"mktemp empty", "~", `D=$(mktemp -d); rm -rf "$D"/*`,
			[]string{"if `$D` is empty", "the root of the file system", "toss-bin"}, nil},
		// The wrapped forms.
		{"sudo", "~", "sudo -u nobody rm -rf ~/x", outside, nil},
		{"env", "~", "env FOO=1 rm -rf ~/x", outside, nil},
		{"command", "~", "command rm -rf ~/x", outside, nil},
		{"nice", "~", "nice -n 5 rm -rf ~/x", outside, nil},
		{"timeout", "~", "timeout 10 rm -rf ~/x", outside, nil},
		{"busybox", "~", "busybox rm -rf ~/x", outside, nil},
		{"toybox", "~", "toybox rm -rf ~/x", outside, nil},
		{"gnu rm", "~", "grm -rf x", outside, nil},
		{"gnu unlink", "~", "gunlink notes.txt", outside, nil},
		{"gnu shred", "~", "/opt/homebrew/bin/gshred -u notes.txt", outside, nil},
		{"escaped", "~", `\rm -rf x`, outside, nil},
		{"path", "~", "/bin/rm -rf x", outside, nil},
		{"assignment", "~", "FOO=1 rm -rf x", outside, nil},
		{"if", "~", "if true; then rm -rf x; fi", outside, nil},
		// git runs these, and git-discard judges them.
		{"git rm", "~", "git rm -rf x", nil, nil},
		{"sudo git rm", "~", "sudo git rm -rf x", nil, nil},
		// Text no shell runs.
		{"echo", "~", "echo rm -rf ~", nil, nil},
		{"comment", "~", "true # rm -rf ~", nil, nil},
		{"cat heredoc", "~", "cat <<'EOF'\nrm -rf ~\nEOF", nil, nil},
		{"commit message", "~", `git commit -m "rm -rf ~"`, nil, nil},
		// Code the shell runs out of sight.
		{"bash heredoc", "~", "bash <<'EOF'\nrm -rf ~/x\nEOF", outside, nil},
		{"substituted", "~", "echo $(rm -rf ~/x)", outside, nil},
		{"eval", "~", "eval 'rm -rf ~/x'", outside, nil},
		{"subshell", "~", "(rm -rf ~/x)", outside, nil},
		{"bash -c cd", "/", "bash -c 'cd ~ && rm -rf x'", outside, nil},
		{"cd chain", "/", "cd ~ && rm -rf x", outside, nil},
		{"unread", "~", "rm -rf x 'oops", []string{"could not read the shell text"}, nil},
		{"unread find", "~", "find x -delete 'oops", []string{"could not read the shell text"}, nil},
	}
	runFileDiscardRows(t, newFileDiscardGuard(t, home), home, rows)
}

// TestFileDiscardShapesOutsideWorkTrees covers the find, rsync and xargs
// rows that need no git: a delete outside every work tree, temp and missing
// paths, the forms that change nothing, and the feeders belt cannot read.
func TestFileDiscardShapesOutsideWorkTrees(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{"x/f": "x", "x/a.pyc": "p", "src/s": "s", "list": "x/f\n"})
	tmp := tempFixture(t)
	writeTree(t, tmp, map[string]string{"t/a.pyc": "p", "t/b": "b", "s/a b": "s", "g/x*": "g", "n/a\nb": "n"})
	outside := []string{"in no git work tree and no temp folder"}
	rows := []fileDiscardRow{
		// find -delete and -exec take what the tests match under each root.
		{"find delete", "~", "find x -delete", append(outside, "-exec toss-bin --safe-mode -r {} +"), nil},
		{"find name delete", "~", "find x -name '*.pyc' -delete", outside, nil},
		{"find no root", "~/x", "find -delete", outside, nil},
		{"find exec rm", "~", `find x -name '*.pyc' -exec rm -f {} \;`, outside, nil},
		{"find exec rm plus", "~", "find x -type f -exec rm {} +", outside, nil},
		{"find ok", "~", "find x -ok rm {} ';'", outside, nil},
		{"find exec wrapped rm", "~", "find x -exec sudo rm {} +", outside, nil},
		{"wrapped find", "~", "sudo find ~/x -delete", outside, nil},
		{"gnu find", "~", "gfind x -delete", outside, nil},
		{"bsd root option", "~", "find -x -f x -delete", outside, nil},
		{"find exec other operand", "~", "find " + tmp + "/t -exec rm {} ~/x/f ';'", outside, nil},
		{"find exec plus not last", "~", "find src -exec rm {} ~/x/f +", []string{"the ; or + that ends"}, nil},
		{"find execdir relative", "~", "find " + tmp + "/t -execdir rm {} f ';'", []string{"relative to the folder of each match"}, nil},
		{"find exec derived", "~", `find src -exec rm {}.bak \;`, []string{"`{}.bak`"}, nil},
		{"find follows links", "~", "find -L " + tmp + "/t -delete", []string{"find -L follows symlinks"}, nil},
		{"find follow primary", "~", "find " + tmp + "/t -follow -delete", []string{"find -L follows symlinks"}, nil},
		{"find no terminator", "~", "find x -exec rm {}", []string{"the ; or + that ends find's -exec"}, nil},
		{"find roots from a file", "~", "find -files0-from list -delete", []string{"roots from a file"}, nil},
		{"find temp", "/", "find " + tmp + "/t -delete", nil, nil},
		{"find missing", "~", "find missing -name '*.pyc' -delete", nil, nil},
		{"find prints", "~", "find x -name '*.pyc' -print", nil, nil},
		{"find exec grep", "~", "find x -exec grep -l rm {} +", nil, nil},
		{"find no match", "~", "find x -name '*.none' -delete", outside, nil},
		// rsync with a delete option drops its local destination.
		{"rsync delete", "~", "rsync -a --delete src/ x/", append(outside, "add --backup"), []string{"toss-bin"}},
		{"rsync del", "~", "rsync -r --del src/ x", outside, nil},
		{"rsync delete after", "~", "rsync -a --delete-after src/ x/", outside, nil},
		{"rsync value options", "~", "rsync -a -e ssh --delete --exclude '*.o' src/ x --filter '- y'", outside, nil},
		{"rsync value cluster", "~", "rsync -avessh --delete src/ x", outside, nil},
		{"rsync remote", "~", "rsync -a --delete src/ host:/srv/x", nil, nil},
		{"rsync module", "~", "rsync -a --delete src/ rsync://host/mod/x", nil, nil},
		{"rsync dry run", "~", "rsync -avn --delete src/ x/", nil, nil},
		{"rsync dry run long", "~", "rsync -a --delete --dry-run src/ x/", nil, nil},
		{"rsync backup", "~", "rsync -ab --delete src/ x/", nil, nil},
		{"rsync missing destination", "~", "rsync -a --delete src/ fresh/", nil, nil},
		{"rsync temp destination", "~", "rsync -a --delete src/ " + tmp + "/t/", nil, nil},
		{"rsync without delete", "~", "rsync -a src/ x/", nil, nil},
		{"rsync listing", "~", "rsync -a --delete x/", nil, nil},
		{"rsync remove sources", "~", "rsync -a --remove-source-files x/ host:/srv", append(outside, "without --remove-source-files"), nil},
		{"rsync remove sources from list", "~", "rsync -a --remove-source-files --files-from=list . host:/srv", []string{"--files-from"}, nil},
		// xargs and a substitution take what their feeder prints.
		{"xargs find", "~", "find x -name '*.pyc' | xargs rm", append(outside, "`toss-bin --safe-mode -r` in place of `rm`"), nil},
		{"xargs find print0", "~", "find x -name '*.pyc' -print0 | xargs -0 rm -f", outside, nil},
		{"xargs find temp", "/", "find " + tmp + "/t -name '*.pyc' | xargs rm", nil, nil},
		{"xargs replace", "~", "find x -type f | xargs -I{} rm -f {}", outside, nil},
		{"xargs replace derived", "~", "find x | xargs -I % rm %.bak", []string{"`%.bak`"}, nil},
		{"xargs grep", "~", "grep -rl x x | xargs rm", outside, nil},
		{"xargs grep files", "~", "grep -l x x/f | xargs rm", outside, nil},
		{"xargs grep dir without -r", "~", "grep -l x x | xargs rm", nil, nil},
		{"xargs grep follows", "~", "grep -Rl x x | xargs rm", []string{"grep -R follows symlinks"}, nil},
		{"xargs grep lines", "~", "grep -r x x | xargs rm", []string{"prints lines, not paths"}, nil},
		{"xargs grep pattern option", "~", "grep -r -e -l x | xargs rm", []string{"prints lines, not paths"}, nil},
		{"xargs rg", "~", "rg -l x x | xargs rm", outside, nil},
		{"xargs rg files", "~", "rg --files x | xargs rm", outside, nil},
		{"xargs rg follows", "~", "rg -L -l x | xargs rm", []string{"rg -L follows symlinks"}, nil},
		{"xargs cat", "~", "cat list | xargs rm", []string{"`cat list` hands xargs", "belt reads only the paths"}, nil},
		{"xargs echo", "~", "echo x/f | xargs rm", []string{"`echo x/f` hands xargs"}, nil},
		{"xargs ls", "~", "ls | xargs rm -rf", []string{"`ls` hands xargs"}, nil},
		{"xargs no pipe", "~", "xargs rm < list", []string{"input belt cannot see"}, nil},
		// A redirect of xargs's stdin replaces the pipe, whatever the feeder.
		{"xargs stdin redirect", "/", "find " + tmp + "/t | xargs rm < list", []string{"from a redirection in place of the pipe"}, nil},
		{"xargs fd 0 redirect", "/", "find " + tmp + "/t | xargs rm 0<list", []string{"redirection in place of the pipe"}, nil},
		{"xargs heredoc", "/", "find " + tmp + "/t | xargs rm <<EOF\n~/x/f\nEOF", []string{"redirection in place of the pipe"}, nil},
		{"xargs herestring", "/", "find " + tmp + "/t | xargs rm <<< ~/x/f", []string{"redirection in place of the pipe"}, nil},
		{"xargs stdout redirect", "/", "find " + tmp + "/t | xargs rm > out.log", nil, nil},
		{"feeder stderr to the pipe", "/", "find " + tmp + "/t 2>&1 | xargs rm", []string{"error output of the command in front of xargs"}, nil},
		{"feeder stderr pipe", "/", "find " + tmp + "/t |& xargs rm", []string{"error output"}, nil},
		{"feeder stderr dropped", "/", "find " + tmp + "/t 2>/dev/null | xargs rm", nil, nil},
		// A feeder find that runs a command prints what that command prints.
		{"feeder find exec", "/", "find " + tmp + "/t -exec echo ~/x/f ';' | xargs rm", []string{"find -exec prints what its command prints"}, nil},
		{"feeder find exec with print", "/", "find " + tmp + "/t -exec true ';' -print | xargs rm", []string{"find -exec prints"}, nil},
		{"feeder find execdir", "/", "find " + tmp + "/t -execdir echo f ';' | xargs rm", []string{"find -exec prints"}, nil},
		{"feeder find ok", "/", "find " + tmp + "/t -ok echo f ';' | xargs rm", []string{"find -exec prints"}, nil},
		{"feeder find fprint", "/", "find " + tmp + "/t -fprint /dev/stdout | xargs rm", []string{"find prints more than paths there"}, nil},
		{"feeder find printf", "/", "find " + tmp + "/t -printf '%p\\n' | xargs rm", []string{"find prints more than paths there"}, nil},
		{"feeder find ls", "/", "find " + tmp + "/t -ls | xargs rm", []string{"find prints more than paths there"}, nil},
		{"substitution find exec", "/", "rm $(find " + tmp + "/t -exec echo ~/x/f ';')", []string{"find -exec prints"}, nil},
		{"feeder find print", "/", "find " + tmp + "/t -print | xargs rm", nil, nil},
		// xargs cuts at blanks, quotes and backslashes, an unquoted $( ) at
		// blanks and expands globs: a name that holds one becomes other paths.
		{"xargs cuts a name", "/", "find " + tmp + "/s | xargs rm -rf", []string{"/s/a b\", which holds a space", "find -print0"}, nil},
		{"xargs -0 keeps a name", "/", "find " + tmp + "/s -print0 | xargs -0 rm -rf", nil, nil},
		{"xargs -d keeps a name", "/", "find " + tmp + "/s | xargs -d '\\n' rm -rf", nil, nil},
		{"xargs -d cuts a newline", "/", "find " + tmp + "/n | xargs -d '\\n' rm -rf", []string{"a newline"}, nil},
		{"xargs cuts a newline", "/", "find " + tmp + "/n | xargs rm -rf", []string{"a newline"}, nil},
		{"xargs -0 keeps a newline", "/", "find " + tmp + "/n -print0 | xargs -0 rm -rf", nil, nil},
		{"substitution cuts a name", "/", "rm -rf $(find " + tmp + "/s)", []string{"holds a space"}, nil},
		{"substitution globs a name", "/", "rm -rf $(find " + tmp + "/g)", []string{"a glob character"}, nil},
		{"xargs does not glob", "/", "find " + tmp + "/g | xargs rm -rf", nil, nil},
		{"quoted substitution keeps a name", "/", "rm -rf \"$(find " + tmp + "/s)\"", nil, nil},
		{"substitution glob in the text", "/", "rm -rf $(find " + tmp + "/t -name '*.pyc')", nil, nil},
		{"xargs arg file", "~", "find x | xargs -a list rm", []string{"from a file belt cannot see"}, nil},
		{"xargs subshell feeder", "~", "(find x) | xargs rm", []string{"command in front of xargs"}, nil},
		{"xargs other command", "~", "find x | xargs grep y", nil, nil},
		{"xargs no command", "~", "find x | xargs", nil, nil},
		{"wrapped xargs", "~", "find x | sudo xargs rm", outside, nil},
		{"xargs shell", "~", `find x | xargs sh -c 'rm "$@"' _`, outside, nil},
		{"substitution find", "~", "rm -f $(find x -name '*.pyc')", outside, nil},
		{"substitution backticks", "~", "rm -f `find x -name '*.pyc'`", outside, nil},
		{"substitution temp", "/", "rm -f $(find " + tmp + "/t -name '*.pyc')", nil, nil},
		{"substitution pipeline", "~", "rm -f $(find x | grep pyc)", []string{"`$(find x | grep pyc)`"}, nil},
	}
	runFileDiscardRows(t, newFileDiscardGuard(t, home), home, rows)
}

// TestFileDiscardUnreadCode: code a command runs that belt cannot read, a
// shell fed by a pipe or the output of a $( ) or <( ), denies when the text
// that builds it names a delete, and passes otherwise.
func TestFileDiscardUnreadCode(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{"x/f": "x", "list": "x/f\n"})
	piped := []string{"belt cannot read the text `sh` runs", "name a delete"}
	printed := []string{"prints as code", "names a delete"}
	evaled := []string{"belt cannot read the text eval runs", "name a delete"}
	outside := []string{"in no git work tree and no temp folder"}
	rows := []fileDiscardRow{
		{"sed into sh", "~", "find x -name f | sed 's/^/rm /' | sh", piped, nil},
		{"cat and sed into bash", "~", "cat list | sed 's/^/rm /' | bash", []string{"`bash` runs"}, nil},
		{"awk into sh", "~", `awk '{print "rm " $0}' list | sh`, piped, nil},
		{"printf through cat into sh", "~", `find x -printf 'rm %p\n' | cat | sh`, piped, nil},
		{"subshell into sh", "~", "(sed 's/^/rm /' list) | sh", piped, nil},
		{"find -delete text into sh", "~", "sed 's/^/find /; s/$/ -delete/' list | sh", piped, nil},
		{"eval of a substitution", "~", `eval "$(sed 's/^/rm /' list)"`, printed, nil},
		{"sh -c of a substitution", "~", `sh -c "$(sed 's/^/rm /' list)"`, printed, nil},
		{"command word substitution", "~", "$(sed 's/^/rm /' list)", printed, nil},
		{"bash of a process substitution", "~", "bash <(sed 's/^/rm /' list)", printed, nil},
		{"bash of a here-string", "~", `bash <<< "$(sed 's/^/rm /' list)"`, printed, nil},
		{"sh of a here-string with text", "~", `sh <<< "set -e; $(sed 's/^/rm /' list)"`, printed, nil},
		{"bash of an unquoted heredoc", "~", "bash <<EOF\nset -e\n$(sed 's/^/rm /' list)\nEOF", []string{"names a delete"}, nil},
		{"bash of a here-string without a delete", "~", `bash <<< "$(mise activate bash)"`, nil, nil},
		{"source of a process substitution", "~", "source <(sed 's/^/unlink /' list)", printed, nil},
		// MAD-672: a backtick body runs with one level of escapes taken off,
		// \" too inside double quotes, so the eval reads the text that
		// command prints and the nested sed is a substitution of its own.
		{"eval of a backtick echo", "~", "eval \"`echo \\\"true; rm x/f\\\"`\"", outside, nil},
		{"sh -c of a backtick echo", "~", "sh -c \"`echo \\\"rm x/f\\\"`\"", outside, nil},
		{"eval of nested backticks", "~", "eval \"`echo \\`sed 's/^/rm /' list\\``\"", evaled, nil},
		{"eval of an escaped $ in backticks", "~", "eval \"`echo rm x/f \\$X`\"", evaled, nil},
		{"backtick echo of a delete as text", "~", "echo \"`echo \\\"; rm x/f; \\\"`\"", nil, nil},
		{"single-quoted eval of a backtick", "~", "eval '\"`echo \\\"; rm x/f; \\\"`\"'", nil, nil},
		// Nothing in the text that builds the code names a delete.
		{"mise", "~", `eval "$(mise activate zsh)"`, nil, nil},
		{"direnv", "~", `eval "$(direnv hook zsh)"`, nil, nil},
		{"fzf", "~", "source <(fzf --zsh)", nil, nil},
		{"ssh-agent", "~", `eval "$(ssh-agent -s)"`, nil, nil},
		{"echo into sh", "~", "echo 'make test' | sh", nil, nil},
		{"install script", "~", "curl -fsSL https://example.com/i.sh | sh", nil, nil},
		{"sed into sh without a delete", "~", "cat list | sed 's/^/echo /' | sh", nil, nil},
		// A delete spelled in pieces is no slip, and passes.
		{"rm in pieces", "~", `awk '{print "r" "m " $0}' list | sh`, nil, nil},
	}
	runFileDiscardRows(t, newFileDiscardGuard(t, home), home, rows)
}

// TestFileDiscardParallel: GNU parallel hands the command one path per
// line of its input, per ::: word or per line of a :::: file, at {} or the
// -I text or after the command's own operands. Options belt does not read,
// input belt cannot see, a cut path and other replacement strings deny.
func TestFileDiscardParallel(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{"x/f": "x", "x/a.pyc": "p", "list": "x/f\n"})
	tmp := tempFixture(t)
	writeTree(t, tmp, map[string]string{"t/a.pyc": "p", "t/b": "b", "s/a b": "s", "n/a\nb": "n"})
	outside := []string{"in no git work tree and no temp folder"}
	rows := []fileDiscardRow{
		{"stdin", "~", "find x -name '*.pyc' | parallel rm", append(outside, "in place of `rm`"), nil},
		{"stdin temp", "/", "find " + tmp + "/t -name '*.pyc' | parallel rm", nil, nil},
		{"stdin {}", "/", "find " + tmp + "/t | parallel rm -rf {}", nil, nil},
		{"stdin -I", "/", "find " + tmp + "/t | parallel -I@ rm -rf @", nil, nil},
		{"bundled options", "/", "find " + tmp + "/t -print0 | parallel -0kj4 rm -f", nil, nil},
		{"value options", "/", "find " + tmp + "/t | parallel -j 4 --bar --halt now,fail=1 --joblog=j rm -f", nil, nil},
		{"wrapped", "~", "nice parallel rm ::: x/f", outside, nil},
		{"words", "~", "parallel rm ::: x/f", outside, nil},
		{"words temp", "/", "parallel rm ::: " + tmp + "/t/b", nil, nil},
		{"words at {}", "/", "parallel rm {} ::: " + tmp + "/t/b", nil, nil},
		{"words glob", "/", "parallel -X rm ::: " + tmp + "/t/*", nil, nil},
		{"second source", "~", "parallel rm ::: " + tmp + "/t/b ::: x/f", outside, nil},
		{"command words read again", "~", `parallel rm "x/f ` + tmp + `/t/b" ::: ` + tmp + "/t/b", outside, nil},
		{"arg file", "~", "parallel rm :::: list", []string{"`list`, a file belt cannot see"}, nil},
		{"-a file", "~", "parallel -a list rm", []string{"a file belt cannot see"}, nil},
		{"-a stdin redirected", "~", "parallel -a - rm < list", []string{"redirection in place of the pipe"}, nil},
		{"no input", "~", "parallel rm", []string{"input belt cannot see"}, nil},
		{"process substitution", "/", "parallel rm :::: <(find " + tmp + "/t -name '*.pyc')", nil, nil},
		{"process substitution outside", "~", "parallel rm :::: <(find x -name '*.pyc')", outside, nil},
		{"keeps blanks", "/", "find " + tmp + "/s | parallel rm -rf", nil, nil},
		{"cuts a newline", "/", "find " + tmp + "/n | parallel rm -rf", []string{"a newline"}, nil},
		{"-0 keeps a newline", "/", "find " + tmp + "/n -print0 | parallel -0 rm -rf", nil, nil},
		{"-d cuts a blank", "/", "find " + tmp + "/s | parallel -d ' ' rm -rf", []string{"a space"}, nil},
		{"trim", "/", "parallel --trim lr rm ::: " + tmp + "/t/b", []string{"trims blanks"}, nil},
		{"no trim", "/", "parallel --trim n rm ::: " + tmp + "/t/b", nil, nil},
		{"pipe", "/", "find " + tmp + "/t | parallel --pipe rm", []string{"parallel's --pipe"}, nil},
		{"abbreviated option", "/", "parallel --pip rm ::: " + tmp + "/t/b", []string{"parallel's --pip"}, nil},
		{"profile", "/", "parallel -J p rm ::: " + tmp + "/t/b", []string{"parallel's -J"}, nil},
		{"replace without a value", "/", "parallel -i rm ::: " + tmp + "/t/b", []string{"parallel's -i"}, nil},
		{"other replacement", "/", "parallel rm {.}.o ::: " + tmp + "/t/b", []string{"`{.}.o`"}, nil},
		{"derived path", "/", "parallel rm {}.bak ::: " + tmp + "/t/b", []string{"`{}.bak`"}, nil},
		{"PARALLEL set", "/", "PARALLEL=--pipe parallel rm ::: " + tmp + "/t/b", []string{"PARALLEL"}, nil},
		{"input as commands", "~", "parallel ::: 'rm x/f'", []string{"runs its input as code"}, nil},
		{"input after {} as commands", "~", "parallel {} ::: 'rm x/f'", []string{"runs its input as code"}, nil},
		{"piped commands", "~", "sed 's/^/rm /' list | parallel", []string{"runs its input as code"}, nil},
		{"commands without a delete", "~", "parallel ::: 'echo hi'", nil, nil},
		{"command text", "/", "parallel 'rm {}; echo done' ::: " + tmp + "/t/b", []string{"command text"}, nil},
		{"shell script", "~", `parallel -q sh -c 'rm -f "$1"' _ ::: x/f`, outside, nil},
		{"find under parallel", "~", "parallel find {} -delete ::: x", []string{"a find that parallel runs"}, nil},
		{"dry run", "~", "parallel --dry-run rm ::: x/f", nil, nil},
		{"other command", "~", "parallel gzip ::: x/f", nil, nil},
		{"rsync without a delete", "~", "parallel rsync -a {} y/ ::: x", nil, nil},
	}
	runFileDiscardRows(t, newFileDiscardGuard(t, home), home, rows)
}

// TestFileDiscardConsumers: the consumer judge reads the command xargs,
// parallel, fd and find -exec run the same way. Input that runs as code (a
// shell's -c with no script or one that holds the replacement text, an
// eval, a command word that holds it) denies when the text that builds it
// names a delete. A -c script that hands its arguments to rm drops the
// input. Any other command that deletes denies only when it names a
// delete.
func TestFileDiscardConsumers(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{"x/f": "x", "x/a.pyc": "p", "list": "x/f\n", "cmds": "rm x/f\n"})
	tmp := tempFixture(t)
	writeTree(t, tmp, map[string]string{"t/a.pyc": "p", "t/b": "b"})
	outside := []string{"in no git work tree and no temp folder"}
	code := func(who string) []string { return []string{who + " runs its input as code", "names a delete"} }
	// An xargs that runs its items as a shell's script reads them as `| sh`
	// does (xargsFeeds): text belt cannot read there is opaque.
	unread := func(shell string) []string {
		return []string{"belt cannot read the text `xargs` hands `" + shell + "` to run", "name a delete"}
	}
	rows := []fileDiscardRow{
		// xargs: the input runs as code.
		{"xargs -I in the script", "~", `find x -name f | xargs -I{} sh -c 'rm "{}"'`, unread("sh"), nil},
		{"xargs -I % in the script", "~", "echo x/f | xargs -I % sh -c 'rm %'", code("xargs"), nil},
		{"xargs -I script", "~", "sed 's/^/rm /' list | xargs -I{} sh -c '{}'", unread("sh"), nil},
		{"xargs -c without a script", "~", `sed 's/^/rm /' list | tr '\n' '\0' | xargs -0 -n1 bash -c`, unread("bash"), nil},
		{"xargs -0 -c without a script", "~", "sed 's/^/rm /' list | xargs -0 bash -c", unread("bash"), nil},
		{"xargs eval of an argument", "~", `sed 's/^/rm /' list | xargs -n1 sh -c 'eval "$0"'`, code("xargs"), nil},
		{"xargs input as the command", "~", "echo rm x/f | xargs sudo", code("xargs"), nil},
		{"xargs -I command word", "~", "sed 's/^/rm /' list | xargs -I{} {}", code("xargs"), nil},
		{"xargs here-string", "~", "xargs -I{} sh -c '{}' <<< 'rm x/f'", code("xargs"), nil},
		{"xargs heredoc", "~", "xargs -I{} sh -c '{}' <<EOF\nrm x/f\nEOF", code("xargs"), nil},
		{"xargs code without a delete", "~", "find x | xargs -I{} sh -c 'echo {}'", nil, nil},
		{"xargs code from a file", "~", "xargs -I{} sh -c '{}' < cmds", nil, nil},
		// xargs: a -c script hands its arguments to rm.
		{"xargs $0", "~", `find x -name f | xargs -n1 sh -c 'rm "$0"'`, outside, nil},
		{"xargs $0 temp", "/", "find " + tmp + `/t | xargs -n1 sh -c 'rm "$0"'`, nil, nil},
		{"xargs $@ temp", "/", "find " + tmp + `/t | xargs sh -c 'rm -f "$@"' _`, nil, nil},
		{"xargs ${1} names a fixed word", "~", "find " + tmp + `/t | xargs sh -c 'rm "${1}"' _ x/f`, outside, nil},
		{"xargs -I word after the script", "/", "find " + tmp + `/t | xargs -I{} sh -c 'rm "$1"' _ {}`, nil, nil},
		{"xargs unset argument", "~", "find " + tmp + `/t | xargs -I{} sh -c 'rm "$1"' _`, []string{"`\"$1\"`"}, nil},
		{"xargs $0 from cat", "~", `cat list | xargs -n1 sh -c 'rm "$0"'`, []string{"`cat list` hands xargs"}, nil},
		{"xargs script of two commands", "/", "find " + tmp + `/t | xargs sh -c 'rm "$1"; echo' _`, []string{"`\"$1\"`"}, nil},
		{"xargs literal braces", "/", "find " + tmp + "/t | xargs sh -c 'rm {}'", []string{"`{}`"}, nil},
		// xargs: another command that deletes.
		{"xargs find delete", "~", "find x -type d | xargs -I{} find {} -delete", []string{"a find that xargs runs"}, nil},
		{"xargs find", "~", "echo x | xargs find", nil, nil},
		// parallel: the input runs as code.
		{"parallel -c ::: script", "~", "parallel bash -c ::: 'rm x/f'", code("parallel"), nil},
		{"parallel -c {}", "~", "parallel sh -c {} ::: 'rm x/f'", code("parallel"), nil},
		{"parallel eval", "~", "parallel eval ::: 'rm x/f'", code("parallel"), nil},
		{"parallel -c piped", "~", "sed 's/^/rm /' list | parallel bash -c", code("parallel"), nil},
		{"parallel -c without a delete", "~", "parallel bash -c ::: 'echo hi'", nil, nil},
		// find: the matches reach a -c script.
		{"find $1", "~", `find x -name f -exec sh -c 'rm "$1"' _ {} \;`, outside, nil},
		{"find $1 temp", "/", "find " + tmp + `/t -exec sh -c 'rm "$1"' _ {} \;`, nil, nil},
		{"find $@ temp", "/", "find " + tmp + `/t -exec sh -c 'rm -f "$@"' _ {} +`, nil, nil},
		{"find execdir $1 temp", "/", "find " + tmp + `/t -execdir sh -c 'rm "$1"' _ {} \;`, nil, nil},
		{"find {} in the script", "/", "find " + tmp + `/t -exec sh -c 'rm {}' \;`, code("find"), nil},
		{"find second -exec script", "/", "find " + tmp + `/t -exec sh -c 'true' \; -exec sh -c 'rm ~/x/f' \;`, outside, nil},
		{"find gzip script", "~", `find x -exec sh -c 'gzip "$1"' _ {} \;`, nil, nil},
	}
	runFileDiscardRows(t, newFileDiscardGuard(t, home), home, rows)
}

// TestFileDiscardFd: fd's -x and -X run a command over the entries fd
// matches, read like find -exec: each root a tree, narrowed by one -e, the
// -t letters and a --glob pattern, skipping hidden and ignored entries as
// fd does. An option fd 10.5.1 does not list, and one that runs fd
// elsewhere, follows symlinks or changes the paths, is not read.
func TestFileDiscardFd(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{"x/f": "x", "x/a.pyc": "p"})
	tmp := tempFixture(t)
	writeTree(t, tmp, map[string]string{"t/a.pyc": "p", "t/b": "b"})
	outside := []string{"in no git work tree and no temp folder"}
	rows := []fileDiscardRow{
		{"exec", "~/x", "fd -e pyc -x rm", append(outside, "-X toss-bin --safe-mode -r"), nil},
		{"exec batch", "~/x", "fd -X rm -f", outside, nil},
		{"exec at {}", "~/x", "fd -t f -x rm {}", outside, nil},
		{"root", "~", "fd . x -x rm", outside, nil},
		{"search path", "~", "fd --search-path x -x rm", outside, nil},
		{"fdfind", "~/x", "fdfind -e pyc -x rm", outside, nil},
		{"long exec", "~/x", "fd --extension=pyc --exec rm", outside, nil},
		{"cluster exec", "~/x", "fd -HIx rm", outside, nil},
		{"exec then options", "~/x", "fd -x rm ';' -e pyc", outside, nil},
		{"temp", "/", "fd . " + tmp + "/t -x rm", nil, nil},
		{"temp batch", "/", "fd -e pyc -0 -j4 . " + tmp + " -X rm -f", nil, nil},
		{"basename", "~/x", "fd -x rm {/}", []string{"`{/}`"}, nil},
		{"parent folder", "~/x", "fd -x rm -rf {//}", []string{"`{//}`"}, nil},
		{"derived path", "~/x", "fd -x rm {}.bak", []string{"`{}.bak`"}, nil},
		{"script holds {}", "~/x", "fd -x sh -c 'rm {}'", []string{"fd runs its input as code"}, nil},
		{"script $1", "~/x", `fd -x sh -c 'rm "$1"' _`, outside, nil},
		{"follows", "~/x", "fd -L -x rm", []string{"fd -L follows symlinks"}, nil},
		{"base directory", "~", "fd -C x -x rm", []string{"fd's --base-directory"}, nil},
		{"path separator", "~/x", "fd --path-separator x -x rm", []string{"fd's --path-separator"}, nil},
		{"unknown option", "~/x", "fd --frobnicate -x rm", []string{"fd's --frobnicate"}, nil},
		{"unknown short", "~/x", "fd -Z -x rm", []string{"fd's -Z"}, nil},
		{"value missing", "~/x", "fd -x rm ';' -e", []string{"takes a value"}, nil},
		{"no exec", "~/x", "fd -e pyc", nil, nil},
		{"type executable", "~/x", "fd -tx", nil, nil},
		{"other command", "~/x", "fd -e pyc -x gzip", nil, nil},
	}
	runFileDiscardRows(t, newFileDiscardGuard(t, home), home, rows)
}

// TestFileDiscardCacheFolders: a delete at or under DerivedData,
// ~/Library/Caches or ~/.cache passes, judged after its symlinks resolve:
// a link in a cache folder to another folder, and a cache folder that is
// itself a link, are judged where they lead. The folders around the caches
// still deny.
func TestFileDiscardCacheFolders(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{
		"Library/Caches/com.x/f": "x", "Library/CachesX/f": "x", "Library/Prefs/f": "p",
		"Library/Developer/Xcode/DerivedData/App-abc/Build/f": "b", "Library/Developer/Xcode/f": "x",
		".cache/pip/f": "p", "notes/f": "n",
	})
	if err := os.Symlink(filepath.Join(home, "notes"), filepath.Join(home, ".cache", "link")); err != nil {
		t.Fatal(err)
	}
	outside := []string{"in no git work tree and no temp folder"}
	userOnly := []string{"the home directory", "only the user may run it"}
	rows := []fileDiscardRow{
		{"caches entry", "/", "rm -rf ~/Library/Caches/com.x", nil, nil},
		{"caches itself", "/", "rm -rf ~/Library/Caches", nil, nil},
		{"caches glob", "/", "rm -rf ~/Library/Caches/*", nil, nil},
		{"caches find", "/", "find ~/Library/Caches -name f -delete", nil, nil},
		{"caches other case", "/", "rm -rf ~/library/caches/com.x", nil, nil},
		{"dot cache", "~", "rm -rf .cache/pip", nil, nil},
		{"dot cache itself", "/", "rm -rf ~/.cache", nil, nil},
		{"derived data", "/", "rm -rf ~/Library/Developer/Xcode/DerivedData/App-abc", nil, nil},
		{"derived data glob", "/", "rm -rf ~/Library/Developer/Xcode/DerivedData/*", nil, nil},
		{"link in a cache itself", "/", "rm -rf ~/.cache/link", nil, nil},
		{"link in a cache followed", "/", "rm -rf ~/.cache/link/", outside, nil},
		{"through a link in a cache", "/", "rm -rf ~/.cache/link/f", outside, nil},
		{"library", "/", "rm -rf ~/Library", outside, nil},
		{"developer", "/", "rm -rf ~/Library/Developer", outside, nil},
		{"xcode", "/", "rm -rf ~/Library/Developer/Xcode", outside, nil},
		{"caches parent by dot dot", "/", "rm -rf ~/Library/Caches/..", outside, nil},
		{"caches lookalike", "/", "rm -rf ~/Library/CachesX", outside, nil},
		{"library glob", "/", "rm -rf ~/Library/*", outside, nil},
		{"home", "/", "rm -rf ~", userOnly, nil},
	}
	runFileDiscardRows(t, newFileDiscardGuard(t, home), home, rows)

	// A cache folder that is itself a link counts by its own name only.
	linked := t.TempDir()
	writeTree(t, linked, map[string]string{"elsewhere/x/f": "x"})
	if err := os.Symlink(filepath.Join(linked, "elsewhere"), filepath.Join(linked, ".cache")); err != nil {
		t.Fatal(err)
	}
	runFileDiscardRows(t, newFileDiscardGuard(t, linked), linked, []fileDiscardRow{
		{"cache folder that is a link", "/", "rm -rf ~/.cache/x", outside, nil},
	})
}

// TestFileDiscardCacheLinkIntoRepo: a link in ~/.cache that leads into a
// work tree is judged there, with git.
func TestFileDiscardCacheLinkIntoRepo(t *testing.T) {
	r := newDiscardRepos(t)
	r.local(map[string]string{"a.txt": "mine\n"})
	home := t.TempDir()
	writeTree(t, home, map[string]string{".cache/": ""})
	if err := os.Symlink(r.clone, filepath.Join(home, ".cache", "repo")); err != nil {
		t.Fatal(err)
	}
	g := NewFileDiscard(config.Config{})
	g.home = func() string { return home }
	for command, want := range map[string]string{
		"rm -rf ~/.cache/repo/":     "the top of a git work tree",
		"rm -f ~/.cache/repo/a.txt": "`a.txt` holds content no commit has",
	} {
		d := g.Check(Input{Event: EventBash, Command: command, Cwd: home})
		if d == nil || !strings.Contains(d.Reason, want) {
			t.Errorf("Check(%q) = %v, want a deny holding %q", command, d, want)
		}
	}
	if d := g.Check(Input{Event: EventBash, Command: "rm -f ~/.cache/repo/b.txt", Cwd: home}); d != nil {
		t.Errorf("a committed file through the link = %q, want a pass", d.Reason)
	}
}

// TestFileDiscardReasonWithoutTossBin: the toss-bin form is offered only
// when toss-bin is on PATH.
func TestFileDiscardReasonWithoutTossBin(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{"notes.txt": "n"})
	g := newFileDiscardGuard(t, home)
	g.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	d := g.Check(Input{Event: EventBash, Command: "rm notes.txt", Cwd: home})
	if d == nil || strings.Contains(d.Reason, "toss-bin") {
		t.Fatalf("Check = %v, want a deny without toss-bin", d)
	}
	if !strings.HasSuffix(d.Reason, gitDiscardAsk) {
		t.Errorf("reason %q does not end with git-discard's tail", d.Reason)
	}
}

// TestFileDiscardSoftMode: a loss in soft mode passes with a warn event.
func TestFileDiscardSoftMode(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{"notes.txt": "n"})
	g := newFileDiscardGuard(t, home)
	g.cfg = config.Config{Guards: map[string]config.Toggle{FileDiscardID: {Mode: config.ModeSoft}}}
	var level, message string
	g.emit = func(_, l, _, m string, _ map[string]string) { level, message = l, m }
	if d := g.Check(Input{Event: EventBash, Command: "rm notes.txt", Cwd: home}); d != nil {
		t.Fatalf("Check = %q, want a pass in soft mode", d.Reason)
	}
	if level != "warn" || !strings.Contains(message, "Allowed (soft mode)") {
		t.Errorf("emit = %q %q, want a soft-mode warning", level, message)
	}
}

// TestFileDiscardAfterRmTemp: an rm whose targets are all temp keeps
// rm-temp's reason, since rm-temp runs first; a temp and a home target
// together get file-discard's.
func TestFileDiscardAfterRmTemp(t *testing.T) {
	ids := []string{}
	for _, g := range All(config.Config{}) {
		ids = append(ids, g.ID())
	}
	if i := slices.Index(ids, RmTempID); i < 0 || i+1 >= len(ids) || ids[i+1] != FileDiscardID {
		t.Fatalf("guards = %v, want %s right after %s", ids, FileDiscardID, RmTempID)
	}
	home := t.TempDir()
	writeTree(t, home, map[string]string{"y": "y"})
	tmp := tempFixture(t)
	writeTree(t, tmp, map[string]string{"x": "x"})
	found := func(string) (string, error) { return "/usr/local/bin/toss-bin", nil }
	guards := []Guard{
		&RmTemp{home: func() string { return home }, lookPath: found},
		newFileDiscardGuard(t, home),
	}
	first := func(command string) *Denial {
		for _, g := range guards {
			if d := g.Check(Input{Event: EventBash, Command: command, Cwd: home}); d != nil {
				return d
			}
		}
		return nil
	}
	if d := first("rm -rf " + tmp + "/x"); d == nil || d.Guard != RmTempID {
		t.Errorf("all temp: denial %+v, want rm-temp's", d)
	}
	if d := first("rm -rf " + tmp + "/x ~/y"); d == nil || d.Guard != FileDiscardID {
		t.Errorf("temp and home: denial %+v, want file-discard's", d)
	}
}

// TestFileDiscardBounds: one path cap for the whole call, and no git at all
// while a GIT_DIR-style variable points git elsewhere.
func TestFileDiscardBounds(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "repo")
	writeTree(t, repo, map[string]string{".git/": "", "a": "a", "b": "b"})
	g := newFileDiscardGuard(t, home)
	g.maxPaths = 1
	var specs []string
	g.status = func(_ context.Context, _ string, pathspecs []string) ([]byte, error) {
		specs = pathspecs
		return []byte(" M a\x00 M b\x00"), nil
	}
	in := Input{Event: EventBash, Command: "rm a; rm b", Cwd: repo}
	d := g.Check(in)
	if d == nil || !strings.Contains(d.Reason, "more than 1 paths") {
		t.Fatalf("Check = %v, want the path cap", d)
	}
	if !slices.Equal(specs, []string{"a", "b"}) {
		t.Errorf("one git status got pathspecs %v, want [a b]", specs)
	}
	t.Setenv("GIT_DIR", "/elsewhere")
	if d := g.Check(in); d == nil || !strings.Contains(d.Reason, "GIT_DIR-style variable") {
		t.Errorf("Check = %v, want the GIT_DIR refusal", d)
	}
}

// TestFileDiscardInWorkTrees runs the guard with its real git backends over
// a clone and its upstream: committed, dirty, untracked and ignored files,
// the top, git's own data, a nested repository and a linked worktree.
func TestFileDiscardInWorkTrees(t *testing.T) {
	runRepoRows(t, []repoRow{
		{"committed file", nil, "rm a.txt", nil},
		{"committed tree", nil, "rm -rf d", nil},
		{"committed glob", nil, "rm *.txt", nil},
		{"dirty file", func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			"rm a.txt", []string{"`a.txt` holds content no commit has", "toss-bin --safe-mode -r a.txt"}},
		{"dirty in tree", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			"rm -rf d", []string{"`d/f.txt` holds content no commit has"}},
		{"dirty glob", func(r discardRepos) { r.local(map[string]string{"b.txt": "mine\n"}) },
			"rm *.txt", []string{"`b.txt`"}},
		{"second operand dirty", func(r discardRepos) { r.local(map[string]string{"b.txt": "mine\n"}) },
			"rm a.txt b.txt", []string{"`b.txt`"}},
		{"equal to upstream", func(r discardRepos) {
			r.upstream(map[string]string{"a.txt": "a2\n"})
			r.local(map[string]string{"a.txt": "a2\n"})
		}, "rm a.txt", nil},
		{"staged only", func(r discardRepos) {
			r.local(map[string]string{"a.txt": "staged\n"})
			r.git(r.clone, "add", "a.txt")
		}, "rm a.txt", []string{"`a.txt`"}},
		{"untracked file", func(r discardRepos) { r.local(map[string]string{"new.txt": "n\n"}) },
			"rm new.txt", []string{"new.txt`, which is untracked"}},
		{"untracked in tree", func(r discardRepos) { r.local(map[string]string{"d/new.txt": "n\n"}) },
			"rm -rf d", []string{"d/new.txt`, which is untracked"}},
		{"ignored tree", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "build/\n")
			r.local(map[string]string{"build/out.o": "o"})
		}, "rm -r build/", nil},
		{"ignored glob", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "build/\n")
			r.local(map[string]string{"build/out.o": "o"})
		}, "rm -rf build/*", nil},
		{"top", nil, "rm -rf .", []string{"the top of a git work tree"}},
		{"git data", nil, "rm -rf .git", []string{"git's own data"}},
		// bash and zsh match a leading dot only to a literal one.
		{"star skips git data", nil, "rm -rf *", nil},
		{"dot slash star skips git data", nil, "rm -r ./*", nil},
		{"dot glob over git data", nil, "rm -rf .[!.]*", []string{"git's own data"}},
		{"star over a dirty file", func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			"rm -rf *", []string{"`a.txt`"}},
		// zsh's ** is recursive.
		{"recursive glob", func(r discardRepos) { r.local(map[string]string{"deep/a/b/k.go": "k\n"}) },
			"rm -rf **/k.go", []string{"deep/a/b/k.go`, which is untracked"}},
		{"recursive glob under a folder", func(r discardRepos) {
			r.local(map[string]string{"deep/a/b/k.go": "k\n"})
		}, "rm -rf deep/**/k.go", []string{"deep/a/b/k.go`, which is untracked"}},
		{"recursive glob over committed files", nil, "rm -rf **/f.txt", nil},
		{"trailing recursive glob", func(r discardRepos) { r.local(map[string]string{"d/e/new.txt": "n\n"}) },
			"rm d/**", []string{"d/e/new.txt`, which is untracked"}},
		// $PWD is the shell's directory.
		{"pwd over a committed tree", nil, `rm -rf "$PWD/d"`, nil},
		{"pwd over a dirty tree", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			`rm -rf "$PWD/d"`, []string{"`d/f.txt` holds content no commit has"}},
		// git status lists an ignored folder as one entry and never looks in.
		{"worktree in an ignored folder", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", ".claude/\n")
			wt := filepath.Join(r.clone, ".claude", "worktrees", "x")
			r.git(r.clone, "worktree", "add", "-q", "-b", "x", wt)
			r.write(wt, "a.txt", "mine\n")
			r.write(wt, "wip.txt", "wip\n")
		}, "rm -rf .claude", []string{"/.claude/worktrees/x`, a work tree of", "git worktree remove "}},
		{"worktree under an ignored folder", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", ".claude/\n")
			r.git(r.clone, "worktree", "add", "-q", "-b", "x", filepath.Join(r.clone, ".claude", "worktrees", "x"))
		}, "rm -rf .claude/worktrees", []string{"/.claude/worktrees/x`, a work tree of"}},
		{"clone in an ignored folder", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "vendor/\n")
			r.git(r.clone, "init", "-q", filepath.Join(r.clone, "vendor", "deps", "lib"))
		}, "rm -rf vendor", []string{"/vendor/deps/lib`, a git repository inside the ignored"}},
		{"clone under a dropped ignored folder", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "vendor/\n")
			r.git(r.clone, "init", "-q", filepath.Join(r.clone, "vendor", "deps", "lib"))
		}, "rm -rf vendor/deps", []string{"/vendor/deps/lib`, a git repository"}},
		{"ignored folder beside a clone", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "vendor/\nbuild/\n")
			r.git(r.clone, "init", "-q", filepath.Join(r.clone, "vendor", "lib"))
			r.local(map[string]string{"build/a/b/out.o": "o"})
		}, "rm -rf build", nil},
		{"ignored file beside a dirty one", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "*.log\n")
			r.local(map[string]string{"d/x.log": "l"})
		}, "rm -rf d", nil},
		{"nested repository", func(r discardRepos) {
			inner := filepath.Join(r.clone, "vendor", "inner")
			r.git(r.clone, "init", "-q", inner)
			r.write(inner, "f", "f\n")
		}, "rm -rf vendor", []string{"a nested repository"}},
		{"linked worktree", func(r discardRepos) {
			r.git(r.clone, "worktree", "add", "-q", "-b", "wt", filepath.Join(r.clone, "..", "wt"))
		}, "rm -rf ../wt", []string{"the top of a git work tree", "git worktree remove "}},
		// A clone a delete takes whole is judged by its own state.
		{"clean clone in an ignored folder", swiftCheckout(nil), "rm -rf .build", nil},
		{"clean clone under a dropped ignored folder", swiftCheckout(nil), "rm -rf .build/checkouts", nil},
		{"clone with a local edit", swiftCheckout(func(r discardRepos, dep string) {
			r.write(dep, "a.txt", "mine\n")
		}), "rm -rf .build", []string{"/.build/checkouts/dep`, a git repository inside the ignored",
			"changes no commit has"}},
		{"clone with an untracked file", swiftCheckout(func(r discardRepos, dep string) {
			r.write(dep, "new.txt", "n\n")
		}), "rm -rf .build", []string{"changes no commit has"}},
		{"clone with a local commit", swiftCheckout(func(r discardRepos, dep string) {
			r.write(dep, "c.txt", "c\n")
			r.git(dep, "add", "c.txt")
			r.git(dep, "commit", "-q", "-m", "local")
		}), "rm -rf .build", []string{"is on no remote branch"}},
		{"clone with a local branch", swiftCheckout(func(r discardRepos, dep string) {
			r.git(dep, "checkout", "-q", "-b", "side")
			r.write(dep, "c.txt", "c\n")
			r.git(dep, "add", "c.txt")
			r.git(dep, "commit", "-q", "-m", "local")
			r.git(dep, "checkout", "-q", "main")
		}), "rm -rf .build", []string{"is on no remote branch"}},
		{"clone with a stash", swiftCheckout(func(r discardRepos, dep string) {
			r.write(dep, "a.txt", "mine\n")
			r.git(dep, "stash", "-q")
		}), "rm -rf .build", []string{"is on no remote branch"}},
		{"clone inside a clean clone", swiftCheckout(func(r discardRepos, dep string) {
			r.write(dep, ".git/info/exclude", "out/\n")
			r.git(dep, "init", "-q", filepath.Join(dep, "out", "inner"))
		}), "rm -rf .build", []string{"/.build/checkouts/dep/out/inner`"}},
		{"clean untracked clone", func(r discardRepos) {
			dep := filepath.Join(r.clone, "vendor", "dep")
			r.git(r.clone, "clone", "-q", filepath.Join(filepath.Dir(r.clone), "up.git"), dep)
			overNetwork(r, dep)
		}, "rm -rf vendor", nil},
		// find takes what its tests match under each root.
		{"find ignored matches beside a dirty file", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "*.pyc\n")
			r.local(map[string]string{"a.txt": "mine\n", "d/x.pyc": "p", "x.pyc": "p"})
		}, "find . -name '*.pyc' -delete", nil},
		{"find over an untracked match", func(r discardRepos) {
			r.local(map[string]string{"a.txt": "mine\n", "d/x.pyc": "p"})
		}, "find . -name '*.pyc' -delete", []string{"d/x.pyc`, which is untracked"}},
		{"find over a dirty match", func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			"find . -iname '*.TXT' -delete", []string{"`a.txt` holds content no commit has"}},
		{"find type over git data", nil, "find . -type f -delete", []string{"/.git/", "git's own data"}},
		{"find name over git data", nil, "find . -name HEAD -delete", []string{"/.git/HEAD`", "git's own data"}},
		{"find committed folder", nil, "find d -type f -delete", nil},
		{"find dirty folder", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			"find d -type f -delete", []string{"`d/f.txt` holds content no commit has"}},
		{"find folders whole", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			"find . -name d -type d -exec rm -rf {} +", []string{"`d/f.txt`"}},
		{"find folders whole, clean", func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			"find . -name d -type d -exec rm -rf {} +", nil},
		{"find files by a folder name", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			"find . -name d -delete", nil},
		{"find test belt does not read", nil, "find . -name '*.txt' -newer a.txt -delete",
			[]string{"the top of a git work tree"}},
		{"find or", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			"find d -name '*.pyc' -o -name '*.txt' -delete", []string{"`d/f.txt`"}},
		{"find delete before the test", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			"find d -delete -name '*.pyc'", []string{"`d/f.txt`"}},
		// rsync --delete drops its destination.
		{"rsync over a committed folder", nil, "rsync -a --delete " + "../seed/d/ d/", nil},
		{"rsync over a dirty folder", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			"rsync -a --delete ../seed/d/ d/", []string{"`d/f.txt`", "add --backup"}},
		{"rsync over the top", nil, "rsync -a --delete ../seed/ ./", []string{"the top of a git work tree"}},
		{"rsync to a remote", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			"rsync -a --delete d/ host:d/", nil},
		// xargs and substitutions take what git, grep, rg or find print.
		{"git ls-files beside an untracked file", func(r discardRepos) {
			r.local(map[string]string{"new.txt": "n\n"})
		}, "git ls-files | xargs rm", nil},
		{"git ls-files over a dirty file", func(r discardRepos) { r.local(map[string]string{"b.txt": "mine\n"}) },
			"git ls-files -z | xargs -0 rm -f", []string{"`b.txt` holds content no commit has"}},
		{"git ls-files of a folder", func(r discardRepos) { r.local(map[string]string{"b.txt": "mine\n"}) },
			"git ls-files d | xargs rm", nil},
		{"git ls-files others", func(r discardRepos) { r.local(map[string]string{"new.txt": "n\n"}) },
			"git ls-files -o | xargs rm", []string{"new.txt`, which is untracked"}},
		{"git ls-files full name", nil, "git ls-files --full-name | xargs rm", []string{"--full-name"}},
		{"git ls-files substitution", func(r discardRepos) { r.local(map[string]string{"new.txt": "n\n"}) },
			"rm $(git ls-files)", nil},
		{"git ls-files substitution, dirty", func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			"rm $(git ls-files)", []string{"`a.txt`"}},
		{"git grep", func(r discardRepos) { r.local(map[string]string{"new.txt": "n\n"}) },
			"git grep -l a | xargs rm", nil},
		{"git grep without -l", nil, "git grep a | xargs rm", []string{"prints lines, not paths"}},
		{"grep a committed folder", func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			"grep -rl x d | xargs rm", nil},
		{"grep a dirty folder", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			"grep -rl x d | xargs rm", []string{"`d/f.txt`"}},
		{"grep from the top", nil, "grep -rl x . | xargs rm", []string{"/.git`, which is git's own data"}},
		{"grep from the top without git data", nil, "grep -rl --exclude-dir=.git x . | xargs rm", nil},
		{"grep from the top, dirty", func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			"grep -rl --exclude-dir=.git x . | xargs rm", []string{"`a.txt`"}},
		{"rg from the top", nil, "rg -l x | xargs rm", nil},
		{"rg hidden from the top", nil, "rg --hidden -l x | xargs rm", []string{"git's own data"}},
		{"find feeder beside a dirty file", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "*.pyc\n")
			r.local(map[string]string{"a.txt": "mine\n", "d/x.pyc": "p"})
		}, "find . -name '*.pyc' -print0 | xargs -0 rm -f", nil},
		{"find feeder over a dirty file", func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n"}) },
			"find . -name '*.txt' | xargs rm", []string{"`a.txt`"}},
	})
}

// TestFileDiscardPipeAfterCd: a pipeline runs only if the && run up to its
// first command ran, so its members share that command's directory; after
// `cd d;`, `cd d ||` or a `;` the directory is not sure.
func TestFileDiscardPipeAfterCd(t *testing.T) {
	pyc := func(r discardRepos) {
		r.write(r.clone, ".git/info/exclude", "*.pyc\n")
		r.local(map[string]string{"d/x.pyc": "p", "a.txt": "mine\n"})
	}
	late := []string{"the cd before it may not have run"}
	runRepoRows(t, []repoRow{
		{"cd && find | xargs", pyc, "cd d && find . -name '*.pyc' | xargs rm -f", nil},
		{"cd && true && find | xargs", pyc, "cd d && true && find . -name '*.pyc' | xargs rm -f", nil},
		{"cd && a pipe into rm", pyc, "cd d && true | rm -f x.pyc", nil},
		{"cd && a pipe, then && rm", pyc, "cd d && true | cat && rm -f x.pyc", nil},
		{"cd ; find | xargs", pyc, "cd d; find . -name '*.pyc' | xargs rm -f", late},
		{"cd || true ; find | xargs", pyc, "cd d || true; find . -name '*.pyc' | xargs rm -f", late},
		{"cd || find | xargs", pyc, "cd d || find . -name '*.pyc' | xargs rm -f", late},
		{"cd && pipeline ; rm", pyc, "cd d && true | cat; rm -f x.pyc", late},
		// A shell that leaves when the cd fails runs the rest in its folder.
		{"cd || exit 1 ; rm", pyc, "cd d || exit 1; rm -f x.pyc", nil},
		{"cd || exit ; rm", pyc, "cd d || exit\nrm -f x.pyc", nil},
		{"cd || exit 1 ; find | xargs", pyc, "cd d || exit 1; find . -name '*.pyc' | xargs rm -f", nil},
		{"cd || exit 1 ; rm in front", pyc, "cd d || exit 1; rm -f ../a.txt", []string{"`a.txt`"}},
		{"cd || exit in a -c script", pyc, "bash -c 'cd d || exit 1; rm -f x.pyc'", nil},
		{"cd || exit 1, then a second cd ; rm", pyc, "cd d || exit 1; cd ..; rm -f x.pyc", late},
		{"cd || exit 1 | cat ; rm", pyc, "cd d || exit 1 | cat; rm -f x.pyc", late},
		{"cd || exit 1 & ; rm", pyc, "cd d || exit 1 & rm -f x.pyc", late},
		{"cd || exit $? ; rm", pyc, "cd d || exit $?; rm -f x.pyc", late},
		{"cd || return 1 ; rm", pyc, "cd d || return 1; rm -f x.pyc", late},
		{"cd ; exit-free || exit", pyc, "cd d; true || exit 1; rm -f x.pyc", late},
	})
}

// TestFileDiscardConsumersInWorkTrees runs the consumer judge with the
// real git backends: fd's matches, and a -c script that hands the paths
// xargs or find feed it to rm.
func TestFileDiscardConsumersInWorkTrees(t *testing.T) {
	pyc := func(r discardRepos) {
		r.write(r.clone, ".git/info/exclude", "*.pyc\n")
		r.local(map[string]string{"x.pyc": "p", "d/x.pyc": "p"})
	}
	dirty := func(r discardRepos) {
		pyc(r)
		r.local(map[string]string{"a.txt": "mine\n"})
	}
	runRepoRows(t, []repoRow{
		{"fd over ignored files", dirty, "fd -e pyc -x rm", nil},
		{"fd -I over ignored files", dirty, "fd -I -e pyc -X rm -f", nil},
		{"fd glob over ignored files", dirty, "fd -g '*.pyc' -I -x rm", nil},
		{"fd over a dirty file", dirty, "fd -e txt -x rm", []string{"`a.txt`"}},
		{"fd regex over a dirty file", dirty, "fd a.txt -X rm", []string{"`a.txt`"}},
		{"fd -H into git data", pyc, "fd -H -X rm -f", []string{"git's own data"}},
		{"xargs $0 over ignored files", dirty, `find . -name '*.pyc' | xargs -n1 sh -c 'rm "$0"'`, nil},
		{"xargs $0 over a dirty file", dirty, `find . -name a.txt | xargs -n1 sh -c 'rm "$0"'`, []string{"`a.txt`"}},
		{"find $1 over ignored files", dirty, `find . -name '*.pyc' -exec sh -c 'rm "$1"' _ {} \;`, nil},
		{"find $1 over a dirty file", dirty, `find . -name a.txt -exec sh -c 'rm "$1"' _ {} \;`, []string{"`a.txt`"}},
		{"xargs -I script over ignored files", dirty, `find . -name '*.pyc' | xargs -I{} sh -c 'rm "{}"'`,
			[]string{"belt cannot read the text `xargs` hands `sh` to run"}},
	})
}

// repoRow is one command run in a fresh clone (newDiscardRepos) after
// setup, if any; want lists what the reason holds, and nil means a pass.
type repoRow struct {
	name    string
	setup   func(r discardRepos)
	command string
	want    []string
}

// runRepoRows runs each row with the guard's real git backends.
func runRepoRows(t *testing.T, rows []repoRow) {
	t.Helper()
	for _, tt := range rows {
		t.Run(tt.name, func(t *testing.T) {
			r := newDiscardRepos(t)
			if tt.setup != nil {
				tt.setup(r)
			}
			g := NewFileDiscard(config.Config{})
			g.lookPath = func(string) (string, error) { return "/usr/local/bin/toss-bin", nil }
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: r.clone})
			switch {
			case tt.want == nil && d != nil:
				t.Fatalf("Check(%q) = %q, want a pass", tt.command, d.Reason)
			case tt.want != nil && d == nil:
				t.Fatalf("Check(%q) passed, want a deny holding %q", tt.command, tt.want)
			}
			for _, w := range tt.want {
				if !strings.Contains(d.Reason, w) {
					t.Errorf("reason %q does not hold %q", d.Reason, w)
				}
			}
		})
	}
}

// swiftCheckout sets up a clone of the upstream under an ignored .build
// folder at checkouts/dep, then runs change on that clone, if any, and
// points its origin across the network (overNetwork).
func swiftCheckout(change func(r discardRepos, dep string)) func(r discardRepos) {
	return func(r discardRepos) {
		r.write(r.clone, ".git/info/exclude", ".build/\n")
		r.local(map[string]string{".build/debug/out.o": "o"})
		dep := filepath.Join(r.clone, ".build", "checkouts", "dep")
		r.git(r.clone, "clone", "-q", filepath.Join(filepath.Dir(r.clone), "up.git"), dep)
		if change != nil {
			change(r, dep)
		}
		overNetwork(r, dep)
	}
}

// networkUpstream is the URL overNetwork gives a clone's origin. Nothing
// is ever fetched from it.
const networkUpstream = "https://example.invalid/up.git"

// overNetwork points the origin of the repository at dir across the
// network, as a clone from a server has it, once a fetch or push from the
// local upstream has filled its remote branches.
func overNetwork(r discardRepos, dir string) {
	r.t.Helper()
	r.git(dir, "remote", "set-url", "origin", networkUpstream)
}

// TestFileDiscardCaseInsensitive: on a volume that ignores case, as APFS
// does by default, a path written in another case is judged under the name
// its folder lists, which is the name git matches. It skips on a volume
// that tells case apart.
func TestFileDiscardCaseInsensitive(t *testing.T) {
	r := newDiscardRepos(t)
	if _, err := os.Lstat(filepath.Join(r.clone, "A.TXT")); err != nil {
		t.Skip("this volume tells case apart")
	}
	r.local(map[string]string{"b.txt": "mine\n", "d/f.txt": "mine\n"})
	tests := []struct {
		command string
		want    []string // nil for a pass
	}{
		{"rm -f B.txt", []string{"`b.txt` holds content no commit has"}},
		{"rm -rf D", []string{"`d/f.txt` holds content no commit has"}},
		{"rm -rf D/F.TXT", []string{"`d/f.txt` holds content no commit has"}},
		{"rm -rf .GIT", []string{"git's own data"}},
		{"rm -f A.TXT", nil},
	}
	g := NewFileDiscard(config.Config{})
	for _, tt := range tests {
		d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: r.clone})
		switch {
		case tt.want == nil && d != nil:
			t.Errorf("Check(%q) = %q, want a pass", tt.command, d.Reason)
		case tt.want != nil && d == nil:
			t.Errorf("Check(%q) passed, want a deny holding %q", tt.command, tt.want)
		}
		for _, w := range tt.want {
			if d != nil && !strings.Contains(d.Reason, w) {
				t.Errorf("Check(%q) = %q, does not hold %q", tt.command, d.Reason, w)
			}
		}
	}
}

// TestFileDiscardWalkCaps: past its folder cap a glob whose matches stay in
// its folder is judged as that folder, any other glob is denied, and so is
// an ignored folder belt cannot finish searching for a repository.
func TestFileDiscardWalkCaps(t *testing.T) {
	r := newDiscardRepos(t)
	r.write(r.clone, ".git/info/exclude", "build/\n")
	r.local(map[string]string{"build/a/b/out.o": "o"})
	g := NewFileDiscard(config.Config{})
	g.globDirs, g.scanDirs = 0, 1
	check := func(command string) *Denial {
		return g.Check(Input{Event: EventBash, Command: command, Cwd: r.clone})
	}
	if d := check("rm -rf d/**/f.txt"); d != nil {
		t.Errorf("a recursive glob over a committed folder = %q, want a pass", d.Reason)
	}
	if d := check("rm -rf */f.txt"); d == nil || !strings.Contains(d.Reason, "cannot list every path") {
		t.Errorf("a glob that can leave its folder = %v, want the glob cap", d)
	}
	if d := check("rm -rf build"); d == nil || !strings.Contains(d.Reason, "could not check every folder") {
		t.Errorf("an ignored folder past the cap = %v, want the scan cap", d)
	}
	r.local(map[string]string{"d/f.txt": "mine\n"})
	if d := check("rm -rf d/**/f.txt"); d == nil || !strings.Contains(d.Reason, "`d/f.txt`") {
		t.Errorf("a recursive glob over a dirty folder = %v, want a deny naming d/f.txt", d)
	}
}

// TestFileDiscardStatusSkipsFSMonitor: the guard's git status runs no
// core.fsmonitor program the repository names.
func TestFileDiscardStatusSkipsFSMonitor(t *testing.T) {
	r := newDiscardRepos(t)
	marker := filepath.Join(t.TempDir(), "ran")
	monitor := filepath.Join(t.TempDir(), "monitor.sh")
	r.write(filepath.Dir(monitor), "monitor.sh", "#!/bin/sh\ntouch '"+marker+"'\n")
	if err := os.Chmod(monitor, 0o755); err != nil {
		t.Fatal(err)
	}
	r.git(r.clone, "config", "core.fsmonitor", monitor)
	r.git(r.clone, "status", "--porcelain")
	if _, err := os.Stat(marker); err != nil {
		t.Skip("this git runs no core.fsmonitor hook on status")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	r.local(map[string]string{"a.txt": "mine\n"})
	if d := NewFileDiscard(config.Config{}).Check(
		Input{Event: EventBash, Command: "rm a.txt", Cwd: r.clone},
	); d == nil {
		t.Fatal("dirty a.txt passed")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the guard's git status ran the core.fsmonitor program")
	}
}
