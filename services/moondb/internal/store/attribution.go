package store

import (
	"cmp"
	"slices"
	"time"
)

// Labels that take attributed cost no repository takes. A repository
// identity always holds host/owner/repo, so neither label can name one.
const (
	// CostOrchestration takes the main conversation's cost when it edited
	// no file in a repository.
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

// majority is the repository most of the edits went to. A tie goes to the
// repository edited first, then to the lower name. It is "" without an
// edit in a repository.
func (t repoTally) majority() string {
	best := ""
	for repo, r := range t {
		if best == "" {
			best = repo
			continue
		}
		b := t[best]
		if c := cmp.Or(
			cmp.Compare(b.count, r.count),
			r.first.Compare(b.first),
			cmp.Compare(repo, best),
		); c < 0 {
			best = repo
		}
	}
	return best
}

// mainRepo is where the main conversation's cost goes: the repository of
// most of its edits, else CostOrchestration.
func (t repoTally) mainRepo() string { return cmp.Or(t.majority(), CostOrchestration) }

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

// MainCostRepo is where the attribution rule sends the main conversation's
// cost, given the edits it made itself: the repository most of them went
// to, else CostOrchestration.
func MainCostRepo(edits []Edit) string { return tallyOf(edits).mainRepo() }

// AgentCostRepo is where the attribution rule sends subagent a's cost,
// given the edits it made: the repository most of them went to, else the
// repository of its cwd, else CostNoRepo.
func AgentCostRepo(a Agent, edits []Edit) string { return tallyOf(edits).agentRepo(a) }

// attribute splits a session's usage by where the attribution rule sends
// each part: main, the main conversation's usage, and each agent's totals,
// by the edits tallies counts. A part without tokens adds nothing, so an
// agent without totals has no entry. The entries come sorted by Repo.
func attribute(main []ModelUsage, agents []Agent, tallies agentTallies) []RepoUsage {
	parts := map[string][][]ModelUsage{}
	if hasTokens(main) {
		repo := tallies[""].mainRepo()
		parts[repo] = append(parts[repo], main)
	}
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
