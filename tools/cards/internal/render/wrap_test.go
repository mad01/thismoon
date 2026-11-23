package render

import (
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/cards/internal/protocol"
)

// plainLines returns one unstyled span per line.
func plainLines(texts ...string) []protocol.Line {
	lines := make([]protocol.Line, len(texts))
	for i, t := range texts {
		lines[i] = protocol.Line{span(t, protocol.ColorDefault)}
	}
	return lines
}

func equalLines(a, b []protocol.Line) bool {
	return slices.EqualFunc(a, b, func(x, y protocol.Line) bool { return slices.Equal(x, y) })
}

func cloneLines(lines []protocol.Line) []protocol.Line {
	out := make([]protocol.Line, len(lines))
	for i, l := range lines {
		out[i] = slices.Clone(l)
	}
	return out
}

func boldSpan(text string, c protocol.Color) protocol.Span {
	return protocol.Span{Text: text, Color: c, Bold: true}
}

func TestWrapLast(t *testing.T) {
	cases := []struct {
		name  string
		lines []protocol.Line
		width int
		want  []protocol.Line
	}{
		{
			name:  "a last line that fits is left as it is",
			lines: plainLines("acme/api#12", "alice"),
			width: 11,
			want:  plainLines("acme/api#12", "alice"),
		},
		{
			name:  "only the last line wraps",
			lines: plainLines("acme/api#12 fix the login redirect", "alice"),
			width: 8,
			want:  plainLines("acme/api#12 fix the login redirect", "alice"),
		},
		{
			name:  "a long last line breaks at the last space that fits",
			lines: plainLines("acme/api#12", "fix the login redirect"),
			width: 12,
			want:  plainLines("acme/api#12", "fix the", "login redirect"),
		},
		{
			name:  "a one-line card takes a second line",
			lines: plainLines("fix the login"),
			width: 9,
			want:  plainLines("fix the", "login"),
		},
		{
			name:  "a space right after the width ends a full first row",
			lines: plainLines("fix the login"),
			width: 7,
			want:  plainLines("fix the", "login"),
		},
		{
			name:  "the second row is left long for the draw to cut",
			lines: plainLines("fix the login redirect"),
			width: 4,
			want:  plainLines("fix", "the login redirect"),
		},
		{
			name:  "a run of spaces at the break is dropped",
			lines: plainLines("fix   the login"),
			width: 4,
			want:  plainLines("fix", "the login"),
		},
		{
			name:  "spaces past the width take no second row",
			lines: plainLines("fix the   "),
			width: 7,
			want:  plainLines("fix the"),
		},
		{
			name:  "no space to break at breaks at the width",
			lines: plainLines("acme/api#12"),
			width: 4,
			want:  plainLines("acme", "/api#12"),
		},
		{
			name:  "leading spaces alone break at the width",
			lines: plainLines("  indented"),
			width: 4,
			want:  plainLines("  in", "dented"),
		},
		{
			name:  "a first row of only spaces leaves the line as it is",
			lines: plainLines("   indented"),
			width: 2,
			want:  plainLines("   indented"),
		},
		{
			name:  "a first character wider than the width leaves the line as it is",
			lines: plainLines("日本"),
			width: 1,
			want:  plainLines("日本"),
		},
		{
			name: "a span the break falls in keeps its style on both rows",
			lines: []protocol.Line{{
				span("#12", protocol.ColorBlue),
				boldSpan(" fix the login", protocol.ColorRed),
			}},
			width: 11,
			want: []protocol.Line{
				{span("#12", protocol.ColorBlue), boldSpan(" fix the", protocol.ColorRed)},
				{boldSpan("login", protocol.ColorRed)},
			},
		},
		{
			name: "a hard break inside a span keeps its style on both rows",
			lines: []protocol.Line{{
				span("#", protocol.ColorDim),
				boldSpan("acme/api", protocol.ColorGreen),
			}},
			width: 5,
			want: []protocol.Line{
				{span("#", protocol.ColorDim), boldSpan("acme", protocol.ColorGreen)},
				{boldSpan("/api", protocol.ColorGreen)},
			},
		},
		{
			name: "spans left empty by the break are dropped",
			lines: []protocol.Line{{
				span("fix", protocol.ColorRed),
				span(" ", protocol.ColorBlue),
				span("", protocol.ColorGreen),
				span("login", protocol.ColorGreen),
			}},
			width: 5,
			want: []protocol.Line{
				{span("fix", protocol.ColorRed)},
				{span("login", protocol.ColorGreen)},
			},
		},
		{name: "no lines", lines: nil, width: 8, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Cells{}).WrapLast(tc.lines, tc.width); !equalLines(got, tc.want) {
				t.Errorf("WrapLast(%d) = %+v, want %+v", tc.width, got, tc.want)
			}
		})
	}
}

func TestWrapLastWideRunes(t *testing.T) {
	// "日本語" and "テスト" are three runes of two cells each. wcwidth counts
	// the family as 6 cells, grapheme width as 2; the flag is 2 either way.
	cases := []struct {
		name         string
		text         string
		width        int
		wc, grapheme []string
	}{
		{
			name:     "wide runes break at a space",
			text:     "日本語 テスト",
			width:    6,
			wc:       []string{"日本語", "テスト"},
			grapheme: []string{"日本語", "テスト"},
		},
		{
			name:     "a wide rune that does not fit starts the second row",
			text:     "日本語テスト",
			width:    5,
			wc:       []string{"日本", "語テスト"},
			grapheme: []string{"日本", "語テスト"},
		},
		{
			name:     "the family fits on the first row by grapheme width only",
			text:     "ab " + family + " cd",
			width:    6,
			wc:       []string{"ab", family + " cd"},
			grapheme: []string{"ab " + family, "cd"},
		},
		{
			name:     "a hard break never splits the family",
			text:     "ab" + family + "cd",
			width:    4,
			wc:       []string{"ab", family + "cd"},
			grapheme: []string{"ab" + family, "cd"},
		},
		{
			name:     "a hard break never splits a flag",
			text:     flag + flag + flag,
			width:    3,
			wc:       []string{flag, flag + flag},
			grapheme: []string{flag, flag + flag},
		},
	}
	for _, tc := range cases {
		modes := []struct {
			name  string
			cells Cells
			want  []string
		}{
			{"wcwidth", wcCells, tc.wc},
			{"grapheme width", graphemeCells, tc.grapheme},
		}
		for _, m := range modes {
			t.Run(tc.name+"/"+m.name, func(t *testing.T) {
				got := m.cells.WrapLast(plainLines(tc.text), tc.width)
				if want := plainLines(m.want...); !equalLines(got, want) {
					t.Errorf("WrapLast(%q, %d) = %+v, want %+v", tc.text, tc.width, got, want)
				}
			})
		}
	}
}

// TestWrapLastFitsAnyWidth checks, at every width from below zero to past
// the text and by both methods, that the first row fits, the second starts
// on a word, and only spaces are lost.
func TestWrapLastFitsAnyWidth(t *testing.T) {
	texts := []string{
		"fix the login redirect",
		"acme/api#12",
		"  indented  text  ",
		"日本語 テスト",
		"x" + family + flag + family,
		"ab " + family + " cd",
		flag + flag + flag,
		" ",
		"",
	}
	for _, c := range []Cells{wcCells, graphemeCells} {
		for _, text := range texts {
			for width := -2; width <= c.Width(text)+1; width++ {
				lines := plainLines("acme/api#12", text)
				got := c.WrapLast(lines, width)
				checkWrap(t, c, text, width, lines, got)
			}
		}
	}
}

func checkWrap(t *testing.T, c Cells, text string, width int, lines, got []protocol.Line) {
	t.Helper()
	if len(got) < 2 || len(got) > 3 || !slices.Equal(got[0], lines[0]) {
		t.Errorf("method %d: WrapLast(%q, %d) = %+v, want the first line kept and 1 or 2 more",
			c.Method, text, width, got)
		return
	}
	if width <= 0 || c.Width(text) <= width {
		if !equalLines(got, lines) {
			t.Errorf("method %d: WrapLast(%q, %d) = %+v, want it unchanged",
				c.Method, text, width, got)
		}
		return
	}
	rows := make([]string, len(got)-1)
	for i, l := range got[1:] {
		rows[i] = lineText(l)
	}
	if len(rows) == 2 {
		if w := c.Width(rows[0]); rows[0] == "" || w > width {
			t.Errorf("method %d: WrapLast(%q, %d) first row %q is %d cells",
				c.Method, text, width, rows[0], w)
		}
		if rows[1] == "" || strings.HasPrefix(rows[1], " ") {
			t.Errorf("method %d: WrapLast(%q, %d) second row = %q, want it to start on a word",
				c.Method, text, width, rows[1])
		}
	}
	noSpaces := func(s string) string { return strings.ReplaceAll(s, " ", "") }
	if joined := strings.Join(rows, ""); noSpaces(joined) != noSpaces(text) {
		t.Errorf("method %d: WrapLast(%q, %d) rows %q lost more than spaces",
			c.Method, text, width, rows)
	}
}

func TestWrapLastKeepsInput(t *testing.T) {
	lines := []protocol.Line{
		{span("acme/api#12", protocol.ColorBlue)},
		{span("fix the", protocol.ColorDefault), span(" login redirect", protocol.ColorRed)},
	}
	before := cloneLines(lines)
	got := (Cells{}).WrapLast(lines, 10)
	if len(got) != 3 {
		t.Fatalf("WrapLast() = %+v, want the last line on two rows", got)
	}
	if !equalLines(lines, before) {
		t.Errorf("WrapLast() changed its input to %+v, want %+v", lines, before)
	}
	got[1][0].Text, got[2][0].Text = "x", "x"
	if !equalLines(lines, before) {
		t.Errorf("changing the result changed the input to %+v, want %+v", lines, before)
	}
}
