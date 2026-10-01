package webkit

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// Theme is one palette family from the embedded collection: a light and a
// dark variant (either may be absent), each a complete map of resolved roles.
// The collection is authored under src/themes/ and compiled by build.mjs into
// dist/themes.json, which this package parses once at init.
type Theme struct {
	// Name is the family key: the value stored under webkit-palette-light or
	// webkit-palette-dark and carried by the html data-palette attribute.
	Name string `json:"name"`
	// Label is the name shown on the themes page.
	Label string `json:"label"`
	// Source is the upstream repository the values came from.
	Source string `json:"source"`
	// License is the upstream licence the values are used under.
	License string   `json:"license"`
	Light   *Variant `json:"light,omitempty"`
	Dark    *Variant `json:"dark,omitempty"`
}

// Variant is one mode of a family: its name, the authoring notes, and every
// role resolved to a literal colour (hex, or rgba for the alpha surfaces).
type Variant struct {
	Name  string            `json:"variant"`
	Notes string            `json:"notes,omitempty"`
	Roles map[string]string `json:"roles"`
}

// collection is the shape of dist/themes.json.
type collection struct {
	Roles    []string `json:"roles"`
	Families []Theme  `json:"families"`
}

var themes = loadThemes()

// loadThemes parses the embedded collection. A failure here is a build defect
// (the file is generated and validated by build.mjs), so it stops the binary
// at init rather than serving a kit with no palette.
func loadThemes() collection {
	b, err := distFS.ReadFile("dist/themes.json")
	if err != nil {
		panic(fmt.Sprintf("webkit: embedded dist/themes.json missing: %v", err))
	}
	var c collection
	if err := json.Unmarshal(b, &c); err != nil {
		panic(fmt.Sprintf("webkit: embedded dist/themes.json invalid: %v", err))
	}
	if len(c.Families) == 0 || c.Families[0].Name != "default" {
		panic("webkit: embedded dist/themes.json must list the default family first")
	}
	return c
}

// Themes returns the palette families the embedded stylesheet carries, the
// default first, each with its resolved light and dark roles. The result is a
// copy; callers may keep or modify it.
func Themes() []Theme {
	out := make([]Theme, len(themes.Families))
	for i, f := range themes.Families {
		out[i] = f
		out[i].Light = cloneVariant(f.Light)
		out[i].Dark = cloneVariant(f.Dark)
	}
	return out
}

func cloneVariant(v *Variant) *Variant {
	if v == nil {
		return nil
	}
	c := *v
	c.Roles = maps.Clone(v.Roles)
	return &c
}

// Roles returns the role names a page may reference by name, for a consumer
// that validates authored input against the palette (a panel accent, a chart
// series colour, a slide tone): primary, the six semantic colours (red, green,
// amber, yellow, blue, purple), series-1 to series-4, the three surfaces (bg,
// paper, chip), and the tone-<name>-bg tints (neutral, green, red, blue,
// amber, purple). Every family resolves every one of them, and each is
// declared as --<role> in webkit.css, so `var(--<role>)` is valid under any
// palette. The slice is a copy.
func Roles() []string {
	return slices.Clone(themes.Roles)
}

// IsRole reports whether name is one of Roles().
func IsRole(name string) bool {
	return slices.Contains(themes.Roles, name)
}
