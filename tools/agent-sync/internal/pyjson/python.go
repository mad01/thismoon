package pyjson

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"unicode"
)

// The helpers in this file reproduce the parts of Python's value model the
// sync scripts' output depends on, for the values Loads produces: bool(v),
// str(v) and repr(v), plus the type names their error messages used.

// Truthy reports whether v is true the way Python's bool(v) is: None,
// False, zero, the empty string and an empty array or object are false,
// and everything else is true, NaN included.
func Truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case *big.Int:
		return t.Sign() != 0
	case float64:
		return t != 0
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	case *Object:
		return t.Len() > 0
	default:
		return true
	}
}

// Str renders v the way Python's str(v) does: a string is itself, and
// every other value reads as Repr renders it.
func Str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return Repr(v)
}

// Repr renders v the way Python's repr(v) does: None, True and False by
// name, a float as float.__repr__ (nan, inf and -inf where json.dumps says
// NaN, Infinity and -Infinity), a string quoted as repr(str) quotes it, and
// an array or object as a list or dict literal.
func Repr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case *big.Int:
		return t.String()
	case float64:
		return floatRepr(t)
	case string:
		return reprString(t)
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = Repr(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *Object:
		parts := make([]string, len(t.keys))
		for i, k := range t.keys {
			parts[i] = reprString(k) + ": " + Repr(t.vals[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		// Only values Loads produces have a Python spelling.
		return fmt.Sprint(v)
	}
}

// Describe names v's JSON type for an error message: "an object", "an
// array", "a string", "null", "a bool" or "a number".
func Describe(v any) string {
	switch v.(type) {
	case *Object:
		return "an object"
	case []any:
		return "an array"
	case string:
		return "a string"
	case nil:
		return "null"
	case bool:
		return "a bool"
	default:
		return "a number"
	}
}

// floatRepr is float.__repr__: json.dumps agrees with it for every finite
// value and spells the other three differently.
func floatRepr(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	default:
		return formatFloat(f)
	}
}

// reprString quotes s the way Python's repr(str) does: single quotes unless
// s holds a single quote and no double quote, with the backslash, the quote
// and every character Python does not print escaped. A lone surrogate, kept
// as WTF-8, is unprintable and so reads as its \uXXXX escape.
func reprString(s string) string {
	quote := '\''
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteRune(quote)
	for i := 0; i < len(s); {
		r, size := decodeWTF8(s[i:])
		i += size
		switch {
		case r == quote || r == '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case unicode.IsPrint(r):
			// Go's printable set is Python's str.isprintable: letters,
			// marks, numbers, punctuation, symbols and the ASCII space.
			b.WriteRune(r)
		case r <= 0xff:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r <= 0xffff:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	b.WriteRune(quote)
	return b.String()
}
