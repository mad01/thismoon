package webkit_test

import (
	"fmt"
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
// below sits between the two bands. MAD-378 moved eighteen other light fills
// into one band or the other, hue held, by one to nine points of lightness
// (a Lab distance under 10). For these eight the nearest fill in either band
// is six to thirteen points away, a different colour at a Lab distance of
// 11.6 to 22, except One's green, which would reach mono-1 ink nine points
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

// statuses are the semantic badge variants and callout tints; each reads its
// text from status-<s>-text on its fill status-<s>-bg.
var statuses = []string{"info", "ok", "warn", "error"}

// statusFloorExceptions names the status pairs that cannot reach 4.5, keyed
// family/mode/status. The ink is the semantic colour pulled toward text-1, so
// it can read no better than text-1 itself on the tinted fill. In these three
// variants text-1 only just clears 4.5 on plain paper (solarized light 4.99,
// solarized dark 4.86, Tokyo Night Day 4.52), and any tint takes it under the
// bar, so no ink share reaches it and they take the deepest, 65%. Reaching
// 4.5 would take an ink deeper than the family's own body text, so they keep
// the derived pair at the floors measured. Each still reads better than the
// old colour-on-tint badge did on paper there (2.6 to 3.5).
var statusFloorExceptions = map[string]float64{
	"solarized/light/info":    4.0, // #45728c on #dee5dd, measured 4.05
	"solarized/light/ok":      3.6, // #687d4c on #ece9c3, measured 3.68
	"solarized/light/warn":    4.2, // #806254 on #f6dec6, measured 4.27
	"solarized/light/error":   4.4, // #86595d on #f8dbca, measured 4.44
	"solarized/dark/info":     3.6, // #719db5 on #0d4357, measured 3.67
	"solarized/dark/ok":       3.7, // #8e9e69 on #194439, measured 3.76
	"solarized/dark/warn":     3.6, // #ad8571 on #253a3c, measured 3.64
	"solarized/dark/error":    3.6, // #ae8382 on #253943, measured 3.65
	"tokyo-night/light/info":  3.5, // #2e66cd on #c5d2e7, measured 3.53
	"tokyo-night/light/ok":    3.8, // #436790 on #ced3cf, measured 3.87
	"tokyo-night/light/warn":  3.9, // #625f7c on #dacfc7, measured 3.98
	"tokyo-night/light/error": 4.4, // #744296 on #e2c4d1, measured 4.41
}

// Every family's status ink reads as body text on its status fill, light and
// dark, with statusFloorExceptions as the only carve-out. As with the ink
// exceptions, a pair that now clears the bar or a key the loop never visits
// fails, so the map cannot outlive its reason.
func TestStatusTextReadsOnStatusFill(t *testing.T) {
	visited := map[string]bool{}
	for _, f := range webkit.Themes() {
		for mode, v := range map[string]*webkit.Variant{"light": f.Light, "dark": f.Dark} {
			if v == nil {
				continue
			}
			for _, s := range statuses {
				ink, fill := v.Roles["status-"+s+"-text"], v.Roles["status-"+s+"-bg"]
				got := contrast(t, ink, fill)
				floor := minInkContrast
				key := f.Name + "/" + mode + "/" + s
				if exception, ok := statusFloorExceptions[key]; ok {
					visited[key] = true
					if got >= minInkContrast {
						t.Errorf(
							"%s %s: --status-%s-text reads %.2f on its fill; drop its exception",
							f.Name,
							mode,
							s,
							got,
						)
					}
					floor = exception
				}
				if got < floor {
					t.Errorf("%s %s: --status-%s-text %s on --status-%s-bg %s = %.2f, want >= %.1f",
						f.Name, mode, s, ink, s, fill, got, floor)
				}
			}
		}
	}
	for key := range statusFloorExceptions {
		if !visited[key] {
			t.Errorf("statusFloorExceptions[%q] names no family, mode and status", key)
		}
	}
}

// inkShares are the shares the build tries, in order, when it pulls a status
// or accent ink toward text-1, and statusFillShare is how far paper moves
// toward the colour for the fill (src/themes.mjs INK_SHARES and
// STATUS_FILL_SHARE).
var inkShares = []float64{0.45, 0.55, 0.65}

const statusFillShare = 0.14

// statusColours names the semantic colour each status derives from.
var statusColours = map[string]string{
	"info":  "blue",
	"ok":    "green",
	"warn":  "amber",
	"error": "red",
}

// The status inks take the first share at which all four read 4.5 on their
// fills, so a variant keeps as much hue as the bar allows: every earlier
// share leaves one of them under the bar. A variant where no share reads
// takes the last; TestStatusTextReadsOnStatusFill holds the bar itself.
func TestStatusInkShareIsTheFirstThatReads(t *testing.T) {
	for _, f := range webkit.Themes() {
		for mode, v := range map[string]*webkit.Variant{"light": f.Light, "dark": f.Dark} {
			if v == nil {
				continue
			}
			share, ok := statusShareOf(t, v)
			if !ok {
				t.Errorf("%s %s: the status inks match no share of %v", f.Name, mode, inkShares)
				continue
			}
			for _, earlier := range inkShares {
				if earlier >= share {
					break
				}
				if statusInksRead(t, v, earlier) {
					t.Errorf("%s %s: the status inks read 4.5 at share %.2f, derived at %.2f",
						f.Name, mode, earlier, share)
				}
			}
		}
	}
}

// statusShareOf finds the share the build derived v's status inks at.
func statusShareOf(t *testing.T, v *webkit.Variant) (float64, bool) {
	t.Helper()
	for _, share := range inkShares {
		match := true
		for s, c := range statusColours {
			want := mixHex(t, v.Roles[c], v.Roles["text-1"], share)
			if !strings.EqualFold(v.Roles["status-"+s+"-text"], want) {
				match = false
			}
		}
		if match {
			return share, true
		}
	}
	return 0, false
}

// statusInksRead reports whether all four status inks, pulled share toward
// text-1, read 4.5 on their fills.
func statusInksRead(t *testing.T, v *webkit.Variant, share float64) bool {
	t.Helper()
	for _, c := range statusColours {
		ink := mixHex(t, v.Roles[c], v.Roles["text-1"], share)
		fill := mixHex(t, v.Roles["paper"], v.Roles[c], statusFillShare)
		if contrast(t, ink, fill) < minInkContrast {
			return false
		}
	}
	return true
}

// accentFloorExceptions names the variants whose accent badge ink reaches
// 4.5 on primary's fill at no share, keyed family/mode. They are the three
// the status test excepts, for the same reason, and take the deepest share.
var accentFloorExceptions = map[string]float64{
	"solarized/light":   3.7, // #477896 on #dfe7e1, measured 3.79
	"solarized/dark":    3.5, // #6d99b2 on #0b4256, measured 3.55
	"tokyo-night/light": 3.4, // #346ace on #c8d4e7, measured 3.42
}

// The accent badge mixes its ink at run time by --accent-ink-share, which
// the build picks per variant as the first share at which the ink reads 4.5
// on primary's fill. As with the status map, an exception that now reads or
// names no variant fails.
func TestAccentBadgeInkReadsOnItsFill(t *testing.T) {
	visited := map[string]bool{}
	for _, f := range webkit.Themes() {
		for mode, v := range map[string]*webkit.Variant{"light": f.Light, "dark": f.Dark} {
			if v == nil {
				continue
			}
			share, err := parsePercent(v.Roles["accent-ink-share"])
			if err != nil {
				t.Errorf("%s %s: --accent-ink-share: %v", f.Name, mode, err)
				continue
			}
			got := accentInkContrast(t, v, share)
			floor := minInkContrast
			key := f.Name + "/" + mode
			if exception, ok := accentFloorExceptions[key]; ok {
				visited[key] = true
				if got >= minInkContrast {
					t.Errorf(
						"%s %s: the accent ink reads %.2f; drop its exception",
						f.Name,
						mode,
						got,
					)
				}
				floor = exception
			}
			if got < floor {
				t.Errorf(
					"%s %s: the accent ink at %.2f reads %.2f, want >= %.1f",
					f.Name,
					mode,
					share,
					got,
					floor,
				)
			}
			for _, earlier := range inkShares {
				if earlier >= share {
					break
				}
				if accentInkContrast(t, v, earlier) >= minInkContrast {
					t.Errorf("%s %s: the accent ink reads 4.5 at share %.2f, emitted %.2f",
						f.Name, mode, earlier, share)
				}
			}
		}
	}
	for key := range accentFloorExceptions {
		if !visited[key] {
			t.Errorf("accentFloorExceptions[%q] names no family and mode", key)
		}
	}
}

// accentInkContrast is the accent badge's ink, on primary, pulled share
// toward text-1, against its fill.
func accentInkContrast(t *testing.T, v *webkit.Variant, share float64) float64 {
	t.Helper()
	ink := mixHex(t, v.Roles["primary"], v.Roles["text-1"], share)
	fill := mixHex(t, v.Roles["paper"], v.Roles["primary"], statusFillShare)
	return contrast(t, ink, fill)
}

// parsePercent reads an emitted share such as "45%" as 0.45.
func parsePercent(s string) (float64, error) {
	n, err := strconv.Atoi(strings.TrimSuffix(s, "%"))
	if err != nil || !strings.HasSuffix(s, "%") {
		return 0, fmt.Errorf("want a whole percentage, got %q", s)
	}
	return float64(n) / 100, nil
}

// mixHex moves a share of the way toward b in plain sRGB, rounding each
// channel the way the build does (src/themes.mjs mix).
func mixHex(t *testing.T, a, b string, share float64) string {
	t.Helper()
	ca, cb := channels(t, a), channels(t, b)
	out := "#"
	for i := range ca {
		v := math.Round(ca[i] + (cb[i]-ca[i])*share)
		out += fmt.Sprintf("%02x", int(math.Max(0, math.Min(255, v))))
	}
	return out
}

// channels splits a 6-digit hex colour into its 8-bit channels.
func channels(t *testing.T, hex string) [3]float64 {
	t.Helper()
	if !hexRE.MatchString(hex) {
		t.Fatalf("not a 6-digit hex colour: %q", hex)
	}
	n, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		t.Fatalf("parse %q: %v", hex, err)
	}
	return [3]float64{float64(n >> 16 & 255), float64(n >> 8 & 255), float64(n & 255)}
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
	c := channels(t, hex)
	// The linear-segment cutoff is 0.04045, the value the WCAG 2.x errata
	// settled on; the 0.03928 the original text carried gives the same
	// result for every 8-bit channel.
	linear := func(c float64) float64 {
		s := c / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(c[0]) + 0.7152*linear(c[1]) + 0.0722*linear(c[2])
}
