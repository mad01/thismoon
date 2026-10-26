package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/scorecard"
	"github.com/mad01/thismoon/services/moondb/internal/server/servertest"
)

func TestSessionScorecardResolvesShortIDs(t *testing.T) {
	c := traceClient(t)
	sc, err := sessionScorecard(context.Background(), c, "sess-o")
	if err != nil {
		t.Fatal(err)
	}
	if sc.SessionID != servertest.OldSession {
		t.Errorf("session = %q, want %q", sc.SessionID, servertest.OldSession)
	}
	_, err = sessionScorecard(context.Background(), c, "nope")
	if err == nil || !strings.Contains(err.Error(), `no session "nope"`) {
		t.Errorf("err = %v, want no session", err)
	}
}

// TestPrintScorecardWithoutPeers: the fixture server has no events
// service or keeper-of-facts, so the text says the hints and ledger are
// unavailable and still prints the trace's parts.
func TestPrintScorecardWithoutPeers(t *testing.T) {
	sc, err := sessionScorecard(context.Background(), traceClient(t), servertest.OldSession)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	printScorecard(&out, sc)
	for _, want := range []string{
		"session sess-old, 1 turn: http",
		"sources: transcripts ok, events unavailable (",
		"kof skipped",
		"cost    $0.026  main $0.014, agents $0.011",
		"FLAGS: none",
		"search  no csl searches",
		"hints   unavailable: the events service did not answer",
		"agents  1 subagent; ledger unavailable",
		"TOOL  CALLS  BY AGENTS  FAILED  TIME",
		"Edit  1      0          0       1m00s",
		"Read  1      1          0       30.0s",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestPrintScorecardParts(t *testing.T) {
	rate := 0.5
	sc := scorecard.Scorecard{
		SessionID: "s1",
		Search:    scorecard.SearchStats{Calls: 4, Zero: 2, ZeroResultHint: 1, Truncated: 1},
		Reads: scorecard.ReadStats{
			Calls: 3, Unranged: 2, Large: 1, LargeLines: 300, Lines: 900,
			Largest: &scorecard.ReadCall{Path: "/w/big.go", Lines: 800, AgentID: "agent-1"},
		},
		Hints: &scorecard.HintStats{
			Shown: 3,
			ByHint: []scorecard.HintCount{
				{Hint: "prefer-csl", Count: 2},
				{Hint: "kof-consult", Count: 1},
			},
			Kof: scorecard.KofStats{
				Assertions:  []scorecard.ShownAssertion{{ID: "id1"}, {ID: "id2"}},
				PinnedPaths: 2, UsedPaths: 1, HitRate: &rate, UnknownPins: 1,
			},
		},
		Agents: scorecard.AgentStats{Ledger: &scorecard.Ledger{PreCompact: 2}},
		Flags:  []string{"2 csl searches with zero results"},
	}
	var out bytes.Buffer
	printScorecard(&out, sc)
	for _, want := range []string{
		"FLAGS\n  2 csl searches with zero results\n",
		"4 csl searches: 2 zero (1 with a zero_result_hint), 1 cut at the limit",
		"3 reads: 2 of a whole file, 1 large (whole file, 300+ lines), 900 lines in all",
		"largest 800 lines, whole file, by agent-1: /w/big.go",
		"3 shown: prefer-csl 2, kof-consult 1; kof: 2 assertions shown, pinned to 2 files, " +
			"1 read or edited later (50%), 1 with unknown pins",
		"0 subagents; ledger: 0 started, 0 stopped, 0 permission denied, 2 compactions",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}
