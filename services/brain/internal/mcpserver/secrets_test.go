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

// TestBrainEditRefusesARevocation pins that brain_edit cannot revoke a note
// or revive a tombstone: those go through the CLI's revocation commands.
func TestBrainEditRefusesARevocation(t *testing.T) {
	root := setupFixtureVault(t)
	h := testServer(t)
	path := filepath.Join(root, "notes", "concept-alpha.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"superseded", "withdrawn", "bogus"} {
		_, _, err := h.handleEdit(context.Background(), nil, editInput{
			Path: "notes/concept-alpha.md", FrontmatterPatch: &frontmatterIn{Status: status},
		})
		if err == nil || !strings.Contains(err.Error(), "invalid status change") {
			t.Errorf("brain_edit status %s = %v, want an invalid status change", status, err)
		}
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("note changed to %q", after)
	}
}

// TestBrainEditClearsEvidence pins that an empty evidence in the patch
// removes the field, as the schema says.
func TestBrainEditClearsEvidence(t *testing.T) {
	setupFixtureVault(t)
	h := testServer(t)
	ctx := context.Background()
	const path = "notes/concept-alpha.md"
	for _, step := range []struct {
		evidence, want string
	}{{"B", "B"}, {"", ""}} {
		if _, _, err := h.handleEdit(ctx, nil, editInput{
			Path: path, FrontmatterPatch: &frontmatterIn{Evidence: ptr(step.evidence)},
		}); err != nil {
			t.Fatalf("evidence %q: %v", step.evidence, err)
		}
		_, out, err := h.handleRead(ctx, nil, readInput{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		if out.Frontmatter.Evidence != step.want {
			t.Errorf("after evidence %q, evidence = %q, want %q", step.evidence, out.Frontmatter.Evidence, step.want)
		}
	}
}

func ptr[T any](v T) *T { return &v }
