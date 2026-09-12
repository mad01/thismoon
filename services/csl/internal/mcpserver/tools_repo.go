package mcpserver

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// repoLookupInput is the typed input for the csl_repo_lookup tool.
type repoLookupInput struct {
	Name string `json:"name" jsonschema:"case-insensitive regex or substring matched against the repo name (e.g. 'myrepo', 'mad01/.*')"`
	formatParam
}

// repoMatch is one entry in the csl_repo_lookup result.
type repoMatch struct {
	Name   string `json:"name"             jsonschema:"org/repo name extracted from the git remote URL"`
	Path   string `json:"path"             jsonschema:"absolute filesystem path to the repo root"`
	Remote string `json:"remote,omitempty" jsonschema:"full origin remote URL"`
	Host   string `json:"host,omitempty"   jsonschema:"git host extracted from remote URL (e.g. github.com, git.example.com)"`
}

// droppedMatch is a repo the walk found and a config filter removed. It turns
// an empty matches array from "no such checkout" into "csl saw it and
// index.hosts or the exclude list took it out", which is the answer an agent
// would otherwise get wrong.
type droppedMatch struct {
	Name   string `json:"name"             jsonschema:"org/repo name extracted from the git remote URL"`
	Path   string `json:"path"             jsonschema:"absolute filesystem path to the repo root"`
	Remote string `json:"remote,omitempty" jsonschema:"full origin remote URL"`
	Host   string `json:"host,omitempty"   jsonschema:"git host extracted from remote URL, empty when the remote has none"`
	Reason string `json:"reason"           jsonschema:"the setting that removed the repo from the index (index.hosts or hooks.post_merge.exclude)"`
}

// repoLookupOutput is the structured output of the csl_repo_lookup tool.
type repoLookupOutput struct {
	Matches []repoMatch    `json:"matches"           jsonschema:"matching repos; empty when csl found no local checkout, or when a config filter dropped it (see dropped)"`
	Dropped []droppedMatch `json:"dropped,omitempty" jsonschema:"matching repos the walk found but index.hosts or hooks.post_merge.exclude removed; present only when matches is empty"`
}

func registerRepoTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_repo_lookup",
		Description: "Resolve a git repo name to its local checkout path. " +
			"Use when the user mentions a repo by name and you need its absolute path before cd-ing, reading, or grepping inside it. " +
			"Matching is case-insensitive regex / substring against the org/repo name. " +
			"Returns an empty matches array if the repo isn't checked out locally. When a dropped array comes back beside it, csl found the checkout but a config filter (index.hosts or hooks.post_merge.exclude) removed it: report the reason rather than calling the repo missing. " +
			"Empty matches with no dropped means the repo isn't present under csl's dirs; tell the user so and don't guess a path under ~/code/src/... or elsewhere.",
	}, withFormat(handleRepoLookup, nil))

	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_repo_info",
		Description: "Check repo health before starting work. " +
			"Returns git state (branch, dirty files, modified/untracked counts), index staleness, and an action field " +
			"indicating what to do: \"ready\" (good to go), \"commit_or_stash\" (dirty working tree), " +
			"\"pull_recommended\" (index stale >30min, likely behind remote), \"needs_reindex\" (local changes not in search index). " +
			"Use this BEFORE creating branches or making changes to a repo.",
	}, withFormat(handleRepoInfo, nil))

	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_repo_health",
		Description: "Fleet-wide repo health report: which local checkouts hold uncommitted or unpushed work. " +
			"Walks every repo and returns branch, dirty counts, and commits ahead/behind the last-fetched upstream (no network fetch). " +
			"action is one of \"commit_or_stash\" (dirty tree), \"diverged\" (ahead and behind), \"push_recommended\" (unpushed commits), " +
			"\"pull_recommended\" (behind upstream), \"no_upstream\" (branch without upstream), \"detached_head\", \"error\", \"ready\". " +
			"Default returns only repos needing attention; set all=true for the full list. " +
			"Use before a machine switch or as a hygiene sweep. Unlike csl_repo_info this runs git across the whole fleet, so it takes a few seconds; " +
			"for one repo's health including index staleness, use csl_repo_info.",
	}, withFormat(handleRepoHealth, nil))

	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_repo_pull",
		Description: "Git pull a workspace repo with safety checks. " +
			"Warns if there are uncommitted changes or detached HEAD. Uses --ff-only (no merge commits). " +
			"Set force=true to pull even with uncommitted changes. " +
			"Use before creating branches on repos that may be out of date.",
	}, withFormat(handleRepoPull, nil))

	mcp.AddTool(s, &mcp.Tool{
		Name: "csl_repo_reindex",
		Description: "Reindex a specific repo in the local zoekt search index. " +
			"Use after making significant changes to ensure csl_search results are current, " +
			"or when csl_repo_info reports needs_reindex. " +
			"The call blocks until indexing finishes and returns the measured duration: typically sub-second for small repos, a few seconds for large ones.",
	}, withFormat(handleRepoReindex, nil))
}

func handleRepoLookup(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in repoLookupInput,
) (*mcp.CallToolResult, repoLookupOutput, error) {
	re, err := finder.CompileMatcher(in.Name)
	if err != nil {
		return nil, repoLookupOutput{}, err
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, repoLookupOutput{}, fmt.Errorf("load csl config: %w", err)
	}

	repos, dropped, err := cfg.DiscoverReposReport()
	if err != nil {
		return nil, repoLookupOutput{}, fmt.Errorf("walk repos: %w", err)
	}

	matches := make([]repoMatch, 0, 4)
	for _, r := range repos {
		if re.MatchString(r.Name) {
			matches = append(
				matches,
				repoMatch{Name: r.Name, Path: r.Path, Remote: r.Remote, Host: r.Host},
			)
		}
	}
	out := repoLookupOutput{Matches: matches}
	if len(matches) > 0 {
		return nil, out, nil
	}
	// Nothing indexed matched. Before the caller concludes the repo is not
	// checked out, say whether a filter is what hid it.
	for _, d := range dropped {
		if re.MatchString(d.Repo.Name) {
			out.Dropped = append(out.Dropped, droppedMatch{
				Name:   d.Repo.Name,
				Path:   d.Repo.Path,
				Remote: d.Repo.Remote,
				Host:   d.Repo.Host,
				Reason: d.Reason(),
			})
		}
	}
	return nil, out, nil
}

// --- csl_repo_info ---

type repoInfoInput struct {
	Name string `json:"name" jsonschema:"case-insensitive regex or substring matched against the repo name"`
	formatParam
}

type repoInfoMatch struct {
	Name           string `json:"name"             jsonschema:"org/repo name"`
	Path           string `json:"path"             jsonschema:"absolute filesystem path"`
	Remote         string `json:"remote,omitempty" jsonschema:"full origin remote URL"`
	Host           string `json:"host,omitempty"   jsonschema:"git host"`
	Branch         string `json:"branch"           jsonschema:"current git branch"`
	Dirty          bool   `json:"dirty"            jsonschema:"working tree has uncommitted changes"`
	ModifiedFiles  int    `json:"modified_files"   jsonschema:"count of modified tracked files"`
	UntrackedFiles int    `json:"untracked_files"  jsonschema:"count of untracked files"`
	IndexStale     bool   `json:"index_stale"      jsonschema:"zoekt index doesn't reflect current state"`
	IndexedAt      string `json:"indexed_at"       jsonschema:"when last indexed (RFC3339 or never)"`
	Action         string `json:"action"           jsonschema:"suggested action: ready, commit_or_stash, pull_recommended, needs_reindex"`
}

type repoInfoOutput struct {
	Matches []repoInfoMatch `json:"matches" jsonschema:"matching repos with health info"`
}

func handleRepoInfo(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in repoInfoInput,
) (*mcp.CallToolResult, repoInfoOutput, error) {
	re, err := finder.CompileMatcher(in.Name)
	if err != nil {
		return nil, repoInfoOutput{}, err
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, repoInfoOutput{}, fmt.Errorf("load csl config: %w", err)
	}

	repos, err := cfg.DiscoverRepos()
	if err != nil {
		return nil, repoInfoOutput{}, fmt.Errorf("walk repos: %w", err)
	}

	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return nil, repoInfoOutput{}, fmt.Errorf("index dir: %w", err)
	}

	state, err := search.LoadState(indexDir)
	if err != nil {
		return nil, repoInfoOutput{}, fmt.Errorf("load index state: %w", err)
	}

	matches := make([]repoInfoMatch, 0, 4)
	for _, r := range repos {
		if !re.MatchString(r.Name) {
			continue
		}

		m := repoInfoMatch{
			Name:   r.Name,
			Path:   r.Path,
			Remote: r.Remote,
			Host:   r.Host,
		}

		// Git state
		fp, err := search.Fingerprint(r.Path)
		if err == nil {
			m.Branch = fp.Branch
			m.Dirty = fp.Dirty
		}

		mod, untracked, err := search.DirtyInfo(r.Path)
		if err == nil {
			m.ModifiedFiles = mod
			m.UntrackedFiles = untracked
		}

		// Index state
		stored, ok := state.GetRepo(r.Path)
		if !ok {
			m.IndexStale = true
			m.IndexedAt = "never"
		} else {
			m.IndexedAt = stored.IndexedAt.Format(time.RFC3339)
			m.IndexStale = stored.Fingerprint != fp.Fingerprint
		}

		// Derive action
		m.Action = deriveAction(m)
		matches = append(matches, m)
	}

	return nil, repoInfoOutput{Matches: matches}, nil
}

func deriveAction(m repoInfoMatch) string {
	if m.Dirty {
		return "commit_or_stash"
	}
	if m.IndexedAt == "never" {
		return "needs_reindex"
	}
	if m.IndexStale {
		return "needs_reindex"
	}
	// Check if index is old (>30min) — likely behind remote
	if t, err := time.Parse(time.RFC3339, m.IndexedAt); err == nil {
		if time.Since(t) > 30*time.Minute {
			return "pull_recommended"
		}
	}
	return "ready"
}

// --- csl_repo_health ---

type repoHealthInput struct {
	All   bool `json:"all,omitempty"   jsonschema:"include clean repos too; default returns only repos needing attention"`
	Fresh bool `json:"fresh,omitempty" jsonschema:"bypass the cached sweep and recompute now; default serves a recent cached sweep (up to a few minutes old)"`
	formatParam
}

// repoHealthEntry is one repo in the csl_repo_health result. It mirrors
// search.GitHealth with jsonschema descriptions for the tool contract.
type repoHealthEntry struct {
	Name           string `json:"name"             jsonschema:"org/repo name"`
	Path           string `json:"path"             jsonschema:"absolute filesystem path"`
	Host           string `json:"host,omitempty"   jsonschema:"git host"`
	Branch         string `json:"branch,omitempty" jsonschema:"current git branch; HEAD when detached"`
	Dirty          bool   `json:"dirty"            jsonschema:"working tree has uncommitted changes"`
	ModifiedFiles  int    `json:"modified_files"   jsonschema:"count of modified tracked files"`
	UntrackedFiles int    `json:"untracked_files"  jsonschema:"count of untracked files"`
	Ahead          int    `json:"ahead"            jsonschema:"commits on HEAD not on the upstream (unpushed work)"`
	Behind         int    `json:"behind"           jsonschema:"commits on the last-fetched upstream not on HEAD"`
	HasUpstream    bool   `json:"has_upstream"     jsonschema:"false when the branch has no upstream to compare against"`
	Action         string `json:"action"           jsonschema:"suggested action: commit_or_stash, diverged, push_recommended, pull_recommended, no_upstream, detached_head, error, ready"`
	Error          string `json:"error,omitempty"  jsonschema:"git failure for this repo; the sweep continues past it"`
}

type repoHealthOutput struct {
	Total      int               `json:"total"                 jsonschema:"repos checked"`
	Attention  int               `json:"attention"             jsonschema:"repos needing attention (action != ready)"`
	ComputedAt string            `json:"computed_at,omitempty" jsonschema:"when this sweep was computed (RFC3339); a recent cached sweep is served unless fresh=true"`
	Repos      []repoHealthEntry `json:"repos"                 jsonschema:"per-repo git health; only repos needing attention unless all=true"`
}

// mcpHealthTTL bounds how stale a cached git-health sweep the MCP tool serves
// before recomputing. A sweep spawns git subprocesses per repo, so on a large
// fleet the cache keeps repeated calls cheap; fresh=true bypasses it.
const mcpHealthTTL = 5 * time.Minute

func handleRepoHealth(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in repoHealthInput,
) (*mcp.CallToolResult, repoHealthOutput, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, repoHealthOutput{}, fmt.Errorf("load csl config: %w", err)
	}

	repos, err := cfg.DiscoverRepos()
	if err != nil {
		return nil, repoHealthOutput{}, fmt.Errorf("walk repos: %w", err)
	}

	ttl := mcpHealthTTL
	if in.Fresh {
		ttl = 0
	}
	snap := search.CachedGitHealthSweep(ctx, repos, ttl)
	out := repoHealthOutput{Total: len(snap.Entries), Repos: []repoHealthEntry{}}
	if !snap.ComputedAt.IsZero() {
		out.ComputedAt = snap.ComputedAt.UTC().Format(time.RFC3339)
	}
	for _, e := range snap.Entries {
		if e.NeedsAttention() {
			out.Attention++
		}
		if in.All || e.NeedsAttention() {
			out.Repos = append(out.Repos, repoHealthEntry(e))
		}
	}
	return nil, out, nil
}

// --- csl_repo_pull ---

type repoPullInput struct {
	Name  string `json:"name"            jsonschema:"case-insensitive regex or substring matched against the repo name; must resolve to exactly one repo"`
	Force bool   `json:"force,omitempty" jsonschema:"pull even if uncommitted changes exist"`
	formatParam
}

type repoPullOutput struct {
	Name    string `json:"name"               jsonschema:"org/repo name"`
	Path    string `json:"path"               jsonschema:"absolute filesystem path"`
	Branch  string `json:"branch"             jsonschema:"current branch"`
	Updated bool   `json:"updated"            jsonschema:"true if HEAD changed after pull"`
	Warning string `json:"warning,omitempty"  jsonschema:"warning about repo state (dirty, detached HEAD)"`
	OldHEAD string `json:"old_head,omitempty" jsonschema:"HEAD before pull"`
	NewHEAD string `json:"new_head,omitempty" jsonschema:"HEAD after pull"`
}

func handleRepoPull(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in repoPullInput,
) (*mcp.CallToolResult, repoPullOutput, error) {
	if in.Name == "" {
		return nil, repoPullOutput{}, fmt.Errorf("name is required")
	}

	repo, err := resolveRepo(in.Name)
	if err != nil {
		return nil, repoPullOutput{}, err
	}

	out := repoPullOutput{
		Name: repo.Name,
		Path: repo.Path,
	}

	// Get current state
	fp, err := search.Fingerprint(repo.Path)
	if err != nil {
		return nil, repoPullOutput{}, fmt.Errorf("fingerprint %s: %w", repo.Name, err)
	}
	out.Branch = fp.Branch
	out.OldHEAD = fp.HEAD

	// Safety checks
	if fp.Branch == "HEAD" {
		out.Warning = "detached HEAD — pull may not work as expected"
		if !in.Force {
			return nil, out, nil
		}
	}
	if fp.Dirty && !in.Force {
		mod, untracked, _ := search.DirtyInfo(repo.Path)
		out.Warning = fmt.Sprintf(
			"uncommitted changes (%d modified, %d untracked) — use force=true to pull anyway",
			mod,
			untracked,
		)
		return nil, out, nil
	}

	// Pull
	cmd := exec.Command("git", "pull", "--ff-only")
	cmd.Dir = repo.Path
	if pullOut, err := cmd.CombinedOutput(); err != nil {
		return nil, repoPullOutput{}, fmt.Errorf(
			"git pull in %s: %w\n%s",
			repo.Path,
			err,
			strings.TrimSpace(string(pullOut)),
		)
	}

	// The pull changed git state; drop any cached fingerprint so staleness
	// checks re-read it instead of waiting out the TTL.
	search.InvalidateFingerprint(repo.Path)

	// Check new HEAD
	fpAfter, err := search.Fingerprint(repo.Path)
	if err == nil {
		out.NewHEAD = fpAfter.HEAD
		out.Updated = out.OldHEAD != out.NewHEAD
	}

	return nil, out, nil
}

// --- csl_repo_reindex ---

type repoReindexInput struct {
	Name string `json:"name" jsonschema:"case-insensitive regex or substring matched against the repo name; must resolve to exactly one repo"`
	formatParam
}

type repoReindexOutput struct {
	Name      string `json:"name"      jsonschema:"org/repo name"`
	Path      string `json:"path"      jsonschema:"absolute filesystem path"`
	Reindexed bool   `json:"reindexed" jsonschema:"true if indexing succeeded"`
	Duration  string `json:"duration"  jsonschema:"how long indexing took"`
}

func handleRepoReindex(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in repoReindexInput,
) (*mcp.CallToolResult, repoReindexOutput, error) {
	if in.Name == "" {
		return nil, repoReindexOutput{}, fmt.Errorf("name is required")
	}

	repo, err := resolveRepo(in.Name)
	if err != nil {
		return nil, repoReindexOutput{}, err
	}

	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return nil, repoReindexOutput{}, fmt.Errorf("index dir: %w", err)
	}

	start := time.Now()
	if err := search.IndexRepo(indexDir, repo); err != nil {
		return nil, repoReindexOutput{}, fmt.Errorf("index %s: %w", repo.Name, err)
	}
	dur := time.Since(start)

	// Update state
	state, err := search.LoadState(indexDir)
	if err == nil {
		fp, err := search.Fingerprint(repo.Path)
		if err == nil {
			fp.IndexedAt = time.Now()
			state.SetRepo(repo.Path, fp)
			_ = state.Save(indexDir)
		}
	}

	notify.EmitEventSync("csl", "info",
		"reindexed "+repo.Name, "",
		map[string]string{
			"repo":     repo.Name,
			"duration": dur.Truncate(time.Millisecond).String(),
		})

	return nil, repoReindexOutput{
		Name:      repo.Name,
		Path:      repo.Path,
		Reindexed: true,
		Duration:  dur.Truncate(time.Millisecond).String(),
	}, nil
}
