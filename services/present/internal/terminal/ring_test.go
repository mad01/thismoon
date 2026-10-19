package terminal

import "testing"

func TestRingKeepsTheNewestBytes(t *testing.T) {
	cases := []struct {
		name   string
		writes []string
		want   string
	}{
		{"empty", nil, ""},
		{"under size", []string{"ab", "c"}, "abc"},
		{"exactly full", []string{"abcd"}, "abcd"},
		{"wraps", []string{"abc", "def"}, "cdef"},
		{"wraps twice", []string{"abc", "def", "gh"}, "efgh"},
		{"one write over size", []string{"abcdefgh"}, "efgh"},
		{"fills to the end then wraps", []string{"ab", "cd", "e"}, "bcde"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRing(4)
			for _, w := range tc.writes {
				r.write([]byte(w))
			}
			if got := string(r.bytes()); got != tc.want {
				t.Errorf("bytes() = %q, want %q", got, tc.want)
			}
		})
	}
}
