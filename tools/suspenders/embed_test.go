package suspenders

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

// TestOperatingDocMentionsDoctor keeps the doctor-command pin the shared
// gate does not cover.
func TestOperatingDocMentionsDoctor(t *testing.T) {
	out, err := agentdoc.Render(OperatingDoc, Facts())
	if err != nil {
		t.Fatalf("Render(OperatingDoc) error = %v", err)
	}
	if !strings.Contains(out, "suspenders doctor") {
		t.Errorf("rendered doc does not mention suspenders doctor")
	}
}
