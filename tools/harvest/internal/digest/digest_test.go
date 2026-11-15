package digest

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

// fixtureDigest is what the fixture renders to: the talk, in order, and none
// of the lines whose text is in capitals.
const fixtureDigest = "## user\nWhy does the nightly export skip the last row?\n\n" +
	"## assistant\nThe loop stops one short: it compares with <= on a zero-based index.\n\n" +
	"## user\nHere is the failing run.\n\n" +
	"## assistant\nFixed: the bound is now < len.\nCafé rows with ü now export too.\n\n"

// leaks is the text of every fixture line that must stay out of a digest.
var leaks = []string{
	"QUEUED-PROMPT", "ATTACHMENT-TEXT", "THINKING-TEXT", "TOOL-INPUT", "TOOL-OUTPUT",
	"SYSTEM-HOOK", "HARNESS-NOTE", "SUBAGENT-TURN", "LAST-PROMPT",
}

func readFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/transcript.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBuildFixture(t *testing.T) {
	fixture := readFixture(t)
	got, err := Build(strings.NewReader(fixture), 0, 1<<20)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Text != fixtureDigest {
		t.Errorf("Build().Text =\n%s\nwant\n%s", got.Text, fixtureDigest)
	}
	if want := strings.Count(fixture, "\n"); got.Lines != want {
		t.Errorf("Build().Lines = %d, want %d", got.Lines, want)
	}
	for _, leak := range leaks {
		if strings.Contains(got.Text, leak) {
			t.Errorf("the digest carries %s", leak)
		}
	}
}

func TestBuild(t *testing.T) {
	user := func(text string) string {
		return `{"type":"user","message":{"role":"user","content":"` + text + `"}}` + "\n"
	}
	tests := []struct {
		name      string
		input     string
		skip      int
		maxBytes  int
		wantText  string
		wantLines int
	}{
		{
			name:      "empty transcript",
			input:     "",
			maxBytes:  100,
			wantText:  "",
			wantLines: 0,
		},
		{
			name:      "lines up to the mark are skipped",
			input:     user("old") + user("new"),
			skip:      1,
			maxBytes:  100,
			wantText:  "## user\nnew\n\n",
			wantLines: 2,
		},
		{
			name:      "a mark past the end renders nothing",
			input:     user("old"),
			skip:      5,
			maxBytes:  100,
			wantText:  "",
			wantLines: 1,
		},
		{
			name:      "a last line without a newline is left for later",
			input:     user("done") + `{"type":"user","message":{"content":"half`,
			maxBytes:  100,
			wantText:  "## user\ndone\n\n",
			wantLines: 1,
		},
		{
			name:      "an undecodable line is counted and skipped",
			input:     "not json\n" + user("after"),
			maxBytes:  100,
			wantText:  "## user\nafter\n\n",
			wantLines: 2,
		},
		{
			name:      "the end of the session is kept",
			input:     user("first") + user("second"),
			maxBytes:  len("## user\nsecond\n\n"),
			wantText:  "## user\nsecond\n\n",
			wantLines: 2,
		},
		{
			name:      "the cut moves past a split rune",
			input:     user("ü"),
			maxBytes:  len("\xbc\n\n"), // the cut lands on ü's second byte
			wantText:  "\n\n",
			wantLines: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Build(strings.NewReader(tc.input), tc.skip, tc.maxBytes)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if got.Text != tc.wantText {
				t.Errorf("Build().Text = %q, want %q", got.Text, tc.wantText)
			}
			if got.Lines != tc.wantLines {
				t.Errorf("Build().Lines = %d, want %d", got.Lines, tc.wantLines)
			}
		})
	}
}

// TestBuildLongLine guards the no-length-limit contract: a tool result of
// many megabytes must not stop the read, and the text after it still counts.
func TestBuildLongLine(t *testing.T) {
	huge := `{"type":"user","message":{"content":[{"type":"tool_result","content":"` +
		strings.Repeat("x", 8<<20) + `"}]}}` + "\n"
	input := huge + `{"type":"assistant","message":{"content":[{"type":"text","text":"after"}]}}` + "\n"
	got, err := Build(strings.NewReader(input), 0, 100)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got.Text != "## assistant\nafter\n\n" || got.Lines != 2 {
		t.Errorf(
			"Build() = %q, %d lines; want the line after the long one, 2 lines",
			got.Text,
			got.Lines,
		)
	}
}

func TestEntry(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   string
		wantOK bool
	}{
		{
			name:   "user string",
			line:   `{"type":"user","message":{"content":"a question"}}`,
			want:   "## user\na question\n\n",
			wantOK: true,
		},
		{
			name: "text blocks join, other blocks drop",
			line: `{"type":"assistant","message":{"content":[{"type":"text","text":"one"},` +
				`{"type":"tool_use","name":"Bash","input":{}},{"type":"text","text":"two"}]}}`,
			want:   "## assistant\none\ntwo\n\n",
			wantOK: true,
		},
		{
			name:   "isSidechain null is a main-session line",
			line:   `{"type":"user","isSidechain":null,"message":{"content":"kept"}}`,
			want:   "## user\nkept\n\n",
			wantOK: true,
		},
		{
			name: "tool use only",
			line: `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}`,
		},
		{
			name: "tool result only",
			line: `{"type":"user","message":{"content":[{"type":"tool_result","content":"out"}]}}`,
		},
		{
			name: "harness note",
			line: `{"type":"user","isMeta":true,"message":{"content":"note"}}`,
		},
		{
			name: "subagent turn",
			line: `{"type":"assistant","isSidechain":true,"message":{"content":[{"type":"text","text":"x"}]}}`,
		},
		{
			name: "system line",
			line: `{"type":"system","content":"x"}`,
		},
		{
			name: "empty string content",
			line: `{"type":"user","message":{"content":""}}`,
		},
		{
			name: "no message",
			line: `{"type":"user"}`,
		},
		{
			name: "content of another shape",
			line: `{"type":"user","message":{"content":{"text":"x"}}}`,
		},
		{
			name: "not JSON",
			line: `{"type":"user",`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Entry([]byte(tc.line))
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("Entry() = %q, %v; want %q, %v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestTail(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{name: "shorter than n", s: "abc", n: 5, want: "abc"},
		{name: "exactly n", s: "abc", n: 3, want: "abc"},
		{name: "ascii cut", s: "abcdef", n: 2, want: "ef"},
		{name: "cut inside a rune moves forward", s: "aéb", n: 2, want: "b"},
		{name: "cut on a rune start keeps it", s: "aéb", n: 3, want: "éb"},
		{name: "zero", s: "abc", n: 0, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Tail(tc.s, tc.n)
			if got != tc.want {
				t.Errorf("Tail(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("Tail(%q, %d) = %q is not valid UTF-8", tc.s, tc.n, got)
			}
		})
	}
}
