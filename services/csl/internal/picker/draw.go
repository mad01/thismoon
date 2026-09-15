package picker

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	runewidth "github.com/mattn/go-runewidth"
)

// Screen columns: the cursor marker owns the first two, item text the rest.
const textColumn = 2

// draw repaints the whole screen bottom-up: prompt on the last row, the
// matched/total count above it, then item lines with the best match closest to
// the prompt.
func (p *picker) draw() {
	p.screen.Clear()
	w, h := p.screen.Size()
	if w < textColumn+2 || h < 2 {
		p.screen.Show()
		return
	}
	p.drawPrompt(h - 1)
	p.drawCount(h - 2)
	p.drawItems(w, h)
	p.screen.Show()
}

func (p *picker) drawPrompt(y int) {
	style := p.paint(tcell.ColorBlue)
	p.screen.SetContent(0, y, '>', nil, style)
	p.screen.SetContent(1, y, ' ', nil, style)
	x, cursorX := textColumn, textColumn
	for i, r := range p.query {
		if i == p.pos {
			cursorX = x
		}
		p.screen.SetContent(x, y, r, nil, p.paint(tcell.ColorDefault).Bold(true))
		x += runewidth.RuneWidth(r)
	}
	if p.pos >= len(p.query) {
		cursorX = x
	}
	p.screen.ShowCursor(cursorX, y)
}

func (p *picker) drawCount(y int) {
	style := p.paint(tcell.ColorYellow)
	count := fmt.Sprintf("%d/%d", len(p.matched), len(p.items))
	for i, r := range count {
		p.screen.SetContent(textColumn+i, y, r, nil, style)
	}
}

func (p *picker) drawItems(w, h int) {
	rows := p.itemRows()
	for row := 0; row < rows && p.offset+row < len(p.matched); row++ {
		m := p.matched[p.offset+row]
		p.drawItem(m, h-3-row, w, p.offset+row == p.cursor)
	}
}

// drawItem draws one item line, its segment colors intact, with the runes the
// query matched picked out. The selected row's band runs the full width so
// the row reads as one, not as text with a shaded backdrop.
func (p *picker) drawItem(m match, y, maxWidth int, cursor bool) {
	if cursor {
		style := p.styleFor(Default, false, true).Foreground(tcell.ColorRed)
		p.screen.SetContent(0, y, '>', nil, style)
		p.screen.SetContent(1, y, ' ', nil, style)
	}
	x := p.drawSegments(m, y, maxWidth, cursor)
	if !cursor {
		return
	}
	pad := p.styleFor(Default, false, true)
	for ; x < maxWidth; x++ {
		p.screen.SetContent(x, y, ' ', nil, pad)
	}
}

// drawSegments draws the item's text from textColumn and returns the column
// after the last cell it painted. A line wider than the screen ends in "..".
func (p *picker) drawSegments(m match, y, maxWidth int, cursor bool) int {
	hits := p.hitRunes(m)
	x, i := textColumn, 0
	for _, seg := range p.items[m.idx] {
		for _, r := range seg.Text {
			style := p.styleFor(seg.Color, i < len(hits) && hits[i], cursor)
			rw := runewidth.RuneWidth(r)
			if x+rw+2 > maxWidth {
				p.screen.SetContent(x, y, '.', nil, style)
				p.screen.SetContent(x+1, y, '.', nil, style)
				return x + 2
			}
			p.screen.SetContent(x, y, r, nil, style)
			x += rw
			i++
		}
	}
	return x
}

// hitRunes marks every rune of the item that any query term matched.
func (p *picker) hitRunes(m match) []bool {
	plain := p.plain[m.idx]
	hits := make([]bool, utf8.RuneCountInString(plain))
	for i, term := range p.terms {
		if i >= len(m.pos) {
			break
		}
		for j, hit := range matchedRunes([]rune(term), plain, m.pos[i]) {
			hits[j] = hits[j] || hit
		}
	}
	return hits
}

// matchedRunes marks the runes of a line that the query matched, walking it
// the way go-fuzzyfinder does: each query rune in turn, only inside the
// matched range, case-insensitively.
func matchedRunes(query []rune, plain string, pos [2]int) []bool {
	runes := []rune(plain)
	hits := make([]bool, len(runes))
	if pos[0] == -1 && pos[1] == -1 {
		return hits
	}
	posIdx := 0
	for i, r := range runes {
		if posIdx >= len(query) {
			break
		}
		if i < pos[0] || i > pos[1] {
			continue
		}
		if unicode.ToLower(query[posIdx]) == unicode.ToLower(r) {
			hits[i] = true
			posIdx++
		}
	}
	return hits
}

// styleFor combines a segment's own color with the cursor band and the match
// highlight. A matched rune has to stand out even inside a colored segment,
// so it wins over the segment color. Dim text would sink into the band, so
// there it is lifted to the default foreground; colored segments keep their
// own palette color. Under NO_COLOR the band is reverse video, the one row
// highlight every terminal renders without a palette.
func (p *picker) styleFor(c Color, hit, cursor bool) tcell.Style {
	style := p.paint(tcellColor(c))
	if cursor {
		style = style.Bold(true)
		switch {
		case p.noColor:
			style = style.Reverse(true)
		case c == Dim:
			style = style.Background(p.band).Foreground(tcell.ColorDefault)
		default:
			style = style.Background(p.band)
		}
	}
	if hit {
		if p.noColor {
			style = style.Bold(true)
		} else {
			style = style.Foreground(tcell.ColorGreen)
		}
	}
	return style
}

// paint builds a style on the terminal's own background, dropping the
// foreground when NO_COLOR asks for plain output.
func (p *picker) paint(fg tcell.Color) tcell.Style {
	if p.noColor {
		fg = tcell.ColorDefault
	}
	return tcell.StyleDefault.Foreground(fg).Background(tcell.ColorDefault)
}

// tcellColor maps a segment color onto the terminal palette.
func tcellColor(c Color) tcell.Color {
	switch c {
	case Cyan:
		return tcell.ColorAqua
	case Magenta:
		return tcell.ColorFuchsia
	case Blue:
		return tcell.ColorBlue
	case Dim:
		return tcell.ColorGray
	default:
		return tcell.ColorDefault
	}
}
