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
