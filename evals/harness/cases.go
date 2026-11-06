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
	workspaceBrain    = "brain"
)

// Grader types a case can declare.
const (
	gradeRegex                = "regex"
	gradeFile                 = "file"
	gradeFrontmatter          = "frontmatter"
	gradeNewNotes             = "new_notes"
	gradeVaultUnchangedExcept = "vault_unchanged_except"
	gradeWikilinks            = "wikilinks"
	gradeBrainCmd             = "brain_cmd"
	gradeGHCall               = "gh_call"
	gradeOriginBranch         = "origin_branch"
	gradeCommit               = "commit"
	gradeToolUsed             = "tool_used"
	gradeToolNotUsed          = "tool_not_used"
	gradeToolBefore           = "tool_before"
	gradeMaxTurns             = "max_turns"
	gradeMaxCostUSD           = "max_cost_usd"
	gradeMaxToolCalls         = "max_tool_calls"
	gradeLLM                  = "llm"
)

// Case is one eval task: where it runs, the limits it runs under, the
// graders that score it, and the prompt. AllowedTools, when set, replaces
// the profile's allow list. Pending holds why a parked case does not run;
// it runs only when asked for.
type Case struct {
	ID              string
	Title           string
	Notes           string
	Workspace       WorkspaceSpec
	Timeout         time.Duration
	MaxTurns        int
	PermissionMode  string
	AllowedTools    []string
	DisallowedTools []string
	Pending         string
	Graders         []Grader
	Prompt          string
	Path            string
}

// WorkspaceSpec is a parsed case workspace: the kind, the repo name or
// path a repo or worktree kind names, and the fixture a brain kind names.
type WorkspaceSpec struct {
	Kind    string
	Repo    string
	Fixture string
}

// String renders the spec the way a case file writes it.
func (w WorkspaceSpec) String() string {
	switch {
	case w.Fixture != "":
		return w.Kind + ":" + w.Fixture
	case w.Repo != "":
		return w.Kind + ":" + w.Repo
	}
	return w.Kind
}

// Grader is one scored check. Type picks which fields apply:
//
//	regex                   Pattern matches the final result text
//	file                    a file matches Path; with Pattern, one of them matches it
//	frontmatter             a file matching Path has Key with a value matching Pattern
//	new_notes               the run added Min to Max notes under Dir, each matching ContentRegex
//	vault_unchanged_except  every path the run changed in the vault matches one of Paths
//	wikilinks               every wikilink in the files matching Path resolves, Min per file
//	brain_cmd               Min to Max brain calls have a subcommand matching Pattern
//	gh_call                 Min to Max of the fake gh's calls match Pattern
//	origin_branch           Min to Max pushed origin branches match Pattern, Path and ContentRegex
//	commit                  the run made at least one commit and every subject matches Pattern
//	tool_used               some tool call matches Tool, every Input, and not InputWithout
//	tool_not_used           no tool call matches Tool, every Input, and not InputWithout
//	tool_before             a call matching Tool happens before any call matching Before
//	max_turns               the run took at most Max turns
//	max_cost_usd            the run cost at most Max dollars
//	max_tool_calls          the run made at most Max tool calls
//	llm                     a haiku judge passes the result against Rubric
//
// Absent turns regex and file around: the pattern, or the file, must not be
// there. Path and Paths are slash globs where "**" spans directories,
// resolved against the vault in a brain workspace and against the workspace
// directory otherwise. NewOnly narrows file, frontmatter and wikilinks to
// the files the run added; it, new_notes and vault_unchanged_except read the
// vault's diff from its baseline, so they need a brain workspace, as do
// gh_call and origin_branch. brain_cmd, gh_call and origin_branch default
// Min to 1, or to 0 when Max is set; brain_cmd skips help lookups and
// denied calls. origin_branch's Pattern, Path and ContentRegex each apply
// only when set.
//
// Input (one regex or a list) and InputWithout are regexes on the call's
// input as compact JSON with sorted keys, so {"file_path":"x.go"} is a Read
// with no range. The tool graders, brain_cmd and max_tool_calls read the
// main session's calls only, unless IncludeAgents adds the subagents' calls.
type Grader struct {
	Type          string    `yaml:"type"`
	Weight        float64   `yaml:"weight"`
	Pattern       string    `yaml:"pattern"`
	Absent        bool      `yaml:"absent"`
	Path          string    `yaml:"path"`
	NewOnly       bool      `yaml:"new_only"`
	Key           string    `yaml:"key"`
	Dir           string    `yaml:"dir"`
	Paths         []string  `yaml:"paths"`
	ContentRegex  string    `yaml:"content_regex"`
	Tool          string    `yaml:"tool"`
	Input         regexList `yaml:"input"`
	InputWithout  string    `yaml:"input_without"`
	Before        string    `yaml:"before"`
	IncludeAgents bool      `yaml:"include_agents"`
	Min           *int      `yaml:"min"`
	Max           *float64  `yaml:"max"`
	Rubric        string    `yaml:"rubric"`

	patternRE, contentRE, toolRE, withoutRE, beforeRE *regexp.Regexp
	inputREs                                          []*regexp.Regexp
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
	flag := func(label string, on bool) {
		if on {
			parts = append(parts, label)
		}
	}
	add("", g.Tool)
	add("input ", strings.Join(g.Input, " & "))
	add("without ", g.InputWithout)
	add("before ", g.Before)
	add("", g.Path)
	add("dir ", g.Dir)
	add("key ", g.Key)
	add("except ", strings.Join(g.Paths, ", "))
	add("", g.Pattern)
	add("content ", g.ContentRegex)
	if g.Min != nil {
		parts = append(parts, fmt.Sprintf("min %d", *g.Min))
	}
	if g.Max != nil {
		label := "max "
		if isBudget(g.Type) {
			label = ""
		}
		parts = append(parts, fmt.Sprintf("%s%g", label, *g.Max))
	}
	flag("absent", g.Absent)
	flag("new only", g.NewOnly)
	flag("incl. agents", g.IncludeAgents)
	return strings.Join(parts, " ")
}

// isBudget reports whether t is one of the budget graders, whose Max is
// the whole point and reads bare in a grader's name.
func isBudget(t string) bool {
	return t == gradeMaxTurns || t == gradeMaxCostUSD || t == gradeMaxToolCalls
}

// sharedDenyList is denied to every case, whatever its workspace: these
// tools write to stores and services that outlive the run (worklog, brain,
// GitHub), which no workspace teardown can undo.
var sharedDenyList = []string{
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
	AllowedTools    []string `yaml:"allowed_tools"`
	DisallowedTools []string `yaml:"disallowed_tools"`
	Pending         string   `yaml:"pending"`
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
		AllowedTools:    f.AllowedTools,
		DisallowedTools: f.DisallowedTools,
		Pending:         strings.TrimSpace(f.Pending),
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
// "brain:<fixture>", or "tmpdir". A brain fixture is a directory name under
// the fixture root, so it takes no path.
func parseWorkspace(s string) (WorkspaceSpec, error) {
	kind, ref, _ := strings.Cut(strings.TrimSpace(s), ":")
	switch kind {
	case workspaceRepo, workspaceWorktree:
		if ref == "" {
			return WorkspaceSpec{}, fmt.Errorf("workspace %q names no repo", s)
		}
	case workspaceBrain:
		if ref == "" || ref == "." || ref == ".." || strings.ContainsAny(ref, `/\`) {
			return WorkspaceSpec{}, fmt.Errorf("workspace %q: want brain:<fixture name>", s)
		}
		return WorkspaceSpec{Kind: kind, Fixture: ref}, nil
	case workspaceTmpdir:
		if ref != "" {
			return WorkspaceSpec{}, fmt.Errorf("workspace %q: tmpdir takes no repo", s)
		}
	default:
		return WorkspaceSpec{}, fmt.Errorf(
			"workspace %q: want repo:<repo>, worktree:<repo>, brain:<fixture>, or tmpdir", s)
	}
	return WorkspaceSpec{Kind: kind, Repo: ref}, nil
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
// compiles every regex and glob once so a bad pattern fails at load.
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
		{g.ContentRegex, &g.contentRE},
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
	for _, glob := range append([]string{g.Path}, g.Paths...) {
		if err := validateGlob(glob); err != nil {
			return err
		}
	}
	return nil
}

func requireFields(g Grader, ws WorkspaceSpec) error {
	if err := checkModifiers(g, ws); err != nil {
		return err
	}
	switch g.Type {
	case gradeRegex, gradeBrainCmd:
		return need("pattern", g.Pattern)
	case gradeFile, gradeWikilinks:
		return need("path", g.Path)
	case gradeFrontmatter:
		return errors.Join(need("path", g.Path), need("key", g.Key), need("pattern", g.Pattern))
	case gradeNewNotes:
		if g.Min == nil && g.Max == nil {
			return errors.New("new_notes needs min, max or both")
		}
		return needBrain(g.Type, ws)
	case gradeVaultUnchangedExcept, gradeOriginBranch:
		return needBrain(g.Type, ws)
	case gradeGHCall:
		return errors.Join(need("pattern", g.Pattern), needBrain(g.Type, ws))
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
		if g.Max == nil || *g.Max <= 0 {
			return errors.New("max must be positive")
		}
		return nil
	case gradeLLM:
		return need("rubric", g.Rubric)
	default:
		return fmt.Errorf("unknown grader type %q", g.Type)
	}
}

// checkModifiers rejects the fields that would change a grader's meaning
// where they do not apply, so a misplaced one fails the load instead of
// silently checking the opposite or nothing.
func checkModifiers(g Grader, ws WorkspaceSpec) error {
	switch {
	case g.Absent && g.Type != gradeRegex && g.Type != gradeFile:
		return errors.New("absent applies to regex and file only")
	case g.NewOnly && g.Type != gradeFile && g.Type != gradeFrontmatter && g.Type != gradeWikilinks:
		return errors.New("new_only applies to file, frontmatter and wikilinks only")
	case g.Min != nil && *g.Min < 0:
		return errors.New("min must not be negative")
	case g.Max != nil && *g.Max < 0:
		return errors.New("max must not be negative")
	case g.NewOnly:
		return needBrain("new_only", ws)
	}
	return nil
}

// needBrain fails what reads the vault's diff from its baseline outside a
// brain workspace, which has no baseline.
func needBrain(what string, ws WorkspaceSpec) error {
	if ws.Kind != workspaceBrain {
		return fmt.Errorf("%s needs a brain workspace", what)
	}
	return nil
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

// selection says which of the loaded cases a run takes. IDs is a
// comma-separated list; "" takes every case Profile runs. Pending cases run
// only with IncludePending.
type selection struct {
	IDs            string
	Profile        string
	IncludePending bool
}

// selectCases keeps the cases s names, or every case its profile runs when
// it names none. A brain workspace runs under the brain-cli profile and that
// profile runs nothing else, so naming a case the profile cannot run is an
// error, as are an unknown name and a pending case without IncludePending.
func selectCases(cases []Case, s selection) ([]Case, error) {
	if strings.TrimSpace(s.IDs) == "" {
		var out []Case
		for _, c := range cases {
			if runsUnder(c, s.Profile) && (c.Pending == "" || s.IncludePending) {
				out = append(out, c)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("no case runs under -profile %s", s.Profile)
		}
		return out, nil
	}
	byID := map[string]Case{}
	for _, c := range cases {
		byID[c.ID] = c
	}
	var out []Case
	for id := range strings.SplitSeq(s.IDs, ",") {
		id = strings.TrimSpace(id)
		c, ok := byID[id]
		switch {
		case !ok:
			return nil, fmt.Errorf("unknown case %q", id)
		case !runsUnder(c, s.Profile):
			return nil, fmt.Errorf("case %q (%s) does not run under -profile %s; "+
				"brain workspaces run under %s, and %s runs nothing else",
				id, c.Workspace, s.Profile, profileBrainCLI, profileBrainCLI)
		case c.Pending != "" && !s.IncludePending:
			return nil, fmt.Errorf("case %q is pending (%s); pass -include-pending", id, c.Pending)
		}
		out = append(out, c)
	}
	return out, nil
}

// runsUnder reports whether profile runs c: a brain workspace needs the
// brain-cli profile, and the brain-cli profile needs a brain workspace.
func runsUnder(c Case, profile string) bool {
	return (c.Workspace.Kind == workspaceBrain) == (profile == profileBrainCLI)
}

// checkFixtures fails when a brain case that is not pending names a fixture
// with no directory under root. A pending case may name a fixture that does
// not exist yet.
func checkFixtures(cases []Case, root string) error {
	for _, c := range cases {
		if c.Workspace.Kind != workspaceBrain || c.Pending != "" {
			continue
		}
		dir := filepath.Join(root, c.Workspace.Fixture)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return fmt.Errorf(
				"%s: fixture %s is not a directory under %s",
				c.Path,
				c.Workspace.Fixture,
				root,
			)
		}
	}
	return nil
}

// writeList prints one line per case for -list, pending ones marked.
func writeList(w io.Writer, cases []Case) {
	fmt.Fprintln(w, "| id | workspace | timeout | max turns | graders | title |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|")
	for _, c := range cases {
		title := c.Title
		if c.Pending != "" {
			title += " (pending: " + c.Pending + ")"
		}
		fmt.Fprintf(w, "| %s | %s | %s | %d | %d | %s |\n",
			c.ID, c.Workspace, c.Timeout, c.MaxTurns, len(c.Graders), cell(title))
	}
}

// cell escapes a value for a markdown table cell.
func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ")
}
