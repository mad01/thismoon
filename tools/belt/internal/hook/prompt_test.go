package hook

import (
	"bytes"
	"strings"
	"testing"
)

// TestRunHintPromptToleratesAMistypedPrompt: belt does not decode the
// prompt, so a value of any type is ignored. The run stays silent and leaves
// no decode warning.
func TestRunHintPromptToleratesAMistypedPrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	raw := `{"session_id":"s1","cwd":"` + t.TempDir() + `","prompt":{"text":"the seen file dedupes"}}`
	var out bytes.Buffer
	var a archived
	runner{load: defaults, emit: a.emit}.hints("prompt", strings.NewReader(raw), &out)
	if out.Len() != 0 || len(a.events) != 0 {
		t.Errorf("wrote %q and archived %+v, want silence", out.String(), a.events)
	}
}
