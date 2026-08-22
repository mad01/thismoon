package agentdoc

import (
	"strings"
	"testing"
)

func TestInstructionsMCPNote(t *testing.T) {
	f := Facts{
		Bin:     "speak",
		Name:    "speak",
		Purpose: "text to speech",
		BaseURL: "http://speak.this",
		MCPNote: "Tools play audio in this process; the web service is separate.",
	}
	got := Instructions(f)
	if !strings.Contains(got, f.MCPNote) {
		t.Fatalf("instructions missing MCPNote: %q", got)
	}
	if strings.Contains(got, "via this stdio shim") {
		t.Fatalf("MCPNote should replace the shim line: %q", got)
	}
	if len(strings.Split(got, "\n")) != 3 {
		t.Fatalf("want 3 lines, got %q", got)
	}
}
