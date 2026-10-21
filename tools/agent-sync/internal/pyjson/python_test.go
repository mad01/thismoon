package pyjson

import (
	"math"
	"math/big"
	"testing"
)

// ch spells a code point, so the source stays ASCII.
func ch(r rune) string { return string(r) }

// The wants below are what Python 3.13 printed for the same input, e.g.
// python3 -c 'print(repr("it'"'"'s"))'.

func TestReprOfStringsMatchesPython(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"a", `'a'`},
		{"", `''`},
		{"it's", `"it's"`},
		{`a"b`, `'a"b'`},
		{`a"b'c`, `'a"b\'c'`},
		{"both ' and \" quotes", `'both \' and " quotes'`},
		{`back\slash`, `'back\\slash'`},
		{"tab\there\n", `'tab\there\n'`},
		{"\t\n\r\x01\x7f", `'\t\n\r\x01\x7f'`},
		{"\x00", `'\x00'`},
		{"caf" + ch(0xe9), "'caf" + ch(0xe9) + "'"},
		{ch(0x301), "'" + ch(0x301) + "'"},
		{ch(0xa0), `'\xa0'`},
		{"zero" + ch(0x200b) + "width", `'zero` + `\` + `u200bwidth'`},
		{ch(0xfffd), "'" + ch(0xfffd) + "'"},
		{ch(0x1f600), "'" + ch(0x1f600) + "'"},
		{ch(0xe0001), `'\U000e0001'`},
	} {
		if got := Repr(c.in); got != c.want {
			t.Errorf("Repr(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

// TestReprOfLoneSurrogate checks the WTF-8 form Loads keeps a lone
// surrogate in: repr('a\ud800b') in Python.
func TestReprOfLoneSurrogate(t *testing.T) {
	v, err := Loads([]byte(`"a\ud800b"`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Repr(v), `'a\ud800b'`; got != want {
		t.Errorf("Repr = %s, want %s", got, want)
	}
}

func TestStrAndReprOfJSONValues(t *testing.T) {
	obj := NewObject()
	obj.Set("source", "github")
	obj.Set("n", big.NewInt(1))
	obj.Set("l", []any{true, nil, 1.5})
	for _, c := range []struct {
		v         any
		str, repr string
	}{
		{nil, "None", "None"},
		{true, "True", "True"},
		{false, "False", "False"},
		{big.NewInt(-7), "-7", "-7"},
		{0.1, "0.1", "0.1"},
		{1.0, "1.0", "1.0"},
		{1e16, "1e+16", "1e+16"},
		{1e-5, "1e-05", "1e-05"},
		{math.NaN(), "nan", "nan"},
		{math.Inf(1), "inf", "inf"},
		{math.Inf(-1), "-inf", "-inf"},
		{"x", "x", "'x'"},
		{[]any{}, "[]", "[]"},
		{
			obj,
			"{'source': 'github', 'n': 1, 'l': [True, None, 1.5]}",
			"{'source': 'github', 'n': 1, 'l': [True, None, 1.5]}",
		},
	} {
		if got := Str(c.v); got != c.str {
			t.Errorf("Str(%v) = %s, want %s", c.v, got, c.str)
		}
		if got := Repr(c.v); got != c.repr {
			t.Errorf("Repr(%v) = %s, want %s", c.v, got, c.repr)
		}
	}
}

func TestTruthy(t *testing.T) {
	falsy := []any{nil, false, big.NewInt(0), 0.0, "", []any{}, NewObject()}
	for _, v := range falsy {
		if Truthy(v) {
			t.Errorf("Truthy(%#v) = true", v)
		}
	}
	nonEmpty := NewObject()
	nonEmpty.Set("k", nil)
	for _, v := range []any{true, big.NewInt(-1), math.NaN(), "0", []any{nil}, nonEmpty} {
		if !Truthy(v) {
			t.Errorf("Truthy(%#v) = false", v)
		}
	}
}

func TestDescribe(t *testing.T) {
	for _, c := range []struct {
		v    any
		want string
	}{
		{NewObject(), "an object"},
		{[]any{}, "an array"},
		{"", "a string"},
		{nil, "null"},
		{false, "a bool"},
		{big.NewInt(1), "a number"},
		{1.5, "a number"},
	} {
		if got := Describe(c.v); got != c.want {
			t.Errorf("Describe(%#v) = %q, want %q", c.v, got, c.want)
		}
	}
}
