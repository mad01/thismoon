package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFileDiscardReplaceOutsideWorkTrees covers the replace rows that need
// no git: each shape over a file outside every work tree, the forms that
// write nothing, the cheap passes and the walk around them.
func TestFileDiscardReplaceOutsideWorkTrees(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{"notes.txt": "n", "zero": "", "dir/f": "f"})
	if err := os.Symlink(filepath.Join(home, "notes.txt"), filepath.Join(home, "ln")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "gone"), filepath.Join(home, "dangling")); err != nil {
		t.Fatal(err)
	}
	tmp := tempFixture(t)
	writeTree(t, tmp, map[string]string{"f": "f"})
	outside := []string{"this overwrites `", "in no git work tree and no temp folder",
		"An overwrite passes only when"}
	notDelete := []string{"--safe-mode -r", "A delete passes"}
	toss := func(file, command string) []string {
		return append(outside, "`toss-bin --safe-mode "+file+" && "+command+"`")
	}
	rows := []fileDiscardRow{
		// A redirection behind a command that prints nothing empties the file.
		{"bare", "~", "> notes.txt", toss("notes.txt", "> notes.txt"), notDelete},
		{"colon", "~", ": > notes.txt", toss("notes.txt", ": > notes.txt"), notDelete},
		{"true", "~", "true > notes.txt", outside, nil},
		{"echo", "~", "echo > notes.txt", outside, nil},
		{"echo -n", "~", "echo -n > notes.txt", outside, nil},
		{"printf empty", "~", "printf '' > notes.txt", outside, nil},
		{"clobber", "~", ": >| notes.txt", outside, nil},
		{"both streams", "~", ": &> notes.txt", outside, nil},
		{"stdout by number", "~", ": 1> notes.txt", outside, nil},
		{"both streams, csh form", "~", "echo >& notes.txt", outside, nil},
		{"assignment only", "~", "x=1 > notes.txt", outside, nil},
		{"if", "~", "if : > notes.txt; then true; fi", outside, nil},
		{"home file from the root", "/", ": > ~/notes.txt", outside, nil},
		{"symlink", "~", ": > ln", append(outside, "`"+physical(home)+"/notes.txt`"), nil},
		{"second target", "~", ": > zero > notes.txt", outside, nil},
		{"stderr", "~", ": 2> notes.txt", toss("notes.txt", ": 2> notes.txt"), notDelete},
		{"echo to stderr", "~", "echo 2> notes.txt", outside, nil},
		{"third descriptor", "~", ": 3> notes.txt", outside, nil},
		{"exec", "~", "exec > notes.txt", toss("notes.txt", "exec > notes.txt"), notDelete},
		{"exec stderr", "~", "exec 2> notes.txt", outside, nil},
		{"exec third descriptor", "~", "exec 3> notes.txt", outside, nil},
		{"cat dev null", "~", "cat /dev/null > notes.txt", outside, nil},
		{"cat dev null twice", "~", "cat /dev/null /dev/null > notes.txt", outside, nil},
		{"bash -c exec", "~", "bash -c 'exec > notes.txt'", outside, nil},
		{"false", "~", "false > notes.txt", outside, nil},
		{"cat stdin dev null", "~", "cat < /dev/null > notes.txt", outside, nil},
		{"cat stdin dev null after", "~", "cat > notes.txt < /dev/null", outside, nil},
		// An empty word prints nothing, and %s or %b of one neither.
		{"echo empty", "~", `echo "" > notes.txt`, outside, nil},
		{"echo empty single", "~", "echo '' > notes.txt", outside, nil},
		{"echo -e empty", "~", `echo -e "" > notes.txt`, outside, nil},
		{"printf %s empty", "~", "printf '%s' '' > notes.txt", outside, nil},
		{"printf %s alone", "~", "printf '%s' > notes.txt", outside, nil},
		{"printf -- %b%s", "~", "printf -- '%b%s' '' '' > notes.txt", outside, nil},
		// >& opens a file name on any descriptor.
		{"stdout by number csh form", "~", ": 1>&notes.txt", outside, nil},
		{"exec stdout by number csh form", "~", "exec 1>&notes.txt", outside, nil},
		{"stderr csh form", "~", ": 2>&notes.txt", outside, nil},
		// A group's redirection takes what its body prints.
		{"subshell group", "~", "( : ) > notes.txt", toss("notes.txt", "> notes.txt"), notDelete},
		{"subshell true", "~", "(true) > notes.txt", outside, nil},
		{"nested subshell", "~", "( ( : ) ) > notes.txt", outside, nil},
		{"brace group", "~", "{ :; } > notes.txt", toss("notes.txt", "> notes.txt"), notDelete},
		{"while toss form", "~", "while false; do :; done > notes.txt", toss("notes.txt", "> notes.txt"), nil},
		{"brace group clobber", "~", "{ true; } >| notes.txt", outside, nil},
		{"brace group stderr", "~", "{ :; } 2> notes.txt", outside, nil},
		{"nested brace group", "~", "{ { :; }; } > notes.txt", outside, nil},
		{"if compound", "~", "if false; then :; fi > notes.txt", outside, nil},
		{"if with subshell", "~", "if :; then ( : ); fi > notes.txt", outside, nil},
		{"while compound", "~", "while false; do :; done > notes.txt", outside, nil},
		{"until compound", "~", "until true; do :; done > notes.txt", outside, nil},
		{"for compound", "~", "for x in a b; do :; done > notes.txt", outside, nil},
		{"piped into group", "~", ": | { :; } > notes.txt", outside, nil},
		{"group over lines", "~", "{\n:\n} > notes.txt", outside, nil},
		{"function call", "~", "f() { :; }; f > notes.txt", outside, nil},
		{"function keyword call", "~", "function g { :; }; g > notes.txt", outside, nil},
		{"function brace on next line", "~", "f()\n{ :; }\nf > notes.txt", outside, nil},
		// Redirections that keep the file, and commands that print.
		{"append", "~", ": >> notes.txt", nil, nil},
		{"append both", "~", ": &>> notes.txt", nil, nil},
		{"exec append", "~", "exec >> notes.txt", nil, nil},
		{"exec a command", "~", "exec go version > notes.txt", nil, nil},
		{"exec dup", "~", "exec 2>&1", nil, nil},
		{"exec close", "~", "exec 3>&-", nil, nil},
		{"cat a file", "~", "cat zero > notes.txt", nil, nil},
		{"cat stdin", "~", "cat > notes.txt", nil, nil},
		{"process substitution", "~", "exec > >(tee -a notes.txt)", nil, nil},
		{"colon into process substitution", "~", ": > >(cat)", nil, nil},
		{"dup stderr", "~", ": 2>&1", nil, nil},
		{"dup to stdout", "~", "echo >&1", nil, nil},
		{"echo text", "~", "echo hi > notes.txt", nil, nil},
		{"printf text", "~", "printf 'x' > notes.txt", nil, nil},
		{"go test", "~", "go test ./... > notes.txt", nil, nil},
		{"subshell output", "~", "(echo hi) > notes.txt", nil, nil},
		{"group output", "~", "{ echo hi; } > notes.txt", nil, nil},
		{"if output", "~", "if :; then echo hi; fi > notes.txt", nil, nil},
		{"while reads a file", "~", "while read x; do :; done < zero > notes.txt", nil, nil},
		{"piped text into group", "~", "echo hi | { cat; } > notes.txt", nil, nil},
		{"group piped on", "~", "{ :; } | cat > notes.txt", nil, nil},
		{"group append", "~", "{ :; } >> notes.txt", nil, nil},
		{"select menu", "~", "select x in a; do :; done 2> notes.txt", nil, nil},
		{"case compound", "~", "case x in a) :;; esac > notes.txt", nil, nil},
		{"function that prints", "~", "f() { echo hi; }; f > notes.txt", nil, nil},
		{"function shadows true", "~", "true() { echo hi; }; true > notes.txt", nil, nil},
		{"function definition redirect", "~", "f() { :; } > notes.txt", nil, nil},
		{"function head then group", "~", "f()\n{ :; } > notes.txt", nil, nil},
		{"echo empty then option", "~", `echo "" -n > notes.txt`, nil, nil},
		{"printf %d empty", "~", "printf '%d' '' > notes.txt", nil, nil},
		{"printf %s text", "~", "printf '%s' x > notes.txt", nil, nil},
		{"cat stdin file", "~", "cat < zero > notes.txt", nil, nil},
		{"cat stdin dev null then file", "~", "cat < /dev/null < zero > notes.txt", nil, nil},
		{"dup stdout to stderr", "~", ": 1>&2", nil, nil},
		{"close stderr", "~", ": 2>&-", nil, nil},
		{"cat heredoc", "~", "cat <<'EOF' > notes.txt\nx\nEOF", nil, nil},
		// Nothing is lost.
		{"missing", "~", ": > missing", nil, nil},
		{"empty", "~", "> zero", nil, nil},
		{"folder", "~", ": > dir", nil, nil},
		{"dangling link", "~", ": > dangling", nil, nil},
		{"dev null", "~", ": > /dev/null", nil, nil},
		{"temp", "~", ": > " + tmp + "/f", nil, nil},
		{"mktemp var", "~", `D=$(mktemp -d); : > "$D/f"`, nil, nil},
		{"comment", "~", "true # : > notes.txt", nil, nil},
		// truncate drops each operand unless it only grows it.
		{"truncate zero", "~", "truncate -s 0 notes.txt",
			toss("notes.txt", "truncate -s 0 notes.txt"), notDelete},
		{"truncate shrink to 10", "~", "truncate -s 10 notes.txt", outside, nil},
		{"truncate by -5", "~", "truncate -s -5 notes.txt", outside, nil},
		{"truncate glued size", "~", "truncate -s0 notes.txt", outside, nil},
		{"truncate long size", "~", "truncate --size=0 notes.txt", outside, nil},
		{"truncate reference", "~", "truncate -r zero notes.txt", outside, nil},
		{"truncate reference and growth", "~", "truncate -r zero -s +1 notes.txt", outside, nil},
		{"truncate unread size", "~", `truncate -s "$N" notes.txt`, outside, nil},
		{"truncate after operand", "~", "truncate notes.txt -s 0", outside, nil},
		{"truncate symlink", "~", "truncate -s 0 ln", outside, nil},
		{"truncate glob", "~", "truncate -s 0 *.txt", outside, nil},
		{"gnu truncate", "~", "gtruncate -s 0 notes.txt", outside, nil},
		{"sudo truncate", "~", "sudo -u nobody truncate -s 0 notes.txt", outside, nil},
		{"truncate grow", "~", "truncate -s +1 notes.txt", nil, nil},
		{"truncate grow glued", "~", "truncate -s+1 notes.txt", nil, nil},
		{"truncate grow long", "~", "truncate --size=+1 notes.txt", nil, nil},
		{"truncate grow abbreviated", "~", "truncate -c --si +1 notes.txt", nil, nil},
		{"truncate missing and empty", "~", "truncate -s 0 missing zero", nil, nil},
		{"truncate temp", "~", "truncate -s 0 " + tmp + "/f", nil, nil},
		// dd drops what of= names.
		{"dd", "~", "dd if=/dev/zero of=notes.txt bs=1 count=1",
			toss("notes.txt", "dd if=/dev/zero of=notes.txt bs=1 count=1"), notDelete},
		{"gnu dd", "~", "gdd if=/dev/zero of=notes.txt count=1", outside, nil},
		{"dd quoted", "~", `dd if=/dev/zero of="notes.txt" count=1`, outside, nil},
		{"dd symlink", "~", "dd if=/dev/zero of=ln count=1", outside, nil},
		{"dd unset", "~", `dd if=/dev/zero of="$UNSET"`, []string{"which path `\"$UNSET\"` names"}, nil},
		{"dd unread operand", "~", "dd if=/dev/zero $ARGS", []string{"which path `$ARGS` names"}, nil},
		{"dd to dev", "~", "dd if=notes.txt of=/dev/null", nil, nil},
		{"dd read only", "~", "dd if=notes.txt bs=1 count=1", nil, nil},
		{"dd missing", "~", "dd if=/dev/zero of=missing count=1", nil, nil},
		// The walk reaches them where the shell runs them.
		{"bash -c", "~", "bash -c ': > notes.txt'", outside, nil},
		{"bash -c bare", "~", "bash -c '> notes.txt'", outside, nil},
		{"bash -c cd", "/", "bash -c 'cd ~ && : > notes.txt'", outside, nil},
		{"cd &&", "/", "cd ~ && : > notes.txt", outside, nil},
		{"cd ; bare", "/", "cd ~; > notes.txt", []string{"the cd before it may not have run"}, nil},
		{"subshell", "~", "(: > notes.txt)", outside, nil},
		{"eval", "~", "eval 'truncate -s 0 notes.txt'", outside, nil},
		{"substituted", "~", "echo $(: > notes.txt)", outside, nil},
		{"bash heredoc", "~", "bash <<'EOF'\n: > notes.txt\nEOF", outside, nil},
		{"unread truncate", "~", "truncate -s 0 notes.txt 'oops", []string{"could not read the shell text"}, nil},
		{"xargs truncate", "~", "echo notes.txt | xargs truncate -s 0",
			[]string{"belt does not read a truncate that xargs runs"}, nil},
	}
	runFileDiscardRows(t, newFileDiscardGuard(t, home), home, rows)
}

// TestFileDiscardReplaceInWorkTrees runs each replace shape over the files of
// a clone: committed and clean, changed, untracked, ignored, missing and
// empty, with the guard's real git backends.
func TestFileDiscardReplaceInWorkTrees(t *testing.T) {
	dirty := func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n", "d/f.txt": "mine\n"}) }
	untracked := func(r discardRepos) { r.local(map[string]string{"new.txt": "n\n"}) }
	ignored := func(r discardRepos) {
		r.write(r.clone, ".git/info/exclude", "*.log\n")
		r.local(map[string]string{"out.log": "x\n"})
	}
	empty := func(r discardRepos) { r.local(map[string]string{"empty.txt": ""}) }
	linkTo := func(name string) func(r discardRepos) {
		return func(r discardRepos) {
			dirty(r)
			if err := os.Symlink(filepath.Join(r.clone, name), filepath.Join(filepath.Dir(r.clone), "link")); err != nil {
				r.t.Fatal(err)
			}
		}
	}
	changed := []string{"this overwrites uncommitted work in ", "a.txt",
		"`toss-bin --safe-mode a.txt && : > a.txt`"}
	shapes := []struct{ name, command string }{
		{"redirect", ": > a.txt"},
		{"bare redirect", "> a.txt"},
		{"stderr redirect", ": 2> a.txt"},
		{"exec", "exec > a.txt"},
		{"cat dev null", "cat /dev/null > a.txt"},
		{"echo empty", `echo "" > a.txt`},
		{"csh form by number", ": 1>&a.txt"},
		{"subshell group", "( : ) > a.txt"},
		{"brace group", "{ :; } > a.txt"},
		{"function call", "f() { :; }; f > a.txt"},
		{"truncate", "truncate -s 0 a.txt"},
		{"dd", "dd if=/dev/zero of=a.txt count=1"},
	}
	var rows []repoRow
	for _, s := range shapes {
		rows = append(rows,
			repoRow{s.name + " clean", nil, s.command, nil},
			repoRow{s.name + " dirty", dirty, s.command, []string{"this overwrites uncommitted work in ", "a.txt"}},
			repoRow{s.name + " untracked", untracked, replaceOn(s.command, "new.txt"),
				[]string{"this overwrites `", "new.txt`, which is untracked"}},
			repoRow{s.name + " ignored", ignored, replaceOn(s.command, "out.log"), nil},
			repoRow{s.name + " missing", nil, replaceOn(s.command, "gone.txt"), nil},
			repoRow{s.name + " empty", empty, replaceOn(s.command, "empty.txt"), nil},
		)
	}
	rows = append(rows, []repoRow{
		{"toss-bin form", dirty, ": > a.txt", changed},
		{"append to dirty", dirty, ": >> a.txt", nil},
		{"grow dirty", dirty, "truncate -s +1 a.txt", nil},
		{"build output over dirty", dirty, "go test ./... > a.txt", nil},
		{"dd from dirty", dirty, "dd if=a.txt of=/dev/null", nil},
		{"link to dirty", linkTo("a.txt"), ": > ../link", []string{"this overwrites uncommitted work in ", "a.txt"}},
		{"link to clean", linkTo("b.txt"), "truncate -s 0 ../link", nil},
		{"cd into dirty", dirty, "cd d && : > f.txt", []string{"uncommitted work", "f.txt"}},
		{"bash -c dirty", dirty, "bash -c 'printf \"\" > a.txt'", []string{"uncommitted work"}},
		{"sudo truncate dirty", dirty, "sudo truncate -s 0 a.txt", []string{"uncommitted work"}},
		{"git data", nil, ": > .git/config", []string{"this overwrites `", ".git/config`, which is git's own data"}},
	}...)
	runRepoRows(t, rows)
}

// replaceOn is a replace command from the table pointed at file.
func replaceOn(command, file string) string {
	if command == "dd if=/dev/zero of=a.txt count=1" {
		return "dd if=/dev/zero of=" + file + " count=1"
	}
	return strings.TrimSuffix(command, "a.txt") + file
}

// TestFileDiscardReplaceLinkedWorktreeGitFile: a linked worktree's .git is
// a file git reads to find the repository, so emptying it is a loss.
func TestFileDiscardReplaceLinkedWorktreeGitFile(t *testing.T) {
	runRepoRows(t, []repoRow{{
		name: "linked .git",
		setup: func(r discardRepos) {
			r.worktree()
		},
		command: ": > ../wt/.git",
		want:    []string{"wt/.git`, which is git's own data"},
	}})
}
