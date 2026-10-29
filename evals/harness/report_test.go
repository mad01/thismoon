package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteCompare(t *testing.T) {
	a := []Row{
		{
			Case:       "where-defined",
			Rep:        0,
			Score:      1,
			CostUSD:    0.20,
			Turns:      4,
			Tokens:     50000,
			ToolCalls:  3,
			CSLCalls:   1,
			CPUSeconds: 30,
			WallMS:     20000,
		},
		{
			Case:       "where-defined",
			Rep:        1,
			Score:      0.5,
			CostUSD:    0.40,
			Turns:      6,
			Tokens:     70000,
			ToolCalls:  5,
			CSLCalls:   3,
			CPUSeconds: 10,
			WallMS:     40000,
		},
		{Case: "dropped", Rep: 0, Score: 1, CostUSD: 0.10, Turns: 2, Tokens: 1000, ToolCalls: 1},
	}
	b := []Row{
		{
			Case:       "where-defined",
			Rep:        0,
			Score:      1,
			CostUSD:    0.10,
			Turns:      3,
			Tokens:     40000,
			ToolCalls:  2,
			CSLCalls:   2,
			CPUSeconds: 5,
			WallMS:     15000,
		},
		{Case: "added", Rep: 0, Score: 0, CostUSD: 0.05, Turns: 1, Tokens: 500, ToolCalls: 0},
	}
	var buf bytes.Buffer
	writeCompare(
		&buf,
		compareSide{
			Label: "before",
			Meta:  runMeta{ClaudeVersion: "2.1.310", Effort: "high"},
			Rows:  a,
		},
		compareSide{
			Label: "after",
			Meta:  runMeta{ClaudeVersion: "2.1.311", Models: []string{"m1"}},
			Rows:  b,
		},
	)
	out := buf.String()
	for _, want := range []string{
		"# Compare before to after",
		"A: 2.1.310, model user default, effort high.",
		"B: 2.1.311, model m1, effort default.",
		// Run A's means are score 0.75, cost 0.30, turns 5, tokens 60k, calls 4,
		// csl 2, 30 seconds wall, 20 CPU seconds.
		"| where-defined | 2/1 | 1.00 (+0.25) | $0.100 (-0.200) | 3.0 (-2.0) | 40.0k (-20.0k) | 2.0 (-2.0) " +
			"| 2.0 (+0.0) | 15 (-15) | 5 (-15) |",
		"| added | 0/1 | only in B |",
		"| dropped | 1/0 | only in A |",
		// Totals cover where-defined alone, the one case both runs have, at
		// one rep each: A's means against B's.
		"Totals over the 1 of 3 cases both runs have, one rep each: " +
			"mean score 1.00 (+0.25), cost $0.100 (-0.200), tool calls 2.0 (-2.0), " +
			"csl calls 2.0 (+0.0), 15 seconds (-15), 5 CPU seconds (-15).",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("compare output missing %q\n%s", want, out)
		}
	}
	if strings.Index(out, "| added") > strings.Index(out, "| dropped") {
		t.Error("cases are not sorted by id")
	}
}

func TestWriteSummary(t *testing.T) {
	meta := runMeta{
		RunID: "20261026-120000", ClaudeVersion: "2.1.310 (Claude Code)",
		Cases: []string{"a"}, Reps: 1, Stopped: "interrupted",
	}
	rows := []Row{{
		Case: "a", Score: 0.5, Turns: 3, ToolCalls: 2, CSLCalls: 1, Tokens: 1500, CostUSD: 0.25,
		WallMS: 12000, CPUSeconds: 4.4, Stop: "success", Tools: map[string]int{"Read": 1, "Bash": 1},
		AgentToolCalls: 2, AgentTools: map[string]int{"Grep": 2},
		Graders: []GraderResult{
			{Grader: "regex x", Weight: 1, Pass: true},
			{Grader: "max_turns 2", Weight: 1, Detail: "3 turns, max 2"},
		},
		ScorecardStatus: scorecardUnavailable,
		Error:           "claude: exit status 1",
	}}
	var buf bytes.Buffer
	writeSummary(&buf, meta, rows)
	out := buf.String()
	for _, want := range []string{
		"2.1.310 (Claude Code), model user default, effort default, cases 1, reps 1.",
		"Stopped early: interrupted.",
		"- a rep 0: Bash 1, Read 1; subagents: Grep 2",
		"1 rows, 0 passed, mean score 0.50",
		"1.5k tokens, 2 tool calls (1 csl), 12s wall, 4 CPU seconds.",
		"| a | 0 | 0.50 | no | 3 | 2 | 1 | 1.5k | $0.250 | 12 | 4 | success |",
		"- a rep 0: Bash 1, Read 1",
		"- a rep 0: `max_turns 2`: 3 turns, max 2",
		"- a rep 0: error: claude: exit status 1",
		"Scorecard: 1 of 1 rows unavailable.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "`regex x`") {
		t.Error("summary lists a passing grader as a failure")
	}
}

func TestWriteCompareNoSharedCases(t *testing.T) {
	var buf bytes.Buffer
	writeCompare(&buf,
		compareSide{Label: "a", Rows: []Row{{Case: "x", Score: 1, CostUSD: 1}}},
		compareSide{Label: "b", Rows: []Row{{Case: "y", Score: 0, CostUSD: 2}}})
	out := buf.String()
	if !strings.Contains(out, "No cases in common") || strings.Contains(out, "Totals") {
		t.Errorf("disjoint runs should print no totals:\n%s", out)
	}
}
