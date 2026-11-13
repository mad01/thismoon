package guard

import "strings"

// readShell is the quote-aware reader the git guards walk and the script
// guard masks with. splitSegments splits a command at every &&, ||, ;, | and
// newline, quoted or not, so a push inside `(...)`, `$(...)` or `sh -c '...'`
// never reaches the git parser as a bare git word, and a `;` inside an echo
// string starts a fake command. readShell builds on the rm-temp lexer: it
// follows quotes, escapes, substitutions, comments and heredoc bodies, and
// it records where each piece lies so a caller can read the code inside a
// piece or blank out the data. It is still not a shell: on input it cannot
// follow it returns not ok, and each caller says what it does then.

// span is the byte range [from, to) of a command text.
type span struct{ from, to int }

// placedWord is one word as readShell reads it: the lexer's word and where it
// lies in the text.
type placedWord struct {
	shWord
	at span
}

// shellDoc is a heredoc body a command reads. quoted marks a delimiter with
// a quote or backslash in it, so the shell expands nothing in the body; odd
// marks a line that starts with the delimiter without being it
// (docBody).
type shellDoc struct {
	body   span
	quoted bool
	odd    bool
}

// shellCmd is one simple command in the order the shell meets it. A ( )
// subshell is a command of its own whose subshells holds the body, so the
// joints around it stay in place.
type shellCmd struct {
	words     []placedWord
	targets   []placedWord // redirection targets, heredoc delimiters included
	docs      []shellDoc
	subshells []span
	comments  []span // the text after a # on the command's line
	before    joint
	after     joint
}

// shellReader reads one command text into shellCmds.
type shellReader struct {
	lex    shLexer
	cmds   []shellCmd
	cur    shellCmd
	next   joint // the separator in front of the command being read
	owners []int // the index of the command each queued heredoc belongs to
	// closing makes an unmatched ) end the read: closeAt is where it stood.
	closing bool
	closed  bool
	closeAt int
	// failed marks text the reader read but cannot vouch for: a heredoc
	// inside ( ) or $( ) with an odd body, which bash and zsh end apart.
	failed bool
	// complete is where the last whole top-level line the reader read ends,
	// heredoc bodies included.
	complete int
}

// maxShellNesting is how deep the reader follows ( ), $( ), <( ) and >( )
// inside one another. Commands people write nest a few levels. Deeper text
// reads as text the reader cannot follow, so a crafted command cannot make
// every walk read it again level by level.
const maxShellNesting = 32

// readShell reads a command text into its simple commands. ok is false for
// text the reader cannot follow: an unterminated quote or substitution, an
// unmatched (, a redirection without a target, a heredoc inside ( ) or $( )
// with a line that starts with its delimiter without being it, or nesting
// past maxShellNesting.
func readShell(src string) ([]shellCmd, bool) {
	cmds, _, ok := readShellLines(src)
	return cmds, ok
}

// readShellLines is readShell that also says how far the text reads: to its
// end when ok, else to the end of the last whole line before the one the
// reader could not follow. bash runs those lines before it meets the syntax
// error, so a walker still reads them as code.
func readShellLines(src string) (cmds []shellCmd, readTo int, ok bool) {
	r := &shellReader{lex: shLexer{src: src}}
	if !r.run() {
		return nil, r.complete, false
	}
	return r.cmds, len(src), true
}

// closeParen reads s from from as the inside of a ( or $( at nesting depth
// depth and returns the index of the ) that closes it.
func closeParen(s string, from, depth int) (int, bool) {
	if depth > maxShellNesting {
		return 0, false
	}
	r := &shellReader{lex: shLexer{src: s, i: from, depth: depth}, closing: true}
	if !r.run() || !r.closed {
		return 0, false
	}
	return r.closeAt, true
}

// run reads to the end of the text, or to the closing ) in closing mode.
func (r *shellReader) run() bool {
	for r.lex.i < len(r.lex.src) && !r.closed {
		if !r.step() {
			return false
		}
	}
	r.end(jointList)
	r.bodies()
	return !r.failed
}

// step consumes one token: blanks, a separator, a comment, a subshell, a
// redirection or a word.
func (r *shellReader) step() bool {
	l := &r.lex
	switch c := l.src[l.i]; {
	case c == ' ' || c == '\t':
		l.i++
	case c == '\\' && l.at(1) == '\n':
		l.i += 2
	case c == '\n':
		l.i++
		r.end(jointList)
		r.bodies()
		if !r.closing {
			r.complete = l.i
		}
		return !r.failed
	case c == '#':
		start := l.i + 1
		l.skipComment()
		r.cur.comments = append(r.cur.comments, span{start, l.i})
	case c == '(':
		return r.subshell()
	case c == ')' && r.closing:
		r.closed, r.closeAt = true, l.i
	case c == ')' || c == ';':
		l.i++
		r.end(jointList)
	case c == '|' || c == '&':
		return r.operator()
	case (c == '<' || c == '>') && l.at(1) != '(':
		return r.redirect(l.redirect)
	default:
		return r.word()
	}
	return true
}

// operator consumes &&, ||, |, |&, a background & or an &> redirection.
func (r *shellReader) operator() bool {
	l := &r.lex
	switch two := l.src[l.i:min(l.i+2, len(l.src))]; {
	case two == "&&":
		l.i += 2
		r.end(jointAnd)
	case two == "||":
		l.i += 2
		r.end(jointOr)
	case two == "&>":
		l.i += 2
		if l.at(0) == '>' {
			l.i++
		}
		return r.redirect(func() bool { return l.redirectTarget(false, false) })
	case two == "|&":
		l.i += 2
		r.end(jointPipe)
	case two[0] == '|':
		l.i++
		r.end(jointPipe)
	default:
		l.i++
		r.end(jointBg)
	}
	return true
}

// redirect runs the lexer's redirection reader and keeps the target, and the
// command any heredoc it queued belongs to.
func (r *shellReader) redirect(read func() bool) bool {
	l := &r.lex
	queued := len(l.heredocs)
	if !read() {
		return false
	}
	r.cur.targets = append(r.cur.targets, placedWord{shWord: l.target, at: l.targetAt})
	for range l.heredocs[queued:] {
		r.owners = append(r.owners, len(r.cmds))
	}
	return true
}

// word reads one word. Digits glued to a redirection are its descriptor.
func (r *shellReader) word() bool {
	l := &r.lex
	start := l.i
	w, ok := l.readWord()
	if !ok {
		return false
	}
	if isDigits(w.raw) && (l.at(0) == '<' || l.at(0) == '>') {
		return true
	}
	r.cur.words = append(r.cur.words, placedWord{shWord: w, at: span{start, l.i}})
	return true
}

// subshell records a ( ) body as a command of its own. A ( glued to a word
// (a function name, a zsh glob qualifier) is recorded the same way.
func (r *shellReader) subshell() bool {
	l := &r.lex
	end, ok := l.matchParen(l.i)
	if !ok {
		return false
	}
	r.cur.subshells = append(r.cur.subshells, span{l.i + 1, end})
	l.i = end + 1
	return true
}

// end finishes the command being read; sep is the separator after it. An
// empty command keeps the separator in front of the next one, unless sep
// says more: `a &&` then a newline still joins the next line with &&.
func (r *shellReader) end(sep joint) {
	c := r.cur
	if len(c.words) == 0 && len(c.targets) == 0 && len(c.subshells) == 0 && len(c.comments) == 0 {
		if sep != jointList {
			r.next = sep
		}
		return
	}
	c.before, c.after = r.next, sep
	r.cmds = append(r.cmds, c)
	r.cur, r.next = shellCmd{}, sep
}

// bodies reads the heredoc bodies queued on the line that just ended and
// hands each to the command that opened it. Inside ( ) or $( ) an odd body
// fails the read.
func (r *shellReader) bodies() {
	queued := r.lex.heredocs
	for i, body := range r.lex.readHeredocs() {
		r.failed = r.failed || (r.closing && body.odd)
		owner := r.owners[i]
		if owner >= len(r.cmds) {
			continue // the command never ended, as at a closing ) mid-line
		}
		r.cmds[owner].docs = append(
			r.cmds[owner].docs,
			shellDoc{body: body.at, quoted: queued[i].quoted, odd: body.odd},
		)
	}
	r.owners = nil
}

// tokens returns the command's words as the git walk compares them (token).
func (c shellCmd) tokens() []string {
	out := make([]string, 0, len(c.words))
	for _, w := range c.words {
		out = append(out, w.token())
	}
	return out
}

// token returns the word the shell passes when that needs no expansion:
// "git", g\it and 'main' read as git and main. A word with an expansion, or
// whose text holds a character expandCdTarget reads as shell syntax (a
// quoted ~ or $), stays as written, so the directory parsers judge its
// quoting as before.
func (w placedWord) token() string {
	if v, ok := w.literal(); ok && v != "" && !strings.ContainsAny(v, unresolvableChars) {
		return v
	}
	return w.raw
}

// script returns the text a shell runs when it gets this word as a script,
// as far as the reader can tell: literal text as the shell passes it, a
// variable as a reference only the shell can expand, and any other
// expansion as $?, a value nothing can name.
func (w placedWord) script() string {
	var b strings.Builder
	for _, p := range w.parts {
		switch p.kind {
		case partLit:
			b.WriteString(p.text)
		case partVar:
			b.WriteString("${" + p.text + "}")
		default:
			b.WriteString("$?")
		}
	}
	return b.String()
}

// pieces returns the bodies of the substitutions and other expansions in
// the command's words and redirection targets that hold text the shell may
// run, in order: $(...), backticks, <(...), >(...), $((...)) and ${...}.
func (c shellCmd) pieces() []span {
	var out []span
	for _, words := range [][]placedWord{c.words, c.targets} {
		for _, w := range words {
			out = append(out, w.pieces()...)
		}
	}
	return out
}

// pieces returns the bodies of the expansions in one word (shellCmd.pieces).
func (w placedWord) pieces() []span {
	var out []span
	for _, p := range w.parts {
		if p.body.to > p.body.from {
			out = append(out, p.body)
		}
	}
	return out
}
