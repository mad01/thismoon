package webkit_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/mad01/thismoon/webkit"
)

// textRoles are the semantic colours consumers set as text: csl paints its
// query tokens with them and present its callout and panel accents. In dark
// mode that text sits on the page, the paper, or a chip, so every family's
// dark variant keeps each colour readable on all three surfaces.
var (
	textRoles = []string{"red", "green", "amber", "blue", "purple"}
	surfaces  = []string{"bg", "paper", "chip-bg"}
)

// minTextContrast is the WCAG 2.x floor for large text and interface parts,
// the bar a semantic colour used as text clears on each surface (MAD-372).
const minTextContrast = 3.0

func TestDarkSemanticColoursReadAsText(t *testing.T) {
	for _, f := range webkit.Themes() {
		if f.Dark == nil {
			continue
		}
		for _, role := range textRoles {
			for _, surface := range surfaces {
				ink, bg := f.Dark.Roles[role], f.Dark.Roles[surface]
				if got := contrast(t, ink, bg); got < minTextContrast {
					t.Errorf("%s dark: --%s %s on --%s %s = %.2f, want >= %.1f",
						f.Name, role, ink, surface, bg, got, minTextContrast)
				}
			}
		}
	}
}

// The contrast maths follows WCAG 2.x: black on white is the 21:1 ceiling, a
// colour on itself is 1:1, and the ratio reads the same from either side.
func TestContrastRatio(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want float64
	}{
		{"#000000", "#FFFFFF", 21},
		{"#FFFFFF", "#000000", 21},
		{"#808080", "#808080", 1},
		{"#777777", "#FFFFFF", 4.48},
	} {
		if got := contrast(t, tc.a, tc.b); math.Abs(got-tc.want) > 0.01 {
			t.Errorf("contrast(%s, %s) = %.2f, want %.2f", tc.a, tc.b, got, tc.want)
		}
	}
}

// contrast is the WCAG 2.x contrast ratio between two hex colours, 1 to 21.
func contrast(t *testing.T, a, b string) float64 {
	t.Helper()
	la, lb := luminance(t, a), luminance(t, b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// luminance is the WCAG 2.x relative luminance of a 6-digit hex colour, 0
// for black to 1 for white.
func luminance(t *testing.T, hex string) float64 {
	t.Helper()
	if !hexRE.MatchString(hex) {
		t.Fatalf("not a 6-digit hex colour: %q", hex)
	}
	n, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		t.Fatalf("parse %q: %v", hex, err)
	}
	channel := func(c uint64) float64 {
		s := float64(c) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(n>>16&255) + 0.7152*channel(n>>8&255) + 0.0722*channel(n&255)
}
