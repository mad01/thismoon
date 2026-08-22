package suspenders

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// TestOperatingDocRenders is the doc-can't-drift gate: the embedded template
// must render against the component Facts with no leftover placeholders.
func TestOperatingDocRenders(t *testing.T) {
	out, err := agentdoc.Render(OperatingDoc, Facts())
	if err != nil {
		t.Fatalf("Render(OperatingDoc) error = %v", err)
	}
	if !strings.Contains(out, "suspenders doctor") {
		t.Errorf("rendered doc does not mention suspenders doctor")
	}
	if strings.Contains(out, "{{") {
		t.Errorf("rendered doc has an unrendered placeholder:\n%s", out)
	}
}
