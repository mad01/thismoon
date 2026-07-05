package notify

import "testing"

func TestAppleScriptStringEscapes(t *testing.T) {
	cases := map[string]string{
		`plain`:         `"plain"`,
		`say "hi"`:      `"say \"hi\""`,
		`back\slash`:    `"back\\slash"`,
		`"; do evil --`: `"\"; do evil --"`,
	}
	for in, want := range cases {
		if got := appleScriptString(in); got != want {
			t.Errorf("appleScriptString(%q) = %q, want %q", in, got, want)
		}
	}
}
