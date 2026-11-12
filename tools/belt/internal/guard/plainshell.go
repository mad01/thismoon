package guard

import "strings"

// shellWord is one word of a plain command with its quotes removed. quoted
// reports whether any part of it was quoted, and tilde whether it starts
// with an unquoted ~ that the shell expands.
type shellWord struct {
	text   string
	quoted bool
	tilde  bool
}

// plainSegments reads a command made of nothing but simple commands joined
// by ;, &&, || or newlines, and returns each command's words. Unlike
// splitSegments it is quote-aware, and it is strict: it reports false for
// any byte that could make the shell do more than run those commands with
// those words. That covers pipes, redirects, command and process
// substitution, backticks, subshells and groups, a background &, heredocs,
// comments, line continuations, variables, globs, brace expansion, an empty
// command, and a word that starts with = (zsh expands =cmd to a path).
//
// An unquoted ~ passes in two places only. At the start of a word it is
// marked, and the caller decides what that word may be. After a letter or
// digit (HEAD~3) the shell leaves it alone. Every other ~ fails the read:
// after = or : it is expanded in some shells. A word-leading ~ also fails
// the read when a quote comes before its first unquoted /: bash then leaves
// the word alone and zsh expands it (~'/x'), so the shells disagree on the
// path.
func plainSegments(command string) ([][]shellWord, bool) {
	var r plainReader
	s := strings.Trim(command, " \t\n")
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case ' ', '\t':
			r.endWord()
		case '\n', ';':
			if !r.endSegment() {
				return nil, false
			}
		case '&', '|':
			// Only && and || join commands: a lone & runs in the
			// background and a lone | is a pipe.
			if i+1 == len(s) || s[i+1] != c || !r.endSegment() {
				return nil, false
			}
			i++
		case '\'', '"':
			n, ok := r.quoted(s[i:])
			if !ok {
				return nil, false
			}
			i += n - 1
		default:
			if !r.unquoted(c) {
				return nil, false
			}
		}
	}
	if !r.endSegment() {
		return nil, false
	}
	return r.segs, true
}

// plainReader holds plainSegments' progress: the finished commands, the
// words of the current one, and the word being read.
type plainReader struct {
	segs  [][]shellWord
	words []shellWord
	cur   shellWord       // the word being read; its text lives in text until it ends
	text  strings.Builder // the current word's text so far
	open  bool            // a word has started, possibly with an empty quoted part
	last  byte            // the current word's last byte, 0 when it has none
	// tildePrefix is true from a word-leading unquoted ~ up to the first
	// unquoted /: the part of the word the shell reads as a tilde prefix.
	tildePrefix bool
}

func (r *plainReader) endWord() {
	if r.open {
		r.cur.text = r.text.String()
		r.words = append(r.words, r.cur)
	}
	r.cur, r.open, r.last, r.tildePrefix = shellWord{}, false, 0, false
	r.text.Reset()
}

// endSegment closes the current command. An empty one fails the read: a
// command line that starts or ends with an operator, or holds two in a
// row, is not the plain shape.
func (r *plainReader) endSegment() bool {
	r.endWord()
	if len(r.words) == 0 {
		return false
	}
	r.segs = append(r.segs, r.words)
	r.words = nil
	return true
}

// quoted reads the quoted string at the start of s into the current word
// and returns how many bytes it spans. Single quotes are literal. Inside
// double quotes $, `, \ and ! keep a meaning to the shell, so any of them
// fails the read, as does a quote that never closes, and a quote inside a
// tilde prefix (see plainSegments).
func (r *plainReader) quoted(s string) (int, bool) {
	if r.tildePrefix {
		return 0, false
	}
	q := s[0]
	end := strings.IndexByte(s[1:], q)
	if end < 0 {
		return 0, false
	}
	body := s[1 : 1+end]
	if q == '"' && strings.ContainsAny(body, "$`\\!") {
		return 0, false
	}
	r.text.WriteString(body)
	r.cur.quoted = true
	r.open = true
	if body != "" {
		r.last = body[len(body)-1]
	}
	return end + 2, true
}

// unquoted adds one unquoted byte to the current word, or reports false
// when the byte means something to the shell.
func (r *plainReader) unquoted(c byte) bool {
	switch {
	case c == '~':
		if r.open && !isAlnum(r.last) {
			return false
		}
		if !r.open {
			r.cur.tilde, r.tildePrefix = true, true
		}
	case c == '/':
		r.tildePrefix = false
	case c == '=':
		if !r.open {
			return false
		}
	case !isAlnum(c) && !strings.ContainsRune("-_.:,@+%", rune(c)):
		return false
	}
	r.text.WriteByte(c)
	r.open = true
	r.last = c
	return true
}

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
