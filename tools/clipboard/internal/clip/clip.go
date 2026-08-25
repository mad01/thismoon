// Package clip wraps the macOS pasteboard commands: pbcopy for writes,
// pbpaste for reads. The CLI and the MCP server are two thin frontends over
// this package, so a tool call and a manual run always hit the pasteboard
// the same way.
package clip

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/tools/clipboard"
)

// The pasteboard binaries are named absolutely: MCP hosts and launchd spawn
// processes with a minimal PATH, so resolving via PATH is the failure mode,
// not the default.
const (
	copyBin  = "/usr/bin/pbcopy"
	pasteBin = "/usr/bin/pbpaste"
)

// Board is the system pasteboard behind both frontends. Construct with New.
type Board struct {
	// run executes one pasteboard command with the given stdin and returns
	// its stdout. A field so tests can swap it without touching the real
	// clipboard.
	run func(bin, stdin string) (string, error)
}

// New returns a Board backed by the real pbcopy and pbpaste binaries.
func New() *Board { return &Board{run: run} }

// Copy replaces the pasteboard contents with text, verbatim: no trimming,
// no newline appended.
func (b *Board) Copy(text string) error {
	_, err := b.run(copyBin, text)
	// The one chokepoint every copy goes through (CLI and MCP); the hint
	// points the reader at the operating doc from here.
	return agentdoc.Hint(err, clipboard.Facts())
}

// Paste returns the pasteboard's current contents, verbatim. Non-text
// content (an image, a file reference) renders as pbpaste renders it:
// usually an empty string.
func (b *Board) Paste() (string, error) {
	out, err := b.run(pasteBin, "")
	return out, agentdoc.Hint(err, clipboard.Facts())
}

// run executes bin with stdin on its standard input and returns its stdout.
// stderr rides the error so "no such file" (not macOS) and pasteboard
// failures surface with the binary named.
func run(bin, stdin string) (string, error) {
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errOut.String()); msg != "" {
			return "", fmt.Errorf("clip: %s: %s: %w", bin, msg, err)
		}
		return "", fmt.Errorf("clip: %s: %w", bin, err)
	}
	return out.String(), nil
}
