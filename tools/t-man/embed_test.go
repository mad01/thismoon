package tman

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// TestOperatingDocRenders is the doc-can't-drift gate: the embedded template
// must render against the component Facts with no leftover placeholders.
func TestOperatingDocRenders(t *testing.T) {
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
	if strings.Contains(out, "{{") {
		t.Errorf("rendered doc has an unrendered placeholder:\n%s", out)
	}
}
