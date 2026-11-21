package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/mad01/thismoon/tools/cards/internal/protocol"
)

// SGR codes the 16-color palette and the attributes render as.
const (
	sgrRed  = "\x1b[31m"
	sgrBlue = "\x1b[34m"
	sgrBold = "\x1b[1m"
	sgrDim  = "\x1b[2m"
)

func span(text string, c protocol.Color) protocol.Span {
	return protocol.Span{Text: text, Color: c}
}

func TestLineColors(t *testing.T) {
	cases := []struct {
		name string
		line protocol.Line
		tone protocol.Color
		want string
	}{
		{
			name: "default span with default tone stays unstyled",
			line: protocol.Line{span("alice", protocol.ColorDefault)},
			tone: protocol.ColorDefault,
			want: "alice",
		},
		{
			name: "tone colors a default span",
			line: protocol.Line{span("alice", protocol.ColorDefault)},
			tone: protocol.ColorRed,
			want: sgrRed + "alice\x1b[m",
		},
		{
			name: "span color wins over tone",
			line: protocol.Line{
				span("#12", protocol.ColorBlue),
				span(" fix", protocol.ColorDefault),
			},
			tone: protocol.ColorRed,
			want: sgrBlue + "#12\x1b[m" + sgrRed + " fix\x1b[m",
		},
		{
			name: "dim tone is faint, not a hue",
			line: protocol.Line{span("2h ago", protocol.ColorDefault)},
			tone: protocol.ColorDim,
			want: sgrDim + "2h ago\x1b[m",
		},
		{
			name: "bold span is bold",
			line: protocol.Line{{Text: "acme/api", Color: protocol.ColorDefault, Bold: true}},
			tone: protocol.ColorDefault,
			want: sgrBold + "acme/api\x1b[m",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Cells{}).Line(tc.line, tc.tone, 80); got != tc.want {
				t.Errorf("Line() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLineCutsToWidth(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		width int
		want  string
	}{
		{name: "fits", text: "acme/api#12", width: 11, want: "acme/api#12"},
		{name: "ascii cut", text: "acme/api#12 fix login", width: 8, want: "acme/ap…"},
		// "日本語" is three runes of two cells each.
		{name: "wide runes cut on a cell boundary", text: "ab日本語", width: 5, want: "ab日…"},
		{name: "a wide rune that does not fit is dropped", text: "ab日本語", width: 4, want: "ab…"},
		{name: "width one leaves the ellipsis", text: "alice", width: 1, want: "…"},
		{name: "width zero is empty", text: "alice", width: 0, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line := protocol.Line{span(tc.text, protocol.ColorRed)}
			got := (Cells{}).Line(line, protocol.ColorDefault, tc.width)
			if plain := ansi.Strip(got); plain != tc.want {
				t.Errorf("Line() = %q, want %q", plain, tc.want)
			}
			if w := ansi.StringWidthWc(got); w > tc.width {
				t.Errorf("Line() is %d cells wide, want at most %d", w, tc.width)
			}
		})
	}
}

func TestLineCutKeepsStyleReset(t *testing.T) {
	got := (Cells{}).Line(
		protocol.Line{span("acme/api#12", protocol.ColorRed)}, protocol.ColorDefault, 6)
	if !strings.HasSuffix(got, "\x1b[m") {
		t.Errorf("Line() = %q, want the style reset kept after the cut", got)
	}
}

func TestPad(t *testing.T) {
	var c Cells
	if got := c.Pad("日", 4); got != "日  " {
		t.Errorf("Pad(日, 4) = %q, want two spaces after the wide rune", got)
	}
	if got := c.Pad("alice", 3); got != "alice" {
		t.Errorf("Pad(alice, 3) = %q, want it unchanged", got)
	}
}

// family is a ZWJ emoji sequence: three wide emoji joined by U+200D. WcWidth
// counts it as 6 cells, GraphemeWidth as one cluster of 2. The flag is two
// regional indicators, 2 cells either way.
const (
	family = "👨\u200d👩\u200d👧"
	flag   = "🇸🇪"
)

var (
	wcCells       = Cells{Method: ansi.WcWidth}
	graphemeCells = Cells{Method: ansi.GraphemeWidth}
)

func TestCellsCountByMethod(t *testing.T) {
	text := "ab" + family + flag + "cd"
	cases := []struct {
		name  string
		cells Cells
		width int
	}{
		{"wcwidth", wcCells, 12},
		{"grapheme width", graphemeCells, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cells.Width(text); got != tc.width {
				t.Errorf("Width(%q) = %d, want %d", text, got, tc.width)
			}
			if got := tc.cells.Width(tc.cells.Pad(text, 14)); got != 14 {
				t.Errorf("Pad to 14 gave %d cells", got)
			}
		})
	}
}

func TestCutFitsTheWidthByItsMethod(t *testing.T) {
	texts := []string{family, flag, "ab" + family + "cd", flag + flag + flag, "x" + family + flag}
	for _, c := range []Cells{wcCells, graphemeCells} {
		for _, text := range texts {
			for width := 1; width <= c.Width(text)+1; width++ {
				got := c.Cut(text, width)
				if w := c.Width(got); w > width {
					t.Errorf(
						"method %d: Cut(%q, %d) = %q is %d cells",
						c.Method,
						text,
						width,
						got,
						w,
					)
				}
				fits := c.Width(text) <= width
				if fits && got != text {
					t.Errorf(
						"method %d: Cut(%q, %d) = %q, want it whole",
						c.Method,
						text,
						width,
						got,
					)
				}
				if !fits && !strings.HasSuffix(got, Ellipsis) {
					t.Errorf("method %d: Cut(%q, %d) = %q, want it to end in the ellipsis",
						c.Method, text, width, got)
				}
			}
		}
	}
	// Grapheme width fits the family in 4 cells; wcwidth must cut it.
	if got := wcCells.Cut(family, 4); got != Ellipsis {
		t.Errorf("wcwidth Cut(family, 4) = %q, want only the ellipsis", got)
	}
	if got := graphemeCells.Cut(family, 4); got != family {
		t.Errorf("grapheme Cut(family, 4) = %q, want the family whole", got)
	}
}

func TestReportSwitchesLikeTheRenderer(t *testing.T) {
	cases := []struct {
		mode  ansi.Mode
		value ansi.ModeSetting
		want  ansi.Method
	}{
		{ansi.ModeUnicodeCore, ansi.ModeSet, ansi.GraphemeWidth},
		{ansi.ModeUnicodeCore, ansi.ModeReset, ansi.GraphemeWidth},
		{ansi.ModeUnicodeCore, ansi.ModePermanentlySet, ansi.GraphemeWidth},
		{ansi.ModeUnicodeCore, ansi.ModePermanentlyReset, ansi.WcWidth},
		{ansi.ModeUnicodeCore, ansi.ModeNotRecognized, ansi.WcWidth},
		{ansi.ModeSynchronizedOutput, ansi.ModeSet, ansi.WcWidth},
	}
	for _, tc := range cases {
		if got := (Cells{}).Report(tc.mode, tc.value).Method; got != tc.want {
			t.Errorf("Report(%v, %v) method = %d, want %d", tc.mode, tc.value, got, tc.want)
		}
	}
}

func TestBoldAndPlain(t *testing.T) {
	plain := protocol.Line{span("Open pull requests", protocol.ColorDefault)}
	if !Plain(plain) {
		t.Error("Plain() = false for a default, non-bold line")
	}
	bold := Bold(plain)
	if Plain(bold) || !bold[0].Bold {
		t.Errorf("Bold() = %+v, want every span bold", bold)
	}
	if plain[0].Bold {
		t.Error("Bold() changed its input")
	}
	if Plain(protocol.Line{span("x", protocol.ColorGreen)}) {
		t.Error("Plain() = true for a colored span")
	}
}
