package cli

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl"
)

func TestDocsClaudeMDPrintsSnippet(t *testing.T) {
	// Cobra keeps flag values across Execute calls; leave the default behind
	// for whichever test runs next.
	t.Cleanup(func() { docsClaudeMDFlag = false })

	out, err := runCLI(t, "docs", "--claude-md")
	if err != nil {
		t.Fatalf("docs --claude-md: %v", err)
	}
	// A leading newline, then the snippet byte for byte: the newline is what
	// makes `>> ~/.claude/CLAUDE.md` safe on a file with no trailing newline.
	if out != "\n"+csl.ClaudeMD {
		t.Errorf("docs --claude-md printed %d bytes, want a newline plus the embedded snippet (%d bytes)",
			len(out), len(csl.ClaudeMD)+1)
	}
}

func TestDocsPrintsOperatingDoc(t *testing.T) {
	out, err := runCLI(t, "docs")
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	heading, _, _ := strings.Cut(csl.ClaudeMD, "\n")
	if strings.Contains(out, heading) {
		t.Error("docs without --claude-md printed the CLAUDE.md fragment")
	}
	if !strings.Contains(out, "csl") {
		t.Errorf("docs printed no operating doc:\n%s", out)
	}
}
