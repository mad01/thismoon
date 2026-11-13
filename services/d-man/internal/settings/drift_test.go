package settings

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/mad01/thismoon/webkit"
)

// kitSource is the kit's TypeScript source, read as text: the value table
// copies what the kit accepts, and these tests fail when the kit moves.
// The webkit import stays in tests only; d-man never links the kit.
var kitSource = filepath.Join("..", "..", "..", "..", "webkit", "src")

func readKit(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(kitSource, name))
	if err != nil {
		t.Fatalf("read kit source: %v", err)
	}
	return string(data)
}

func TestFamiliesMatchKit(t *testing.T) {
	var kit []string
	for _, theme := range webkit.Themes() {
		kit = append(kit, theme.Name)
	}
	if !slices.Equal(families, kit) {
		t.Errorf("families = %v, webkit.Themes() names = %v", families, kit)
	}
}

func TestFontsMatchKit(t *testing.T) {
	src := readKit(t, "webkit.ts")
	block := regexp.MustCompile(`(?s)const FONT_STACKS[^{]*\{(.*?)\n\};`).FindStringSubmatch(src)
	if block == nil {
		t.Fatal("FONT_STACKS not found in webkit.ts")
	}
	var kit []string
	for _, m := range regexp.MustCompile(`(?m)^\s*'([^']+)':`).FindAllStringSubmatch(block[1], -1) {
		kit = append(kit, m[1])
	}
	if !slices.Equal(fonts, kit) {
		t.Errorf("fonts = %v, FONT_STACKS keys = %v", fonts, kit)
	}
}

func TestSizeBoundsMatchKit(t *testing.T) {
	src := readKit(t, "size.ts")
	for name, want := range map[string]int{"SIZE_MIN": sizeMin, "SIZE_MAX": sizeMax} {
		m := regexp.MustCompile(`export const ` + name + `\s*=\s*(\d+);`).FindStringSubmatch(src)
		if m == nil {
			t.Errorf("%s not found in size.ts", name)
			continue
		}
		if got, _ := strconv.Atoi(m[1]); got != want {
			t.Errorf("%s = %d in the kit, %d here", name, got, want)
		}
	}
}

// Every stored key is one the kit writes to localStorage.
func TestKeysMatchKit(t *testing.T) {
	src := readKit(t, "webkit.ts")
	for name, value := range map[string]string{
		"THEME_KEY":          "webkit-theme",
		"PALETTE_KEY_PREFIX": "webkit-palette-",
		"FONT_KEY":           "webkit-font",
		"SIZE_KEY":           "webkit-size",
		"FIXATION_KEY":       "webkit-fixation",
	} {
		decl := `const ` + name + `\s*=\s*'` + regexp.QuoteMeta(value) + `';`
		if !regexp.MustCompile(decl).MatchString(src) {
			t.Errorf("webkit.ts no longer declares %s = '%s'", name, value)
		}
	}
	checked := []string{
		"webkit-theme", "webkit-palette-light", "webkit-palette-dark",
		"webkit-font", "webkit-size", "webkit-fixation",
	}
	for key := range valueTable {
		if !slices.Contains(checked, key) {
			t.Errorf("value table key %s has no kit declaration checked here", key)
		}
	}
}
