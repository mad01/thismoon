package ui_test

import (
	"fmt"
	"maps"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/mad01/thismoon/webkit"
)

// meaningRoles are the node colours that carry meaning in graph.js: the
// tier, status, evidence and staleness maps, the grey of "none", and the
// teal and beige of tag and dangling nodes, which show in every mode.
var meaningRoles = []string{
	"blue", "orange", "green", "purple", "yellow", "red", "grey", "teal", "beige",
}

const (
	// minNodeDistance is the least CIEDE2000 difference two meaning colours
	// keep in any family and mode: well past a just-noticeable step (about
	// 2), so two states never read as one.
	minNodeDistance = 9.0
	// minClusterDistance is the lower floor for the twelve colours a
	// categorical mode (cluster, vault) hands out, plus the grey of loose
	// notes. The kit's roles hold no twelve colours 9 apart in every family;
	// 5 still reads as two colours side by side.
	minClusterDistance = 5.0
	// minNodeContrast is the least WCAG contrast a meaning colour keeps on
	// the canvas (--graph-bg), so a node never fades into it.
	minNodeContrast = 1.5
)

// nodeRoleRE matches one --node-<name>: var(--<role>) declaration.
var (
	nodeRoleRE = regexp.MustCompile(`--node-([a-z]+):var\(--([a-z0-9-]+)\)`)
	hexRE      = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// nodeMapping reads the webkit role each node colour points at, light from
// the :root rule and dark from the [data-theme="dark"] rule over it.
func nodeMapping(t *testing.T) map[string]map[string]string {
	t.Helper()
	shell := read(t, "shell.html")
	block := func(selector string) string {
		m := regexp.MustCompile(regexp.QuoteMeta(selector) + `\{([^}]*)\}`).
			FindStringSubmatch(shell)
		if m == nil {
			t.Fatalf("shell.html has no %s rule", selector)
		}
		return m[1]
	}
	light := map[string]string{}
	for _, m := range nodeRoleRE.FindAllStringSubmatch(block(":root"), -1) {
		light[m[1]] = m[2]
	}
	dark := maps.Clone(light)
	for _, m := range nodeRoleRE.FindAllStringSubmatch(block(`[data-theme="dark"]`), -1) {
		dark[m[1]] = m[2]
	}
	return map[string]map[string]string{"light": light, "dark": dark}
}

// TestNodeColoursStayApart holds the meaning colours apart from each other
// and visible on the canvas in every theme family, light and dark, keeps the
// first six of graph.js's NODE_ROLES (the largest clusters) among them, and
// holds all twelve NODE_ROLES and the loose grey to the lower cluster floor.
func TestNodeColoursStayApart(t *testing.T) {
	nodeRoles := jsList(t, read(t, "graph.js"), "NODE_ROLES")
	checked := map[string]bool{}
	for _, r := range meaningRoles {
		checked["node-"+r] = true
	}
	for _, r := range nodeRoles[:6] {
		if !checked[r] {
			t.Errorf(
				"NODE_ROLES leads with %s, which is not a meaning colour this test keeps apart",
				r,
			)
		}
	}
	cluster := []string{"grey"}
	for _, r := range nodeRoles {
		cluster = append(cluster, strings.TrimPrefix(r, "node-"))
	}
	palettes := nodePalettes(t)
	for _, p := range palettes {
		for _, name := range meaningRoles {
			if c := contrastRatio(t, p.colour(t, name), p.canvas); c < minNodeContrast {
				t.Errorf("%s: --node-%s %s on the canvas %s = %.2f, want >= %.1f",
					p.name, name, p.colour(t, name), p.canvas, c, minNodeContrast)
			}
		}
	}
	t.Logf("closest meaning pair: %s", closestPair(t, palettes, meaningRoles, minNodeDistance))
	t.Logf("closest cluster pair: %s", closestPair(t, palettes, cluster, minClusterDistance))
}

// nodePalette is the node colours of one theme family in one mode.
type nodePalette struct {
	name    string            // family and mode, as "nord dark"
	canvas  string            // --graph-bg
	colours map[string]string // node colour name (blue, grey...) to hex
}

func (p nodePalette) colour(t *testing.T, name string) string {
	t.Helper()
	hex, ok := p.colours[name]
	if !ok {
		t.Fatalf("shell.html maps no --node-%s", name)
	}
	return hex
}

// nodePalettes resolves shell.html's node mapping in every family and mode.
func nodePalettes(t *testing.T) []nodePalette {
	t.Helper()
	mapping := nodeMapping(t)
	var out []nodePalette
	for _, f := range webkit.Themes() {
		for _, mv := range []struct {
			mode string
			v    *webkit.Variant
		}{{"light", f.Light}, {"dark", f.Dark}} {
			if mv.v == nil {
				continue
			}
			p := nodePalette{
				name:    f.Name + " " + mv.mode,
				canvas:  mv.v.Roles["graph-bg"],
				colours: map[string]string{},
			}
			for name, role := range mapping[mv.mode] {
				hex := mv.v.Roles[role]
				if !hexRE.MatchString(hex) {
					t.Fatalf(
						"%s: --node-%s reads --%s = %q, not a hex colour",
						p.name,
						name,
						role,
						hex,
					)
				}
				p.colours[name] = hex
			}
			out = append(out, p)
		}
	}
	return out
}

// closestPair fails each pair of the named colours that sits closer than
// floor in any palette, and describes the closest pair it saw.
func closestPair(t *testing.T, palettes []nodePalette, names []string, floor float64) string {
	t.Helper()
	worst, desc := math.Inf(1), ""
	for _, p := range palettes {
		for i, a := range names {
			for _, b := range names[i+1:] {
				ca, cb := p.colour(t, a), p.colour(t, b)
				d := deltaE2000(t, ca, cb)
				line := fmt.Sprintf(
					"%s: --node-%s %s and --node-%s %s differ by %.2f",
					p.name,
					a,
					ca,
					b,
					cb,
					d,
				)
				if d < worst {
					worst, desc = d, line
				}
				if d < floor {
					t.Errorf(
						"%s, want >= %.1f; remap the --node-* roles in services/brain/internal/server/ui/shell.html",
						line,
						floor,
					)
				}
			}
		}
	}
	return desc
}

// rgb splits a 6-digit hex colour into channels from 0 to 1.
func rgb(t *testing.T, hex string) [3]float64 {
	t.Helper()
	if !hexRE.MatchString(hex) {
		t.Fatalf("not a 6-digit hex colour: %q", hex)
	}
	n, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		t.Fatalf("parse %q: %v", hex, err)
	}
	return [3]float64{float64(n>>16&255) / 255, float64(n>>8&255) / 255, float64(n&255) / 255}
}

// linear undoes the sRGB transfer curve of one channel.
func linear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// contrastRatio is the WCAG 2.x contrast between two hex colours, 1 to 21.
func contrastRatio(t *testing.T, a, b string) float64 {
	t.Helper()
	lum := func(hex string) float64 {
		c := rgb(t, hex)
		return 0.2126*linear(c[0]) + 0.7152*linear(c[1]) + 0.0722*linear(c[2])
	}
	la, lb := lum(a), lum(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

// lab converts a hex colour to CIELAB under D65.
func lab(t *testing.T, hex string) [3]float64 {
	t.Helper()
	c := rgb(t, hex)
	r, g, bl := linear(c[0]), linear(c[1]), linear(c[2])
	x := (0.4124*r + 0.3576*g + 0.1805*bl) / 0.95047
	y := 0.2126*r + 0.7152*g + 0.0722*bl
	z := (0.0193*r + 0.1192*g + 0.9505*bl) / 1.08883
	f := func(v float64) float64 {
		if v > 216.0/24389 {
			return math.Cbrt(v)
		}
		return (24389.0/27*v + 16) / 116
	}
	fx, fy, fz := f(x), f(y), f(z)
	return [3]float64{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}

// deltaE2000 is the CIEDE2000 colour difference between two hex colours.
func deltaE2000(t *testing.T, hex1, hex2 string) float64 {
	t.Helper()
	return ciede2000(lab(t, hex1), lab(t, hex2))
}

// The CIEDE2000 maths matches pairs from Sharma, Wu and Dalal's test data
// (2005), the hue wrap-around (pair 11) among them.
func TestCIEDE2000MatchesReference(t *testing.T) {
	for _, tc := range []struct {
		lab1, lab2 [3]float64
		want       float64
	}{
		{[3]float64{50, 2.6772, -79.7751}, [3]float64{50, 0, -82.7485}, 2.0425},
		{[3]float64{50, 0, 0}, [3]float64{50, -1, 2}, 2.3669},
		{[3]float64{50, 2.49, -0.001}, [3]float64{50, -2.49, 0.0009}, 7.1792},
		{[3]float64{50, 2.5, 0}, [3]float64{73, 25, -18}, 27.1492},
		{[3]float64{60.2574, -34.0099, 36.2677}, [3]float64{60.4626, -34.1751, 39.4387}, 1.2644},
	} {
		if got := ciede2000(tc.lab1, tc.lab2); math.Abs(got-tc.want) > 0.0001 {
			t.Errorf("ciede2000(%v, %v) = %.4f, want %.4f", tc.lab1, tc.lab2, got, tc.want)
		}
	}
}

// ciede2000 is the CIEDE2000 difference between two CIELAB colours.
func ciede2000(c1, c2 [3]float64) float64 {
	l1, a1, b1 := c1[0], c1[1], c1[2]
	l2, a2, b2 := c2[0], c2[1], c2[2]
	rad := math.Pi / 180
	cBar := (math.Hypot(a1, b1) + math.Hypot(a2, b2)) / 2
	g := 0.5 * (1 - math.Sqrt(math.Pow(cBar, 7)/(math.Pow(cBar, 7)+math.Pow(25, 7))))
	a1p, a2p := (1+g)*a1, (1+g)*a2
	c1p, c2p := math.Hypot(a1p, b1), math.Hypot(a2p, b2)
	hue := func(b, ap float64) float64 { return math.Mod(math.Atan2(b, ap)/rad+360, 360) }
	h1p, h2p := hue(b1, a1p), hue(b2, a2p)
	dhp, hBar := 0.0, h1p+h2p
	if c1p*c2p != 0 {
		dhp = h2p - h1p
		switch {
		case dhp > 180:
			dhp -= 360
		case dhp < -180:
			dhp += 360
		}
		hBar = (h1p + h2p) / 2
		if math.Abs(h1p-h2p) > 180 {
			hBar += 180
			if hBar >= 360 {
				hBar -= 360
			}
		}
	}
	dLp, dCp := l2-l1, c2p-c1p
	dHp := 2 * math.Sqrt(c1p*c2p) * math.Sin(dhp*rad/2)
	lBar, cBarP := (l1+l2)/2, (c1p+c2p)/2
	tt := 1 - 0.17*math.Cos((hBar-30)*rad) + 0.24*math.Cos(2*hBar*rad) +
		0.32*math.Cos((3*hBar+6)*rad) - 0.2*math.Cos((4*hBar-63)*rad)
	hOff := (hBar - 275) / 25
	dTheta := 30 * math.Exp(-hOff*hOff)
	rc := 2 * math.Sqrt(math.Pow(cBarP, 7)/(math.Pow(cBarP, 7)+math.Pow(25, 7)))
	lOff := (lBar - 50) * (lBar - 50)
	sl := 1 + 0.015*lOff/math.Sqrt(20+lOff)
	sc := 1 + 0.045*cBarP
	sh := 1 + 0.015*cBarP*tt
	rt := -math.Sin(2*dTheta*rad) * rc
	l, c, h := dLp/sl, dCp/sc, dHp/sh
	return math.Sqrt(l*l + c*c + h*h + rt*c*h)
}
