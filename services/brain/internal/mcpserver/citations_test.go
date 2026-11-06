package mcpserver

import (
	"context"
	"strings"
	"testing"
)

// TestWriteToolsWarnAboutCitations pins that brain_write and brain_edit
// return the CLI's citation warnings.
func TestWriteToolsWarnAboutCitations(t *testing.T) {
	setupFixtureVault(t)
	ctx := context.Background()
	h := testServer(t)
	_, res, err := h.handleWrite(ctx, nil, writeInput{
		Tier: "notes", Title: "Cites", Body: "See [[raw-thought]].",
	})
	if err != nil {
		t.Fatalf("brain_write: %v", err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "which is raw") {
		t.Fatalf("brain_write warnings = %q, want the raw citation", res.Warnings)
	}
	_, res, err = h.handleEdit(ctx, nil, editInput{Path: res.Path, Append: "No new links."})
	if err != nil {
		t.Fatalf("brain_edit: %v", err)
	}
	if len(res.Warnings) != 1 {
		t.Errorf("brain_edit warnings = %q, want the citation the body still makes", res.Warnings)
	}
}
