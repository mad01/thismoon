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

// TestBrainWriteRefusesAFilenameOutsideTheTier pins that brain_write takes a
// file name, never a path out of its tier.
func TestBrainWriteRefusesAFilenameOutsideTheTier(t *testing.T) {
	root := setupFixtureVault(t)
	h := testServer(t)
	_, _, err := h.handleWrite(context.Background(), nil, writeInput{
		Tier: "inbox", Title: "Esc", Filename: "../../escaped",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid path") {
		t.Fatalf("brain_write = %v, want an invalid path refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(root), "escaped.md")); statErr == nil {
		t.Error("brain_write created a file outside the vault")
	}
}

// TestWriteToolsRefuseUnsafeTitlesAndMOCs pins that the MCP write tools
// share the vault's title and MOC checks.
func TestWriteToolsRefuseUnsafeTitlesAndMOCs(t *testing.T) {
	root := setupFixtureVault(t)
	ctx := context.Background()
	h := testServer(t)
	if _, _, err := h.handleWrite(ctx, nil, writeInput{
		Tier: "inbox", Title: "two\nlines",
	}); err == nil || !strings.Contains(err.Error(), "control character") {
		t.Errorf("brain_write = %v, want a control character refusal", err)
	}
	if _, _, err := h.handleEdit(ctx, nil, editInput{
		Path: "notes/concept-alpha.md", FrontmatterPatch: &frontmatterIn{Title: "red \x1b[31m"},
	}); err == nil || !strings.Contains(err.Error(), "control character") {
		t.Errorf("brain_edit = %v, want a control character refusal", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "inbox"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("fixture inbox: %v", err)
	}
	inbox := "inbox/" + entries[0].Name()
	if _, _, err := h.handlePromote(ctx, nil, promoteInput{
		Path: inbox, MOC: filepath.Join(filepath.Dir(root), "outside.md"),
	}); err == nil || !strings.Contains(err.Error(), "invalid path") {
		t.Errorf("brain_promote = %v, want an invalid path refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(root), "outside.md")); statErr == nil {
		t.Error("brain_promote appended to a file outside the vault")
	}
}
