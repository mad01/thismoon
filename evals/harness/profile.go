package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

// Profiles pick what a case run has for code search. full is the user's
// session as it is; the other three model a workspace with csl as an MCP
// server plus the CLI (a machine with csl set up), with csl as a CLI only,
// and with grep alone.
const (
	profileFull   = "full"
	profileCSLMCP = "csl-mcp"
	profileCSLCLI = "csl-cli"
	profileNone   = "grep"
)

var profileNames = []string{profileFull, profileCSLMCP, profileCSLCLI, profileNone}

// defaultSteerFile is the csl rules fragment the steered profiles append to
// the system prompt: the file csl embeds and its recipe links to
// ~/.claude/CLAUDE.csl.md, so a run carries the text a workspace imports.
// The path is relative to the repo root, where the harness runs.
const defaultSteerFile = "services/csl/claude-md.md"

// defaultDenyCLIs are the thismoon CLIs a non-full profile takes away, so
// a run cannot route around the profile through a tool the workspace
// under test would not have. csl itself is handled per profile.
var defaultDenyCLIs = []string{
	"belt", "brain", "callgraph", "deps", "events", "ks", "moondb",
	"present", "status", "suspenders", "worklog",
}

// grepTools are the filesystem search tools -no-grep takes away from a
// steered profile, to measure the forced variant against the steered one.
var grepTools = []string{"Grep", "Bash(grep:*)", "Bash(rg:*)", "Bash(ugrep:*)", "Bash(find:*)"}

// cslCLIDeny is the deny pattern for the csl CLI.
const cslCLIDeny = "Bash(csl:*)"

// profile is one resolved profile: the flags it adds to every case run.
// The zero value is the full profile.
type profile struct {
	Name string
	// Steer is appended to the system prompt; "" appends nothing.
	Steer string
	// MCPConfig is the JSON for --mcp-config, passed with
	// --strict-mcp-config so the user's own servers stay out; "" leaves
	// the user's servers in.
	MCPConfig string
	// DisableHooks turns every hook off through --settings.
	DisableHooks bool
	Allow        []string
	Deny         []string
}

// profileOptions are the flags that shape a profile.
type profileOptions struct {
	Name      string
	SteerFile string
	DenyCLIs  []string
	NoGrep    bool
}

// loadProfile resolves a profile by name. The steered profiles read the
// steer file and add one line saying which form of csl the session has,
// so the rules file itself stays the same text a workspace would carry.
func loadProfile(o profileOptions) (profile, error) {
	if !slices.Contains(profileNames, o.Name) {
		return profile{}, fmt.Errorf("-profile %q: want one of %s", o.Name, strings.Join(profileNames, ", "))
	}
	p := profile{Name: o.Name}
	if o.Name == profileFull {
		if o.NoGrep {
			return profile{}, fmt.Errorf("-no-grep needs a profile other than %s", profileFull)
		}
		return p, nil
	}
	if o.Name == profileNone {
		if o.NoGrep {
			return profile{}, fmt.Errorf("-no-grep makes the %s profile unusable", profileNone)
		}
		p.Steer = noneSteer
		p.MCPConfig = emptyMCPConfig
		p.DisableHooks = true
		p.Deny = append(denyCLIPatterns(o.DenyCLIs), cslCLIDeny)
		return p, nil
	}
	steer, err := os.ReadFile(o.SteerFile)
	if err != nil {
		return profile{}, fmt.Errorf("steer file: %w", err)
	}
	p.Deny = denyCLIPatterns(o.DenyCLIs)
	if o.NoGrep {
		p.Deny = append(p.Deny, grepTools...)
	}
	switch o.Name {
	case profileCSLMCP:
		cfg, err := cslMCPConfig()
		if err != nil {
			return profile{}, err
		}
		p.MCPConfig = cfg
		p.Steer = string(steer) + "\n\n" + mcpSteer
		p.Allow = []string{"Bash(csl:*)"}
	case profileCSLCLI:
		p.MCPConfig = emptyMCPConfig
		p.Steer = string(steer) + "\n\n" + cliSteer
		p.Allow = []string{"Bash(csl:*)"}
	}
	return p, nil
}

// The one-line session facts the steered profiles add under the rules,
// and the whole system prompt addition of the none profile.
const (
	mcpSteer = "In this session the csl MCP tools (mcp__csl__*) are registered " +
		"and the csl CLI is on PATH as well, as on a machine with csl set up."
	cliSteer = "In this session no csl MCP tools are registered. " +
		"The csl CLI is on PATH; run it through Bash."
	noneSteer = "This session has only Claude Code's built-in tools: Bash, Read, Glob, Grep " +
		"and the editing tools. No MCP servers and no custom CLIs are available."
	emptyMCPConfig = `{"mcpServers":{}}`
)

// cslMCPConfig is the --mcp-config JSON registering csl alone, with the
// binary resolved on PATH so the run does not depend on how the user
// registered csl.
func cslMCPConfig() (string, error) {
	bin, err := exec.LookPath("csl")
	if err != nil {
		return "", fmt.Errorf("csl-mcp profile: %w", err)
	}
	cfg := map[string]any{"mcpServers": map[string]any{
		"csl": map[string]any{"command": bin, "args": []string{"mcp"}},
	}}
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// denyCLIPatterns renders Bash deny patterns for the named CLIs, csl
// excluded because each profile decides about it on its own.
func denyCLIPatterns(clis []string) []string {
	var out []string
	for _, c := range clis {
		c = strings.TrimSpace(c)
		if c == "" || c == "csl" {
			continue
		}
		out = append(out, "Bash("+c+":*)")
	}
	return out
}

// splitList parses a comma-separated flag value.
func splitList(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// cslCLIRE matches a Bash call's input whose command runs the csl CLI.
// The input is sorted-key JSON, so the command is quoted after "command".
var cslCLIRE = regexp.MustCompile(`"command":"[^"]*\bcsl (search|count|read|outline|repo|ls|hybrid|semantic|query|docs|doctor|index|config)\b`)

// cslCalls counts the csl calls in a transcript, MCP tools and CLI runs
// alike, across the main session and its subagents. It is the one number
// that says whether a profile's steer took: a case grades on its outcome
// whatever the profile, and this column shows how the outcome was reached.
func cslCalls(t Transcript) int {
	n := 0
	for _, c := range t.ToolCalls {
		switch {
		case strings.HasPrefix(c.Name, "mcp__csl__"):
			n++
		case c.Name == "Bash" && cslCLIRE.MatchString(c.Input):
			n++
		}
	}
	return n
}
