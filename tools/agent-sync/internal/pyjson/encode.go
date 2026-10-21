package pyjson

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Compact renders v the way json.dumps(v) does: one line, ", " between
// items, ": " after keys, keys in their stored order.
func Compact(v any) string {
	var b strings.Builder
	encoder{itemSep: ", "}.value(&b, v, 0)
	return b.String()
}

// Indent renders v the way json.dumps(v, indent=n) does: every item on its
// own line, nested n spaces deeper per level, "," ending each line but the
// last, empty objects and arrays as {} and []. No trailing newline.
func Indent(v any, n int) string {
	var b strings.Builder
	encoder{itemSep: ",", indent: strings.Repeat(" ", n), multiline: true}.value(&b, v, 0)
	return b.String()
}

// encoder holds one json.dumps layout. Both layouts escape every non-ASCII
// character (ensure_ascii) and leave "/" and HTML characters alone.
type encoder struct {
	itemSep   string
	indent    string
	multiline bool
}

func (e encoder) value(b *strings.Builder, v any, level int) {
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(t))
	case *big.Int:
		b.WriteString(t.String())
	case float64:
		b.WriteString(formatFloat(t))
	case string:
		quote(b, t)
	case []any:
		e.list(b, t, level)
	case *Object:
		e.object(b, t, level)
	default:
		// Only values Loads produces reach here; anything else is a bug in
		// the caller, not bad input.
		panic(fmt.Sprintf("pyjson: cannot encode %T", v))
	}
}

func (e encoder) list(b *strings.Builder, l []any, level int) {
	if len(l) == 0 {
		b.WriteString("[]")
		return
	}
	b.WriteByte('[')
	sep := e.open(b, level+1)
	for i, item := range l {
		if i > 0 {
			b.WriteString(sep)
		}
		e.value(b, item, level+1)
	}
	e.close(b, level)
	b.WriteByte(']')
}

func (e encoder) object(b *strings.Builder, o *Object, level int) {
	if o.Len() == 0 {
		b.WriteString("{}")
		return
	}
	b.WriteByte('{')
	sep := e.open(b, level+1)
	for i, k := range o.keys {
		if i > 0 {
			b.WriteString(sep)
		}
		quote(b, k)
		b.WriteString(": ")
		e.value(b, o.vals[k], level+1)
	}
	e.close(b, level)
	b.WriteByte('}')
}

// open starts a non-empty container's body at level and returns the
// separator that goes between its items.
func (e encoder) open(b *strings.Builder, level int) string {
	if !e.multiline {
		return e.itemSep
	}
	newline := "\n" + strings.Repeat(e.indent, level)
	b.WriteString(newline)
	return e.itemSep + newline
}

// close ends a non-empty container's body opened one level below level.
func (e encoder) close(b *strings.Builder, level int) {
	if e.multiline {
		b.WriteString("\n" + strings.Repeat(e.indent, level))
	}
}

// formatFloat renders f as Python's float repr does: the shortest digits
// that round-trip, positional between 1e-4 and 1e16 with ".0" on whole
// values, scientific outside it with a signed exponent of at least two
// digits, and NaN, Infinity and -Infinity for the non-finite values.
func formatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	// Go's shortest form, e.g. "-1.25e-07": sign, digits, exponent.
	s := strconv.FormatFloat(f, 'e', -1, 64)
	sign := ""
	if s[0] == '-' {
		sign, s = "-", s[1:]
	}
	mantissa, expText, _ := strings.Cut(s, "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	exp, _ := strconv.Atoi(expText)
	if exp < -4 || exp >= 16 {
		m := digits[:1]
		if len(digits) > 1 {
			m += "." + digits[1:]
		}
		esign := "+"
		if exp < 0 {
			esign, exp = "-", -exp
		}
		return fmt.Sprintf("%s%se%s%02d", sign, m, esign, exp)
	}
	point := exp + 1 // digits before the decimal point
	switch {
	case point <= 0:
		return sign + "0." + strings.Repeat("0", -point) + digits
	case point >= len(digits):
		return sign + digits + strings.Repeat("0", point-len(digits)) + ".0"
	default:
		return sign + digits[:point] + "." + digits[point:]
	}
}

// quote writes s as a JSON string with Python's ensure_ascii escaping:
// printable ASCII stays, the short escapes cover \" \\ \n \r \t \b \f, and
// every other character becomes \uXXXX in lowercase hex, as a surrogate
// pair above U+FFFF.
func quote(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := decodeWTF8(s[i:])
		i += size
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r >= 0x20 && r <= 0x7e:
			b.WriteByte(byte(r))
		case r >= 0x10000:
			r -= 0x10000
			fmt.Fprintf(b, `\u%04x\u%04x`, 0xd800|r>>10, 0xdc00|r&0x3ff)
		default:
			fmt.Fprintf(b, `\u%04x`, r)
		}
	}
	b.WriteByte('"')
}

// decodeWTF8 decodes the first character of s, accepting the three-byte
// surrogate form writeWTF8 produces. Invalid bytes decode as U+FFFD one at a
// time.
func decodeWTF8(s string) (rune, int) {
	if len(s) >= 3 && s[0] == 0xed && s[1] >= 0xa0 && s[1] <= 0xbf && s[2] >= 0x80 &&
		s[2] <= 0xbf {
		return 0xd000 | rune(s[1]&0x3f)<<6 | rune(s[2]&0x3f), 3
	}
	return utf8.DecodeRuneInString(s)
}
