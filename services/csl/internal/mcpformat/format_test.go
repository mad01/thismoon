package mcpformat

import (
	"slices"
	"strings"
	"testing"
)

func TestNames(t *testing.T) {
	want := []string{Text, JSON, JSONL, TOON, CSV, MarkdownKV, XML}
	if got := Names(); !slices.Equal(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	first := Names()
	first[0] = "mutated"
	if got := Names()[0]; got != Text {
		t.Fatalf("Names()[0] = %q after mutating an earlier result, want %q", got, Text)
	}
}

func TestValid(t *testing.T) {
	for _, name := range Names() {
		if !Valid(name) {
			t.Errorf("Valid(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", "yaml", "JSON", "Text", "markdown"} {
		if Valid(name) {
			t.Errorf("Valid(%q) = true, want false", name)
		}
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name       string
		param      string
		configured string
		want       string
		wantErr    string // substring of the error; empty means success
	}{
		{name: "param wins over config", param: CSV, configured: XML, want: CSV},
		{name: "config used when param empty", configured: XML, want: XML},
		{name: "both empty yields default", want: Default},
		{name: "unknown param names the value", param: "yaml", configured: XML, wantErr: `"yaml"`},
		{name: "unknown config names the key", configured: "yaml", wantErr: "mcp.response_format"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.param, tc.configured)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Resolve(%q, %q) error = %v, want one containing %q", tc.param, tc.configured, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(%q, %q) error = %v", tc.param, tc.configured, err)
			}
			if got != tc.want {
				t.Fatalf("Resolve(%q, %q) = %q, want %q", tc.param, tc.configured, got, tc.want)
			}
		})
	}
}

func TestEncodeRejectsText(t *testing.T) {
	_, err := Encode(Text, newFixture())
	if err == nil || !strings.Contains(err.Error(), "caller") {
		t.Fatalf("Encode(Text) error = %v, want one saying the caller renders text", err)
	}
}

func TestEncodeRejectsUnknownFormat(t *testing.T) {
	_, err := Encode("yaml", newFixture())
	if err == nil || !strings.Contains(err.Error(), `"yaml"`) {
		t.Fatalf("Encode(\"yaml\") error = %v, want one naming the format", err)
	}
}

func TestEncodeEveryFormat(t *testing.T) {
	for _, format := range Names() {
		if format == Text {
			continue
		}
		t.Run(format, func(t *testing.T) {
			out, err := Encode(format, newFixture())
			if err != nil {
				t.Fatalf("Encode(%q) error = %v", format, err)
			}
			if out == "" {
				t.Fatalf("Encode(%q) returned empty output", format)
			}
		})
	}
}

// orderedFixture has struct field order that contradicts alphabetical order
// at both the top level and inside the records.
type orderedFixture struct {
	Zeta  int          `json:"zeta"`
	Alpha int          `json:"alpha"`
	Rows  []orderedRow `json:"rows"`
}

type orderedRow struct {
	Zulu  string `json:"zulu"`
	Apple string `json:"apple"`
}

func TestEncodeKeyOrderFollowsStruct(t *testing.T) {
	v := orderedFixture{Zeta: 1, Alpha: 2, Rows: []orderedRow{{Zulu: "z", Apple: "a"}}}
	for _, format := range []string{JSONL, CSV, MarkdownKV, XML} {
		t.Run(format, func(t *testing.T) {
			out, err := Encode(format, v)
			if err != nil {
				t.Fatalf("Encode(%q) error = %v", format, err)
			}
			assertBefore(t, out, "zeta", "alpha")
			assertBefore(t, out, "zulu", "apple")
		})
	}
}

func assertBefore(t *testing.T, out, first, second string) {
	t.Helper()
	i, j := strings.Index(out, first), strings.Index(out, second)
	if i < 0 || j < 0 {
		t.Fatalf("output lacks %q or %q:\n%s", first, second, out)
	}
	if i > j {
		t.Fatalf("%q appears after %q, want struct order:\n%s", first, second, out)
	}
}
