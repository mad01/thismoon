package catalog

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// CheckUnique verifies that entity names are unique across the whole catalog.
// Systems and Components share a single global namespace, so a System and a
// Component may not share a name either. It returns an error listing every
// collision (with the kinds and source files involved), or nil when all names
// are distinct. This is the rule enforced by `catalog validate` and the
// catalog repo's PR check.
func CheckUnique(entities []Entity) error {
	groups := make(map[string][]Entity)
	for _, e := range entities {
		groups[e.Metadata.Name] = append(groups[e.Metadata.Name], e)
	}

	var dupes []string
	for name, group := range groups {
		if len(group) > 1 {
			dupes = append(dupes, name)
		}
	}
	if len(dupes) == 0 {
		return nil
	}
	sort.Strings(dupes)

	var b strings.Builder
	b.WriteString("duplicate names in catalog namespace (names must be globally unique across Systems and Components):")
	for _, name := range dupes {
		fmt.Fprintf(&b, "\n  %q declared %d times:", name, len(groups[name]))
		for _, e := range groups[name] {
			loc := e.SourcePath
			if loc == "" {
				loc = "(unknown source)"
			}
			fmt.Fprintf(&b, "\n    - %s at %s", e.Kind, loc)
		}
	}
	return errors.New(b.String())
}
