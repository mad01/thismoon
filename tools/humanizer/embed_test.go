package humanizer

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/agentdoc/agentdoctest"
)

// TestOperatingDocRenders is the doc-can't-drift gate: the embedded template
// must render against the component Facts with no leftover placeholders.
func TestOperatingDocRenders(t *testing.T) {
	agentdoctest.VerifyOperatingDoc(t, OperatingDoc, Facts())
}

// TestOperatingDocMentionsDefaults keeps the pins the shared gate does not
// cover: the docs command itself and the cache directory.
func TestOperatingDocMentionsDefaults(t *testing.T) {
	out, err := agentdoc.Render(OperatingDoc, Facts())
	if err != nil {
		t.Fatalf("Render(OperatingDoc) error = %v", err)
	}
	if !strings.Contains(out, "humanizer docs") {
		t.Errorf("rendered doc does not mention 'humanizer docs'")
	}
	if !strings.Contains(out, DefaultCacheDir) {
		t.Errorf("rendered doc does not mention %s", DefaultCacheDir)
	}
}
