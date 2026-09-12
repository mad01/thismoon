package mcpserver

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/tools/worklog/internal/store"
)

func TestBuildZeroHintEmptyStore(t *testing.T) {
	s := store.New(t.TempDir())

	hint := buildZeroHint(s, "the status/repo filter matched none")
	if hint == nil {
		t.Fatal("want a hint for an empty store")
	}
	if hint.ItemsStored != 0 {
		t.Errorf("items_stored = %d, want 0", hint.ItemsStored)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "holds no items") {
		t.Errorf("notes = %v, want the empty-store note", hint.Notes)
	}
}

func TestBuildZeroHintFilterMiss(t *testing.T) {
	s := store.New(t.TempDir())
	if _, err := s.Checkpoint("ABC-1234", store.CheckpointInput{
		Where: "state", Note: "n", Repo: "a",
	}); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}

	hint := buildZeroHint(s, `no key or content contains "zebra"`)
	if hint == nil {
		t.Fatal("want a hint")
	}
	if hint.ItemsStored != 1 {
		t.Errorf("items_stored = %d, want 1", hint.ItemsStored)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "1 items stored") {
		t.Errorf("notes = %v, want the filter-miss note with the store count", hint.Notes)
	}
}

// TestToolAnnotationContract holds every registered tool to the repo-wide
// annotation rules: a spec-legal name, a description, an explicit open-world
// hint, and a destructive hint on anything that writes. New only registers
// tools, so an empty config path never gets resolved here.
func TestToolAnnotationContract(t *testing.T) {
	mcptest.VerifyToolAnnotations(t, New("test", ""))
}
