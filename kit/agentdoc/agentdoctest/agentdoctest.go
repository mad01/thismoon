// Package agentdoctest holds the shared test gate every component runs
// over its embedded operating doc, so the per-component embed test is one
// call instead of a hand-rolled copy.
package agentdoctest

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// VerifyOperatingDoc asserts doc renders cleanly against f: Render
// succeeds, the output names the binary and the base URL (when one is
// set), and no template placeholder survives.
func VerifyOperatingDoc(t testing.TB, doc string, f agentdoc.Facts) {
	t.Helper()
	out, err := agentdoc.Render(doc, f)
	if err != nil {
		t.Fatalf("Render(operating doc) error: %v", err)
	}
	if !strings.Contains(out, f.Bin) {
		t.Errorf("rendered doc does not mention the binary %q", f.Bin)
	}
	if f.BaseURL != "" && !strings.Contains(out, f.BaseURL) {
		t.Errorf("rendered doc does not mention the base URL %q", f.BaseURL)
	}
	if strings.Contains(out, "{{") {
		t.Errorf("rendered doc has an unrendered placeholder:\n%s", out)
	}
}
