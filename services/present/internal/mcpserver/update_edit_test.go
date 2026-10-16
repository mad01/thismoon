package mcpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// TestUpdateRecompilesUnderATitleChangedMeanwhile pins the update edit's
// re-render: a Doc renders the page title into its hero, so content compiled
// under the title read before the edit is compiled again when another
// writer renamed the page in between.
func TestUpdateRecompilesUnderATitleChangedMeanwhile(t *testing.T) {
	h := &handlers{mode: ModeLocal, now: time.Now}
	in := updateInput{
		ID:      "abcdef0123",
		Content: ptrStr(`{"sections":[{"h":"S","blocks":[{"t":"p","text":"body"}]}]}`),
	}
	compiled, err := resolveUpdate(in, "Old title")
	if err != nil {
		t.Fatalf("resolveUpdate: %v", err)
	}
	cur := store.Snapshot{Page: store.Page{Title: "New title", Content: "<p>x</p>", HasBrief: true}}
	next, err := h.updateEdit(in, compiled)(cur)
	if err != nil {
		t.Fatalf("update edit: %v", err)
	}
	if !strings.Contains(next.Page.Content, "New title") ||
		strings.Contains(next.Page.Content, "Old title") {
		t.Fatalf("content compiled under the stale title:\n%s", next.Page.Content)
	}
	if next.Sources.Doc == nil {
		t.Fatal("the update's Doc did not reach the sources")
	}
}
