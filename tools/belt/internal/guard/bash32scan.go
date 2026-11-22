package guard

import "strings"

// bash32Closes reports whether bash 3.2, macOS's /bin/bash, runs no more of
// a $( ) whose text is sub than cat, when belt reads sub as a
// `cat <<'EOF' ... EOF` heredoc (dataCat). bash 3.2 knows no heredoc inside
// $( ) and finds its ) twice: the parse step (bash32Scan.pair) ends the
// word, and the expansion (bash32Expansion.command) ends the command it
// runs. Each stops at the first ) it cannot match, so a ) in the body can
// end the substitution early, and a quote, backtick or comment in the body
// changes which ) that is. The two read a # apart: the expansion takes one
// after a blank for a comment inside double quotes too, the parse step only
// outside them. sub passes when both scans end at the ) after sub and keep
// sub as written. Inside double quotes (quoted) it also passes when the
// earlier of the two ends past the first line and the rest of sub from
// there holds no ", backtick, $ or backslash: bash 3.2 then runs cat on part
// of the heredoc, which has lost its last line, and reads the rest as text
// of the same quoted word, in both scans. Unquoted, the rest would be
// commands. Everything else fails closed: the body is then scanned as code,
// unless bash 3.2 never closes the $( ) at all (bash32Unclosed).
func bash32Closes(sub string, quoted bool) bool {
	if strings.ContainsAny(sub, bash32Escaped) {
		return false
	}
	text := sub + ")"
	parsed, end := bash32Scan(text).pair(0, 0, '(', ')', substFlags(quoted))
	if end != pairClosed {
		return false
	}
	expanded, ok := bash32Expansion(text).command(0)
	switch {
	case !ok:
		return false
	case parsed == len(text) && expanded == len(text):
		return true
	}
	early := min(parsed, expanded)
	firstLine := strings.IndexByte(sub, '\n')
	return quoted && firstLine >= 0 && early-1 > firstLine &&
		!strings.ContainsAny(text[early:], "\"`$\\")
}

// bash32Unclosed reports whether bash 3.2, reading cmd as its whole input,
// stops at a syntax error before it runs the $( ) whose body is sub: its
// parse step reads on from sub through the rest of cmd and never closes the
// $( ), so the command the $( ) stands in never runs, nor anything after it.
// The body is then data, as bash 5 and zsh read it. bash 3.2 must reach the
// $( ) where belt does, so the text in front of it holds no $, backtick,
// backslash or process substitution, which it may read another way: an
// earlier $( ) it closes past belt's close may take in this one. The parse
// step must keep the rest of cmd as written too (pairUnread).
func bash32Unclosed(cmd string, sub span, quoted bool) bool {
	head, ok := strings.CutSuffix(cmd[:sub.from], "$(")
	if !ok || strings.ContainsAny(cmd, bash32Escaped) || strings.ContainsAny(head, "$`\\") ||
		strings.Contains(head, "<(") || strings.Contains(head, ">(") {
		return false
	}
	_, end := bash32Scan(cmd[sub.from:]).pair(0, 0, '(', ')', substFlags(quoted))
	return end == pairOpen
}

// bash32Rejoins reports whether bash 3.2 reads the text after the $( ) p as
// belt does, though it may run more of its body than cat (bash32Closes
// refuses it). wordEnd is where belt ends the word that holds p. bash 3.2's
// parse step, which ends the word, reads on through the rest of cmd from
// the ) it ends p at (bash32WordEnd). It ends the word where belt does when
// that ) is belt's, or when its reading of the rest says so. message says
// the word is message text a command only takes in, at the end of the
// shell's whole input: a word that takes in all the rest of it is passed
// on, not run. The expansion then reads the word again (bash32Expands),
// and must run nothing after belt's ) that belt reads as data. Anything
// else can leave a quote of the body open into the text after it, which
// may then run as code where belt reads data.
func bash32Rejoins(cmd string, p wordPart, wordEnd int, message bool) bool {
	sub := p.body
	if strings.ContainsAny(cmd, bash32Escaped) || sub.to >= len(cmd) || cmd[sub.to] != ')' {
		return false
	}
	end, how := bash32Scan(cmd[sub.from:]).pair(0, 0, '(', ')', substFlags(p.quoted))
	if how != pairClosed {
		return false
	}
	at := wordEnd
	if sub.from+end != sub.to+1 {
		var ok bool
		if at, ok = bash32WordEnd(cmd, sub.from+end, sub.to+1, p.quoted); !ok {
			return false
		}
	}
	switch {
	case at == wordEnd:
		return bash32Expands(cmd, sub, at, p.quoted, false)
	case message && at > wordEnd && strings.TrimSpace(cmd[at:]) == "":
		return bash32Expands(cmd, sub, at, p.quoted, true)
	}
	return false
}

// bash32Expands reports whether bash 3.2's expansion (bash32Expansion), as
// it reads the word the parse step ended at end again, runs nothing after
// belt's ) of the $( ) sub that belt reads as data. It ends the $( ) where
// it can match a ), which may not be the parse step's or belt's. It may end
// it at belt's ), or earlier and read the rest of the body as text of the
// same word, quoted as belt quotes the text after the ) (bash32Reaches). It
// then reads the rest of the word as belt does, up to belt's end of it.
// rest says the word takes in the text after belt's end of it too, where
// the expansion must run nothing (bash32Inert). When it ends the $( ) past
// belt's ), the $( ) runs cat on the heredoc and then stops at belt's ):
// bash 3.2 runs no command that starts with a ). The word then goes on
// from that later ), and the expansion must run nothing there either. When
// it finds no ) in the word at all, bash 3.2 reports a bad substitution
// and drops the command before it expands the word, so nothing in the word
// runs.
func bash32Expands(cmd string, sub span, end int, quoted, rest bool) bool {
	closed, ok := bash32Expansion(cmd[sub.from:end]).command(0)
	if !ok {
		return true
	}
	closed += sub.from
	after := sub.to + 1
	switch {
	case closed > after:
		return bash32Inert(cmd[closed:end], quoted)
	case closed < after && !bash32Reaches(cmd[closed:after], quoted):
		return false
	}
	return !rest || bash32Inert(cmd[after:end], quoted)
}

// bash32Reaches reports whether bash 3.2's expansion, reading text as the
// rest of a word from inside double quotes when quoted
// (expand_word_internal), ends text in the same quoting, with each quote,
// backtick, $( ), ${ } and <( ) it opens ended inside text. Text inside
// double quotes is read as string_extract_double_quoted reads it
// (bash32Expansion.doubleQuoted).
func bash32Reaches(text string, quoted bool) bool {
	s := bash32Expansion(text + `"`) // the " ends double quotes that reach the end of text
	start := quoted
	for at := 0; at < len(text); {
		next, ok := at+1, true
		switch rest := string(s[at:]); {
		case quoted:
			next, ok = s.doubleQuoted(at)
			if ok && next > len(text) {
				return start
			}
			quoted = false
		case rest[0] == '"':
			quoted = true
		case rest[0] == '\'':
			next, ok = s.singleQuoted(at + 1)
		case rest[0] == '\\':
			next = at + 2
		case rest[0] == '`':
			next, ok = s.backtick(at + 1)
		case strings.HasPrefix(rest, "${"):
			next, ok = s.brace(at + 2)
		case strings.HasPrefix(rest, "$("), strings.HasPrefix(rest, "<("), strings.HasPrefix(rest, ">("):
			next, ok = s.command(at + 2)
		}
		if !ok || next > len(text) {
			return false
		}
		at = next
	}
	return quoted == start
}

// bash32Inert reports whether bash 3.2's expansion, reading text as the
// rest of a word from inside double quotes when quoted, runs, splits and
// globs nothing in it: text holds no $, backtick or backslash outside
// single quotes, and outside double quotes too no <( or >( and no glob or
// brace. A ' outside double quotes starts text read as written up to the
// next ' or the end of the word.
func bash32Inert(text string, quoted bool) bool {
	for at := 0; at < len(text); at++ {
		switch ch := text[at]; {
		case strings.IndexByte("$`\\", ch) >= 0:
			return false
		case ch == '"':
			quoted = !quoted
		case quoted:
		case ch == '\'':
			end := strings.IndexByte(text[at+1:], '\'')
			if end < 0 {
				return true
			}
			at += end + 1
		case strings.IndexByte("*?[{", ch) >= 0,
			strings.IndexByte("<>", ch) >= 0 && at+1 < len(text) && text[at+1] == '(':
			return false
		}
	}
	return true
}

// bash32WordEnd returns where bash 3.2's parse step ends a word it reads on
// from at, just past the ) it ends a $( ) at, inside double quotes when
// quoted. close is just past belt's ) of that $( ): before it lies the body,
// which belt reads fail-closed, and from it on text belt reads as words of
// its own. Each " switches between double-quoted text (bash32Quoted) and
// bare text, which ends the word at a blank or an operator byte. ok is
// false for a word that may run text belt reads as data, or split it in
// other places: bare text holding a quote, $, backtick, backslash, glob or
// brace, or double-quoted text bash32Quoted refuses.
func bash32WordEnd(cmd string, at, close int, quoted bool) (int, bool) {
	for at < len(cmd) {
		if quoted {
			next, ok := bash32Quoted(cmd, at, close)
			if !ok {
				return 0, false
			}
			at, quoted = next, false
			continue
		}
		switch ch := cmd[at]; {
		case ch == '"':
			quoted = true
		case strings.IndexByte(wordEnds, ch) >= 0:
			return at, true
		case strings.IndexByte("'`$\\*?[{", ch) >= 0:
			return 0, false
		}
		at++
	}
	return at, !quoted
}

// bash32Quoted returns the index just past the " that ends the
// double-quoted text bash 3.2 reads from at (bash32WordEnd). Before close,
// in the body, it follows the backticks, $( ), ${ } and $[ ] that text opens
// (bash32Scan.pair), which must end before close. From close on, in text
// belt reads as its own words, it takes no $, backtick or backslash, since
// bash 3.2 would expand them where belt reads plain text.
func bash32Quoted(cmd string, at, close int) (int, bool) {
	if at < close {
		body := cmd[at:close] + `"`
		end, how := bash32Scan(body).pair(0, '"', '"', '"', 0)
		switch {
		case how != pairClosed:
			return 0, false
		case end < len(body):
			return at + end, true
		}
		at = close // still inside the double quotes at belt's )
	}
	end := strings.IndexAny(cmd[at:], "\"$`\\")
	if end < 0 || cmd[at+end] != '"' {
		return 0, false
	}
	return at + end + 1, true
}

// substFlags are the flags bash 3.2's parse step reads a $( ) with: inside
// double quotes (quoted) none, outside them P_COMMAND.
func substFlags(quoted bool) pairFlags {
	if quoted {
		return 0
	}
	return pairCommand
}

// bash32Escaped are CTLESC and CTLNUL, the bytes bash 3.2's scan stores with
// an escape in front of them.
const bash32Escaped = "\x01\x7f"

// pairFlags are the flags of bash 3.2's parse_matched_pair (parse.y) that
// change how it scans.
type pairFlags uint8

const (
	pairFirstClose pairFlags = 1 << iota // P_FIRSTCLOSE: an inner { does not nest
	pairDquote                           // P_DQUOTE: inside double quotes
	pairCommand                          // P_COMMAND: a command, where # starts a comment
)

// bash32Quotes are the bytes parse_matched_pair reads a quoted string from
// inside a $( ), ${ } or $[ ] (shellquote).
const bash32Quotes = "'\"`"

// bash32Scan is text as bash 3.2's parse_matched_pair reads it.
type bash32Scan string

// pairEnd says how a parse_matched_pair scan ended.
type pairEnd uint8

const (
	pairClosed pairEnd = iota // at the close that ends it
	pairOpen                  // the text ended first: bash 3.2 reports a syntax error there
	pairUnread                // where bash 3.2 keeps other text than the scan holds
)

// pair reads s from i, just past open, the way parse_matched_pair(qc, open,
// close, flags) does, and returns the index just past the close that ends
// it, with pairClosed. qc is the quote the construct stands in, 0 for none.
// It returns pairOpen when s ends first, and pairUnread where bash 3.2 keeps
// other text than s holds: outside single quotes it drops a
// backslash-newline, and it translates $'...' and $"..." in a $( ), ${ } or
// $[ ].
func (s bash32Scan) pair(i int, qc, open, close byte, flags pairFlags) (int, pairEnd) {
	comments := flags&(pairCommand|pairDquote) == pairCommand && strings.IndexByte(bash32Quotes, qc) < 0
	inner := flags & pairDquote
	if qc == '"' {
		inner = pairDquote
	}
	count, escaped, dollar, comment := 1, false, false, false
	var last byte // the byte the scan kept last, 0 before the first
	for count > 0 {
		if i >= len(s) {
			return 0, pairOpen
		}
		ch := s[i]
		if ch == '\\' && qc != '\'' && !escaped && i+1 < len(s) && s[i+1] == '\n' {
			return 0, pairUnread
		}
		i++
		switch {
		case comment:
			comment, last = ch != '\n', ch
			continue
		case escaped:
			escaped, last = false, ch
			continue
		case comments && ch == '#' && strings.IndexByte("\x00\n \t", last) >= 0:
			comment = true
		}
		switch {
		case ch == close:
			count--
		case open == '{' && dollar && ch == '{':
			count++
		case flags&pairFirstClose == 0 && ch == open:
			count++
		}
		last = ch
		if count == 0 || open == '\'' {
			continue
		}
		escaped = ch == '\\'
		next, end := s.nested(i, ch, open, close, dollar, inner)
		if end != pairClosed {
			return 0, end
		}
		if next > i {
			i, last = next, s[next-1]
		}
		dollar = ch == '$'
	}
	return i, pairClosed
}

// nested reads the construct ch starts at i inside a pair from open to
// close, as parse_matched_pair does after it keeps ch, and returns the
// index just past it, or i when ch starts none, with how it ended (pair).
// dollar says a $ came just before ch; inner is the P_DQUOTE flag the pair
// hands on. Inside a $( ), ${ } or $[ ] a quote starts a quoted string;
// inside double quotes a backtick, $( ), ${ } or $[ ] starts one of those.
func (s bash32Scan) nested(i int, ch, open, close byte, dollar bool, inner pairFlags) (int, pairEnd) {
	switch {
	case open != close:
		if strings.IndexByte(bash32Quotes, ch) < 0 {
			return i, pairClosed
		}
		if dollar && ch != '`' {
			return 0, pairUnread // $'...' or $"...", which bash 3.2 translates
		}
		return s.pair(i, ch, ch, ch, inner)
	case open == '"' && ch == '`':
		return s.pair(i, 0, '`', '`', inner)
	case open == '"' && dollar && ch == '(':
		return s.pair(i, 0, '(', ')', inner&^pairDquote)
	case open == '"' && dollar && ch == '{':
		return s.pair(i, 0, '{', '}', pairFirstClose|inner)
	case open == '"' && dollar && ch == '[':
		return s.pair(i, 0, '[', ']', inner)
	}
	return i, pairClosed
}

// bash32Expansion is text as bash 3.2's word expansion reads it (subst.c),
// the second scan that finds the ) of a $( ): extract_delimited_string with
// EX_COMMAND, which string_extract_double_quoted, param_expand and
// extract_dollar_brace_string all call for a $( ). Unlike the parse step it
// takes a # after a blank or newline for a comment start inside double
// quotes too, and it knows no ${ } of its own.
type bash32Expansion string

// command reads s from i, just past a $( or (, the way
// extract_delimited_string(s, &i, "$(", "(", ")", EX_COMMAND) does, and
// returns the index just past the ) that ends it. ok is false when s ends
// first. s starts just past the $( of the substitution, so the byte before
// index 0 is that (.
func (s bash32Expansion) command(i int) (int, bool) {
	escaped, comment := false, false
	for i < len(s) {
		ch := s[i]
		next, ok := i+1, true
		switch {
		case comment:
			comment = ch != '\n'
		case escaped:
			escaped = false
		case ch == '#' && i > 0 && strings.IndexByte("\n \t", s[i-1]) >= 0:
			comment = true
		case ch == '\\':
			escaped = true
		case ch == '(':
			next, ok = s.command(i + 1) // a $( or a ( nests the same way
		case ch == ')':
			return i + 1, true
		case ch == '`':
			next, ok = s.backtick(i + 1)
		case ch == '\'':
			next, ok = s.singleQuoted(i + 1)
		case ch == '"':
			next, ok = s.doubleQuoted(i + 1)
		}
		if !ok {
			return 0, false
		}
		i = next
	}
	return 0, false
}

// backtick reads s from i, just past a backtick, as string_extract(s, &i,
// "`", 0) does, and returns the index just past the backtick that ends it.
func (s bash32Expansion) backtick(i int) (int, bool) {
	for ; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '`':
			return i + 1, true
		}
	}
	return 0, false
}

// singleQuoted reads s from i, just past a ', as skip_single_quoted does.
func (s bash32Expansion) singleQuoted(i int) (int, bool) {
	end := strings.IndexByte(string(s[i:]), '\'')
	if end < 0 {
		return 0, false
	}
	return i + end + 1, true
}

// doubleQuoted reads s from i, just past a ", as skip_double_quoted does:
// a backslash quotes the next byte, a backtick pair is skipped, and a $( or
// ${ is read to its end.
func (s bash32Expansion) doubleQuoted(i int) (int, bool) {
	backquote := false
	for i < len(s) {
		ch := s[i]
		next, ok := i+1, true
		switch {
		case ch == '\\':
			next = i + 2
		case backquote:
			backquote = ch != '`'
		case ch == '`':
			backquote = true
		case ch == '$' && i+1 < len(s) && s[i+1] == '(':
			next, ok = s.command(i + 2)
		case ch == '$' && i+1 < len(s) && s[i+1] == '{':
			next, ok = s.brace(i + 2)
		case ch == '"':
			return i + 1, true
		}
		if !ok {
			return 0, false
		}
		i = next
	}
	return 0, false
}

// brace reads s from i, just past a ${, as extract_dollar_brace_string does,
// and returns the index just past the } that ends it.
func (s bash32Expansion) brace(i int) (int, bool) {
	depth := 1
	for i < len(s) {
		ch := s[i]
		next, ok := i+1, true
		switch {
		case ch == '\\':
			next = i + 2
		case ch == '$' && i+1 < len(s) && s[i+1] == '{':
			depth, next = depth+1, i+2
		case ch == '}':
			if depth--; depth == 0 {
				return i + 1, true
			}
		case ch == '`':
			next, ok = s.backtick(i + 1)
		case ch == '$' && i+1 < len(s) && s[i+1] == '(':
			next, ok = s.command(i + 2)
		case ch == '\'':
			next, ok = s.singleQuoted(i + 1)
		case ch == '"':
			next, ok = s.doubleQuoted(i + 1)
		}
		if !ok {
			return 0, false
		}
		i = next
	}
	return 0, false
}
