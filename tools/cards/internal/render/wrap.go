package render

import (
	"strings"

	"github.com/mad01/thismoon/tools/cards/internal/protocol"
)

// WrapLast returns lines with the last line given a second row when it is
// wider than width cells as c counts them. The line breaks at the last space
// that keeps the first row within width, or at width when there is none, and
// the spaces at the break are dropped, so the second row starts on a word.
// The second row keeps all the rest; drawing it cuts it to fit, ending in
// Ellipsis. A span the break falls in keeps its color and bold on both rows.
// A line wider only by spaces at its end loses them and takes no second row.
//
// When the last line fits, or the part of it that fits in width is only
// spaces or nothing, lines comes back as it is. WrapLast never changes lines.
func (c Cells) WrapLast(lines []protocol.Line, width int) []protocol.Line {
	if len(lines) == 0 || width <= 0 {
		return lines
	}
	last := lines[len(lines)-1]
	text := lineText(last)
	if c.Width(text) <= width {
		return lines
	}
	end, start := c.breakAt(text, width)
	if end == 0 {
		return lines
	}
	out := make([]protocol.Line, 0, len(lines)+1)
	out = append(out, lines[:len(lines)-1]...)
	out = append(out, sliceLine(last, 0, end))
	if start < len(text) {
		out = append(out, sliceLine(last, start, len(text)))
	}
	return out
}

// breakAt finds where text, wider than width cells, breaks into two rows: the
// first row ends at byte end, the second starts at byte start, and the spaces
// between them are dropped. end is 0 when the part of text that fits in width
// is empty or only spaces, as a first row would show nothing.
func (c Cells) breakAt(text string, width int) (end, start int) {
	fit := len(c.truncate(text, width, ""))
	// A space right after the part that fits ends a first row that fills
	// width exactly, so it counts too. A space byte is never part of a
	// multibyte rune, so the window may end inside one.
	window := text[:min(fit+1, len(text))]
	if space := strings.LastIndexByte(window, ' '); space >= 0 {
		end = len(strings.TrimRight(text[:space], " "))
		start = len(text) - len(strings.TrimLeft(text[space:], " "))
		if end > 0 {
			return end, start
		}
	}
	if strings.TrimLeft(text[:fit], " ") == "" {
		return 0, 0
	}
	return fit, fit
}

// lineText returns the text of l's spans, joined.
func lineText(l protocol.Line) string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

// sliceLine returns the part of l from byte from up to byte to of its text,
// as spans that keep their style. A span left with no text is dropped.
func sliceLine(l protocol.Line, from, to int) protocol.Line {
	var out protocol.Line
	at := 0
	for _, s := range l {
		next := at + len(s.Text)
		if lo, hi := max(from, at), min(to, next); lo < hi {
			s.Text = s.Text[lo-at : hi-at]
			out = append(out, s)
		}
		at = next
	}
	return out
}
