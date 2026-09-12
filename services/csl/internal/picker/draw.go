package picker

import (
	"fmt"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/ktr0731/go-fuzzyfinder/matching"
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
	x := textColumn
	for _, r := range p.query {
		p.screen.SetContent(x, y, r, nil, p.paint(tcell.ColorDefault).Bold(true))
		x += runewidth.RuneWidth(r)
	}
	p.screen.ShowCursor(x, y)
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
// query matched picked out.
func (p *picker) drawItem(m matching.Matched, y, maxWidth int, cursor bool) {
	if cursor {
		style := p.paint(tcell.ColorRed).Background(tcell.ColorBlack).Bold(true)
		p.screen.SetContent(0, y, '>', nil, style)
		p.screen.SetContent(1, y, ' ', nil, style)
	}
	hits := matchedRunes(p.query, p.plain[m.Idx], m.Pos)
	x, i := textColumn, 0
	for _, seg := range p.items[m.Idx] {
		for _, r := range seg.Text {
			style := p.styleFor(seg.Color, i < len(hits) && hits[i], cursor)
			rw := runewidth.RuneWidth(r)
			if x+rw+2 > maxWidth {
				p.screen.SetContent(x, y, '.', nil, style)
				p.screen.SetContent(x+1, y, '.', nil, style)
				return
			}
			p.screen.SetContent(x, y, r, nil, style)
			x += rw
			i++
		}
	}
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

// styleFor combines a segment's own color with the match and cursor
// highlights. A matched rune has to stand out even inside a colored segment,
// so it wins over the segment color.
func (p *picker) styleFor(c Color, hit, cursor bool) tcell.Style {
	style := p.paint(tcellColor(c))
	if hit {
		if p.noColor {
			style = style.Bold(true)
		} else {
			style = style.Foreground(tcell.ColorGreen)
		}
	}
	if cursor {
		style = style.Background(tcell.ColorBlack).Bold(true)
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
