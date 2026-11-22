package guard

import (
	"slices"
	"testing"
)

// commitFiles writes files into the clone's work tree and commits them.
func commitFiles(r discardRepos, files map[string]string) {
	r.t.Helper()
	r.local(files)
	r.git(r.clone, "add", "-A")
	r.git(r.clone, "commit", "-q", "-m", "fixture")
}

// TestFileDiscardTrustsCommittedCode covers the trust rule with the real git
// backends: a delete whose path belt cannot read passes in a build file or
// a script git holds committed, and denies in one with an edit, an
// untracked or ignored one, and one the command changes first. A literal
// path in committed code still takes the ladder.
func TestFileDiscardTrustsCommittedCode(t *testing.T) {
	makefile := makefileText("clean:", ">rm -rf $(NOPE)/x", "tree:", ">rm -rf d",
		"script:", ">python3 tools/clean.py", "inline:", ">python3 -c 'import shutil,sys; shutil.rmtree(sys.argv[1])' x",
		"quiet:", ">echo hi")
	pkg := `{"scripts": {"clean": "rm -rf \"$X/y\""}}`
	justfile := "clean:\n    rm -rf \"$X/y\"\n"
	script := "import os, shutil\nshutil.rmtree(os.environ['X'])\n"
	committed := func(r discardRepos) {
		commitFiles(r, map[string]string{"Makefile": makefile, "package.json": pkg, "justfile": justfile,
			"tools/clean.py": script})
	}
	edited := func(name string) func(r discardRepos) {
		return func(r discardRepos) {
			committed(r)
			r.write(r.clone, name, map[string]string{"Makefile": makefile, "package.json": pkg,
				"justfile": justfile, "tools/clean.py": script}[name]+"\n")
		}
	}
	unknown := "belt cannot tell which path"
	withLocal := func(files map[string]string) func(r discardRepos) {
		return func(r discardRepos) {
			committed(r)
			r.local(files)
		}
	}
	deletesD := withLocal(map[string]string{"d/f.txt": "mine\n", "new.mk": "quiet:\n\trm -rf d\n"})
	rows := []repoRow{
		{"make committed", committed, "make clean", nil},
		{"make edited", edited("Makefile"), "make clean", []string{unknown, "`Makefile` is not"}},
		{"make untracked", func(r discardRepos) { r.local(map[string]string{"Makefile": makefile}) },
			"make clean", []string{unknown}},
		{"make ignored", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "Makefile\n")
			r.local(map[string]string{"Makefile": makefile})
		}, "make clean", []string{unknown}},
		{"make committed, dirty tree", func(r discardRepos) {
			committed(r)
			r.local(map[string]string{"d/f.txt": "mine\n"})
		}, "make tree", []string{"the make target `tree`", "`d/f.txt` holds content no commit has"}},
		{"make changed first", committed, "sed -i.bak 's/x/y/' Makefile && make clean",
			[]string{"this command changes"}},
		{"make written first", committed, "echo >> Makefile && make clean", []string{unknown, "`Makefile` is not"}},
		{"make from standard input", committed, "make -f - clean < Makefile", []string{"standard input"}},
		{"make from /dev/stdin", committed, "cat Makefile | make -f /dev/stdin clean", []string{"standard input"}},
		{"make --eval", committed, "make --eval 'zap: ; rm -rf $(NOPE)' zap", []string{"`--eval`"}},
		{"make copied over first", deletesD, "cp new.mk Makefile && make quiet", []string{"`quiet`", "d/f.txt"}},
		{"make moved over first", deletesD, "mv new.mk Makefile && make quiet", []string{"`quiet`", "d/f.txt"}},
		{"make copied over, no delete", withLocal(map[string]string{"new.mk": "quiet:\n\techo hi\n"}),
			"cp new.mk Makefile && make quiet", nil},
		{"make written by a heredoc", deletesD, "cat > Makefile <<'EOF'\nquiet:\n\trm -rf d\nEOF\nmake quiet",
			[]string{"`quiet`", "d/f.txt"}},
		{"make made by a heredoc", withLocal(map[string]string{"d/f.txt": "mine\n", "sub/k": "k\n"}),
			"cat > sub/Makefile <<'EOF'\nx:\n\trm -rf ../d\nEOF\nmake -C sub x", []string{"d/f.txt"}},
		{"make written by a script", deletesD,
			"python3 - <<'EOF'\nopen('Makefile', 'w').write(open('new.mk').read())\nEOF\nmake quiet",
			[]string{"in a way belt cannot read"}},
		{"make written by a download", committed, "curl -fsS https://example.com/x.mk > Makefile && make quiet",
			[]string{"in a way belt cannot read"}},
		{"make after a checkout", committed, "git checkout -q main && make clean", []string{"this command changes"}},
		{"make after a checkout, target names no delete", committed, "git checkout -q main && make quiet", nil},
		{"make read first", committed, "cat Makefile && make clean", nil},
		{"make after a step naming a longer name", committed, `cmake -G "Unix Makefiles" . && make quiet`, nil},
		{"make inline script", committed, "make inline", nil},
		{"make runs a committed script", committed, "make script", nil},
		{"make runs an edited script", edited("tools/clean.py"), "make script", []string{"`rmtree`"}},
		{"npm committed", committed, "npm run clean", nil},
		{"npm edited", edited("package.json"), "npm run clean", []string{unknown}},
		{"just committed", committed, "just clean", nil},
		{"just edited", edited("justfile"), "just clean", []string{unknown}},
		{"script committed", committed, "python3 tools/clean.py", nil},
		{"script edited", edited("tools/clean.py"), "python3 tools/clean.py", []string{"`rmtree`"}},
		{"script copied over first", committed, "cp a.txt tools/clean.py && python3 tools/clean.py", nil},
		{"script copied over with a delete", withLocal(map[string]string{"evil.py": script}),
			"cp evil.py tools/clean.py && python3 tools/clean.py", []string{unknown, "`clean.py` is not"}},
		{"script copied from", committed, "cp tools/clean.py b.py && python3 tools/clean.py", nil},
		{"script appended to", committed, `echo "# x" >> tools/clean.py; python3 tools/clean.py`,
			[]string{unknown, "`clean.py` is not"}},
		{"untracked script appended to",
			withLocal(map[string]string{"d/f.txt": "mine\n", "t/c.py": "import shutil\nshutil.rmtree('d')\n"}),
			`echo "# x" >> t/c.py; python3 t/c.py`, []string{"d/f.txt"}},
		{"script edited between two runs", committed,
			"python3 tools/clean.py; sed -i.bak s/X/Y/ tools/clean.py; python3 tools/clean.py",
			[]string{"this command changes the script", "`clean.py` is not"}},
		{"npm workspaces committed", committed, "npm run clean --workspaces", nil},
		{"yarn workspace committed", committed, "yarn workspace pkg clean", nil},
		{"npm workspace untracked", withLocal(map[string]string{"pkg/package.json": pkg}), "npm run clean -w pkg",
			[]string{"every package.json"}},
		{"pnpm recursive, root edited", edited("package.json"), "pnpm -r clean", []string{"`package.json` is not"}},
	}
	runRepoRows(t, rows)
}

// TestFileDiscardTrustsWhatABuildReads covers the trust rule over every
// file a build run reads: a make include (from make's folder, an -I folder
// or MAKEFILES), a just import and the .env just loads. A delete whose path
// belt cannot read passes only when each of them is committed, and a
// missing optional include leaves the run trusted. It also covers $1 under
// just's positional-arguments, which takes the recipe's argument.
func TestFileDiscardTrustsWhatABuildReads(t *testing.T) {
	files := map[string]string{
		"inc/Makefile":  makefileText("include local.mk", "wipe:", ">rm -rf $(DIR)/y"),
		"inc/local.mk":  "DIR = $(shell cat .d)\n",
		"opt/Makefile":  makefileText("-include gone.mk", "DIR ?= $(shell cat .d)", "wipe:", ">rm -rf $(DIR)/y"),
		"srch/Makefile": makefileText("-include x.mk", "DIR ?= build", "wipe:", ">rm -rf $(DIR)/y"),
		"srch/mk/x.mk":  "DIR = $(shell cat .d)\n",
		"justfile":      "import 'local.just'\nwipe:\n    rm -rf {{d}}/y\n",
		"local.just":    "d := `cat .d`\n",
		"env/justfile":  "set dotenv-load\nwipe:\n    rm -rf \"$D/y\"\n",
		"pos/justfile":  "set positional-arguments\nwipe dir:\n    rm -rf \"$1\"\n",
	}
	// committedBut commits every file but the ones named, then writes those
	// as local files, with extra after their text.
	committedBut := func(extra string, names ...string) func(r discardRepos) {
		return func(r discardRepos) {
			held := map[string]string{}
			for k, v := range files {
				if !slices.Contains(names, k) {
					held[k] = v
				}
			}
			commitFiles(r, held)
			for _, name := range names {
				r.write(r.clone, name, files[name]+extra)
			}
		}
	}
	committed := committedBut("")
	withLocal := func(local map[string]string) func(r discardRepos) {
		return func(r discardRepos) {
			committed(r)
			r.local(local)
		}
	}
	unknown := "belt cannot tell which path"
	rows := []repoRow{
		{"include committed", committed, "make -C inc wipe", nil},
		{"include untracked", committedBut("", "inc/local.mk"), "make -C inc wipe",
			[]string{unknown, "`Makefile` reads `local.mk`, which is not"}},
		{"include edited", func(r discardRepos) {
			committed(r)
			r.write(r.clone, "inc/local.mk", files["inc/local.mk"]+"# e\n")
		}, "make -C inc wipe", []string{unknown, "reads `local.mk`"}},
		{"include ignored", func(r discardRepos) {
			committedBut("", "inc/local.mk")(r)
			r.write(r.clone, ".git/info/exclude", "local.mk\n")
		}, "make -C inc wipe", []string{unknown, "reads `local.mk`"}},
		{"include appended to first", committed, `echo "# x" >> inc/local.mk && make -C inc wipe`,
			[]string{unknown, "reads `local.mk`"}},
		{"optional include missing", committed, "make -C opt wipe", nil},
		{"include from -I committed", committed, "make -C srch -I mk wipe", nil},
		{"include from -I untracked", committedBut("", "srch/mk/x.mk"), "make -C srch -I mk wipe",
			[]string{unknown, "reads `x.mk`"}},
		{"MAKEFILES untracked", withLocal(map[string]string{"opt/extra.mk": "DIR = $(shell cat .d)\n"}),
			"MAKEFILES=extra.mk make -C opt wipe", []string{unknown, "reads `extra.mk`"}},
		{"just import committed", committed, "just wipe", nil},
		{"just import edited", committedBut("# e\n", "local.just"), "just wipe",
			[]string{unknown, "reads `local.just`"}},
		{"just .env absent", committed, "cd env && just wipe", nil},
		{"just .env committed", func(r discardRepos) {
			committed(r)
			commitFiles(r, map[string]string{"env/.env": "D=build\n"})
		}, "cd env && just wipe", nil},
		{"just .env ignored", func(r discardRepos) {
			committed(r)
			r.write(r.clone, ".git/info/exclude", ".env\n")
			r.local(map[string]string{"env/.env": "D=src\n"})
		}, "cd env && just wipe", []string{unknown, "reads `.env`"}},
		{"just .env by option", withLocal(map[string]string{"env/local.env": "D=src\n"}),
			"cd env && just --dotenv-filename local.env wipe", []string{unknown, "reads `local.env`"}},
		{"just positional argument", withLocal(map[string]string{"d/f.txt": "mine\n"}), "cd pos && just wipe ../d",
			[]string{"the just recipe `wipe`", "d/f.txt"}},
		{"just positional argument, clean path", committed, "cd pos && just wipe ../gone", nil},
	}
	runRepoRows(t, rows)
}
