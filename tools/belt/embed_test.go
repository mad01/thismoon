package belt

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/agentdoc/agentdoctest"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// TestOperatingDocRenders is the doc-can't-drift gate: the embedded template
// must render against the component Facts with no leftover placeholders.
func TestOperatingDocRenders(t *testing.T) {
	agentdoctest.VerifyOperatingDoc(t, OperatingDoc, Facts())
}

// TestOperatingDocMentionsDefaults keeps the pins the shared gate does not
// cover: the config path and the doctor command.
func TestOperatingDocMentionsDefaults(t *testing.T) {
	out, err := agentdoc.Render(OperatingDoc, Facts())
	if err != nil {
		t.Fatalf("Render(OperatingDoc) error = %v", err)
	}
	if !strings.Contains(out, DefaultConfigPath) {
		t.Errorf("rendered doc does not mention %s", DefaultConfigPath)
	}
	if !strings.Contains(out, "belt doctor") {
		t.Errorf("rendered doc does not mention belt doctor")
	}
}

// TestDefaultConfigPathMatchesConfig pins the doc's config path to the one
// internal/config actually loads, so the two cannot drift.
func TestDefaultConfigPathMatchesConfig(t *testing.T) {
	p, err := config.DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths() error = %v", err)
	}
	if got, want := config.ExpandHome(DefaultConfigPath), p.BeltYAML; got != want {
		t.Errorf("ExpandHome(DefaultConfigPath) = %q, want %q", got, want)
	}
}
