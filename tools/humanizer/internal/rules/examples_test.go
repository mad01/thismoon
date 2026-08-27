package rules

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// TestRulesFireOnTheirBeforeExamples runs every rule's documented
// before-example through Detect and asserts the rule fires on it. This is
// the parity gate between what `rules explain` shows users and what the
// pack actually detects — a rule whose own example passes clean is dead
// weight or a regex bug (the \b-wrap gotcha in CLAUDE.md silently killed
// four rules before this gate existed).
func TestRulesFireOnTheirBeforeExamples(t *testing.T) {
	if _, err := exec.LookPath("vale"); err != nil {
		t.Skip("vale not installed")
	}
	cacheDir := t.TempDir()
	for _, r := range All() {
		t.Run(r.ID, func(t *testing.T) {
			if strings.TrimSpace(r.Before) == "" {
				t.Fatalf("%s has no before example", r.ID)
			}
			findings, err := Detect(context.Background(), dedent(r.Before)+"\n", DetectOptions{
				CacheDir: cacheDir,
				Rules:    []string{r.ID},
			})
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if len(findings) == 0 {
				t.Fatalf("%s did not fire on its own before example:\n%s", r.ID, r.Before)
			}
		})
	}
}

// dedent strips the common leading indentation that multi-line YAML block
// examples carry after metadata parsing, so markdown constructs like
// headings sit at column zero the way they would in a real document.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	margin := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		indent := len(l) - len(strings.TrimLeft(l, " "))
		if margin < 0 || indent < margin {
			margin = indent
		}
	}
	if margin <= 0 {
		return s
	}
	for i, l := range lines {
		if len(l) >= margin {
			lines[i] = l[margin:]
		}
	}
	return strings.Join(lines, "\n")
}
