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

// TestBash32Rejoins pins when bash 3.2 reads the text after a
// `$(cat <<'EOF' ... EOF)` it may run more of than cat as belt does. word is
// the rest of the word that holds the $( ), and rest the text after that
// word. Each row was checked by running /bin/bash 3.2.57 on the same shape
// with printf in place of git commit and echo markers in place of the
// commands, whose output shows what it runs. A message word that takes in
// all the rest runs none of it, so message rows say whether the word is
// message text (messageWords).
func TestBash32Rejoins(t *testing.T) {
	cat := func(body string) string { return "cat <<'EOF'\n" + body + "\nEOF\n" }
	const commit = `git commit -m "`
	tests := []struct {
		name, head, sub, word, rest string
		quoted, message, want       bool
	}{
		// The parse step ends the $( ) at belt's ). The expansion reads the
		// word again: the rest reads alike when it ends the $( ) at belt's )
		// or earlier with the rest of the body as text of the same quoted word.
		{"the expansion ends it early", commit, cat("a # (\nb ) $HOME"), `"`, `; echo t 'p;q'`, true, false, true},
		{"the expansion ends it early, then a quote", commit, cat("a # (\nb ) \"'\""), `"'$(echo x)'`, ``, true, false, false},
		{"the expansion ends it early, then backticks", commit, cat("a # (\nb ) \"'\""), "\"'`echo x`'", ``, true, false, false},
		{"a list the expansion ends early", commit, cat("- fix # (part 1\n- done ) \"'\""), `"'$(echo x)'`, ``, true, false, false},
		{"a pr body the expansion ends early", `gh pr create --title t --body "`, cat("a # (\nb ) \"'\""), `"'$(echo x)'`, ``, true, true, false},
		// It ends it past belt's ): the $( ) runs cat and stops at belt's ),
		// and the word goes on from the later ).
		{"the expansion ends it late", commit, cat("a ( # )"), `"'$(echo x)'")"`, ``, true, false, true},
		{"the expansion ends it late, then a substitution", commit, cat("a ( # )"), `"'x'")"$(echo x)`, ``, true, false, false},
		// It finds no ) in the word: a bad substitution, which drops the command.
		{"the expansion never ends it", commit, cat("PR #1 renames the bar's x\nand speak's y"), `"`, ` && echo t`, true, false, true},
		{"the expansion never ends it, then a quote", commit, cat("PR #1 renames the bar's x\nand speak's y"), `"'$(echo x)'`, ``, true, false, true},
		// It ends it early, and the rest of the body is text of the word.
		{"a list, then backticks", commit, cat("x\n\n1) adds the `echo y` flag\n2) docs"), `"`, ` && echo t '(x)'`, true, false, true},
		{"a list, then quoted words", commit, cat("x\n\n1) renames \"a\" to \"b\"\n2) docs"), `"`, ` && echo t '(x)'`, true, false, true},
		{"a list, then a quoted phrase", commit, cat("x\n1) set \"max tries\" to 3"), `"`, ` && echo t`, true, false, false},
		{"a list, then a quote that ends the word", commit, cat("x\n1) say \"hi"), `"`, `; echo 'x'`, true, false, false},
		{"the expansion ends it earlier, then a <( )", commit, cat("a # (\nb ) '\"' ) 'z"), `<(echo x)"`, ``, true, false, false},
		{"an unquoted list", `git commit -m `, cat("1) it's"), ``, "; echo '\necho x ; #'", false, false, false},
		// It ends it past belt's ): a quote of the body is open in between.
		{"a later word closes the quote", commit, cat("It's done"), `"`, "; echo '\n)\" ; echo x ; #'", true, false, false},
		{"a later heredoc closes the quote", commit, cat("It's done"), `"`, "; cat <<'Y'\n')\" ; echo x ; #'\nY", true, false, false},
		{"a later comment closes the quote", commit, cat("It's done"), `"`, "; echo 'x'\n# ')\" ; echo x ; #'", true, false, false},
		{"a backtick past belt's )", commit, cat("It's x"), `"`, " && echo 'q\n) `echo x >&2` \"\necho \"'", true, true, false},
		{"a message that takes in the rest", commit, cat("It's x"), `"`, " && cat <<'Y'\n') ; echo x ; echo \"z", true, true, true},
		{"a -c script that takes in the rest", `sh -c "`, cat("It's x"), `"`, " && cat <<'Y'\n') ' ; echo x ; echo \"z", true, false, false},
		{"text after the message that takes in the rest", commit, cat("It's x"), `"`, " && cat <<'Y'\n') ; echo x ; echo \"z\nY\necho x", true, true, false},
		{"a later ) in a later word", commit, cat("- fix (part 1"), `"`, ` -m "next ) ; echo x ; echo ("`, true, true, true},
		// The message takes in the rest, but the expansion, which skips the
		// heading as a comment, reads the rest as text belt quotes.
		{"a message whose expansion runs a later heredoc", commit, cat("# It's x"), `"`, " && cat <<'Y'\nZ $(echo x) ')\nY\n# \"", true, true, false},
		{"a message whose expansion runs a later comment", commit, cat("# It's x"), `"`, " && echo 'q\nZ' # $(echo x) ')\"", true, true, false},
		{"a message whose expansion runs a later <( )", commit, cat("# It's x"), `"`, " && cat <<'Y'\nZ <(echo x) ')\nY\n# \"", true, true, false},
		// It never ends it: bash32Unclosed decides those.
		{"a lone apostrophe", commit, cat("don't"), `"`, ``, true, true, false},
	}
	for _, tt := range tests {
		cmd := tt.head + "$(" + tt.sub + ")" + tt.word + tt.rest
		p := wordPart{kind: partSubst, quoted: tt.quoted, body: span{len(tt.head) + 2, len(tt.head) + 2 + len(tt.sub)}}
		wordEnd := p.body.to + 1 + len(tt.word)
		if got := bash32Rejoins(cmd, p, wordEnd, tt.message); got != tt.want {
			t.Errorf("%s: bash32Rejoins(%q, %v) = %v, want %v", tt.name, cmd, tt.message, got, tt.want)
		}
	}
}
