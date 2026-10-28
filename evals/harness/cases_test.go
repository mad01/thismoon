package main

import (
	"slices"
	"strings"
	"testing"
	"time"
)

const validCase = `---
id: where-defined
title: Where is X defined
workspace: repo:thismoon
timeout: 300
max_turns: 12
permission_mode: acceptEdits
disallowed_tools: [Edit, Write, "Bash(git push:*)"]
graders:
  - type: regex
    pattern: 'gitloc\.go'
  - type: tool_used
    tool: '^mcp__csl__csl_search$'
    input: 'sym:'
    weight: 3
  - type: max_turns
    max: 6
---
Where is CanonicalRepo defined?
`

func TestParseCase(t *testing.T) {
	c, err := parseCase(validCase)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "where-defined" || c.Title != "Where is X defined" {
		t.Errorf("id/title = %q/%q", c.ID, c.Title)
	}
	if c.Workspace != (WorkspaceSpec{Kind: "repo", Repo: "thismoon"}) {
		t.Errorf("workspace = %+v", c.Workspace)
	}
	if c.Timeout != 300*time.Second || c.MaxTurns != 12 || c.PermissionMode != "acceptEdits" {
		t.Errorf("limits = %s, %d, %q", c.Timeout, c.MaxTurns, c.PermissionMode)
	}
	if strings.Join(c.DisallowedTools, ",") != "Edit,Write,Bash(git push:*)" {
		t.Errorf("disallowed = %q", c.DisallowedTools)
	}
	if c.Prompt != "Where is CanonicalRepo defined?" {
		t.Errorf("prompt = %q", c.Prompt)
	}
	if len(c.Graders) != 3 {
		t.Fatalf("graders = %d, want 3", len(c.Graders))
	}
	if c.Graders[0].Weight != 1 || c.Graders[1].Weight != 3 {
		t.Errorf("weights = %g, %g; want default 1 and explicit 3",
			c.Graders[0].Weight, c.Graders[1].Weight)
	}
	if got := c.Graders[1].Name(); got != "tool_used ^mcp__csl__csl_search$ input sym:" {
		t.Errorf("name = %q", got)
	}
}

func TestParseCaseErrors(t *testing.T) {
	replace := func(old, new string) string { return strings.Replace(validCase, old, new, 1) }
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"no fence", "id: x\n", "frontmatter fence"},
		{"no closing fence", "---\nid: x\n", "closing"},
		{"missing id", replace("id: where-defined\n", ""), "missing id"},
		{"unknown key", replace("title:", "titel: x\ntitle:"), "titel"},
		{"bad workspace", replace("repo:thismoon", "cloud"), "want repo:"},
		{"repo without name", replace("repo:thismoon", `"repo:"`), "names no repo"},
		{"tmpdir with repo", replace("repo:thismoon", "tmpdir:x"), "takes no repo"},
		{"zero timeout", replace("timeout: 300", "timeout: 0"), "timeout"},
		{"unknown grader", replace("type: regex", "type: vibes"), "unknown grader type"},
		{"bad regex", replace(`'gitloc\.go'`, `'('`), "missing closing"},
		{"budget without max", replace("max: 6", "max: 0"), "max must be positive"},
		{"negative weight", replace("weight: 3", "weight: -1"), "negative"},
		{"commit outside worktree", replace("type: regex", "type: commit"), "worktree workspace"},
		{
			"empty prompt",
			strings.TrimSuffix(validCase, "Where is CanonicalRepo defined?\n"),
			"empty prompt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCase(tt.raw)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestSelectCases(t *testing.T) {
	cases := []Case{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got, err := selectCases(cases, "c, a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "c" || got[1].ID != "a" {
		t.Errorf("got %+v, want c then a", got)
	}
	if all, _ := selectCases(cases, ""); len(all) != 3 {
		t.Errorf("empty selection kept %d cases, want 3", len(all))
	}
	if _, err := selectCases(cases, "zzz"); err == nil {
		t.Error("unknown id: want an error")
	}
}

func TestParseCaseInputListAndNotes(t *testing.T) {
	raw := strings.Replace(validCase, "    input: 'sym:'\n",
		"    input: ['sym:', 'thismoon']\n    include_agents: true\n", 1)
	raw = strings.Replace(raw, "title: Where is X defined\n",
		"title: Where is X defined\nnotes: budgets are estimates\n", 1)
	c, err := parseCase(raw)
	if err != nil {
		t.Fatal(err)
	}
	g := c.Graders[1]
	if len(g.Input) != 2 || !g.IncludeAgents || len(g.inputREs) != 2 {
		t.Errorf("grader = %+v", g)
	}
	if c.Notes != "budgets are estimates" {
		t.Errorf("notes = %q", c.Notes)
	}
	if got := g.Name(); got != "tool_used ^mcp__csl__csl_search$ input sym: & thismoon incl. agents" {
		t.Errorf("name = %q", got)
	}
}

func TestDisallowed(t *testing.T) {
	repo := Case{
		Workspace:       WorkspaceSpec{Kind: workspaceRepo, Repo: "x"},
		DisallowedTools: []string{"Edit", "CaseOnly"},
	}
	got := repo.Disallowed()
	for _, want := range []string{
		"CaseOnly", "Write", "Bash(git commit:*)", "mcp__worklog__worklog_checkpoint",
		"mcp__brain__brain_write", "mcp__csl__csl_repo_pull",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("repo case list misses %q", want)
		}
	}
	edits := 0
	for _, tool := range got {
		if tool == "Edit" {
			edits++
		}
	}
	if edits != 1 {
		t.Errorf("Edit listed %d times, want once", edits)
	}
	for _, kind := range []string{workspaceWorktree, workspaceTmpdir} {
		c := Case{
			Workspace:       WorkspaceSpec{Kind: kind},
			DisallowedTools: []string{"Bash(git push:*)"},
		}
		got := c.Disallowed()
		for _, want := range []string{
			"Bash(git push:*)", "mcp__worklog__worklog_checkpoint", "mcp__brain__brain_edit",
			"mcp__gh_com__merge_pull_request", "mcp__gh_com__push_files",
		} {
			if !slices.Contains(got, want) {
				t.Errorf("%s case list misses %q", kind, want)
			}
		}
		if slices.Contains(got, "Edit") || slices.Contains(got, "Bash(git commit:*)") {
			t.Errorf("%s case got the repo-only deny list: %v", kind, got)
		}
	}
}
