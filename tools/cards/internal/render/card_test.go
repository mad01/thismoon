package render

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

var allStyles = []CardStyle{CardNone, CardSpace, CardLine, CardBox}

func TestCardStyleText(t *testing.T) {
	want := []string{"none", "space", "line", "box"}
	if got := CardStyleNames(); !slices.Equal(got, want) {
		t.Errorf("CardStyleNames() = %q, want %q", got, want)
	}
	for _, s := range allStyles {
		text, err := s.MarshalText()
		if err != nil {
			t.Fatalf("%v.MarshalText() error = %v", s, err)
		}
		var back CardStyle
		if err := back.UnmarshalText(text); err != nil || back != s {
			t.Errorf("UnmarshalText(%q) = %v, %v, want %v", text, back, err, s)
		}
	}

	back := CardBox
	err := back.UnmarshalText([]byte("boxed"))
	if err == nil || !strings.Contains(err.Error(), "none, space, line, box") {
		t.Errorf("UnmarshalText(boxed) error = %v, want one naming the styles", err)
	}
	if back != CardBox {
		t.Errorf("UnmarshalText(boxed) set the style to %v, want it left as box", back)
	}
}

func TestDrawTakesRowsAndFitsWidth(t *testing.T) {
	text := []string{"acme/api#12 fix the login redirect", "alice"}
	for _, s := range allStyles {
		for width := range 12 {
			rows, card := s.Draw(Cells{}, text, CardBounds{Width: width, Lead: 3, Lines: 3})
			if n := len(rows); n != s.Rows(3) || card > n {
				t.Errorf("%v width %d: drew %d rows, %d of them the card, want Rows(3) = %d",
					s, width, n, card, s.Rows(3))
			}
			for i, r := range rows {
				if w := ansi.StringWidthWc(r); w > width {
					t.Errorf("%v width %d: row %d = %q is %d cells wide", s, width, i, r, w)
				}
			}
		}
	}
}

func TestDrawFitsEmojiByEitherMethod(t *testing.T) {
	text := []string{"x" + family + flag + family, flag + flag + flag + flag}
	for _, c := range []Cells{wcCells, graphemeCells} {
		for _, s := range allStyles {
			for width := range 14 {
				t.Run(fmt.Sprintf("method %d %v width %d", c.Method, s, width), func(t *testing.T) {
					inner := s.TextWidth(width)
					cut := []string{c.Cut(text[0], inner), c.Cut(text[1], inner)}
					rows, _ := s.Draw(c, cut, CardBounds{Width: width, Lines: 2})
					for i, r := range rows {
						if w := c.Width(r); w > width {
							t.Errorf("row %d = %q is %d cells wide", i, r, w)
						}
					}
				})
			}
		}
	}
}

func TestTextRows(t *testing.T) {
	text := []string{"ab", "cd"}
	for _, s := range allStyles {
		rows, _ := s.Draw(Cells{}, text, CardBounds{Width: 8, Lines: 2})
		start, end := s.TextStart(), s.TextEnd(len(text))
		if end-start != len(text) || end > len(rows) {
			t.Fatalf("%v: text rows %d to %d of %d rows, want the 2 lines of text", s, start,
				end, len(rows))
		}
		for i, want := range text {
			if got := ansi.Strip(rows[start+i]); !strings.Contains(got, want) {
				t.Errorf("%v: row %d = %q, want text %q", s, start+i, got, want)
			}
		}
	}
}

func TestDrawRows(t *testing.T) {
	text := []string{"10:15  ab", "       cd"}
	cases := []struct {
		style CardStyle
		want  []string
		card  int
	}{
		{CardNone, []string{"10:15  ab", "       cd", ""}, 2},
		{CardSpace, []string{"10:15  ab", "       cd", ""}, 2},
		{CardLine, []string{"10:15  ab", "       cd", "", "       ─────"}, 2},
		{CardBox, []string{
			"╭──────────╮",
			"│ 10:15  … │",
			"│        … │",
			"│          │",
			"╰──────────╯",
		}, 5},
	}
	for _, tc := range cases {
		rows, card := tc.style.Draw(Cells{}, text, CardBounds{Width: 12, Lead: 7, Lines: 3})
		for i, r := range rows {
			rows[i] = ansi.Strip(r)
		}
		if !slices.Equal(rows, tc.want) || card != tc.card {
			t.Errorf("%v: rows = %q, card %d, want %q, card %d",
				tc.style, rows, card, tc.want, tc.card)
		}
	}
}
