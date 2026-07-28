package ref

import "testing"

func TestStringRoundTrips(t *testing.T) {
	s := String(7432, "refactor-auth")
	if s != "wire://localhost:7432/refactor-auth" {
		t.Fatalf("String = %q", s)
	}
	// What the server emits, the client has to accept — that round trip is the
	// whole point of the token.
	got, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	if got != "refactor-auth" {
		t.Errorf("Parse(%q) = %q", s, got)
	}
}

func TestParsePassesThroughPlainRefs(t *testing.T) {
	for _, in := range []string{"refactor-auth", "ch_79debcd3a329", "  spaced  "} {
		got, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got == "" {
			t.Errorf("Parse(%q) dropped the ref", in)
		}
	}
	if got, _ := Parse("  spaced  "); got != "spaced" {
		t.Errorf("Parse did not trim: %q", got)
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	for _, in := range []string{
		"",
		"http://localhost:7432/foo", // wrong scheme, not a silent pass
		"wire://localhost:7432/",    // no channel
		"wire://localhost:7432/a/b", // more than one path segment
		"wire://localhost:7432",     // no path at all
	} {
		if got, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %q, want an error", in, got)
		}
	}
}
