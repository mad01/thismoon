package mcpserver

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/mcptest"
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

// TestToolAnnotationContract holds every registered tool to the repo-wide
// annotation rules: a spec-legal name, a description, an explicit open-world
// hint, and a destructive hint on anything that writes.
func TestToolAnnotationContract(t *testing.T) {
	mcptest.VerifyToolAnnotations(t, New("test"))
}
