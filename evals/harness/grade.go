package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Evidence is what graders read: the run's transcript, the directory its
// paths resolve against (the vault in a brain workspace), the subjects of
// the commits it made in a worktree, and the prompt the llm grader shows
// its judge. Files, Changes and Diff describe a brain workspace's vaults
// after the run: every file, and how the run changed it from the baseline.
// TwoVault marks a two-vault workspace, whose paths start with private/ or
// shared/; Branches are the shared origin's branches the run pushed, and
// GHCalls the fake gh's calls. They are empty for other workspaces.
type Evidence struct {
	Transcript Transcript
	Dir        string
	Commits    []string
	Prompt     string
	Files      []string
	Changes    []Change
	Diff       string
	TwoVault   bool
	Branches   []OriginBranch
	GHCalls    []string
}

// added reports whether p is a file the run created.
func (ev Evidence) added(p string) bool {
	return slices.ContainsFunc(ev.Changes, func(c Change) bool {
		return c.Status == "A" && c.Path == p
	})
}

// files lists every file under ev.Dir: the vault's list in a brain
// workspace, a walk of the directory otherwise.
func (ev Evidence) files() ([]string, error) {
	if ev.Files != nil {
		return ev.Files, nil
	}
	return listFiles(ev.Dir)
}

// judge is the llm grader's model call. It is an interface so tests grade
// without the claude binary.
type judge interface {
	Judge(ctx context.Context, in JudgeInput) (Verdict, error)
}

// JudgeInput is what the llm grader shows its judge: the rubric, the task,
// the run's final answer and, in a brain workspace, the vault diff.
type JudgeInput struct {
	Rubric string
	Prompt string
	Answer string
	Diff   string
}

// Verdict is a judge's answer on one rubric, with what the call cost.
type Verdict struct {
	Pass    bool    `json:"pass"`
	Reason  string  `json:"reason"`
	CostUSD float64 `json:"-"`
}

// GraderResult is one grader's outcome on one run.
type GraderResult struct {
	Grader string  `json:"grader"`
	Weight float64 `json:"weight"`
	Pass   bool    `json:"pass"`
	Detail string  `json:"detail,omitempty"`
}

// Grade is the scored outcome of one run. Score is the passed share of the
// total grader weight; Pass means every grader passed.
type Grade struct {
	Score        float64
	Pass         bool
	Results      []GraderResult
	JudgeCostUSD float64
}

// gradeRun runs every grader of c against ev. A nil judge fails llm
// graders with a detail that says so, rather than skipping them.
func gradeRun(ctx context.Context, c Case, ev Evidence, j judge) Grade {
	g := Grade{Pass: true}
	var total, passed float64
	for _, gr := range c.Graders {
		res, cost := evaluate(ctx, gr, ev, j)
		g.Results = append(g.Results, res)
		g.JudgeCostUSD += cost
		total += gr.Weight
		if res.Pass {
			passed += gr.Weight
		} else {
			g.Pass = false
		}
	}
	if total > 0 {
		g.Score = passed / total
	}
	return g
}

// evaluate runs one grader and returns its result and any judge cost.
func evaluate(ctx context.Context, g Grader, ev Evidence, j judge) (GraderResult, float64) {
	res := GraderResult{Grader: g.Name(), Weight: g.Weight}
	var cost float64
	calls := ev.Transcript.Calls(g.IncludeAgents)
	switch g.Type {
	case gradeRegex:
		res.Pass, res.Detail = checkRegex(g, ev.Transcript.Result)
	case gradeFile:
		res.Pass, res.Detail = checkFile(g, ev)
	case gradeFrontmatter:
		res.Pass, res.Detail = checkFrontmatter(g, ev)
	case gradeNewNotes:
		res.Pass, res.Detail = checkNewNotes(g, ev)
	case gradeVaultUnchangedExcept:
		res.Pass, res.Detail = checkUnchangedExcept(g, ev.Changes)
	case gradeWikilinks:
		res.Pass, res.Detail = checkWikilinks(g, ev)
	case gradeBrainCmd:
		res.Pass, res.Detail = checkBrainCmd(g, brainCallsIn(calls))
	case gradeCommit:
		res.Pass, res.Detail = checkCommits(g, ev.Commits)
	case gradeToolUsed, gradeToolNotUsed:
		res.Pass, res.Detail = checkToolUse(g, calls)
	case gradeToolBefore:
		res.Pass, res.Detail = checkOrder(g, calls)
	case gradeMaxTurns:
		res.Pass, res.Detail = atMost(float64(ev.Transcript.NumTurns), *g.Max, "turns")
	case gradeMaxCostUSD:
		res.Pass, res.Detail = atMost(ev.Transcript.CostUSD, *g.Max, "USD")
	case gradeMaxToolCalls:
		res.Pass, res.Detail = atMost(float64(len(calls)), *g.Max, "tool calls")
	case gradeLLM:
		res.Pass, res.Detail, cost = checkLLM(ctx, g, ev, j)
	default:
		res.Detail = "unknown grader type"
	}
	return res, cost
}

// checkRegex passes when the pattern matches the final result, or with
// Absent, when it does not.
func checkRegex(g Grader, result string) (bool, string) {
	loc := g.patternRE.FindStringIndex(result)
	switch {
	case g.Absent && loc != nil:
		return false, fmt.Sprintf("result contains %q", clip(result[loc[0]:loc[1]], 80))
	case !g.Absent && loc == nil:
		return false, "result does not match"
	}
	return true, ""
}

// scope returns the files g.Path matches, relative to ev.Dir, only the ones
// the run added when g.NewOnly is set. A literal path outside a brain
// workspace is checked on its own, so a repo is not walked for one file.
func scope(g Grader, ev Evidence) ([]string, error) {
	if ev.Files == nil && !isGlob(g.Path) {
		_, err := os.Stat(filepath.Join(ev.Dir, g.Path))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return nil, nil
		case err != nil:
			return nil, err
		}
		return []string{g.Path}, nil
	}
	files, err := ev.files()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		if matchGlob(g.Path, f) && (!g.NewOnly || ev.added(f)) {
			out = append(out, f)
		}
	}
	return out, nil
}

// checkFile passes when a file matches the path and, with a pattern, one
// of the matching files has content that matches it. Absent turns both
// around: no file matches the path or, with a pattern, none of the matching
// files has content that matches it.
func checkFile(g Grader, ev Evidence) (bool, string) {
	files, err := scope(g, ev)
	switch {
	case err != nil:
		return false, err.Error()
	case len(files) == 0:
		if g.Absent {
			return true, ""
		}
		return false, "missing"
	case g.patternRE == nil:
		if g.Absent {
			return false, "found " + listSome(files)
		}
		return true, ""
	}
	hit, excerpt, err := firstMatch(ev.Dir, files, g.patternRE)
	switch {
	case err != nil:
		return false, err.Error()
	case g.Absent && hit != "":
		return false, fmt.Sprintf("%s contains %q", hit, clip(excerpt, 80))
	case !g.Absent && hit == "":
		return false, "content does not match in " + listSome(files)
	}
	return true, ""
}

// firstMatch returns the first of files under dir whose content re
// matches, and the matched text, or "" when none matches.
func firstMatch(dir string, files []string, re *regexp.Regexp) (string, string, error) {
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return "", "", err
		}
		if m := re.Find(data); m != nil {
			return f, string(m), nil
		}
	}
	return "", "", nil
}

// checkFrontmatter passes when a file in scope has g.Key with a value the
// pattern matches. A list value is matched one item per line.
func checkFrontmatter(g Grader, ev Evidence) (bool, string) {
	files, err := scope(g, ev)
	switch {
	case err != nil:
		return false, err.Error()
	case len(files) == 0:
		return false, "no file matches the path"
	}
	var misses []string
	for _, f := range files {
		value, ok, err := frontmatterValue(filepath.Join(ev.Dir, f), g.Key)
		switch {
		case err != nil:
			misses = append(misses, fmt.Sprintf("%s: %v", f, err))
		case !ok:
			misses = append(misses, f+": no "+g.Key)
		case g.patternRE.MatchString(value):
			return true, ""
		default:
			misses = append(misses, fmt.Sprintf("%s: %s is %q", f, g.Key, clip(value, 80)))
		}
	}
	return false, listSome(misses)
}

// checkNewNotes counts the markdown files the run added under g.Dir. It
// passes when the count is within min and max and, with content_regex,
// every one of them matches it.
func checkNewNotes(g Grader, ev Evidence) (bool, string) {
	var added []string
	for _, c := range ev.Changes {
		if c.Status == "A" && strings.HasSuffix(c.Path, ".md") && inDir(c.Path, g.Dir) {
			added = append(added, c.Path)
		}
	}
	detail := fmt.Sprintf("%d new: %s", len(added), listSome(added))
	if g.Min != nil && len(added) < *g.Min {
		return false, detail + fmt.Sprintf("; want at least %d", *g.Min)
	}
	if g.Max != nil && float64(len(added)) > *g.Max {
		return false, detail + "; want at most " + roundNum(*g.Max)
	}
	if g.contentRE == nil {
		return true, ""
	}
	for _, p := range added {
		data, err := os.ReadFile(filepath.Join(ev.Dir, p))
		if err != nil {
			return false, err.Error()
		}
		if !g.contentRE.Match(data) {
			return false, p + " does not match content_regex"
		}
	}
	return true, ""
}

func inDir(p, dir string) bool {
	dir = strings.Trim(dir, "/")
	return dir == "" || strings.HasPrefix(p, dir+"/")
}

// checkUnchangedExcept passes when every changed path matches one of
// g.Paths; with no paths the vault must be unchanged.
func checkUnchangedExcept(g Grader, changes []Change) (bool, string) {
	var stray []string
	for _, c := range changes {
		if !slices.ContainsFunc(
			g.Paths,
			func(glob string) bool { return matchGlob(glob, c.Path) },
		) {
			stray = append(stray, c.Status+" "+c.Path)
		}
	}
	if len(stray) > 0 {
		return false, "unexpected changes: " + listSome(stray)
	}
	return true, ""
}

// checkWikilinks passes when every wikilink in the files in scope resolves
// to a note and each file links at least min distinct notes.
func checkWikilinks(g Grader, ev Evidence) (bool, string) {
	files, err := scope(g, ev)
	if err != nil {
		return false, err.Error()
	}
	if len(files) == 0 {
		return false, "no file matches the path"
	}
	all, err := ev.files()
	if err != nil {
		return false, err.Error()
	}
	links := newLinkIndex(all, ev.TwoVault)
	minLinks := 0
	if g.Min != nil {
		minLinks = *g.Min
	}
	var problems []string
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(ev.Dir, f))
		if err != nil {
			return false, err.Error()
		}
		resolved := map[string]bool{}
		for _, target := range wikilinkTargets(string(data)) {
			if note, ok := links.resolve(f, target); ok {
				resolved[note] = true
			} else {
				problems = append(problems, fmt.Sprintf("%s: [[%s]] does not resolve", f, target))
			}
		}
		if len(resolved) < minLinks {
			problems = append(
				problems,
				fmt.Sprintf("%s links %d notes, want %d", f, len(resolved), minLinks),
			)
		}
	}
	if len(problems) > 0 {
		return false, listSome(problems)
	}
	return true, ""
}

// checkBrainCmd counts the brain calls whose subcommand label matches the
// pattern, help lookups left out. It passes when the count is at least min
// and at most max; min defaults to 1, or to 0 when max is set, so max: 0
// alone forbids the subcommand.
func checkBrainCmd(g Grader, calls []BrainCall) (bool, string) {
	n := 0
	for _, c := range calls {
		if !c.isHelp() && g.patternRE.MatchString(c.SubcommandLabel()) {
			n++
		}
	}
	minCalls := 1
	if g.Max != nil {
		minCalls = 0
	}
	if g.Min != nil {
		minCalls = *g.Min
	}
	if n >= minCalls && (g.Max == nil || float64(n) <= *g.Max) {
		return true, ""
	}
	ran := "none"
	if len(calls) > 0 {
		ran = formatCounts(brainCommandCounts(calls))
	}
	return false, fmt.Sprintf("%d matching brain calls; ran: %s", n, ran)
}

func checkCommits(g Grader, subjects []string) (bool, string) {
	if len(subjects) == 0 {
		return false, "no commits"
	}
	for _, s := range subjects {
		if !g.patternRE.MatchString(s) {
			return false, fmt.Sprintf("subject %q does not match", s)
		}
	}
	return true, ""
}

// checkToolUse passes tool_used when some call matches and tool_not_used
// when none does.
func checkToolUse(g Grader, calls []ToolCall) (bool, string) {
	matched := matchingCalls(g, calls)
	switch {
	case g.Type == gradeToolUsed && len(matched) == 0:
		return false, "no matching call; tools: " + toolSummary(calls)
	case g.Type == gradeToolNotUsed && len(matched) > 0:
		return false, fmt.Sprintf("%d matching calls, first %s %s",
			len(matched), matched[0].Name, clip(matched[0].Input, 160))
	}
	return true, ""
}

// matchingCalls returns the calls whose name matches g.Tool and whose input
// matches g.Input and not g.InputWithout, each only when set.
func matchingCalls(g Grader, calls []ToolCall) []ToolCall {
	var out []ToolCall
	for _, c := range calls {
		if callMatches(g, c) {
			out = append(out, c)
		}
	}
	return out
}

func callMatches(g Grader, c ToolCall) bool {
	if !g.toolRE.MatchString(c.Name) {
		return false
	}
	for _, re := range g.inputREs {
		if !re.MatchString(c.Input) {
			return false
		}
	}
	return g.withoutRE == nil || !g.withoutRE.MatchString(c.Input)
}

// checkOrder passes when a call matching g.Tool comes before every call
// whose name matches g.Before. A run that never calls the Before tool
// passes as long as it calls the first one.
func checkOrder(g Grader, calls []ToolCall) (bool, string) {
	first := slices.IndexFunc(calls, func(c ToolCall) bool { return callMatches(g, c) })
	if first < 0 {
		return false, "first tool never called"
	}
	before := slices.IndexFunc(
		calls,
		func(c ToolCall) bool { return g.beforeRE.MatchString(c.Name) },
	)
	if before >= 0 && before < first {
		return false, fmt.Sprintf("%s at call %d, first match at call %d",
			calls[before].Name, before+1, first+1)
	}
	return true, ""
}

func atMost(got, limit float64, unit string) (bool, string) {
	detail := fmt.Sprintf("%s %s, max %s", roundNum(got), unit, roundNum(limit))
	return got <= limit, detail
}

// roundNum renders a number with at most four decimals, so a summed cost
// reads 1.3177 and not 1.3176865000000002.
func roundNum(x float64) string {
	return strconv.FormatFloat(math.Round(x*1e4)/1e4, 'f', -1, 64)
}

func checkLLM(ctx context.Context, g Grader, ev Evidence, j judge) (bool, string, float64) {
	if j == nil {
		return false, "no judge configured", 0
	}
	v, err := j.Judge(ctx, JudgeInput{
		Rubric: g.Rubric, Prompt: ev.Prompt, Answer: ev.Transcript.Result, Diff: ev.Diff,
	})
	if err != nil {
		return false, "judge: " + err.Error(), v.CostUSD
	}
	return v.Pass, clip(v.Reason, 300), v.CostUSD
}

// toolSummary renders calls as "Name n, Name n", sorted by name, for
// failure details.
func toolSummary(calls []ToolCall) string {
	if len(calls) == 0 {
		return "none"
	}
	counts := map[string]int{}
	for _, c := range calls {
		counts[c.Name]++
	}
	return formatCounts(counts)
}

func formatCounts(counts map[string]int) string {
	parts := make([]string, 0, len(counts))
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		parts = append(parts, fmt.Sprintf("%s %d", name, counts[name]))
	}
	return strings.Join(parts, ", ")
}

// listSome renders up to five items and how many more there are.
func listSome(items []string) string {
	const shown = 5
	switch {
	case len(items) == 0:
		return "none"
	case len(items) <= shown:
		return strings.Join(items, "; ")
	}
	return fmt.Sprintf("%s; and %d more", strings.Join(items[:shown], "; "), len(items)-shown)
}

// clip bounds a detail excerpt.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
