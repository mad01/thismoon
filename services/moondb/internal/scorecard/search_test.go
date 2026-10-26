package scorecard

import (
	"cmp"
	"testing"
)

// TestClassifySearch reads csl results in each form csl answers with: the
// text grammar (services/csl, render_text.go), the JSON format, an error,
// a result saved to a file, and a subagent's result cut to its head. Each
// case is a csl_search unless it names another tool.
func TestClassifySearch(t *testing.T) {
	const zeroHint = "no matches\nparsed query: (and case_regex:\"retry\" repo:(?i)acme/repo$)\n" +
		"repos searched: 1 (21 indexed, 20 discovered)\n" +
		"index age: newest 2026-10-24T17:18:37+02:00, oldest 2026-09-21T16:34:13+02:00"
	tests := []struct {
		name    string
		tool    string // the csl tool, csl_search when empty
		content string // the tool_result content, as JSON
		isError bool
		agent   bool // a subagent's line, its result a cut string
		want    searchOutcome
	}{
		{name: "files", content: texts("acme/repo/a.go\nacme/repo/b.go\n\n2 files")},
		{
			name:    "zero without a hint",
			content: texts("no matches"),
			want:    searchOutcome{zero: true},
		},
		{
			name: "zero with a hint", content: texts(zeroHint),
			want: searchOutcome{zero: true, zeroHint: true},
		},
		{
			name: "files cut at the limit",
			content: texts(
				"acme/repo/a.go\n\n10 files\ntruncated: showing 10 of 40 files; next offset=10",
			),
			want: searchOutcome{truncated: true},
		},
		{
			name: "lines cut at the cap",
			content: texts("acme/repo/a.go\n12:retry()\n\n300 lines in 4 files\n" +
				"truncated: showing 300 of 912 lines; next offset=3"),
			want: searchOutcome{truncated: true},
		},
		{
			name: "relaxed",
			content: texts("relaxed: dropped 'request' (0 files); showing results for `pull`\n\n" +
				"acme/repo/CLAUDE.md\n\n1 file"),
			want: searchOutcome{relaxed: true},
		},
		{
			name: "hit lines that name the trailer words are hits",
			content: texts("acme/repo/tools.go\n12:\t// no matches is the zero line\n" +
				"13-zero_result_hint: ...\n--\n40:truncated: no\n\n3 lines in 1 file"),
		},
		{
			name: "json zero with a hint",
			content: str(`{"output_mode":"files_with_matches","total":0,"truncated":false,` +
				`"zero_result_hint":{"parsed_query":"x","repos_searched":3}}`),
			want: searchOutcome{zero: true, zeroHint: true},
		},
		{
			name: "json truncated",
			content: texts(`{"files":[{"path":"kit/a.go","repo":"acme/repo"}],` +
				`"output_mode":"files_with_matches","total":3,"total_available":9,"truncated":true}`),
			want: searchOutcome{truncated: true},
		},
		{
			name: "json hit text that names a key is a hit",
			content: texts(
				`{"lines":[{"text":"zero_result_hint total 0"}],"total":1,"truncated":false}`,
			),
		},
		{
			name: "semantic, no hits", tool: cslSemantic, content: texts("0 hits"),
			want: searchOutcome{zero: true},
		},
		{
			name: "semantic snippet with a trailer word", tool: cslSemantic,
			content: texts("acme/repo/a.go:10-24 score=0.83 kind=func\n" +
				"// truncated: showing 10 of 40 files\n\n1 hit"),
		},
		{
			name: "semantic unavailable", tool: cslSemantic,
			content: texts("semantic search unavailable"),
		},
		{
			name: "hybrid, one hit", tool: cslHybrid,
			content: texts("acme/repo/a.go score=0.0312 lex=#3 L71\nL71:retry()\n\n1 hit"),
		},
		{
			name: "hybrid snippet with a trailer word", tool: cslHybrid,
			content: texts("lexical-only results; semantic search unavailable\n" +
				"acme/repo/a.go score=0.0312 lex=#3 L71\nL71:no matches\n" +
				"truncated: showing 1 of 2\n\n1 hit"),
		},
		{
			name: "json semantic, no hits", tool: cslSemantic, content: texts(`{"available":true}`),
			want: searchOutcome{zero: true},
		},
		{
			name: "json semantic unavailable", tool: cslSemantic,
			content: texts(`{"available":false,"note":"run csl semantic index first"}`),
		},
		{
			name: "json semantic hits", tool: cslSemantic,
			content: texts(`{"available":true,"hits":[{"repo":"acme/repo","path":"a.go"}]}`),
		},
		{
			name: "json hybrid, no hits", tool: cslHybrid,
			content: texts(`{"semantic_available":true}`), want: searchOutcome{zero: true},
		},
		{
			name: "json hybrid, lexical hits", tool: cslHybrid,
			content: texts(`{"semantic_available":false,"hits":[{"repo":"acme/repo"}],"note":"x"}`),
		},
		{
			name: "failed", isError: true, content: str("query has an unbalanced double quote"),
			want: searchOutcome{failed: true},
		},
		{
			name: "saved to a file",
			content: texts("<persisted-output>\nOutput too large (55KB). Full output saved to: " +
				"/tmp/r.json\n\nPreview (first 2KB):\nacme/repo/a.go"),
			want: searchOutcome{unread: true},
		},
		{
			name: "agent: zero survives the head", agent: true,
			content: "no matches\nparsed query: (and case_regex:\"x\")\nrepos searched: 1 (21 ...[+164 bytes]",
			want:    searchOutcome{zero: true, zeroHint: true},
		},
		{
			name: "agent: hits cut before the trailer", agent: true,
			content: "acme/repo/app.js\n2189:  stream = new EventSource(base);\n--\n...[+490 bytes]",
			want:    searchOutcome{unread: true},
		},
		{
			name: "agent: json zero in the head", agent: true,
			content: `{"output_mode":"files_with_matches","total":0,"truncated":false,"zero_result_hint":{"new...[+300 bytes]`,
			want:    searchOutcome{zero: true, zeroHint: true},
		},
		{
			name: "agent: json files cut", agent: true,
			content: `{"files":[{"path":"a.go","repo":"acme/repo"},{"path":"b.go"...[+900 bytes]`,
			want:    searchOutcome{unread: true},
		},
		{
			name: "agent: semantic hits cut", agent: true, tool: cslSemantic,
			content: "acme/repo/a.go:10-24 score=0.83 kind=func\nfunc retry() {...[+900 bytes]",
			want:    searchOutcome{unread: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := "mcp__csl__" + cmp.Or(tt.tool, cslSearch)
			lines := chunk(use(1, "toolu_s", name, searchInput),
				result(2, "toolu_s", tt.content, tt.isError, ""))
			agent := ""
			if tt.agent {
				agent = "agent-1"
				lines = chunk(agentUse(1, "toolu_s", name, searchInput),
					agentResult(2, "toolu_s", tt.content))
			}
			calls := ParseCalls(lines, agent)
			if len(calls) != 1 {
				t.Fatalf("calls = %+v", calls)
			}
			if got := classifySearch(calls[0]); got != tt.want {
				t.Errorf("outcome = %+v, want %+v (result %q)", got, tt.want, calls[0].Result)
			}
		})
	}
}

func TestClassifySearchUnanswered(t *testing.T) {
	c := Call{Name: "mcp__csl__csl_search"}
	if got := classifySearch(c); got != (searchOutcome{unread: true}) {
		t.Errorf("outcome = %+v, want unread", got)
	}
}

func TestSearchStatsCountsOnlySearches(t *testing.T) {
	calls := ParseCalls(chunk(
		use(1, "s1", "mcp__csl__csl_search", searchInput),
		result(2, "s1", texts("no matches\nparsed query: x"), false, ""),
		use(3, "s2", "mcp__csl__csl_hybrid_search", `{"query":"retry"}`),
		result(4, "s2", texts("0 hits"), false, ""),
		use(5, "s3", "mcp__csl__csl_search", searchInput),
		result(
			6,
			"s3",
			texts("a.go\n\n10 files\ntruncated: showing 10 of 11 files; next offset=10"),
			false,
			"",
		),
		use(7, "r1", "mcp__csl__csl_read", `{"repo":"acme/repo","file":"a.go"}`),
		result(8, "r1", texts("no matches"), false, ""),
		use(9, "g1", "Grep", `{"pattern":"retry"}`),
		result(10, "g1", str("No matches found"), false, ""),
	), "")
	got := searchStats(calls)
	want := SearchStats{Calls: 3, Zero: 2, ZeroResultHint: 1, Truncated: 1}
	if got != want {
		t.Errorf("stats = %+v, want %+v", got, want)
	}
}
