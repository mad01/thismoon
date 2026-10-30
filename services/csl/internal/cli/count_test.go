package cli

import (
	"bytes"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/search"
)

// TestPrintCount: --names prints one group per line and nothing else, so a
// pipe can take the list as is; --group-by alone keeps the counts and total.
func TestPrintCount(t *testing.T) {
	prevGroup, prevNames, prevJSON := countGroupByFlag, countNamesFlag, countJSONFlag
	t.Cleanup(func() { countGroupByFlag, countNamesFlag, countJSONFlag = prevGroup, prevNames, prevJSON })

	results := []search.CountResult{{Group: "org/a", Count: 3}, {Group: "org/b", Count: 1}}

	for _, tt := range []struct {
		name         string
		group        string
		names, json_ bool
		want         string
	}{
		{name: "names", group: "repo", names: true, want: "org/a\norg/b\n"},
		{name: "grouped", group: "repo", want: "org/a                                    3\norg/b                                    1\n\ntotal: 4\n"},
		{name: "total", want: "4\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			countGroupByFlag, countNamesFlag, countJSONFlag = tt.group, tt.names, tt.json_

			var buf bytes.Buffer
			if err := printCount(&buf, results, 4); err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}
