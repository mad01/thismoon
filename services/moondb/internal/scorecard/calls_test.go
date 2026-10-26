package scorecard

import (
	"testing"
	"time"
)

func TestParseCallsPairsUsesWithResults(t *testing.T) {
	lines := chunk(
		`{"type":"api-request","timestamp":"`+at(0)+`"}`,
		use(1, "toolu_read", "Read", `{"file_path":"/w/repo/a.go"}`),
		result(2, "toolu_read", str("1\tpackage a\n2\t\n3\tfunc A() {}"), false,
			fileOwn("/w/repo/a.go", 3, 1, 3)),
		use(3, "toolu_search", "mcp__csl__csl_search", searchInput),
		result(4, "toolu_search", texts("no matches"), false, ""),
		use(5, "toolu_bad", "mcp__csl__csl_search", `{"query":"\"open"}`),
		result(6, "toolu_bad", str("query has an unbalanced double quote"), true, ""),
		use(7, "toolu_open", "Bash", `{"command":"sleep 600"}`),
		`not json`,
	)
	calls := ParseCalls(lines, "")
	if len(calls) != 4 {
		t.Fatalf("calls = %+v, want 4", calls)
	}
	read := calls[0]
	if read.Name != "Read" || !read.Answered || read.Lines != 3 || read.TotalLines != 3 ||
		read.AgentID != "" || !read.At.Equal(t0.Add(time.Second)) {
		t.Errorf("read = %+v", read)
	}
	if calls[1].Result != "no matches" || calls[1].Lines != unknownLines {
		t.Errorf("search = %+v, want its text and no line count", calls[1])
	}
	if !calls[2].IsError || calls[2].Result != "query has an unbalanced double quote" {
		t.Errorf("failed search = %+v", calls[2])
	}
	if calls[3].Answered {
		t.Errorf("unanswered call = %+v, want Answered false", calls[3])
	}
}

// TestParseCallsAgentTranscript: a subagent's lines are sidechain lines,
// read for an agent and skipped in a turn's lines, and a result cut to its
// head is marked Cut.
func TestParseCallsAgentTranscript(t *testing.T) {
	lines := chunk(
		agentUse(1, "toolu_r", "Read", `{"file_path":"/w/repo/docs/guide.md"}`),
		agentResult(2, "toolu_r", "1\t# Guide\n2\t\n3\tSome processes run...[+2876 bytes]"),
	)
	if calls := ParseCalls(lines, ""); len(calls) != 0 {
		t.Errorf("a turn's parse = %+v, want the sidechain lines skipped", calls)
	}
	calls := ParseCalls(lines, "agent-1")
	if len(calls) != 1 {
		t.Fatalf("calls = %+v", calls)
	}
	if c := calls[0]; c.AgentID != "agent-1" || !c.Cut || c.Lines != unknownLines {
		t.Errorf("call = %+v, want agent-1's, cut, with no line count", c)
	}
}

// TestParseCallsSharedResultLine: a line answering two calls leaves both
// line counts unknown, since its toolUseResult belongs to neither alone.
func TestParseCallsSharedResultLine(t *testing.T) {
	both := `{"type":"user","timestamp":"` + at(3) + `","message":{"content":[` +
		`{"type":"tool_result","tool_use_id":"r1","content":"1\tx"},` +
		`{"type":"tool_result","tool_use_id":"r2","content":"1\ty"}]},` +
		`"toolUseResult":` + fileOwn("/w/a", 1, 1, 1) + `}`
	calls := ParseCalls(chunk(
		use(1, "r1", "Read", `{"file_path":"/w/a"}`),
		use(2, "r2", "Read", `{"file_path":"/w/b"}`),
		both,
	), "")
	for _, c := range calls {
		if !c.Answered || c.Lines != unknownLines {
			t.Errorf("call %s = %+v, want answered with no line count", c.ID, c)
		}
	}
}

func TestParseCallsPersistedOutputIsCut(t *testing.T) {
	preview := "<persisted-output>\nOutput too large (52.6KB). Full output saved to: " +
		"/tmp/tool-results/toolu_p.json\n\nPreview (first 2KB):\n[\n  {"
	calls := ParseCalls(chunk(
		use(1, "toolu_p", "mcp__csl__csl_search", searchInput),
		result(2, "toolu_p", texts(preview), false, ""),
	), "")
	if len(calls) != 1 || !calls[0].Cut {
		t.Errorf("calls = %+v, want one cut result", calls)
	}
}

func TestToolName(t *testing.T) {
	for in, want := range map[string]string{
		"mcp__csl__csl_search": "csl_search",
		"Read":                 "Read",
		"mcp__plugin_x__tool":  "tool",
	} {
		if got := toolName(in); got != want {
			t.Errorf("toolName(%q) = %q, want %q", in, got, want)
		}
	}
}
