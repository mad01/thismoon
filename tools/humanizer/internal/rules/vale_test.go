package rules

import "testing"

func TestParseValeJSONMapShape(t *testing.T) {
	payload := []byte(
		`{"stdin.md":[{"Check":"Humanizer.AIVocabulary","Line":3,"Match":"delve","Message":"AI vocab","Severity":"warning","Span":[12,17]}]}`,
	)
	findings, err := parseValeJSON(payload)
	if err != nil {
		t.Fatalf("parseValeJSON: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.RuleID != "Humanizer.AIVocabulary" {
		t.Errorf("rule id: got %q", f.RuleID)
	}
	if f.Line != 3 || f.Column != 12 {
		t.Errorf("position: got line=%d col=%d", f.Line, f.Column)
	}
	if f.Severity != "warning" {
		t.Errorf("severity: got %q", f.Severity)
	}
}

func TestParseValeJSONEmpty(t *testing.T) {
	cases := [][]byte{[]byte(``), []byte(`{}`), []byte(`[]`), []byte(`{"stdin.md":[]}`)}
	for _, c := range cases {
		findings, err := parseValeJSON(c)
		if err != nil {
			t.Fatalf("parseValeJSON(%q): %v", string(c), err)
		}
		if len(findings) != 0 {
			t.Errorf("parseValeJSON(%q): expected 0 findings, got %d", string(c), len(findings))
		}
	}
}

func TestSeverityRank(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 1}, {"suggestion", 1}, {"warning", 2}, {"error", 3}, {"bogus", 0},
	}
	for _, c := range cases {
		if got := severityRank(c.in); got != c.want {
			t.Errorf("severityRank(%q)=%d, want %d", c.in, got, c.want)
		}
	}
}
