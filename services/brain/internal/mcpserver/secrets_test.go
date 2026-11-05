package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureKey is AWS's documented example access key id: shaped like a real
// one so the scanner fires, and valid nowhere.
const fixtureKey = "AKIAIOSFODNN7EXAMPLE"

// TestWriteToolsRefuseASecret pins that brain_write and brain_edit share the
// CLI's scan: the note is never written and the error names the rule.
func TestWriteToolsRefuseASecret(t *testing.T) {
	root := setupFixtureVault(t)
	ctx := context.Background()
	h := testServer(t)
	_, _, err := h.handleWrite(ctx, nil, writeInput{
		Tier: "inbox", Title: "Leak", Body: "key " + fixtureKey,
	})
	if err == nil || !strings.Contains(err.Error(), "secret found") ||
		!strings.Contains(
			err.Error(),
			"aws-access-key-id",
		) || strings.Contains(err.Error(), fixtureKey) {
		t.Fatalf("brain_write = %v, want a redacted secret refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "inbox", "leak.md")); !os.IsNotExist(statErr) {
		t.Errorf("refused note exists: %v", statErr)
	}
	_, _, err = h.handleEdit(
		ctx,
		nil,
		editInput{Path: "notes/concept-alpha.md", Append: "key " + fixtureKey},
	)
	if err == nil || !strings.Contains(err.Error(), "secret found") {
		t.Fatalf("brain_edit = %v, want a secret refusal", err)
	}
}
