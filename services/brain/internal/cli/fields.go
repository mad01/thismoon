package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// fieldFlags are the frontmatter flags write and edit share: --evidence,
// repeatable --set key=value and repeatable --unset key.
type fieldFlags struct {
	evidence string
	set      []string
	unset    []string
}

func (f *fieldFlags) register(c *cobra.Command) {
	c.Flags().StringVar(&f.evidence, "evidence", "",
		"evidence tier for empirical notes: A, B, C, D or mixed")
	// StringArray, not StringSlice: a value like [a,b] must not split on commas.
	c.Flags().StringArrayVar(&f.set, "set", nil,
		"set a frontmatter key to a YAML value, e.g. owner=alex or 'tags=[a, b]' (repeatable)")
	c.Flags().StringArrayVar(&f.unset, "unset", nil, "remove a frontmatter key (repeatable)")
}

// given reports whether any frontmatter flag was passed.
func (f *fieldFlags) given() bool {
	return f.evidence != "" || len(f.set) > 0 || len(f.unset) > 0
}

// parse validates --set and --unset and returns the keys to set and remove.
func (f *fieldFlags) parse() ([]markdown.Field, []string, error) {
	set, err := parseSets(f.set)
	if err != nil {
		return nil, nil, err
	}
	unset, err := parseUnsets(f.unset)
	if err != nil {
		return nil, nil, err
	}
	return set, unset, nil
}

// parseSets turns key=value flags into frontmatter fields. The value is
// parsed as YAML and checked against the typed key it targets, so a bad
// value is a usage error before any file is touched.
func parseSets(raw []string) ([]markdown.Field, error) {
	fields := make([]markdown.Field, 0, len(raw))
	for _, kv := range raw {
		key, value, ok := strings.Cut(kv, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, usageErrorf("--set %q: want key=value", kv)
		}
		if strings.TrimSpace(value) == "" {
			return nil, usageErrorf(
				"--set %s: empty value; remove the key with --unset %s",
				key,
				key,
			)
		}
		node, err := markdown.ParseValue(value)
		if err != nil {
			return nil, usageErrorf("--set %s: %w", key, err)
		}
		var probe markdown.Frontmatter
		if err := probe.Set(key, node); err != nil {
			return nil, usageErrorf("--set %s: %w", key, err)
		}
		fields = append(fields, markdown.Field{Key: key, Value: node})
	}
	return fields, nil
}

// parseUnsets validates --unset keys.
func parseUnsets(raw []string) ([]string, error) {
	keys := make([]string, 0, len(raw))
	for _, k := range raw {
		k = strings.TrimSpace(k)
		if k == "" {
			return nil, usageErrorf("--unset: empty key")
		}
		keys = append(keys, k)
	}
	return keys, nil
}
