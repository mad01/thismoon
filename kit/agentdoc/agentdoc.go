// Package agentdoc renders a component's agent-facing operating doc, MCP
// instructions, and error hints from one set of mechanical facts. Components
// fill Facts from their own config and constants, so the rendered text
// cannot drift from the code that serves it.
package agentdoc

import (
	"fmt"
	"strings"
	"text/template"
)

// Facts holds the mechanical facts a component renders into its operating
// doc, MCP instructions, and error hints. Values come from the component's
// own config/constants so docs cannot drift from code.
type Facts struct {
	Name      string // component name, e.g. "keeper-of-facts"
	Bin       string // installed binary name, e.g. "kof"
	Purpose   string // one-line purpose, used in MCP instructions
	BaseURL   string // default backing-service base URL, e.g. "http://kof.this"; empty for CLI-only tools
	StorePath string // default on-disk store location; empty if none
	LogPath   string // default log location; empty if none
	HasDoctor bool   // whether the binary ships a doctor subcommand
	MCPNote   string // overrides the instructions transport line for MCPs that are not a shim over the service

	// MCPDoctorTool names the server's read-only doctor tool, e.g.
	// "kof_doctor". Empty when the server exposes none, which keeps the
	// instructions from advertising a tool that is not registered.
	MCPDoctorTool string
}

// Render executes the operating doc (text/template source) with f. A
// placeholder that names no Facts field is an error, so a typo fails tests
// instead of reaching readers.
func Render(doc string, f Facts) (string, error) {
	tmpl, err := template.New("operating").Option("missingkey=error").Parse(doc)
	if err != nil {
		return "", fmt.Errorf("agentdoc: parse operating doc: %w", err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, f); err != nil {
		return "", fmt.Errorf("agentdoc: render operating doc: %w", err)
	}
	return b.String(), nil
}

// Instructions returns the fixed MCP instructions block: purpose, the
// backing-service line (omitted for CLI-only tools with no BaseURL), and
// where to turn on error. Three lines at most: ten servers inject this into
// every session whether or not anything fails (ADR-0009).
func Instructions(f Facts) string {
	lines := []string{
		fmt.Sprintf("%s: %s.", f.Bin, strings.TrimSuffix(f.Purpose, ".")),
	}
	switch {
	case f.MCPNote != "":
		lines = append(lines, f.MCPNote)
	case f.BaseURL != "":
		lines = append(lines, fmt.Sprintf(
			"Tools call the local %s service (default %s) via this stdio shim; the service must be running.",
			f.Name, f.BaseURL))
	}
	return strings.Join(append(lines, recovery(f)), "\n")
}

// recovery is the instructions block's last line: what to reach for when a
// tool errors or comes back empty. The MCP doctor tool leads, because a
// client without a shell can call a tool but cannot run the binary; the
// doctor subcommand follows for the ones that can.
func recovery(f Facts) string {
	const lead = "On any tool error or unexpected empty result:"
	switch {
	case f.MCPDoctorTool != "" && f.HasDoctor:
		return fmt.Sprintf("%s call '%s' or run '%s doctor'. Full doc: '%s docs'.",
			lead, f.MCPDoctorTool, f.Bin, f.Bin)
	case f.MCPDoctorTool != "":
		return fmt.Sprintf("%s call '%s'. Full doc: '%s docs'.", lead, f.MCPDoctorTool, f.Bin)
	case f.HasDoctor:
		return fmt.Sprintf("%s run '%s doctor'. Full doc: '%s docs'.", lead, f.Bin, f.Bin)
	default:
		return fmt.Sprintf("%s see '%s docs'.", lead, f.Bin)
	}
}

// RegistrationSnippet returns the block that registers this component's MCP
// server with the two clients the platform targets, for the `mcp` command's
// help text. The wiring itself stays machine-private (ADR-0006), so the
// snippet is what a reader copies, not something the binary applies.
func RegistrationSnippet(f Facts) string {
	return fmt.Sprintf(`Register this server with a client:

  claude mcp add %s -- %s mcp

  # Codex, in ~/.codex/config.toml
  [mcp_servers.%s]
  command = "%s"
  args = ["mcp"]

Both assume %s is on the client's PATH; give an absolute path when it is not.`,
		f.Bin, f.Bin, f.Bin, f.Bin, f.Bin)
}

// Hint wraps err with a pointer to the component's self-diagnosis surface.
// It returns nil when err is nil; otherwise the result unwraps to err.
func Hint(err error, f Facts) error {
	if err == nil {
		return nil
	}
	if f.HasDoctor {
		return fmt.Errorf("%w (run '%s doctor' to diagnose)", err, f.Bin)
	}
	return fmt.Errorf("%w (see '%s docs' for troubleshooting)", err, f.Bin)
}
