package mcpserver

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/lexical"
	"github.com/mad01/thismoon/services/csl/internal/lexical/lexicaltest"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// fixtureIndex indexes the shared two-file test/repo (lexicaltest.Index).
func fixtureIndex(t *testing.T) (lexical.Local, []finder.Repo) {
	t.Helper()
	indexDir, repo := lexicaltest.Index(t)
	b := lexical.Local{IndexDir: indexDir, RepoNames: map[string]string{repo.Name: repo.Path}}
	return b, []finder.Repo{repo}
}

func TestExplainZero_FixtureRelaxesAbsentTerm(t *testing.T) {
	b, repos := fixtureIndex(t)
	in := searchInput{Query: "IndexRepo symbolSections zzznotaterm"}

	out := goldenExplain(t, b, in, repos)

	if out.RelaxedQuery != "IndexRepo symbolSections" {
		t.Errorf("relaxed_query = %q, want %q", out.RelaxedQuery, "IndexRepo symbolSections")
	}
	if !reflect.DeepEqual(out.DroppedTerms, []string{"zzznotaterm"}) {
		t.Errorf("dropped_terms = %q", out.DroppedTerms)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "a.go" || out.ZeroHint != nil {
		t.Errorf("files = %+v hint = %v, want a.go only and no hint", out.Files, out.ZeroHint)
	}
	text := renderSearchText(out)
	wantLine := "relaxed: dropped 'zzznotaterm' (0 files); showing results for `IndexRepo symbolSections`"
	if !strings.HasPrefix(text, wantLine+"\n\n") {
		t.Errorf("text rendering:\n%s\nwant it to open with %q", text, wantLine)
	}
}

func TestExplainZero_FixtureCountsFilesNotMatches(t *testing.T) {
	b, repos := fixtureIndex(t)

	out := goldenExplain(t, b, searchInput{Query: "IndexRepo Blocks"}, repos)

	if out.RelaxedQuery != "" || out.Total != 0 || out.ZeroHint == nil {
		t.Fatalf("want a zero result with a hint, got %+v", out)
	}
	if !strings.Contains(renderSearchText(out), "files per term: IndexRepo=1, Blocks=1") {
		t.Errorf("text rendering lacks the per-term line:\n%s", renderSearchText(out))
	}
}

// TestExplainZero_EchoesOffsetPastFirstPage: an empty page past the first
// keeps the requested offset, so the reader sees which page came back empty.
func TestExplainZero_EchoesOffsetPastFirstPage(t *testing.T) {
	b, repos := fixtureIndex(t)

	out := goldenExplain(t, b, searchInput{Query: "IndexRepo zzznotaterm", Offset: 50}, repos)

	if out.Offset != 50 || out.RelaxedQuery != "" {
		t.Errorf("offset=%d relaxed=%q, want the requested 50 and no relaxation",
			out.Offset, out.RelaxedQuery)
	}
}

// TestExplainZero_NotesNameTheTools: csl_search's notes point at the MCP
// tools, including the parameter-name trap only the tool has.
func TestExplainZero_NotesNameTheTools(t *testing.T) {
	b, repos := fixtureIndex(t)

	out := goldenExplain(t, b, searchInput{Query: "output_mode context_lines"}, repos)
	if !hasNote(out.ZeroHint.Notes, "parameter names") {
		t.Errorf("notes = %q, want the parameter-name trap", out.ZeroHint.Notes)
	}

	out = goldenExplain(t, b, searchInput{Query: "zzz", Repo: "nomatch"}, repos)
	if !hasNote(out.ZeroHint.Notes, "check the name with csl_repo_lookup") {
		t.Errorf("notes = %q, want csl_repo_lookup named", out.ZeroHint.Notes)
	}
}

func TestHandleQueryValidate_MalformedAndSplit(t *testing.T) {
	_, out, err := handleQueryValidate(context.Background(), nil, queryValidateInput{Query: `"`})
	if err != nil {
		t.Fatalf("malformed query returned an error, want valid=false: %v", err)
	}
	if out.Valid || !strings.Contains(out.Error, "only quote characters") || out.Hint == "" {
		t.Errorf("validate(%q) = %+v, want valid=false with problem and fix", `"`, out)
	}

	_, out, err = handleQueryValidate(context.Background(), nil,
		queryValidateInput{Query: `retry f:\.go$ "back off" -test`})
	if err != nil || !out.Valid {
		t.Fatalf("valid query: err=%v out=%+v", err, out)
	}
	if !reflect.DeepEqual(out.Terms, []string{"retry", `"back off"`}) {
		t.Errorf("terms = %q", out.Terms)
	}
	if !reflect.DeepEqual(out.Filters, []string{`f:\.go$`, "-test"}) {
		t.Errorf("filters = %q", out.Filters)
	}

	_, out, _ = handleQueryValidate(
		context.Background(),
		nil,
		queryValidateInput{Query: "output_mode"},
	)
	if !strings.Contains(out.Hint, "parameter names") {
		t.Errorf("validate(output_mode).hint = %q, want the parameter-name trap", out.Hint)
	}
}

func hasNote(notes []string, substr string) bool {
	for _, n := range notes {
		if strings.Contains(n, substr) {
			return true
		}
	}
	return false
}

// TestHandleQueryValidate_RefusedSymbolShape: a sym: term zoekt's symbol
// matcher cannot run and csl cannot split is valid=false with the fix, the
// same refusal csl_search returns instead of zoekt's internal error.
func TestHandleQueryValidate_RefusedSymbolShape(t *testing.T) {
	_, out, err := handleQueryValidate(
		context.Background(),
		nil,
		queryValidateInput{Query: "sym:(Hello|Point)+"},
	)
	if err != nil {
		t.Fatalf("refused sym: shape returned an error, want valid=false: %v", err)
	}
	if out.Valid || !strings.Contains(out.Error, "repeats an alternation") ||
		!strings.Contains(out.Hint, "sym:Foo or sym:Bar") {
		t.Errorf("validate(sym:(Hello|Point)+) = %+v, want valid=false with the fix", out)
	}

	_, out, err = handleQueryValidate(
		context.Background(),
		nil,
		queryValidateInput{Query: "sym:Hello|Point"},
	)
	if err != nil || !out.Valid || !strings.Contains(out.Parsed, `sym:case_substr:"Point"`) {
		t.Errorf("validate(sym:Hello|Point) = %+v, %v; want the split tree", out, err)
	}
}
