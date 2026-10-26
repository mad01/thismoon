package main

import (
	"strings"
	"testing"
)

// TestSeedCasesLoad keeps the committed cases loadable: a typo in a grader
// field or a bad regex fails here, not in a paid run.
func TestSeedCasesLoad(t *testing.T) {
	cases, err := loadCases("cases")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 8 {
		t.Errorf("loaded %d seed cases, want at least 8", len(cases))
	}
	for _, c := range cases {
		if !strings.HasSuffix(c.Path, c.ID+".md") {
			t.Errorf("%s: file name does not match id %q", c.Path, c.ID)
		}
	}
}
