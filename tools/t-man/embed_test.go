package tman

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

// TestOperatingDocMentionsPaths keeps the pins the shared gate does not
// cover: the plist store and the per-service log directory.
func TestOperatingDocMentionsPaths(t *testing.T) {
	f := Facts()
	out, err := agentdoc.Render(OperatingDoc, f)
	if err != nil {
		t.Fatalf("Render(OperatingDoc) error = %v", err)
	}
	for _, want := range []string{f.StorePath, f.LogPath} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered doc does not mention %s", want)
		}
	}
}
