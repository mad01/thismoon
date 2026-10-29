package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeSteer(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "steer.md")
	if err := os.WriteFile(path, []byte("# csl rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadProfileFull(t *testing.T) {
	p, err := loadProfile(profileOptions{Name: profileFull, DenyCLIs: []string{"belt"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Steer != "" || p.MCPConfig != "" || p.DisableHooks || len(p.Deny) > 0 || len(p.Allow) > 0 {
		t.Errorf("full profile changes the session: %+v", p)
	}
	if _, err := loadProfile(profileOptions{Name: profileFull, NoGrep: true}); err == nil {
		t.Error("full with -no-grep: want an error")
	}
	if _, err := loadProfile(profileOptions{Name: "fast"}); err == nil {
		t.Error("unknown profile: want an error")
	}
}

// TestLoadProfileCSLCLI: the CLI profile keeps the rules file, drops
// every MCP server, allows the csl CLI and denies the other CLIs.
func TestLoadProfileCSLCLI(t *testing.T) {
	p, err := loadProfile(profileOptions{
		Name: profileCSLCLI, SteerFile: writeSteer(t), DenyCLIs: []string{"belt", "csl", " worklog "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.Steer, "# csl rules\n") || !strings.Contains(p.Steer, "csl CLI is on PATH") {
		t.Errorf("steer = %q", p.Steer)
	}
	if p.MCPConfig != emptyMCPConfig || p.DisableHooks {
		t.Errorf("profile = %+v", p)
	}
	if !slices.Equal(p.Allow, []string{"Bash(csl:*)"}) {
		t.Errorf("allow = %v", p.Allow)
	}
	if !slices.Equal(p.Deny, []string{"Bash(belt:*)", "Bash(worklog:*)"}) {
		t.Errorf("deny = %v", p.Deny)
	}
}

// TestLoadProfileCSLMCP: the MCP profile registers csl alone, keeps the
// csl CLI, and -no-grep adds the filesystem search tools.
func TestLoadProfileCSLMCP(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "csl")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(bin))
	p, err := loadProfile(profileOptions{Name: profileCSLMCP, SteerFile: writeSteer(t), NoGrep: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.MCPConfig, `"command":"`+bin+`"`) || !strings.Contains(p.MCPConfig, `"args":["mcp"]`) {
		t.Errorf("mcp config = %s", p.MCPConfig)
	}
	if !strings.Contains(p.Steer, "mcp__csl__") {
		t.Errorf("steer = %q", p.Steer)
	}
	for _, want := range grepTools {
		if !slices.Contains(p.Deny, want) {
			t.Errorf("deny %v missing %s", p.Deny, want)
		}
	}
	if slices.Contains(p.Deny, cslCLIDeny) || !slices.Equal(p.Allow, []string{"Bash(csl:*)"}) {
		t.Errorf("csl-mcp must keep the CLI: deny %v allow %v", p.Deny, p.Allow)
	}
}

func TestLoadProfileNone(t *testing.T) {
	p, err := loadProfile(profileOptions{Name: profileNone, DenyCLIs: defaultDenyCLIs})
	if err != nil {
		t.Fatal(err)
	}
	if p.MCPConfig != emptyMCPConfig || !p.DisableHooks || p.Steer != noneSteer {
		t.Errorf("profile = %+v", p)
	}
	if !slices.Contains(p.Deny, cslCLIDeny) || !slices.Contains(p.Deny, "Bash(belt:*)") {
		t.Errorf("deny = %v", p.Deny)
	}
	if _, err := loadProfile(profileOptions{Name: profileNone, NoGrep: true}); err == nil {
		t.Error("none with -no-grep: want an error")
	}
}

// TestInvocationProfileArgs: the profile's flags land before the tool
// lists, and the steer text is passed whole.
func TestInvocationProfileArgs(t *testing.T) {
	inv := invocation{
		SessionID: "sid", MaxTurns: 3, Prompt: "hi",
		Profile:      profile{Steer: "rules\nmore", MCPConfig: `{"mcpServers":{}}`, DisableHooks: true},
		AllowedTools: []string{"Bash(csl:*)"}, DisallowedTools: []string{"Grep"},
	}
	got := strings.Join(inv.args(), " ")
	want := "-p --output-format stream-json --verbose --session-id sid --max-turns 3 " +
		"--append-system-prompt rules\nmore --strict-mcp-config --mcp-config {\"mcpServers\":{}} " +
		`--settings {"disableAllHooks":true} --allowedTools Bash(csl:*) --disallowedTools Grep -- hi`
	if got != want {
		t.Errorf("args =\n%s\nwant\n%s", got, want)
	}
	tmpl := strings.Join(flagTemplate("", "", 0, inv.Profile), " ")
	if !strings.Contains(tmpl, "--append-system-prompt <steer>") || strings.Contains(tmpl, "rules") {
		t.Errorf("template keeps the steer text: %s", tmpl)
	}
}

func TestCSLCalls(t *testing.T) {
	tr := Transcript{ToolCalls: []ToolCall{
		{Name: "mcp__csl__csl_search", Input: `{"query":"x"}`, Origin: originMain},
		{Name: "mcp__csl__csl_count", Input: `{"query":"x"}`, Origin: originAgent},
		{Name: "Bash", Input: `{"command":"csl search foo --repo bar","description":"d"}`, Origin: originMain},
		{Name: "Bash", Input: `{"command":"cd /x && csl count foo | head","description":"d"}`, Origin: originMain},
		{Name: "Bash", Input: `{"command":"grep -r csl .","description":"d"}`, Origin: originMain},
		{Name: "Bash", Input: `{"command":"go test ./services/csl/...","description":"d"}`, Origin: originMain},
		{Name: "Grep", Input: `{"pattern":"csl search"}`, Origin: originMain},
	}}
	if got := cslCalls(tr); got != 4 {
		t.Errorf("cslCalls = %d, want 4", got)
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" a, b ,,c ")
	if !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("splitList = %v", got)
	}
	if got := splitList(""); got != nil {
		t.Errorf("splitList(\"\") = %v", got)
	}
}
