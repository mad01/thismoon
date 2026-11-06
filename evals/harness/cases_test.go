package main

import (
	"os"
	"path/filepath"
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
		{"brain without fixture", replace("repo:thismoon", `"brain:"`), "brain:<fixture name>"},
		{"brain fixture path", replace("repo:thismoon", "brain:../team"), "brain:<fixture name>"},
		{
			"absent on a tool grader",
			replace("    weight: 3\n", "    weight: 3\n    absent: true\n"),
			"absent applies",
		},
		{
			"new_only on regex",
			replace("type: regex\n", "type: regex\n    new_only: true\n"),
			"new_only applies",
		},
		{
			"new_only outside brain",
			replace(
				"type: regex\n    pattern: 'gitloc\\.go'",
				"type: file\n    path: x\n    new_only: true",
			),
			"brain workspace",
		},
		{
			"new_notes outside brain",
			replace("type: regex\n    pattern: 'gitloc\\.go'", "type: new_notes\n    max: 0"),
			"brain workspace",
		},
		{
			"negative min",
			replace("type: regex\n", "type: brain_cmd\n    min: -1\n"),
			"min must not be negative",
		},
		{"bad glob", replace("type: regex\n", "type: file\n    path: '[x'\n"), "glob"},
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
	full := func(ids string) selection { return selection{IDs: ids, Profile: profileFull} }
	got, err := selectCases(cases, full("c, a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "c" || got[1].ID != "a" {
		t.Errorf("got %+v, want c then a", got)
	}
	if all, _ := selectCases(cases, full("")); len(all) != 3 {
		t.Errorf("empty selection kept %d cases, want 3", len(all))
	}
	if _, err := selectCases(cases, full("zzz")); err == nil {
		t.Error("unknown id: want an error")
	}
}

// TestSelectCasesProfileAndPending: brain workspaces run under brain-cli
// alone, brain-cli runs nothing else, and a pending case runs only when
// asked for.
func TestSelectCasesProfileAndPending(t *testing.T) {
	brain := WorkspaceSpec{Kind: workspaceBrain, Fixture: "team"}
	cases := []Case{
		{ID: "repo", Workspace: WorkspaceSpec{Kind: workspaceRepo, Repo: "x"}},
		{ID: "find", Workspace: brain},
		{ID: "later", Workspace: brain, Pending: "needs multi-vault"},
	}
	ids := func(cs []Case) string {
		var out []string
		for _, c := range cs {
			out = append(out, c.ID)
		}
		return strings.Join(out, ",")
	}
	tests := []struct {
		name    string
		sel     selection
		want    string
		wantErr string
	}{
		{"full skips brain", selection{Profile: profileFull}, "repo", ""},
		{"brain-cli skips the rest and pending", selection{Profile: profileBrainCLI}, "find", ""},
		{
			"pending on request",
			selection{Profile: profileBrainCLI, IncludePending: true},
			"find,later",
			"",
		},
		{
			"named brain case under full",
			selection{IDs: "find", Profile: profileFull},
			"",
			"brain-cli",
		},
		{
			"named repo case under brain-cli",
			selection{IDs: "repo", Profile: profileBrainCLI},
			"",
			"does not run",
		},
		{
			"named pending case",
			selection{IDs: "later", Profile: profileBrainCLI},
			"",
			"-include-pending",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectCases(cases, tt.sel)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || ids(got) != tt.want {
				t.Errorf("got %q, %v; want %q", ids(got), err, tt.want)
			}
		})
	}
	if _, err := selectCases(cases[:1], selection{Profile: profileBrainCLI}); err == nil {
		t.Error("brain-cli with no brain case: want an error")
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

const brainCase = `---
id: brain-capture
title: Capture to the inbox
workspace: brain:team
pending: needs multi-vault
timeout: 240
max_turns: 15
allowed_tools: [Bash(brain:*), Read]
graders:
  - type: new_notes
    dir: inbox
    min: 1
    max: 1
  - type: frontmatter
    path: inbox/*.md
    new_only: true
    key: type
    pattern: '^source$'
  - type: brain_cmd
    pattern: '^write$'
    max: 0
  - type: vault_unchanged_except
    paths: [inbox/*.md]
---
Save this.
`

func TestParseBrainCase(t *testing.T) {
	c, err := parseCase(brainCase)
	if err != nil {
		t.Fatal(err)
	}
	if c.Workspace != (WorkspaceSpec{Kind: workspaceBrain, Fixture: "team"}) ||
		c.Workspace.String() != "brain:team" {
		t.Errorf("workspace = %+v", c.Workspace)
	}
	if c.Pending != "needs multi-vault" ||
		!slices.Equal(c.AllowedTools, []string{"Bash(brain:*)", "Read"}) {
		t.Errorf("pending %q, allowed %v", c.Pending, c.AllowedTools)
	}
	names := []string{}
	for _, g := range c.Graders {
		names = append(names, g.Name())
	}
	want := []string{
		"new_notes dir inbox min 1 max 1",
		"frontmatter inbox/*.md key type ^source$ new only",
		"brain_cmd ^write$ max 0",
		"vault_unchanged_except except inbox/*.md",
	}
	if !slices.Equal(names, want) {
		t.Errorf("names =\n%q\nwant\n%q", names, want)
	}
	noBounds := strings.Replace(brainCase, "    min: 1\n    max: 1\n", "", 1)
	if _, err := parseCase(noBounds); err == nil ||
		!strings.Contains(err.Error(), "min, max or both") {
		t.Errorf("new_notes without bounds: %v", err)
	}
}

func TestCheckFixtures(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "team"), 0o755); err != nil {
		t.Fatal(err)
	}
	brain := func(fixture, pending string) Case {
		return Case{
			ID:        fixture,
			Workspace: WorkspaceSpec{Kind: workspaceBrain, Fixture: fixture},
			Pending:   pending,
		}
	}
	if err := checkFixtures([]Case{brain("team", ""), brain("team-multi", "later")}, root); err != nil {
		t.Errorf("a pending case's missing fixture failed the check: %v", err)
	}
	if err := checkFixtures([]Case{brain("team-multi", "")}, root); err == nil {
		t.Error("a missing fixture passed the check")
	}
}
