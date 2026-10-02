package webkit_test

import (
	"math"
	"strconv"
	"strings"
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
// the matching semantic colour. The build picks white or the variant's page
// background for a dark fill, whichever reads better on it (MAD-376).
const minInkContrast = 4.5

// inkFloorExceptions names the dark fills whose ink cannot reach 4.5 once the
// fill itself clears 3.0 as text on the chip, keyed family/role. Solarized's
// base02 chip has a relative luminance of 0.031, so a colour needs at least
// 3.0 * (0.031 + 0.05) - 0.05 = 0.193 to read on it, while white ink at 4.5
// needs a fill at or under 1.05 / 4.5 - 0.05 = 0.183; the two bands do not
// meet. The family's dark ink (base03, luminance 0.020) needs a fill at
// 4.5 * (0.020 + 0.05) - 0.05 = 0.265. Blue is lifted to reach it, while
// red, orange, and violet would have to leave the published hues, so these
// three hold the floors measured on the lifted values with the better ink.
var inkFloorExceptions = map[string]float64{
	"solarized/red":    3.9, // #e04c49 under white, measured 3.96; base03 reads 3.79
	"solarized/amber":  3.9, // #dd5218 under white, measured 3.97; base03 reads 3.79
	"solarized/purple": 3.9, // #777cc8 under base03, measured 3.93; white reads 3.82
}

// lightInks are the on-<colour> inks every light variant compiled to before
// the ink rule changed, in the order red, green, amber, yellow, blue, purple.
// MAD-376 moved dark only: where the rule would now pick the text ink in
// light, the family pins white under its light overrides, so these hold.
var lightInks = map[string][6]string{
	"default":     {"#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF"},
	"catppuccin":  {"#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF"},
	"gruvbox":     {"#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF"},
	"nord":        {"#FFFFFF", "#2e3440", "#FFFFFF", "#2e3440", "#FFFFFF", "#FFFFFF"},
	"one":         {"#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF"},
	"rose-pine":   {"#FFFFFF", "#FFFFFF", "#464261", "#464261", "#FFFFFF", "#FFFFFF"},
	"solarized":   {"#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF"},
	"tokyo-night": {"#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF", "#FFFFFF"},
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

// Every dark variant's on-<role> ink reads as body text on its fill, with the
// exception map as the only carve-out. An exception whose pair now clears
// the floor is stale and fails too, so the map never outlives its reason.
func TestDarkInkReadsOnFill(t *testing.T) {
	for _, f := range webkit.Themes() {
		if f.Dark == nil {
			continue
		}
		for _, role := range textRoles {
			fill, ink := f.Dark.Roles[role], f.Dark.Roles["on-"+role]
			got := contrast(t, ink, fill)
			floor := minInkContrast
			if exception, ok := inkFloorExceptions[f.Name+"/"+role]; ok {
				if got >= minInkContrast {
					t.Errorf("%s dark: --on-%s reads %.2f on --%s; drop its exception",
						f.Name, role, got, role)
				}
				floor = exception
			}
			if got < floor {
				t.Errorf("%s dark: --on-%s %s on --%s %s = %.2f, want >= %.1f",
					f.Name, role, ink, role, fill, got, floor)
			}
		}
	}
}

// The light inks are byte for byte what they were before the ink rule
// changed; a family the table does not know fails until its values are added.
func TestLightInksUnchanged(t *testing.T) {
	for _, f := range webkit.Themes() {
		if f.Light == nil {
			continue
		}
		want, ok := lightInks[f.Name]
		if !ok {
			t.Errorf("%s: no pinned light inks; add its compiled values", f.Name)
			continue
		}
		for i, role := range inkRoles {
			if got := f.Light.Roles["on-"+role]; !strings.EqualFold(got, want[i]) {
				t.Errorf("%s light: --on-%s = %s, want %s", f.Name, role, got, want[i])
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
