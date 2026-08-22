package worklog

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

// TestOperatingDocMentionsStoreRoot keeps the store-root pin the shared gate
// does not cover.
func TestOperatingDocMentionsStoreRoot(t *testing.T) {
	out, err := agentdoc.Render(OperatingDoc, Facts())
	if err != nil {
		t.Fatalf("Render(OperatingDoc) error = %v", err)
	}
	if !strings.Contains(out, DefaultRoot) {
		t.Errorf("rendered doc does not mention %s", DefaultRoot)
	}
}
