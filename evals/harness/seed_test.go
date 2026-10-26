package main

import (
	"context"
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

// TestLargeFileGraders pins the ranged-read case to the exact file: a ranged
// read of it passes, an unranged read of it fails, and reads of other files
// count for neither.
func TestLargeFileGraders(t *testing.T) {
	cases, err := loadCases("cases")
	if err != nil {
		t.Fatal(err)
	}
	c, err := selectCases(cases, "large-file-ranged-read")
	if err != nil {
		t.Fatal(err)
	}
	const file = "/src/thismoon/tools/belt/internal/config/config.go"
	read := func(input string) ToolCall { return ToolCall{Name: "Read", Input: input, Origin: originMain} }
	tests := []struct {
		name        string
		calls       []ToolCall
		used, clean bool
	}{
		{
			"ranged read",
			[]ToolCall{read(`{"file_path":"` + file + `","limit":40,"offset":1060}`)},
			true,
			true,
		},
		{
			"ranged csl_read",
			[]ToolCall{{
				Name: "mcp__csl__csl_read", Origin: originMain,
				Input: `{"end_line":1080,"file":"tools/belt/internal/config/config.go","repo":"thismoon","start_line":1060}`,
			}},
			true, true,
		},
		{"full read", []ToolCall{read(`{"file_path":"` + file + `"}`)}, false, false},
		{
			"ranged read of another file",
			[]ToolCall{read(`{"file_path":"/src/x/config.go","limit":5,"offset":1}`)},
			false,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := Evidence{Transcript: Transcript{ToolCalls: tt.calls}}
			for _, g := range c[0].Graders {
				want, ok := map[string]bool{gradeToolUsed: tt.used, gradeToolNotUsed: tt.clean}[g.Type]
				if !ok {
					continue
				}
				if res, _ := evaluate(context.Background(), g, ev, nil); res.Pass != want {
					t.Errorf("%s: pass = %v, want %v", g.Type, res.Pass, want)
				}
			}
		})
	}
}
