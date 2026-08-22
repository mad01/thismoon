package agentdoctest

import (
	"testing"

	"github.com/mad01/thismoon/kit/agentdoc"
)

// TestVerifyOperatingDoc pins the helper's happy path: a doc naming the
// binary and base URL with every placeholder rendered passes untouched.
// The failure paths call t.Fatalf/t.Errorf and cannot be exercised without
// faking testing.TB, which its unexported method forbids.
func TestVerifyOperatingDoc(t *testing.T) {
	f := agentdoc.Facts{
		Name: "keeper-of-facts", Bin: "kof",
		BaseURL: "http://kof.this", StorePath: "~/.local/share/kof",
	}
	doc := "run {{.Bin}} against {{.BaseURL}}, store in {{.StorePath}}\n"
	VerifyOperatingDoc(t, doc, f)
}
