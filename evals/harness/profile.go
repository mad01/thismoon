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
// session as it is; csl-mcp, csl-cli and grep model a workspace with csl as
// an MCP server plus the CLI (a machine with csl set up), with csl as a CLI
// only, and with grep alone. brain-cli runs the brain workspaces, with the
// user's setup shut out and the brain CLI as the agent's only way to write.
const (
	profileFull     = "full"
	profileCSLMCP   = "csl-mcp"
	profileCSLCLI   = "csl-cli"
	profileNone     = "grep"
	profileBrainCLI = "brain-cli"
)

var profileNames = []string{profileFull, profileCSLMCP, profileCSLCLI, profileNone, profileBrainCLI}

// The steer files the steered profiles append to the system prompt when
// -steer-file names none, relative to the repo root, where the harness
// runs. The csl one is the rules fragment csl embeds and its recipe links
// to ~/.claude/CLAUDE.csl.md, so a run carries the text a workspace
// imports; the brain one tells the agent the vault is behind the brain CLI.
const (
	defaultSteerFile      = "services/csl/claude-md.md"
	defaultBrainSteerFile = "evals/harness/steer/brain.md"
)

// defaultDenyCLIs are the thismoon CLIs a non-full profile takes away, so
// a run cannot route around the profile through a tool the workspace
// under test would not have. csl itself is handled per profile, and
// brain-cli keeps brain.
var defaultDenyCLIs = []string{
	"belt", "brain", "callgraph", "deps", "events", "ks", "moondb",
	"present", "status", "suspenders", "worklog",
}

// grepTools are the filesystem search tools -no-grep takes away from a
// steered profile, to measure the forced variant against the steered one.
var grepTools = []string{"Grep", "Bash(grep:*)", "Bash(rg:*)", "Bash(ugrep:*)", "Bash(find:*)"}

// cslCLIDeny is the deny pattern for the csl CLI.
const cslCLIDeny = "Bash(csl:*)"

// brainAllow is what the brain-cli profile allows: Bash for the brain CLI
// alone, the read-only file tools, and file writes under the work directory
// so the agent can hand brain a body file. The vault sits outside it.
var brainAllow = []string{"Bash(brain:*)", "Read", "Glob", "Grep", "Edit(./**)"}

// permissionDontAsk denies every tool call the allow list does not cover,
// instead of asking.
const permissionDontAsk = "dontAsk"

// profile is one resolved profile: the flags it adds to every case run.
// The zero value is the full profile.
type profile struct {
	Name string
	// Steer is appended to the system prompt; "" appends nothing.
	// SteerFile is the file it came from, as run.json records it.
	Steer     string
	SteerFile string
	// MCPConfig is the JSON for --mcp-config, passed with
	// --strict-mcp-config so the user's own servers stay out; "" leaves
	// the user's servers in.
	MCPConfig string
	// DisableHooks turns every hook off through --settings.
	DisableHooks bool
	// Isolated shuts the user's setup out: no setting sources (so no
	// CLAUDE.md, output style, permissions or plugins), no auto memory, no
	// session file, and of the built-in tools only those the allow list
	// names.
	Isolated bool
	// PermissionMode applies when a case sets none; "" leaves claude's.
	PermissionMode string
	Allow          []string
	Deny           []string
}

// profileOptions are the flags that shape a profile. SteerFile "" means
// the profile's own default.
type profileOptions struct {
	Name      string
	SteerFile string
	DenyCLIs  []string
	NoGrep    bool
}

// loadProfile resolves a profile by name. The csl profiles read the steer
// file and add one line saying which form of csl the session has, so the
// rules file itself stays the same text a workspace would carry.
func loadProfile(o profileOptions) (profile, error) {
	if !slices.Contains(profileNames, o.Name) {
		return profile{}, fmt.Errorf(
			"-profile %q: want one of %s",
			o.Name,
			strings.Join(profileNames, ", "),
		)
	}
	switch o.Name {
	case profileFull:
		if o.NoGrep {
			return profile{}, fmt.Errorf("-no-grep needs a profile other than %s", profileFull)
		}
		return profile{Name: o.Name}, nil
	case profileNone:
		if o.NoGrep {
			return profile{}, fmt.Errorf("-no-grep makes the %s profile unusable", profileNone)
		}
		return profile{
			Name: o.Name, Steer: noneSteer, MCPConfig: emptyMCPConfig, DisableHooks: true,
			Deny: appendNew(denyCLIPatterns(o.DenyCLIs, "csl"), cslCLIDeny),
		}, nil
	case profileBrainCLI:
		return brainCLIProfile(o)
	}
	return cslProfile(o)
}

// cslProfile resolves csl-mcp and csl-cli.
func cslProfile(o profileOptions) (profile, error) {
	steer, file, err := readSteer(o.SteerFile, defaultSteerFile)
	if err != nil {
		return profile{}, err
	}
	p := profile{
		Name: o.Name, SteerFile: file, Allow: []string{"Bash(csl:*)"},
		Deny: denyCLIPatterns(o.DenyCLIs, "csl"),
	}
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
		p.Steer = steer + "\n\n" + mcpSteer
	case profileCSLCLI:
		p.MCPConfig = emptyMCPConfig
		p.Steer = steer + "\n\n" + cliSteer
	}
	return p, nil
}

// brainCLIProfile resolves brain-cli: no MCP servers, no hooks, the user's
// setup shut out, and tool calls the allow list does not cover denied
// without asking. It keeps brain and denies the other thismoon CLIs, csl
// included.
func brainCLIProfile(o profileOptions) (profile, error) {
	steer, file, err := readSteer(o.SteerFile, defaultBrainSteerFile)
	if err != nil {
		return profile{}, err
	}
	p := profile{
		Name: o.Name, Steer: steer, SteerFile: file, MCPConfig: emptyMCPConfig,
		DisableHooks: true, Isolated: true, PermissionMode: permissionDontAsk,
		Allow: slices.Clone(brainAllow),
		Deny:  appendNew(denyCLIPatterns(o.DenyCLIs, "brain"), cslCLIDeny),
	}
	if o.NoGrep {
		p.Deny = appendNew(p.Deny, grepTools...)
	}
	return p, nil
}

// readSteer reads the steer file, def when file is "", and returns its
// text and the path it read.
func readSteer(file, def string) (string, string, error) {
	if file == "" {
		file = def
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", "", fmt.Errorf("steer file: %w", err)
	}
	return string(data), file, nil
}

// tools is the --tools list for a run whose allow list is allowed. An
// isolated profile gets the tool names in the permission rules, so
// "Bash(brain:*)" makes Bash available and the rule limits it. Claude Code
// checks every file-writing tool against Edit rules, so an Edit rule brings
// in Write as well. Any other profile gets nil, which leaves every built-in
// tool in.
func (p profile) tools(allowed []string) []string {
	if !p.Isolated {
		return nil
	}
	var out []string
	for _, rule := range allowed {
		name, _, _ := strings.Cut(rule, "(")
		name = strings.TrimSpace(name)
		names := []string{name}
		if name == "Edit" {
			names = append(names, "Write")
		}
		for _, n := range names {
			if n != "" && !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	return out
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

// denyCLIPatterns renders Bash deny patterns for the named CLIs, keep
// excluded: the CLI the profile is about, which it allows or denies on its
// own.
func denyCLIPatterns(clis []string, keep string) []string {
	var out []string
	for _, c := range clis {
		c = strings.TrimSpace(c)
		if c == "" || c == keep {
			continue
		}
		out = appendNew(out, "Bash("+c+":*)")
	}
	return out
}

// appendNew appends the items list does not hold yet.
func appendNew(list []string, items ...string) []string {
	for _, it := range items {
		if !slices.Contains(list, it) {
			list = append(list, it)
		}
	}
	return list
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
var cslCLIRE = regexp.MustCompile(
	`"command":"[^"]*\bcsl (search|count|read|outline|repo|ls|hybrid|semantic|query|docs|doctor|index|config)\b`,
)

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
