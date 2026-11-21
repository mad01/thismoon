package render

import (
	"fmt"
	"strings"

	"github.com/mad01/thismoon/tools/cards/internal/protocol"
)

// CardStyle is how a mode sets each card apart from the next. The zero
// CardStyle is CardNone.
type CardStyle int

const (
	// CardNone draws the cards with nothing between them.
	CardNone CardStyle = iota
	// CardSpace draws one blank row between two cards.
	CardSpace
	// CardLine draws a dim rule row under each card.
	CardLine
	// CardBox draws a dim rounded border around each card, with one column
	// of padding inside it on each side.
	CardBox
)

// cardStyle is what a style adds to a card. Every row and cell count a
// mode needs for its layout comes from here.
type cardStyle struct {
	name string
	// border is the border rows above and below the text.
	border int
	// rule is the rule rows under the card.
	rule int
	// gap is the blank rows between two cards.
	gap int
	// beside is the cells the style takes from the text width: a box's two
	// borders and their padding.
	beside int
}

var cardStyles = [...]cardStyle{
	CardNone:  {name: "none"},
	CardSpace: {name: "space", gap: 1},
	CardLine:  {name: "line", rule: 1},
	CardBox:   {name: "box", border: 2, beside: 4},
}

// The box border, in rounded corners.
const (
	boxTopLeft     = "╭"
	boxTopRight    = "╮"
	boxBottomLeft  = "╰"
	boxBottomRight = "╯"
	boxSide        = "│"
	// ruleRune draws the box's top and bottom edges and the rule row.
	ruleRune = "─"
)

// CardStyleNames returns the style names in order: none, space, line, box.
func CardStyleNames() []string {
	names := make([]string, len(cardStyles))
	for i, s := range cardStyles {
		names[i] = s.name
	}
	return names
}

// spec returns what s adds to a card. A CardStyle outside the constants
// draws as CardNone.
func (s CardStyle) spec() cardStyle {
	if s < 0 || int(s) >= len(cardStyles) {
		return cardStyles[CardNone]
	}
	return cardStyles[s]
}

// String returns the style's name.
func (s CardStyle) String() string { return s.spec().name }

// MarshalText returns the style's name, so a CardStyle can back a flag.
func (s CardStyle) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText sets s to the style named text, or returns an error naming
// the styles there are.
func (s *CardStyle) UnmarshalText(text []byte) error {
	for i, spec := range cardStyles {
		if spec.name == string(text) {
			*s = CardStyle(i)
			return nil
		}
	}
	return fmt.Errorf("render: card style %q is not one of %s",
		text, strings.Join(CardStyleNames(), ", "))
}

// Rows returns the screen rows a card with lines rows of text takes: the
// text, the border and the rule, but not the gap to the next card.
func (s CardStyle) Rows(lines int) int {
	spec := s.spec()
	return lines + spec.border + spec.rule
}

// TextStart returns the first row of text in a card, counted from the
// card's first row. All that comes before it is the box's top border.
func (s CardStyle) TextStart() int {
	// A box has half its border rows above the text, half below.
	return s.spec().border / 2
}

// TextEnd returns the row just below the last row of text in a card with
// lines rows of text, counted from the card's first row. All that follows it
// is the box's bottom border or the rule.
func (s CardStyle) TextEnd(lines int) int { return s.TextStart() + lines }

// Gap returns the blank rows between two cards.
func (s CardStyle) Gap() int { return s.spec().gap }

// TextWidth returns the cells left for a card's text in a card width cells
// wide, never below zero.
func (s CardStyle) TextWidth(width int) int { return max(width-s.spec().beside, 0) }

// CardBounds is the space Draw fills with one card.
type CardBounds struct {
	// Width is the cells the card takes.
	Width int
	// Lead is the cells at the start of each text row that come before the
	// card's own text, such as feed mode's time column. A rule leaves them
	// blank; a box takes them in.
	Lead int
	// Lines is the rows of text to make room for, so cards side by side end
	// on the same row. Fewer than the card's text means the card's text.
	Lines int
}

// Draw draws a card from text, its rows of text already cut to TextWidth,
// in Rows(Lines) rows of at most Width cells as c counts them. The first card
// rows are the card itself, border included, for a mode that marks them. A
// box grows blank rows inside it to fill Lines; without one, blank rows
// follow the text, and the rule comes last.
func (s CardStyle) Draw(c Cells, text []string, b CardBounds) (rows []string, card int) {
	spec := s.spec()
	lines := max(b.Lines, len(text))
	rows = make([]string, 0, s.Rows(lines))
	if spec.border > 0 {
		rows = append(rows, boxRows(c, text, lines, b.Width)...)
		card = len(rows)
	} else {
		rows = append(rows, text...)
		card = len(text)
		for len(rows) < lines {
			rows = append(rows, "")
		}
	}
	for range spec.rule {
		rows = append(rows, rule(b))
	}
	for i, r := range rows {
		rows[i] = c.Cut(r, b.Width)
	}
	return rows, card
}

// boxRows draws text in a dim box width cells wide, with blank rows below
// the text to make lines rows inside the border.
func boxRows(c Cells, text []string, lines, width int) []string {
	inner := CardBox.TextWidth(width)
	edge := strings.Repeat(ruleRune, max(width-2, 0))
	side := Paint(boxSide, protocol.ColorDim, false)
	rows := make([]string, 0, lines+2)
	rows = append(rows, Paint(boxTopLeft+edge+boxTopRight, protocol.ColorDim, false))
	for i := range lines {
		t := ""
		if i < len(text) {
			t = text[i]
		}
		rows = append(rows, side+" "+c.Pad(c.Cut(t, inner), inner)+" "+side)
	}
	return append(rows, Paint(boxBottomLeft+edge+boxBottomRight, protocol.ColorDim, false))
}

// rule draws a dim rule row under the text of a card in b, after its lead.
func rule(b CardBounds) string {
	lead := max(b.Lead, 0)
	line := strings.Repeat(ruleRune, max(b.Width-lead, 0))
	return strings.Repeat(" ", lead) + Paint(line, protocol.ColorDim, false)
}
