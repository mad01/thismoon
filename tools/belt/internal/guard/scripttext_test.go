package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// textDoc is a markdown body naming a denied command the ways a doc does:
// in prose, and in a code block whose lines start with bash -c and xargs.
// Read as shell lines, the code block is denied.
const textDoc = "# Cleanup\n\nRun `kubectl delete pod x` when a pod is stuck:\n\n```\n" +
	"kubectl get pods -o name | xargs kubectl delete\nbash -c 'kubectl delete pod x'\n```"

// textHeredocCases are commands with a heredoc and the decision the guard
// must reach. cwd holds tool, an existing executable.
func textHeredocCases(t *testing.T) (cwd string, cases []scriptDenyCase) {
	t.Helper()
	cwd = t.TempDir()
	writeScript(t, cwd, "tool", "#!/bin/sh\necho hi\n")
	const del = "kubectl delete pod x"
	doc := func(open, tail string) string { return open + "\n" + textDoc + "\nEOF" + tail }
	return cwd, []scriptDenyCase{
		// Text: written to a file nothing in the command runs.
		{"cat to markdown", doc("cat > doc.md <<'EOF'", ""), cwd, false},
		{"redirect after the operator", doc("cat <<'EOF' > doc.md", ""), cwd, false},
		{"append after mkdir", doc("mkdir -p docs && cat >> docs/doc.md <<'EOF'", ""), cwd, false},
		{"tee to a file", doc("tee doc.md <<'EOF'", ""), cwd, false},
		{"sudo tee, stdout dropped", doc("sudo tee -a doc.md > /dev/null <<'EOF'", ""), cwd, false},
		{"tee to a text log", "tee build.log <<'EOF'\nxargs " + del + "\nEOF", cwd, false},
		{
			"unquoted delimiter, nothing to expand",
			"cat > doc.md <<EOF\nxargs " + del + "\nEOF",
			cwd,
			false,
		},
		{"tab-stripped body", "cat > doc.md <<-'EOF'\n\tbash -c '" + del + "'\n\tEOF", cwd, false},
		{
			"staged and published",
			doc("cat > pr.md <<'EOF'", "\ngit add pr.md && gh pr create --body-file pr.md"),
			cwd,
			false,
		},
		{
			"two text heredocs",
			doc("cat > a.md <<'EOF'", "\n"+doc("cat > b.md <<'EOF'", "")),
			cwd,
			false,
		},

		// Code: the heredoc feeds an interpreter, or the written file runs.
		// As the ticket wrote it: the shell reads `EOF; bash x.sh` as body.
		{"written script then run", "cat > x.sh <<'EOF'\n" + del + "\nEOF; bash x.sh", cwd, true},
		{
			"written script run on the next line",
			"cat > x.sh <<'EOF'\n" + del + "\nEOF\nbash x.sh",
			cwd,
			true,
		},
		{"heredoc into bash", "bash <<'EOF'\n" + del + "\nEOF", cwd, true},
		{
			"heredoc into python",
			"python3 - <<'EOF'\nimport os\nos.system('" + del + "')\nEOF",
			cwd,
			true,
		},
		{
			"written file made executable",
			"cat > x.sh <<'EOF'\n" + del + "\nEOF\nchmod +x x.sh",
			cwd,
			true,
		},
		{"markdown made executable", doc("cat > doc.md <<'EOF'", "\nchmod 755 doc.md"), cwd, true},
		{
			"written file sourced",
			"cat > env.md <<'EOF'\n" + del + "\nEOF\nsource env.md",
			cwd,
			true,
		},
		{"written file dot-sourced", doc("cat > doc.md <<'EOF'", "\n. ./doc.md"), cwd, true},
		{"heredoc cat piped into sh", "cat <<'EOF' | sh\n" + del + "\nEOF", cwd, true},
		{"heredoc tee piped into sh", "tee doc.md <<'EOF' | sh\n" + del + "\nEOF", cwd, true},
		{"here-string piped into sh", "cat <<< '" + del + "' | sh", cwd, true},
		{"markdown run by bash", doc("cat > doc.md <<'EOF'", "\nbash doc.md"), cwd, true},
		{
			"markdown copied then run",
			doc("cat > doc.md <<'EOF'", "\ncp doc.md run.sh && ./run.sh"),
			cwd,
			true,
		},
		{"markdown run through a glob", doc("cat > doc.md <<'EOF'", "\nsh *.md"), cwd, true},
		{
			"markdown run in the background",
			doc("cat > doc.md <<'EOF'", "\necho done & sh doc.md"),
			cwd,
			true,
		},
		{"markdown run in a subshell", doc("cat > doc.md <<'EOF'", "\n(sh doc.md)"), cwd, true},
		{
			"markdown run by a substitution",
			doc("cat > doc.md <<'EOF'", "\necho $(sh doc.md)"),
			cwd,
			true,
		},
		{
			"git with a config override",
			doc("cat > doc.md <<'EOF'", "\ngit -c core.pager=sh log"),
			cwd,
			true,
		},

		// Code: the target or the body says script.
		{"script extension", "cat > x.py <<'EOF'\nxargs " + del + "\nEOF", cwd, true},
		{
			"json is not a document",
			"tee file.json <<'EOF'\n{\"run\": \"x\"}\nxargs " + del + "\nEOF",
			cwd,
			true,
		},
		{"no extension", "cat > NOTES <<'EOF'\nxargs " + del + "\nEOF", cwd, true},
		{"existing executable target", "cat > tool <<'EOF'\nxargs " + del + "\nEOF", cwd, true},
		{"shell startup file", "cat >> ~/.zshenv <<'EOF'\nxargs " + del + "\nEOF", cwd, true},
		{"shebang body", "cat > deploy <<'EOF'\n#!/bin/sh\nxargs " + del + "\nEOF", cwd, true},
		{
			"unquoted body expands",
			"cat > doc.md <<EOF\nBuilt $(date)\nxargs " + del + "\nEOF",
			cwd,
			true,
		},
		{
			"stdout to the terminal device",
			"cat > /dev/stdout <<'EOF'\nxargs " + del + "\nEOF",
			cwd,
			true,
		},
		{"cat with no file", "cat <<'EOF'\nxargs " + del + "\nEOF", cwd, true},
		{"cat by path", "./cat > doc.md <<'EOF'\nxargs " + del + "\nEOF", cwd, true},

		// Code: the shell may not read the heredoc line the way the guard does.
		{
			"open quote before the line",
			"echo \"\ncat > doc.md <<'EOF'\n\"\nbash -c '" + del + "'\nEOF",
			cwd,
			true,
		},
		{
			"operator behind a comment",
			"true # ; cat > doc.md <<'EOF'\nbash -c '" + del + "'\nEOF",
			cwd,
			true,
		},
		{"operator in a substitution", "x=$(cat <<'EOF'\nxargs " + del + "\nEOF\n)", cwd, true},
		{
			"inline script before the heredoc",
			"bash -c 'true' && cat > doc.md <<'EOF'\n" + del + "\nEOF",
			cwd,
			true,
		},
	}
}

func TestScriptDenyListTextHeredocs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CDPATH", "")
	cwd, cases := textHeredocCases(t)
	g := NewScriptDenyList(scriptDenyConfig([]string{"kubectl delete"}, nil))
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: cwd})
			if got := d != nil; got != tt.deny {
				t.Fatalf("deny = %v, want %v (denial: %+v)\n%s", got, tt.deny, d, tt.command)
			}
		})
	}
}

// The text corpus for the differential test: heredoc lines, bodies and the
// command after the terminator, crossed with the delimiter forms. Every body
// has a line origin/main denies when it reads the body as shell lines.
// textSinkLines are read as text when the tail is in textSafeTails;
// codeSinkLines never are. %s is the delimiter as written after <<.
var (
	textSinkLines = []string{
		"cat > doc.md <<%s",
		"cat >> doc.md <<%s",
		"cat <<%s > doc.md",
		"tee doc.md <<%s",
		"tee -a doc.txt > /dev/null <<%s",
		"sudo tee doc.md <<%s",
		"mkdir -p docs && cat > docs/doc.md <<%s",
		"cd docs && cat > doc.md <<%s && git add doc.md",
	}
	codeSinkLines = []string{
		"cat > doc.sh <<%s",
		"cat > tool <<%s",
		"cat > ~/.zshrc <<%s",
		"tee -a doc.json > /dev/null <<%s",
		"cat > lefthook.yml <<%s",
		"cat >> .git/config <<%s",
		"cat > .gitattributes <<%s",
		"cat >> .husky/pre-commit <<%s",
		"cat > .claude/commands/doc.md <<%s",
		"sudo tee -a /etc/zshenv > /dev/null <<%s",
		"cat > ~/Library/LaunchAgents/doc.txt <<%s",
		"cd /tmp; cat > doc.md <<%s",
		"cd docs\ncat > doc.md <<%s",
		"true || cd docs && cat > doc.md <<%s",
		"cat > doc.md 2>> .git/hooks/pre-commit <<%s",
		"cat <<%s | sh",
		"cat <<%s",
		"cat > /dev/stdout <<%s",
		"tee doc.md <<%s | sh",
		"./cat > doc.md <<%s",
		"cat > doc.md <<%s &",
		"cat > doc.md <<%s; sh doc.md",
		"true # ; cat > doc.md <<%s",
		"x=$(cat > doc.md <<%s",
		"bash <<%s",
	}
	textBodies = []string{
		"Clean up with:\n\n```\nbash -c 'rm -rf build'\n```",
		"kubectl get pods -o name | xargs rm -rf",
		"find . -name '*.tmp' -exec rm -rf {} +",
		"eval \"git config --global user.name x\"",
	}
	textTails = []string{
		"",
		"\ngit add doc.md",
		"\ngh pr create --title x --body-file doc.md",
		"\nbash doc.md",
		"\n. ./doc.md",
		"\nsource doc.md",
		"\nchmod +x doc.md",
		"\ncat doc.md | sh",
		"\nsh < doc.md",
		"\ncp doc.md run.sh && ./run.sh",
		"\nsh *.md",
		"\necho done & sh doc.md",
		"\necho $(sh doc.md)",
		"\nmake",
		"\ncat doc.md >> .git/hooks/pre-commit",
		"\necho source doc.md >> ~/.zshenv",
		"\ngit log -1 --format=%B --output=.git/hooks/pre-commit",
		"\ngit add --chmod=+x doc.md",
		"\ngh pr checkout 1",
		"\nchmod 600 doc.md",
		"\ncd /tmp; git status",
	}
	textSafeTails = map[string]bool{
		"": true, "\ngit add doc.md": true, "\ngh pr create --title x --body-file doc.md": true,
	}
)

// textTightened are the hand-reviewed inputs origin/main allowed and the
// guard now denies: a written file made executable, and a heredoc or
// here-string passed through cat or tee into an interpreter.
var textTightened = []string{
	"cat > x.sh <<'EOF'\nrm -rf /tmp/x\nEOF\nchmod +x x.sh",
	"echo 'rm -rf /tmp/x' > gen.sh && chmod +x gen.sh",
	"cat <<'EOF' | sh\nrm -rf /tmp/x\nEOF",
	"tee x.md <<'EOF' | bash\ngit config --global user.name x\nEOF",
	"cat <<< 'rm -rf /tmp/x' | sh",
}

// textHeredocCorpus renders every heredoc line, delimiter, body and tail.
// relaxed is the test's own statement of the text shape: a text line, a
// tail that runs nothing, and a body the shell hands over unchanged.
func textHeredocCorpus() []corpusInput {
	var out []corpusInput
	for _, line := range textSinkLines {
		out = append(out, textCorpusLine(line, true)...)
	}
	for _, line := range codeSinkLines {
		out = append(out, textCorpusLine(line, false)...)
	}
	return out
}

// textCorpusLine renders one heredoc line with every delimiter, body and
// tail.
func textCorpusLine(line string, text bool) []corpusInput {
	var out []corpusInput
	for _, delim := range heredocDelims {
		for _, body := range textBodies {
			unchanged := strings.ContainsAny(delim, `'"\`) || !strings.ContainsAny(body, "$`\\")
			for _, tail := range textTails {
				out = append(out, corpusInput{
					command: renderTextHeredoc(line, delim, body, tail),
					relaxed: text && unchanged && textSafeTails[tail],
				})
			}
		}
	}
	return out
}

// renderTextHeredoc builds one corpus command; a <<- delimiter gets its body
// and terminator indented with tabs.
func renderTextHeredoc(line, delim, body, tail string) string {
	term := delimWord(delim)
	if strings.HasPrefix(delim, "-") {
		body = "\t" + strings.ReplaceAll(body, "\n", "\n\t")
		term = "\t" + term
	}
	return fmt.Sprintf(line, delim) + "\n" + body + "\n" + term + tail
}

// textRunLine is a heredoc body line origin/main denies: read as a command,
// xargs runs rm -rf.
const textRunLine = "xargs rm -rf < /tmp/list"

// textReviewFixture builds a hermetic home and repo for the review inputs.
// The repo has an executable hook, hook.md linked to it, docs linked to the
// hooks directory, rc.md hard-linked to ~/.zshrc and zshenv.md linked to a
// ~/.zshenv that does not exist yet. sub is an empty directory.
func textReviewFixture(t *testing.T) (cwd, home string) {
	t.Helper()
	home, cwd = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CDPATH", "")
	for _, dir := range []string{".git/hooks", "sub"} {
		if err := os.MkdirAll(filepath.Join(cwd, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeScript(t, cwd, ".git/hooks/pre-commit", "#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export X=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	links := []struct{ old, name string }{
		{filepath.Join(cwd, ".git/hooks/pre-commit"), "hook.md"},
		{filepath.Join(cwd, ".git/hooks"), "docs"},
		{filepath.Join(home, ".zshenv"), "zshenv.md"},
	}
	for _, l := range links {
		if err := os.Symlink(l.old, filepath.Join(cwd, l.name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Link(filepath.Join(home, ".zshrc"), filepath.Join(cwd, "rc.md")); err != nil {
		t.Fatal(err)
	}
	return cwd, home
}

// textReviewCases are the inputs a review found fail open, origin/main
// denied them and the shell runs their body, plus the ones found while
// fixing them. Each must be denied, except the chmod modes that add no
// execute bit, which must be allowed.
func textReviewCases() []scriptDenyCase {
	write := func(open, tail string) string { return open + "\n" + textRunLine + "\nEOF" + tail }
	notes := func(tail string) string { return write("cat > notes.md <<'EOF'", tail) }
	prose := func(tail string) string {
		return "cat > notes.md <<'EOF'\nNever run rm -rf here.\nEOF" + tail
	}
	cases := []scriptDenyCase{
		// A git or gh step runs a command from a file the heredoc writes.
		{
			"git config fsmonitor, then git status",
			"cat >> .git/config <<'EOF'\n[core]\n\tfsmonitor = echo /tmp/x | xargs rm -rf\nEOF\ngit status",
			"", true,
		},
		{
			"global gitconfig fsmonitor, then git status",
			"cat >> ~/.gitconfig <<'EOF'\n[core]\n\tfsmonitor = echo /tmp/x | xargs rm -rf\nEOF\ngit status",
			"", true,
		},
		{
			"global gitconfig external diff, then git diff",
			"cat >> ~/.gitconfig <<'EOF'\n[diff]\n\texternal = echo /tmp/x | xargs rm -rf\nEOF\ngit diff",
			"", true,
		},
		{
			"gitattributes filter, then git add",
			"cat > .gitattributes <<'EOF'\n* filter=x\nEOF\ncat >> .git/config <<'EOF'\n[filter \"x\"]\n" +
				"\tclean = echo /tmp/x | xargs rm -rf\nEOF\ngit add notes.md",
			"", true,
		},
		{
			"husky hook, then git commit",
			write("cat >> .husky/pre-commit <<'EOF'", "\ngit commit -m wip"),
			"",
			true,
		},
		{
			"pre-push hook, then gh pr create",
			write("cat >> .git/hooks/pre-push <<'EOF'", "\ngh pr create --fill"),
			"",
			true,
		},
		{"gh pr checkout", notes("\ngh pr checkout 12"), "", true},
		{
			"git log writes the message into a hook",
			notes("\ngit add notes.md && git commit -F notes.md\n" +
				"git log -1 --format=%B --output=.git/hooks/pre-commit\ngit commit --allow-empty -m wip"),
			"", true,
		},
		{"git add sets the execute bit", notes("\ngit add --chmod=+x notes.md"), "", true},
		{
			"yml a hook runner reads",
			write("cat > lefthook.yml <<'EOF'", "\ngit commit -m wip"),
			"",
			true,
		},
		{
			"package.json a hook runner reads",
			write("cat > package.json <<'EOF'", "\ngit commit -m wip"),
			"",
			true,
		},
		{"mise.toml", write("cat > mise.toml <<'EOF'", ""), "", true},

		// A safe step copies the text into something that runs.
		{
			"copied into a hook",
			notes("\ncat notes.md >> .git/hooks/pre-commit\ngit commit -m wip"),
			"",
			true,
		},
		{"copied into zshenv", notes("\ncat notes.md >> ~/.zshenv"), "", true},
		{"sourced from zshenv", notes("\necho source notes.md >> ~/.zshenv"), "", true},

		// The guard's directory is not the one the shell writes in.
		{
			"cd that may fail, then a hook",
			write(
				"cd /nonexistent-dir; cat >> .git/hooks/pre-commit <<'EOF'",
				"\ngit commit -m wip",
			),
			"", true,
		},
		{
			"cd then ;, the cwd file links to a hook",
			write("cd sub; cat >> hook.md <<'EOF'", ""),
			"",
			true,
		},
		{"cd on its own line", write("cd sub\ncat >> hook.md <<'EOF'", ""), "", true},
		{
			"cd whose chain ended",
			write("ls missing && cd sub && true\ncat >> hook.md <<'EOF'", ""),
			"",
			true,
		},
		{"cd skipped by ||", write("true || cd sub && cat >> hook.md <<'EOF'", ""), "", true},
		{
			"cd set by CDPATH in the command",
			write("CDPATH=/tmp cd sub && cat >> hook.md <<'EOF'", ""),
			"",
			true,
		},
		{"cd -", write("cd - && cat > notes.md <<'EOF'", ""), "", true},
		{"cd with no argument", write("cd && cat > notes.md <<'EOF'", ""), "", true},
		{"pushd", write("pushd sub && cat > notes.md <<'EOF'", ""), "", true},
		{
			"cd into a dot directory",
			write("cd ~/.claude/commands && cat > deploy.md <<'EOF'", ""),
			"",
			true,
		},

		// The target is a file something runs or sources.
		{
			"sudo tee to /etc/zshenv",
			write("sudo tee -a /etc/zshenv > /dev/null <<'EOF'", ""),
			"",
			true,
		},
		{
			"sudo tee to /etc/zshrc",
			write("sudo tee -a /etc/zshrc > /dev/null <<'EOF'", ""),
			"",
			true,
		},
		{
			"sudo tee to /etc/zprofile",
			write("sudo tee -a /etc/zprofile > /dev/null <<'EOF'", ""),
			"",
			true,
		},
		{
			"sudo tee to /etc/profile",
			write("sudo tee -a /etc/profile > /dev/null <<'EOF'", ""),
			"",
			true,
		},
		{
			"sudo tee to /etc/bashrc",
			write("sudo tee -a /etc/bashrc > /dev/null <<'EOF'", ""),
			"",
			true,
		},
		{
			"a text name under /etc",
			write("sudo tee -a /etc/notes.txt > /dev/null <<'EOF'", ""),
			"",
			true,
		},
		{
			"a case-folded /etc",
			write("sudo tee -a /ETC/notes.txt > /dev/null <<'EOF'", ""),
			"",
			true,
		},
		{"bash_login", write("cat >> ~/.bash_login <<'EOF'", ""), "", true},
		{"zlogout", write("cat >> ~/.zlogout <<'EOF'", ""), "", true},
		{"xdg git config", write("cat >> ~/.config/git/config <<'EOF'", ""), "", true},
		{"launch agent", write("cat > ~/Library/LaunchAgents/x.plist <<'EOF'", ""), "", true},
		{
			"a text name in ~/Library",
			write("cat > ~/Library/LaunchAgents/x.txt <<'EOF'", ""),
			"",
			true,
		},
		{"claude settings", write("cat > .claude/settings.json <<'EOF'", ""), "", true},
		{"claude local settings", write("cat > .claude/settings.local.json <<'EOF'", ""), "", true},
		{"ssh rc", write("cat >> ~/.ssh/rc <<'EOF'", ""), "", true},
		{"dangling link to a startup file", write("cat >> zshenv.md <<'EOF'", ""), "", true},
		{"hard link to a startup file", write("cat >> rc.md <<'EOF'", ""), "", true},
		{"symlinked parent directory", write("cat > docs/notes.md <<'EOF'", ""), "", true},

		// chmod scans the command only when the mode adds an execute bit.
		{"chmod 600", prose("\nchmod 600 notes.md"), "", false},
		{"chmod 644", prose("\nchmod 644 notes.md"), "", false},
		{"chmod -x", prose("\nchmod -x notes.md"), "", false},
		{"chmod a-x", prose("\nchmod a-x notes.md"), "", false},
		{"chmod u=rw,go=r", prose("\nchmod u=rw,go=r notes.md"), "", false},
		{"chmod -R 600", prose("\nchmod -R 600 notes.md"), "", false},
		{"chmod +x", prose("\nchmod +x notes.md"), "", true},
		{"chmod u+x", prose("\nchmod u+x notes.md"), "", true},
		{"chmod a+x", prose("\nchmod a+x notes.md"), "", true},
		{"chmod 744", prose("\nchmod 744 notes.md"), "", true},
		{"chmod 4755", prose("\nchmod 4755 notes.md"), "", true},
		{"chmod g=u copies a bit", prose("\nchmod g=u notes.md"), "", true},
		{"chmod with an ACL", prose("\nchmod +a 'everyone allow execute' notes.md"), "", true},
		{"chmod --reference", prose("\nchmod --reference=tool notes.md"), "", true},

		// Still text after the fix.
		{
			"markdown, then git add and gh pr create",
			notes("\ngit add notes.md && gh pr create --body-file notes.md"),
			"",
			false,
		},
		{
			"cd joined by && into a plain directory",
			write("cd sub && cat > notes.md <<'EOF'", ""),
			"",
			false,
		},
	}
	return cases
}

func TestScriptDenyListTextHeredocReview(t *testing.T) {
	cwd, _ := textReviewFixture(t)
	g := NewScriptDenyList(scriptDenyConfig([]string{"rm -rf"}, nil))
	for _, tt := range textReviewCases() {
		t.Run(tt.name, func(t *testing.T) {
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: cwd})
			if got := d != nil; got != tt.deny {
				t.Fatalf("deny = %v, want %v (denial: %+v)\n%s", got, tt.deny, d, tt.command)
			}
		})
	}
}

// TestTextHeredocCdPathFromEnvironment checks a relative cd when CDPATH is
// set where the guard runs: the shell may change to a directory under a
// CDPATH entry, so the directory is unknown and a relative target is not
// text. Here sub/hook.md under the CDPATH entry links to the hook.
func TestTextHeredocCdPathFromEnvironment(t *testing.T) {
	cwd, _ := textReviewFixture(t)
	cdroot := t.TempDir()
	if err := os.Mkdir(filepath.Join(cdroot, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(cwd, ".git/hooks/pre-commit")
	if err := os.Symlink(hook, filepath.Join(cdroot, "sub/hook.md")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CDPATH", cdroot)
	cmd := "cd sub && cat >> hook.md <<'EOF'\n" + textRunLine + "\nEOF"
	g := NewScriptDenyList(scriptDenyConfig([]string{"rm -rf"}, nil))
	if g.Check(Input{Event: EventBash, Command: cmd, Cwd: cwd}) == nil {
		t.Fatalf("allowed a relative cd under an environment CDPATH:\n%s", cmd)
	}
}

// TestTextHeredocBodiesSpan checks the skipped range: the body and the
// terminator line, and nothing after it.
func TestTextHeredocBodiesSpan(t *testing.T) {
	cmd := "cat > doc.md <<'EOF'\nline one\nEOF\ngit add doc.md"
	spans := textHeredocBodies(cmd, t.TempDir())
	if len(spans) != 1 {
		t.Fatalf("want one text body, got %v", spans)
	}
	got := cmd[spans[0].from:spans[0].to]
	if got != "line one\nEOF\n" {
		t.Fatalf("span = %q, want the body and terminator line", got)
	}
	if !strings.HasPrefix(cmd[spans[0].to:], "git add") {
		t.Fatalf("span ends at %d, before %q", spans[0].to, cmd[spans[0].to:])
	}
}

// TestTextHeredocNeedsKnownDirectory checks that a relative target is read
// as text only where the walk knows the directory, since the execute-bit
// check must look at the file the shell would write.
func TestTextHeredocNeedsKnownDirectory(t *testing.T) {
	home, cwd, other := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CDPATH", "")
	for _, dir := range []string{cwd + "/sub", cwd + "/dir.md", other + "/real"} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(other+"/real", other+"/link"); err != nil {
		t.Fatal(err)
	}
	writeScript(t, cwd, "sub/tool.md", "#!/bin/sh\n")
	if err := os.WriteFile(filepath.Join(cwd, "sub/doc.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, cwd, command string
		text               bool
	}{
		{"no cwd", "", "cat > doc.md <<'EOF'\nx\nEOF", false},
		{"home target, no cwd", "", "cat > ~/doc.md <<'EOF'\nx\nEOF", true},
		{"absolute home target, no cwd", "", "cat > " + home + "/doc.md <<'EOF'\nx\nEOF", true},
		{"cd to the previous directory", cwd, "cd - && cat > doc.md <<'EOF'\nx\nEOF", false},
		{"cd with a flag", cwd, "cd -P sub && cat > doc.md <<'EOF'\nx\nEOF", false},
		{"relative cd from the cwd", cwd, "cd sub && cat > doc.md <<'EOF'\nx\nEOF", true},
		{"absolute cd into the cwd", cwd, "cd " + cwd + " && cat > doc.md <<'EOF'\nx\nEOF", true},
		{"cd home, no cwd", "", "cd ~ && cat > doc.md <<'EOF'\nx\nEOF", true},
		{
			"cd on an earlier line may have failed",
			cwd,
			"cd sub\ncat > doc.md <<'EOF'\nx\nEOF",
			false,
		},
		{
			"cd chain ended before a later heredoc",
			cwd,
			"cd sub && cat > a.md <<'EOF'\nx\nEOF\ncat > b.md <<'EOF'\ny\nEOF",
			false,
		},
		{"executable found after the cd", cwd, "cd sub && cat > tool.md <<'EOF'\nx\nEOF", false},
		{"existing plain file", cwd, "cat >> sub/doc.md <<'EOF'\nx\nEOF", true},
		{"directory target", cwd, "cat > dir.md <<'EOF'\nx\nEOF", false},
		{"fifo target", cwd, "cat > pipe.md <<'EOF'\nx\nEOF", false},
		{"dot-slash target", cwd, "cat > ./doc.md <<'EOF'\nx\nEOF", false},
		{"parent target", cwd, "cat > sub/../doc.md <<'EOF'\nx\nEOF", false},
		{"variable target", cwd, "cat > $HOME/doc.md <<'EOF'\nx\nEOF", false},
		{"other user's home", cwd, "cat > ~root/doc.md <<'EOF'\nx\nEOF", false},
		{"symlink outside the cwd", cwd, "cat > " + other + "/link/doc.md <<'EOF'\nx\nEOF", false},
	}
	if err := syscall.Mkfifo(filepath.Join(cwd, "pipe.md"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := textHeredocBodies(tt.command, tt.cwd) != nil; got != tt.text {
				t.Fatalf("read as text = %v, want %v:\n%s", got, tt.text, tt.command)
			}
		})
	}
}

// textTempFixture builds a scratch home whose ~/.tmp holds link, a symlink
// to a real directory, and tool.md, an executable. It returns a name no
// file in the real temp folders starts with, so the test never meets a file
// it did not make there.
func textTempFixture(t *testing.T) (home, name string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CDPATH", "")
	tmp := filepath.Join(home, ".tmp")
	if err := os.Mkdir(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(tmp, "link")); err != nil {
		t.Fatal(err)
	}
	writeScript(t, tmp, "tool.md", "#!/bin/sh\n")
	name = fmt.Sprintf("belt-text-%d-%d", os.Getpid(), time.Now().UnixNano())
	for _, dir := range []string{"/tmp", "/private/tmp", "/var/tmp", "/private/var/tmp"} {
		if found, _ := filepath.Glob(filepath.Join(dir, name+"*")); len(found) > 0 {
			t.Fatalf("%s already exists", found[0])
		}
	}
	return home, name
}

// TestTextHeredocTempFolders checks the temp folders as trusted bases: a
// note or a pull request body written to one is text. The folder is judged
// from there down, so the part below it keeps every check.
func TestTextHeredocTempFolders(t *testing.T) {
	home, name := textTempFixture(t)
	write := func(target, tail string) string {
		return "cat > " + target + " <<'EOF'\n" + textRunLine + "\nEOF" + tail
	}
	note := "/tmp/" + name + ".md"
	cases := []scriptDenyCase{
		{"note in /tmp", write(note, ""), "", false},
		{"pull request body in /tmp", write(note, "\ngh pr create --body-file "+note), "", false},
		{"/private/tmp", write("/private/tmp/"+name+".txt", ""), "", false},
		{"/var/tmp", write("/var/tmp/"+name+".md", ""), "", false},
		{"/private/var/tmp", write("/private/var/tmp/"+name+".md", ""), "", false},
		{"~/.tmp", write("~/.tmp/notes.md", ""), "", false},
		{"~/.tmp by its absolute path", write(home+"/.tmp/notes.md", ""), "", false},

		{"script in /tmp", write("/tmp/"+name+".sh", ""), "", true},
		{"dot directory below /tmp", write("/tmp/.hidden/x.md", ""), "", true},
		{"parent part out of /tmp", write("/tmp/../etc/x.md", ""), "", true},
		{"note in /tmp, then run", write(note, "\nbash "+note), "", true},
		{"symlinked directory in ~/.tmp", write("~/.tmp/link/x.md", ""), "", true},
		{"dot directory below ~/.tmp", write("~/.tmp/.x/y.md", ""), "", true},
		{"executable in ~/.tmp", write("~/.tmp/tool.md", ""), "", true},
		{"/private/etc", write("/private/etc/x.md", ""), "", true},
	}
	// /var/folders is refused because /var is a symlink on macOS. On Linux it
	// is a plain directory, and there is no /var/folders to guard.
	if runtime.GOOS == "darwin" {
		cases = append(
			cases,
			scriptDenyCase{"/var/folders", write("/var/folders/a/x.md", ""), "", true},
		)
	}
	cwd := t.TempDir()
	g := NewScriptDenyList(scriptDenyConfig([]string{"rm -rf"}, nil))
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: cwd})
			if got := d != nil; got != tt.deny {
				t.Fatalf("deny = %v, want %v (denial: %+v)\n%s", got, tt.deny, d, tt.command)
			}
		})
	}
}
