package commands

import (
	"testing"

	"github.com/mad01/thismoon/tools/suspenders/internal/history"
	"github.com/mad01/thismoon/tools/suspenders/internal/scanner"
)

// TestAddBlocked_keepsDistinctCasings pins the collection rule for the
// cleanup path: the rewriter replaces case-sensitively, so every casing of
// a blocked name must be collected as its own replacement string.
func TestAddBlocked_keepsDistinctCasings(t *testing.T) {
	h := &historyFindings{
		secrets: make(map[string][]scanner.Finding),
		blocked: make(map[string][]string),
	}
	c := history.Commit{Hash: "abc123"}

	h.addBlocked(c, []string{"AcmeCorp", "acmecorp"})
	h.addBlocked(c, []string{"AcmeCorp"}) // duplicate across calls is dropped

	got := h.blocked[c.Hash]
	if len(got) != 2 || got[0] != "AcmeCorp" || got[1] != "acmecorp" {
		t.Errorf("expected [AcmeCorp acmecorp], got %v", got)
	}
}
