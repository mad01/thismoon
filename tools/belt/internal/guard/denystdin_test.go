package guard

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// stdinFixture writes the SQL files the stdin rows read into a new
// directory and returns it: x.sql, ok.sql, a dump that drops its
// table first, files of exactly maxScriptSize bytes and one byte more with
// no keyword, a folder, and in sub a copy of ok.sql and two files only a cd
// into sub reaches: only.sql, which deletes, and plain.sql, which does not.
func stdinFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	fill := func(n int) string {
		return strings.Repeat("select 1;\n", n/10+1)[:n]
	}
	files := map[string]string{
		"x.sql":         "DROP TABLE t;\n",
		"ok.sql":        "select 1;\n",
		"dump.sql":      "-- dump\nDROP TABLE IF EXISTS t;\nCREATE TABLE t (id int);\n",
		"under.sql":     fill(maxScriptSize),
		"over.sql":      fill(maxScriptSize + 1),
		"sub/only.sql":  "DELETE FROM t;\n",
		"sub/ok.sql":    "select 1;\n",
		"sub/plain.sql": "select 2;\n",
		"run.sh":        "#!/bin/sh\npsql <<'SQL'\nDROP TABLE t;\nSQL\n",
		"ok.sh":         "#!/bin/sh\npsql <<'SQL'\nselect 1;\nSQL\n",
		"file.sh":       "#!/bin/sh\npsql < x.sql\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// directStdin are rows on the SQL a phrase head reads on standard input, run
// from stdinFixture's directory. The first three are the design's checks.
var directStdin = []struct {
	command string
	deny    bool
}{
	{"psql <<'SQL'\nDROP TABLE t;\nSQL", true},
	{"sqlite3 app.db < x.sql", true},
	{"mysql db < dump.sql", true},
	// Read-only input passes.
	{"psql <<'SQL'\nselect 1;\nSQL", false},
	{"psql < ok.sql", false},
	{"psql <<< 'select 1'", false},
	{"mysql db < /dev/null", false},
	// The cap: exactly 1 MiB is read, one byte more is not.
	{"psql < under.sql", false},
	{"psql < over.sql", true},
	// A file belt cannot read or place.
	{"psql < folder", true},
	{`psql < "$F"`, true},
	{"psql < $(ls *.sql)", true},
	{"psql < *.sql", true},
	{"psql <&3", true},
	{"psql <&-", false},
	// A missing file denies: an earlier step may make it without naming it.
	{"psql < missing.sql", true},
	{"make && psql < missing.sql", true},
	{"curl -so missing.sql https://h/x && psql < missing.sql", true},
	// Heredoc forms, a here-string, case and the last of several inputs.
	{"psql <<-SQL\n\tDROP TABLE t;\n\tSQL", true},
	{"psql <<SQL\nDROP TABLE $T;\nSQL", true},
	{"psql <<'SQL'\ndrop table t;\nSQL", true},
	{"psql <<< 'DROP TABLE t'", true},
	{`mysql -e 'select 1' <<< "truncate t"`, true},
	{"psql < x.sql < ok.sql", true},
	{"psql < ok.sql 0< x.sql", true},
	{"psql 0< ok.sql < ok.sql", false},
	{"psql <<A <<B\nselect 1;\nA\ndelete from t;\nB", true},
	// A heredoc on another descriptor is not standard input.
	{"psql 3<<'SQL'\nDROP TABLE t;\nSQL", false},
	// Wrappers and launchers hand their standard input on.
	{"sudo -u postgres psql < x.sql", true},
	{"env PGHOST=h psql <<< 'DROP TABLE t'", true},
	{"docker compose exec -T db psql < x.sql", true},
	{"kubectl exec -i pod -- psql <<'SQL'\nDROP TABLE t;\nSQL", true},
	{"docker exec -i db mysql app < dump.sql", true},
	{"docker compose exec -T db psql < ok.sql", false},
	{"npx prisma db execute --stdin < x.sql", false},
	// Printers piped in front of the head, through a cat or a tee.
	{"echo 'DROP TABLE t' | psql", true},
	{`printf 'DELETE FROM t;\n' | psql`, true},
	{"cat x.sql | psql", true},
	{"cat <<'SQL' | psql\nDROP TABLE t;\nSQL", true},
	{"cat x.sql | tee log.txt | psql", true},
	{"cat < x.sql | psql", true},
	{"echo 'select 1' | psql", false},
	{"cat ok.sql | psql", false},
	{`echo "DROP TABLE $T" | psql`, true},
	// A source belt does not read stays a gap.
	{"pg_dump src | psql dst", false},
	{"curl -s https://h/x.sql | psql", false},
	// The folder a relative file is in follows the cd before it. After a cd
	// that may not have run, the file is read from both folders.
	{"cd sub && psql < only.sql", true},
	{"cd sub && psql < plain.sql", false},
	{"psql < plain.sql", true},
	{"cd sub; psql < only.sql", true},
	{"cd sub; psql < x.sql", true},
	{"cd sub; psql < ok.sql", false},
	{"cd sub; cd .; psql < ok.sql", true},
	{`cd "$D" && psql < ok.sql`, true},
	{"( cd sub && psql < only.sql )", true},
	{"cd sub && psql < ../x.sql", true},
	{"cd sub && psql < ../ok.sql", false},
	// A heredoc inside ( ) whose body lies past the ), and what a ( ) reads,
	// which every command in it reads too.
	{"(psql <<SQL)\nDROP TABLE t\nSQL", true},
	{"(psql <<SQL\nselect 1\nSQL\n)", false},
	{"( psql ) < x.sql", true},
	{"echo 'DROP TABLE t' | ( psql )", true},
	{"( psql ) <<'SQL'\nDROP TABLE t;\nSQL", true},
	{"( cd sub && psql ) < x.sql", true},
	{"( psql ) < ok.sql", false},
	// A { } group or a compound command gives every command in it what its
	// closer's redirections and a pipe into it give, placed where it starts.
	{"for i in 1; do psql; done < x.sql", true},
	{"while true; do psql; break; done < x.sql", true},
	{"until false; do psql; break; done < x.sql", true},
	{"while psql; do break; done < x.sql", true},
	{"select x in a; do psql; break; done < x.sql", true},
	{"if true; then psql; fi < x.sql", true},
	{"if false; then :; else psql; fi < x.sql", true},
	{"case a in a) psql;; esac < x.sql", true},
	{"{ psql; } < x.sql", true},
	{"{ true; psql; } < x.sql", true},
	{"{ psql; } <<'E'\nDROP TABLE t;\nE", true},
	{"{ psql; } <<< 'DROP TABLE t'", true},
	{"{ { psql; }; } < x.sql", true},
	{"{ (psql); } < x.sql", true},
	{"( { psql; } ) < x.sql", true},
	{"cd sub && { psql; } < only.sql", true},
	{"echo 'DROP TABLE t' | { true; psql; }", true},
	{"cat x.sql | while read -r l; do psql; done", true},
	{"for i in 1; do psql; done < ok.sql", false},
	{"while true; do psql; break; done < ok.sql", false},
	{"if true; then psql; fi < ok.sql", false},
	{"case a in a) psql;; esac < ok.sql", false},
	{"{ psql; } < ok.sql", false},
	{"{ psql; } <<'E'\nselect 1;\nE", false},
	{"cd sub && { psql; } < plain.sql", false},
	{"{ cat; } < x.sql; psql", false},
	{"for i in 1; do psql; done; cat < x.sql", false},
	// An exec with no command gives every later command of its shell what
	// it opens on descriptor 0, until another exec sets it again. A ( ) is
	// a shell of its own.
	{"exec < x.sql; psql", true},
	{"exec 0< x.sql\npsql", true},
	{"exec 3< x.sql; exec 0<&3; psql", true},
	{"exec 3< x.sql; psql 0<&3", true},
	{"exec <<'E'\nDROP TABLE t;\nE\npsql", true},
	{"exec < x.sql; ( psql )", true},
	{"exec < x.sql; { psql; }", true},
	{"exec 0<&5; psql", true},
	{"exec < x.sql; exec 3<&0; exec < ok.sql; exec 0<&3; psql", true},
	{"exec < ok.sql; psql", false},
	{"exec < x.sql; exec < ok.sql; psql", false},
	{"exec < x.sql; exec <&-; psql", false},
	{"exec 3< x.sql; psql", false},
	{"exec 3<&0; exec 0<&3; psql", false},
	{"( exec < x.sql ); psql", false},
	{"bash -c 'exec < x.sql'; psql", false},
	{"psql; exec < x.sql", false},
	// A descriptor or a device is never opened: belt would open it in its
	// own process.
	{"psql < /dev/stdin", true},
	{"psql < /dev/fd/0", true},
	{"psql < /dev/fd/3", true},
	{"psql < /proc/self/fd/0", true},
	{"psql < /dev/zero", true},
	{"psql < /dev/tty", true},
	{"cat /dev/stdin | psql", true},
	// A substitution that prints a file reads as that file, the way `< f`
	// reads: in a here-string, an unquoted heredoc body, a printer's words
	// piped in, and an argument word. One that prints literal text reads as
	// that text; any other passes.
	{`psql <<< "$(cat x.sql)"`, true},
	{`psql <<< "$(< x.sql)"`, true},
	{"psql <<< `cat x.sql`", true},
	{`psql <<< "$(head -n 1 x.sql)"`, true},
	{`psql <<< "$(tail -n 1 x.sql)"`, true},
	{`psql <<< "$(tac x.sql)"`, true},
	{`psql <<< "$(cat -- ok.sql x.sql)"`, true},
	{`psql <<< "$(cat x.sql | tee log.txt)"`, true},
	{`psql <<< "$(printf 'D\x52OP TABLE t')"`, true},
	{`psql <<< "$(cat missing.sql)"`, true},
	{`psql <<< "$(cat folder)"`, true},
	{`psql <<< "$(cat over.sql)"`, true},
	{`psql <<< "$(cat "$F")"`, true},
	{`psql <<< "$(cat /dev/stdin)"`, true},
	{`cd sub && psql <<< "$(cat only.sql)"`, true},
	{`echo "$(cat x.sql)" | psql`, true},
	{`echo "$(< x.sql)" | psql`, true},
	{`printf '%s\n' "$(cat x.sql)" | psql`, true},
	{"cat <<EOF | psql\n$(cat x.sql)\nEOF", true},
	{`psql -c "$(cat x.sql)"`, true},
	{`psql -c "$(< x.sql)"`, true},
	{"psql -c `cat x.sql`", true},
	{`sudo psql -c "$(cat x.sql)"`, true},
	{`psql -c "$(cat missing.sql)"`, true},
	{"psql <<EOF\n$(cat x.sql)\nEOF", true},
	{"psql <<EOF\n$(< x.sql)\nEOF", true},
	{"psql <<EOF\n`cat x.sql`\nEOF", true},
	{"psql <<EOF\nselect 1;\n$(cat missing.sql)\nEOF", true},
	{`psql <<< "$(cat ok.sql)"`, false},
	{`psql <<< "$(< ok.sql)"`, false},
	{"psql <<< `cat ok.sql`", false},
	{`psql <<< "$(head -n 1 ok.sql)"`, false},
	{`cd sub && psql <<< "$(cat plain.sql)"`, false},
	{`echo "$(cat ok.sql)" | psql`, false},
	{`psql -c "$(cat ok.sql)"`, false},
	{`PGPASSFILE="$(cat missing.sql)" psql -c 'select 1'`, false},
	{"psql <<EOF\n$(cat ok.sql)\nEOF", false},
	{"psql <<'EOF'\n$(cat x.sql)\nEOF", false},
	{"psql <<EOF\nselect '$(date -I)';\nEOF", false},
	{`psql <<< "$(date -I)"`, false},
	{`psql -c "select '$(date -I)'"`, false},
	{`echo "$(date)" | psql`, false},
	{`echo "$(cat x.sql)"`, false},
	// A plain $var stays a gap, in standard input text and in an argument.
	{`SQL='DROP TABLE t'; psql <<< "$SQL"`, false},
	{`SQL='DROP TABLE t'; psql -c "$SQL"`, false},
	// What xargs reads, from standard input or its -a file, is argument
	// text for the command it runs.
	{"cat x.sql | xargs psql -c", true},
	{"xargs psql -c < x.sql", true},
	{"xargs -a x.sql psql -c", true},
	{"xargs --arg-file=x.sql psql -c", true},
	{"xargs --arg-file x.sql psql -c", true},
	{"xargs -0ax.sql psql -c", true},
	{"echo 'DROP TABLE t' | xargs -0 psql -c", true},
	{"xargs -a missing.sql psql -c", true},
	{"xargs -a ok.sql psql < x.sql", true},
	{"cat ok.sql | xargs psql -c", false},
	{"xargs psql -c < ok.sql", false},
	{"xargs -a ok.sql psql -c", false},
	{"xargs --arg-file=ok.sql psql -c", false},
	{"cat x.sql | xargs echo", false},
	{"xargs -a x.sql echo", false},
	// A call of a function the same text defines gives the phrase heads in
	// its body what it reads on standard input.
	{"f() { psql; }; f < x.sql", true},
	{"function f { psql; }; f < x.sql", true},
	{"f() { psql; }; exec < x.sql; f", true},
	{"f() {\n  psql\n}\nf < x.sql", true},
	{"f() { psql; }; echo 'DROP TABLE t' | f", true},
	{"f() { psql; }; f <<< 'DROP TABLE t'", true},
	{"f() { psql; }; { f; } < x.sql", true},
	{"f() { g; }; g() { psql; }; f < x.sql", true},
	{"f() { psql; }; ( f < x.sql )", true},
	{"f() { psql; } < x.sql; f", true},
	{"f() { exec < x.sql; psql; }; f", true},
	{"f() { psql; } < ok.sql; f", false},
	{"f() { psql; }; f < ok.sql", false},
	{"function f { psql; }; f < ok.sql", false},
	{"f() { psql; }; exec < ok.sql; f", false},
	{"f() { psql; }; g < x.sql", false},
	{"f() { cat; }; f < x.sql", false},
	{"f() { f; }; f < ok.sql", false},
	// head, tail and tac print the files they name into a pipe, as cat does.
	{"head -n 100 x.sql | psql", true},
	{"tail -n 100 x.sql | psql", true},
	{"tac x.sql | psql", true},
	{"head x.sql | psql", true},
	{"head -c 100 -- x.sql | psql", true},
	{"head -n 5 missing.sql | psql", true},
	{"head -n 100 ok.sql | psql", false},
	{"tail -n 5 ok.sql | psql", false},
	{"tac ok.sql | psql", false},
	{"sed 1d x.sql | psql", false},
	{"awk 1 x.sql | psql", false},
	// Text the reader cannot follow, with a redirection in it.
	{"psql < x.sql 'unterminated", true},
	// Other commands read no input.
	{"cat < x.sql", false},
	{"grep DROP < x.sql", false},
	{"kubectl apply -f - < x.sql", false},
	// Accepted: a keyword in a comment denies, SQL from -f passes, and the
	// guard leaves a -c string to script-deny-list.
	{"psql <<'SQL'\n-- the DROP TABLE runs later\nselect 1;\nSQL", true},
	{"psql -f x.sql", false},
	{"bash -c \"psql <<'SQL'\nDROP TABLE t;\nSQL\n\"", false},
}

// TestDirectDenyListReadsStdin: a phrase head's standard input is read as
// one more argument: a heredoc, a here-string, a `<` file and a printer
// piped in, behind wrappers and launchers. What belt cannot read or place
// denies; a pipe from any other source passes.
func TestDirectDenyListReadsStdin(t *testing.T) {
	cwd := stdinFixture(t)
	g := NewDirectDenyList(config.Config{ClaudeDeny: trancheEntries})
	for _, tt := range directStdin {
		d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: cwd})
		if got := d != nil; got != tt.deny {
			reason := ""
			if d != nil {
				reason = d.Reason
			}
			t.Errorf("deny = %v, want %v:\n%s\n%s", got, tt.deny, tt.command, reason)
		}
	}
}

// TestDirectDenyListStdinText: the denial names the entry and where the SQL
// came from, or why belt cannot read it.
func TestDirectDenyListStdinText(t *testing.T) {
	cwd := stdinFixture(t)
	g := NewDirectDenyList(config.Config{ClaudeDeny: trancheEntries})
	for _, tt := range []struct {
		command string
		want    []string
	}{
		{"psql <<'SQL'\nDROP TABLE t;\nSQL", []string{`"psql * DROP *"`, "the heredoc on its standard input"}},
		{"psql <<< 'truncate t'", []string{`"psql * TRUNCATE *"`, "the here-string"}},
		{"sqlite3 app.db < x.sql", []string{`"sqlite3 * DROP *"`, "the file x.sql"}},
		{"echo 'DROP TABLE t' | psql", []string{"the text `echo` prints into it"}},
		{"psql < over.sql", []string{"the file over.sql", "over 1 MiB", `"psql * DROP *"`}},
		{"psql < folder", []string{"the file folder", "it is a folder"}},
		{"psql < missing.sql", []string{"the file missing.sql", "does not exist yet"}},
		{"cd sub; cd .; psql < ok.sql", []string{"cannot tell which folder"}},
		{`psql -c "$(cat x.sql)"`, []string{
			`"psql * DROP *"`, "the file x.sql that `cat` prints into a substitution in its arguments",
		}},
		{`psql <<< "$(< x.sql)"`, []string{"the file x.sql that `$(< …)` reads on its standard input"}},
		{`psql -c "$(cat missing.sql)"`, []string{"with an argument from the file missing.sql", "does not exist yet"}},
		{"head -n 5 x.sql | psql", []string{"the file x.sql that `head` prints into it on its standard input"}},
		{"cat x.sql | xargs psql -c", []string{"prints into it (through `xargs`) in its arguments"}},
		{"xargs -a x.sql psql -c", []string{"the file x.sql that `xargs` reads its items from in its arguments"}},
		{"f() { psql; }; f < x.sql", []string{"the file x.sql (through the function f) on its standard input"}},
	} {
		d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: cwd})
		if d == nil {
			t.Errorf("allows %q", tt.command)
			continue
		}
		for _, w := range append(tt.want, askUserToRun, "belt[direct-deny-list]") {
			if !strings.Contains(d.Reason, w) {
				t.Errorf("reason for %q lacks %q:\n%s", tt.command, w, d.Reason)
			}
		}
	}
}

// TestDirectDenyListStdinNeedsAPhraseEntry: with no entry that names a
// phrase, no command reads its standard input.
func TestDirectDenyListStdinNeedsAPhraseEntry(t *testing.T) {
	cwd := stdinFixture(t)
	g := NewDirectDenyList(config.Config{ClaudeDeny: []string{"dropdb", "kubectl delete"}})
	for _, c := range []string{"psql < x.sql", "psql < folder", "echo 'dropdb x' | psql", "psql < $F"} {
		if d := g.Check(Input{Event: EventBash, Command: c, Cwd: cwd}); d != nil {
			t.Errorf("denies %q:\n%s", c, d.Reason)
		}
	}
}

// scriptStdin are rows on the SQL a nested phrase head or one in a script
// file reads on standard input, which script-deny-list reads the way
// direct-deny-list reads a call's own commands. The top level is
// direct-deny-list's.
var scriptStdin = []struct {
	command string
	deny    bool
}{
	{"bash -c \"psql <<'SQL'\nDROP TABLE t;\nSQL\n\"", true},
	{"bash -c 'psql < x.sql'", true},
	{"bash -c 'psql < ok.sql'", false},
	{"bash -c 'psql <<< \"select 1\"'", false},
	{"sh -c \"echo 'DROP TABLE t' | psql\"", true},
	{`bash -c 'psql < "$F"'`, true},
	{"cd sub && bash -c 'psql < only.sql'", true},
	{"bash -c 'cd sub && psql < only.sql'", true},
	{"bash -c 'cd sub && psql < plain.sql'", false},
	{"bash -c 'psql < plain.sql'", true},
	// A group, a ( ) and an exec inside the nested code, read the way
	// direct-deny-list reads them.
	{"bash -c 'for i in 1; do psql; done < x.sql'", true},
	{"bash -c 'while true; do psql; break; done < x.sql'", true},
	{"bash -c 'until false; do psql; break; done < x.sql'", true},
	{"bash -c 'if true; then psql; fi < x.sql'", true},
	{"bash -c 'case a in a) psql;; esac < x.sql'", true},
	{"bash -c '{ psql; } < x.sql'", true},
	{"bash -c \"{ psql; } <<'E'\nDROP TABLE t;\nE\n\"", true},
	{"bash -c '( psql ) < x.sql'", true},
	{"bash -c 'exec < x.sql; psql'", true},
	{"bash -c 'exec 0< x.sql\npsql'", true},
	{"bash -c 'exec 3< x.sql; exec 0<&3; psql'", true},
	{"bash -c 'exec 3< x.sql; psql 0<&3'", true},
	{"bash -c 'psql < /dev/stdin'", true},
	{"bash -c 'psql < /dev/zero'", true},
	{"bash -c 'for i in 1; do psql; done < ok.sql'", false},
	{"bash -c '{ psql; } < ok.sql'", false},
	{"bash -c '( psql ) < ok.sql'", false},
	{"bash -c 'exec < ok.sql; psql'", false},
	{"bash -c '( exec < x.sql ); psql'", false},
	// Substitutions that print a file, xargs, function calls and head, tail
	// or tac, read inside the nested code the way direct-deny-list reads
	// them at the top.
	{`bash -c 'psql <<< "$(cat x.sql)"'`, true},
	{`bash -c 'psql <<< "$(< x.sql)"'`, true},
	{`bash -c 'echo "$(cat x.sql)" | psql'`, true},
	{`bash -c 'psql -c "$(cat x.sql)"'`, true},
	{"bash -c \"psql <<EOF\n\\$(cat x.sql)\nEOF\n\"", true},
	{"bash -c 'cat x.sql | xargs psql -c'", true},
	{"bash -c 'xargs psql -c < x.sql'", true},
	{"bash -c 'xargs -a x.sql psql -c'", true},
	{"bash -c 'xargs --arg-file=x.sql psql -c'", true},
	{"bash -c 'f() { psql; }; f < x.sql'", true},
	{"bash -c 'function f { psql; }; f < x.sql'", true},
	{"bash -c 'f() { psql; }; exec < x.sql; f'", true},
	{"bash -c 'head -n 100 x.sql | psql'", true},
	{`bash -c 'psql <<< "$(cat ok.sql)"'`, false},
	{`bash -c 'psql -c "$(cat ok.sql)"'`, false},
	{"bash -c \"psql <<EOF\nselect '\\$(date -I)';\nEOF\n\"", false},
	{"bash -c 'cat ok.sql | xargs psql -c'", false},
	{"bash -c 'xargs -a ok.sql psql -c'", false},
	{"bash -c 'f() { psql; }; f < ok.sql'", false},
	{"bash -c 'head -n 100 ok.sql | psql'", false},
	{`bash -c 'SQL="DROP TABLE t"; psql <<< "$SQL"'`, false},
	// A nested shell starts with its caller's standard input, which its own
	// exec lines may replace.
	{"bash -c 'psql' < x.sql", true},
	{"sh -c 'psql' < x.sql", true},
	{"exec < x.sql; bash -c 'psql'", true},
	{"{ bash -c 'psql'; } < x.sql", true},
	{`exec < x.sql; echo "$(psql)"`, true},
	{"echo 'DROP TABLE t' | bash -c 'psql'", true},
	{"bash -c 'exec 3<&0; exec < ok.sql; exec 0<&3; psql' < x.sql", true},
	{"bash -c \"bash -c 'psql' < x.sql\"", true},
	{"bash -c \"exec < x.sql; bash -c 'psql'\"", true},
	{"bash -c \"{ bash -c 'psql'; } < x.sql\"", true},
	{`bash -c 'exec < x.sql; echo "$(psql)"'`, true},
	{"bash -c 'psql' < ok.sql", false},
	{"sh -c 'psql' < ok.sql", false},
	{"exec < ok.sql; bash -c 'psql'", false},
	{"{ bash -c 'psql'; } < ok.sql", false},
	{`exec < ok.sql; echo "$(psql)"`, false},
	{"bash -c 'exec < ok.sql; psql' < x.sql", false},
	{"bash -c 'psql' 3< x.sql", false},
	{"echo \"$(psql <<'SQL'\nDROP TABLE t;\nSQL\n)\"", true},
	{"bash <<'EOF'\npsql <<'SQL'\nDROP TABLE t;\nSQL\nEOF", true},
	{"bash run.sh", true},
	{"bash ok.sh", false},
	{"sh file.sh", true},
	{"psql <<'SQL'\nDROP TABLE t;\nSQL", false},
}

// TestScriptDenyListReadsStdin: script-deny-list reads a nested phrase
// head's standard input and a script file's the same way.
func TestScriptDenyListReadsStdin(t *testing.T) {
	cwd := stdinFixture(t)
	g := NewScriptDenyList(config.Config{ClaudeDeny: trancheEntries})
	for _, tt := range scriptStdin {
		d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: cwd})
		if got := d != nil; got != tt.deny {
			reason := ""
			if d != nil {
				reason = d.Reason
			}
			t.Errorf("deny = %v, want %v:\n%s\n%s", got, tt.deny, tt.command, reason)
		}
	}
	d := g.Check(Input{Event: EventBash, Command: "bash run.sh", Cwd: cwd})
	if d == nil || !strings.Contains(d.Reason, "(line 2) with the SQL from the heredoc") ||
		!strings.Contains(d.Reason, `"psql * DROP *"`) {
		t.Errorf("script file reason: %v", d)
	}
	d = g.Check(Input{Event: EventBash, Command: "bash -c 'psql' < x.sql", Cwd: cwd})
	if d == nil || !strings.Contains(d.Reason, "the nested command runs \"psql * DROP *\" (line 1) "+
		"with the SQL from the file x.sql on its standard input") {
		t.Errorf("caller's standard input reason: %v", d)
	}
}

// TestDirectDenyListStdinOpensNoDescriptor: with a regular file on belt's
// own descriptor 0, as when `belt check bash` is fed from a file, a
// descriptor path denies, and belt leaves its own input unread.
func TestDirectDenyListStdinOpensNoDescriptor(t *testing.T) {
	cwd := stdinFixture(t)
	in := filepath.Join(cwd, "stdin.txt")
	if err := os.WriteFile(in, []byte("select 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/stdin", filepath.Join(cwd, "in.sql")); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := unix.Dup(0)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Dup2(int(f.Fd()), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = unix.Dup2(saved, 0)
		_ = unix.Close(saved)
		_ = f.Close()
	})
	g := NewDirectDenyList(config.Config{ClaudeDeny: trancheEntries})
	for _, c := range []string{"psql < /dev/stdin", "psql < /dev/fd/0", "psql < in.sql", "cat /dev/stdin | psql"} {
		if d := g.Check(Input{Event: EventBash, Command: c, Cwd: cwd}); d == nil {
			t.Errorf("allows %q", c)
		}
	}
	if off, err := unix.Seek(0, 0, io.SeekCurrent); err != nil || off != 0 {
		t.Errorf("belt read its own standard input: offset %d, %v", off, err)
	}
}

// TestDirectDenyListStdinReadsRegularFilesAlone: a FIFO, a link to a device
// and a link to a descriptor deny at once, without an open that could block;
// a link to a regular file is read.
func TestDirectDenyListStdinReadsRegularFilesAlone(t *testing.T) {
	cwd := stdinFixture(t)
	if err := syscall.Mkfifo(filepath.Join(cwd, "fifo.sql"), 0o644); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{
		"zero.sql": "/dev/zero", "fd.sql": "/dev/fd/0", "drop.sql": "x.sql", "select.sql": "ok.sql",
		"dir": "sub", "loop.sql": "loop.sql",
	} {
		if err := os.Symlink(target, filepath.Join(cwd, link)); err != nil {
			t.Fatal(err)
		}
	}
	g := NewDirectDenyList(config.Config{ClaudeDeny: trancheEntries})
	for _, tt := range []struct {
		command string
		deny    bool
	}{
		{"psql < fifo.sql", true},
		{"cat fifo.sql | psql", true},
		{"psql < zero.sql", true},
		{"psql < fd.sql", true},
		{"psql < loop.sql", true},
		{"psql < drop.sql", true},
		{"psql < dir/only.sql", true},
		{"psql < select.sql", false},
	} {
		done := make(chan *Denial, 1)
		go func() { done <- g.Check(Input{Event: EventBash, Command: tt.command, Cwd: cwd}) }()
		select {
		case d := <-done:
			if got := d != nil; got != tt.deny {
				t.Errorf("deny = %v, want %v: %s", got, tt.deny, tt.command)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%q blocks", tt.command)
		}
	}
}

// TestDirectDenyListStdinRoot: with stdinRoot set, the reads take it for
// the root of the file system and read no file of the host.
func TestDirectDenyListStdinRoot(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{"tmp/drop.sql": "DROP TABLE t;\n", "tmp/ok.sql": "select 1;\n"} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	g := NewDirectDenyList(config.Config{ClaudeDeny: trancheEntries})
	g.stdinRoot = root
	for _, tt := range []struct {
		command string
		deny    bool
	}{
		{"psql < /tmp/ok.sql", false},
		{"cd /tmp && psql < ok.sql", false},
		{"psql < /tmp/drop.sql", true},
		{"cd /tmp; psql < missing.sql", true},
		{"psql < /dev/null", false},
	} {
		d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: root})
		if got := d != nil; got != tt.deny {
			t.Errorf("deny = %v, want %v: %s", got, tt.deny, tt.command)
		}
	}
}
