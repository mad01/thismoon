package finder

import "testing"

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "clean", in: "octo", want: "octo"},
		{name: "trailing ascii space", in: "octo ", want: "octo"},
		{name: "leading ascii space", in: " octo", want: "octo"},
		{name: "both sides ascii space", in: "  octo  ", want: "octo"},
		{name: "tab and newline", in: "\tocto\n", want: "octo"},
		{name: "non-breaking space", in: "octo ", want: "octo"},
		{name: "narrow no-break space", in: " octo", want: "octo"},
		{name: "ideographic space", in: "octo　", want: "octo"},
		{name: "empty", in: "", want: ""},
		{name: "only whitespace", in: " \t\n ", want: ""},
		{name: "internal space preserved", in: "foo bar", want: "foo bar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeQuery(tc.in); got != tc.want {
				t.Errorf("NormalizeQuery(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCompileMatcher(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantErr    bool
		matches    []string
		nonMatches []string
	}{
		{
			name:       "substring",
			query:      "octo",
			matches:    []string{"mad01/octo", "someone/octopus", "MAD01/OCTO"},
			nonMatches: []string{"mad01/other"},
		},
		{
			name:       "trailing space still matches cleanly",
			query:      "octo ",
			matches:    []string{"mad01/octo", "someone/octopus"},
			nonMatches: []string{"mad01/other"},
		},
		{
			name:       "non-breaking space trimmed",
			query:      "octo ",
			matches:    []string{"mad01/octo"},
			nonMatches: []string{"mad01/other"},
		},
		{
			name:       "regex anchors work",
			query:      "^mad01/octo$",
			matches:    []string{"mad01/octo"},
			nonMatches: []string{"mad01/octopus", "someone/mad01/octo"},
		},
		{name: "empty is rejected", query: "", wantErr: true},
		{name: "whitespace-only is rejected", query: " \t ", wantErr: true},
		{name: "invalid regex is rejected", query: "[", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re, err := CompileMatcher(tc.query)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("CompileMatcher(%q) expected error, got nil", tc.query)
				}
				return
			}
			if err != nil {
				t.Fatalf("CompileMatcher(%q) error: %v", tc.query, err)
			}
			for _, s := range tc.matches {
				if !re.MatchString(s) {
					t.Errorf("expected %q to match %q", tc.query, s)
				}
			}
			for _, s := range tc.nonMatches {
				if re.MatchString(s) {
					t.Errorf("expected %q NOT to match %q", tc.query, s)
				}
			}
		})
	}
}
