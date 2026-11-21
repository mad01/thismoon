package guard

import (
	"os/exec"
	"testing"
)

// pushLocal commits files in the clone and pushes them to its upstream.
// Call it before any other change to the clone: it commits every file.
func pushLocal(r discardRepos, files map[string]string) {
	r.t.Helper()
	r.local(files)
	r.git(r.clone, "add", "-A")
	r.git(r.clone, "commit", "-q", "-m", "local")
	r.git(r.clone, "push", "-q", "origin", "main")
}

// ignoredOdd sets up a dirty a.txt beside an ignored file whose name holds
// a space and the name of a.txt.
func ignoredOdd(r discardRepos) {
	r.write(r.clone, ".git/info/exclude", "*.pyc\n")
	r.local(map[string]string{"a.txt": "mine\n", "a.txt x.pyc": "p"})
}

// trackedOdd pushes files with odd names, then dirties a.txt.
func trackedOdd(files map[string]string) func(r discardRepos) {
	return func(r discardRepos) {
		pushLocal(r, files)
		r.local(map[string]string{"a.txt": "mine\n"})
	}
}

// workIn sets up an ignored folder dir that holds a repository with an
// untracked file, lib/new.txt.
func workIn(dir string) func(r discardRepos) {
	return func(r discardRepos) {
		r.write(r.clone, ".git/info/exclude", dir+"/\n")
		lib := r.clone + "/" + dir + "/lib"
		r.git(r.clone, "init", "-q", lib)
		r.write(lib, "new.txt", "n\n")
	}
}

// TestFileDiscardFeedNames: a path a feeder prints reaches the command
// whole, or the command cuts it into other paths. xargs without -0 cuts at
// blanks, quotes and backslashes, an unquoted $( ) at blanks, and git
// prints an odd name in C quotes without -z.
func TestFileDiscardFeedNames(t *testing.T) {
	runRepoRows(t, []repoRow{
		{"xargs cuts an ignored name", ignoredOdd, "find . -name '*.pyc' | xargs rm -f",
			[]string{`"a.txt x.pyc", which holds a space`, "xargs -0"}},
		{"xargs -0 keeps an ignored name", ignoredOdd, "find . -name '*.pyc' -print0 | xargs -0 rm -f", nil},
		{"substitution cuts an ignored name", ignoredOdd, "rm -f $(find . -name '*.pyc')", []string{"holds a space"}},
		{"quoted substitution keeps an ignored name", ignoredOdd, `rm -f "$(find . -name '*.pyc')"`, nil},
		{"xargs cuts a name in an ignored folder", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "build/\n")
			r.local(map[string]string{"a.txt": "mine\n", "build/x y.o": "o"})
		}, "find . -name '*.o' | xargs rm", []string{`"build/x y.o"`}},
		{"xargs cuts an ignored folder's name", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "x y/\n")
			r.local(map[string]string{"a.txt": "mine\n", "x y/out.o": "o"})
		}, "find . -name '*.o' | xargs rm", []string{`"x y/out.o"`}},
		{"xargs keeps plain names", func(r discardRepos) {
			r.write(r.clone, ".git/info/exclude", "*.pyc\n")
			r.local(map[string]string{"a.txt": "mine\n", "x.pyc": "p", "d/y.pyc": "p"})
		}, "find . -name '*.pyc' | xargs rm -f", nil},
		{"git ls-files cuts a tracked name", trackedOdd(map[string]string{"a.txt x.txt": "n\n"}),
			"git ls-files | xargs rm -f", []string{`"a.txt x.txt", which holds a space`}},
		{"git ls-files substitution cuts a tracked name", trackedOdd(map[string]string{"a.txt x.txt": "n\n"}),
			"rm -f $(git ls-files)", []string{"holds a space"}},
		{"git ls-files -z keeps a tracked name", func(r discardRepos) {
			pushLocal(r, map[string]string{"a.txt x.txt": "n\n"})
		}, "git ls-files -z | xargs -0 rm -f", nil},
		{"git quotes a name", trackedOdd(map[string]string{`a"b.txt`: "n\n"}),
			"git ls-files | xargs rm -f", []string{"`a\"b.txt` in C quotes"}},
		{"git quotes a name in a substitution", trackedOdd(map[string]string{`a"b.txt`: "n\n"}),
			"rm -f $(git ls-files)", []string{"in C quotes"}},
		{"git quotes a non-ASCII name", trackedOdd(map[string]string{"café.txt": "n\n"}),
			`git ls-files | xargs -d '\n' rm -f`, []string{"in C quotes"}},
		{"git ls-files -z keeps a quoted name", func(r discardRepos) {
			pushLocal(r, map[string]string{`a"b.txt`: "n\n", "café.txt": "n\n"})
		}, "git ls-files -z | xargs -0 rm -f", nil},
		{"git quotepath off", nil, "git -c core.quotepath=false ls-files | xargs rm -f", []string{"git -c core.quotepath=false"}},
		// git ls-files and git grep list no ignored file; -o without
		// --exclude-standard does.
		{"git ls-files skips ignored folders", workIn("vendor"), "git ls-files | xargs rm", nil},
		{"git grep skips ignored folders", workIn("vendor"), "git grep -l a | xargs rm", nil},
		{"git ls-files others skips ignored folders", workIn("vendor"),
			"git ls-files -o --exclude-standard | xargs rm", nil},
		{"git ls-files others enters ignored folders", workIn("vendor"),
			"git ls-files -o | xargs rm -rf", []string{"/vendor/lib`, a git repository"}},
	})
}

// TestFileDiscardRgNames runs the rg rows when rg is on PATH: rg lists no
// ignored file unless -u or --no-ignore says so.
func TestFileDiscardRgNames(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not on PATH")
	}
	runRepoRows(t, []repoRow{
		{"rg cuts a tracked name", trackedOdd(map[string]string{"a.txt x.txt": "TODO\n"}),
			"rg -l TODO | xargs rm -f", []string{`"a.txt x.txt", which holds a space`}},
		{"rg -0 keeps a tracked name", func(r discardRepos) {
			pushLocal(r, map[string]string{"a.txt x.txt": "TODO\n"})
		}, "rg -l0 TODO | xargs -0 rm -f", nil},
		{"rg skips ignored folders", workIn("vendor"), "rg -l x | xargs rm", nil},
		{"rg no-ignore enters ignored folders", workIn("vendor"),
			"rg --no-ignore -l x | xargs rm", []string{"/vendor/lib/new.txt`, which is untracked"}},
		{"rg -u enters ignored folders", workIn("vendor"),
			"rg -u -l x | xargs rm", []string{"/vendor/lib/new.txt`, which is untracked"}},
	})
}
