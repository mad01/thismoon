package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFileDiscardCopyOutsideWorkTrees covers the copy rows that need no
// git: cp, mv, install and ln -f over a file outside every work tree, into
// a folder, the forms that keep the old file, and the walk and the
// consumers around them.
func TestFileDiscardCopyOutsideWorkTrees(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{
		"notes.txt": "n", "zero": "", "dir/notes.txt": "n", "dir2/data.bin": "d",
		"src/notes.txt": "s", "src/new": "s", "src2/x y.txt": "s", "bak/notes.txt": "n",
		"bak/notes.txt.bak": "b",
	})
	for link, to := range map[string]string{"ln": "notes.txt", "dangling": "gone", "dirlink": "dir"} {
		if err := os.Symlink(filepath.Join(home, to), filepath.Join(home, link)); err != nil {
			t.Fatal(err)
		}
	}
	tmp := tempFixture(t)
	writeTree(t, tmp, map[string]string{"f": "f", "notes.txt": "n"})
	outside := []string{"this overwrites `", "in no git work tree and no temp folder",
		"An overwrite passes only when"}
	toss := func(file, command string) []string {
		return append(outside, "`toss-bin --safe-mode "+file+" && "+command+"`")
	}
	in := func(file string) []string { return append(outside, "/"+file+"`") }
	rows := []fileDiscardRow{
		// The destination is the one file written.
		{"cp", "~", "cp src/new notes.txt", toss("notes.txt", "cp src/new notes.txt"), []string{"-r"}},
		{"mv", "~", "mv src/new notes.txt", outside, nil},
		{"mv -f", "~", "mv -f src/new notes.txt", outside, nil},
		{"install", "~", "install -m 644 src/new notes.txt", outside, nil},
		{"ln -sf", "~", "ln -sf src/new notes.txt", outside, nil},
		{"ln -f", "~", "ln -f src/new notes.txt", outside, nil},
		{"gnu names", "~", "gcp src/new notes.txt", outside, nil},
		{"gnu mv", "~", "gmv src/new notes.txt", outside, nil},
		{"gnu install", "~", "ginstall src/new notes.txt", outside, nil},
		{"gnu ln", "~", "gln -sf src/new notes.txt", outside, nil},
		{"option after operands", "~", "cp src/new notes.txt -v", outside, nil},
		{"cp -n then -f", "~", "cp -n -f src/new notes.txt", outside, nil},
		{"cp writes through a symlink", "~", "cp src/new ln", in("notes.txt"), nil},
		// GNU's none and off make no backup, and -b or a bare --backup after
		// them keeps that.
		{"backup none", "~", "cp --backup=none src/new notes.txt", outside, nil},
		{"backup off", "~", "mv --backup=off src/new notes.txt", outside, nil},
		{"backup none prefix", "~", "ln -sf --backup=no src/new notes.txt", outside, nil},
		{"backup none then -b", "~", "cp --backup=none -b src/new notes.txt", outside, nil},
		{"backup none then bare", "~", "install --backup=none --backup src/new notes.txt", outside, nil},
		{"backup numbered then none", "~", "cp --backup=numbered --backup=none src/new notes.txt", outside, nil},
		{"absolute", "/", "cp " + home + "/src/new " + home + "/notes.txt", outside, nil},
		// A folder takes a file of the source's name.
		{"into folder", "~", "cp src/notes.txt dir/", toss("dir/notes.txt", "cp src/notes.txt dir/"), nil},
		{"into folder no slash", "~", "mv src/notes.txt dir", in("dir/notes.txt"), nil},
		{"into folder link", "~", "cp src/notes.txt dirlink", in("notes.txt"), nil},
		{"target folder", "~", "cp -t dir src/notes.txt", in("dir/notes.txt"), nil},
		{"target folder long", "~", "mv --target-directory=dir src/new src/notes.txt", in("dir/notes.txt"), nil},
		{"several into folder", "~", "cp src/new src/notes.txt dir", in("dir/notes.txt"), nil},
		{"glob into folder", "~", "cp src/*.txt dir/", in("dir/notes.txt"), nil},
		{"ln into folder", "~", "ln -sf ../src/notes.txt dir/", in("dir/notes.txt"), nil},
		{"ln one operand", "~/dir", "ln -sf ../src/notes.txt", in("dir/notes.txt"), nil},
		{"recursive file into folder", "~", "cp -r src/notes.txt dir/", in("dir/notes.txt"), nil},
		{"parents", "~", "cp --parents notes.txt dir/", in("dir/notes.txt"), nil},
		{"pwd folder", "~", `cp src/notes.txt "$PWD/dir/"`, in("dir/notes.txt"), nil},
		// What keeps the old file, or writes none.
		{"no clobber", "~", "cp -n src/new notes.txt", nil, nil},
		{"no clobber long", "~", "cp --no-clobber src/new notes.txt", nil, nil},
		{"update none", "~", "cp --update=none src/new notes.txt", nil, nil},
		{"cp -f then -n", "~", "cp -f -n src/new notes.txt", nil, nil},
		{"backup", "~", "cp -b src/new notes.txt", nil, nil},
		{"backup long", "~", "cp --backup=numbered src/new notes.txt", nil, nil},
		{"backup none then numbered", "~", "cp --backup=none --backup=numbered src/new notes.txt", nil, nil},
		{"backup empty after none", "~", "cp --backup=none --backup= src/new notes.txt", nil, nil},
		{"backup ambiguous", "~", "cp --backup=n src/new notes.txt", nil, nil},
		{"mv suffix", "~", "mv -S .old src/new notes.txt", nil, nil},
		{"mv no clobber", "~", "mv -n src/new notes.txt", nil, nil},
		{"mv exchange", "~", "mv --exchange src/new notes.txt", nil, nil},
		{"ln without force", "~", "ln -s src/new notes.txt", nil, nil},
		{"ln backup", "~", "ln -sf --backup src/new notes.txt", nil, nil},
		{"install folders", "~", "install -d dir notes.txt", nil, nil},
		{"help", "~", "cp --help notes.txt", nil, nil},
		{"one operand", "~", "cp notes.txt", nil, nil},
		{"several onto a file", "~", "cp src/new src/notes.txt notes.txt", nil, nil},
		{"target not a folder", "~", "cp -t notes.txt src/new", nil, nil},
		{"no target folder", "~", "mv -T src/new dir", nil, nil},
		{"ln -n onto a folder link", "~", "ln -sfn src/new dirlink", nil, nil},
		{"mv onto a symlink", "~", "mv src/new ln", nil, nil},
		{"recursive merge", "~", "cp -r src dir/", nil, nil},
		{"missing", "~", "cp src/new missing", nil, nil},
		{"empty", "~", "mv src/new zero", nil, nil},
		{"dangling link", "~", "mv src/new dangling", nil, nil},
		{"new name in folder", "~", "cp src/new dir/", nil, nil},
		{"temp", "~", "cp src/new " + tmp + "/f", nil, nil},
		{"temp folder", "~", "cp src/notes.txt " + tmp, nil, nil},
		{"mktemp", "~", `D=$(mktemp -d); cp notes.txt "$D"`, nil, nil},
		{"mktemp slash", "~", `D=$(mktemp -d); cp notes.txt "$D/"`, nil, nil},
		{"a wrapped command's argument", "~", "sudo apt install x notes.txt", nil, nil},
		{"git mv", "~", "git mv src/new notes.txt", nil, nil},
		// Paths belt cannot place.
		{"unset", "~", `cp src/new "$UNSET"`, []string{"which path `\"$UNSET\"` names"}, nil},
		{"variable source into folder", "~", `cp "$F" dir/`, []string{"which path `\"$F\"` names"}, nil},
		{"glob destination", "~", "cp src/new di*", []string{"which path `di*` names"}, nil},
		{"cd ; cp", "/", "cd ~; cp src/new notes.txt", []string{"the cd before it may not have run"}, nil},
		// The walk reaches them where the shell runs them.
		{"sudo", "~", "sudo cp src/new notes.txt", outside, nil},
		{"sudo -u", "~", "sudo -u nobody mv src/new notes.txt", outside, nil},
		{"env", "~", "env X=1 cp src/new notes.txt", outside, nil},
		{"busybox", "~", "busybox cp src/new notes.txt", outside, nil},
		{"bash -c", "~", "bash -c 'cp src/new notes.txt'", outside, nil},
		{"cd &&", "/", "cd ~ && mv src/new notes.txt", outside, nil},
		{"unread", "~", "cp src/new notes.txt 'oops", []string{"could not read the shell text"}, nil},
		// A consumer's inputs.
		{"xargs -J into folder", "~", "find src -name notes.txt | xargs -J% mv % dir/", in("dir/notes.txt"), nil},
		{"xargs -J new name", "~", "find src -name new | xargs -J% mv % dir/", nil, nil},
		{"xargs -I into folder", "~", "find src -name notes.txt | xargs -I{} cp {} dir/", in("dir/notes.txt"), nil},
		{"xargs -0 -t", "~", "find src -name notes.txt -print0 | xargs -0 cp -t dir", in("dir/notes.txt"), nil},
		{"xargs -0 -t new name", "~", "find src -name new -print0 | xargs -0 cp -t dir", nil, nil},
		{"xargs -t temp", "~", "find src -print0 | xargs -0 cp -t " + tmp, nil, nil},
		{"xargs appends the destination", "~", "find src -name new -print0 | xargs -0 cp",
			[]string{"the last path the feeder hands `cp` is where it writes"}, nil},
		{"xargs cuts a path", "~", "find src2 -name '*.txt' | xargs cp -t dir2",
			[]string{"cut at blanks and quotes", "`data.bin`", "xargs -0"}, nil},
		{"xargs cuts no path", "~", "find src -name '*.txt' | xargs cp -t dir2", nil, nil},
		{"find -exec into folder", "~", "find src -name notes.txt -exec cp {} dir/ \\;", in("dir/notes.txt"), nil},
		{"find -exec -t", "~", "find src -name notes.txt -exec mv -t dir {} +", in("dir/notes.txt"), nil},
		{"find -exec new name", "~", "find src -name new -exec cp {} dir/ \\;", nil, nil},
		// A folder outside every work tree that a feeder hands paths from is
		// judged whole, as a delete's is; home is no user-only loss there,
		// since only the files below it are written.
		{"destination input", "~", "find . -maxdepth 1 -name notes.txt | xargs -I{} cp src/new {}",
			outside, []string{"the home directory", "only the user may run it"}},
		{"destination input in temp", tmp, "find . -maxdepth 1 -name notes.txt | xargs -I{} cp " + home + "/src/new {}", nil, nil},
		{"suffix", "~/bak", "find . -name notes.txt -exec cp {} {}.bak \\;", outside, nil},
		{"suffix in temp", tmp, "find . -name notes.txt -exec cp {} {}.bak \\;", nil, nil},
		{"suffix root", "~", "find dir/notes.txt -exec cp {} {}.bak \\;", nil, nil},
		{"suffix root over a file", "~", "find bak/notes.txt -exec cp {} {}.bak \\;", in("bak/notes.txt.bak"), nil},
		{"prefix", "~", "find src -name new -exec cp {} old-{} \\;", []string{"which path `old-{}` names"}, nil},
		{"script", "~", `find src -name notes.txt -exec sh -c 'cp "$1" dir/' _ {} \;`, in("dir/notes.txt"), nil},
		{"script new name", "~", `find src -name new -exec sh -c 'cp "$1" dir/' _ {} \;`, nil, nil},
		{"parallel", "~", "parallel cp src/{} dir/ ::: notes.txt", in("dir/notes.txt"), nil},
		{"parallel new name", "~", "parallel cp src/{} dir/ ::: new", nil, nil},
		{"substitution into folder", "~", "cp $(find src -name notes.txt) dir/", in("dir/notes.txt"), nil},
		{"substitution new name", "~", "cp $(find src -name new) dir/", nil, nil},
		{"execdir relative", "~", "find src -name new -execdir cp {} dir/ \\;",
			[]string{"relative to the folder of each match"}, nil},
	}
	runFileDiscardRows(t, newFileDiscardGuard(t, home), home, rows)
}

// TestFileDiscardCopyInWorkTrees runs each copy shape over the files of a
// clone: committed and clean, changed, untracked, ignored and missing,
// with the guard's real git backends. A shape onto a top-level file takes
// a.txt, new.txt and out.log; a shape into d/ takes the file of the same
// name there.
func TestFileDiscardCopyInWorkTrees(t *testing.T) {
	srcs := func(r discardRepos) {
		for _, name := range []string{"new", "a.txt", "f.txt", "new.txt", "out.log", "gone.txt"} {
			r.write(filepath.Dir(r.clone), "srcs/"+name, "s\n")
		}
	}
	dirty := func(r discardRepos) {
		srcs(r)
		r.local(map[string]string{"a.txt": "mine\n", "d/f.txt": "mine\n"})
	}
	untracked := func(r discardRepos) {
		srcs(r)
		r.local(map[string]string{"new.txt": "n\n", "d/new.txt": "n\n", "a.txt.bak": "b\n"})
	}
	ignored := func(r discardRepos) {
		srcs(r)
		r.write(r.clone, ".git/info/exclude", "*.log\n")
		r.local(map[string]string{"out.log": "x\n", "d/out.log": "x\n"})
	}
	onto := []string{"cp ../srcs/new %s", "mv ../srcs/new %s", "install ../srcs/new %s", "ln -sf ../srcs/new %s"}
	into := []string{
		"cp ../srcs/%s d/", "mv -t d ../srcs/%s", "find ../srcs -name %s | xargs -J%% mv %% d/",
		"find ../srcs -name %s -exec cp {} d/ \\;",
	}
	var rows []repoRow
	add := func(command, clean, prefix string) {
		rows = append(rows,
			repoRow{command + " clean", srcs, fmtCommand(command, clean), nil},
			repoRow{command + " dirty", dirty, fmtCommand(command, clean),
				[]string{"this overwrites uncommitted work in ", clean}},
			repoRow{command + " untracked", untracked, fmtCommand(command, "new.txt"),
				[]string{"this overwrites `", prefix + "new.txt`, which is untracked"}},
			repoRow{command + " ignored", ignored, fmtCommand(command, "out.log"), nil},
			repoRow{command + " missing", srcs, fmtCommand(command, "gone.txt"), nil},
		)
	}
	for _, c := range onto {
		add(c, "a.txt", "/")
	}
	for _, c := range into {
		add(c, "f.txt", "d/")
	}
	rows = append(rows, []repoRow{
		{"no clobber over dirty", dirty, "cp -n ../srcs/new a.txt", nil},
		{"backup over dirty", dirty, "mv --backup ../srcs/new a.txt", nil},
		{"no backup over dirty", dirty, "cp --backup=off ../srcs/new a.txt", []string{"uncommitted work", "a.txt"}},
		{"git data", srcs, "cp ../srcs/new .git/config", []string{".git/config`, which is git's own data"}},
		{"suffix over untracked", untracked, "find . -maxdepth 1 -name a.txt -exec cp {} {}.bak \\;",
			[]string{"a.txt.bak`, which is untracked"}},
		{"suffix over nothing", srcs, "find . -maxdepth 1 -name a.txt -exec cp {} {}.bak \\;", nil},
		{"tracked files into dirty folder", dirty, "git ls-files -z d | xargs -0 cp -t d", []string{"f.txt"}},
	}...)
	runRepoRows(t, rows)
}

// fmtCommand puts file in place of each %s in command, and % in place of
// %%.
func fmtCommand(command, file string) string {
	return strings.ReplaceAll(strings.ReplaceAll(command, "%s", file), "%%", "%")
}

// TestReadXargsWords covers file-discard's read of xargs's options, and
// checks the script guard's token read (readXargs) agrees, since both go
// through scanXargs.
func TestReadXargsWords(t *testing.T) {
	tests := []struct {
		line, command, replace, cut, why string
		batch, file                      bool
	}{
		{line: "xargs rm", command: "rm", cut: xargsSplit},
		{line: "xargs -0 rm", command: "rm"},
		{line: "xargs -0 -d x rm", command: "rm", cut: "x"},
		{line: "xargs -d x -0 rm", command: "rm"},
		{line: `xargs -d '\n' rm`, command: "rm", cut: "\n"},
		{line: `xargs -d "$D" rm`, command: "rm", why: "belt cannot read the xargs delimiter"},
		{line: "xargs -d xy rm", command: "rm", why: "belt cannot read the xargs delimiter `xy`"},
		{line: "xargs -I{} cp {} d", command: "cp", replace: "{}", cut: xargsSplit},
		{line: "xargs -0I{} cp {} d", command: "cp", replace: "{}"},
		{line: "xargs -i cp {} d", command: "cp", replace: "{}", cut: xargsSplit},
		{line: "xargs --replace=@ cp @ d", command: "cp", replace: "@", cut: xargsSplit},
		{line: "xargs -J% mv % d", command: "mv", replace: "%", batch: true, cut: xargsSplit},
		{line: "xargs -J % mv % d", command: "mv", replace: "%", batch: true, cut: xargsSplit},
		{line: "xargs -a list rm", command: "rm", file: true, cut: xargsSplit},
		{line: "xargs -n 1 rm", command: "rm", cut: xargsSplit},
		{line: "xargs --max-args 1 rm", command: "rm", cut: xargsSplit},
		{line: "xargs -- rm", command: "rm", cut: xargsSplit},
		{line: `xargs "$X" rm`, command: `"$X"`, cut: xargsSplit},
		{line: "xargs -Z rm", command: "rm", cut: xargsSplit},
		{line: "xargs -n", cut: xargsSplit},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			cmds, ok := lexCommands(tt.line)
			if !ok || len(cmds) != 1 {
				t.Fatalf("lexCommands(%q) = %v, %v", tt.line, cmds, ok)
			}
			x, command := readXargsWords(cmds[0][1:])
			head := ""
			if len(command) > 0 {
				head = command[0].raw
			}
			why := x.delimWhy()
			switch {
			case head != tt.command:
				t.Errorf("command starts at %q, want %q", head, tt.command)
			case x.replace != tt.replace || x.batch != tt.batch:
				t.Errorf("replace %q batch %v, want %q %v", x.replace, x.batch, tt.replace, tt.batch)
			case tt.why == "" && (why != "" || x.cut() != tt.cut):
				t.Errorf("cut %q (%q), want %q", x.cut(), why, tt.cut)
			case tt.why != "" && !strings.Contains(why, tt.why):
				t.Errorf("delimWhy %q, want %q", why, tt.why)
			case (x.itemFile > 0) != tt.file:
				t.Errorf("itemFile %d, want a file %v", x.itemFile, tt.file)
			}
			tokens := readXargs(consumerTokens(cmds[0]))
			if tokens.replace != x.replace || tokens.batch != x.batch || tokens.itemFile != x.itemFile ||
				tokens.split != x.split || len(tokens.command) != len(command) {
				t.Errorf("readXargs reads %+v, readXargsWords %+v", tokens, x)
			}
		})
	}
}
