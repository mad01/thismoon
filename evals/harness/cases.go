package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Workspace kinds a case can run in.
const (
	workspaceRepo     = "repo"
	workspaceWorktree = "worktree"
	workspaceTmpdir   = "tmpdir"
)

// Grader types a case can declare.
const (
	gradeRegex        = "regex"
	gradeFile         = "file"
	gradeCommit       = "commit"
	gradeToolUsed     = "tool_used"
	gradeToolNotUsed  = "tool_not_used"
	gradeToolBefore   = "tool_before"
	gradeMaxTurns     = "max_turns"
	gradeMaxCostUSD   = "max_cost_usd"
	gradeMaxToolCalls = "max_tool_calls"
	gradeLLM          = "llm"
)

// Case is one eval task: where it runs, the limits it runs under, the
// graders that score it, and the prompt.
type Case struct {
	ID              string
	Title           string
	Notes           string
	Workspace       WorkspaceSpec
	Timeout         time.Duration
	MaxTurns        int
	PermissionMode  string
	DisallowedTools []string
	Graders         []Grader
	Prompt          string
	Path            string
}

// WorkspaceSpec is a parsed case workspace: the kind and, for repo and
// worktree kinds, the repo name or path it names.
type WorkspaceSpec struct {
	Kind string
	Repo string
}

// String renders the spec the way a case file writes it.
func (w WorkspaceSpec) String() string {
	if w.Repo == "" {
		return w.Kind
	}
	return w.Kind + ":" + w.Repo
}

// Grader is one scored check. Type picks which fields apply:
//
//	regex           Pattern matches the final result text
//	file            Path exists in the workspace; Pattern, if set, matches its content
//	commit          the run made at least one commit and every subject matches Pattern
//	tool_used       some tool call matches Tool, every Input, and not InputWithout
//	tool_not_used   no tool call matches Tool, every Input, and not InputWithout
//	tool_before     a call matching Tool happens before any call matching Before
//	max_turns       the run took at most Max turns
//	max_cost_usd    the run cost at most Max dollars
//	max_tool_calls  the run made at most Max tool calls
//	llm             a haiku judge passes the result against Rubric
//
// Input (one regex or a list) and InputWithout are regexes on the call's
// input as compact JSON with sorted keys, so {"file_path":"x.go"} is a Read
// with no range. The tool graders and max_tool_calls read the main
// session's calls only, unless IncludeAgents adds the subagents' calls.
type Grader struct {
	Type          string    `yaml:"type"`
	Weight        float64   `yaml:"weight"`
	Pattern       string    `yaml:"pattern"`
	Path          string    `yaml:"path"`
	Tool          string    `yaml:"tool"`
	Input         regexList `yaml:"input"`
	InputWithout  string    `yaml:"input_without"`
	Before        string    `yaml:"before"`
	IncludeAgents bool      `yaml:"include_agents"`
	Max           float64   `yaml:"max"`
	Rubric        string    `yaml:"rubric"`

	patternRE, toolRE, withoutRE, beforeRE *regexp.Regexp
	inputREs                               []*regexp.Regexp
}

// regexList is a YAML field that takes one regex or a list of them.
type regexList []string

// UnmarshalYAML accepts a scalar or a sequence of scalars.
func (l *regexList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*l = regexList{node.Value}
		return nil
	}
	var many []string
	if err := node.Decode(&many); err != nil {
		return err
	}
	*l = many
	return nil
}

// Name is a short label for reports: the type and the fields that make
// this grader different from another of the same type.
func (g Grader) Name() string {
	parts := []string{g.Type}
	add := func(label, v string) {
		if v != "" {
			parts = append(parts, label+v)
		}
	}
	add("", g.Tool)
	add("input ", strings.Join(g.Input, " & "))
	add("without ", g.InputWithout)
	add("before ", g.Before)
	add("", g.Path)
	add("", g.Pattern)
	if g.Max > 0 {
		parts = append(parts, fmt.Sprintf("%g", g.Max))
	}
	if g.IncludeAgents {
		parts = append(parts, "incl. agents")
	}
	return strings.Join(parts, " ")
}

// sharedDenyList is denied to every case, whatever its workspace: these
// tools write to stores and services that outlive the run (kof, worklog,
// brain, GitHub), which no workspace teardown can undo.
var sharedDenyList = []string{
	"mcp__kof__kof_assert", "mcp__kof__kof_retract",
	"mcp__worklog__worklog_checkpoint",
	"mcp__brain__brain_write", "mcp__brain__brain_edit",
	"mcp__gh_com__create_pull_request", "mcp__gh_com__update_pull_request",
	"mcp__gh_com__update_pull_request_branch", "mcp__gh_com__merge_pull_request",
	"mcp__gh_com__push_files", "mcp__gh_com__create_or_update_file",
	"mcp__gh_com__delete_file", "mcp__gh_com__issue_write", "mcp__gh_com__sub_issue_write",
	"mcp__gh_com__add_issue_comment", "mcp__gh_com__add_reply_to_pull_request_comment",
	"mcp__gh_com__pull_request_review_write", "mcp__gh_com__add_comment_to_pending_review",
	"mcp__gh_com__create_branch", "mcp__gh_com__create_repository",
	"mcp__gh_com__delete_repository", "mcp__gh_com__fork_repository",
}

// repoDenyList is also denied to a case in a repo workspace, which is the
// canonical checkout: these tools write to the checkout or its index. Bash
// patterns use the permission-rule prefix form --disallowedTools accepts.
var repoDenyList = []string{
	"Edit", "Write", "NotebookEdit", "MultiEdit",
	"Bash(git add:*)", "Bash(git am:*)", "Bash(git apply:*)", "Bash(git branch -d:*)",
	"Bash(git branch -D:*)", "Bash(git branch -m:*)", "Bash(git branch -M:*)",
	"Bash(git checkout:*)", "Bash(git cherry-pick:*)", "Bash(git clean:*)",
	"Bash(git commit:*)", "Bash(git fetch:*)", "Bash(git merge:*)", "Bash(git mv:*)",
	"Bash(git pull:*)", "Bash(git push:*)", "Bash(git rebase:*)", "Bash(git reset:*)",
	"Bash(git restore:*)", "Bash(git revert:*)", "Bash(git rm:*)",
	"Bash(git sparse-checkout:*)", "Bash(git stash:*)", "Bash(git switch:*)",
	"Bash(git tag:*)", "Bash(git update-ref:*)", "Bash(git worktree:*)",
	"Bash(sed -i:*)", "Bash(rm:*)", "Bash(mv:*)",
	"mcp__csl__csl_repo_pull", "mcp__csl__csl_repo_reindex",
}

// Disallowed is the --disallowedTools list a run of c gets: the case's own
// list, then sharedDenyList, then repoDenyList for a repo workspace,
// without duplicates.
func (c Case) Disallowed() []string {
	out := slices.Clone(c.DisallowedTools)
	add := func(tools []string) {
		for _, tool := range tools {
			if !slices.Contains(out, tool) {
				out = append(out, tool)
			}
		}
	}
	add(sharedDenyList)
	if c.Workspace.Kind == workspaceRepo {
		add(repoDenyList)
	}
	return out
}

// caseFile is the frontmatter as written; parseCase turns it into a Case.
type caseFile struct {
	ID              string   `yaml:"id"`
	Title           string   `yaml:"title"`
	Notes           string   `yaml:"notes"`
	Workspace       string   `yaml:"workspace"`
	Timeout         int      `yaml:"timeout"`
	MaxTurns        int      `yaml:"max_turns"`
	PermissionMode  string   `yaml:"permission_mode"`
	DisallowedTools []string `yaml:"disallowed_tools"`
	Graders         []Grader `yaml:"graders"`
}

// loadCases reads every *.md under dir, sorted by id.
func loadCases(dir string) ([]Case, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no case files under %s", dir)
	}
	var cases []Case
	seen := map[string]string{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		c, err := parseCase(string(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if other, dup := seen[c.ID]; dup {
			return nil, fmt.Errorf("%s: id %q already used by %s", path, c.ID, other)
		}
		seen[c.ID] = path
		c.Path = path
		cases = append(cases, c)
	}
	slices.SortFunc(cases, func(a, b Case) int { return strings.Compare(a.ID, b.ID) })
	return cases, nil
}

// parseCase decodes one case file: YAML frontmatter between "---" fences,
// then the prompt. Unknown keys are an error so a typo'd grader field
// fails the load instead of silently never checking anything.
func parseCase(raw string) (Case, error) {
	front, body, err := splitFrontmatter(raw)
	if err != nil {
		return Case{}, err
	}
	var f caseFile
	dec := yaml.NewDecoder(strings.NewReader(front))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return Case{}, fmt.Errorf("frontmatter: %w", err)
	}
	ws, err := parseWorkspace(f.Workspace)
	if err != nil {
		return Case{}, err
	}
	c := Case{
		ID:              f.ID,
		Title:           f.Title,
		Notes:           strings.TrimSpace(f.Notes),
		Workspace:       ws,
		Timeout:         time.Duration(f.Timeout) * time.Second,
		MaxTurns:        f.MaxTurns,
		PermissionMode:  f.PermissionMode,
		DisallowedTools: f.DisallowedTools,
		Graders:         f.Graders,
		Prompt:          strings.TrimSpace(body),
	}
	if err := validateCase(c); err != nil {
		return Case{}, err
	}
	for i := range c.Graders {
		if err := compileGrader(&c.Graders[i], ws); err != nil {
			return Case{}, fmt.Errorf("grader %d (%s): %w", i+1, c.Graders[i].Type, err)
		}
	}
	return c, nil
}

// parseWorkspace reads "repo:<name or path>", "worktree:<name or path>",
// or "tmpdir".
func parseWorkspace(s string) (WorkspaceSpec, error) {
	kind, repo, _ := strings.Cut(strings.TrimSpace(s), ":")
	switch kind {
	case workspaceRepo, workspaceWorktree:
		if repo == "" {
			return WorkspaceSpec{}, fmt.Errorf("workspace %q names no repo", s)
		}
	case workspaceTmpdir:
		if repo != "" {
			return WorkspaceSpec{}, fmt.Errorf("workspace %q: tmpdir takes no repo", s)
		}
	default:
		return WorkspaceSpec{}, fmt.Errorf(
			"workspace %q: want repo:<repo>, worktree:<repo>, or tmpdir", s)
	}
	return WorkspaceSpec{Kind: kind, Repo: repo}, nil
}

func validateCase(c Case) error {
	switch {
	case c.ID == "":
		return errors.New("missing id")
	case c.Title == "":
		return errors.New("missing title")
	case c.Timeout <= 0:
		return errors.New("timeout must be a positive number of seconds")
	case c.MaxTurns <= 0:
		return errors.New("max_turns must be positive")
	case len(c.Graders) == 0:
		return errors.New("no graders")
	case c.Prompt == "":
		return errors.New("empty prompt")
	}
	return nil
}

// compileGrader checks the fields g.Type needs, defaults the weight, and
// compiles every regex once so a bad pattern fails at load.
func compileGrader(g *Grader, ws WorkspaceSpec) error {
	if g.Weight < 0 {
		return errors.New("weight must not be negative")
	}
	if g.Weight == 0 {
		g.Weight = 1
	}
	if err := requireFields(*g, ws); err != nil {
		return err
	}
	for _, f := range []struct {
		src string
		dst **regexp.Regexp
	}{
		{g.Pattern, &g.patternRE},
		{g.Tool, &g.toolRE},
		{g.InputWithout, &g.withoutRE},
		{g.Before, &g.beforeRE},
	} {
		if f.src == "" {
			continue
		}
		re, err := regexp.Compile(f.src)
		if err != nil {
			return err
		}
		*f.dst = re
	}
	for _, src := range g.Input {
		re, err := regexp.Compile(src)
		if err != nil {
			return err
		}
		g.inputREs = append(g.inputREs, re)
	}
	return nil
}

func requireFields(g Grader, ws WorkspaceSpec) error {
	switch g.Type {
	case gradeRegex:
		return need("pattern", g.Pattern)
	case gradeFile:
		return need("path", g.Path)
	case gradeCommit:
		if ws.Kind != workspaceWorktree {
			return errors.New("commit needs a worktree workspace")
		}
		return need("pattern", g.Pattern)
	case gradeToolUsed, gradeToolNotUsed:
		return need("tool", g.Tool)
	case gradeToolBefore:
		if err := need("tool", g.Tool); err != nil {
			return err
		}
		return need("before", g.Before)
	case gradeMaxTurns, gradeMaxCostUSD, gradeMaxToolCalls:
		if g.Max <= 0 {
			return errors.New("max must be positive")
		}
		return nil
	case gradeLLM:
		return need("rubric", g.Rubric)
	default:
		return fmt.Errorf("unknown grader type %q", g.Type)
	}
}

func need(field, value string) error {
	if value == "" {
		return fmt.Errorf("missing %s", field)
	}
	return nil
}

// splitFrontmatter returns the text between the opening "---" line and the
// closing one, and everything after the closing fence.
func splitFrontmatter(raw string) (front, body string, err error) {
	raw = strings.TrimPrefix(raw, "\uFEFF")
	rest, ok := strings.CutPrefix(raw, "---\n")
	if !ok {
		return "", "", errors.New("file must start with a --- frontmatter fence")
	}
	front, body, ok = strings.Cut(rest, "\n---\n")
	if !ok {
		return "", "", errors.New("frontmatter has no closing --- fence")
	}
	return front, body, nil
}

// selectCases keeps the cases named in ids, a comma-separated list; an
// empty list keeps every case. A name that matches no case is an error.
func selectCases(cases []Case, ids string) ([]Case, error) {
	if strings.TrimSpace(ids) == "" {
		return cases, nil
	}
	byID := map[string]Case{}
	for _, c := range cases {
		byID[c.ID] = c
	}
	var out []Case
	for id := range strings.SplitSeq(ids, ",") {
		id = strings.TrimSpace(id)
		c, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown case %q", id)
		}
		out = append(out, c)
	}
	return out, nil
}

// writeList prints one line per case for -list.
func writeList(w io.Writer, cases []Case) {
	fmt.Fprintln(w, "| id | workspace | timeout | max turns | graders | title |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, c := range cases {
		fmt.Fprintf(w, "| %s | %s | %s | %d | %d | %s |\n",
			c.ID, c.Workspace, c.Timeout, c.MaxTurns, len(c.Graders), cell(c.Title))
	}
}

// cell escapes a value for a markdown table cell.
func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}
