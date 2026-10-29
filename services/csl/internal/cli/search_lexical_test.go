package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/lexical"
	"github.com/mad01/thismoon/services/csl/internal/lexical/lexicaltest"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// keepSearchFlags restores the package-level search flags after the test, so
// each test sets only the flags it is about.
func keepSearchFlags(t *testing.T) {
	t.Helper()
	mode, asJSON, toon := searchOutputModeFlag, searchJSONFlag, searchToonFlag
	repo, file, limit := searchRepoFlag, searchFileFlag, searchLimitFlag
	t.Cleanup(func() {
		searchOutputModeFlag, searchJSONFlag, searchToonFlag = mode, asJSON, toon
		searchRepoFlag, searchFileFlag, searchLimitFlag = repo, file, limit
	})
	searchOutputModeFlag, searchJSONFlag, searchToonFlag = "files_with_matches", false, false
	searchRepoFlag, searchFileFlag, searchLimitFlag = "", "", 50
}

// runReport runs reportSearch for pattern against the shared fixture index
// the way runSearch does after an empty or non-empty first search, and
// returns stdout and stderr.
func runReport(t *testing.T, pattern string) (string, string) {
	t.Helper()
	indexDir, repo := lexicaltest.Index(t)
	local := lexical.Local{IndexDir: indexDir, RepoNames: map[string]string{repo.Name: repo.Path}}
	req := lexical.Request{
		Options:  searchOptions(pattern),
		Repo:     searchRepoFlag,
		Repos:    []finder.Repo{repo},
		IndexDir: indexDir,
	}
	matches, err := local.Search(context.Background(), req.Options)
	if err != nil {
		t.Fatalf("search %q: %v", pattern, err)
	}

	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := reportSearch(context.Background(), cmd, local, req, matches); err != nil {
		t.Fatalf("reportSearch: %v", err)
	}
	return stdout.String(), stderr.String()
}

// TestRunSearch_RefusesMalformedQuery: a query zoekt cannot run, or one sent
// by mistake, fails before any search with the problem and its fix.
func TestRunSearch_RefusesMalformedQuery(t *testing.T) {
	keepSearchFlags(t)
	tests := []struct{ query, want string }{
		{query: "  ", want: "query is empty; put the text to search for"},
		{query: `""`, want: "only quote characters; put the search term itself"},
		{query: `foo "bar`, want: "unbalanced double quote; close the phrase"},
		{query: `foo\`, want: "lone backslash; double it"},
	}
	for _, tt := range tests {
		err := runSearch(&cobra.Command{}, []string{tt.query})
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("csl search %q: err = %v, want it to contain %q", tt.query, err, tt.want)
		}
	}
}

// TestReportSearch_ZeroHintOnStderr: an empty result keeps "no matches" on
// stdout and explains itself on stderr: the parsed query, files per term,
// repos searched, and notes naming CLI commands.
func TestReportSearch_ZeroHintOnStderr(t *testing.T) {
	keepSearchFlags(t)

	stdout, stderr := runReport(t, "IndexRepo Blocks")

	if stdout != "no matches\n" {
		t.Errorf("stdout = %q, want only the no-matches line", stdout)
	}
	for _, want := range []string{
		"parsed query: (and ",
		"files per term: IndexRepo=1, Blocks=1",
		"repos searched: 0 (0 indexed, 1 discovered)",
		"- every term matches files on its own but no file contains all 2; search IndexRepo|Blocks",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
}

// TestReportSearch_NotesNameCLICommands: the hint points a CLI reader at csl
// commands, not at the MCP tools, and skips the MCP parameter-name trap.
func TestReportSearch_NotesNameCLICommands(t *testing.T) {
	keepSearchFlags(t)

	searchRepoFlag = "nomatch"
	_, stderr := runReport(t, "zzznotaterm")
	if !strings.Contains(stderr, "check the name with csl repo --list") ||
		strings.Contains(stderr, "csl_repo_lookup") {
		t.Errorf("stderr = %q, want csl repo --list named and no MCP tool", stderr)
	}

	searchRepoFlag = "TEST"
	_, stderr = runReport(t, "output_mode context_lines")
	if !strings.Contains(stderr, "run csl index --repo <path> on them") {
		t.Errorf("stderr = %q, want csl index named for the unindexed repo", stderr)
	}
	if strings.Contains(stderr, "parameter names") {
		t.Errorf("stderr = %q, want no csl_search parameter-name note", stderr)
	}
}

// TestReportSearch_RelaxedRerun: when one AND term matches no file, the
// search reruns once without it; stderr says which term was dropped and
// stdout holds the rerun's results.
func TestReportSearch_RelaxedRerun(t *testing.T) {
	keepSearchFlags(t)

	stdout, stderr := runReport(t, "IndexRepo symbolSections zzznotaterm")

	wantErr := "relaxed: dropped 'zzznotaterm' (0 files); showing results for `IndexRepo symbolSections`\n"
	if stderr != wantErr {
		t.Errorf("stderr = %q, want %q", stderr, wantErr)
	}
	if stdout != "test/repo/a.go\n" {
		t.Errorf("stdout = %q, want the rerun's file", stdout)
	}
}

// TestReportSearch_RelaxedRerunContentMode: the rerun keeps the output mode,
// so content mode prints the rerun's lines.
func TestReportSearch_RelaxedRerunContentMode(t *testing.T) {
	keepSearchFlags(t)
	searchOutputModeFlag = "content"

	stdout, stderr := runReport(t, "Blocks zzznotaterm")

	if !strings.HasPrefix(stderr, "relaxed: dropped 'zzznotaterm'") {
		t.Errorf("stderr = %q, want the relaxed line", stderr)
	}
	if want := "test/repo/b.go\n3:6:func Blocks() {}\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestReportSearch_JSONCarriesFields: --json puts the matches under
// "matches" and the relaxed rerun or the hint beside them, with nothing on
// stderr.
func TestReportSearch_JSONCarriesFields(t *testing.T) {
	keepSearchFlags(t)
	searchJSONFlag = true

	stdout, stderr := runReport(t, "IndexRepo symbolSections zzznotaterm")
	var relaxed struct {
		Matches      []search.Match   `json:"matches"`
		RelaxedQuery string           `json:"relaxed_query"`
		DroppedTerms []string         `json:"dropped_terms"`
		ZeroHint     *json.RawMessage `json:"zero_result_hint"`
	}
	if err := json.Unmarshal([]byte(stdout), &relaxed); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if relaxed.RelaxedQuery != "IndexRepo symbolSections" ||
		len(relaxed.DroppedTerms) != 1 || relaxed.DroppedTerms[0] != "zzznotaterm" ||
		len(relaxed.Matches) == 0 || relaxed.ZeroHint != nil || stderr != "" {
		t.Errorf("relaxed --json = %+v, stderr %q", relaxed, stderr)
	}

	stdout, stderr = runReport(t, "IndexRepo Blocks")
	var zero struct {
		Matches  []search.Match    `json:"matches"`
		ZeroHint *lexical.ZeroHint `json:"zero_result_hint"`
	}
	if err := json.Unmarshal([]byte(stdout), &zero); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if !strings.Contains(stdout, `"matches": []`) {
		t.Errorf("--json = %s, want an empty matches array, not null", stdout)
	}
	if zero.ZeroHint == nil || len(zero.ZeroHint.TermCounts) != 2 || stderr != "" {
		t.Errorf("zero --json hint = %+v, stderr %q", zero.ZeroHint, stderr)
	}
}

// TestSearchOptions_RepoCaseInsensitive: --repo matches repo names the way
// csl_search's repo parameter does, whatever the case.
func TestSearchOptions_RepoCaseInsensitive(t *testing.T) {
	keepSearchFlags(t)
	indexDir, repo := lexicaltest.Index(t)
	local := lexical.Local{IndexDir: indexDir, RepoNames: map[string]string{repo.Name: repo.Path}}

	searchRepoFlag = "TEST/Repo"
	matches, err := local.Search(context.Background(), searchOptions("Blocks"))
	if err != nil || len(matches) == 0 || matches[0].Repo != "test/repo" {
		t.Errorf("--repo TEST/Repo: matches=%+v err=%v, want test/repo", matches, err)
	}

	got, err := reposMatching([]finder.Repo{
		{Name: "mad01/thismoon"}, {Name: "mad01/ralph"},
	}, "THISMOON")
	if err != nil || len(got) != 1 || got[0].Name != "mad01/thismoon" {
		t.Errorf("reposMatching(THISMOON) = %+v, %v; want mad01/thismoon", got, err)
	}
}

// TestReposMatching pins the repos the in-process path checks and indexes to
// the zoekt repo: filter the search applies: a case-insensitive regex, where an
// anchored name finds the one repo and not the repos that share its prefix.
func TestReposMatching(t *testing.T) {
	repos := []finder.Repo{
		{Name: "mad01/thismoon"},
		{Name: "mad01/thismoon-arcade"},
		{Name: "mad01/dotfiles"},
	}
	tests := []struct {
		expr string
		want []string
	}{
		{"^mad01/thismoon$", []string{"mad01/thismoon"}},
		{"thismoon", []string{"mad01/thismoon", "mad01/thismoon-arcade"}},
		{"dotfiles|arcade", []string{"mad01/thismoon-arcade", "mad01/dotfiles"}},
		{"^MAD01/", []string{"mad01/thismoon", "mad01/thismoon-arcade", "mad01/dotfiles"}},
		{"^MAD01/THISMOON$", []string{"mad01/thismoon"}},
		{"^other/", nil},
	}
	for _, tc := range tests {
		got, err := reposMatching(repos, tc.expr)
		if err != nil {
			t.Fatalf("reposMatching(%q) error: %v", tc.expr, err)
		}
		var names []string
		for _, r := range got {
			names = append(names, r.Name)
		}
		if !slices.Equal(names, tc.want) {
			t.Errorf("reposMatching(%q) = %q, want %q", tc.expr, names, tc.want)
		}
	}
	_, err := reposMatching(repos, "(")
	if err == nil || !strings.Contains(err.Error(), "--repo") {
		t.Errorf(`reposMatching("(") error = %v, want an invalid-regex error naming --repo`, err)
	}
}
