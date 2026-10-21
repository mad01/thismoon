package pyjson

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
)

// SyntaxError is a JSON syntax error, worded and positioned the way
// Python's json.JSONDecodeError is: Pos counts characters (code points), not
// bytes, and Line and Col start at 1.
type SyntaxError struct {
	Msg  string
	Pos  int
	Line int
	Col  int
}

// Error formats the error the way str(json.JSONDecodeError) does.
func (e *SyntaxError) Error() string {
	return fmt.Sprintf("%s: line %d column %d (char %d)", e.Msg, e.Line, e.Col, e.Pos)
}

// EncodingError reports input that is not valid UTF-8. Python fails on it
// while reading the file, before json sees it, and the sync scripts let that
// failure end the run instead of treating it as a parse error.
type EncodingError struct {
	// Byte is the first byte that does not decode, at byte offset Pos.
	Byte byte
	Pos  int
	// Len is how many bytes the error covers: CPython reports a broken or
	// cut-short sequence as a range.
	Len int
	// Reason is CPython's reason: reasonStart, reasonContinuation or
	// reasonEnd.
	Reason string
}

// The reasons CPython's UTF-8 decoder gives.
const (
	reasonStart        = "invalid start byte"
	reasonContinuation = "invalid continuation byte"
	reasonEnd          = "unexpected end of data"
)

// Error formats the error the way str() of Python's UnicodeDecodeError
// does: one byte by value, a longer run as a position range.
func (e *EncodingError) Error() string {
	if e.Len > 1 {
		return fmt.Sprintf("'utf-8' codec can't decode bytes in position %d-%d: %s",
			e.Pos, e.Pos+e.Len-1, e.Reason)
	}
	return fmt.Sprintf("'utf-8' codec can't decode byte 0x%02x in position %d: %s",
		e.Byte, e.Pos, e.Reason)
}

// ReadFile reads and parses the JSON file at path the way Python's
// json.loads(Path(path).read_text()) does, newline translation included. A
// read failure comes back as the *fs.PathError os.ReadFile returns, invalid
// UTF-8 as an *EncodingError, and bad JSON as a *SyntaxError.
func ReadFile(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// read_text decodes the file's bytes before it translates newlines, so
	// an encoding error counts positions in the bytes as they are on disk.
	if err := checkUTF8(data); err != nil {
		return nil, err
	}
	// Text mode turns \r\n and a lone \r into \n. Outside a string either
	// one is whitespace and inside one it is an error, so this only moves
	// the positions syntax errors report.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	return Loads(data)
}

// Loads parses data as json.loads parses a str: whitespace around the value,
// NaN and Infinity accepted, a duplicate key keeping its first position and
// its last value, and Python's error messages on failure.
func Loads(data []byte) (any, error) {
	if err := checkUTF8(data); err != nil {
		return nil, err
	}
	d := &decoder{s: []rune(string(data))}
	if len(d.s) > 0 && d.s[0] == 0xfeff {
		return nil, d.errorAt("Unexpected UTF-8 BOM (decode using utf-8-sig)", 0)
	}
	v, end, err := d.scanOnce(d.skipSpace(0))
	if err != nil {
		return nil, err
	}
	if end = d.skipSpace(end); end != len(d.s) {
		return nil, d.errorAt("Extra data", end)
	}
	return v, nil
}

// checkUTF8 returns the *EncodingError CPython's UTF-8 decoder raises for
// the first sequence in data that does not decode, or nil when data is
// valid UTF-8.
func checkUTF8(data []byte) error {
	for i := 0; i < len(data); {
		n, reason := utf8Sequence(data[i:])
		if reason != "" {
			return &EncodingError{Byte: data[i], Pos: i, Len: n, Reason: reason}
		}
		i += n
	}
	return nil
}

// utf8Sequence classifies the UTF-8 sequence s starts with the way CPython's
// decoder does (Objects/stringlib/codecs.h): its length and "" when it is
// valid, otherwise how many bytes the error covers and the reason. s is not
// empty.
func utf8Sequence(s []byte) (int, string) {
	switch c := s[0]; {
	case c < 0x80:
		return 1, ""
	case c < 0xc2:
		// A continuation byte, or the lead of an overlong 2-byte form.
		return 1, reasonStart
	case c < 0xe0:
		return continuation(s, 2, 0x80, 0xbf)
	case c == 0xe0:
		return continuation(s, 3, 0xa0, 0xbf) // no overlong forms
	case c == 0xed:
		return continuation(s, 3, 0x80, 0x9f) // no surrogates
	case c < 0xf0:
		return continuation(s, 3, 0x80, 0xbf)
	case c == 0xf0:
		return continuation(s, 4, 0x90, 0xbf) // no overlong forms
	case c < 0xf4:
		return continuation(s, 4, 0x80, 0xbf)
	case c == 0xf4:
		return continuation(s, 4, 0x80, 0x8f) // nothing past U+10FFFF
	default:
		return 1, reasonStart
	}
}

// continuation checks the n-1 continuation bytes after s[0], the first of
// which must lie in [lo, hi]. CPython reports a bad one as an error covering
// the bytes before it, and input that ends inside the sequence as an error
// covering the rest of it.
func continuation(s []byte, n int, lo, hi byte) (int, string) {
	for i := 1; i < n; i++ {
		if i == len(s) {
			return i, reasonEnd
		}
		b := s[i]
		if b < 0x80 || b > 0xbf || (i == 1 && (b < lo || b > hi)) {
			return i, reasonContinuation
		}
	}
	return n, ""
}

// decoder ports CPython's C scanner (Modules/_json.c) over the input's code
// points, so indexes, accepted inputs and error positions match Python's.
type decoder struct {
	s []rune
}

func (d *decoder) errorAt(msg string, pos int) error {
	line := 1
	lastNewline := -1
	for i := 0; i < pos && i < len(d.s); i++ {
		if d.s[i] == '\n' {
			line++
			lastNewline = i
		}
	}
	return &SyntaxError{Msg: msg, Pos: pos, Line: line, Col: pos - lastNewline}
}

func (d *decoder) skipSpace(idx int) int {
	for idx < len(d.s) && isSpace(d.s[idx]) {
		idx++
	}
	return idx
}

func isSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// hasWord reports whether the literal word starts at idx.
func (d *decoder) hasWord(idx int, word string) bool {
	w := []rune(word)
	if idx+len(w)-1 >= len(d.s) {
		return false
	}
	for i, r := range w {
		if d.s[idx+i] != r {
			return false
		}
	}
	return true
}

// scanOnce parses one value starting at idx and returns it with the index
// just past it.
func (d *decoder) scanOnce(idx int) (any, int, error) {
	if idx >= len(d.s) {
		return nil, 0, d.errorAt("Expecting value", idx)
	}
	switch d.s[idx] {
	case '"':
		return d.scanString(idx + 1)
	case '{':
		return d.parseObject(idx + 1)
	case '[':
		return d.parseArray(idx + 1)
	case 'n':
		if d.hasWord(idx, "null") {
			return nil, idx + 4, nil
		}
	case 't':
		if d.hasWord(idx, "true") {
			return true, idx + 4, nil
		}
	case 'f':
		if d.hasWord(idx, "false") {
			return false, idx + 5, nil
		}
	case 'N':
		if d.hasWord(idx, "NaN") {
			return math.NaN(), idx + 3, nil
		}
	case 'I':
		if d.hasWord(idx, "Infinity") {
			return math.Inf(1), idx + 8, nil
		}
	case '-':
		if d.hasWord(idx, "-Infinity") {
			return math.Inf(-1), idx + 9, nil
		}
	}
	return d.matchNumber(idx)
}

func (d *decoder) parseObject(idx int) (any, int, error) {
	obj := NewObject()
	last := len(d.s) - 1
	idx = d.skipSpace(idx)
	if idx > last || d.s[idx] != '}' {
		for {
			if idx > last || d.s[idx] != '"' {
				return nil, 0, d.errorAt(
					"Expecting property name enclosed in double quotes", idx,
				)
			}
			key, next, err := d.scanString(idx + 1)
			if err != nil {
				return nil, 0, err
			}
			idx = d.skipSpace(next)
			if idx > last || d.s[idx] != ':' {
				return nil, 0, d.errorAt("Expecting ':' delimiter", idx)
			}
			val, next, err := d.scanOnce(d.skipSpace(idx + 1))
			if err != nil {
				return nil, 0, err
			}
			obj.Set(key.(string), val)
			idx = d.skipSpace(next)
			if idx <= last && d.s[idx] == '}' {
				break
			}
			if idx > last || d.s[idx] != ',' {
				return nil, 0, d.errorAt("Expecting ',' delimiter", idx)
			}
			comma := idx
			idx = d.skipSpace(idx + 1)
			if idx <= last && d.s[idx] == '}' {
				return nil, 0, d.errorAt("Illegal trailing comma before end of object", comma)
			}
		}
	}
	return obj, idx + 1, nil
}

func (d *decoder) parseArray(idx int) (any, int, error) {
	list := []any{}
	last := len(d.s) - 1
	idx = d.skipSpace(idx)
	if idx > last || d.s[idx] != ']' {
		for {
			val, next, err := d.scanOnce(idx)
			if err != nil {
				return nil, 0, err
			}
			list = append(list, val)
			idx = d.skipSpace(next)
			if idx <= last && d.s[idx] == ']' {
				break
			}
			if idx > last || d.s[idx] != ',' {
				return nil, 0, d.errorAt("Expecting ',' delimiter", idx)
			}
			comma := idx
			idx = d.skipSpace(idx + 1)
			if idx <= last && d.s[idx] == ']' {
				return nil, 0, d.errorAt("Illegal trailing comma before end of array", comma)
			}
		}
	}
	return list, idx + 1, nil
}

// matchNumber parses a number at start: an integer becomes a *big.Int, a
// number with a fraction or an exponent a float64. An exponent marker with no
// digits after it is left unconsumed, as the C scanner leaves it.
func (d *decoder) matchNumber(start int) (any, int, error) {
	s, last, idx := d.s, len(d.s)-1, start
	if s[idx] == '-' {
		idx++
		if idx > last {
			return nil, 0, d.errorAt("Expecting value", start)
		}
	}
	switch {
	case s[idx] >= '1' && s[idx] <= '9':
		idx++
		for idx <= last && isDigit(s[idx]) {
			idx++
		}
	case s[idx] == '0':
		idx++
	default:
		return nil, 0, d.errorAt("Expecting value", start)
	}
	isFloat := false
	if idx < last && s[idx] == '.' && isDigit(s[idx+1]) {
		isFloat = true
		idx += 2
		for idx <= last && isDigit(s[idx]) {
			idx++
		}
	}
	if idx < last && (s[idx] == 'e' || s[idx] == 'E') {
		eStart := idx
		idx++
		if idx < last && (s[idx] == '-' || s[idx] == '+') {
			idx++
		}
		for idx <= last && isDigit(s[idx]) {
			idx++
		}
		if isDigit(s[idx-1]) {
			isFloat = true
		} else {
			idx = eStart
		}
	}
	text := string(s[start:idx])
	if isFloat {
		// Out-of-range literals become +Inf, -Inf or 0, as Python's float() makes
		// them; the error only says which happened.
		f, _ := strconv.ParseFloat(text, 64)
		return f, idx, nil
	}
	n, ok := new(big.Int).SetString(text, 10)
	if !ok {
		return nil, 0, d.errorAt("Expecting value", start)
	}
	return n, idx, nil
}

// scanString parses a string body starting just after its opening quote and
// returns the index just past the closing quote.
func (d *decoder) scanString(end int) (any, int, error) {
	s, n := d.s, len(d.s)
	begin := end - 1
	var b strings.Builder
	for {
		next := end
		var c rune
		for ; next < n; next++ {
			c = s[next]
			if c == '"' || c == '\\' {
				break
			}
			if c <= 0x1f {
				return nil, 0, d.errorAt("Invalid control character at", next)
			}
		}
		if c != '"' && c != '\\' {
			return nil, 0, d.errorAt("Unterminated string starting at", begin)
		}
		b.WriteString(string(s[end:next]))
		next++
		if c == '"' {
			return b.String(), next, nil
		}
		if next == n {
			return nil, 0, d.errorAt("Unterminated string starting at", begin)
		}
		var err error
		if c, end, err = d.scanEscape(next); err != nil {
			return nil, 0, err
		}
		writeWTF8(&b, c)
	}
}

// scanEscape decodes the escape whose letter sits at next (just past the
// backslash) and returns the code point with the index after the escape. A
// \u high surrogate directly followed by a \u low surrogate joins into one
// code point; any other surrogate stays lone.
func (d *decoder) scanEscape(next int) (rune, int, error) {
	s, n := d.s, len(d.s)
	if c := s[next]; c != 'u' {
		end := next + 1
		switch c {
		case '"', '\\', '/':
			return c, end, nil
		case 'b':
			return '\b', end, nil
		case 'f':
			return '\f', end, nil
		case 'n':
			return '\n', end, nil
		case 'r':
			return '\r', end, nil
		case 't':
			return '\t', end, nil
		default:
			return 0, 0, d.errorAt(`Invalid \escape`, end-2)
		}
	}
	next++
	end := next + 4
	if end >= n {
		return 0, 0, d.errorAt(`Invalid \uXXXX escape`, next-1)
	}
	c, ok := hex4(s[next:end])
	if !ok {
		return 0, 0, d.errorAt(`Invalid \uXXXX escape`, end-5)
	}
	if c >= 0xd800 && c <= 0xdbff && end+6 < n && s[end] == '\\' && s[end+1] == 'u' {
		c2, ok := hex4(s[end+2 : end+6])
		if !ok {
			return 0, 0, d.errorAt(`Invalid \uXXXX escape`, end+1)
		}
		if c2 >= 0xdc00 && c2 <= 0xdfff {
			return 0x10000 + (c-0xd800)<<10 + (c2 - 0xdc00), end + 6, nil
		}
	}
	return c, end, nil
}

func hex4(digits []rune) (rune, bool) {
	var c rune
	for _, r := range digits {
		c <<= 4
		switch {
		case r >= '0' && r <= '9':
			c |= r - '0'
		case r >= 'a' && r <= 'f':
			c |= r - 'a' + 10
		case r >= 'A' && r <= 'F':
			c |= r - 'A' + 10
		default:
			return 0, false
		}
	}
	return c, true
}

// writeWTF8 appends r as UTF-8, or a surrogate as the three bytes UTF-8
// would give it if it were allowed, so a lone surrogate keeps its identity.
func writeWTF8(b *strings.Builder, r rune) {
	if r >= 0xd800 && r <= 0xdfff {
		b.WriteByte(byte(0xe0 | r>>12))
		b.WriteByte(byte(0x80 | (r>>6)&0x3f))
		b.WriteByte(byte(0x80 | r&0x3f))
		return
	}
	b.WriteRune(r)
}
