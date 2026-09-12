package mcpformat

import (
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
)

// encodeCSV renders the primary array as an RFC 4180 table: meta fields as
// leading "# key: value" comment lines, a header that is the union of record
// keys in first-seen order, then one row per record. Nested values become
// compact JSON; null or absent values an empty cell. Without a primary array
// the top-level fields become a two-column key,value table.
func encodeCSV(root *node) (string, error) {
	if root.kind != kindObject {
		return "", errors.New("mcpformat: csv needs an object at the top level")
	}
	idx, ok := primaryArray(root)
	if !ok {
		return writeCSV(nil, keyValueRows(root))
	}
	var comments []string
	for i, f := range root.fields {
		if i != idx {
			comments = append(comments, metaComment(f))
		}
	}
	records := root.fields[idx].value.items
	header := unionKeys(records)
	rows := make([][]string, 0, len(records)+1)
	rows = append(rows, header)
	for _, rec := range records {
		rows = append(rows, recordRow(rec, header))
	}
	return writeCSV(comments, rows)
}

func writeCSV(comments []string, rows [][]string) (string, error) {
	var b strings.Builder
	for _, c := range comments {
		b.WriteString(c)
		b.WriteByte('\n')
	}
	w := csv.NewWriter(&b)
	if err := w.WriteAll(rows); err != nil {
		return "", fmt.Errorf("mcpformat: write csv: %w", err)
	}
	return b.String(), nil
}

// metaComment renders one meta field as a "# key: value" line. A string
// holding a newline is JSON-quoted so the comment stays on one line.
func metaComment(f field) string {
	text := cellText(f.value)
	if f.value.kind == kindString && strings.Contains(text, "\n") {
		text = jsonString(text)
	}
	if text == "" {
		return "# " + f.key + ":"
	}
	return "# " + f.key + ": " + text
}

// cellText is a value's text in a cell: scalars bare, null empty, containers
// as compact JSON.
func cellText(n *node) string {
	if n.isScalar() {
		return n.scalarText()
	}
	return n.compactJSON()
}

// unionKeys collects every key across the records in first-seen order.
func unionKeys(records []*node) []string {
	var keys []string
	seen := map[string]bool{}
	for _, rec := range records {
		for _, f := range rec.fields {
			if !seen[f.key] {
				seen[f.key] = true
				keys = append(keys, f.key)
			}
		}
	}
	return keys
}

func recordRow(rec *node, header []string) []string {
	row := make([]string, len(header))
	for i, key := range header {
		if v, ok := rec.lookup(key); ok {
			row[i] = cellText(v)
		}
	}
	return row
}

func keyValueRows(root *node) [][]string {
	rows := make([][]string, 0, len(root.fields)+1)
	rows = append(rows, []string{"key", "value"})
	for _, f := range root.fields {
		rows = append(rows, []string{f.key, cellText(f.value)})
	}
	return rows
}
