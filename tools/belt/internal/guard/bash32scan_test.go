package guard

import "testing"

// TestBash32Closes pins how bash 3.2, macOS's /bin/bash, reads a
// `$(cat <<'EOF' ... EOF)` body, inside double quotes (quoted) and outside,
// in its parse step and in its expansion. The parse step of each row was
// checked against /bin/bash 3.2.57 with bash -D, which parses without
// running anything. The expansion was checked by running bash 3.2 in a
// sandbox on the same shapes built from letters, quotes, #, parens and echo
// markers, whose output shows what it runs.
func TestBash32Closes(t *testing.T) {
	cat := func(body string) string { return "cat <<'EOF'\n" + body + "\nEOF\n" }
	tests := []struct {
		name   string
		sub    string
		quoted bool
		want   bool
	}{
		{"plain words", cat("plain words"), true, true},
		{"a backslash-quoted delimiter", "cat <<\\EOF\nx\nEOF\n", true, true},
		{"a backtick pair", cat("fix(belt): read `rm -rf` behind sudo"), true, true},
		{"a ) in a backtick pair", cat("a `)` b"), true, true},
		{"an issue ref, quoted", cat("Closes #12"), true, true},
		{"an issue ref, unquoted", cat("Closes #12"), false, true},
		{"a # heading", cat("## Summary\n- block rm -rf"), true, true},
		{"a $ and a ${ }", cat("see `$HOME` and \"${x:-)}\""), true, true},
		{"paired apostrophes", cat("it's (one) and it's two"), true, true},
		{"an escaped paren", cat("x \\) y"), true, true},
		{"a ( in a double-quoted pair", cat("say \"(x\" ok"), true, true},
		{"an apostrophe in a backtick pair", cat("run `don't` here"), true, true},
		// A ) the scans cannot match ends the substitution early. Inside
		// double quotes the rest is text of the same word when it holds no
		// quote, backtick, $ or backslash from the earlier of the two closes.
		{"a numbered list, quoted", cat("fix: two cases\n1) rm -rf\n2) a wrapper"), true, true},
		{"a numbered list, unquoted", cat("fix: two cases\n1) rm -rf\n2) a wrapper"), false, false},
		{"an early ) then a quote", cat("x\n)\"; rm -rf y; echo \"("), true, false},
		{"an early ) then a substitution", cat("a\n1) x $(rm -rf y)"), true, false},
		{"an early ) then backticks", cat("a\n1) x `rm -rf y`"), true, false},
		{"an early ) then a backslash", cat("a\n1) x \\\n2) y"), true, false},
		{"an early ) on the first line", "x) y\nz\n", true, false},
		{"an early ) then another $(", cat("x\n)\" \"$("), true, false},
		// The parse step takes a # after a blank for a comment start outside
		// double quotes only; the expansion takes one inside them too. A )
		// in a comment counts for nothing, and a quote there opens nothing.
		{"a ) after an issue ref, quoted", cat("fix (see #12) rm -rf"), true, false},
		{"a ) after an issue ref, unquoted", cat("fix (see #12) rm -rf"), false, false},
		{"an apostrophe in a comment, unquoted", cat("# don't rm -rf"), false, true},
		{"an apostrophe in a heading, quoted", cat("# don't rm -rf"), true, false},
		{"a ) in a comment, unquoted", cat("a #b)"), false, true},
		{"a ) in a heading, quoted", cat("## step 1)\n- rm -rf"), true, true},
		{"a ) in a heading, then backticks", cat("## step 1)\nrun `rm -rf` here"), true, false},
		{"an escaped #, unquoted", cat("a\\#b) c"), false, false},
		{
			"an apostrophe in a heading, then backticks",
			cat("## What's new\n1) a fix for `rm -rf y`\n- don't break it"),
			true,
			false,
		},
		{"an apostrophe in a heading, then a substitution", cat("# it's\n1) $(rm -rf y)\nx'"), true, false},
		{"a # in a $( ) in a double-quoted pair", cat("say \"$( # x)\" ok"), true, false},
		// The expansion knows no ${ } of its own, so a ) in one ends the
		// substitution there.
		{"a ) in a bare ${ }, quoted", cat("a ${x:-)} b"), true, true},
		{"a ) in a bare ${ }, unquoted", cat("a ${x:-)} b"), false, false},
		// What the scan cannot pair up runs past the end.
		{"a lone apostrophe", cat("don't"), true, false},
		{"an unmatched (", cat("("), true, false},
		{"a $( in a double-quoted pair", cat("say \"a $(b\" ok\""), true, false},
		// Where bash 3.2 keeps other text than the body holds.
		{"a backslash-newline", cat("EO\\\nF\nrm -rf y"), true, false},
		{"a $'...'", cat("a $'\\nEOF\\nrm -rf y\\n'"), true, false},
		{"a $\"...\"", cat("a $\"x\" b"), true, false},
		{"a CTLESC byte", cat("a \x01 b"), true, false},
	}
	for _, tt := range tests {
		if got := bash32Closes(tt.sub, tt.quoted); got != tt.want {
			t.Errorf("%s: bash32Closes(%q, %v) = %v, want %v", tt.name, tt.sub, tt.quoted, got, tt.want)
		}
	}
}

// TestBash32Unclosed pins when bash 3.2 stops at a syntax error before it
// runs a `$(cat <<'EOF' ... EOF)`: its parse step reads on from the body
// through the rest of the command and never closes the $( ). Each row was
// checked against /bin/bash 3.2.57 with bash -n and bash -D, which parse
// without running anything.
func TestBash32Unclosed(t *testing.T) {
	cat := func(body string) string { return "cat <<'EOF'\n" + body + "\nEOF\n" }
	const (
		commit = `git commit -m "`
		pr     = `gh pr create --title "fix(belt): stash drop" --body "`
	)
	stash := cat("belt now denies `git stash drop` and `git stash clear`, since every worktree " +
		"shares one stash list.\n\nIt's covered by new test rows.")
	tests := []struct {
		name, head, sub, tail string
		quoted, want          bool
	}{
		{"a lone apostrophe in a pr body", pr, stash, `"`, true, true},
		{"a lone apostrophe in a commit message", commit, stash, `"`, true, true},
		{"a second message after it", commit, stash, `" -m "second (one)"`, true, true},
		{"an && step after it", commit, stash, `" && git status`, true, true},
		{"a second line after it", commit, stash, "\"\ngit status", true, true},
		{"a later apostrophe, nothing to close the $( )", pr, stash, `" --label "it's x"`, true, true},
		{"an unquoted message", `git commit -m `, cat("don't"), ``, false, true},
		{"an unclosed (", commit, cat("(x"), `"`, true, true},
		// A later word closes the quote and a ) after it the $( ): bash 3.2
		// parses the command and runs the $( ), so the message stays code.
		{"a later word closes the $( )", commit, cat("don't"), `" -m "it's done)"`, true, false},
		{"a later word on lines of its own", commit, cat("don't"), "\" -m 'x)\"\necho hi\n#'", true, false},
		{"an unquoted message a later word closes", `git commit -m `, cat("don't"), ` -m "it's x)"`, false, false},
		{"paired quotes close at the end", commit, cat("it's 'x', it's y"), `"`, true, false},
		// bash 3.2 may reach the $( ) another way than belt does.
		{"an earlier $( )", commit + "$(cat <<'EOF'\ndon't\nEOF\n)\" -m \"", cat("it's x)\"; echo \""), `"`, true, false},
		{"a $ in front", `git -C "$HOME/x" commit -m "`, cat("don't"), `"`, true, false},
		{"a backtick in front", "git commit -m \"`date` ", cat("don't"), `"`, true, false},
		{"a backslash in front", `git commit -m "a\"`, cat("don't"), `"`, true, false},
		{"a process substitution in front", `diff <(echo a) x; git commit -m "`, cat("don't"), `"`, true, false},
		// Where bash 3.2 keeps other text than the command holds.
		{"a $'...' after it", commit, cat("(x"), `"" $'a'`, true, false},
		{"a backslash-newline after it", commit, cat("(x"), "\" -m \"a\\\nb\"", true, false},
		{"a CTLESC byte after it", commit, cat("don't"), "\" -m \"a\x01\"", true, false},
	}
	for _, tt := range tests {
		cmd := tt.head + "$(" + tt.sub + ")" + tt.tail
		body := span{len(tt.head) + 2, len(tt.head) + 2 + len(tt.sub)}
		if got := bash32Unclosed(cmd, body, tt.quoted); got != tt.want {
			t.Errorf("%s: bash32Unclosed(%q, %v) = %v, want %v", tt.name, cmd, tt.quoted, got, tt.want)
		}
	}
}
