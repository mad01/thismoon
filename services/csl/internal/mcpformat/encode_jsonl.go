package mcpformat

import "strings"

// encodeJSONL writes the meta fields as one compact object on the first line,
// then one compact object per primary-array record, each line newline
// terminated. Without a primary array the whole value is a single line.
func encodeJSONL(root *node) string {
	var b strings.Builder
	idx, ok := primaryArray(root)
	if !ok {
		b.WriteString(root.compactJSON())
		b.WriteByte('\n')
		return b.String()
	}
	b.WriteString(root.without(idx).compactJSON())
	b.WriteByte('\n')
	for _, rec := range root.fields[idx].value.items {
		b.WriteString(rec.compactJSON())
		b.WriteByte('\n')
	}
	return b.String()
}
