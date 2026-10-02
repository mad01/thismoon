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

// minInkContrast is the WCAG 2.x floor for body text, the bar the on-<role>
// ink clears on its fill: the danger button and the toasts set on-<role> on
// the matching semantic colour. The build picks white or the mode's deepest
// ink, text-1 in light and the page background in dark, whichever reads
// better on the fill (MAD-376, MAD-378).
const minInkContrast = 4.5

// darkInkFloorExceptions names the dark fills whose ink cannot reach 4.5 once
// the fill itself clears 3.0 as text on the chip, keyed family/role.
// Solarized's base02 chip has a relative luminance of 0.031, so a colour
// needs at least 3.0 * (0.031 + 0.05) - 0.05 = 0.193 to read on it, while
// white ink at 4.5 needs a fill at or under 1.05 / 4.5 - 0.05 = 0.183; the
// two bands do not meet. The family's dark ink (base03, luminance 0.020)
// needs a fill at 4.5 * (0.020 + 0.05) - 0.05 = 0.265. Blue is lifted to
// reach it, while red, orange, and violet would have to leave the published
// hues, so these three keep white ink at the floors measured on the lifted
// values. On violet base03 would read 3.93 by WCAG, but far worse by APCA,
// so the family pins white there to match red and orange.
var darkInkFloorExceptions = map[string]float64{
	"solarized/red":    3.9, // #e04c49, measured 3.96; base03 reads 3.79
	"solarized/amber":  3.9, // #dd5218, measured 3.97; base03 reads 3.79
	"solarized/purple": 3.8, // #777cc8, measured 3.82; base03 reads 3.93
}

// lightInkFloorExceptions names the light fills no ink reaches at 4.5, keyed
// family/role. White ink at 4.5 needs a fill at or under luminance 0.183, and
// a family's text ink needs one at or above 4.5 * (ink + 0.05) - 0.05: 0.54
// for Latte's text (0.081), 0.37 for One's mono-1 (0.043), 0.83 for
// solarized's base01 (0.145), and 0.76 for Day's blue ink (0.129). Every fill
// below sits between the two bands. MAD-378 moved seventeen other light fills
// into one band or the other, hue held, by one to seven points of lightness
// (a Lab distance under 8.5). For these nine the nearest fill in either band
// is six to thirteen points away, a different colour at a Lab distance of
// 9.9 to 22, except One's green, which would reach mono-1 ink nine points
// lighter (8.9) but then read under 3.0 as text on paper. So they keep the
// published value and the better of the two inks, at the floors measured.
var lightInkFloorExceptions = map[string]float64{
	"catppuccin/green":  3.3, // #40a02b, white 3.34; the text ink reads 2.39
	"catppuccin/amber":  2.9, // #fe640b, white 2.98; the text ink reads 2.68
	"catppuccin/yellow": 3.0, // #df8e1d, the text ink 3.05; white reads 2.62
	"one/red":           3.6, // #e45649, white 3.67; mono-1 reads 3.09
	"one/green":         3.5, // #50a14f, mono-1 3.54; white reads 3.21
	"one/yellow":        3.5, // #c18401, mono-1 3.55; white reads 3.20
	"solarized/green":   3.2, // #859900, white 3.20; base01 reads 1.68
	"solarized/yellow":  3.2, // #b58900, white 3.21; base01 reads 1.68
	"tokyo-night/red":   3.8, // #f52a65, white 3.89; the blue ink reads 1.51
}

var inkRoles = []string{"red", "green", "amber", "yellow", "blue", "purple"}

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

// Every dark variant's on-<role> ink, yellow included, reads as body text on
// its fill, with darkInkFloorExceptions as the only carve-out.
func TestDarkInkReadsOnFill(t *testing.T) {
	inkReadsOnFill(t, "dark", darkInkFloorExceptions)
}

// Every light variant's on-<role> ink reads as body text on its fill too,
// with lightInkFloorExceptions as the only carve-out (MAD-378).
func TestLightInkReadsOnFill(t *testing.T) {
	inkReadsOnFill(t, "light", lightInkFloorExceptions)
}

// inkReadsOnFill holds every family's six on-<role> inks in one mode to 4.5
// on their fills. An exception whose pair now clears the floor is stale and
// fails too, and so does a key the loop never visits, so the map can neither
// outlive its reason nor hide a misspelt family or role.
func inkReadsOnFill(t *testing.T, mode string, exceptions map[string]float64) {
	t.Helper()
	visited := map[string]bool{}
	for _, f := range webkit.Themes() {
		v := f.Light
		if mode == "dark" {
			v = f.Dark
		}
		if v == nil {
			continue
		}
		for _, role := range inkRoles {
			fill, ink := v.Roles[role], v.Roles["on-"+role]
			got := contrast(t, ink, fill)
			floor := minInkContrast
			key := f.Name + "/" + role
			if exception, ok := exceptions[key]; ok {
				visited[key] = true
				if got >= minInkContrast {
					t.Errorf("%s %s: --on-%s reads %.2f on --%s; drop its exception",
						f.Name, mode, role, got, role)
				}
				floor = exception
			}
			if got < floor {
				t.Errorf("%s %s: --on-%s %s on --%s %s = %.2f, want >= %.1f",
					f.Name, mode, role, ink, role, fill, got, floor)
			}
		}
	}
	for key := range exceptions {
		if !visited[key] {
			t.Errorf("%s exceptions[%q] names no family and ink role", mode, key)
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
	// The linear-segment cutoff is 0.04045, the value the WCAG 2.x errata
	// settled on; the 0.03928 the original text carried gives the same
	// result for every 8-bit channel.
	channel := func(c uint64) float64 {
		s := float64(c) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(n>>16&255) + 0.7152*channel(n>>8&255) + 0.0722*channel(n&255)
}
