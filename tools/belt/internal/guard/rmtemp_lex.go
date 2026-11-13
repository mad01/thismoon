package guard

import "strings"

// This file is the quote-aware shell reader behind the rm-temp guard. The
// token parser the git guards share (splitSegments) splits inside quoted
// strings, which is fine for finding a git push anywhere in a command and
// wrong for rm-temp: `git commit -m "fix; rm -rf /tmp/x"` must not read as
// an rm. So this reader follows quotes, escapes, `$(...)` and backtick
// substitutions, redirections, comments and heredoc bodies, and hands back
// the simple commands in order. It is still not a shell: what it cannot
// follow comes back as not ok, and the guard then says nothing.

// shWord is one shell word: the text as written, and its pieces.
type shWord struct {
	raw   string
	parts []wordPart
}

// partKind says what a piece of a word is.
type partKind int

const (
	partLit     partKind = iota // literal text, quotes removed
	partVar                     // $NAME or ${NAME}: text is the name
	partSubst                   // $(...) or `...`: text is the inner command
	partUnknown                 // any other expansion ($1, ${x:-y}, $((...)), $'...', <(...))
)

// wordPart is one piece of a word. quoted marks literal text that was
// quoted or escaped, where glob and tilde characters mean themselves. body
// is where the text of a $(...), backtick, <(...), >(...), $((...)) or
// ${...} piece lies in the source, so a reader can treat it as commands.
type wordPart struct {
	kind   partKind
	text   string
	quoted bool
	body   span
}

// literal returns the word's value when it holds no expansion at all.
func (w shWord) literal() (string, bool) {
	var b strings.Builder
	for _, p := range w.parts {
		if p.kind != partLit {
			return "", false
		}
		b.WriteString(p.text)
	}
	return b.String(), true
}

// heredoc is a here-document whose body starts at the next newline. quoted
// marks a delimiter with a quote or backslash in it, so the shell hands the
// body over without expanding it.
type heredoc struct {
	delim     string
	stripTabs bool
	quoted    bool
}

// shLexer reads a command string into simple commands. target and targetAt
// hold the last redirection target read, for readers built on this one.
type shLexer struct {
	src      string
	i        int
	depth    int // how many ( and $( the cursor sits inside (closeParen)
	cmds     [][]shWord
	cur      []shWord
	heredocs []heredoc
	target   shWord
	targetAt span
}

// lexCommands splits a shell command into its simple commands, in order,
// each as its words with redirections left out. A command ends at `;`,
// `&`, `|`, `(`, `)` and an unquoted newline, so `&&`, `||` and pipelines
// split too. Heredoc bodies and comments are skipped. ok is false for input
// it cannot follow: an unterminated quote or substitution, or a redirection
// without a target.
func lexCommands(src string) ([][]shWord, bool) {
	l := &shLexer{src: src}
	for l.i < len(l.src) {
		if !l.step() {
			return nil, false
		}
	}
	l.endCommand()
	return l.cmds, true
}

// step consumes one token: blanks, a separator, a comment, a redirection
// or a word.
func (l *shLexer) step() bool {
	c := l.src[l.i]
	switch {
	case c == ' ' || c == '\t':
		l.i++
	case c == '\\' && l.at(1) == '\n':
		l.i += 2
	case c == '\n':
		l.i++
		l.endCommand()
		l.skipHeredocs()
	case c == '#':
		l.skipComment()
	case c == ';' || c == '|' || c == '(' || c == ')':
		l.i++
		l.endCommand()
	case c == '&' && l.at(1) == '>':
		l.i += 2
		if l.at(0) == '>' {
			l.i++
		}
		return l.redirectTarget(false, false)
	case c == '&':
		l.i++
		l.endCommand()
	case (c == '<' || c == '>') && l.at(1) != '(':
		return l.redirect()
	default:
		w, ok := l.readWord()
		if !ok {
			return false
		}
		// `2>/dev/null`: digits glued to a redirection are its fd, not a word.
		if isDigits(w.raw) && (l.at(0) == '<' || l.at(0) == '>') {
			return true
		}
		l.cur = append(l.cur, w)
	}
	return true
}

// at returns the byte off positions past the cursor, or 0 past the end.
func (l *shLexer) at(off int) byte {
	if l.i+off < len(l.src) {
		return l.src[l.i+off]
	}
	return 0
}

func (l *shLexer) endCommand() {
	if len(l.cur) > 0 {
		l.cmds = append(l.cmds, l.cur)
	}
	l.cur = nil
}

func (l *shLexer) skipComment() {
	if j := strings.IndexByte(l.src[l.i:], '\n'); j >= 0 {
		l.i += j
		return
	}
	l.i = len(l.src)
}

// redirect consumes a redirection operator and its target word.
func (l *shLexer) redirect() bool {
	rest := l.src[l.i:]
	for _, op := range []string{"<<<", "<<-", "<<", "<>", "<&", "<", ">>", ">&", ">|", ">"} {
		if strings.HasPrefix(rest, op) {
			l.i += len(op)
			return l.redirectTarget(op == "<<" || op == "<<-", op == "<<-")
		}
	}
	return false
}

// redirectTarget reads the word a redirection names. For a heredoc that
// word is the delimiter, and the body is queued for the next newline.
func (l *shLexer) redirectTarget(isHeredoc, stripTabs bool) bool {
	for l.at(0) == ' ' || l.at(0) == '\t' {
		l.i++
	}
	if l.i >= len(l.src) || strings.IndexByte("\n;&|()<>", l.src[l.i]) >= 0 {
		return false
	}
	start := l.i
	w, ok := l.readWord()
	if !ok {
		return false
	}
	l.target, l.targetAt = w, span{start, l.i}
	if isHeredoc {
		delim, lit := w.literal()
		if !lit {
			delim = w.raw
		}
		l.heredocs = append(l.heredocs, heredoc{
			delim: delim, stripTabs: stripTabs, quoted: strings.ContainsAny(w.raw, `'"\`),
		})
	}
	return true
}

// skipHeredocs moves past the bodies of the heredocs queued on the line
// that just ended.
func (l *shLexer) skipHeredocs() { l.readHeredocs() }

// docBody is where one heredoc body lies, its terminator line left out.
// odd marks a body with a line that starts with the delimiter without being
// it: inside $( ) bash ends the body at such a line (`EOF)`) and reads the
// rest of the line as code, while zsh reads on to the exact line.
type docBody struct {
	at  span
	odd bool
}

// readHeredocs moves past the bodies of the heredocs queued on the line
// that just ended and returns each body. A body with no terminator line runs
// to the end, as in the shell.
func (l *shLexer) readHeredocs() []docBody {
	bodies := make([]docBody, 0, len(l.heredocs))
	for _, h := range l.heredocs {
		body := docBody{at: span{l.i, l.i}}
		for l.i < len(l.src) {
			line, next := bodyLine(l.src, l.i, !h.quoted)
			if h.stripTabs {
				line = strings.TrimLeft(line, "\t")
			}
			l.i = next
			if line == h.delim {
				break
			}
			body.odd = body.odd || strings.HasPrefix(line, h.delim)
			body.at.to = next
		}
		bodies = append(bodies, body)
	}
	l.heredocs = nil
	return bodies
}

// bodyLine returns the heredoc body line that starts at i, its newline left
// out, and where the next line starts. In an unquoted body (join) a line
// that ends in an odd run of backslashes joins the next one, the backslash
// and newline dropped, as bash and zsh read it: the joined line is what they
// compare with the delimiter, so `x\` then `EOF` ends nothing.
func bodyLine(src string, i int, join bool) (string, int) {
	joined := ""
	for {
		line, next := src[i:], len(src)
		j := strings.IndexByte(line, '\n')
		if j >= 0 {
			line, next = line[:j], i+j+1
		}
		trailing := len(line) - len(strings.TrimRight(line, `\`))
		if !join || j < 0 || trailing%2 == 0 {
			return joined + line, next
		}
		joined += line[:len(line)-1]
		i = next
	}
}

// wordEnds are the unquoted bytes that end a word.
const wordEnds = " \t\n;&|()<>"

// readWord reads one word from the cursor.
func (l *shLexer) readWord() (shWord, bool) {
	start := l.i
	var parts []wordPart
	for l.i < len(l.src) {
		c := l.src[l.i]
		var ok bool
		switch {
		case (c == '<' || c == '>') && l.at(1) == '(':
			ok = l.skipParens(l.i+1, &parts) // process substitution
		case strings.IndexByte(wordEnds, c) >= 0:
			if c == '(' && l.i > start {
				// A paren glued to a word is a zsh glob qualifier or a
				// function definition: either way not a plain path.
				parts = append(parts, wordPart{kind: partUnknown})
			}
			return shWord{raw: l.src[start:l.i], parts: parts}, true
		case c == '\'':
			ok = l.readSingle(&parts)
		case c == '"':
			ok = l.readDouble(&parts)
		case c == '\\':
			l.readEscape(&parts, false)
			ok = true
		case c == '$':
			ok = l.readDollar(&parts, false)
		case c == '`':
			ok = l.readBacktick(&parts, false)
		default:
			addLit(&parts, l.src[l.i:l.i+1], false)
			l.i++
			ok = true
		}
		if !ok {
			return shWord{}, false
		}
	}
	return shWord{raw: l.src[start:l.i], parts: parts}, true
}

// addLit appends literal text, merging with a literal piece of the same
// quoting before it.
func addLit(parts *[]wordPart, text string, quoted bool) {
	if n := len(*parts); n > 0 {
		last := &(*parts)[n-1]
		if last.kind == partLit && last.quoted == quoted {
			last.text += text
			return
		}
	}
	*parts = append(*parts, wordPart{kind: partLit, text: text, quoted: quoted})
}

func (l *shLexer) readSingle(parts *[]wordPart) bool {
	end := strings.IndexByte(l.src[l.i+1:], '\'')
	if end < 0 {
		return false
	}
	addLit(parts, l.src[l.i+1:l.i+1+end], true)
	l.i += end + 2
	return true
}

// readEscape reads a backslash and the byte it escapes; a backslash before
// a newline joins the lines. Inside double quotes only $ ` " \ escape.
func (l *shLexer) readEscape(parts *[]wordPart, inDouble bool) {
	next := l.at(1)
	switch {
	case next == '\n':
		l.i += 2
	case l.i+1 >= len(l.src):
		addLit(parts, `\`, true)
		l.i++
	case inDouble && strings.IndexByte("$`\"\\", next) < 0:
		addLit(parts, `\`, true)
		l.i++
	default:
		addLit(parts, string(next), true)
		l.i += 2
	}
}

func (l *shLexer) readDouble(parts *[]wordPart) bool {
	l.i++
	for l.i < len(l.src) {
		ok := true
		switch c := l.src[l.i]; c {
		case '"':
			l.i++
			return true
		case '\\':
			l.readEscape(parts, true)
		case '$':
			ok = l.readDollar(parts, true)
		case '`':
			ok = l.readBacktick(parts, true)
		default:
			addLit(parts, string(c), true)
			l.i++
		}
		if !ok {
			return false
		}
	}
	return false
}

// readDollar reads an expansion that starts with $. A $ that starts none
// is a literal dollar sign.
func (l *shLexer) readDollar(parts *[]wordPart, quoted bool) bool {
	switch next := l.at(1); {
	case next == '(':
		return l.readDollarParen(parts, quoted)
	case next == '{':
		return l.readDollarBrace(parts, quoted)
	case next == '\'' && !quoted:
		return l.readANSIC(parts)
	case isNameStart(next):
		j := l.i + 1
		for j < len(l.src) && isNameByte(l.src[j]) {
			j++
		}
		*parts = append(*parts, wordPart{kind: partVar, text: l.src[l.i+1 : j], quoted: quoted})
		l.i = j
	case next != 0 && strings.IndexByte("@*#?$!-0123456789", next) >= 0:
		*parts = append(*parts, wordPart{kind: partUnknown})
		l.i += 2
	default:
		addLit(parts, "$", quoted)
		l.i++
	}
	return true
}

// readDollarParen reads a $(...) substitution or a $((...)) arithmetic
// expansion.
func (l *shLexer) readDollarParen(parts *[]wordPart, quoted bool) bool {
	end, ok := l.matchParen(l.i + 1)
	if !ok {
		return false
	}
	at := span{l.i + 2, end}
	body := l.src[at.from:at.to]
	if strings.HasPrefix(body, "(") {
		*parts = append(*parts, wordPart{kind: partUnknown, body: at}) // $((arithmetic))
	} else {
		*parts = append(*parts, wordPart{kind: partSubst, text: body, quoted: quoted, body: at})
	}
	l.i = end + 1
	return true
}

// readDollarBrace reads ${NAME} as a variable and any other ${...} as an
// expansion only the shell can resolve.
func (l *shLexer) readDollarBrace(parts *[]wordPart, quoted bool) bool {
	end := strings.IndexByte(l.src[l.i+2:], '}')
	if end < 0 {
		return false
	}
	name := l.src[l.i+2 : l.i+2+end]
	if isShellName(name) {
		*parts = append(*parts, wordPart{kind: partVar, text: name, quoted: quoted})
	} else {
		*parts = append(*parts, wordPart{kind: partUnknown, body: span{l.i + 2, l.i + 2 + end}})
	}
	l.i += end + 3
	return true
}

// readANSIC reads a $'...' string, whose backslash escapes only the shell
// decodes.
func (l *shLexer) readANSIC(parts *[]wordPart) bool {
	j := l.i + 2
	for j < len(l.src) && l.src[j] != '\'' {
		if l.src[j] == '\\' {
			j++
		}
		j++
	}
	if j >= len(l.src) {
		return false
	}
	*parts = append(*parts, wordPart{kind: partUnknown}) // $'ANSI-C'
	l.i = j + 1
	return true
}

func (l *shLexer) readBacktick(parts *[]wordPart, quoted bool) bool {
	j := l.i + 1
	for j < len(l.src) && l.src[j] != '`' {
		if l.src[j] == '\\' {
			j++
		}
		j++
	}
	if j >= len(l.src) {
		return false
	}
	at := span{l.i + 1, j}
	*parts = append(
		*parts,
		wordPart{kind: partSubst, text: l.src[at.from:at.to], quoted: quoted, body: at},
	)
	l.i = j + 1
	return true
}

// skipParens consumes a parenthesised construct that starts at open and
// records it as an unknown piece.
func (l *shLexer) skipParens(open int, parts *[]wordPart) bool {
	end, ok := l.matchParen(open)
	if !ok {
		return false
	}
	*parts = append(*parts, wordPart{kind: partUnknown, body: span{open + 1, end}})
	l.i = end + 1
	return true
}

// matchParen returns the index of the parenthesis that closes the one at
// s[open]. The text between is read as commands (closeParen), so a quote,
// an escape, a comment or a heredoc body inside cannot close it early or
// hide the close: `$(cat <<'EOF'` with a body line "don't" closes where the
// shell closes it.
func matchParen(s string, open int) (int, bool) {
	return closeParen(s, open+1, 1)
}

// matchParen is matchParen for a parenthesis the lexer met, one level deeper
// than the cursor.
func (l *shLexer) matchParen(open int) (int, bool) {
	return closeParen(l.src, open+1, l.depth+1)
}

func isNameStart(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
