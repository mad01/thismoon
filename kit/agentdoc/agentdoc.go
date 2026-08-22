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
// where to turn on error.
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
	if f.HasDoctor {
		lines = append(lines, fmt.Sprintf(
			"On any tool error or unexpected empty result: run '%s doctor'. Full doc: '%s docs'.",
			f.Bin, f.Bin))
	} else {
		lines = append(lines, fmt.Sprintf(
			"On any tool error or unexpected empty result: see '%s docs'.", f.Bin))
	}
	return strings.Join(lines, "\n")
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
