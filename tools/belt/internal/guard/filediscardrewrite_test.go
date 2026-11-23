package guard

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFileDiscardRewriteOutsideWorkTrees covers the in-place edit rows that
// need no git: which words make a backup, which files a sed or perl edits,
// the named files that pass, and the feeders, globs and consumers that
// hand an edit files over files outside every work tree.
func TestFileDiscardRewriteOutsideWorkTrees(t *testing.T) {
	home := t.TempDir()
	writeTree(t, home, map[string]string{
		"notes.txt": "a\n", "b.txt": "b\n", "zero": "", "sub/c.txt": "c\n", "edit.sed": "s/a/b/\n",
		"links/real": "r\n",
	})
	if err := os.Symlink(filepath.Join(home, "notes.txt"), filepath.Join(home, "links", "ln")); err != nil {
		t.Fatal(err)
	}
	tmp := tempFixture(t)
	writeTree(t, tmp, map[string]string{"f.txt": "f"})
	outside := []string{"this rewrites `", "in no git work tree and no temp folder",
		"An in-place edit passes when it keeps a backup or names each file as plain text", rewriteKeep}
	notOther := []string{"--safe-mode", "A delete passes", "An overwrite passes"}
	unknown := []string{"belt cannot tell which path"}
	rows := []fileDiscardRow{
		// The files a glob, an expansion or a feeder hands the edit drop.
		{"sed glob", "~", "sed -i 's/a/b/' *.txt", outside, notOther},
		{"gsed glob", "~", "gsed -i 's/a/b/' *.txt", outside, nil},
		{"sed globstar", "~", "sed -i 's/a/b/' **/*.txt", outside, nil},
		{"sed glob from the root", "/", "sed -i 's/a/b/' ~/*.txt", outside, []string{"the home directory"}},
		{"sed substitution", "~", "sed -i 's/a/b/' $(find . -name '*.txt')", outside, nil},
		{"sed option after the script", "~", "sed 's/a/b/' -i *.txt", outside, nil},
		{"sed -e", "~", "sed -i -e 's/a/b/' *.txt", outside, nil},
		{"sed -f", "~", "sed -i -f edit.sed *.txt", outside, nil},
		{"sed --", "~", "sed -i 's/a/b/' -- *.txt", outside, nil},
		{"sed bundled -E", "~", "sed -Ei 's/a/b/' *.txt", outside, nil},
		{"sed long in-place", "~", "sed --in-place 's/a/b/' *.txt", outside, nil},
		{"sed long prefix", "~", "sed --in-pl 's/a/b/' *.txt", outside, nil},
		{"sed long empty suffix", "~", "sed --in-place= 's/a/b/' *.txt", outside, nil},
		{"sed --follow-symlinks", "~", "sed -i --follow-symlinks 's/a/b/' links/l*",
			append(outside, "notes.txt`"), nil},
		{"sudo sed", "~", "sudo sed -i 's/a/b/' *.txt", outside, nil},
		{"bash -c sed", "~", "bash -c \"sed -i 's/a/b/' *.txt\"", outside, nil},
		// A separate word after -i is GNU's script and BSD's suffix: no backup.
		{"sed empty suffix word", "~", "sed -i '' 's/a/b/' *.txt", outside, nil},
		{"sed suffix word", "~", "sed -i .bak 's/a/b/' *.txt", outside, nil},
		{"sed long suffix word", "~", "sed --in-place .bak 's/a/b/' *.txt", outside, nil},
		{"sed unread suffix", "~", `sed -i"$S" 's/a/b/' *.txt`, outside, nil},
		// GNU sed and perl put the file's name in place of each `*`: `*`
		// and `./*` name the file itself and keep no backup.
		{"sed star suffix", "~", "sed -i'*' 's/a/b/' *.txt", outside, nil},
		{"sed long star suffix", "~", "sed --in-place='*' 's/a/b/' *.txt", outside, nil},
		{"sed dot star suffix", "~", "sed -i'./*' 's/a/b/' *.txt", outside, nil},
		{"sed dot slashes star suffix", "~", "sed -i'.//*' 's/a/b/' *.txt", outside, nil},
		{"sed stars only suffix", "~", "sed -i'**' 's/a/b/' *.txt", outside, nil},
		{"sed star suffix from the root", "~", "sed -i'/*' 's/a/b/' *.txt", outside, nil},
		{"sed star suffix up a folder", "~", "sed -i'../*' 's/a/b/' *.txt", outside, nil},
		{"sed star suffix through a folder", "~", "sed -i'x/../*' 's/a/b/' *.txt", outside, nil},
		{"perl star extension alone", "~", "perl -pi'*' -e 's/a/b/' *.txt", outside, nil},
		{"perl dot star extension", "~", "perl -i'./*' -pe 's/a/b/' *.txt", outside, nil},
		{"xargs star suffix", "~", "find . -name '*.txt' | xargs sed -i'*' 's/a/b/'", outside, nil},
		{"find -exec perl star extension", "~", "find . -name '*.txt' -exec perl -pi'*' -e 's/a/b/' {} +",
			outside, nil},
		// A word the shell may cut into several may hold files, as the
		// script, an option's value or the -i word.
		{"sed split script", "~", `f='s/a/b/ *.txt'; sed -i $f`, append(unknown, "`$f`"), nil},
		{"sed split script before a name", "~", "sed -i $S notes.txt", []string{"`$S`"}, nil},
		{"sed split -e value", "~", "sed -i -e $S notes.txt", []string{"`$S`"}, nil},
		{"sed split -f value", "~", "sed -i -f $(cat list) notes.txt", []string{"`$(cat list)`"}, nil},
		{"sed split suffix", "~", "sed -i$S 's/a/b/' notes.txt", []string{"`-i$S`"}, nil},
		{"perl split program", "~", `f='-e s/a/b/ *.txt'; perl -pi $f`, []string{"`$f`"}, nil},
		{"perl split -e value", "~", "perl -pi -e $P notes.txt", []string{"`$P`"}, nil},
		{"xargs split script", "~", "find . -name '*.txt' | xargs sed -i $S", []string{"`$S`"}, nil},
		// A variable or loop operand is no name.
		{"sed unset variable", "~", `sed -i 's/a/b/' "$F"`, unknown, nil},
		{"sed loop", "~", `for f in *.txt; do sed -i 's/a/b/' "$f"; done`, unknown, nil},
		{"perl loop", "~", `for f in *.txt; do perl -pi -e 's/a/b/' "$f"; done`, unknown, nil},
		// perl reads its switches as clusters.
		{"perl -pi", "~", "perl -pi -e 's/a/b/' *.txt", outside, notOther},
		{"perl -i -pe", "~", "perl -i -pe 's/a/b/' *.txt", outside, nil},
		{"perl -0777", "~", "perl -0777 -pi -e 's/a/b/' *.txt", outside, nil},
		{"perl -lpi", "~", "perl -lpi -e 's/a/b/' *.txt", outside, nil},
		{"perl -0 cluster", "~", "perl -00pi -e 's/a/b/' *.txt", outside, nil},
		{"perl program file", "~", "perl -pi edit.pl *.txt", outside, nil},
		{"perl --", "~", "perl -pi -e 's/a/b/' -- *.txt", outside, nil},
		{"perl -I dir", "~", "perl -I lib -pi -e 's/a/b/' *.txt", outside, nil},
		// Consumers hand the edit the paths they read.
		{"find -exec sed", "~", "find . -name '*.txt' -exec sed -i 's/a/b/' {} +", outside, nil},
		{"find -execdir perl", "~", "find . -name '*.txt' -execdir perl -pi -e 's/a/b/' {} \\;", outside, nil},
		{"find -exec sh -c", "~", `find . -name '*.txt' -exec sh -c 'sed -i "s/a/b/" "$1"' _ {} \;`, outside, nil},
		{"xargs from find", "~", "find . -name '*.txt' | xargs sed -i 's/a/b/'", outside, nil},
		{"xargs -I", "~", "find . -name '*.txt' | xargs -I{} sed -i 's/a/b/' {}", outside, nil},
		{"xargs from a list", "~", "cat list | xargs sed -i 's/a/b/'", []string{"belt cannot tell which paths"}, nil},
		{"xargs operand holds the mark", "~", "find . -name '*.txt' | xargs -I{} sed -i 's/a/b/' {}.orig",
			unknown, nil},
		{"parallel glob", "~", "parallel sed -i 's/a/b/' ::: *.txt", outside, nil},
		// A backup suffix both seds keep, or perl's extension, passes.
		{"sed -i.bak", "~", "sed -i.bak 's/a/b/' *.txt", nil, nil},
		{"sed -i~", "~", "sed -i~ 's/a/b/' *.txt", nil, nil},
		{"sed -ie", "~", "sed -ie 's/a/b/' *.txt", nil, nil},
		{"sed -ni.bak", "~", "sed -ni.bak 's/a/b/p' *.txt", nil, nil},
		{"sed long suffix", "~", "sed --in-place=.bak 's/a/b/' *.txt", nil, nil},
		{"sed long prefix suffix", "~", "sed --in=.bak 's/a/b/' *.txt", nil, nil},
		{"sed suffix last", "~", "sed -i -i.bak 's/a/b/' *.txt", nil, nil},
		{"gsed suffix", "~", "gsed -i.orig 's/a/b/' *.txt", nil, nil},
		{"perl -pi.bak", "~", "perl -pi.bak -e 's/a/b/' *.txt", nil, nil},
		{"perl -i.bak -pe", "~", "perl -i.bak -pe 's/a/b/' *.txt", nil, nil},
		{"perl star extension", "~", "perl -i'orig_*' -pe 's/a/b/' *.txt", nil, nil},
		{"perl star then extension", "~", "perl -pi'*.orig' -e 's/a/b/' *.txt", nil, nil},
		{"sed star prefix", "~", "sed -i'orig_*' 's/a/b/' *.txt", nil, nil},
		{"sed star then suffix", "~", "sed -i'*.bak' 's/a/b/' *.txt", nil, nil},
		{"sed star then tilde", "~", "sed --in-place='*~' 's/a/b/' *.txt", nil, nil},
		{"sed star in a folder", "~", "sed -i'bak/*' 's/a/b/' *.txt", nil, nil},
		{"perl -pie", "~", "perl -pie 's/a/b/' *.txt", nil, nil},
		{"find -exec backup", "~", "find . -name '*.txt' -exec sed -i.bak 's/a/b/' {} +", nil, nil},
		{"xargs backup", "~", "find . -name '*.txt' | xargs perl -pi.bak -e 's/a/b/'", nil, nil},
		// No in-place edit.
		{"sed prints", "~", "sed 's/a/b/' *.txt", nil, nil},
		{"sed -n", "~", "sed -n 's/a/b/p' *.txt", nil, nil},
		{"perl prints", "~", "perl -pe 's/a/b/' *.txt", nil, nil},
		{"perl -C letters", "~", "perl -CSi -pe 's/a/b/' *.txt", nil, nil},
		{"find -exec grep", "~", "find . -name '*.txt' -exec grep -l a {} +", nil, nil},
		{"wrapped grep names sed", "~", "nice grep -r sed -i *.txt", nil, nil},
		// Files named as plain text pass.
		{"sed named", "~", "sed -i 's/a/b/' notes.txt b.txt", nil, nil},
		{"sed named quoted", "~", `sed -i "s/a/b/" "notes.txt"`, nil, nil},
		{"sed quoted glob", "~", "sed -i 's/a/b/' '*.txt'", nil, nil},
		{"sed named from the root", "/", "sed -i 's/a/b/' ~/notes.txt", nil, nil},
		{"sed empty suffix named", "~", "sed -i '' 's/a/b/' notes.txt", nil, nil},
		{"perl named", "~", "perl -pi -e 's/a/b/' notes.txt", nil, nil},
		{"parallel named", "~", "parallel sed -i 's/a/b/' ::: notes.txt", nil, nil},
		{"sed quoted script variable", "~", `sed -i "$S" notes.txt`, nil, nil},
		{"sed quoted unread suffix named", "~", `sed -i"$S" 's/a/b/' notes.txt`, nil, nil},
		{"perl quoted code variable", "~", `perl -pi -e "$P" notes.txt`, nil, nil},
		// A variable the command sets to a literal is that literal.
		{"sed variable set to a name", "~", `p=notes.txt; sed -i 's/a/b/' "$p"`, nil, nil},
		{"sed unquoted variable set to a name", "~", `p=notes.txt; sed -i 's/a/b/' $p`, nil, nil},
		// Nothing is lost.
		{"sed glob over a symlink", "~", "sed -i 's/a/b/' links/l*", nil, nil},
		{"sed glob no match", "~", "sed -i 's/a/b/' *.md", nil, nil},
		{"sed glob empty", "~", "sed -i 's/a/b/' zer*", nil, nil},
		{"sed temp", "~", "sed -i 's/a/b/' " + tmp + "/*.txt", nil, nil},
	}
	runFileDiscardRows(t, newFileDiscardGuard(t, home), home, rows)
}

// TestFileDiscardRewriteInWorkTrees runs each in-place edit shape over the
// files of a clone, committed and clean or changed, then the forms that
// pass over changed files, with the guard's real git backends.
func TestFileDiscardRewriteInWorkTrees(t *testing.T) {
	dirty := func(r discardRepos) { r.local(map[string]string{"a.txt": "mine\n", "d/f.txt": "mine\n"}) }
	untracked := func(r discardRepos) { r.local(map[string]string{"new.txt": "n\n"}) }
	ignored := func(r discardRepos) {
		r.write(r.clone, ".git/info/exclude", "*.log\n")
		r.local(map[string]string{"out.log": "x\n", "a.txt": "mine\n"})
	}
	// linkOut commits l1.lnk, a link to a.txt in the seed, another
	// repository; dirtyTarget changes that file there.
	linkOut := func(dirtyTarget bool) func(r discardRepos) {
		return func(r discardRepos) {
			to := filepath.Join("..", "seed", "a.txt")
			if err := os.Symlink(to, filepath.Join(r.clone, "l1.lnk")); err != nil {
				r.t.Fatal(err)
			}
			r.git(r.clone, "add", "-A")
			r.git(r.clone, "commit", "-q", "-m", "link")
			if dirtyTarget {
				r.write(r.seed, "a.txt", "mine\n")
			}
		}
	}
	const followSed = "sed --follow-symlinks -i 's/a/b/'"
	rewrites := []string{"this rewrites uncommitted work in ", "seed"}
	overwrites := []string{"this overwrites uncommitted work in ", "seed"}
	changed := []string{"this rewrites uncommitted work in ", "An in-place edit passes", rewriteKeep}
	shapes := []struct{ name, command string }{
		{"perl glob", "perl -pi -e 's/a/b/' *.txt"},
		{"sed glob", "sed -i 's/a/b/' *.txt"},
		{"sed empty suffix word", "sed -i '' 's/a/b/' *.txt"},
		{"sed suffix word", "sed -i .bak 's/a/b/' *.txt"},
		{"sed star suffix", "sed -i'*' 's/a/b/' *.txt"},
		{"sed long star suffix", "sed --in-place='*' 's/a/b/' *.txt"},
		{"sed dot star suffix", "sed -i'./*' 's/a/b/' *.txt"},
		{"perl star extension", "perl -pi'*' -e 's/a/b/' *.txt"},
		{"perl dot star extension", "perl -i'./*' -pe 's/a/b/' *.txt"},
		{"git ls-files -m xargs star", "git ls-files -m | xargs sed -i'*' 's/a/b/'"},
		{"find exec perl star", "find . -name '*.txt' -exec perl -pi'*' -e 's/a/b/' {} +"},
		{"gsed glob", "gsed -i 's/a/b/' *.txt"},
		{"globstar", "sed -i 's/a/b/' **/*.txt"},
		{"substitution", "sed -i 's/a/b/' $(git ls-files)"},
		{"git ls-files xargs", "git ls-files | xargs sed -i 's/a/b/'"},
		{"grep xargs", "grep -rl --exclude-dir=.git x . | xargs sed -i 's/a/b/'"},
		{"grep folder xargs", "grep -rl x d | xargs perl -pi -e 's/a/b/'"},
		{"find exec", "find . -name '*.txt' -exec sed -i 's/a/b/' {} +"},
		{"find execdir", "find . -name '*.txt' -execdir perl -pi -e 's/a/b/' {} \\;"},
		{"find exec sh -c", `find . -name '*.txt' -exec sh -c 'sed -i "s/a/b/" "$1"' _ {} \;`},
		{"parallel glob", "parallel sed -i 's/a/b/' ::: *.txt"},
	}
	var rows []repoRow
	for _, s := range shapes {
		rows = append(rows,
			repoRow{s.name + " clean", nil, s.command, nil},
			repoRow{s.name + " dirty", dirty, s.command, changed},
		)
	}
	rows = append(rows, []repoRow{
		{"glob untracked", untracked, "perl -pi -e 's/a/b/' *.txt", []string{"this rewrites `", "new.txt`, which is untracked"}},
		{"grep untracked", untracked, "grep -rl --exclude-dir=.git x . | xargs sed -i 's/a/b/'",
			[]string{"new.txt`, which is untracked"}},
		{"git ls-files skips untracked", untracked, "git ls-files | xargs sed -i 's/a/b/'", nil},
		{"glob ignored", ignored, "sed -i 's/a/b/' *.log", nil},
		{"grep from the top", nil, "grep -rl x . | xargs sed -i 's/a/b/'", []string{"/.git`, which is git's own data"}},
		{"git data glob", nil, "sed -i 's/a/b/' .git/*", []string{"git's own data"}},
		{"loop", nil, `for f in *.txt; do sed -i 's/a/b/' "$f"; done`, []string{"belt cannot tell which path"}},
		{"loop dirty", dirty, `for f in *.txt; do sed -i 's/a/b/' "$f"; done`,
			[]string{"belt cannot tell which path"}},
		{"unset variable dirty", dirty, `sed -i 's/a/b/' "$F"`,
			[]string{"belt cannot tell which path `\"$F\"`"}},
		// f also spells a part of d/f.txt, so the resolver cannot vouch
		// for it (resolveLiteralVars) and the variable is judged.
		{"variable", nil, `f=d/f.txt; sed -i 's/a/b/' "$f"`, []string{"belt cannot tell which path"}},
		// A word the shell may cut into several is judged as a file.
		{"split script untracked", untracked, `f='s/a/b/ *.txt'; sed -i $f`, []string{"`$f`"}},
		{"split perl program untracked", untracked, `f='-e s/a/b/ *.txt'; perl -pi $f`, []string{"`$f`"}},
		// A variable the command sets to a literal names that file.
		{"variable set to a name dirty", dirty, `p=a.txt; sed -i 's/a/b/' "$p"`, nil},
		{"unquoted variable set to a name untracked", untracked, `p=new.txt; sed -i 's/a/b/' $p`, nil},
		// A link a feeder hands a command that follows links leads to the
		// file it writes, here a changed one in another repository.
		{"xargs follows a link", linkOut(true), "find . -name '*.lnk' | xargs " + followSed,
			append(rewrites, "a.txt")},
		{"find exec follows a link", linkOut(true), "find . -name '*.lnk' -exec " + followSed + " {} +",
			append(rewrites, "a.txt")},
		{"git ls-files follows a link", linkOut(true), "git ls-files | xargs " + followSed, rewrites},
		{"glob follows a link", linkOut(true), followSed + " *.lnk", rewrites},
		{"cp through a fed link", linkOut(true), "find . -name '*.lnk' | xargs -I{} cp b.txt {}", overwrites},
		{"find exec cp through a link", linkOut(true), "find . -name '*.lnk' -exec cp b.txt {} \\;", overwrites},
		{"cp suffix through a link", func(r discardRepos) {
			linkOut(true)(r)
			r.local(map[string]string{"l1": "x\n"})
		}, "find . -name 'l1' -exec cp b.txt {}.lnk \\;", overwrites},
		{"xargs link without following", linkOut(true), "find . -name '*.lnk' | xargs sed -i 's/a/b/'", nil},
		{"xargs follows a link to clean work", linkOut(false), "find . -name '*.lnk' | xargs " + followSed, nil},
		{"find type f skips the link", linkOut(true), "find . -type f -name '*.lnk' | xargs " + followSed, nil},
		{"follows no links in a clean tree", nil, "find . -name '*.txt' -exec " + followSed + " {} +", nil},
		// Backups and named files pass over changed files.
		{"sed -i.bak dirty", dirty, "sed -i.bak 's/a/b/' *.txt", nil},
		{"sed long suffix dirty", dirty, "sed --in-place=.orig 's/a/b/' **/*.txt", nil},
		{"perl -pi.bak dirty", dirty, "perl -pi.bak -e 's/a/b/' *.txt", nil},
		{"perl -i~ dirty", dirty, "perl -i~ -pe 's/a/b/' *.txt", nil},
		{"sed -i~ dirty", dirty, "sed -i~ 's/a/b/' *.txt", nil},
		{"sed star prefix dirty", dirty, "sed -i'orig_*' 's/a/b/' *.txt", nil},
		{"sed star then suffix untracked", untracked, "sed -i'*.bak' 's/a/b/' *.txt", nil},
		{"perl star prefix untracked", untracked, "perl -pi'orig_*' -e 's/a/b/' *.txt", nil},
		{"xargs backup dirty", dirty, "git ls-files | xargs sed -i.bak 's/a/b/'", nil},
		{"find backup dirty", dirty, "find . -name '*.txt' -exec perl -i.bak -pe 's/a/b/' {} +", nil},
		{"named sed dirty", dirty, "sed -i 's/a/b/' a.txt", nil},
		{"named sed empty suffix dirty", dirty, "sed -i '' 's/a/b/' a.txt d/f.txt", nil},
		{"named perl dirty", dirty, "perl -pi -e 's/a/b/' a.txt d/f.txt", nil},
		{"named parallel dirty", dirty, "parallel sed -i 's/a/b/' ::: a.txt", nil},
		// An edit of a build file leaves the build after it unread.
		{"make after a named edit", func(r discardRepos) {
			commitFiles(r, map[string]string{"Makefile": makefileText("clean:", ">rm -rf $(NOPE)/x")})
		}, "sed -i 's/x/y/' Makefile && make clean", []string{"this command changes"}},
	}...)
	runRepoRows(t, rows)
}
