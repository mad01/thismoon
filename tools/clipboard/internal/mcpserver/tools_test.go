package mcpserver

import (
	"strings"
	"testing"
)

func TestPasteViewEmptyCarriesNote(t *testing.T) {
	out := pasteView("")
	if out.Text != "" {
		t.Errorf("text = %q, want empty", out.Text)
	}
	if !strings.Contains(out.Note, "non-text content") {
		t.Errorf("note = %q, want the empty-clipboard explanation", out.Note)
	}
}

func TestPasteViewTextHasNoNote(t *testing.T) {
	out := pasteView("hello")
	if out.Text != "hello" {
		t.Errorf("text = %q, want %q", out.Text, "hello")
	}
	if out.Note != "" {
		t.Errorf("note = %q, want empty on a non-empty paste", out.Note)
	}
}
