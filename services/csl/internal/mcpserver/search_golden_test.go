package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/services/csl/internal/lexical"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// goldenPath holds the csl_search output the golden test pins. Set
// CSL_UPDATE_GOLDEN=1 to rewrite it after an intended change.
const goldenPath = "testdata/search_golden.txt"

// goldenCase is one zero-hit search the golden test runs through the
// csl_search zero-result path against the fixture index.
type goldenCase struct {
	name string
	in   searchInput
}

var goldenCases = []goldenCase{
	{"relaxed files", searchInput{Query: "IndexRepo symbolSections zzznotaterm"}},
	{"relaxed content", searchInput{
		Query: "IndexRepo zzznotaterm", OutputMode: contentOutputMode, ContextLines: 1,
	}},
	{"never together", searchInput{Query: "IndexRepo Blocks"}},
	{"parameter names", searchInput{Query: "output_mode context_lines"}},
	{"repo filter matches none", searchInput{Query: "zzznotaterm", Repo: "nomatch"}},
	{"repo filter unindexed", searchInput{Query: "IndexRepo Blocks", Repo: "TEST/REPO"}},
	{"file filter relaxes", searchInput{Query: "IndexRepo Blocks zzz", File: `b\.go$`}},
	{"syntax traps", searchInput{Query: `"IndexRepo Blocks" OR zzz | x`, File: `\.go$`}},
	{"too many terms", searchInput{Query: "a b c d e f g"}},
	{"past first page", searchInput{Query: "IndexRepo zzznotaterm", Offset: 50}},
}

var goldenShapes = []string{
	"", "   ", `"`, `""`, `foo "bar`, `foo\`, `foo "" bar`,
	`retry f:\.go$ "back off" -test`, "output_mode", "sym:(Hello|Point)+",
}

// TestSearchGolden pins what csl_search returns on the paths the shared
// lexical package serves: the zero-result hint, the relaxed rerun, the
// query-shape refusals, and the tool schemas. The CLI shares that code, so
// this golden is what proves the MCP side unchanged.
func TestSearchGolden(t *testing.T) {
	var b strings.Builder
	backend, repos := fixtureIndex(t)
	for _, c := range goldenCases {
		out := goldenExplain(t, backend, c.in, repos)
		fmt.Fprintf(&b, "== %s ==\n-- json --\n%s\n-- text --\n%s\n\n",
			c.name, mustJSON(t, out), renderSearchText(out))
	}
	for _, q := range goldenShapes {
		shape := "ok"
		if m := lexical.CheckShape(q, mcpSurface); m != nil {
			shape = m.Error()
		}
		_, v, err := handleQueryValidate(context.Background(), nil, queryValidateInput{Query: q})
		if err != nil {
			t.Fatalf("validate(%q): %v", q, err)
		}
		fmt.Fprintf(&b, "== shape %q ==\n%s\n%s\n\n", q, shape, mustJSON(t, v))
	}
	for _, tool := range mcptest.ListTools(t, New("test", Options{SemanticEnabled: true})) {
		switch tool.Name {
		case "csl_search", "csl_count", "csl_query_validate", "csl_hybrid_search":
			fmt.Fprintf(&b, "== schema %s ==\n%s\n%s\n\n",
				tool.Name, mustJSON(t, tool.InputSchema), mustJSON(t, tool.OutputSchema))
		}
	}
	compareGolden(t, goldenPath, b.String())
}

// goldenExplain runs the zero-result path handleSearch takes after an empty
// first page, with the options handleSearch builds from the input.
func goldenExplain(
	t *testing.T,
	b lexical.Local,
	in searchInput,
	repos []finder.Repo,
) searchOutput {
	t.Helper()
	mode := in.OutputMode
	if mode == "" {
		mode = defaultSearchOutput
	}
	opts := search.SearchOptions{
		Pattern:      in.Query,
		RepoFilter:   lexical.RepoFilter(in.Repo),
		FileFilter:   in.File,
		Limit:        defaultSearchLimit,
		Offset:       in.Offset,
		ContextLines: in.ContextLines,
		OutputMode:   mode,
	}
	req := lexical.Request{Options: opts, Repo: in.Repo, Repos: repos, IndexDir: b.IndexDir}
	return explainZero(context.Background(), b, req)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}

// compareGolden fails when got differs from the file at path, or rewrites
// the file when CSL_UPDATE_GOLDEN=1.
func compareGolden(t *testing.T, path, got string) {
	t.Helper()
	if os.Getenv("CSL_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (CSL_UPDATE_GOLDEN=1 writes it): %v", err)
	}
	if got != string(want) {
		t.Errorf(
			"csl_search output differs from %s; diff it with CSL_UPDATE_GOLDEN=1 and git diff\n--- got ---\n%s",
			path,
			got,
		)
	}
}
