// Package render turns protocol lines into styled terminal strings, for every
// cards mode. Colors are the 16 terminal colors, so the user's theme picks
// the shades, and dim is the faint attribute rather than a hue. Every width
// here is in terminal cells, counted by Cells the way the Bubble Tea
// renderer counts them: a wide rune counts as two, escape codes count as
// none, and a line cut to fit ends in Ellipsis.
//
// A CardStyle sets each card apart from the next, the same way in every
// mode, and says how many rows and cells that costs a mode's layout.
package render

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mad01/thismoon/tools/cards/internal/protocol"
)

// Ellipsis ends a line that was cut to fit.
const Ellipsis = "…"

// hues maps each protocol color with a hue to its terminal color index.
// ColorDefault and ColorDim have none.
var hues = map[protocol.Color]color.Color{
	protocol.ColorRed:     lipgloss.Red,
	protocol.ColorGreen:   lipgloss.Green,
	protocol.ColorYellow:  lipgloss.Yellow,
	protocol.ColorBlue:    lipgloss.Blue,
	protocol.ColorMagenta: lipgloss.Magenta,
	protocol.ColorCyan:    lipgloss.Cyan,
	protocol.ColorWhite:   lipgloss.White,
}

// Cells counts and cuts text in terminal cells by one width method, which
// must be the one the Bubble Tea renderer uses: a row cut to fit by another
// method can be wider for the renderer, and then its tail, an Ellipsis or a
// grid column to its right, falls off the screen. The two methods differ on
// a ZWJ emoji sequence, which ansi.WcWidth counts code point by code point
// and ansi.GraphemeWidth as one wide cluster.
//
// The renderer starts on ansi.WcWidth and moves to ansi.GraphemeWidth when
// the terminal answers its query for Unicode core mode (2027); Report makes
// the same move. The zero Cells counts with ansi.WcWidth.
type Cells struct {
	Method ansi.Method
}

// Report returns the cells to count with once the terminal reports value
// for mode, as a tea.ModeReportMsg carries them. It switches to
// ansi.GraphemeWidth on the reports for mode 2027 that make Bubble Tea's
// renderer switch (set, reset or permanently set) and returns c unchanged
// for any other report.
func (c Cells) Report(mode ansi.Mode, value ansi.ModeSetting) Cells {
	if mode != ansi.ModeUnicodeCore {
		return c
	}
	switch value {
	case ansi.ModeSet, ansi.ModeReset, ansi.ModePermanentlySet:
		return Cells{Method: ansi.GraphemeWidth}
	}
	return c
}

// Width returns how many cells s takes; escape codes take none.
func (c Cells) Width(s string) int { return c.Method.StringWidth(s) }

// Line renders l as one row at most width cells wide. A span in ColorDefault
// takes tone; a span with a color of its own keeps it. A line wider than
// width is cut and ends in Ellipsis.
func (c Cells) Line(l protocol.Line, tone protocol.Color, width int) string {
	var b strings.Builder
	for _, s := range l {
		color := s.Color
		if color == protocol.ColorDefault {
			color = tone
		}
		b.WriteString(Paint(s.Text, color, s.Bold))
	}
	return c.Cut(b.String(), width)
}

// Paint styles text in color c, bold when bold is set. ColorDefault leaves
// the color to the terminal; ColorDim makes the text faint.
func Paint(text string, c protocol.Color, bold bool) string {
	if text == "" {
		return ""
	}
	style := lipgloss.NewStyle().Bold(bold)
	if c == protocol.ColorDim {
		style = style.Faint(true)
	} else if hue, ok := hues[c]; ok {
		style = style.Foreground(hue)
	}
	return style.Render(text)
}

// Cut shortens s, which may hold escape codes, to at most width cells. A cut
// string ends in Ellipsis; a width of zero or less gives "".
func (c Cells) Cut(s string, width int) string {
	switch {
	case width <= 0:
		return ""
	case c.Width(s) <= width:
		return s
	}
	// ansi's Truncate returns s whole when it fits by grapheme width,
	// whatever method it was given, so a string wider only by wcwidth would
	// stay uncut. Spaces past the end make s too wide by either method; the
	// cut falls inside s, so none of them is kept.
	return c.Method.Truncate(s+strings.Repeat(" ", width+1), width, Ellipsis)
}

// Pad fills s with spaces on the right to width cells. A string already that
// wide or wider comes back unchanged.
func (c Cells) Pad(s string, width int) string {
	if gap := width - c.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// Bold returns a copy of l with every span bold.
func Bold(l protocol.Line) protocol.Line {
	out := make(protocol.Line, len(l))
	for i, s := range l {
		s.Bold = true
		out[i] = s
	}
	return out
}

// Plain reports whether l carries no styling of its own: no span has a
// color or is bold.
func Plain(l protocol.Line) bool {
	for _, s := range l {
		if s.Color != protocol.ColorDefault || s.Bold {
			return false
		}
	}
	return true
}
