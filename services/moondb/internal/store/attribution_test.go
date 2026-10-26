package store

import (
	"cmp"
	"maps"
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
// the repository most of its edits went to, else to orchestration. An
// agent without totals adds nothing, and an edit outside any repository
// counts nowhere.
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
			name:  "main with edits goes to their majority",
			turns: []Turn{mainTurn(1)},
			edits: []Edit{
				editIn("1", "", repoA, 1),
				editIn("2", "", repoB, 2),
				editIn("3", "", repoB, 3),
			},
			want: map[string]int64{repoB: 1},
		},
		{
			name:  "main without edits goes to orchestration",
			turns: []Turn{mainTurn(1)},
			// A subagent's edit is not the main conversation's.
			agents: []Agent{agentWith("x", "", 10)},
			edits:  []Edit{editIn("1", "x", repoA, 1)},
			want:   map[string]int64{CostOrchestration: 1, repoA: 10},
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

// TestCostRepoMatchesSummarize: the per-agent answers a listing gives an
// agent row name the entry Summarize put the agent's usage in.
func TestCostRepoMatchesSummarize(t *testing.T) {
	agent := agentWith("x", repoB, 10)
	edits := []Edit{editIn("1", "x", repoA, 1), editIn("2", "", repoB, 2)}
	if got := AgentCostRepo(agent, edits[:1]); got != repoA {
		t.Errorf("AgentCostRepo = %q, want %q", got, repoA)
	}
	if got := AgentCostRepo(agentWith("y", "", 1), nil); got != CostNoRepo {
		t.Errorf("AgentCostRepo without edits or cwd = %q, want %q", got, CostNoRepo)
	}
	if got := MainCostRepo(edits[1:]); got != repoB {
		t.Errorf("MainCostRepo = %q, want %q", got, repoB)
	}
	if got := MainCostRepo(nil); got != CostOrchestration {
		t.Errorf("MainCostRepo without edits = %q, want %q", got, CostOrchestration)
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
