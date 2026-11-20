package store

import (
	"cmp"
	"maps"
	"reflect"
	"slices"
	"testing"
)

const (
	repoA = "example.com/o/a"
	repoB = "example.com/o/b"
)

// inputUsage is usage of n input tokens on one model, so a test can tell
// the parts apart by their token counts.
func inputUsage(n int64) []ModelUsage {
	return []ModelUsage{{Model: "m", Tokens: Tokens{Input: n}}}
}

// mainTurn is a closed turn whose totals hold n input tokens.
func mainTurn(n int64) Turn {
	t := closedTurn("t1", 1, 0, 10)
	t.Totals = &TurnTotals{Usage: inputUsage(n)}
	return t
}

// agentWith is subagent id with cwd repository cwdRepo and totals of n
// input tokens; n 0 leaves it without totals.
func agentWith(id, cwdRepo string, n int64) Agent {
	a := Agent{ID: id, SessionID: "s", Type: "general-purpose", Location: Location{Repo: cwdRepo}}
	if n > 0 {
		a.Totals = &TurnTotals{Usage: inputUsage(n)}
	}
	return a
}

// editIn is an edit by agent ("" for main) in repo at minute m.
func editIn(id, agent, repo string, m int) Edit {
	return Edit{
		ID:        id,
		SessionID: "s",
		AgentID:   agent,
		At:        minutes(m),
		Location:  Location{Repo: repo},
	}
}

// inTurn is e with turn as its TurnID.
func inTurn(e Edit, turn string) Edit {
	e.TurnID = turn
	return e
}

// tokenTurn is a closed turn from start to start+5 minutes whose totals
// hold tokens on one model.
func tokenTurn(id string, seq, start int, tokens Tokens) Turn {
	t := closedTurn(id, seq, start, start+5)
	t.Totals = &TurnTotals{Usage: []ModelUsage{{Model: "m", Tokens: tokens}}}
	return t
}

// tokensByRepo maps each attributed entry to its tokens.
func tokensByRepo(parts []RepoUsage) map[string]Tokens {
	out := map[string]Tokens{}
	for _, p := range parts {
		for _, u := range p.Usage {
			out[p.Repo] = out[p.Repo].Add(u.Tokens)
		}
	}
	return out
}

func editSeq(edits []Edit) func(func(*Edit) bool) {
	return func(yield func(*Edit) bool) {
		for i := range edits {
			if !yield(&edits[i]) {
				return
			}
		}
	}
}

// inputByRepo maps each attributed entry to its input tokens.
func inputByRepo(parts []RepoUsage) map[string]int64 {
	out := map[string]int64{}
	for _, p := range parts {
		for _, u := range p.Usage {
			out[p.Repo] += u.Tokens.Input
		}
	}
	return out
}

// TestAttribution: a subagent's cost goes to the repository most of its
// edits went to, a tie to the one it edited first, without edits to its
// cwd's repository, else to no_repo. The main conversation's cost goes to
// the repositories of a turn's own edits, else of its subagents' edits,
// else to orchestration (TestMainSplit covers the split). An agent without
// totals adds nothing, and an edit outside any repository counts nowhere.
func TestAttribution(t *testing.T) {
	tests := []struct {
		name   string
		turns  []Turn
		agents []Agent
		edits  []Edit
		want   map[string]int64
	}{
		{
			name:   "subagent majority wins",
			agents: []Agent{agentWith("x", repoB, 10)},
			edits: []Edit{
				editIn("1", "x", repoB, 1), editIn("2", "x", repoA, 2), editIn("3", "x", repoA, 3),
			},
			want: map[string]int64{repoA: 10},
		},
		{
			name:   "tie goes to the repository edited first",
			agents: []Agent{agentWith("x", "", 10)},
			edits: []Edit{
				// Two each: repoB was edited first, though repoA sorts first.
				editIn("1", "x", repoA, 5), editIn("2", "x", repoB, 1),
				editIn("3", "x", repoB, 6), editIn("4", "x", repoA, 7),
			},
			want: map[string]int64{repoB: 10},
		},
		{
			name:   "subagent without edits goes to its cwd repository",
			agents: []Agent{agentWith("x", repoB, 10)},
			want:   map[string]int64{repoB: 10},
		},
		{
			name:   "subagent with neither goes to no_repo",
			agents: []Agent{agentWith("x", "", 10)},
			want:   map[string]int64{CostNoRepo: 10},
		},
		{
			name:   "edits outside any repository count nowhere",
			agents: []Agent{agentWith("x", repoB, 10)},
			edits:  []Edit{editIn("1", "x", "", 1), editIn("2", "x", "", 2)},
			want:   map[string]int64{repoB: 10},
		},
		{
			name:  "main with edits splits by them",
			turns: []Turn{mainTurn(7)},
			edits: []Edit{
				editIn("1", "", repoA, 1),
				editIn("2", "", repoB, 2),
				editIn("3", "", repoB, 3),
			},
			// 7 by 1:2 is 2.33 and 4.67; the leftover token goes to repoB's
			// larger remainder.
			want: map[string]int64{repoA: 2, repoB: 5},
		},
		{
			name:   "main without edits of its own takes its subagents'",
			turns:  []Turn{mainTurn(1)},
			agents: []Agent{agentWith("x", "", 10)},
			edits:  []Edit{editIn("1", "x", repoA, 1)},
			want:   map[string]int64{repoA: 11},
		},
		{
			name:   "main without any edit goes to orchestration",
			turns:  []Turn{mainTurn(1)},
			agents: []Agent{agentWith("x", repoB, 10)},
			want:   map[string]int64{CostOrchestration: 1, repoB: 10},
		},
		{
			name:  "agent without totals adds nothing",
			turns: []Turn{mainTurn(1)},
			agents: []Agent{
				agentWith("priced", repoA, 10), agentWith("unpriced", repoB, 0),
			},
			edits: []Edit{editIn("1", "unpriced", repoB, 1), editIn("2", "", repoA, 2)},
			want:  map[string]int64{repoA: 11},
		},
		{
			name: "main without totals adds nothing",
			turns: []Turn{
				openAt("t1", 1, 0),
			},
			edits: []Edit{editIn("1", "", repoA, 1)},
			want:  map[string]int64{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sum := Summarize(Session{ID: "s", StartedAt: minutes(0)}, tc.turns, tc.agents,
				editSeq(tc.edits))
			if got := inputByRepo(sum.RepoUsage); !maps.Equal(got, tc.want) {
				t.Errorf("attributed input = %v, want %v", got, tc.want)
			}
			if !slices.IsSortedFunc(sum.RepoUsage, func(a, b RepoUsage) int {
				return cmp.Compare(a.Repo, b.Repo)
			}) {
				t.Errorf("entries %v are not sorted by repo", sum.RepoUsage)
			}
		})
	}
}

// TestMainSplit: each turn's main usage goes to the repositories of the
// main conversation's own edits in it, by count; else of its subagents'
// edits in it; else to the repositories the turn before it went to; and
// before the first such turn to orchestration. Every token count splits
// whole, the leftover to the largest remainder, a tie to the repository
// edited first. An edit counts under the turn its TurnID names, else
// under the turn running when it was made. MainRepoUsage gives the parts
// Summarize gives.
func TestMainSplit(t *testing.T) {
	in := func(n int64) Tokens { return Tokens{Input: n} }
	tests := []struct {
		name  string
		turns []Turn
		edits []Edit
		want  map[string]Tokens
	}{
		{
			name:  "each turn's edits take that turn's usage",
			turns: []Turn{tokenTurn("t1", 1, 0, in(3)), tokenTurn("t2", 2, 20, in(5))},
			edits: []Edit{
				inTurn(editIn("1", "", repoA, 1), "t1"), inTurn(editIn("2", "", repoB, 21), "t2"),
			},
			want: map[string]Tokens{repoA: in(3), repoB: in(5)},
		},
		{
			name: "a turn with edits in two repositories splits every count by them",
			turns: []Turn{tokenTurn("t1", 1, 0, Tokens{
				Input: 7, CacheRead: 3, Output: 5, Thinking: 1,
			})},
			edits: []Edit{
				inTurn(editIn("1", "", repoA, 1), "t1"),
				inTurn(editIn("2", "", repoB, 2), "t1"),
				inTurn(editIn("3", "", repoA, 3), "t1"),
			},
			// By 2:1, input 7 is 4.67 and 2.33, output 5 is 3.33 and 1.67 and
			// thinking 1 is 0.67 and 0.33: each leftover token goes to the
			// larger remainder.
			want: map[string]Tokens{
				repoA: {Input: 5, CacheRead: 2, Output: 3, Thinking: 1},
				repoB: {Input: 2, CacheRead: 1, Output: 2},
			},
		},
		{
			name:  "a tie in remainders goes to the repository edited first",
			turns: []Turn{tokenTurn("t1", 1, 0, in(5))},
			edits: []Edit{
				inTurn(editIn("1", "", repoB, 1), "t1"), inTurn(editIn("2", "", repoA, 2), "t1"),
			},
			want: map[string]Tokens{repoA: in(2), repoB: in(3)},
		},
		{
			name:  "a part without tokens adds no entry",
			turns: []Turn{tokenTurn("t1", 1, 0, in(1))},
			edits: []Edit{
				inTurn(editIn("1", "", repoA, 1), "t1"),
				inTurn(editIn("2", "", repoA, 2), "t1"),
				inTurn(editIn("3", "", repoB, 3), "t1"),
			},
			want: map[string]Tokens{repoA: in(1)},
		},
		{
			name: "a turn without edits takes the weights of the turn before it",
			turns: []Turn{
				tokenTurn("t1", 1, 0, in(1)), tokenTurn("t2", 2, 20, in(2)),
				tokenTurn("t3", 3, 40, in(4)), tokenTurn("t4", 4, 60, in(8)),
			},
			edits: []Edit{
				inTurn(editIn("1", "", repoA, 1), "t1"), inTurn(editIn("2", "", repoB, 41), "t3"),
			},
			want: map[string]Tokens{repoA: in(3), repoB: in(12)},
		},
		{
			name: "turns before the first edit go to orchestration",
			turns: []Turn{
				tokenTurn("t1", 1, 0, in(1)), tokenTurn("t2", 2, 20, in(2)),
				tokenTurn("t3", 3, 40, in(4)),
			},
			// An edit outside any repository gives t1 no weights.
			edits: []Edit{
				inTurn(editIn("1", "", "", 1), "t1"), inTurn(editIn("2", "", repoA, 21), "t2"),
			},
			want: map[string]Tokens{CostOrchestration: in(1), repoA: in(6)},
		},
		{
			name:  "a turn with only subagent edits splits by them",
			turns: []Turn{tokenTurn("t1", 1, 0, in(3))},
			edits: []Edit{
				inTurn(editIn("1", "x", repoA, 1), "t1"),
				inTurn(editIn("2", "y", repoB, 2), "t1"),
				inTurn(editIn("3", "x", repoA, 3), "t1"),
			},
			want: map[string]Tokens{repoA: in(2), repoB: in(1)},
		},
		{
			name:  "the main conversation's own edits outweigh its subagents'",
			turns: []Turn{tokenTurn("t1", 1, 0, in(3))},
			edits: []Edit{
				inTurn(editIn("1", "x", repoA, 1), "t1"),
				inTurn(editIn("2", "x", repoA, 2), "t1"),
				inTurn(editIn("3", "", repoB, 3), "t1"),
			},
			want: map[string]Tokens{repoB: in(3)},
		},
		{
			name:  "an edit counts under the turn its TurnID names, else by its time",
			turns: []Turn{tokenTurn("t1", 1, 0, in(3)), tokenTurn("t2", 2, 20, in(5))},
			edits: []Edit{
				// Made while t2 ran, but it names t1.
				inTurn(editIn("1", "", repoA, 25), "t1"),
				// Names no turn of the session: t2 was running.
				inTurn(editIn("2", "", repoB, 22), "gone"),
			},
			want: map[string]Tokens{repoA: in(3), repoB: in(5)},
		},
		{
			name:  "an edit before every turn counts under none",
			turns: []Turn{tokenTurn("t1", 1, 10, in(3))},
			edits: []Edit{editIn("1", "", repoA, 5)},
			want:  map[string]Tokens{CostOrchestration: in(3)},
		},
		{
			name: "a turn without totals still passes its weights on",
			turns: []Turn{
				{ID: "t1", Seq: 1, StartedAt: minutes(0)}, tokenTurn("t2", 2, 20, in(5)),
			},
			edits: []Edit{inTurn(editIn("1", "", repoA, 1), "t1")},
			want:  map[string]Tokens{repoA: in(5)},
		},
		{
			name: "turns go by start, then Seq, not by their order in the list",
			turns: []Turn{
				tokenTurn("t3", 3, 20, in(4)), tokenTurn("t2", 2, 0, in(2)),
				tokenTurn("t1", 1, 0, in(1)),
			},
			edits: []Edit{inTurn(editIn("1", "", repoA, 1), "t1")},
			want:  map[string]Tokens{repoA: in(7)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sum := Summarize(Session{ID: "s", StartedAt: minutes(0)}, tc.turns, nil,
				editSeq(tc.edits))
			if got := tokensByRepo(sum.RepoUsage); !maps.Equal(got, tc.want) {
				t.Errorf("attributed = %v, want %v", got, tc.want)
			}
			var whole, parts Tokens
			for _, u := range sum.MainUsage {
				whole = whole.Add(u.Tokens)
			}
			for _, p := range tokensByRepo(sum.RepoUsage) {
				parts = parts.Add(p)
			}
			if parts != whole {
				t.Errorf("parts add up to %+v, the turns to %+v", parts, whole)
			}
			if got := MainRepoUsage(tc.turns, tc.edits); !reflect.DeepEqual(got, sum.RepoUsage) {
				t.Errorf("MainRepoUsage = %v, Summarize gives %v", got, sum.RepoUsage)
			}
		})
	}
}

// TestRepoUsageMatchesSummarize: the answers a listing gives the main row
// (MainRepoUsage) and each agent row (AgentCostRepo) add up to the parts
// Summarize gives the session.
func TestRepoUsageMatchesSummarize(t *testing.T) {
	x, y := agentWith("x", repoB, 10), agentWith("y", "", 1)
	turns := []Turn{
		tokenTurn("t1", 1, 0, Tokens{Input: 100}), tokenTurn("t2", 2, 20, Tokens{Input: 51}),
	}
	tests := []struct {
		name  string
		edits []Edit
	}{
		{
			name: "edits",
			edits: []Edit{
				inTurn(editIn("1", "x", repoA, 1), "t1"), inTurn(editIn("2", "", repoB, 2), "t1"),
				inTurn(editIn("3", "x", repoA, 21), "t2"), inTurn(editIn("4", "", "", 22), "t2"),
			},
		},
		{name: "no edits"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			byAgent := map[string][]Edit{}
			for _, e := range tc.edits {
				byAgent[e.AgentID] = append(byAgent[e.AgentID], e)
			}
			rows := MergeRepoUsage(MainRepoUsage(turns, tc.edits), []RepoUsage{
				{Repo: AgentCostRepo(x, byAgent["x"]), Usage: x.Totals.Usage},
				{Repo: AgentCostRepo(y, byAgent["y"]), Usage: y.Totals.Usage},
			})
			sum := Summarize(Session{ID: "s", StartedAt: minutes(0)}, turns, []Agent{x, y},
				editSeq(tc.edits))
			if !reflect.DeepEqual(rows, sum.RepoUsage) {
				t.Errorf("the rows' answers give %v, Summarize %v", rows, sum.RepoUsage)
			}
		})
	}
}

// TestMergeRepoUsage: a window over two sessions that share a repository
// adds their parts there and keeps the others apart.
func TestMergeRepoUsage(t *testing.T) {
	one := []RepoUsage{
		{Repo: repoA, Usage: inputUsage(1)},
		{Repo: CostOrchestration, Usage: inputUsage(2)},
	}
	two := []RepoUsage{
		{Repo: repoA, Usage: inputUsage(10)},
		{Repo: CostNoRepo, Usage: inputUsage(20)},
	}
	got := MergeRepoUsage(one, two)
	want := map[string]int64{repoA: 11, CostOrchestration: 2, CostNoRepo: 20}
	if !maps.Equal(inputByRepo(got), want) || len(got) != 3 {
		t.Errorf("merged = %v, want %v", inputByRepo(got), want)
	}
	if len(got[0].Usage) != 1 {
		t.Errorf("merged %s usage = %v, want one entry per rate", got[0].Repo, got[0].Usage)
	}
}
