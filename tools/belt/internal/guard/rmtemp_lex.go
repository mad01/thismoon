package guard

import (
	"strconv"
	"strings"
)

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
	partUnknown                 // any other expansion ($1, ${x:-y}, $((...)), <(...))
	partANSIC                   // $'...': text is its value, the escapes decoded
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
	return w.value(false)
}

// literalANSIC is literal with a $'...' piece read as its value: the word
// the shell passes when it holds no other expansion. The deny guards read
// words this way (ansiCWord).
func (w shWord) literalANSIC() (string, bool) {
	return w.value(true)
}

// value joins the word's literal pieces, and its $'...' pieces when ansiC
// is set. ok is false when the word holds any other piece.
func (w shWord) value(ansiC bool) (string, bool) {
	var b strings.Builder
	for _, p := range w.parts {
		if p.kind != partLit && (!ansiC || p.kind != partANSIC) {
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
// hold the last redirection target read, and op its operator, for readers
// built on this one.
type shLexer struct {
	src      string
	i        int
	depth    int // how many ( and $( the cursor sits inside (closeParen)
	cmds     [][]shWord
	cur      []shWord
	heredocs []heredoc
	target   shWord
	targetAt span
	op       string
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
			l.op = op
			return l.redirectTarget(op == "<<" || op == "<<-", op == "<<-")
		}
	}
	return false
}

// redirectTarget reads the word a redirection names. For a heredoc that
// word is the delimiter, and the body is queued for the next newline. A
// process substitution is a word too (`sh < <(cmd)`).
func (l *shLexer) redirectTarget(isHeredoc, stripTabs bool) bool {
	for l.at(0) == ' ' || l.at(0) == '\t' {
		l.i++
	}
	procSubst := (l.at(0) == '<' || l.at(0) == '>') && l.at(1) == '('
	if l.i >= len(l.src) || (!procSubst && strings.IndexByte("\n;&|()<>", l.src[l.i]) >= 0) {
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

// joinedBody returns an unquoted heredoc body as the shell expands it: each
// line joined to the next wherever bodyLine joins them. bash and zsh drop
// those backslash-newlines as they read the body, before they look for a
// substitution, so `$\` at a line end and `(cmd)` on the next line is
// `$(cmd)`.
func joinedBody(body string) string {
	if !strings.Contains(body, "\\\n") {
		return body
	}
	var b strings.Builder
	for i := 0; i < len(body); {
		line, next := bodyLine(body, i, true)
		b.WriteString(line)
		if body[next-1] == '\n' {
			b.WriteByte('\n')
		}
		i = next
	}
	return b.String()
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
// is a literal dollar sign. bash drops a backslash-newline between the $
// and the byte after it, in an unquoted word and inside double quotes alike
// (joinedAt), so `"$\` at a line end and `(cmd)"` on the next line is a
// substitution. zsh reads that as text, and belt reads it as bash does.
func (l *shLexer) readDollar(parts *[]wordPart, quoted bool) bool {
	k := joinedAt(l.src, l.i+1)
	var next byte
	if k < len(l.src) {
		next = l.src[k]
	}
	switch {
	case next == '(':
		return l.readDollarParen(parts, quoted, k)
	case next == '{':
		return l.readDollarBrace(parts, quoted, k)
	case next == '\'' && !quoted:
		return l.readANSIC(parts, k)
	case isNameStart(next):
		j := k
		for j < len(l.src) && isNameByte(l.src[j]) {
			j++
		}
		*parts = append(*parts, wordPart{kind: partVar, text: l.src[k:j], quoted: quoted})
		l.i = j
	case next != 0 && strings.IndexByte("@*#?$!-0123456789", next) >= 0:
		*parts = append(*parts, wordPart{kind: partUnknown})
		l.i = k + 1
	default:
		addLit(parts, "$", quoted)
		l.i++
	}
	return true
}

// joinedAt returns the index of the first byte at or after i that does not
// start a backslash-newline, the line join the shell drops.
func joinedAt(s string, i int) int {
	for i+1 < len(s) && s[i] == '\\' && s[i+1] == '\n' {
		i += 2
	}
	return i
}

// readDollarParen reads a $(...) substitution or a $((...)) arithmetic
// expansion whose first ( is at open.
func (l *shLexer) readDollarParen(parts *[]wordPart, quoted bool, open int) bool {
	end, ok := l.matchParen(open)
	if !ok {
		return false
	}
	at := span{open + 1, end}
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
// expansion only the shell can resolve. Its { is at open.
func (l *shLexer) readDollarBrace(parts *[]wordPart, quoted bool, open int) bool {
	end := strings.IndexByte(l.src[open+1:], '}')
	if end < 0 {
		return false
	}
	at := span{open + 1, open + 1 + end}
	if name := l.src[at.from:at.to]; isShellName(name) {
		*parts = append(*parts, wordPart{kind: partVar, text: name, quoted: quoted})
	} else {
		*parts = append(*parts, wordPart{kind: partUnknown, body: at})
	}
	l.i = at.to + 1
	return true
}

// readANSIC reads a $'...' string whose opening quote is at quote. Its
// piece holds the value with the backslash escapes decoded (ansiCValue),
// which only the deny guards read as literal text (literalANSIC); to every
// other reader it is an expansion.
func (l *shLexer) readANSIC(parts *[]wordPart, quote int) bool {
	j := ansiCEnd(l.src, quote+1)
	if j < 0 {
		return false
	}
	*parts = append(*parts, wordPart{kind: partANSIC, text: ansiCValue(l.src[quote+1 : j])})
	l.i = j + 1
	return true
}

// ansiCEnd returns where the quote that closes a $'...' string lies in src,
// its body starting at from, past the backslash escapes in it. It is -1
// when no quote closes the string.
func ansiCEnd(src string, from int) int {
	j := from
	for j < len(src) && src[j] != '\'' {
		if src[j] == '\\' {
			j++
		}
		j++
	}
	if j >= len(src) {
		return -1
	}
	return j
}

// ansiCEscapes are the one-letter escapes of a $'...' string.
var ansiCEscapes = map[byte]byte{
	'a': '\a', 'b': '\b', 'e': 0x1b, 'E': 0x1b, 'f': '\f', 'n': '\n', 'r': '\r',
	't': '\t', 'v': '\v', '\\': '\\', '\'': '\'', '"': '"', '?': '?',
}

// ansiCValue decodes the body of a $'...' string as bash does: the
// one-letter escapes, octal \nnn, \xHH, \uHHHH, \UHHHHHHHH and \cX for a
// control character. An unknown escape keeps its backslash, and a NUL ends
// the value, as it ends an argument the shell passes.
func ansiCValue(body string) string {
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' || i+1 == len(body) {
			b.WriteByte(body[i])
			continue
		}
		i++
		esc := body[i]
		if c, ok := ansiCEscapes[esc]; ok {
			b.WriteByte(c)
			continue
		}
		v, wide, used := ansiCNumber(body[i:])
		switch {
		case used > 0 && wide:
			b.WriteRune(rune(v))
			i += used - 1
		case used > 0:
			b.WriteByte(byte(v))
			i += used - 1
		case esc == 'c' && i+1 < len(body):
			b.WriteByte(body[i+1] & 0x1f)
			i++
		default:
			b.WriteByte('\\')
			b.WriteByte(esc)
		}
	}
	value, _, _ := strings.Cut(b.String(), "\x00")
	return value
}

// ansiCNumber reads a numeric escape from just after its backslash: up to
// three octal digits, or x, u or U and up to two, four or eight hex digits.
// wide marks a \u or \U character, written as UTF-8; the others are one
// byte. used is how many bytes the escape takes, 0 when s starts none.
func ansiCNumber(s string) (v uint64, wide bool, used int) {
	base, set, digits, start := 8, "01234567", 3, 0
	const hex = "0123456789abcdefABCDEF"
	switch s[0] {
	case 'x':
		base, set, digits, start = 16, hex, 2, 1
	case 'u':
		base, set, digits, start, wide = 16, hex, 4, 1, true
	case 'U':
		base, set, digits, start, wide = 16, hex, 8, 1, true
	}
	end := start
	for end < len(s) && end-start < digits && strings.IndexByte(set, s[end]) >= 0 {
		end++
	}
	if end == start {
		return 0, false, 0
	}
	v, _ = strconv.ParseUint(s[start:end], base, 32)
	return v, wide, end
}

func (l *shLexer) readBacktick(parts *[]wordPart, quoted bool) bool {
	j := backtickEnd(l.src, l.i+1)
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

// backtickEnd returns the index of the backtick that closes a backtick
// substitution whose body starts at from, a backslash and the byte after it
// skipped, or len(s) when none does.
func backtickEnd(s string, from int) int {
	for j := from; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '`':
			return j
		}
	}
	return len(s)
}

// backtickCode returns the command a backtick substitution runs: its body
// with one level of escapes taken off, since inside backticks a backslash
// before $, ` or \ quotes that byte and the shell drops it before it reads
// the command. Inside double quotes (quoted) the same goes for a backslash
// before ": bash and zsh run "`echo \"x\"`" as echo "x". So the body
// echo \`cmd\` runs echo with a nested cmd substitution, and each level of
// nesting takes one more level off. A backslash before any other byte
// stays, as does \" in an unquoted word or heredoc body.
func backtickCode(body string, quoted bool) string {
	if !strings.Contains(body, `\`) {
		return body
	}
	escapes := "$`\\"
	if quoted {
		escapes += `"`
	}
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) && strings.IndexByte(escapes, body[i+1]) >= 0 {
			i++
		}
		b.WriteByte(body[i])
	}
	return b.String()
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
