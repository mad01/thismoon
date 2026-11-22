package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The rows below run each language's recursive delete through file-discard,
// as Check reads the command: nothing runs.

// TestFileDiscardScriptsOutsideWorkTrees covers the four languages over a
// temp folder, which passes, and a folder in no work tree, which denies.
func TestFileDiscardScriptsOutsideWorkTrees(t *testing.T) {
	home := t.TempDir()
	tmp := tempFixture(t)
	g := newFileDiscardGuard(t, home)
	writeTree(t, home, map[string]string{"work/notes.txt": "n\n", "work/sub/x.txt": "x\n"})
	writeTree(t, tmp, map[string]string{"cache/x.o": "o"})
	work, cache := filepath.Join(home, "work"), filepath.Join(tmp, "cache")
	noTree := "which is in no git work tree"
	unknown := "belt cannot tell which path the `rmtree` call"
	rows := []fileDiscardRow{
		{"python temp", "~", py(`import shutil; shutil.rmtree("%s")`, cache), nil, nil},
		{"python outside", "~", py(`import shutil; shutil.rmtree("%s")`, work),
			[]string{noTree, "toss-bin --safe-mode -r " + work}, nil},
		{"python relative", "~", py(`import shutil; shutil.rmtree("work")`), []string{noTree}, nil},
		{"python tilde is a folder name", "~", py(`import shutil; shutil.rmtree("~")`), nil, nil},
		{"python home", "~", py(`import shutil; shutil.rmtree("%s")`, home), []string{"`! <command>`"}, nil},
		{"python from import", "~", py(`from shutil import rmtree; rmtree("%s", ignore_errors=True)`, work),
			[]string{noTree}, nil},
		{"python alias", "~", py(`from shutil import rmtree as zap; zap("%s")`, work), []string{noTree}, nil},
		{"python assigned alias", "~", py(`import shutil; zap = shutil.rmtree; zap("%s")`, work),
			[]string{noTree}, nil},
		{"python under uv run", "~", "uv run --with x " + py(`import shutil; shutil.rmtree("%s")`, work),
			[]string{noTree}, nil},
		{"python Path", "~", py(`import shutil, pathlib; shutil.rmtree(pathlib.Path("%s"))`, work),
			[]string{noTree}, nil},
		{"python raw string", "~", py(`import shutil; shutil.rmtree(r"%s")`, work), []string{noTree}, nil},
		{"python argv", tmp, py(`import shutil, sys; shutil.rmtree(sys.argv[1])`) + " x", []string{unknown}, nil},
		{"python f-string", tmp, py(`import shutil; shutil.rmtree(f"{d}/x")`), []string{unknown}, nil},
		{"python dir_fd", tmp, py(`import shutil; shutil.rmtree("x", dir_fd=3)`), []string{unknown}, nil},
		{"python shell variable", tmp, `python3 -c "import shutil; shutil.rmtree('$D')"`, []string{unknown}, nil},
		{"python chdir", tmp, py(`import os, shutil; os.chdir("/"); shutil.rmtree("x")`),
			[]string{"from the folder the script changes to"}, nil},
		{"python behind sudo", "~", "sudo " + py(`import shutil; shutil.rmtree("%s")`, work),
			[]string{noTree}, nil},
		{"python print only", "~", py(`print("hello")`), nil, nil},
		{"node temp", "~", node(`require("fs").rmSync("%s", {recursive: true, force: true})`, cache), nil, nil},
		{"node outside", "~", node(`require("fs").rmSync("%s", {recursive: true, force: true})`, work),
			[]string{noTree}, nil},
		{"node rmdirSync", "~", node(`require("fs").rmdirSync("%s", {recursive: true})`, work),
			[]string{noTree}, nil},
		{"node promises", "~", node(`require("fs").promises.rm("%s", {recursive: true})`, work),
			[]string{noTree}, nil},
		{"node alias", "~", node(`const {rmSync: zap} = require("fs"); zap("%s", {recursive: true})`, work),
			[]string{noTree}, nil},
		{"node assigned alias", "~",
			node(`const fs = require("fs"); const zap = fs.rmSync; zap("%s", {recursive: true})`, work),
			[]string{noTree}, nil},
		{"bun eval", "~", fmt.Sprintf(`bun -e 'require("fs").rmSync("%s", {recursive: true})'`, work),
			[]string{noTree}, nil},
		{"bun temp", "~", fmt.Sprintf(`bun -e 'require("fs").rmSync("%s", {recursive: true})'`, cache), nil, nil},
		{"node options variable", "~", node(`const o = {}; require("fs").rmSync("%s", o)`, work),
			[]string{noTree}, nil},
		{"node not recursive", "~", node(`require("fs").rmSync("%s", {recursive: false})`, work), nil, nil},
		{"node file only", "~", node(`require("fs").rmSync("%s/notes.txt")`, work), nil, nil},
		{"node print flag", "~", fmt.Sprintf(`node -p 'require("fs").rmSync("%s", {recursive: true})'`, work),
			[]string{noTree}, nil},
		{"node template", tmp, node("require(\"fs\").rmSync(`${d}`, {recursive: true})"),
			[]string{"belt cannot tell which path the `rmSync` call"}, nil},
		{"node import only", "~", node(`const {rmSync} = require("fs"); console.log(1)`), nil, nil},
		{"ruby temp", "~", ruby(`require "fileutils"; FileUtils.rm_rf("%s")`, cache), nil, nil},
		{"ruby rm_rf", "~", ruby(`require "fileutils"; FileUtils.rm_rf("%s")`, work), []string{noTree}, nil},
		{"ruby rm_r bare", "~", ruby(`FileUtils.rm_r "%s" if true`, work), []string{noTree}, nil},
		{"ruby remove_dir", "~", ruby(`FileUtils.remove_dir("%s")`, work), []string{noTree}, nil},
		{"ruby remove_entry_secure", "~", ruby(`FileUtils.remove_entry_secure("%s")`, work),
			[]string{noTree}, nil},
		{"ruby list", "~", ruby(`FileUtils.rm_rf(["%s", "%s"])`, cache, work), []string{noTree}, nil},
		{"ruby interpolation", tmp, ruby(`FileUtils.rm_rf("#{d}")`),
			[]string{"belt cannot tell which path the `rm_rf` call"}, nil},
		{"ruby symbol", "~", ruby(`p FileUtils.method(:rm_rf)`), nil, nil},
		{"perl temp", "~", perl(`use File::Path qw(rmtree); rmtree("%s")`, cache), nil, nil},
		{"perl rmtree", "~", perl(`use File::Path qw(rmtree); rmtree("%s")`, work), []string{noTree}, nil},
		{"perl remove_tree", "~", perl(`use File::Path qw(remove_tree); remove_tree("%s", {safe => 1})`, work),
			[]string{noTree}, nil},
		{"perl old flags", "~", perl(`File::Path::rmtree(["%s"], 0, 1)`, work), []string{noTree}, nil},
		{"perl variable", tmp, perl(`use File::Path qw(rmtree); rmtree($d)`), []string{unknown}, nil},
		{"perl import only", "~", perl(`use File::Path qw(rmtree remove_tree); print 1`), nil, nil},
	}
	runFileDiscardRows(t, g, home, rows)
}

// TestFileDiscardScriptsReadFromInput covers the scripts an interpreter
// reads from standard input, a file the command wrote, and the scripts
// xargs and find hand their inputs to.
func TestFileDiscardScriptsReadFromInput(t *testing.T) {
	home := t.TempDir()
	g := newFileDiscardGuard(t, home)
	writeTree(t, home, map[string]string{"work/notes.txt": "n\n"})
	work := filepath.Join(home, "work")
	noTree := "which is in no git work tree"
	body := fmt.Sprintf("import shutil\nshutil.rmtree(%q)\n", work)
	rows := []fileDiscardRow{
		{"heredoc", "~", "python3 - <<'EOF'\n" + body + "EOF", []string{noTree}, nil},
		{"heredoc comment", "~", "python3 <<'EOF'\n# shutil.rmtree(x)\nprint(1)\nEOF", nil, nil},
		{"heredoc with a shell variable", "~", "python3 <<EOF\nimport shutil\nshutil.rmtree(\"$X\")\nEOF",
			[]string{"belt cannot read all of", "`rmtree`"}, nil},
		{"here-string", "~", fmt.Sprintf(`python3 <<< 'import shutil; shutil.rmtree("%s")'`, work),
			[]string{noTree}, nil},
		{"echo pipe", "~", fmt.Sprintf(`echo 'import shutil; shutil.rmtree("%s")' | python3`, work),
			[]string{noTree}, nil},
		{"written then run", "~", "cat > w.py <<'EOF'\n" + body + "EOF\npython3 w.py", []string{noTree}, nil},
		{"ruby heredoc", "~", fmt.Sprintf("ruby <<'EOF'\nFileUtils.rm_rf('%s')\nEOF", work),
			[]string{noTree}, nil},
		{"inside bash -c", "~", fmt.Sprintf(`bash -c "python3 -c 'import shutil; shutil.rmtree(\"%s\")'"`, work),
			[]string{noTree}, nil},
		{"xargs", "~", `echo x | xargs python3 -c 'import shutil, sys; shutil.rmtree(sys.argv[1])'`,
			[]string{"the `python3` script xargs runs"}, nil},
		{"find -exec", "~", `find . -name x -exec node -e 'require("fs").rmSync(process.argv[1], ` +
			`{recursive: true})' {} \;`, []string{"the `node` script find runs"}, nil},
		{"find -execdir relative", "~", `find . -name x -execdir python3 -c 'import shutil; ` +
			`shutil.rmtree("build")' \;`, []string{"belt cannot tell which path `build` names"}, nil},
	}
	runFileDiscardRows(t, g, home, rows)
}

// TestFileDiscardScriptFiles covers a script file the command names: an
// interpreter's operand, a #! script run by path, one past maxScriptSize,
// and a missing one.
func TestFileDiscardScriptFiles(t *testing.T) {
	home := t.TempDir()
	tmp := tempFixture(t)
	g := newFileDiscardGuard(t, home)
	work := filepath.Join(home, "work")
	writeTree(t, home, map[string]string{
		"work/notes.txt": "n\n",
		"bin/clean.py":   fmt.Sprintf("import shutil\n\nshutil.rmtree(%q)\n", work),
		"bin/clean.js":   fmt.Sprintf("const fs = require('fs')\nfs.rmSync('%s', { recursive: true })\n", work),
		"bin/clean.rb":   fmt.Sprintf("require 'fileutils'\nFileUtils.rm_r('%s')\n", work),
		"bin/clean.pl":   fmt.Sprintf("use File::Path 'remove_tree';\nremove_tree('%s');\n", work),
		"bin/clean":      fmt.Sprintf("#!/usr/bin/env python3\nimport shutil\nshutil.rmtree(%q)\n", work),
		"bin/tmp.py":     fmt.Sprintf("import shutil\nshutil.rmtree(%q)\n", filepath.Join(tmp, "x")),
		"bin/plain.py":   "print('hello')\n",
		"bin/big.py":     "import shutil\nshutil.rmtree(d)\n" + strings.Repeat("#\n", maxScriptSize),
		"bin/tool.sh":    fmt.Sprintf("#!/bin/sh\necho 'shutil.rmtree(%q)'\n", work),
	})
	noTree := "which is in no git work tree"
	rows := []fileDiscardRow{
		{"python file", "~", "python3 bin/clean.py", []string{noTree}, nil},
		{"python file by full path", "/", "python3 " + filepath.Join(home, "bin/clean.py"), []string{noTree}, nil},
		{"python file in a temp folder", "~", "python3 bin/tmp.py", nil, nil},
		{"python file that deletes nothing", "~", "python3 bin/plain.py", nil, nil},
		{"node file", "~", "node bin/clean.js", []string{noTree}, nil},
		{"ruby file", "~", "ruby bin/clean.rb", []string{noTree}, nil},
		{"perl file", "~", "perl -w bin/clean.pl", []string{noTree}, nil},
		{"#! script by path", "~", "./bin/clean", []string{noTree}, nil},
		{"shell script by path", "~", "./bin/tool.sh", nil, nil},
		{"file past the size cap", "~", "python3 bin/big.py", []string{"belt cannot read all of"}, nil},
		{"missing file", "~", "python3 bin/none.py", nil, nil},
	}
	runFileDiscardRows(t, g, home, rows)
}

// TestFileDiscardScriptsInWorkTrees judges a script's delete inside a work
// tree with the real git backends, the way rm -r is judged there.
func TestFileDiscardScriptsInWorkTrees(t *testing.T) {
	runRepoRows(t, []repoRow{
		{"python committed tree", nil, py(`import shutil; shutil.rmtree("d")`), nil},
		{"python dirty tree", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			py(`import shutil; shutil.rmtree("d")`), []string{"`d/f.txt` holds content no commit has"}},
		{"python untracked", func(r discardRepos) { r.local(map[string]string{"d/new.txt": "n\n"}) },
			py(`import shutil; shutil.rmtree("d")`), []string{"d/new.txt`, which is untracked"}},
		{"python ignored", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "build/\n")
			r.local(map[string]string{"build/out.o": "o"})
		}, py(`import shutil; shutil.rmtree("build")`), nil},
		{"python top", nil, py(`import shutil; shutil.rmtree(".")`), []string{"the top of a git work tree"}},
		{"python git data", nil, py(`import shutil; shutil.rmtree(".git")`), []string{"git's own data"}},
		{"node dirty tree", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			node(`require("fs").rmSync("d", {recursive: true})`), []string{"`d/f.txt`"}},
		{"ruby committed tree", nil, ruby(`FileUtils.rm_rf("d")`), nil},
		{"perl dirty tree", func(r discardRepos) { r.local(map[string]string{"d/f.txt": "mine\n"}) },
			perl(`use File::Path qw(remove_tree); remove_tree("d")`), []string{"`d/f.txt`"}},
		{"script file over a dirty tree", func(r discardRepos) {
			r.local(map[string]string{"d/f.txt": "mine\n", "clean.py": "import shutil\nshutil.rmtree('d')\n"})
		}, "python3 clean.py", []string{"`d/f.txt`"}},
	})
}

// py, node, ruby and perl write an inline script run by each language,
// with any args filled in.
func py(script string, args ...any) string {
	return "python3 -c '" + fmt.Sprintf(script, args...) + "'"
}

func node(script string, args ...any) string {
	return "node -e '" + fmt.Sprintf(script, args...) + "'"
}

func ruby(script string, args ...any) string {
	return "ruby -rfileutils -e '" + fmt.Sprintf(script, args...) + "'"
}

func perl(script string, args ...any) string {
	return "perl -e '" + fmt.Sprintf(script, args...) + "'"
}

// TestFileDiscardScriptFileUnreadable passes a script file belt cannot
// open: the interpreter cannot run it either.
func TestFileDiscardScriptFileUnreadable(t *testing.T) {
	home := t.TempDir()
	g := newFileDiscardGuard(t, home)
	p := filepath.Join(home, "locked.py")
	writeTree(t, home, map[string]string{"locked.py": "import shutil\nshutil.rmtree(d)\n"})
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if f, err := os.Open(p); err == nil {
		_ = f.Close()
		t.Skip("running as a user that reads any file")
	}
	runFileDiscardRows(t, g, home, []fileDiscardRow{{"unreadable", "~", "python3 locked.py", nil, nil}})
}
