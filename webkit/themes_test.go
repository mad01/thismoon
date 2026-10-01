package webkit_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/mad01/thismoon/webkit"
)

// defaultLight and defaultDark are the literal values the hand-written
// palette blocks, the dark code block, and present's getGraphColors and
// getChartColors carried before the theme collection existed. The default
// family must generate exactly these, so the migration moves no pixel on an
// existing page. Change a value here only when the default palette is meant
// to change.
var defaultLight = map[string]string{
	"primary": "#C4704B", "bg": "#FAF9F7", "paper": "#FFFFFF",
	"text-1": "#252320", "text-2": "#6B6459", "text-3": "#8C8578",
	"border": "#D8D4CD", "border-hover": "#B8B2A7",
	"card-bg": "#FFFFFF", "chip-bg": "#EEECE8",
	"chip-active-bg": "#C4704B", "chip-active-text": "#FFFFFF",
	"progress-bg": "#EEECE8", "progress-fill": "#C4704B",
	"tag-a": "#E8D5C4", "tag-a-text": "#8B5A2B", "tag-b": "#D4E8D4", "tag-b-text": "#2B6B3E",
	"tag-c": "#D4DEE8", "tag-c-text": "#2B4A6B", "ra-highlight": "#F3DDD2",
	"terracotta": "#C4704B", "green-light": "#6BC48A", "primary-soft": "#d4855f",
	"amber": "#D97706", "green": "#4A9E6B", "yellow": "#C4960B", "red": "#C45B4B",
	"blue": "#5B8EC4", "purple": "#8B6BB0",
	"text-body": "#4A453D",
	"topbar-bg": "rgba(250,249,247,0.88)", "scrim": "rgba(37,35,32,0.45)",
	"focus-ring": "rgba(196,112,75,0.15)",
	"code-bg":    "#FFFFFF", "code-header-bg": "#EEECE8", "code-border": "#D8D4CD",
	"code-keyword": "#C4704B", "code-string": "#4A9E6B", "code-number": "#D97706", "code-symbol": "#5B8EC4",
	"graph-bg": "#FFFFFF", "graph-center-bg": "#FFF5F0", "graph-center-border": "#C4704B",
	"graph-center-text": "#252320", "graph-leaf-bg": "#FFFFFF", "graph-leaf-border": "#D8D4CD",
	"graph-leaf-text": "#252320",
	"graph-module-1":  "#C4704B", "graph-module-2": "#5B8EC4", "graph-module-3": "#4A9E6B",
	"graph-module-4": "#8B6BB0",
	"graph-edge":     "#D8D4CD", "graph-edge-arrow": "#B8B2A7", "graph-hot": "#C4704B",
	"graph-registry-bg": "#EEECE8", "graph-registry-border": "#D8D4CD", "graph-registry-text": "#6B6459",
	"tone-neutral-bg": "#EEECE8", "tone-neutral-border": "#C9C4BB", "tone-neutral-text": "#3D3A34",
	"tone-green-bg": "#E6F4EC", "tone-green-border": "#4A9E6B", "tone-green-text": "#1F5C38",
	"tone-red-bg": "#FBEAE3", "tone-red-border": "#C4704B", "tone-red-text": "#7A3A1F",
	"tone-blue-bg": "#E7EFF9", "tone-blue-border": "#5B8EC4", "tone-blue-text": "#2A4E75",
	"tone-amber-bg": "#FBF1DC", "tone-amber-border": "#C9973A", "tone-amber-text": "#6B4E12",
	"tone-purple-bg": "#EFE8F5", "tone-purple-border": "#8B6BB0", "tone-purple-text": "#4E3A6B",
	"series-1": "#C4704B", "series-2": "#5B8EC4", "series-3": "#4A9E6B", "series-4": "#8B6BB0",
	"chart-grid": "#D8D4CD", "chart-text": "#6B6459", "chart-label": "#252320",
}

var defaultDark = map[string]string{
	"primary": "#E8956A", "bg": "#1A1916", "paper": "#252320",
	"text-1": "#D8D4CD", "text-2": "#B8B2A7", "text-3": "#8C8578",
	"border": "#35322C", "border-hover": "#4A453D",
	"card-bg": "#252320", "chip-bg": "#35322C",
	"chip-active-bg": "#E8956A", "chip-active-text": "#1A1916",
	"progress-bg": "#35322C", "progress-fill": "#E8956A",
	"tag-a": "#3A2E20", "tag-a-text": "#E8B86A", "tag-b": "#1E3A2A", "tag-b-text": "#6BC48A",
	"tag-c": "#1E2A3A", "tag-c-text": "#7AAAE8", "ra-highlight": "#4A3328",
	"green-light": "#6BC48A", "primary-soft": "#d4855f",
	// The semantic colours keep their light values: the old dark block never
	// overrode them (decision 7 of the design).
	"amber": "#D97706", "green": "#4A9E6B", "yellow": "#C4960B", "red": "#C45B4B",
	"blue": "#5B8EC4", "purple": "#8B6BB0",
	"text-body": "#B8B2A7",
	"topbar-bg": "rgba(26,25,22,0.88)", "scrim": "rgba(37,35,32,0.45)",
	"code-bg": "#1A1916", "code-header-bg": "#252320", "code-border": "#35322C",
	"code-keyword": "#E8956A", "code-string": "#6BC48A", "code-number": "#E8B86A", "code-symbol": "#7AAAE8",
	"graph-bg": "#1A1916", "graph-center-bg": "#3A2A20", "graph-center-border": "#E8956A",
	"graph-center-text": "#F5F3EF", "graph-leaf-bg": "#252320", "graph-leaf-border": "#4A453D",
	"graph-leaf-text": "#F5F3EF",
	"graph-module-1":  "#E8956A", "graph-module-2": "#7AAAE8", "graph-module-3": "#6BC48A",
	"graph-module-4": "#8B6BB0",
	"graph-edge":     "#4A453D", "graph-edge-arrow": "#6B6459", "graph-hot": "#E8956A",
	"graph-registry-bg": "#35322C", "graph-registry-border": "#4A453D", "graph-registry-text": "#B8B2A7",
	"tone-neutral-bg": "#35322C", "tone-neutral-border": "#5A544B", "tone-neutral-text": "#E6E1D8",
	"tone-green-bg": "#1F3A2B", "tone-green-border": "#6BC48A", "tone-green-text": "#C8EBD5",
	"tone-red-bg": "#3F2A22", "tone-red-border": "#E8956A", "tone-red-text": "#F3CDBB",
	"tone-blue-bg": "#1F2E42", "tone-blue-border": "#7AAAE8", "tone-blue-text": "#C8DAF3",
	"tone-amber-bg": "#3E3418", "tone-amber-border": "#D9AE55", "tone-amber-text": "#F0DEB0",
	"tone-purple-bg": "#32283F", "tone-purple-border": "#A98BCB", "tone-purple-text": "#DDD0EA",
	"series-1": "#E8956A", "series-2": "#7AAAE8", "series-3": "#6BC48A", "series-4": "#8B6BB0",
	"chart-grid": "#4A453D", "chart-text": "#B8B2A7", "chart-label": "#F5F3EF",
}

// wantFamilies is the shipped collection, in the order the picker lists it.
var wantFamilies = []string{
	"default", "catppuccin", "gruvbox", "nord", "one", "rose-pine", "solarized", "tokyo-night",
}

var hexRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func findTheme(t *testing.T, name string) webkit.Theme {
	t.Helper()
	for _, f := range webkit.Themes() {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("Themes() has no family %q", name)
	return webkit.Theme{}
}

func TestDefaultFamilyPinsTodaysPalette(t *testing.T) {
	def := findTheme(t, "default")
	for mode, want := range map[string]map[string]string{"light": defaultLight, "dark": defaultDark} {
		v := def.Light
		if mode == "dark" {
			v = def.Dark
		}
		if v == nil {
			t.Fatalf("default family has no %s variant", mode)
		}
		for role, hex := range want {
			if got := v.Roles[role]; !strings.EqualFold(got, hex) {
				t.Errorf("default %s --%s = %q, want %q", mode, role, got, hex)
			}
		}
	}
}

func TestThemesCollection(t *testing.T) {
	families := webkit.Themes()
	var names []string
	for _, f := range families {
		names = append(names, f.Name)
		if f.Label == "" || f.Source == "" || f.License == "" {
			t.Errorf("family %q lacks label, source, or license: %+v", f.Name, f)
		}
		if f.Light == nil && f.Dark == nil {
			t.Errorf("family %q ships no variant", f.Name)
		}
	}
	if strings.Join(names, ",") != strings.Join(wantFamilies, ",") {
		t.Errorf("Themes() = %v, want %v", names, wantFamilies)
	}
}

// Every variant resolves every reference role, and every role the graph and
// chart code reads is a literal hex: Cytoscape and Chart.js cannot resolve a
// var() or a color-mix().
func TestEveryVariantResolvesEveryRole(t *testing.T) {
	roles := webkit.Roles()
	def := findTheme(t, "default")
	for _, f := range webkit.Themes() {
		for mode, v := range map[string]*webkit.Variant{"light": f.Light, "dark": f.Dark} {
			if v == nil {
				continue
			}
			if v.Name == "" {
				t.Errorf("%s %s: variant has no name", f.Name, mode)
			}
			for role := range def.Light.Roles {
				if v.Roles[role] == "" {
					t.Errorf("%s %s: role %q missing", f.Name, mode, role)
				}
			}
			for _, role := range roles {
				if v.Roles[role] == "" {
					t.Errorf("%s %s: reference role %q missing", f.Name, mode, role)
				}
			}
			for role, value := range v.Roles {
				if strings.Contains(value, "var(") || strings.Contains(value, "color-mix(") {
					t.Errorf("%s %s: --%s = %q is not a literal", f.Name, mode, role, value)
				}
				canvas := strings.HasPrefix(role, "graph-") || strings.HasPrefix(role, "tone-") ||
					strings.HasPrefix(role, "chart-") || strings.HasPrefix(role, "series-")
				if canvas && !hexRE.MatchString(value) {
					t.Errorf(
						"%s %s: canvas role --%s = %q must be a hex literal",
						f.Name,
						mode,
						role,
						value,
					)
				}
			}
		}
	}
}

// Roles() is the vocabulary a page may reference by name; the deck layout
// work validates per-slide tones against it.
func TestRolesVocabulary(t *testing.T) {
	roles := webkit.Roles()
	want := []string{
		"primary", "red", "green", "amber", "yellow", "blue", "purple",
		"series-1", "series-2", "series-3", "series-4",
		"bg", "paper", "chip",
		"tone-neutral-bg", "tone-green-bg", "tone-red-bg", "tone-blue-bg", "tone-amber-bg", "tone-purple-bg",
	}
	if strings.Join(roles, ",") != strings.Join(want, ",") {
		t.Errorf("Roles() = %v, want %v", roles, want)
	}
	for _, r := range want {
		if !webkit.IsRole(r) {
			t.Errorf("IsRole(%q) = false", r)
		}
	}
	if webkit.IsRole("terracotta") || webkit.IsRole("") {
		t.Error("IsRole accepts a name outside the vocabulary")
	}
	roles[0] = "mutated"
	if webkit.Roles()[0] != "primary" {
		t.Error("Roles() returned shared backing storage")
	}
}

func TestThemesReturnsCopies(t *testing.T) {
	a := webkit.Themes()
	a[0].Light.Roles["bg"] = "#000000"
	if b := webkit.Themes(); b[0].Light.Roles["bg"] == "#000000" {
		t.Error("Themes() returned shared role maps")
	}
}

// The stylesheet carries a selector pair for every family and mode, and the
// default doubles as :root and the bare dark selector so a page with no
// palette attribute, or one naming a family that no longer ships, renders the
// default.
func TestStylesheetCarriesEveryFamily(t *testing.T) {
	css := serve(t, "/webkit/webkit.css").Body.String()
	for _, want := range []string{
		`:root, [data-palette="default"][data-theme="light"] {`,
		`[data-theme="dark"], [data-palette="default"][data-theme="dark"] {`,
		"--on-primary:", "--topbar-bg:", "--scrim:", "--focus-ring:", "--text-body:",
		"--graph-leaf-bg:", "--tone-amber-bg:", "--series-4:", "--chart-grid:",
		// themes page styling
		".wk-theme-card", ".wk-theme-grid",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("webkit.css missing %q", want)
		}
	}
	for _, f := range webkit.Themes() {
		for mode, v := range map[string]*webkit.Variant{"light": f.Light, "dark": f.Dark} {
			if v == nil {
				continue
			}
			sel := `[data-palette="` + f.Name + `"][data-theme="` + mode + `"]`
			if !strings.Contains(css, sel) {
				t.Errorf("webkit.css has no block for %s", sel)
			}
		}
	}
	// The ramp names are gone: a rule reading them would be invalid under
	// another family. --terracotta stays as a declared alias of primary.
	for _, gone := range []string{"var(--wg", "var(--cream)", "var(--off-white)", "var(--terracotta-light)"} {
		if strings.Contains(css, gone) {
			t.Errorf("webkit.css still reads the ramp token %q", gone)
		}
	}
}

func TestHandlerServesThemesPage(t *testing.T) {
	rec := serve(t, "/webkit/themes")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /webkit/themes: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<script src="/webkit/boot.js"></script>`, `/webkit/webkit.css`, `/webkit/webkit.js`, `/webkit/themes.js`,
		"<wk-header", `id="wk-theme-system"`, `id="wk-theme-reset"`, `id="wk-theme-light"`, `id="wk-theme-dark"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("themes page missing %q", want)
		}
	}
}

func TestHandlerServesThemesJSON(t *testing.T) {
	rec := serve(t, "/webkit/themes.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /webkit/themes.json: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("Content-Type = %q, want json", ct)
	}
	var got struct {
		Roles    []string       `json:"roles"`
		Families []webkit.Theme `json:"families"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("themes.json is not valid JSON: %v", err)
	}
	if len(got.Families) != len(wantFamilies) || len(got.Roles) != len(webkit.Roles()) {
		t.Errorf("themes.json has %d families and %d roles, want %d and %d",
			len(got.Families), len(got.Roles), len(wantFamilies), len(webkit.Roles()))
	}
}

func TestHandlerServesThemesScript(t *testing.T) {
	rec := serve(t, "/webkit/themes.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /webkit/themes.js: status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"/webkit/themes.json", "setPalette", "setThemeMode", "resetTheme", "wk-theme-card"} {
		if !strings.Contains(body, want) {
			t.Errorf("themes.js missing %q", want)
		}
	}
}
