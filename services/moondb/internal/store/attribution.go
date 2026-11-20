package store

import (
	"cmp"
	"slices"
	"sort"
	"time"
)

// Labels that take attributed cost no repository takes. A repository
// identity always holds host/owner/repo, so neither label can name one.
const (
	// CostOrchestration takes the main conversation's usage of the turns
	// before the first turn with an edit in a repository, its own or a
	// subagent's (mainSplit).
	CostOrchestration = "orchestration"
	// CostNoRepo takes a subagent's cost when it edited no file in a
	// repository and its cwd sits in none.
	CostNoRepo = "no_repo"
)

// RepoUsage is the usage attributed to one repository, or to
// CostOrchestration or CostNoRepo: the attributed cost before pricing.
type RepoUsage struct {
	Repo  string
	Usage []ModelUsage
}

// repoTally counts one agent's edits per repository, with the time of the
// first edit in each. An edit outside any repository counts nowhere.
type repoTally map[string]repoEdits

type repoEdits struct {
	count int
	first time.Time
}

func (t repoTally) add(e *Edit) {
	if e.Repo == "" {
		return
	}
	r, ok := t[e.Repo]
	if !ok || e.At.Before(r.first) {
		r.first = e.At
	}
	r.count++
	t[e.Repo] = r
}

// share is one repository's weight in a split: its edit count, with the
// time of its first edit for the order.
type share struct {
	repo string
	repoEdits
}

// shares lists t's repositories with more edits first, a tie to the one
// edited first, then to the lower name.
func (t repoTally) shares() []share {
	out := make([]share, 0, len(t))
	for repo, r := range t {
		out = append(out, share{repo: repo, repoEdits: r})
	}
	slices.SortFunc(out, func(a, b share) int {
		return cmp.Or(
			cmp.Compare(b.count, a.count),
			a.first.Compare(b.first),
			cmp.Compare(a.repo, b.repo),
		)
	})
	return out
}

// majority is the repository most of the edits went to. A tie goes to the
// repository edited first, then to the lower name. It is "" without an
// edit in a repository.
func (t repoTally) majority() string {
	if s := t.shares(); len(s) > 0 {
		return s[0].repo
	}
	return ""
}

// agentRepo is where subagent a's cost goes: the repository of most of
// its edits, else its cwd's repository, else CostNoRepo.
func (t repoTally) agentRepo(a Agent) string {
	return cmp.Or(t.majority(), a.Location.Repo, CostNoRepo)
}

// agentTallies holds each agent's repoTally by agent id, "" for the main
// conversation.
type agentTallies map[string]repoTally

func (t agentTallies) add(e *Edit) {
	if t[e.AgentID] == nil {
		t[e.AgentID] = repoTally{}
	}
	t[e.AgentID].add(e)
}

func tallyOf(edits []Edit) repoTally {
	t := repoTally{}
	for i := range edits {
		t.add(&edits[i])
	}
	return t
}

// AgentCostRepo is where the attribution rule sends subagent a's cost,
// given the edits it made: the repository most of them went to, else the
// repository of its cwd, else CostNoRepo.
func AgentCostRepo(a Agent, edits []Edit) string { return tallyOf(edits).agentRepo(a) }

// MainRepoUsage is the main conversation's usage split turn by turn by the
// attribution rule (mainSplit), given the session's turns and all of its
// edits, the subagents' among them: the main conversation's parts of
// Summarize's RepoUsage, one entry per repository or CostOrchestration,
// sorted by Repo.
func MainRepoUsage(turns []Turn, edits []Edit) []RepoUsage {
	split := newMainSplit(turns)
	for i := range edits {
		split.add(&edits[i])
	}
	return mergeTurnParts(split.parts())
}

// mainSplit splits the main conversation's usage turn by turn, the turns
// taken by start, then Seq. A turn's weights are the edit counts per
// repository of the main conversation's own edits in it; without one, of
// its subagents' edits in it; without one, the weights of the turn before
// it, so they carry forward. Before the first turn with weights,
// CostOrchestration takes it all. An edit outside any repository gives no
// weight.
type mainSplit struct {
	turns []Turn
	// index maps a turn id to its position in turns.
	index map[string]int
	// edits holds each turn's edit counts, by its position in turns.
	edits []turnEdits
}

// turnEdits counts one turn's edits: the main conversation's own and its
// subagents'.
type turnEdits struct {
	main, agents repoTally
}

func newMainSplit(turns []Turn) *mainSplit {
	sorted := slices.Clone(turns)
	slices.SortStableFunc(sorted, func(a, b Turn) int {
		return cmp.Or(a.StartedAt.Compare(b.StartedAt), cmp.Compare(a.Seq, b.Seq))
	})
	m := &mainSplit{
		turns: sorted,
		index: make(map[string]int, len(sorted)),
		edits: make([]turnEdits, len(sorted)),
	}
	for i, t := range sorted {
		m.index[t.ID] = i
		m.edits[i] = turnEdits{main: repoTally{}, agents: repoTally{}}
	}
	return m
}

// add counts e under the turn its TurnID names. An edit whose TurnID names
// no turn counts under the latest turn that started at or before it, and
// one made before every turn counts under none.
func (m *mainSplit) add(e *Edit) {
	i, ok := m.index[e.TurnID]
	if !ok {
		i = m.startedBy(e.At)
	}
	if i < 0 {
		return
	}
	if e.AgentID == "" {
		m.edits[i].main.add(e)
		return
	}
	m.edits[i].agents.add(e)
}

// startedBy is the position of the latest turn that started at or before
// at, -1 when none did.
func (m *mainSplit) startedBy(at time.Time) int {
	after := func(i int) bool { return m.turns[i].StartedAt.After(at) }
	return sort.Search(len(m.turns), after) - 1
}

// weights are the shares a turn's own edits give it: the main
// conversation's, else its subagents'; empty without either.
func (e turnEdits) weights() []share {
	if len(e.main) > 0 {
		return e.main.shares()
	}
	return e.agents.shares()
}

// turnPart is one turn's main usage split by repository, with the turn's
// start, which names the day the part goes to.
type turnPart struct {
	start time.Time
	usage []RepoUsage
}

// parts splits each turn's usage by its weights, in turn order. A turn
// without totals has no part.
func (m *mainSplit) parts() []turnPart {
	out := make([]turnPart, 0, len(m.turns))
	weights := []share{{repo: CostOrchestration, repoEdits: repoEdits{count: 1}}}
	for i, t := range m.turns {
		if w := m.edits[i].weights(); len(w) > 0 {
			weights = w
		}
		if t.Totals != nil {
			usage := splitUsage(t.Totals.Usage, weights)
			out = append(out, turnPart{start: t.StartedAt, usage: usage})
		}
	}
	return out
}

// mergeTurnParts sums the turns' parts into one entry per repository or
// label, sorted by Repo.
func mergeTurnParts(turns []turnPart) []RepoUsage {
	lists := make([][]RepoUsage, len(turns))
	for i, t := range turns {
		lists[i] = t.usage
	}
	return MergeRepoUsage(lists...)
}

// splitUsage shares every token count of every usage entry across
// weights (split). A part without tokens adds no entry. The parts come
// sorted by Repo.
func splitUsage(usage []ModelUsage, weights []share) []RepoUsage {
	parts := map[string][][]ModelUsage{}
	for _, u := range usage {
		for i, tokens := range splitTokens(u.Tokens, weights) {
			if tokens == (Tokens{}) {
				continue
			}
			part := u
			part.Tokens = tokens
			parts[weights[i].repo] = append(parts[weights[i].repo], []ModelUsage{part})
		}
	}
	return mergeParts(parts)
}

// splitTokens shares each of t's counts across weights (split), so the
// parts' counts add up to t's.
func splitTokens(t Tokens, weights []share) []Tokens {
	out := make([]Tokens, len(weights))
	for field, n := range tokenCounts(&t) {
		for i, part := range split(*n, weights) {
			*tokenCounts(&out[i])[field] = part
		}
	}
	return out
}

// tokenCounts points at each of t's six counts.
func tokenCounts(t *Tokens) [6]*int64 {
	return [6]*int64{
		&t.Input, &t.CacheWrite5m, &t.CacheWrite1h, &t.CacheRead, &t.Output, &t.Thinking,
	}
}

// split shares n across weights, which hold at least one share, by the
// largest-remainder method: each share takes the whole part of its quota,
// and what is left goes one each to the largest remainders, a tie to the
// share listed first. The parts add up to n. A negative n, which no trace
// gives, goes whole to the first share.
func split(n int64, weights []share) []int64 {
	parts := make([]int64, len(weights))
	if n < 0 {
		parts[0] = n
		return parts
	}
	var total int64
	for _, w := range weights {
		total += int64(w.count)
	}
	rems := make([]int64, len(weights))
	left := n
	for i, w := range weights {
		// n*c/total as q*c + r*c/total, so n*c cannot overflow.
		q, r, c := n/total, n%total, int64(w.count)
		parts[i], rems[i] = q*c+r*c/total, r*c%total
		left -= parts[i]
	}
	for _, i := range byRemainder(rems)[:left] {
		parts[i]++
	}
	return parts
}

// byRemainder lists the positions of rems, the largest remainder first, a
// tie in position order.
func byRemainder(rems []int64) []int {
	order := make([]int, len(rems))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(rems[b], rems[a]) })
	return order
}

// agentParts splits the agents' usage by the subagent rule (agentRepo),
// over the edits tallies counts. An agent without tokens in its totals
// adds nothing. The parts come sorted by Repo.
func agentParts(agents []Agent, tallies agentTallies) []RepoUsage {
	parts := map[string][][]ModelUsage{}
	for _, a := range agents {
		if a.Totals == nil || !hasTokens(a.Totals.Usage) {
			continue
		}
		repo := tallies[a.ID].agentRepo(a)
		parts[repo] = append(parts[repo], a.Totals.Usage)
	}
	return mergeParts(parts)
}

// MergeRepoUsage sums attributed usage lists into one with an entry per
// repository or label, sorted by Repo, so a reader can price a window of
// sessions once per entry.
func MergeRepoUsage(lists ...[]RepoUsage) []RepoUsage {
	parts := map[string][][]ModelUsage{}
	for _, list := range lists {
		for _, ru := range list {
			parts[ru.Repo] = append(parts[ru.Repo], ru.Usage)
		}
	}
	return mergeParts(parts)
}

// mergeParts merges each repository's usage lists into one RepoUsage,
// sorted by Repo.
func mergeParts(parts map[string][][]ModelUsage) []RepoUsage {
	out := make([]RepoUsage, 0, len(parts))
	for repo, lists := range parts {
		out = append(out, RepoUsage{Repo: repo, Usage: MergeUsage(lists...)})
	}
	slices.SortFunc(out, func(a, b RepoUsage) int { return cmp.Compare(a.Repo, b.Repo) })
	return out
}

func hasTokens(usage []ModelUsage) bool {
	return slices.ContainsFunc(usage, func(u ModelUsage) bool { return u.Tokens.Total() > 0 })
}
