package webkit_test

import (
	"math"
	"testing"

	"github.com/mad01/thismoon/webkit"
)

// minSeriesDistance is the CIEDE2000 colour difference series-1 and series-2
// keep in every variant. A two-series line or bar chart draws exactly these
// two, and closer than 20 they read as one colour at chart size. Seven
// variants sat between 12.6 and 18.9 until each moved its series-2 to another
// published colour of its family (MAD-582).
const minSeriesDistance = 20.0

// Every variant's first two chart series stay far enough apart to tell two
// lines or bars apart.
func TestFirstTwoSeriesStayApart(t *testing.T) {
	for _, f := range webkit.Themes() {
		for _, m := range []struct {
			name    string
			variant *webkit.Variant
		}{{"light", f.Light}, {"dark", f.Dark}} {
			if m.variant == nil {
				continue
			}
			a, b := m.variant.Roles["series-1"], m.variant.Roles["series-2"]
			if got := ciede2000(labOf(t, a), labOf(t, b)); got < minSeriesDistance {
				t.Errorf("%s %s: --series-1 %s and --series-2 %s are %.1f apart, want >= %.0f",
					f.Name, m.name, a, b, got, minSeriesDistance)
			}
		}
	}
}

// The distance maths matches the published CIEDE2000 test data (Sharma, Wu
// and Dalal, 2005), the hue-wrap pairs included, and the sRGB to Lab step
// lands on the textbook values for pure red and white.
func TestCIEDE2000MatchesReferenceData(t *testing.T) {
	for _, tc := range []struct {
		a, b lab
		want float64
	}{
		{lab{50, 2.6772, -79.7751}, lab{50, 0, -82.7485}, 2.0425},
		{lab{50, 0, 0}, lab{50, -1, 2}, 2.3669},
		{lab{50, 2.49, -0.001}, lab{50, -2.49, 0.0009}, 7.1792},
		{lab{50, 2.49, -0.001}, lab{50, -2.49, 0.0011}, 7.2195},
		{lab{50, 2.5, 0}, lab{73, 25, -18}, 27.1492},
		{lab{60.2574, -34.0099, 36.2677}, lab{60.4626, -34.1751, 39.4387}, 1.2644},
		{lab{2.0776, 0.0795, -1.135}, lab{0.9033, -0.0636, -0.5514}, 0.9082},
	} {
		if got := ciede2000(tc.a, tc.b); math.Abs(got-tc.want) > 1e-4 {
			t.Errorf("ciede2000(%v, %v) = %.4f, want %.4f", tc.a, tc.b, got, tc.want)
		}
		if got := ciede2000(tc.b, tc.a); math.Abs(got-tc.want) > 1e-4 {
			t.Errorf("ciede2000(%v, %v) = %.4f, want %.4f (swapped)", tc.b, tc.a, got, tc.want)
		}
	}
	for _, tc := range []struct {
		hex  string
		want lab
	}{
		{"#FF0000", lab{53.24, 80.09, 67.20}},
		{"#FFFFFF", lab{100, 0, 0}},
	} {
		got := labOf(t, tc.hex)
		for i := range got {
			if math.Abs(got[i]-tc.want[i]) > 0.01 {
				t.Errorf("labOf(%s) = %.2f, want %.2f", tc.hex, got, tc.want)
				break
			}
		}
	}
}

// lab is a CIELAB colour: lightness L* (0 to 100), then a* and b*.
type lab [3]float64

// labOf converts a 6-digit sRGB hex colour to CIELAB under the D65 white
// point, the illuminant sRGB is defined against.
func labOf(t *testing.T, hex string) lab {
	t.Helper()
	c := channels(t, hex)
	var rgb [3]float64
	for i, v := range c {
		s := v / 255
		if s <= 0.04045 {
			rgb[i] = s / 12.92
		} else {
			rgb[i] = math.Pow((s+0.055)/1.055, 2.4)
		}
	}
	// sRGB to XYZ, each row divided by the D65 white (0.95047, 1, 1.08883).
	x := (0.4124564*rgb[0] + 0.3575761*rgb[1] + 0.1804375*rgb[2]) / 0.95047
	y := 0.2126729*rgb[0] + 0.7151522*rgb[1] + 0.0721750*rgb[2]
	z := (0.0193339*rgb[0] + 0.1191920*rgb[1] + 0.9503041*rgb[2]) / 1.08883
	f := func(v float64) float64 {
		const epsilon, kappa = 216.0 / 24389, 24389.0 / 27
		if v > epsilon {
			return math.Cbrt(v)
		}
		return (kappa*v + 16) / 116
	}
	fx, fy, fz := f(x), f(y), f(z)
	return lab{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}

// ciede2000 is the CIEDE2000 colour difference between two Lab colours with
// the parametric weights at 1, after Sharma, Wu and Dalal (2005). Around 2
// is a just-noticeable difference; 20 separates two chart series.
func ciede2000(c1, c2 lab) float64 {
	l1, a1, b1 := c1[0], c1[1], c1[2]
	l2, a2, b2 := c2[0], c2[1], c2[2]
	cBar := (math.Hypot(a1, b1) + math.Hypot(a2, b2)) / 2
	g := 0.5 * (1 - math.Sqrt(pow7(cBar)/(pow7(cBar)+pow7(25))))
	a1p, a2p := (1+g)*a1, (1+g)*a2
	c1p, c2p := math.Hypot(a1p, b1), math.Hypot(a2p, b2)
	h1p, h2p := hueDeg(b1, a1p), hueDeg(b2, a2p)

	dLp, dCp := l2-l1, c2p-c1p
	dhp, hBarP := hueDiffAndMean(h1p, h2p, c1p*c2p == 0)
	dHp := 2 * math.Sqrt(c1p*c2p) * math.Sin(rad(dhp/2))

	lBarP, cBarP := (l1+l2)/2, (c1p+c2p)/2
	tt := 1 - 0.17*math.Cos(rad(hBarP-30)) + 0.24*math.Cos(rad(2*hBarP)) +
		0.32*math.Cos(rad(3*hBarP+6)) - 0.20*math.Cos(rad(4*hBarP-63))
	hOff, lOff := (hBarP-275)/25, lBarP-50
	dTheta := 30 * math.Exp(-hOff*hOff)
	rc := 2 * math.Sqrt(pow7(cBarP)/(pow7(cBarP)+pow7(25)))
	sl := 1 + 0.015*lOff*lOff/math.Sqrt(20+lOff*lOff)
	sc := 1 + 0.045*cBarP
	sh := 1 + 0.015*cBarP*tt
	rt := -math.Sin(rad(2*dTheta)) * rc

	dl, dc, dh := dLp/sl, dCp/sc, dHp/sh
	return math.Sqrt(dl*dl + dc*dc + dh*dh + rt*dc*dh)
}

// hueDiffAndMean returns the signed hue difference h2 - h1, taken the short
// way round the circle, and the mean hue, both in degrees. A neutral colour
// (achromatic) has no hue, so the difference is 0 and the mean is the sum.
func hueDiffAndMean(h1, h2 float64, achromatic bool) (diff, mean float64) {
	if achromatic {
		return 0, h1 + h2
	}
	diff = h2 - h1
	switch {
	case diff > 180:
		diff -= 360
	case diff < -180:
		diff += 360
	}
	switch {
	case math.Abs(h1-h2) <= 180:
		mean = (h1 + h2) / 2
	case h1+h2 < 360:
		mean = (h1 + h2 + 360) / 2
	default:
		mean = (h1 + h2 - 360) / 2
	}
	return diff, mean
}

// hueDeg is the hue angle of (a, b) in degrees, 0 to 360, and 0 for a
// neutral colour.
func hueDeg(b, a float64) float64 {
	if a == 0 && b == 0 {
		return 0
	}
	h := math.Atan2(b, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return h
}

func rad(deg float64) float64 { return deg * math.Pi / 180 }

func pow7(v float64) float64 { return math.Pow(v, 7) }
